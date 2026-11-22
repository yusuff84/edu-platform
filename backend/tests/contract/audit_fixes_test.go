package contract_test

// Регрессионные контрактные тесты исправлений аудита бэкенда (ноябрь 2026).

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"edu-platform/backend/internal/repo"
)

const hugePage = "9223372036854775807"

// Заблокированный ученик со старой сессией не видит закрытый контент
// купленного курса: публичная страница считает его гостем.
func TestAuditBlockedViewerSeesCourseAsGuest(t *testing.T) {
	app, router := newTestApp()
	_, courseID, slug := seedPublishedCourse(t, app, router, 50000)
	student, studentID := registerUser(t, router, "blocked-viewer@example.com", "password123", "")
	if e, _ := app.Store.EnsureEnrollment(studentID, courseID); e == nil {
		t.Fatal("EnsureEnrollment")
	}
	quizOf := func() any {
		rec := doRequest(t, router, http.MethodGet, "/api/v1/courses/"+slug, nil, student)
		if rec.Code != http.StatusOK {
			t.Fatalf("course page: %d %s", rec.Code, rec.Body.String())
		}
		var out struct {
			Modules []struct {
				Lessons []map[string]any `json:"lessons"`
			} `json:"modules"`
		}
		decodeBody(t, rec, &out)
		return out.Modules[0].Lessons[0]["quiz"]
	}
	if quizOf() == nil {
		t.Fatal("записанный ученик должен видеть тест урока")
	}
	if !app.Store.SetUserStatus(studentID, repo.UserBlocked) {
		t.Fatal("SetUserStatus")
	}
	if q := quizOf(); q != nil {
		t.Fatalf("заблокированному закрытый контент не отдаётся: %v", q)
	}
	// /auth/me по-прежнему отвечает заблокированному 403 (контракт T24).
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, student); rec.Code != http.StatusForbidden {
		t.Fatalf("/auth/me заблокированного: %d", rec.Code)
	}
}

// Огромный page раньше переполнял (page-1)*limit: GET /courses падал в
// панику (500), остальные списки — в отрицательное смещение.
func TestAuditHugePageIsEmptyNotPanic(t *testing.T) {
	app, router := newTestApp()
	teacher, courseID, _ := seedPublishedCourse(t, app, router, 0)
	admin := loginUser(t, router, "admin@example.com", "password123")
	student, _ := registerUser(t, router, "huge-page@example.com", "password123", "")
	if rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{"courseId": courseID}, student); rec.Code != http.StatusCreated {
		t.Fatalf("free enroll: %d %s", rec.Code, rec.Body.String())
	}
	cases := []struct {
		path    string
		cookies []*http.Cookie
	}{
		{"/api/v1/courses?page=" + hugePage, nil},
		{"/api/v1/me/payments?page=" + hugePage, student},
		{"/api/v1/courses/" + courseID + "/questions?page=" + hugePage, nil},
		{"/api/v1/admin/users?page=" + hugePage, admin},
		{"/api/v1/admin/audit?page=" + hugePage, admin},
		{"/api/v1/admin/questions?page=" + hugePage, admin},
		{"/api/v1/admin/reviews?page=" + hugePage, admin},
		{"/api/v1/admin/courses?page=" + hugePage, admin},
		{"/api/v1/admin/payments?page=" + hugePage, admin},
		{"/api/v1/admin/enrollments?page=" + hugePage, admin},
		{"/api/v1/teacher/courses/" + courseID + "/students?page=" + hugePage, teacher},
	}
	for _, tc := range cases {
		rec := doRequest(t, router, http.MethodGet, tc.path, nil, tc.cookies)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status=%d body=%s", tc.path, rec.Code, rec.Body.String())
			continue
		}
		var out struct {
			Items []any `json:"items"`
		}
		decodeBody(t, rec, &out)
		if len(out.Items) != 0 {
			t.Errorf("%s: на далёкой странице ждали пусто, получили %d", tc.path, len(out.Items))
		}
	}
	// Тотал при этом честный.
	rec := doRequest(t, router, http.MethodGet, "/api/v1/courses?page="+hugePage, nil, nil)
	var catalog struct {
		Total int `json:"total"`
	}
	decodeBody(t, rec, &catalog)
	if catalog.Total != 1 {
		t.Fatalf("total каталога: %d", catalog.Total)
	}
}

// Цена и лимит промокода больше INT4 — понятная 400, а не 500 из Postgres.
func TestAuditInt4Bounds(t *testing.T) {
	app, router := newTestApp()
	teacher, courseID := seedTeacherCourse(t, app, router, "int4")
	if rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": "Дорогой", "description": "Описание", "priceCents": 3_000_000_000,
	}, teacher); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Цена слишком большая") {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, router, http.MethodPatch, "/api/v1/teacher/courses/"+courseID, map[string]any{
		"priceCents": 3_000_000_000,
	}, teacher); rec.Code != http.StatusBadRequest {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, router, http.MethodPatch, "/api/v1/teacher/courses/"+courseID, map[string]any{
		"priceCents": 2147483647,
	}, teacher); rec.Code != http.StatusOK {
		t.Fatalf("patch на границе INT4: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/promo-codes", map[string]any{
		"code": "BIGLIMIT", "discountPct": 10, "maxUses": 3_000_000_000,
	}, teacher); rec.Code != http.StatusBadRequest {
		t.Fatalf("promo maxUses: %d %s", rec.Code, rec.Body.String())
	}
}

// Ссылки уроков со схемой javascript:/data: отклоняются (хранимый XSS в
// href/src у учеников); http(s) и адреса без схемы — как раньше.
func TestAuditLessonLinkScheme(t *testing.T) {
	app, router := newTestApp()
	teacher, courseID := seedTeacherCourse(t, app, router, "links")
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/modules", map[string]any{"title": "М"}, teacher)
	var mod struct{ ID string }
	decodeBody(t, rec, &mod)
	create := func(body map[string]any) *httptest.ResponseRecorder {
		body["moduleId"] = mod.ID
		return doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", body, teacher)
	}
	if rec := create(map[string]any{"title": "Эфир", "kind": "live", "liveUrl": "javascript:alert(document.cookie)"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("live javascript: %d %s", rec.Code, rec.Body.String())
	}
	if rec := create(map[string]any{"title": "Видео", "kind": "video_text", "videoUrl": "data:text/html,<script>1</script>"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("video data: %d %s", rec.Code, rec.Body.String())
	}
	rec = create(map[string]any{"title": "Эфир", "kind": "live", "liveUrl": "zoom.us/j/123"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("live без схемы: %d %s", rec.Code, rec.Body.String())
	}
	var lesson struct{ ID string }
	decodeBody(t, rec, &lesson)
	if rec := create(map[string]any{"title": "Видео", "kind": "video_text", "videoUrl": "https://example.com/v.mp4"}); rec.Code != http.StatusCreated {
		t.Fatalf("video https: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, router, http.MethodPatch, "/api/v1/teacher/courses/"+courseID+"/lessons/"+lesson.ID,
		map[string]any{"liveUrl": "JavaScript:alert(1)"}, teacher); rec.Code != http.StatusBadRequest {
		t.Fatalf("patch javascript: %d %s", rec.Code, rec.Body.String())
	}
}

// Текст отзыва ограничен 2000 символами, как ответ преподавателя.
func TestAuditReviewBodyLimit(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 50000)
	student, studentID := registerUser(t, router, "review-limit@example.com", "password123", "")
	if e, _ := app.Store.EnsureEnrollment(studentID, courseID); e == nil {
		t.Fatal("EnsureEnrollment")
	}
	if rec := doRequest(t, router, http.MethodPost, "/api/v1/courses/"+courseID+"/reviews",
		map[string]any{"rating": 5, "body": strings.Repeat("ы", 2001)}, student); rec.Code != http.StatusBadRequest {
		t.Fatalf("2001 символ: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, router, http.MethodPost, "/api/v1/courses/"+courseID+"/reviews",
		map[string]any{"rating": 5, "body": strings.Repeat("ы", 2000)}, student); rec.Code != http.StatusCreated {
		t.Fatalf("2000 символов: %d %s", rec.Code, rec.Body.String())
	}
}

// Имя при создании пользователя админом — по правилам профиля.
func TestAuditAdminCreateUserDisplayName(t *testing.T) {
	app, router := newTestApp()
	admin, _ := seedAdminUser(t, app, router, "dn-admin@example.com")
	if rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/users",
		map[string]any{"email": "long-name@example.com", "displayName": strings.Repeat("я", 61)}, admin); rec.Code != http.StatusBadRequest {
		t.Fatalf("61 символ: %d %s", rec.Code, rec.Body.String())
	}
	if _, exists := app.Store.GetUserByEmail("long-name@example.com"); exists {
		t.Fatal("при ошибке валидации пользователь не должен создаваться")
	}
	rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/users",
		map[string]any{"email": "spaced@example.com", "displayName": "  Иван   Петров "}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	if u, _ := app.Store.GetUserByEmail("spaced@example.com"); u.DisplayName != "Иван Петров" {
		t.Fatalf("displayName = %q", u.DisplayName)
	}
}

// Смена email админом гасит письма-токены, выданные на прежний адрес.
func TestAuditEmailChangeRevokesAccountTokens(t *testing.T) {
	app, router := newTestApp()
	admin, _ := seedAdminUser(t, app, router, "tok-admin@example.com")
	_, userID := registerUser(t, router, "old-address@example.com", "password123", "")
	u, _ := app.Store.GetUserByID(userID)
	now := time.Now().UTC()
	if err := app.Store.IssueAccountToken(u.ID, u.PasswordHash, repo.EmailVerification, "verify-hash", now, now.Add(time.Hour)); err != nil {
		t.Fatalf("IssueAccountToken: %v", err)
	}
	if rec := doRequest(t, router, http.MethodPatch, "/api/v1/admin/users/"+userID+"/profile",
		map[string]any{"email": "new-address@example.com"}, admin); rec.Code != http.StatusOK {
		t.Fatalf("profile: %d %s", rec.Code, rec.Body.String())
	}
	if err := app.Store.ConsumeAccountToken("verify-hash", repo.EmailVerification, "", now); !errors.Is(err, repo.ErrInvalidAccountToken) {
		t.Fatalf("старая ссылка подтверждения после смены email: err=%v", err)
	}
	if nu, _ := app.Store.GetUserByID(userID); nu.EmailVerifiedAt != nil {
		t.Fatal("новый адрес не должен считаться подтверждённым")
	}
}

// failingStore — хранилище, которое «не смогло» записать статус курса или
// создать курс/модуль/урок (так ведёт себя PostgresStore при ошибке БД).
type failingStore struct {
	repo.StoreAPI
	failStatus, failCreate bool
}

func (s *failingStore) SetCourseStatus(id string, status repo.CourseStatus, comment string) bool {
	if s.failStatus {
		return false
	}
	return s.StoreAPI.SetCourseStatus(id, status, comment)
}

func (s *failingStore) CreateCourse(c *repo.Course) *repo.Course {
	if s.failCreate {
		return nil
	}
	return s.StoreAPI.CreateCourse(c)
}

func (s *failingStore) CreateModule(courseID, title string) *repo.Module {
	if s.failCreate {
		return nil
	}
	return s.StoreAPI.CreateModule(courseID, title)
}

func (s *failingStore) CreateLesson(l *repo.Lesson) *repo.Lesson {
	if s.failCreate {
		return nil
	}
	return s.StoreAPI.CreateLesson(l)
}

// Сбой хранилища больше не превращается в 200/201 с несуществующими данными.
func TestAuditStoreFailuresAreNotSuccess(t *testing.T) {
	app, router := newTestApp()
	teacher, courseID, _ := seedPublishedCourse(t, app, router, 50000)
	admin := loginUser(t, router, "admin@example.com", "password123")
	rec := doRequest(t, router, http.MethodGet, "/api/v1/teacher/courses/"+courseID, nil, teacher)
	var detail struct {
		Modules []struct{ ID string } `json:"modules"`
	}
	decodeBody(t, rec, &detail)

	fs := &failingStore{StoreAPI: app.Store}
	app.Store = fs

	fs.failStatus = true
	for _, path := range []string{"/archive", "/unpublish"} {
		if rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+courseID+path, nil, admin); rec.Code != http.StatusInternalServerError {
			t.Fatalf("%s при сбое записи: %d %s", path, rec.Code, rec.Body.String())
		}
	}
	if c, _ := app.Store.GetCourseByID(courseID); c.Status != repo.CoursePublished {
		t.Fatalf("статус не должен меняться: %s", c.Status)
	}
	fs.failStatus = false

	fs.failCreate = true
	if rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": "Новый", "description": "Описание",
	}, teacher); rec.Code != http.StatusInternalServerError {
		t.Fatalf("create course при сбое: %d %s", rec.Code, rec.Body.String())
	}
	// Модуль/урок — в черновик: снимаем курс с публикации.
	if rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+courseID+"/unpublish", nil, admin); rec.Code != http.StatusOK {
		t.Fatalf("unpublish: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/modules", map[string]any{"title": "М"}, teacher); rec.Code != http.StatusInternalServerError {
		t.Fatalf("create module при сбое: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
		"moduleId": detail.Modules[0].ID, "title": "У", "kind": "video_text", "bodyMd": "текст",
	}, teacher); rec.Code != http.StatusInternalServerError {
		t.Fatalf("create lesson при сбое: %d %s", rec.Code, rec.Body.String())
	}
}

// MCP: имя неизвестного инструмента экранируется в JSON, тело до 6 МБ
// принимается целиком, больше — 413; огромная цена не переполняет int.
func TestAuditMCPEdges(t *testing.T) {
	app, router := newTestApp()
	registerUser(t, router, "mcp-audit@example.com", "password123", "")
	promoteToAdmin(t, app, "mcp-audit@example.com")
	admin := loginUser(t, router, "mcp-audit@example.com", "password123")
	rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/mcp-tokens", map[string]any{"name": "audit"}, admin)
	var created struct {
		Token string `json:"token"`
	}
	decodeBody(t, rec, &created)

	code, out := mcpCall(t, router, created.Token, "tools/call", map[string]any{"name": `bad"tool`, "arguments": map[string]any{}})
	if code != http.StatusOK {
		t.Fatalf("unknown tool: http %d", code)
	}
	text := out["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	var parsed map[string]string
	if err := json.Unmarshal([]byte(text), &parsed); err != nil || !strings.Contains(parsed["error"], `bad"tool`) {
		t.Fatalf("ответ неизвестного инструмента — не JSON: %q (%v)", text, err)
	}

	res, isErr := tool(t, router, created.Token, "create_course", map[string]any{
		"title": "Огромная цена", "description": "Описание", "price_rub": 1e300,
	})
	if !isErr || !strings.Contains(res["error"].(string), "Цена слишком большая") {
		t.Fatalf("price_rub=1e300: isErr=%v %v", isErr, res)
	}

	send := func(body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+created.Token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	// ~5 МБ: раньше тело обрезалось на 4 МБ и превращалось в «Parse error».
	pad := strings.Repeat("a", 5<<20)
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 7, "method": "ping", "params": map[string]any{"pad": pad}})
	if rec := send(body); rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "Parse error") {
		t.Fatalf("5 МБ: %d %.200s", rec.Code, rec.Body.String())
	}
	body, _ = json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 8, "method": "ping", "params": map[string]any{"pad": pad + pad}})
	if rec := send(body); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("10 МБ: %d %.200s", rec.Code, rec.Body.String())
	}
}
