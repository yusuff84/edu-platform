package contract_test

// P11-9: недостающие контрактные проверки, выявленные при аудите покрытия:
//   - регистрация по просроченному приглашению учителя не создаёт пользователя;
//   - code-урок: успешный вердикт runner завершает урок, неуспешный — нет,
//     повторная попытка после неуспеха может завершить урок.
//
// Failed-платёж уже покрыт payments_security_p11_test.go
// (TestP11FailedPaymentDoesNotActivateEnrollment и
// TestP11FailedSucceededDoesNotReactivate): проверяются статус платежа,
// отсутствие активной записи и доступа. Здесь это покрытие не дублируется.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"edu-platform/backend/internal/auth"
	"edu-platform/backend/internal/config"
	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/repo"
)

const p11ExpiredInviteToken = "p11-expired-invite-token"

// Просроченный invite учителя: 400 с текстом про истёкший срок, пользователь
// не создаётся, приглашение остаётся неиспользованным.
func TestP11ExpiredTeacherInviteRejected(t *testing.T) {
	app, router := newTestApp()
	email := "p11-expired-teacher@example.com"
	tokenHash := auth.HashToken(p11ExpiredInviteToken)
	app.Store.CreateInvite(email, tokenHash, "p11exp", "", time.Now().Add(-time.Hour))

	rec := doRequest(t, router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email": email, "password": "password123", "inviteToken": p11ExpiredInviteToken,
	}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("регистрация по просроченному invite: status=%d body=%s, ждали 400", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	msg, _ := out["error"].(string)
	if !strings.Contains(msg, "истёк") {
		t.Fatalf("текст ошибки=%q, ждали упоминание истёкшего срока: %s", msg, rec.Body.String())
	}
	if u, ok := app.Store.GetUserByEmail(email); ok {
		t.Fatalf("просроченный invite создал пользователя: %+v", u)
	}
	inv, ok := app.Store.GetInviteByTokenHash(tokenHash)
	if !ok || inv.Status != repo.InviteSent {
		t.Fatalf("приглашение изменено просроченной регистрацией: %+v", inv)
	}
}

// p11RunnerVerdict — ответ заглушки code-runner на POST /run.
type p11RunnerVerdict struct {
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
}

// newP11Runner поднимает заглушку code-runner: POST /run отвечает вердиктами
// по порядку, последний вердикт повторяется. Возвращает счётчик вызовов.
func newP11Runner(t *testing.T, verdicts ...p11RunnerVerdict) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	if len(verdicts) == 0 {
		t.Fatal("нужен хотя бы один вердикт runner")
	}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/run" {
			t.Errorf("runner: %s %s, ждали POST /run", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		n := int(calls.Add(1)) - 1
		if n >= len(verdicts) {
			n = len(verdicts) - 1
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(verdicts[n])
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

// seedP11CodeLesson собирает приложение с указанным runner, публикует
// бесплатный курс с quiz-уроком, добавляет в него code-урок и записывает
// студента на курс.
func seedP11CodeLesson(t *testing.T, runnerURL string) (app *handler.App, router http.Handler, courseID, lessonID, studentID string, cookies []*http.Cookie) {
	t.Helper()
	app, router = newTestAppWithCfg(&config.Config{
		PaymentsProvider: "mock",
		CommissionPct:    15,
		RunnerURL:        runnerURL,
		AllowedOrigins:   []string{},
	})
	teacherCookies, cid, _ := seedPublishedCourse(t, app, router, 0)
	courseID = cid

	mods := app.Store.ListModulesByCourse(courseID)
	if len(mods) == 0 {
		t.Fatal("в курсе нет модулей")
	}
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
		"moduleId": mods[0].ID, "title": "Печать числа", "kind": "code",
		"codeTask": map[string]any{
			"language": "python", "template": "print(2)",
			"tests": []any{map[string]any{"name": "пример", "input": "", "expected": "2"}},
		},
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create code lesson: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var lesson map[string]any
	decodeBody(t, rec, &lesson)
	lessonID, _ = lesson["id"].(string)
	if lessonID == "" {
		t.Fatalf("в ответе нет id code-урока: %s", rec.Body.String())
	}

	cookies, studentID = registerUser(t, router, "p11-code-verdicts@example.com", "password123", "")
	rec = doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID,
	}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll: status=%d body=%s", rec.Code, rec.Body.String())
	}
	return app, router, courseID, lessonID, studentID, cookies
}

// completeP11CodeLesson отправляет решение и возвращает разобранный ответ
// /me/courses/{id}/lessons/{id}/complete.
func completeP11CodeLesson(t *testing.T, router http.Handler, courseID, lessonID string, cookies []*http.Cookie) map[string]any {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost,
		"/api/v1/me/courses/"+courseID+"/lessons/"+lessonID+"/complete",
		map[string]any{"code": "print(2)", "language": "python"}, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete code lesson: status=%d body=%s, ждали 200", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	return out
}

// Вердикты code-runner на границе API: accepted завершает урок, failed —
// нет, retry после failed завершает успешной попыткой.
func TestP11CodeLessonVerdicts(t *testing.T) {
	t.Run("accepted", func(t *testing.T) {
		runner, calls := newP11Runner(t, p11RunnerVerdict{Passed: true})
		app, router, courseID, lessonID, studentID, cookies := seedP11CodeLesson(t, runner.URL)

		out := completeP11CodeLesson(t, router, courseID, lessonID, cookies)
		if out["passed"] != true || out["completed"] != true {
			t.Fatalf("accepted: passed=%v completed=%v body=%v", out["passed"], out["completed"], out)
		}
		if !app.Store.IsLessonCompleted(studentID, lessonID) {
			t.Fatal("accepted: урок не отмечен пройденным")
		}
		if got := calls.Load(); got != 1 {
			t.Fatalf("accepted: runner вызван %d раз, ждали 1", got)
		}
	})

	t.Run("failed", func(t *testing.T) {
		runner, calls := newP11Runner(t, p11RunnerVerdict{Passed: false, Message: "Неверный вывод"})
		app, router, courseID, lessonID, studentID, cookies := seedP11CodeLesson(t, runner.URL)

		out := completeP11CodeLesson(t, router, courseID, lessonID, cookies)
		if out["passed"] != false || out["completed"] != false {
			t.Fatalf("failed: passed=%v completed=%v body=%v", out["passed"], out["completed"], out)
		}
		if out["message"] != "Неверный вывод" {
			t.Fatalf("failed: message=%v, ждали текст runner", out["message"])
		}
		if app.Store.IsLessonCompleted(studentID, lessonID) {
			t.Fatal("failed: урок ошибочно отмечен пройденным")
		}
		if got := calls.Load(); got != 1 {
			t.Fatalf("failed: runner вызван %d раз, ждали 1", got)
		}
	})

	t.Run("retry", func(t *testing.T) {
		runner, calls := newP11Runner(t,
			p11RunnerVerdict{Passed: false, Message: "Пока неверно"},
			p11RunnerVerdict{Passed: true},
		)
		app, router, courseID, lessonID, studentID, cookies := seedP11CodeLesson(t, runner.URL)

		out := completeP11CodeLesson(t, router, courseID, lessonID, cookies)
		if out["passed"] != false || out["completed"] != false {
			t.Fatalf("retry, попытка 1: passed=%v completed=%v body=%v", out["passed"], out["completed"], out)
		}
		if app.Store.IsLessonCompleted(studentID, lessonID) {
			t.Fatal("retry: урок завершён после неуспешной попытки")
		}

		out = completeP11CodeLesson(t, router, courseID, lessonID, cookies)
		if out["passed"] != true || out["completed"] != true {
			t.Fatalf("retry, попытка 2: passed=%v completed=%v body=%v", out["passed"], out["completed"], out)
		}
		if !app.Store.IsLessonCompleted(studentID, lessonID) {
			t.Fatal("retry: успешная попытка не завершила урок")
		}
		if got := calls.Load(); got != 2 {
			t.Fatalf("retry: runner вызван %d раз, ждали 2", got)
		}
	})
}
