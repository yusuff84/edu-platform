package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"edu-platform/backend/internal/repo"
)

// CheckQuiz сверяет ответы ученика с квизом.
// answers: questionID -> выбранный индекс варианта. Зачёт — все вопросы верно.
func CheckQuiz(quiz *repo.QuizData, answers map[string]int) (passed bool, correct, total int) {
	if quiz == nil || len(quiz.Questions) == 0 {
		return false, 0, 0
	}
	total = len(quiz.Questions)
	for _, q := range quiz.Questions {
		got, ok := answers[q.ID]
		if !ok {
			continue
		}
		if got == q.Correct {
			correct++
		}
	}
	return correct == total, correct, total
}

// runnerRequest — контракт запроса к code-runner (POST {runnerURL}/run).
type runnerRequest struct {
	Language string          `json:"language"`
	Code     string          `json:"code"`
	Tests    []repo.CodeTest `json:"tests"`
}

type runnerResponse struct {
	Passed  bool   `json:"passed"`
	Output  string `json:"output"`
	Message string `json:"message"`
	ErrorRu string `json:"error_ru"`
}

// CheckCode проксирует код ученика в runner с таймаутом.
// Вызывающий handler должен передать контекст с таймаутом ~5с.
func CheckCode(ctx context.Context, runnerURL, language, code string, tests []repo.CodeTest) (passed bool, message string, err error) {
	language = strings.ToLower(strings.TrimSpace(language))
	switch language {
	case "python", "js", "htmlcss":
	default:
		return false, "", fmt.Errorf("неподдерживаемый язык: %s", language)
	}
	if strings.TrimSpace(code) == "" {
		return false, "", errors.New("пустой код решения")
	}
	body, err := json.Marshal(runnerRequest{Language: language, Code: code, Tests: tests})
	if err != nil {
		return false, "", errors.New("не удалось подготовить запрос к runner")
	}
	url := strings.TrimRight(runnerURL, "/") + "/run"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return false, "", errors.New("не удалось создать запрос к runner")
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, "", errors.New("сервис проверки кода недоступен, попробуйте позже")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, "", errors.New("сервис проверки кода вернул ошибку, попробуйте позже")
	}
	var out runnerResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false, "", errors.New("сервис проверки кода вернул некорректный ответ")
	}
	msg := out.Message
	if msg == "" {
		msg = out.ErrorRu
	}
	return out.Passed, msg, nil
}
