package handler

// MCP-сервер skillex.space: нейросеть (Claude, Cursor и др.) создаёт и правит
// курсы через протокол Model Context Protocol (транспорт Streamable HTTP,
// JSON-ответы без SSE).
//
//   POST /api/v1/mcp  — JSON-RPC 2.0: initialize, ping, tools/list, tools/call
//
// Безопасность:
//   - доступ только по токену «Authorization: Bearer skx_…», который выпускает
//     администратор в админке; в базе хранится лишь SHA-256 токена;
//   - токен работает, пока его владелец — активный администратор и токен не отозван;
//   - cookie-сессии здесь не принимаются (поэтому CSRF к маршруту не применяется);
//   - канал — HTTPS (TLS на nginx); отдельный лимит запросов;
//   - каждый вызов инструмента пишется в журнал (admin-audit).
//
// Инструменты не дублируют логику: они вызывают те же обработчики, что и
// конструктор курсов в интерфейсе (валидация, слаги, тесты, правила черновика).

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/repo"
)

const mcpProtocolVersion = "2025-06-18"

// RegisterMCPRoutes — MCP-эндпоинт и управление токенами в админке.
func (a *App) RegisterMCPRoutes(r chi.Router) {
	r.With(a.rateLimit("mcp", 600)).Post("/mcp", a.MCPHandler)
	r.Get("/mcp", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Allow", "POST")
		writeErr(w, http.StatusMethodNotAllowed, "MCP: используйте POST (Streamable HTTP без SSE)")
	})
	adm := r.With(a.requireAuth, a.requireRole(repo.RoleAdmin))
	adm.Get("/admin/mcp-tokens", a.AdminListMCPTokens)
	adm.Post("/admin/mcp-tokens", a.AdminCreateMCPToken)
	adm.Delete("/admin/mcp-tokens/{id}", a.AdminRevokeMCPToken)
}

func hashAPIToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ---- управление токенами (админка) ----

func (a *App) AdminListMCPTokens(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": a.Store.ListAPITokens()})
}

// AdminCreateMCPToken — POST /admin/mcp-tokens {name}: 201 {token, item}.
// Токен возвращается ОДИН раз; в базе остаётся только хеш.
func (a *App) AdminCreateMCPToken(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	name := strings.Join(strings.Fields(req.Name), " ")
	if name == "" {
		name = "MCP"
	}
	if utf8.RuneCountInString(name) > 60 {
		writeErr(w, http.StatusBadRequest, "Название токена не длиннее 60 символов")
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось создать токен")
		return
	}
	token := "skx_" + base64.RawURLEncoding.EncodeToString(raw)
	item, err := a.Store.CreateAPIToken(u.ID, name, hashAPIToken(token), token[:10])
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось создать токен")
		return
	}
	a.auditAdmin(r, "Выпущен MCP-токен «%s» (%s…)", name, item.Prefix)
	writeJSON(w, http.StatusCreated, map[string]any{"token": token, "item": item})
}

func (a *App) AdminRevokeMCPToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !a.Store.RevokeAPIToken(id) {
		writeErr(w, http.StatusNotFound, "Токен не найден или уже отозван")
		return
	}
	a.auditAdmin(r, "Отозван MCP-токен %s", id)
	w.WriteHeader(http.StatusNoContent)
}

// mcpUser проверяет «Authorization: Bearer» и возвращает администратора-владельца токена.
func (a *App) mcpUser(r *http.Request) (*repo.User, *repo.APIToken, bool) {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(h, "Bearer ") {
		return nil, nil, false
	}
	token := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	if !strings.HasPrefix(token, "skx_") || len(token) < 20 {
		return nil, nil, false
	}
	t, ok := a.Store.GetAPITokenByHash(hashAPIToken(token))
	if !ok || t.RevokedAt != nil {
		return nil, nil, false
	}
	u, ok := a.Store.GetUserByID(t.UserID)
	if !ok || u.Role != repo.RoleAdmin || u.Status == repo.UserBlocked {
		return nil, nil, false
	}
	return u, t, true
}

// ---- JSON-RPC ----

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func rpcReply(id json.RawMessage, result any, err *rpcError) map[string]any {
	out := map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id)}
	if err != nil {
		out["error"] = err
	} else {
		out["result"] = result
	}
	return out
}

// MCPHandler — POST /mcp: один JSON-RPC запрос/уведомление или пакет.
func (a *App) MCPHandler(w http.ResponseWriter, r *http.Request) {
	u, tok, ok := a.mcpUser(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="skillex-mcp"`)
		writeErr(w, http.StatusUnauthorized, "MCP: нужен действующий токен администратора (Authorization: Bearer skx_…)")
		return
	}
	a.Store.TouchAPIToken(tok.ID)
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Некорректное тело запроса")
		return
	}
	body = bytes.TrimSpace(body)
	if len(body) > 0 && body[0] == '[' {
		var batch []rpcRequest
		if err := json.Unmarshal(body, &batch); err != nil {
			writeJSON(w, http.StatusOK, rpcReply(json.RawMessage("null"), nil, &rpcError{-32700, "Parse error"}))
			return
		}
		out := []any{}
		for _, req := range batch {
			if resp := a.mcpDispatch(r.Context(), u, req); resp != nil {
				out = append(out, resp)
			}
		}
		if len(out) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusOK, rpcReply(json.RawMessage("null"), nil, &rpcError{-32700, "Parse error"}))
		return
	}
	resp := a.mcpDispatch(r.Context(), u, req)
	if resp == nil { // уведомление
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *App) mcpDispatch(ctx context.Context, u *repo.User, req rpcRequest) map[string]any {
	isNotification := len(req.ID) == 0
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		version := mcpProtocolVersion
		if p.ProtocolVersion != "" {
			version = p.ProtocolVersion
		}
		return rpcReply(req.ID, map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "skillex-courses", "title": "skillex.space — курсы", "version": "1.0.0"},
			"instructions": "Инструменты для создания и редактирования курсов платформы skillex.space. " +
				"Порядок: create_course → add_module → add_lesson (text/video/quiz) → set_course_cover (обложка) → publish_course. " +
				"Контент правится только в черновике: для опубликованного курса сначала unpublish_course. " +
				"Разметка уроков — Markdown (##, списки, **жирный**, `код`, блоки кода); таблиц и картинок нет.",
		}, nil)
	case "ping":
		return rpcReply(req.ID, map[string]any{}, nil)
	case "tools/list":
		return rpcReply(req.ID, map[string]any{"tools": mcpTools()}, nil)
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return rpcReply(req.ID, nil, &rpcError{-32602, "Invalid params"})
		}
		text, isErr := a.mcpCallTool(ctx, u, p.Name, p.Arguments)
		a.auditMCP(ctx, u, p.Name, p.Arguments, isErr)
		return rpcReply(req.ID, map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
			"isError": isErr,
		}, nil)
	default:
		if isNotification {
			return nil
		}
		return rpcReply(req.ID, nil, &rpcError{-32601, "Method not found: " + req.Method})
	}
}

// ---- инструменты ----

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

func obj(props map[string]any, required ...string) map[string]any {
	o := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		o["required"] = required
	}
	return o
}

var questionsSchema = map[string]any{
	"type":        "array",
	"description": "Вопросы теста: у каждого 2–6 вариантов и ровно один верный (индекс с 0). Тест засчитывается, если верны все ответы.",
	"items": obj(map[string]any{
		"text":    str("Текст вопроса"),
		"options": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"correct": map[string]any{"type": "integer", "minimum": 0, "description": "Индекс верного варианта"},
	}, "text", "options", "correct"),
}

func mcpTools() []map[string]any {
	tool := func(name, title, desc string, schema map[string]any) map[string]any {
		return map[string]any{"name": name, "title": title, "description": desc, "inputSchema": schema}
	}
	lessonKind := map[string]any{"type": "string", "enum": []string{"text", "video", "quiz"},
		"description": "text — конспект (Markdown), video — видео + текст, quiz — тест"}
	return []map[string]any{
		tool("list_courses", "Список курсов", "Все курсы платформы: id, slug, название, статус, цена, автор, число уроков.",
			obj(map[string]any{"query": str("Поиск по названию (необязательно)")})),
		tool("get_course", "Курс целиком", "Курс с модулями и уроками (тексты, тесты с ключами). Нужен id курса.",
			obj(map[string]any{"course_id": str("UUID курса")}, "course_id")),
		tool("create_course", "Создать курс", "Создаёт курс-черновик. Цена в рублях (0 — бесплатный). Категории: programming, design, data, marketing, ai, other.",
			obj(map[string]any{
				"title": str("Название"), "description": str("Описание (2–3 абзаца)"),
				"price_rub": map[string]any{"type": "integer", "minimum": 0}, "category": str("Категория"), "slug": str("Адрес /courses/<slug> (необязательно)"),
			}, "title", "description")),
		tool("update_course", "Изменить курс", "Меняет название, описание, цену, категорию или slug черновика.",
			obj(map[string]any{
				"course_id": str("UUID курса"), "title": str(""), "description": str(""),
				"price_rub": map[string]any{"type": "integer", "minimum": 0}, "category": str(""), "slug": str(""),
			}, "course_id")),
		tool("add_module", "Добавить модуль", "Добавляет модуль в конец программы курса.",
			obj(map[string]any{"course_id": str("UUID курса"), "title": str("Название модуля")}, "course_id", "title")),
		tool("rename_module", "Переименовать модуль", "Меняет название модуля.",
			obj(map[string]any{"course_id": str("UUID курса"), "module_id": str("UUID модуля"), "title": str("Новое название")}, "course_id", "module_id", "title")),
		tool("delete_module", "Удалить модуль", "Удаляет модуль вместе с уроками (только черновик).",
			obj(map[string]any{"course_id": str("UUID курса"), "module_id": str("UUID модуля")}, "course_id", "module_id")),
		tool("add_lesson", "Добавить урок", "Добавляет урок в конец модуля: конспект, видео (video_url) или тест (questions).",
			obj(map[string]any{
				"course_id": str("UUID курса"), "module_id": str("UUID модуля"), "title": str("Название урока"),
				"kind": lessonKind, "body_md": str("Текст урока в Markdown"), "video_url": str("Ссылка на видео (для kind=video)"),
				"questions": questionsSchema, "is_preview": map[string]any{"type": "boolean", "description": "Доступен без записи (превью)"},
			}, "course_id", "module_id", "title", "kind")),
		tool("update_lesson", "Изменить урок", "Меняет поля урока; переданные поля заменяются целиком.",
			obj(map[string]any{
				"course_id": str("UUID курса"), "lesson_id": str("UUID урока"), "title": str(""), "body_md": str(""),
				"video_url": str(""), "questions": questionsSchema, "module_id": str("Перенести в другой модуль (UUID)"),
				"is_preview": map[string]any{"type": "boolean"},
			}, "course_id", "lesson_id")),
		tool("delete_lesson", "Удалить урок", "Удаляет урок (только черновик).",
			obj(map[string]any{"course_id": str("UUID курса"), "lesson_id": str("UUID урока")}, "course_id", "lesson_id")),
		tool("publish_course", "Опубликовать курс", "Публикует курс сразу (без очереди модерации). В курсе должен быть хотя бы один урок.",
			obj(map[string]any{"course_id": str("UUID курса")}, "course_id")),
		tool("unpublish_course", "Снять с публикации", "Возвращает опубликованный курс в черновик, чтобы править программу.",
			obj(map[string]any{"course_id": str("UUID курса")}, "course_id")),
		tool("delete_course", "Удалить курс", "Удаляет курс-черновик без учеников.",
			obj(map[string]any{"course_id": str("UUID курса")}, "course_id")),
		tool("set_course_cover", "Обложка курса", "Ставит обложку курса (опубликованного тоже, кроме архива). Картинка PNG, JPEG или WebP до 5 МБ в base64; лучше 16:10, например 1600×1000, без текста — название курса сайт накладывает сам снизу слева.",
			obj(map[string]any{"course_id": str("UUID курса"), "image_base64": str("Картинка в base64 (можно с префиксом data:image/...;base64,)")}, "course_id", "image_base64")),
	}
}

// mcpInvoke вызывает обычный обработчик от имени администратора и возвращает ответ.
func (a *App) mcpInvoke(ctx context.Context, u *repo.User, h http.HandlerFunc, method string, params map[string]string, body any) (int, []byte) {
	var rd io.Reader = http.NoBody
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	return a.mcpInvokeRaw(ctx, u, h, method, params, rd, "application/json")
}

// mcpInvokeRaw — как mcpInvoke, но с готовым телом и Content-Type (multipart для обложки).
func (a *App) mcpInvokeRaw(ctx context.Context, u *repo.User, h http.HandlerFunc, method string, params map[string]string, rd io.Reader, contentType string) (int, []byte) {
	req := httptest.NewRequest(method, "/internal/mcp", rd).WithContext(ctx)
	req.Header.Set("Content-Type", contentType)
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	c := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	c = context.WithValue(c, userCtxKey, u)
	rec := httptest.NewRecorder()
	h(rec, req.WithContext(c))
	return rec.Code, rec.Body.Bytes()
}

func argStr(args map[string]any, k string) (string, bool) {
	v, ok := args[k]
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	return strings.TrimSpace(s), ok
}

func argInt(args map[string]any, k string) (int, bool) {
	switch v := args[k].(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	}
	return 0, false
}

func mcpQuiz(args map[string]any) (*quizRequest, bool) {
	raw, ok := args["questions"]
	if !ok || raw == nil {
		return nil, false
	}
	b, _ := json.Marshal(raw)
	var qs []quizQuestionRequest
	if json.Unmarshal(b, &qs) != nil {
		return nil, false
	}
	return &quizRequest{Questions: qs}, true
}

// mcpCallTool выполняет инструмент; возвращает текст ответа (JSON) и признак ошибки.
func (a *App) mcpCallTool(ctx context.Context, u *repo.User, name string, args map[string]any) (string, bool) {
	if args == nil {
		args = map[string]any{}
	}
	cid, _ := argStr(args, "course_id")
	var (
		code int
		out  []byte
	)
	switch name {
	case "list_courses":
		q, _ := argStr(args, "query")
		h := func(w http.ResponseWriter, r *http.Request) {
			r.URL.RawQuery = "limit=100&q=" + url.QueryEscape(q)
			a.AdminListCourses(w, r)
		}
		code, out = a.mcpInvoke(ctx, u, h, http.MethodGet, nil, nil)
	case "get_course":
		code, out = a.mcpInvoke(ctx, u, a.TeacherGetCourse, http.MethodGet, map[string]string{"id": cid}, nil)
	case "create_course":
		body := map[string]any{"currency": "RUB"}
		for _, k := range []string{"title", "description", "slug", "category"} {
			if v, ok := argStr(args, k); ok {
				body[k] = v
			}
		}
		price, _ := argInt(args, "price_rub")
		body["priceCents"] = price * 100
		code, out = a.mcpInvoke(ctx, u, a.TeacherCreateCourse, http.MethodPost, nil, body)
	case "update_course":
		body := map[string]any{}
		for _, k := range []string{"title", "description", "slug", "category"} {
			if v, ok := argStr(args, k); ok {
				body[k] = v
			}
		}
		if price, ok := argInt(args, "price_rub"); ok {
			body["priceCents"] = price * 100
		}
		code, out = a.mcpInvoke(ctx, u, a.TeacherUpdateCourse, http.MethodPatch, map[string]string{"id": cid}, body)
	case "add_module":
		t, _ := argStr(args, "title")
		code, out = a.mcpInvoke(ctx, u, a.TeacherCreateModule, http.MethodPost, map[string]string{"id": cid}, map[string]any{"title": t})
	case "rename_module":
		mid, _ := argStr(args, "module_id")
		t, _ := argStr(args, "title")
		code, out = a.mcpInvoke(ctx, u, a.TeacherUpdateModule, http.MethodPatch, map[string]string{"id": cid, "moduleID": mid}, map[string]any{"title": t})
	case "delete_module":
		mid, _ := argStr(args, "module_id")
		code, out = a.mcpInvoke(ctx, u, a.TeacherDeleteModule, http.MethodDelete, map[string]string{"id": cid, "moduleID": mid}, nil)
	case "add_lesson":
		mid, _ := argStr(args, "module_id")
		t, _ := argStr(args, "title")
		kind, _ := argStr(args, "kind")
		bodyMD, _ := argStr(args, "body_md")
		body := map[string]any{"moduleId": mid, "title": t, "bodyMd": bodyMD}
		switch kind {
		case "quiz":
			body["kind"] = repo.LessonQuiz
			qz, ok := mcpQuiz(args)
			if !ok {
				return `{"error":"Для теста нужен массив questions"}`, true
			}
			body["quiz"] = qz
		case "video":
			body["kind"] = repo.LessonVideoText
			v, _ := argStr(args, "video_url")
			body["videoUrl"] = v
		default:
			body["kind"] = repo.LessonVideoText
		}
		code, out = a.mcpInvoke(ctx, u, a.TeacherCreateLesson, http.MethodPost, map[string]string{"id": cid}, body)
		if code < 300 && args["is_preview"] == true {
			var l struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(out, &l) == nil && l.ID != "" {
				a.mcpInvoke(ctx, u, a.TeacherUpdateLesson, http.MethodPatch, map[string]string{"id": cid, "lessonID": l.ID}, map[string]any{"isPreview": true})
			}
		}
	case "update_lesson":
		lid, _ := argStr(args, "lesson_id")
		body := map[string]any{}
		if v, ok := argStr(args, "title"); ok {
			body["title"] = v
		}
		if v, ok := argStr(args, "body_md"); ok {
			body["bodyMd"] = v
		}
		if v, ok := argStr(args, "video_url"); ok {
			body["videoUrl"] = v
		}
		if v, ok := argStr(args, "module_id"); ok {
			body["moduleId"] = v
		}
		if qz, ok := mcpQuiz(args); ok {
			body["quiz"] = qz
		}
		if v, ok := args["is_preview"].(bool); ok {
			body["isPreview"] = v
		}
		code, out = a.mcpInvoke(ctx, u, a.TeacherUpdateLesson, http.MethodPatch, map[string]string{"id": cid, "lessonID": lid}, body)
	case "delete_lesson":
		lid, _ := argStr(args, "lesson_id")
		code, out = a.mcpInvoke(ctx, u, a.TeacherDeleteLesson, http.MethodDelete, map[string]string{"id": cid, "lessonID": lid}, nil)
	case "publish_course":
		code, out = a.mcpInvoke(ctx, u, a.AdminPublishCourse, http.MethodPost, map[string]string{"id": cid}, nil)
	case "unpublish_course":
		code, out = a.mcpInvoke(ctx, u, a.AdminUnpublishCourse, http.MethodPost, map[string]string{"id": cid}, nil)
	case "delete_course":
		code, out = a.mcpInvoke(ctx, u, a.TeacherDeleteCourse, http.MethodDelete, map[string]string{"id": cid}, nil)
	case "set_course_cover":
		raw, _ := argStr(args, "image_base64")
		if i := strings.Index(raw, ";base64,"); i >= 0 {
			raw = raw[i+len(";base64,"):]
		}
		raw = strings.Map(func(r rune) rune {
			if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
				return -1
			}
			return r
		}, raw)
		data, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			if data, err = base64.RawStdEncoding.DecodeString(raw); err != nil {
				return `{"error":"image_base64: не удалось декодировать base64"}`, true
			}
		}
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", "cover")
		_, _ = fw.Write(data)
		_ = mw.Close()
		code, out = a.mcpInvokeRaw(ctx, u, a.TeacherUploadCover, http.MethodPost, map[string]string{"id": cid}, &buf, mw.FormDataContentType())
	default:
		return `{"error":"Неизвестный инструмент: ` + name + `"}`, true
	}
	if len(out) == 0 {
		out = []byte(`{"ok":true}`)
	}
	return string(out), code >= 300
}
