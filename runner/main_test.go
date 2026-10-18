// Тесты runner: контракт /run и /health, общий бюджет времени,
// лимит вывода, семафор конкурентности, статическая проверка htmlcss.
//
// ВАЖНО: тесты на python/js пропускаются (t.Skip), если интерпретатор
// не установлен в текущем окружении: в чистом golang:1.24-bookworm
// python3 обычно уже есть, а nodejs — нет. Реальный прогон с обоими
// интерпретаторами выполняется в этапе сборки Dockerfile (там ставятся
// python3 и nodejs) и командой из README «Проверка runner (python3/nodejs)».
package main

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// requireBin пропускает тест, если интерпретатор недоступен.
func requireBin(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("интерпретатор %s не установлен — тест пропущен", name)
	}
}

func doRun(t *testing.T, rn *runner, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/run", strings.NewReader(body))
	rn.runHandler(rec, req)
	return rec
}

// ---- Контракт -----------------------------------------------------------

func TestHealth(t *testing.T) {
	rec := httptest.NewRecorder()
	rn := newRunner()
	rn.handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("GET /health: код %d, тело %q", rec.Code, rec.Body.String())
	}
}

func TestRunRejectsGET(t *testing.T) {
	rec := httptest.NewRecorder()
	newRunner().runHandler(rec, httptest.NewRequest(http.MethodGet, "/run", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /run: ожидался 405, получен %d", rec.Code)
	}
}

func TestUnknownLanguage(t *testing.T) {
	rec := doRun(t, newRunner(), `{"language":"ruby","code":"puts 1"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("неизвестный язык: ожидался 400, получен %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "error_ru") {
		t.Fatalf("нет error_ru в ответе: %s", rec.Body.String())
	}
}

func TestEmptyCode(t *testing.T) {
	rec := doRun(t, newRunner(), `{"language":"python","code":"   "}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("пустой код: ожидался 400, получен %d", rec.Code)
	}
}

// ---- python -------------------------------------------------------------

func TestPythonSuccess(t *testing.T) {
	requireBin(t, "python3")
	body := `{"language":"python","code":"x=int(input())\nprint(x*2)","tests":[{"name":"удвоение","input":"21","expected":"42"}]}`
	rec := doRun(t, newRunner(), body)
	if rec.Code != http.StatusOK {
		t.Fatalf("код ответа %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"passed":true`) {
		t.Fatalf("ожидался passed=true: %s", rec.Body.String())
	}
}

func TestPythonFailureShowsDiff(t *testing.T) {
	requireBin(t, "python3")
	body := `{"language":"python","code":"print('мир')","tests":[{"name":"приветствие","input":"","expected":"привет"}]}`
	rec := doRun(t, newRunner(), body)
	if !strings.Contains(rec.Body.String(), `"passed":false`) {
		t.Fatalf("ожидался passed=false: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "не пройден") {
		t.Fatalf("нет сообщения о провале теста: %s", rec.Body.String())
	}
}

// ---- js -----------------------------------------------------------------

func TestJSSuccess(t *testing.T) {
	requireBin(t, "node")
	code := `const fs=require('fs');const d=fs.readFileSync(0,'utf8').trim();console.log(Number(d)*2);`
	body := `{"language":"js","code":` + jsonQuote(code) + `,"tests":[{"name":"удвоение","input":"21","expected":"42"}]}`
	rec := doRun(t, newRunner(), body)
	if !strings.Contains(rec.Body.String(), `"passed":true`) {
		t.Fatalf("ожидался passed=true: %s", rec.Body.String())
	}
}

// ---- Лимиты -------------------------------------------------------------

// TestRequestBudgetKillsInfiniteLoop — бесконечный цикл укладывается
// в общий бюджет запроса, а не в потолок на тест: execTimeout заведомо
// больше бюджета, «виноват» именно бюджет.
func TestRequestBudgetKillsInfiniteLoop(t *testing.T) {
	requireBin(t, "python3")
	rn := newRunner()
	rn.budget = 1 * time.Second
	rn.execTimeout = 30 * time.Second // заведомо больше бюджета

	start := time.Now()
	rec := doRun(t, rn, `{"language":"python","code":"while True: pass","tests":[{"name":"т","input":"","expected":""}]}`)
	elapsed := time.Since(start)

	if elapsed > 10*time.Second {
		t.Fatalf("бесконечный цикл держал соединение %v — бюджет не сработал", elapsed)
	}
	if !strings.Contains(rec.Body.String(), `"passed":false`) {
		t.Fatalf("ожидался passed=false: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "превышено время выполнения") {
		t.Fatalf("нет сообщения о превышении бюджета: %s", rec.Body.String())
	}
}

// TestOutputLimit — программа печатает мегабайты: процесс не падает,
// вывод обрезается с пометкой.
func TestOutputLimit(t *testing.T) {
	requireBin(t, "python3")
	rn := newRunner()
	rn.maxOutput = 1024

	rec := doRun(t, rn, `{"language":"python","code":"print('x' * 1000000)","tests":[]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("код ответа %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"passed":true`) {
		t.Fatalf("программа с большим выводом не должна падать: %s", rec.Body.String())
	}

	// Тело ответа — ограниченный буфер + пометка, а не мегабайты.
	if rec.Body.Len() > rn.maxOutput*4 {
		t.Fatalf("вывод не ограничен: %d байт в ответе", rec.Body.Len())
	}
	if !strings.Contains(rec.Body.String(), "вывод обрезан") {
		t.Fatalf("нет пометки об обрезке вывода")
	}
}

// TestConcurrencyRejected — при заполненном семафоре новый запрос
// получает 503 немедленно, а не висит в очереди.
func TestConcurrencyRejected(t *testing.T) {
	rn := newRunner()
	for i := 0; i < cap(rn.sem); i++ {
		rn.sem <- struct{}{} // занимаем все слоты, как будто идут проверки
	}
	start := time.Now()
	rec := doRun(t, rn, `{"language":"python","code":"print(1)","tests":[{"name":"т","input":"","expected":"1"}]}`)
	elapsed := time.Since(start)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("ожидался 503 при переполнении семафора, получен %d: %s", rec.Code, rec.Body.String())
	}
	if elapsed > 2*time.Second {
		t.Fatalf("503 должен приходить мгновенно, заняло %v", elapsed)
	}

	// После освобождения слотов проверка снова работает (нужен python3).
	requireBin(t, "python3")
	for i := 0; i < cap(rn.sem); i++ {
		<-rn.sem
	}
	rec = doRun(t, rn, `{"language":"python","code":"print(1)","tests":[{"name":"т","input":"","expected":"1"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("после освобождения семафора ожидался 200, получен %d", rec.Code)
	}
}

// ---- htmlcss (статический анализатор, без интерпретаторов) --------------

const htmlCode = `<!DOCTYPE html>
<html>
<head><style>body { color: red; }</style></head>
<body><h1 class="title">Привет</h1></body>
</html>`

func TestHTMLCSSPass(t *testing.T) {
	body := `{"language":"htmlcss","code":` + jsonQuote(htmlCode) + `,"tests":[` +
		`{"name":"заголовок","expected":"<h1>"},` +
		`{"name":"класс title","expected":"class=\"title\""},` +
		`{"name":"цвет текста","expected":"color: red"},` +
		`{"name":"одиночное свойство","expected":"color"}]}`
	rec := doRun(t, newRunner(), body)
	if !strings.Contains(rec.Body.String(), `"passed":true`) {
		t.Fatalf("ожидался passed=true: %s", rec.Body.String())
	}
}

func TestHTMLCSSStyleInsensitivity(t *testing.T) {
	// Регистр, кавычки и лишние пробелы не должны ломать проверку.
	code := `<DIV CLASS = 'Title'>Hi</DIV><STYLE>body{COLOR:RED}</STYLE>`
	body := `{"language":"htmlcss","code":` + jsonQuote(code) + `,"tests":[` +
		`{"name":"div","expected":"<div class=title>"},` +
		`{"name":"цвет","expected":"color:red"}]}`
	rec := doRun(t, newRunner(), body)
	if !strings.Contains(rec.Body.String(), `"passed":true`) {
		t.Fatalf("ожидался passed=true при нормализации стиля: %s", rec.Body.String())
	}
}

func TestHTMLCSSElementNotMentionedInText(t *testing.T) {
	// Слово «color» в тексте страницы не должно засчитываться как CSS-свойство.
	code := `<p>Цвет color — это просто текст</p>`
	body := `{"language":"htmlcss","code":` + jsonQuote(code) + `,"tests":[{"name":"свойство color","expected":"color"}]}`
	rec := doRun(t, newRunner(), body)
	if !strings.Contains(rec.Body.String(), `"passed":false`) {
		t.Fatalf("упоминание слова в тексте не должно проходить проверку: %s", rec.Body.String())
	}
}

func TestHTMLCSSErrorExplainsLimitation(t *testing.T) {
	body := `{"language":"htmlcss","code":"<p>нет нужного тега</p>","tests":[{"name":"таблица","expected":"<table>"}]}`
	rec := doRun(t, newRunner(), body)
	s := rec.Body.String()
	if !strings.Contains(s, "не найдены фрагменты") {
		t.Fatalf("нет перечня недостающих фрагментов: %s", s)
	}
	if !strings.Contains(s, "статическая проверка") {
		t.Fatalf("error_ru должен честно объяснять ограничение проверки: %s", s)
	}
}

// ---- утилиты ------------------------------------------------------------

// jsonQuote — минимальная обёртка, чтобы не тащить encoding/json в тесты.
func jsonQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// limitedWriter — юнит-тест обрезки на уровне writer.
func TestLimitedWriter(t *testing.T) {
	l := &limitedWriter{limit: 10}
	n, err := l.Write([]byte("0123456789ABCDEF"))
	if err != nil || n != 16 {
		t.Fatalf("Write: n=%d err=%v", n, err)
	}
	if l.buf.String() != "0123456789" || !l.truncated {
		t.Fatalf("буфер %q, truncated=%v", l.buf.String(), l.truncated)
	}
	// Последующие записи проглатываются без ошибок.
	more := []byte("ещё")
	if n, err := l.Write(more); err != nil || n != len(more) {
		t.Fatalf("Write после лимита: n=%d err=%v", n, err)
	}
}
