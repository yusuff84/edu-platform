package handler

// Тесты CORS-префлайта (T42, закрытие находки T20): Access-Control-Allow-Methods
// должен перечислять реально зарегистрированные методы запрошенного пути +
// OPTIONS, а не быть жёсткой константой «GET, POST, PUT, PATCH, DELETE, OPTIONS»
// для всех маршрутов, включая несуществующие.
//
// Роутер собирается через newTestRouter (security_test.go): он же владеет
// Mux, по которому cors считает методы через Match, — тест проверяет связку
// NewRouter + middleware, как в реальном приложении.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// corsAllowedOrigin — Origin из devConfig() (security_test.go): разрешён CORS.
const corsAllowedOrigin = "http://localhost:3000"

// optReq шлёт preflight-запрос (OPTIONS + Origin) и возвращает ответ.
func optReq(t *testing.T, router http.Handler, path, origin string) *httptest.ResponseRecorder {
	t.Helper()
	return doJSON(t, router, http.MethodOptions, path, "", func(r *http.Request) {
		r.Header.Set("Origin", origin)
		r.Header.Set("Access-Control-Request-Method", "POST")
		r.Header.Set("Access-Control-Request-Headers", "content-type")
	})
}

// hasMethod проверяет наличие метода в списке Allow-Methods по элементу,
// а не подстрокой («GET» не должен матчиться внутри «GET, OPTIONS» неточно).
func hasMethod(list, method string) bool {
	for _, m := range strings.Split(list, ",") {
		if strings.TrimSpace(m) == method {
			return true
		}
	}
	return false
}

// TestCORSPreflightAllowMethodsByRoute — маршрутные методы префлайта:
// только реально зарегистрированные методы пути + OPTIONS; неизвестный путь —
// заголовка нет.
func TestCORSPreflightAllowMethodsByRoute(t *testing.T) {
	_, router := newTestRouter(devConfig())

	// 1. /api/v1/courses — только GET: ровно «GET, OPTIONS».
	rec := optReq(t, router, "/api/v1/courses", corsAllowedOrigin)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS /api/v1/courses: status=%d, ждали 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, OPTIONS" {
		t.Fatalf("Allow-Methods=%q, ждали %q", got, "GET, OPTIONS")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != corsAllowedOrigin {
		t.Fatalf("Allow-Origin=%q, ждали %q", got, corsAllowedOrigin)
	}

	// 2. /api/v1/teacher/courses — GET и POST: оба в списке, плюс OPTIONS.
	rec = optReq(t, router, "/api/v1/teacher/courses", corsAllowedOrigin)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS /api/v1/teacher/courses: status=%d, ждали 204", rec.Code)
	}
	methods := rec.Header().Get("Access-Control-Allow-Methods")
	for _, m := range []string{"GET", "POST", "OPTIONS"} {
		if !hasMethod(methods, m) {
			t.Fatalf("Allow-Methods=%q, ждали метод %s в списке", methods, m)
		}
	}

	// 3. /api/v1/auth/login — только POST: POST есть, GET нет.
	rec = optReq(t, router, "/api/v1/auth/login", corsAllowedOrigin)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS /api/v1/auth/login: status=%d, ждали 204", rec.Code)
	}
	methods = rec.Header().Get("Access-Control-Allow-Methods")
	if !hasMethod(methods, "POST") {
		t.Fatalf("Allow-Methods=%q, ждали POST в списке", methods)
	}
	if hasMethod(methods, "GET") {
		t.Fatalf("Allow-Methods=%q: GET не зарегистрирован у /auth/login", methods)
	}

	// 4. Неизвестный путь: 204 (поведение префлайта не менялось), но
	// Allow-Methods не ставится — методов у пути нет.
	rec = optReq(t, router, "/api/v1/нет-такого-пути", corsAllowedOrigin)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS неизвестного пути: status=%d, ждали 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "" {
		t.Fatalf("неизвестный путь получил Allow-Methods=%q", got)
	}
}

// TestCORSPreflightForeignOrigin — чужой Origin не получает CORS-заголовков
// доступа (Allow-Origin и Allow-Methods), статус префлайта — прежний 204.
func TestCORSPreflightForeignOrigin(t *testing.T) {
	_, router := newTestRouter(devConfig())

	rec := optReq(t, router, "/api/v1/courses", "https://evil.example")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight с чужого origin: status=%d, ждали 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("чужой origin получил Allow-Origin=%q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "" {
		t.Fatalf("чужой origin получил Allow-Methods=%q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("чужой origin получил Allow-Credentials=%q", got)
	}
}

// TestCORSNonOptionsUnchanged — не-OPTIONS запросы middleware не трогает:
// обычный GET /api/v1/courses без Origin проходит в роутер и отвечает 200
// без CORS-заголовков.
func TestCORSNonOptionsUnchanged(t *testing.T) {
	_, router := newTestRouter(devConfig())

	rec := doJSON(t, router, http.MethodGet, "/api/v1/courses", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/courses: status=%d, ждали 200", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("без Origin получен Allow-Origin=%q", got)
	}
}
