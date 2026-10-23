package contract_test

// Контрактные тесты T55 — детали курса для админа (GET /admin/courses/{id}):
// форма ответа и порядок программы, 404/403/401, статистика.
//
// Форма заморожена разделом T55 Task/00-api-contract.md: course — элемент
// GET /admin/courses (T25) плюс coverPath; lessons внутри модуля — ТОЛЬКО
// id/title/kind/position (лишних полей быть не должно); stats полностью
// нулевой у курса, на который никто не записывался.

import (
	"net/http"
	"testing"

	"edu-platform/backend/internal/repo"
)

// adminCourseDetail запрашивает детали курса и возвращает разобранный ответ.
func adminCourseDetail(t *testing.T, router http.Handler, cookies []*http.Cookie, courseID string) map[string]any {
	t.Helper()
	rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/courses/"+courseID, nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("детали курса %s: status=%d body=%s", courseID, rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	return out
}

// createCourseWithProgram строит курс с 2 модулями: в первом — video_text и
// quiz (позиции 1 и 2), во втором — live. Возвращает id курса.
func createCourseWithProgram(t *testing.T, router http.Handler, teacherCookies []*http.Cookie) string {
	t.Helper()
	courseID := createDraftCourse(t, router, teacherCookies, "Детали курса для админа")

	// Модуль 1 (position 1)
	var modA map[string]any
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/modules", map[string]any{
		"title": "Модуль А",
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("module A: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &modA)

	// Урок 1 (video_text, position 1)
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
		"moduleId": modA["id"], "title": "Текст и видео", "kind": "video_text", "bodyMd": "текст",
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("lesson video_text: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Урок 2 (quiz, position 2)
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
		"moduleId": modA["id"], "title": "Проверочный тест", "kind": "quiz",
		"quiz": map[string]any{"questions": []any{map[string]any{
			"id": "q1", "text": "Два плюс два?", "options": []string{"три", "четыре"}, "correct": 1,
		}}},
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("lesson quiz: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Модуль 2 (position 2)
	var modB map[string]any
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/modules", map[string]any{
		"title": "Модуль Б",
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("module B: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &modB)

	// Урок live в модуле 2 (kind №3 в выдаче)
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
		"moduleId": modB["id"], "title": "Вебинар", "kind": "live", "liveUrl": "https://meet.example.com/webinar",
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("lesson live: status=%d body=%s", rec.Code, rec.Body.String())
	}
	return courseID
}

// ---- Сценарий 1: happy path — форма ответа и порядок программы ----

// TestAdminCourseDetailHappyPath: курс с двумя модулями и уроками разных
// kinds; программа отсортирована по position; урок — только метаданные;
// teacherEmail непустой; форма course совпадает с элементом списка T25
// плюс coverPath (null: T54 ещё не влита).
func TestAdminCourseDetailHappyPath(t *testing.T) {
	app, router := newTestApp()
	// Эталонная строка списка T25 (seedPublishedCourse заводит и админа,
	// необходимого для входа) + сравниваемые курсы создаются дальше.
	_, otherID, _ := seedPublishedCourse(t, app, router, 10000)
	_ = otherID
	adminCookies := loginUser(t, router, "admin@example.com", "password123")
	teacherCookies, _ := registerUser(t, router, "detail-teacher@example.com", "password123", "")
	if !app.Store.SetUserRole(userIDByEmail(t, app.Store, "detail-teacher@example.com"), repo.RoleTeacher) {
		t.Fatal("не удалось выдать роль teacher")
	}

	courseID := createCourseWithProgram(t, router, teacherCookies)

	rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/courses", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("список курсов: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var list map[string]any
	decodeBody(t, rec, &list)
	items, _ := list["items"].([]any)
	published := courseItemByID(t, items, courseID)

	body := adminCourseDetail(t, router, adminCookies, courseID)

	// --- course: ровно 16 полей = 13 элемента списка T25 + coverPath + category (T65) + moderationComment (T87) ---
	course, ok := body["course"].(map[string]any)
	if !ok {
		t.Fatalf("ответ без course: %v", body)
	}
	wantKeys(t, course, "id", "slug", "title", "category", "teacherId", "teacherEmail", "status",
		"priceCents", "createdAt", "updatedAt", "modulesCount", "lessonsCount",
		"studentsCount", "coverPath", "moderationComment")
	if course["id"] != courseID {
		t.Fatalf("course.id=%v, ждали %s", course["id"], courseID)
	}
	// Элемент списка и детали совпадают по общим полям (та же форма T25).
	for _, k := range []string{"slug", "title", "teacherId", "teacherEmail", "status",
		"priceCents", "modulesCount", "lessonsCount", "studentsCount"} {
		if course[k] != published[k] {
			t.Fatalf("поле %q отличается от элемента списка: детали %v, список %v", k, course[k], published[k])
		}
	}
	if course["teacherEmail"] != "detail-teacher@example.com" {
		t.Fatalf("teacherEmail пуст или чужой: %v", course["teacherEmail"])
	}
	if course["coverPath"] != nil {
		t.Fatalf("без T54 coverPath должен быть null: %v", course["coverPath"])
	}
	// T87: аддитивное поле всегда строка; у курса без замечаний — пустая.
	if course["moderationComment"] != "" {
		t.Fatalf("moderationComment без замечаний должен быть пустым: %v", course["moderationComment"])
	}

	// --- modules: 2 модуля по position, порядок стабильный ---
	mods, ok := body["modules"].([]any)
	if !ok {
		t.Fatalf("modules не массив: %v", body)
	}
	if len(mods) != 2 {
		t.Fatalf("модулей %d, ждали 2: %v", len(mods), mods)
	}
	modFirst, ok := mods[0].(map[string]any)
	if !ok {
		t.Fatalf("модуль 0 не объект: %v", mods[0])
	}
	wantKeys(t, modFirst, "id", "title", "position", "lessons")
	if modFirst["title"] != "Модуль А" || modFirst["position"] != float64(1) {
		t.Fatalf("первый модуль не «Модуль А» по position 1: %v", modFirst)
	}
	if mods[1].(map[string]any)["title"] != "Модуль Б" {
		t.Fatalf("второй модуль не «Модуль Б»: %v", mods[1])
	}

	// --- lessons: внутри модуля по position; kinds разные; только 4 поля ---
	lessons, ok := modFirst["lessons"].([]any)
	if !ok {
		t.Fatalf("lessons не массив: %v", modFirst)
	}
	if len(lessons) != 2 {
		t.Fatalf("уроков в модуле А %d, ждали 2: %v", len(lessons), lessons)
	}
	first := lessons[0].(map[string]any)
	wantKeys(t, first, "id", "title", "kind", "position")
	if first["title"] != "Текст и видео" || first["kind"] != "video_text" || first["position"] != float64(1) {
		t.Fatalf("первый урок неверен (kind/position): %v", first)
	}
	second := lessons[1].(map[string]any)
	if second["kind"] != "quiz" || second["position"] != float64(2) {
		t.Fatalf("второй урок не quiz по position 2: %v", second)
	}
	modSecond := mods[1].(map[string]any)
	lessonsB, ok := modSecond["lessons"].([]any)
	if !ok || len(lessonsB) != 1 {
		t.Fatalf("уроков в модуле Б должно быть 1: %v", modSecond)
	}
	if lessonsB[0].(map[string]any)["kind"] != "live" {
		t.Fatalf("в модуле Б ждали live-урок: %v", lessonsB[0])
	}

	// --- stats: программа есть, учеников и денег нет ---
	stats, ok := body["stats"].(map[string]any)
	if !ok {
		t.Fatalf("ответ без stats: %v", body)
	}
	wantKeys(t, stats, "studentsCount", "activeCount", "reviewsCount", "avgRating",
		"paymentsCount", "paidTotalCents")
	for _, k := range []string{"studentsCount", "activeCount", "reviewsCount", "paymentsCount", "paidTotalCents"} {
		wantNum(t, stats, k, 0)
	}
	wantNum(t, stats, "avgRating", 0)
}

// ---- Сценарий 2: 404 на несуществующий курс ----

// TestAdminCourseDetailNotFound: любого fake id (в том числе не-UUID) —
// 404 «Курс не найден».
func TestAdminCourseDetailNotFound(t *testing.T) {
	app, router := newTestApp()
	// seedPublishedCourse заводит админа, необходимого для входа; сам курс в
	// проверках не участвует — запрашиваются заведомо несуществующие id.
	_, _, _ = seedPublishedCourse(t, app, router, 0)
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	for _, id := range []string{"00000000-0000-0000-0000-000000000000", "fake-id"} {
		rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/courses/"+id, nil, adminCookies)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("детали %q: status=%d body=%s", id, rec.Code, rec.Body.String())
		}
		var out map[string]any
		decodeBody(t, rec, &out)
		if out["error"] != "Курс не найден" {
			t.Fatalf("текст ошибки %q, ждали «Курс не найден»: %v", id, out["error"])
		}
	}
}

// ---- Сценарий 3: 403 учителю, 401 без сессии ----

// TestAdminCourseDetailRoleGuards: teacher-сессия — 403, аноним — 401.
func TestAdminCourseDetailRoleGuards(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 0)

	// Учитель (даже currentUser) — 403.
	rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/courses/"+courseID, nil, teacherCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("учитель на деталях курса: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Без cookie — 401 (requireAuth срабатывает раньше requireRole).
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/courses/"+courseID, nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("аноним на деталях курса: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// ---- Сценарий 4: статистика нулевого курса ----

// TestAdminCourseDetailEmptyStats: у курса без записей, отзывов и платежей
// все счётчики stats равны нулю; modules — пустой массив (не null); поле
// coverPath присутствует со значением null.
func TestAdminCourseDetailEmptyStats(t *testing.T) {
	app, router := newTestApp()
	// seedPublishedCourse заводит admin/teacher@example.com; курсов без
	// записей, отзывов и платежей у него нет, поэтому stats нулевой, а у
	// ключевого «пустого» курса нет даже программы.
	_, _, _ = seedPublishedCourse(t, app, router, 0)
	teacherCookies := loginUser(t, router, "teacher@example.com", "password123")
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	emptyID := createDraftCourse(t, router, teacherCookies, "Пустой курс без программы")

	body := adminCourseDetail(t, router, adminCookies, emptyID)
	stats, ok := body["stats"].(map[string]any)
	if !ok {
		t.Fatalf("ответ без stats: %s", body)
	}
	for _, k := range []string{"studentsCount", "activeCount", "reviewsCount", "avgRating", "paymentsCount", "paidTotalCents"} {
		wantNum(t, stats, k, 0)
	}
	// Пустая программа — массив [], не null.
	mods, isArray := body["modules"].([]any)
	if !isArray || len(mods) != 0 {
		t.Fatalf("у пустого курса modules должен быть []: %v", body["modules"])
	}
	course := body["course"].(map[string]any)
	if cover, present := course["coverPath"]; !present || cover != nil {
		t.Fatalf("coverPath должен присутствовать и быть null: %v (есть поле: %v)", cover, present)
	}
	// Счётчики элемента курса тоже нулевые.
	wantNum(t, course, "modulesCount", 0)
	wantNum(t, course, "lessonsCount", 0)
	wantNum(t, course, "studentsCount", 0)
}

// ---- Сценарий 5: статистика курса с учеником/отзывом/платежом ----

// TestAdminCourseDetailStatsWithActivity: запись на бесплатный курс активна
// сразу (succeeded платёж на 0 копеек), отзыв поднимает reviewsCount/avgRating;
// studentsCount и activeCount считают записи.
func TestAdminCourseDetailStatsWithActivity(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 0) // бесплатный
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	studentCookies, _ := registerUser(t, router, "detail-student@example.com", "password123", "")
	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodPost, "/api/v1/courses/"+courseID+"/reviews", map[string]any{
		"rating": 5, "body": "Хороший курс",
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("review: status=%d body=%s", rec.Code, rec.Body.String())
	}

	body := adminCourseDetail(t, router, adminCookies, courseID)
	stats := body["stats"].(map[string]any)
	wantNum(t, stats, "studentsCount", 1)
	wantNum(t, stats, "activeCount", 1)
	wantNum(t, stats, "reviewsCount", 1)
	wantNum(t, stats, "avgRating", 5)
	wantNum(t, stats, "paymentsCount", 1)
	// Бесплатный курс: платёж зафиксирован, но нулевой.
	wantNum(t, stats, "paidTotalCents", 0)

	// studentsCount элемента курса — то же значение (активная запись), что в списке.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/courses", nil, adminCookies)
	var list map[string]any
	decodeBody(t, rec, &list)
	items, _ := list["items"].([]any)
	published := courseItemByID(t, items, courseID)
	wantNum(t, published, "studentsCount", 1)
}

// Отзывы курса в админке остаются прежними: GET /admin/courses/{id}/reviews
// не перехвачен новым маршрутом GET /admin/courses/{id} (T25, не трогаем).
func TestAdminCourseDetailDoesNotShadowReviews(t *testing.T) {
	app, router := newTestApp()
	// Админ нужен только для сессии — его заводит seedPublishedCourse.
	_, _, _ = seedPublishedCourse(t, app, router, 0)
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	rec := doRequest(t, router, http.MethodGet,
		"/api/v1/admin/courses/00000000-0000-0000-0000-000000000000/reviews", nil, adminCookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("reviews чужого курса: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
