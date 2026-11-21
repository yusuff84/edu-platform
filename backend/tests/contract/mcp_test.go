package contract_test

// MCP-сервер (handler/mcp.go): токены из админки, отказ без токена/с
// отозванным/у не-админа, JSON-RPC initialize/tools/list и полный цикл
// создания курса инструментами.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func mcpCall(t *testing.T, router http.Handler, token string, method string, params any) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// tool вызывает инструмент и возвращает разобранный JSON-ответ и признак ошибки.
func tool(t *testing.T, router http.Handler, token, name string, args map[string]any) (map[string]any, bool) {
	t.Helper()
	code, out := mcpCall(t, router, token, "tools/call", map[string]any{"name": name, "arguments": args})
	if code != http.StatusOK {
		t.Fatalf("%s: http %d", name, code)
	}
	res := out["result"].(map[string]any)
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	parsed := map[string]any{}
	_ = json.Unmarshal([]byte(text), &parsed)
	return parsed, res["isError"].(bool)
}

func TestMCPServer(t *testing.T) {
	app, router := newTestApp()
	registerUser(t, router, "mcp-admin@example.com", "password123", "")
	promoteToAdmin(t, app, "mcp-admin@example.com")
	admin := loginUser(t, router, "mcp-admin@example.com", "password123")
	studentCookies, _ := registerUser(t, router, "mcp-student@example.com", "password123", "")

	// Токен выдаёт только админ.
	if rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/mcp-tokens", map[string]any{"name": "x"}, studentCookies); rec.Code != http.StatusForbidden {
		t.Fatalf("student token: %d", rec.Code)
	}
	rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/mcp-tokens", map[string]any{"name": "Claude"}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create token: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Token string `json:"token"`
		Item  struct{ ID, Prefix string } `json:"item"`
	}
	decodeBody(t, rec, &created)
	if !strings.HasPrefix(created.Token, "skx_") || !strings.HasPrefix(created.Token, created.Item.Prefix) {
		t.Fatalf("token: %+v", created)
	}
	// В списке токенов самого токена нет — только префикс.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/mcp-tokens", nil, admin)
	if strings.Contains(rec.Body.String(), created.Token) {
		t.Fatal("список токенов раскрывает токен")
	}

	// Без токена / с мусором / cookie вместо токена — 401.
	if code, _ := mcpCall(t, router, "", "tools/list", nil); code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", code)
	}
	if code, _ := mcpCall(t, router, "skx_wrongwrongwrongwrongwrong", "tools/list", nil); code != http.StatusUnauthorized {
		t.Fatalf("bad token: %d", code)
	}

	// initialize + tools/list.
	_, init := mcpCall(t, router, created.Token, "initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test"}})
	if init["result"].(map[string]any)["serverInfo"].(map[string]any)["name"] != "skillex-courses" {
		t.Fatalf("initialize: %v", init)
	}
	_, list := mcpCall(t, router, created.Token, "tools/list", nil)
	if n := len(list["result"].(map[string]any)["tools"].([]any)); n < 10 {
		t.Fatalf("tools: %d", n)
	}

	// Полный цикл: курс → модуль → текст + тест → публикация.
	course, isErr := tool(t, router, created.Token, "create_course", map[string]any{"title": "Системное администрирование", "description": "Описание курса", "price_rub": 0, "category": "other"})
	if isErr {
		t.Fatalf("create_course: %v", course)
	}
	cid := course["id"].(string)
	mod, isErr := tool(t, router, created.Token, "add_module", map[string]any{"course_id": cid, "title": "Как устроен компьютер"})
	if isErr {
		t.Fatalf("add_module: %v", mod)
	}
	mid := mod["id"].(string)
	if l, isErr := tool(t, router, created.Token, "add_lesson", map[string]any{"course_id": cid, "module_id": mid, "title": "1.1 Процессор", "kind": "text", "body_md": "## Процессор\n\nТекст урока.", "is_preview": true}); isErr {
		t.Fatalf("add text lesson: %v", l)
	}
	if l, isErr := tool(t, router, created.Token, "add_lesson", map[string]any{"course_id": cid, "module_id": mid, "title": "1.2 Тест", "kind": "quiz",
		"questions": []any{map[string]any{"text": "Что такое ОЗУ?", "options": []string{"Оперативная память", "Жёсткий диск"}, "correct": 0}}}); isErr {
		t.Fatalf("add quiz lesson: %v", l)
	}
	// Тест без вопросов — ошибка валидации, а не падение.
	if _, isErr := tool(t, router, created.Token, "add_lesson", map[string]any{"course_id": cid, "module_id": mid, "title": "пустой", "kind": "quiz", "questions": []any{}}); !isErr {
		t.Fatal("пустой тест должен вернуть ошибку")
	}
	full, isErr := tool(t, router, created.Token, "get_course", map[string]any{"course_id": cid})
	if isErr || len(full["modules"].([]any)) != 1 {
		t.Fatalf("get_course: %v", full)
	}
	if _, isErr := tool(t, router, created.Token, "publish_course", map[string]any{"course_id": cid}); isErr {
		t.Fatal("publish_course")
	}
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/courses/"+course["slug"].(string), nil, nil); rec.Code != http.StatusOK {
		t.Fatalf("курс не опубликован: %d", rec.Code)
	}
	if found, _ := tool(t, router, created.Token, "list_courses", map[string]any{"query": "Системное"}); !strings.Contains(func() string { b, _ := json.Marshal(found); return string(b) }(), cid) {
		t.Fatal("list_courses не нашёл курс")
	}

	// Отзыв токена — дальше 401.
	if rec := doRequest(t, router, http.MethodDelete, "/api/v1/admin/mcp-tokens/"+created.Item.ID, nil, admin); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d", rec.Code)
	}
	if code, _ := mcpCall(t, router, created.Token, "tools/list", nil); code != http.StatusUnauthorized {
		t.Fatalf("revoked token: %d", code)
	}
}
