package contract_test

// T75 (P9): превью-уроки. Проверяем аддитивный флаг isPreview во всех видах
// урока (teacher create/patch, публичная страница курса, полный вид владельца)
// и публичный GET /courses/{slug}/preview/{lessonId}: 200 без входа только
// для превью-урока опубликованного курса, 404 для не-превью/чужого урока и
// неопубликованного курса; ответ не раскрывает правильные ответы теста и
// скрытые тесты code-задания.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/repo"
)

// previewFixture — приложение с учителем, админом, черновым курсом и модулем.
type previewFixture struct {
	app            *handler.App
	router         http.Handler
	teacherCookies []*http.Cookie
	adminCookies   []*http.Cookie
	courseID       string
	slug           string
	moduleID       string
}

func newPreviewFixture(t *testing.T) *previewFixture {
	t.Helper()
	app, router := newTestApp()

	teacherCookies, _ := registerUser(t, router, "preview-teacher@example.com", "password123", "")
	if !app.Store.SetUserRole(userIDByEmail(t, app.Store, "preview-teacher@example.com"), repo.RoleTeacher) {
		t.Fatal("не удалось выдать роль teacher")
	}
	adminCookies, _ := registerUser(t, router, "preview-admin@example.com", "password123", "")
	if !app.Store.SetUserRole(userIDByEmail(t, app.Store, "preview-admin@example.com"), repo.RoleAdmin) {
		t.Fatal("не удалось выдать роль admin")
	}

	f := &previewFixture{
		app: app, router: router,
		teacherCookies: teacherCookies, adminCookies: adminCookies,
	}
	f.courseID, f.slug, f.moduleID = f.createCourse(t, "Курс с превью")
	return f
}

// createCourse создаёт черновой курс с одним модулем и возвращает его id,
// slug и id модуля. Курсы в тестах создаются от одного учителя.
func (f *previewFixture) createCourse(t *testing.T, title string) (courseID, slug, moduleID string) {
	t.Helper()
	var course map[string]any
	rec := doRequest(t, f.router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": title, "description": "Описание курса T75", "priceCents": 1000,
	}, f.teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create course: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &course)
	courseID, _ = course["id"].(string)
	slug, _ = course["slug"].(string)

	var mod map[string]any
	rec = doRequest(t, f.router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/modules",
		map[string]any{"title": "Модуль 1"}, f.teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create module: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &mod)
	moduleID, _ = mod["id"].(string)
	if courseID == "" || slug == "" || moduleID == "" {
		t.Fatalf("курс/модуль созданы без id: course=%s slug=%s module=%s", courseID, slug, moduleID)
	}
	return courseID, slug, moduleID
}

// postLesson создаёт урок через teacher API и возвращает ответ (repo.Lesson).
func (f *previewFixture) postLesson(t *testing.T, courseID, moduleID string, body map[string]any) map[string]any {
	t.Helper()
	payload := map[string]any{"moduleId": moduleID}
	for k, v := range body {
		payload[k] = v
	}
	rec := doRequest(t, f.router, http.MethodPost,
		"/api/v1/teacher/courses/"+courseID+"/lessons", payload, f.teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create lesson %v: status=%d body=%s", body["title"], rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	return out
}

// patchLesson отправляет PATCH урока и возвращает ответ.
func (f *previewFixture) patchLesson(t *testing.T, courseID, lessonID string, body map[string]any) map[string]any {
	t.Helper()
	rec := doRequest(t, f.router, http.MethodPatch,
		"/api/v1/teacher/courses/"+courseID+"/lessons/"+lessonID, body, f.teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch lesson: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	return out
}

// publishCourse проводит курс через submit и модерацию (approve).
func (f *previewFixture) publishCourse(t *testing.T, courseID string) {
	t.Helper()
	rec := doRequest(t, f.router, http.MethodPost,
		"/api/v1/teacher/courses/"+courseID+"/submit", map[string]any{}, f.teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, f.router, http.MethodPost,
		"/api/v1/admin/courses/"+courseID+"/moderate",
		map[string]any{"decision": "approve"}, f.adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("moderate: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// lessonFromPublicPage вытаскивает урок из ответа GET /courses/{slug} по id.
// Заодно проверяет, что поле isPreview присутствует у каждого урока (оно
// аддитивное, но не omitempty — контракт P9).
func lessonFromPublicPage(t *testing.T, body []byte, lessonID string) map[string]any {
	t.Helper()
	var page struct {
		Modules []struct {
			Lessons []map[string]any `json:"lessons"`
		} `json:"modules"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("decode course page: %v (%s)", err, string(body))
	}
	for _, m := range page.Modules {
		for _, l := range m.Lessons {
			if _, ok := l["isPreview"]; !ok {
				t.Fatalf("в уроке нет поля isPreview: %v", l)
			}
			if l["id"] == lessonID {
				return l
			}
		}
	}
	t.Fatalf("урок %s не найден на странице курса: %s", lessonID, string(body))
	return nil
}

// TestLessonPreviewFlagInTeacherAndPublicViews — isPreview в ответах teacher
// create/patch, в публичной странице курса и в полном виде владельца.
func TestLessonPreviewFlagInTeacherAndPublicViews(t *testing.T) {
	f := newPreviewFixture(t)

	// Создание без поля — false; с isPreview:true — true.
	plain := f.postLesson(t, f.courseID, f.moduleID, map[string]any{
		"title": "Обычный урок", "kind": "video_text", "bodyMd": "текст урока",
	})
	plainID, _ := plain["id"].(string)
	if v, ok := plain["isPreview"].(bool); !ok || v {
		t.Fatalf("создание без isPreview: %s", mustJSON(plain))
	}

	preview := f.postLesson(t, f.courseID, f.moduleID, map[string]any{
		"title": "Превью-урок", "kind": "video_text", "bodyMd": "текст превью",
		"isPreview": true,
	})
	previewID, _ := preview["id"].(string)
	if v, ok := preview["isPreview"].(bool); !ok || !v {
		t.Fatalf("создание с isPreview=true: %s", mustJSON(preview))
	}

	// PATCH только с isPreview — валидация по kind не ломается, флаг меняется.
	patched := f.patchLesson(t, f.courseID, plainID, map[string]any{"isPreview": true})
	if v, _ := patched["isPreview"].(bool); !v {
		t.Fatalf("PATCH isPreview=true не сохранился: %s", mustJSON(patched))
	}

	// PATCH без isPreview не меняет флаг.
	renamed := f.patchLesson(t, f.courseID, previewID, map[string]any{"title": "Превью-урок (v2)"})
	if v, _ := renamed["isPreview"].(bool); !v {
		t.Fatalf("PATCH без isPreview сбросил флаг: %s", mustJSON(renamed))
	}
	if renamed["title"] != "Превью-урок (v2)" {
		t.Fatalf("PATCH не обновил title: %s", mustJSON(renamed))
	}

	// Явный false снимает флаг (потом возвращаем true — урок нужен публичным).
	off := f.patchLesson(t, f.courseID, previewID, map[string]any{"isPreview": false})
	if v, _ := off["isPreview"].(bool); v {
		t.Fatalf("PATCH isPreview=false не сбросил флаг: %s", mustJSON(off))
	}
	f.patchLesson(t, f.courseID, previewID, map[string]any{"isPreview": true})

	// Третий урок без флага — false в публичном виде (поле всё равно есть).
	third := f.postLesson(t, f.courseID, f.moduleID, map[string]any{
		"title": "Закрытый урок", "kind": "video_text", "bodyMd": "закрытый текст",
	})
	thirdID, _ := third["id"].(string)

	f.publishCourse(t, f.courseID)

	// Публичная страница курса без входа: флаг у каждого урока.
	rec := doRequest(t, f.router, http.MethodGet, "/api/v1/courses/"+f.slug, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("course page: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if v, _ := lessonFromPublicPage(t, rec.Body.Bytes(), plainID)["isPreview"].(bool); !v {
		t.Fatalf("публичная страница: у plain-урока isPreview=false")
	}
	if v, _ := lessonFromPublicPage(t, rec.Body.Bytes(), previewID)["isPreview"].(bool); !v {
		t.Fatalf("публичная страница: у превью-урока isPreview=false")
	}
	if v, _ := lessonFromPublicPage(t, rec.Body.Bytes(), thirdID)["isPreview"].(bool); v {
		t.Fatalf("публичная страница: у закрытого урока isPreview=true")
	}

	// Полный вид владельца: поле есть и совпадает.
	rec = doRequest(t, f.router, http.MethodGet, "/api/v1/courses/"+f.slug, nil, f.teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("course page (teacher): status=%d body=%s", rec.Code, rec.Body.String())
	}
	if v, _ := lessonFromPublicPage(t, rec.Body.Bytes(), thirdID)["isPreview"].(bool); v {
		t.Fatalf("полный вид: у закрытого урока isPreview=true")
	}
}

// TestLessonPreviewPublicEndpoint — публичный просмотр превью-урока: 200 без
// входа, публичный вид (без correct и скрытых тестов) и все 404-ветки.
func TestLessonPreviewPublicEndpoint(t *testing.T) {
	f := newPreviewFixture(t)

	quiz := f.postLesson(t, f.courseID, f.moduleID, map[string]any{
		"title": "Превью-тест", "kind": "quiz", "isPreview": true,
		"quiz": map[string]any{"questions": []any{map[string]any{
			"id": "q1", "text": "Что такое переменная?",
			"options": []string{"контейнер данных", "цикл"}, "correct": 0,
			"explanation": "Переменная хранит значение.",
		}}},
	})
	quizID, _ := quiz["id"].(string)

	code := f.postLesson(t, f.courseID, f.moduleID, map[string]any{
		"title": "Превью-код", "kind": "code", "isPreview": true,
		"codeTask": map[string]any{
			"language": "python", "template": "print(1)",
			"tests": []any{
				map[string]any{"name": "visibleTest", "input": "1", "expected": "1"},
				map[string]any{"name": "hiddenSecret", "input": "2", "expected": "2", "hidden": true},
			},
		},
	})
	codeID, _ := code["id"].(string)

	closed := f.postLesson(t, f.courseID, f.moduleID, map[string]any{
		"title": "Закрытый урок", "kind": "video_text", "bodyMd": "текст",
	})
	closedID, _ := closed["id"].(string)

	f.publishCourse(t, f.courseID)

	// 200 без входа: курс — ровно id/slug/title, урок — публичный вид.
	rec := doRequest(t, f.router, http.MethodGet,
		"/api/v1/courses/"+f.slug+"/preview/"+quizID, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview quiz: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Course map[string]any `json:"course"`
		Lesson map[string]any `json:"lesson"`
	}
	decodeBody(t, rec, &resp)
	if len(resp.Course) != 3 || resp.Course["id"] != f.courseID ||
		resp.Course["slug"] != f.slug || resp.Course["title"] != "Курс с превью" {
		t.Fatalf("course в ответе превью: %s", rec.Body.String())
	}
	if resp.Lesson["id"] != quizID || resp.Lesson["isPreview"] != true {
		t.Fatalf("lesson в ответе превью: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"correct"`) {
		t.Fatalf("превью-ответ раскрывает правильные ответы: %s", rec.Body.String())
	}
	quizView, _ := resp.Lesson["quiz"].(map[string]any)
	questions, _ := quizView["questions"].([]any)
	if len(questions) != 1 {
		t.Fatalf("в публичном тесте нет вопросов: %s", rec.Body.String())
	}

	// Code-урок: скрытые тесты не уходят наружу, видимые — остаются.
	rec = doRequest(t, f.router, http.MethodGet,
		"/api/v1/courses/"+f.slug+"/preview/"+codeID, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview code: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "hiddenSecret") {
		t.Fatalf("превью-ответ раскрывает скрытый тест: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "visibleTest") {
		t.Fatalf("в превью-ответе нет видимого теста: %s", rec.Body.String())
	}

	// Закрытый урок — 404 «Урок не найден».
	rec = doRequest(t, f.router, http.MethodGet,
		"/api/v1/courses/"+f.slug+"/preview/"+closedID, nil, nil)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "Урок не найден") {
		t.Fatalf("не-превью урок: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Несуществующий (и мусорный) id урока — 404 «Урок не найден».
	for _, lessonID := range []string{"no-such-lesson", "not-a-uuid"} {
		rec = doRequest(t, f.router, http.MethodGet,
			"/api/v1/courses/"+f.slug+"/preview/"+lessonID, nil, nil)
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "Урок не найден") {
			t.Fatalf("чужой id %q: status=%d body=%s", lessonID, rec.Code, rec.Body.String())
		}
	}

	// Второй курс (черновик): чужой урок и неопубликованный курс — 404.
	secondID, secondSlug, secondModuleID := f.createCourse(t, "Второй курс")
	foreign := f.postLesson(t, secondID, secondModuleID, map[string]any{
		"title": "Чужой превью-урок", "kind": "video_text", "bodyMd": "текст", "isPreview": true,
	})
	foreignID, _ := foreign["id"].(string)

	rec = doRequest(t, f.router, http.MethodGet,
		"/api/v1/courses/"+f.slug+"/preview/"+foreignID, nil, nil)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "Урок не найден") {
		t.Fatalf("урок другого курса: status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, f.router, http.MethodGet,
		"/api/v1/courses/"+secondSlug+"/preview/"+foreignID, nil, nil)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "Курс не найден") {
		t.Fatalf("неопубликованный курс: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Неизвестный slug — 404 «Курс не найден».
	rec = doRequest(t, f.router, http.MethodGet,
		"/api/v1/courses/no-such-course/preview/"+quizID, nil, nil)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "Курс не найден") {
		t.Fatalf("неизвестный slug: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
