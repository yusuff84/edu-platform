// Runner: изолированное выполнение учебного кода.
// POST /run {"language":"python"|"js"|"htmlcss","code":"...","tests":[{"name","input","expected"}]}
// Лимиты: тело до 200KB, timeout 5s, без сетевого доступа.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	maxBody     = 200 * 1024 // 200KB
	execTimeout = 5 * time.Second
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func runHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, runResponse{Passed: false, Error: "только POST"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, runResponse{Passed: false, Error: "некорректный JSON или тело больше 200KB"})
		return
	}
	if len(req.Code) == 0 {
		writeJSON(w, http.StatusBadRequest, runResponse{Passed: false, Error: "пустой код"})
		return
	}

	switch req.Language {
	case "python":
		runPython(w, req)
	case "js":
		runJS(w, req)
	case "htmlcss":
		runHTML(w, req)
	default:
		writeJSON(w, http.StatusBadRequest, runResponse{Passed: false, Error: "неизвестный язык: используйте python, js или htmlcss"})
	}
}

// runCmd выполняет команду с timeout и stdin, без сети
// (сеть ограничивается на уровне контейнера).
func runCmd(ctx context.Context, stdin, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func checkTests(kind, code string, tests []codeTest, run func(input string) (string, error)) (bool, string, string) {
	for _, t := range tests {
		out, err := run(t.Input)
		if err != nil || strings.TrimSpace(out) != strings.TrimSpace(t.Expected) {
			msg := "тест «" + t.Name + "» не пройден"
			if !t.Hidden {
				msg += ": ожидалось " + strconv.Quote(strings.TrimSpace(t.Expected)) + ", получено " + strconv.Quote(strings.TrimSpace(out))
			}
			return false, out, msg
		}
	}
	return true, "все тесты пройдены", ""
}

func runPython(w http.ResponseWriter, req runRequest) {
	if len(req.Tests) == 0 {
		out, err := runCmd(context.Background(), "", "python3", "-c", req.Code)
		if err != nil {
			writeJSON(w, http.StatusOK, runResponse{Passed: false, Output: out, Error: "ошибка выполнения"})
			return
		}
		writeJSON(w, http.StatusOK, runResponse{Passed: true, Output: out})
		return
	}
	ok, out, msg := checkTests("python", req.Code, req.Tests, func(input string) (string, error) {
		return runCmd(context.Background(), input, "python3", "-c", req.Code)
	})
	writeJSON(w, http.StatusOK, runResponse{Passed: ok, Output: out, Error: msg})
}

func runJS(w http.ResponseWriter, req runRequest) {
	if len(req.Tests) == 0 {
		out, err := runCmd(context.Background(), "", "node", "-e", req.Code)
		if err != nil {
			writeJSON(w, http.StatusOK, runResponse{Passed: false, Output: out, Error: "ошибка выполнения"})
			return
		}
		writeJSON(w, http.StatusOK, runResponse{Passed: true, Output: out})
		return
	}
	ok, out, msg := checkTests("js", req.Code, req.Tests, func(input string) (string, error) {
		return runCmd(context.Background(), input, "node", "-e", req.Code)
	})
	writeJSON(w, http.StatusOK, runResponse{Passed: ok, Output: out, Error: msg})
}

// runHTML эвристика без запуска: проверяет наличие ожидаемых фрагментов
// (поле expected) в коде ученика.
func runHTML(w http.ResponseWriter, req runRequest) {
	var missing []string
	lower := strings.ToLower(req.Code)
	for _, t := range req.Tests {
		want := t.Expected
		if want == "" {
			want = t.Name
		}
		if !strings.Contains(lower, strings.ToLower(want)) {
			missing = append(missing, t.Name)
		}
	}
	if len(missing) > 0 {
		writeJSON(w, http.StatusOK, runResponse{
			Passed: false,
			Output: req.Code,
			Error:  "не найдены фрагменты: " + strings.Join(missing, ", "),
		})
		return
	}
	writeJSON(w, http.StatusOK, runResponse{Passed: true, Output: "все проверки пройдены"})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/run", runHandler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	_ = http.ListenAndServe(":8081", mux)
}
