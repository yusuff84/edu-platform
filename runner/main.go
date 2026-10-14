// Runner: изолированное выполнение учебного кода.
//
// Контракт с бэкендом (backend/internal/service/checker.go) — НЕ МЕНЯТЬ:
//
//	POST /run  {"language":"python"|"js"|"htmlcss","code":"...","tests":[{"name","input","expected","hidden"}]}
//	ответ 200  {"passed":bool,"output":"...","error_ru":"..."}
//	GET  /health -> "ok"
//
// Уровни защиты (подробности — в отчёте T11):
//   - общий бюджет времени на ВЕСЬ запрос (context.WithTimeout в runHandler),
//     а не на каждый тест;
//   - потолок времени на один тест (min(5 с, остаток бюджета));
//   - семафор конкурентности: при переполнении — мгновенный 503;
//   - лимит вывода процесса (256 КБ), дальше байты отбрасываются;
//   - код кладётся во временный файл (os.MkdirTemp, 0700), а не в argv
//     (python3 -c / node -e упираются в длину аргумента и требуют экранирования);
//   - cmd.Env — белый список переменных, рабочий каталог — временный каталог;
//   - http.Server с ReadHeaderTimeout/ReadTimeout/WriteTimeout/IdleTimeout
//     и MaxHeaderBytes.
//
// Сеть, память, CPU и права процесса ограничиваются на уровне контейнера
// (см. рекомендованный блок docker-compose.yml в отчёте T11): внутренняя
// сеть без выхода в интернет, read_only, cap_drop: [ALL], tmpfs на /tmp,
// mem/cpu-limits, pids_limit. Этот файл сознательно не содержит настроек
// процесс-групп (SysProcAttr): они платформозависимы, а exec.CommandContext
// — основной механизм убийства зависших программ.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// maxBody — максимальный размер тела запроса.
	maxBody = 200 * 1024 // 200 КБ

	// maxOutput — лимит вывода одного запуска процесса (stdout+stderr).
	// Дальше байты отбрасываются, процесс не убивается, в вывод
	// добавляется пометка об обрезке.
	maxOutput = 256 * 1024 // 256 КБ

	// execTimeout — потолок времени на ОДИН тест. Фактически действует
	// min(execTimeout, остаток бюджета запроса).
	execTimeout = 5 * time.Second

	// requestBudget — общий бюджет времени на весь запрос /run.
	// Намеренно меньше 5 с: backend/internal/service/checker.go держит
	// http.Client{Timeout: 5s}, поэтому ответ обязан успеть вернуться.
	// Если бэкенд поднимет свой таймаут — поднимите и это значение.
	requestBudget = 4 * time.Second

	// maxConcurrent — одновременных исполнений кода учеников.
	// Превышение → мгновенный 503, без ожидания в очереди.
	maxConcurrent = 4

	// Таймауты HTTP-сервера.
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second // тело запроса читается ≤ 10 с
	writeTimeout      = 60 * time.Second // ответ никогда не «висит» бесконечно
	idleTimeout       = 60 * time.Second // keep-alive соединения
	maxHeaderBytes    = 16 * 1024        // 16 КБ заголовков

	// port — порт сервиса.
	port = ":8081"
)

type codeTest struct {
	Name     string `json:"name"`
	Input    string `json:"input"`
	Expected string `json:"expected"`
	Hidden   bool   `json:"hidden,omitempty"`
}

type runRequest struct {
	Language string     `json:"language"`
	Code     string     `json:"code"`
	Tests    []codeTest `json:"tests"`
}

type runResponse struct {
	Passed bool   `json:"passed"`
	Output string `json:"output"`
	Error  string `json:"error_ru,omitempty"`
}

// runner — состояние сервиса. Лимиты вынесены в поля, чтобы тесты могли
// переопределять их без изменения production-констант.
type runner struct {
	sem         chan struct{} // семафор конкурентности
	budget      time.Duration // общий бюджет запроса
	execTimeout time.Duration // потолок на один тест
	maxOutput   int           // лимит вывода одного запуска
}

func newRunner() *runner {
	return &runner{
		sem:         make(chan struct{}, maxConcurrent),
		budget:      requestBudget,
		execTimeout: execTimeout,
		maxOutput:   maxOutput,
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // ошибка записи ответа клиенту нам не важна
}

// limitedWriter — io.Writer с жёстким лимитом.
// После заполнения буфера новые байты отбрасываются, но Write возвращает
// успех (len(p), nil): иначе exec упадёт с pipe-ошибкой, и мы не отличим
// «программа печатала гигабайты» от «программа упала». Факт обрезки
// фиксируется в поле truncated и отражается в выводе.
type limitedWriter struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.truncated {
		return len(p), nil // уже переполнено — просто проглатываем
	}
	rest := l.limit - l.buf.Len()
	if rest <= 0 {
		l.truncated = true
		return len(p), nil
	}
	if len(p) > rest {
		_, _ = l.buf.Write(p[:rest])
		l.truncated = true
		return len(p), nil
	}
	return l.buf.Write(p)
}

// execEnv — белый список переменных окружения для кода ученика.
// Окружение процесса runner НЕ наследуется (в нем нет секретов, но и
// полезных переменных нет — детерминизм важнее).
func execEnv(dir string) []string {
	return []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=" + dir,
		"TMPDIR=" + dir,
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"PYTHONDONTWRITEBYTECODE=1", // cat /tmp — read-only, кэш Python не нужен
		"PYTHONIOENCODING=utf-8",
	}
}

// runCmd выполняет файл программы через интерпретатор внутри каталога dir.
// Дедлайн = min(execTimeout, дедлайн ctx запроса). Вывод ограничивается
// rn.maxOutput байтами. Сеть процессу не запрещается кодом — это работа
// контейнера (internal-сеть в docker-compose.yml).
func (rn *runner) runCmd(ctx context.Context, dir, stdin, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, rn.execTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...) // при дедлайне убивает процесс (SIGKILL)
	cmd.Dir = dir                                  // рабочий каталог — временный каталог
	cmd.Env = execEnv(dir)                         // белый список переменных
	cmd.Stdin = strings.NewReader(stdin)           // stdin — только input текущего теста

	out := &limitedWriter{limit: rn.maxOutput}
	cmd.Stdout = out
	cmd.Stderr = out

	err := cmd.Run()
	text := out.buf.String()
	if out.truncated {
		text += "\n…вывод обрезан: превышен лимит " + strconv.Itoa(rn.maxOutput/1024) + " КБ"
	}
	return text, err
}

// budgetMsg — сообщение, когда время запроса вышло.
func (rn *runner) budgetMsg() string {
	return "превышено время выполнения: общий бюджет проверки " + rn.budget.String()
}

// runError — текст ошибки запуска: если «виноват» бюджет — говорим об этом,
// иначе — стандартное «ошибка выполнения» (диагностика в поле output).
func (rn *runner) runError(ctx context.Context) string {
	if ctx.Err() != nil {
		return rn.budgetMsg()
	}
	return "ошибка выполнения"
}

// checkTests прогоняет тесты по одному и тому же файлу программы.
// Цикл прерывается немедленно, когда истёк общий бюджет запроса.
func (rn *runner) checkTests(ctx context.Context, dir, interpreter, file string, tests []codeTest) (bool, string, string) {
	var out string
	for _, t := range tests {
		if ctx.Err() != nil {
			return false, out, rn.budgetMsg()
		}
		var err error
		out, err = rn.runCmd(ctx, dir, t.Input, interpreter, file)
		if ctx.Err() != nil {
			return false, out, rn.budgetMsg()
		}
		if err != nil || normalizeOutput(out) != normalizeOutput(t.Expected) {
			msg := "тест «" + t.Name + "» не пройден"
			if !t.Hidden {
				msg += ": ожидалось " + strconv.Quote(normalizeOutput(t.Expected)) +
					", получено " + strconv.Quote(normalizeOutput(out))
			}
			return false, out, msg
		}
	}
	return true, "все тесты пройдены", ""
}

// normalizeOutput — сравнение вывода: trim + выравнивание переводов строк
// (CRLF из Windows не должен ронять тест).
func normalizeOutput(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimSpace(s)
}

// runLanguage — общий путь для python и js: код во временный файл,
// запуск «интерпретатор <файл>», проверка тестов.
func (rn *runner) runLanguage(ctx context.Context, w http.ResponseWriter, req runRequest, interpreter, ext string) {
	if _, err := exec.LookPath(interpreter); err != nil {
		writeJSON(w, http.StatusOK, runResponse{
			Passed: false,
			Error:  "интерпретатор " + interpreter + " не установлен на сервере проверки, попробуйте позже",
		})
		return
	}

	// Временный каталог под код ученика: 0700, удаляется всегда.
	dir, err := os.MkdirTemp("", "runner-")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, runResponse{Passed: false, Error: "не удалось подготовить рабочий каталог"})
		return
	}
	defer os.RemoveAll(dir)

	file := filepath.Join(dir, "solution"+ext)
	if err := os.WriteFile(file, []byte(req.Code), 0o600); err != nil {
		writeJSON(w, http.StatusInternalServerError, runResponse{Passed: false, Error: "не удалось записать код решения"})
		return
	}

	if len(req.Tests) == 0 {
		// Без тестов — просто выполняем и возвращаем вывод.
		out, err := rn.runCmd(ctx, dir, "", interpreter, file)
		if err != nil {
			writeJSON(w, http.StatusOK, runResponse{Passed: false, Output: out, Error: rn.runError(ctx)})
			return
		}
		writeJSON(w, http.StatusOK, runResponse{Passed: true, Output: out})
		return
	}

	ok, out, msg := rn.checkTests(ctx, dir, interpreter, file, req.Tests)
	writeJSON(w, http.StatusOK, runResponse{Passed: ok, Output: out, Error: msg})
}

// ---------------------------------------------------------------------------
// htmlcss — статический анализатор, НЕ рендеринг.
//
// Честное предупреждение: runner не отображает страницу и не применяет CSS.
// Здесь нет браузера — проверяется только то, что нужные конструкции
// присутствуют в коде ученика (теги, атрибуты, CSS-правила), после
// нормализации регистра, пробелов и кавычек. Headless-браузер намеренно
// НЕ добавляется: он раздул бы образ на сотни мегабайт (нарушение
// Docker-First) и расширил бы атакующую поверхность песочницы.
//
// Ограничения, о которых честно сказано студенту в error_ru:
//   - вложенность и корректность DOM не проверяются;
//   - сопоставление — по подстроке, возможны ложные совпадения;
//   - визуальное отображение, картинки и шрифты не проверяются.
// ---------------------------------------------------------------------------

var (
	// wsRe — любая пробельная последовательность → один пробел.
	wsRe = regexp.MustCompile(`\s+`)
	// punctRe — пробелы вокруг значимых знаков разметки/CSS не значимы:
	// `color : red` == `color:red`, `class = "x"` == `class=x`.
	// Знаки < и > НЕ входят: они разбираются структурно (см. containsConstruct).
	punctRe = regexp.MustCompile(`\s*([{}();:,=])\s*`)
	// quoteRe — стиль кавычек не значим: class="x" == class='x' == class=x.
	quoteRe = regexp.MustCompile(`["']`)
	// openTagRe — требование-тег вида «<h1>» или «<br».
	openTagRe = regexp.MustCompile(`^<\s*([a-z][a-z0-9]*)\s*>?$`)
	// closeTagRe — требование-закрывающий тег вида «</h1>».
	closeTagRe = regexp.MustCompile(`^</\s*([a-z][a-z0-9]*)\s*>$`)
	// attrRe — атрибут внутри требования: name = "value" | 'value' | value.
	attrRe = regexp.MustCompile(`([a-z-]+)\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
)

// tagInCodeRe ищет открывающий или закрывающий тег в исходном коде
// (регистронезависимо, лишние пробелы внутри тега допустимы).
func tagInCodeRe(tag string) *regexp.Regexp {
	return regexp.MustCompile(`<\s*/?\s*` + regexp.QuoteMeta(tag) + `[\s/>]`)
}

// closeTagInCodeRe ищет закрывающий тег </tag> в исходном коде.
func closeTagInCodeRe(tag string) *regexp.Regexp {
	return regexp.MustCompile(`</\s*` + regexp.QuoteMeta(tag) + `\s*>`)
}

// tagNameOf извлекает имя тега из строки вида «<img src=...>».
// Возвращает "", если имя прочитать не удалось (например, «<!doctype ...>»).
func tagNameOf(lw string) string {
	s := strings.TrimPrefix(lw, "<")
	s = strings.TrimPrefix(s, "/")
	i := strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	if i <= 0 {
		return ""
	}
	return s[:i]
}

// normalizeMarkup приводит HTML/CSS-текст к каноничному виду для сравнения:
// нижний регистр, CRLF→LF, схлопнутые пробелы, без пробелов вокруг
// знаков разметки, без кавычек.
func normalizeMarkup(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = wsRe.ReplaceAllString(s, " ")
	s = punctRe.ReplaceAllString(s, "$1")
	s = quoteRe.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// isSingleToken — требование-одиночное слово: свойство CSS (`color`),
// имя атрибута (`class`), селектор (`.title`), тег (`h1`).
func isSingleToken(w string) bool {
	return w != "" && !strings.ContainsAny(w, " \t\r\n<>{}();:,=\"'")
}

// singleTokenRe — одиночное слово должно использоваться как конструкция,
// а не встречаться в тексте страницы: `class` требует class= / class:,
// `color` — color:, `.title` — селектор .title{...}.
func singleTokenRe(w string) *regexp.Regexp {
	// перед словом: начало, пробел, точка/решётка (селектор), { } " ' ( < : =
	// после слова: : = { ; } ) >
	return regexp.MustCompile(`(^|[\s.#,{"'(<:=])` + regexp.QuoteMeta(w) + `([:={;,)}>]|$)`)
}

// containsConstruct проверяет одно требование в коде ученика.
//
// Правила:
//   - «<h1>» / «<h1»  — тег h1 должен присутствовать в коде (с атрибутами
//     или без), точное совпадение подстроки не требуется;
//   - «</h1>»         — должен присутствовать закрывающий тег;
//   - «<img src="x">» — тег img обязателен, и каждый указанный атрибут
//     (src=x) должен встречаться в коде;
//   - «color», «.title», «class» — одиночное слово должно использоваться
//     как CSS-свойство / селектор / атрибут, а не быть частью текста;
//   - остальное («color: red», «display: flex») — нормализованная подстрока.
func containsConstruct(rawLower, normCode, want string) bool {
	w := strings.TrimSpace(want)
	if w == "" {
		return true // пустое требование считается выполненным
	}
	lw := strings.ToLower(w)

	if m := openTagRe.FindStringSubmatch(lw); m != nil {
		return tagInCodeRe(m[1]).MatchString(rawLower)
	}
	if m := closeTagRe.FindStringSubmatch(lw); m != nil {
		return closeTagInCodeRe(m[1]).MatchString(rawLower)
	}
	if strings.HasPrefix(lw, "<") && strings.Contains(lw, ">") {
		if tag := tagNameOf(lw); tag != "" {
			if !tagInCodeRe(tag).MatchString(rawLower) {
				return false
			}
			// Каждый атрибут из требования обязан встречаться в коде.
			for _, m := range attrRe.FindAllStringSubmatch(lw, -1) {
				if !strings.Contains(normCode, normalizeMarkup(m[0])) {
					return false
				}
			}
			return true
		}
		// Нечто вроде «<!doctype html>» — сравниваем как нормализованную подстроку.
		return strings.Contains(normCode, normalizeMarkup(lw))
	}
	if isSingleToken(lw) {
		return singleTokenRe(lw).MatchString(normCode)
	}
	return strings.Contains(normCode, normalizeMarkup(lw))
}

// runHTML — статическая проверка HTML/CSS-задания (см. комментарий выше).
// Формат ответа не меняется: output — код ученика при ошибке,
// «все проверки пройдены» при успехе; error_ru — что именно не найдено
// и честное предупреждение о границах проверки.
func (rn *runner) runHTML(w http.ResponseWriter, req runRequest) {
	rawLower := strings.ToLower(req.Code)
	normCode := normalizeMarkup(req.Code)
	var missing []string
	for _, t := range req.Tests {
		want := strings.TrimSpace(t.Expected)
		if want == "" {
			want = strings.TrimSpace(t.Name) // обратная совместимость: без expected ждём фрагмент из name
		}
		if !containsConstruct(rawLower, normCode, want) {
			missing = append(missing, t.Name)
		}
	}
	if len(missing) > 0 {
		writeJSON(w, http.StatusOK, runResponse{
			Passed: false,
			Output: req.Code,
			Error: "не найдены фрагменты: " + strings.Join(missing, ", ") +
				". Это статическая проверка: страница не рендерится, runner ищет теги, атрибуты и CSS-правила в коде",
		})
		return
	}
	writeJSON(w, http.StatusOK, runResponse{Passed: true, Output: "все проверки пройдены"})
}

// handler собирает маршруты.
func (rn *runner) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/run", rn.runHandler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

func (rn *runner) runHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, runResponse{Passed: false, Error: "только POST"})
		return
	}

	// Семафор: при переполнении отвечаем 503 немедленно, а не ставим в очередь.
	select {
	case rn.sem <- struct{}{}:
		defer func() { <-rn.sem }()
	default:
		writeJSON(w, http.StatusServiceUnavailable, runResponse{
			Passed: false,
			Error:  "сервис проверки кода перегружен, повторите попытку чуть позже",
		})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, runResponse{Passed: false, Error: "некорректный JSON или тело больше 200KB"})
		return
	}
	if strings.TrimSpace(req.Code) == "" {
		writeJSON(w, http.StatusBadRequest, runResponse{Passed: false, Error: "пустой код"})
		return
	}

	// Общий бюджет времени на весь запрос. r.Context() в base — если бэкенд
	// оборвал соединение (свой таймаут), исполнение тоже останавливается.
	ctx, cancel := context.WithTimeout(r.Context(), rn.budget)
	defer cancel()

	switch strings.ToLower(strings.TrimSpace(req.Language)) {
	case "python":
		rn.runLanguage(ctx, w, req, "python3", ".py")
	case "js":
		rn.runLanguage(ctx, w, req, "node", ".js")
	case "htmlcss":
		rn.runHTML(w, req)
	default:
		writeJSON(w, http.StatusBadRequest, runResponse{Passed: false, Error: "неизвестный язык: используйте python, js или htmlcss"})
	}
}

func main() {
	rn := newRunner()

	srv := &http.Server{
		Addr:              port,
		Handler:           rn.handler(),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}

	// Корректное завершение по SIGTERM/SIGINT (docker stop).
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		log.Println("runner: получено сигнал завершения, останавливаемся")
		if err := srv.Shutdown(context.Background()); err != nil {
			log.Printf("runner: ошибка при остановке: %v", err)
		}
	}()

	log.Printf("runner: слушаю %s (бюджет запроса %s, до %d параллельных запусков)", port, rn.budget, maxConcurrent)
	// Ошибка ListenAndServe игнорируется только для штатного ErrServerClosed.
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("runner: сервер остановлен с ошибкой: %v", err)
	}
	log.Println("runner: корректно остановлен")
}
