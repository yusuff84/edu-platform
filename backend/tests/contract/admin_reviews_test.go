package contract_test

// Контрактные тесты T83 — глобальная модерация отзывов:
//   GET /admin/reviews — фильтры courseId/rating/q/from/to, сортировка
//   sort/dir, пагинация page/limit; 400 на неверный query; 401/403.
// Удаление — существующий DELETE /admin/reviews/{id} (T25); здесь проверяется,
// что из глобального списка отзыв удаляется и рейтинг курса пересчитывается.
//
// Форма элементов — ровно по контракту T83 (items всегда массив):
// id, courseId, courseTitle, courseSlug, studentId, studentEmail, studentName,
// rating, body, createdAt, teacherReply, teacherReplyAt.

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/repo"
)

// adminReviewsFixture — данные сценария T83: два опубликованных курса,
// три ученика и четыре отзыва в порядке создания r1..r4.
type adminReviewsFixture struct {
	app            *handler.App
	router         http.Handler
	adminCookies   []*http.Cookie
	teacherCookies []*http.Cookie
	courseA        string
	courseB        string
	slugA          string
	slugB          string
	student1       string
	student2       string
	student3       string
	r1, r2, r3, r4 string
}

// getAdminReviews запрашивает GET /admin/reviews и проверяет 200 + массив
// items. query — без ведущего «?»; пустая строка — без параметров.
func getAdminReviews(t *testing.T, router http.Handler, cookies []*http.Cookie, query string) (map[string]any, []any) {
	t.Helper()
	path := "/api/v1/admin/reviews"
	if query != "" {
		path += "?" + query
	}
	rec := doRequest(t, router, http.MethodGet, path, nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status=%d body=%s", path, rec.Code, rec.Body.String())
	}
	var body map[string]any
	decodeBody(t, rec, &body)
	items, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("items отсутствует или не массив: %s", rec.Body.String())
	}
	return body, items
}

// adminReviewByID находит строку глобального списка отзывов по id.
func adminReviewByID(t *testing.T, items []any, id string) map[string]any {
	t.Helper()
	for _, raw := range items {
		row, ok := raw.(map[string]any)
		if ok && row["id"] == id {
			return row
		}
	}
	t.Fatalf("отзыв %s не найден среди %d строк", id, len(items))
	return nil
}

// reviewEnroll записывает ученика на бесплатный курс (доступ сразу активен).
func reviewEnroll(t *testing.T, router http.Handler, cookies []*http.Cookie, courseID string) {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll %s: status=%d body=%s", courseID, rec.Code, rec.Body.String())
	}
}

// reviewCreate оставляет отзыв и возвращает его id.
func reviewCreate(t *testing.T, router http.Handler, cookies []*http.Cookie, courseID string, rating int, body string) string {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost, "/api/v1/courses/"+courseID+"/reviews", map[string]any{
		"rating": rating, "body": body,
	}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create review (%d, %q): status=%d body=%s", rating, body, rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	id, _ := out["id"].(string)
	if id == "" {
		t.Fatalf("CreateReview не вернул id: %s", rec.Body.String())
	}
	return id
}

// seedAdminReviewsFixture собирает два published-курса, трёх учеников и
// четыре отзыва. Отзывы создаются последовательно, поэтому createdAt у них
// строго возрастает (in-memory nextCreatedAt).
func seedAdminReviewsFixture(t *testing.T) *adminReviewsFixture {
	t.Helper()
	app, router := newTestApp()
	teacherCookies, courseA, slugA := seedPublishedCourse(t, app, router, 0) // бесплатный
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	// Второй опубликованный курс — тем же учителем, через модерацию.
	courseB := createDraftCourse(t, router, teacherCookies, "Python для аналитиков")
	submitCourseThroughReview(t, router, teacherCookies, courseB)
	rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+courseB+"/moderate", map[string]any{
		"decision": "approve",
	}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("moderate second course: status=%d body=%s", rec.Code, rec.Body.String())
	}
	courseBData, ok := app.Store.GetCourseByID(courseB)
	if !ok {
		t.Fatal("второй курс не найден в хранилище")
	}

	student1Cookies, student1 := registerUser(t, router, "t83-anna@example.com", "password123", "")
	student2Cookies, student2 := registerUser(t, router, "t83-boris@example.com", "password123", "")
	student3Cookies, student3 := registerUser(t, router, "t83-vera@example.com", "password123", "")
	if !app.Store.SetUserDisplayName(student1, "Анна Смирнова") {
		t.Fatal("не удалось задать display_name ученику 1")
	}
	if !app.Store.SetUserDisplayName(student2, "Борис Петров") {
		t.Fatal("не удалось задать display_name ученику 2")
	}

	reviewEnroll(t, router, student1Cookies, courseA)
	reviewEnroll(t, router, student2Cookies, courseA)
	reviewEnroll(t, router, student1Cookies, courseB)
	reviewEnroll(t, router, student3Cookies, courseB)

	f := &adminReviewsFixture{
		app: app, router: router, adminCookies: adminCookies, teacherCookies: teacherCookies,
		courseA: courseA, courseB: courseB, slugA: slugA, slugB: courseBData.Slug,
		student1: student1, student2: student2, student3: student3,
	}
	// Порядок важен: r4 — самый новый, r1 — самый старый.
	f.r1 = reviewCreate(t, router, student1Cookies, courseA, 5, "Отличный курс про Go")
	f.r2 = reviewCreate(t, router, student2Cookies, courseA, 4, "")
	f.r3 = reviewCreate(t, router, student3Cookies, courseB, 5, "Нужен Python")
	f.r4 = reviewCreate(t, router, student1Cookies, courseB, 3, "Python сложный")
	return f
}

// TestAdminReviewsEmptyAndGuards: пустой список — total 0, items []; на
// эндпоинт пускают только admin (401 без сессии, 403 студенту и учителю).
func TestAdminReviewsEmptyAndGuards(t *testing.T) {
	app, router := newTestApp()

	adminCookies, _ := registerUser(t, router, "t83-admin@example.com", "password123", "")
	promoteToAdmin(t, app, "t83-admin@example.com")

	body, items := getAdminReviews(t, router, adminCookies, "")
	wantNum(t, body, "total", 0)
	wantNum(t, body, "page", 1)
	wantNum(t, body, "limit", 20)
	if len(items) != 0 {
		t.Fatalf("на пустом хранилище items=%d, ждали 0", len(items))
	}

	// Прямой вызов хранилища тоже возвращает не-nil срез.
	if rows, total := app.Store.ListAdminReviews(repo.AdminReviewFilter{}); rows == nil || total != 0 {
		t.Fatalf("ListAdminReviews на пустом хранилище: rows=%v total=%d, ждали [] и 0", rows, total)
	}

	studentCookies, _ := registerUser(t, router, "t83-plain-student@example.com", "password123", "")
	teacherCookies, _ := registerUser(t, router, "t83-plain-teacher@example.com", "password123", "")
	if !app.Store.SetUserRole(userIDByEmail(t, app.Store, "t83-plain-teacher@example.com"), repo.RoleTeacher) {
		t.Fatal("не удалось выдать роль teacher")
	}
	for _, who := range []struct {
		name    string
		cookies []*http.Cookie
	}{{"студент", studentCookies}, {"учитель", teacherCookies}} {
		rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/reviews", nil, who.cookies)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s на /admin/reviews: status=%d body=%s", who.name, rec.Code, rec.Body.String())
		}
	}
	rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/reviews", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("аноним на /admin/reviews: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestAdminReviewsFiltersSortAndPaging: форма строки, фильтры courseId/
// rating/q/from-to, сортировки, пагинация и total до limit.
func TestAdminReviewsFiltersSortAndPaging(t *testing.T) {
	f := seedAdminReviewsFixture(t)
	router, cookies := f.router, f.adminCookies

	// По умолчанию — createdAt desc (r4, r3, r2, r1).
	body, items := getAdminReviews(t, router, cookies, "")
	wantNum(t, body, "total", 4)
	wantNum(t, body, "page", 1)
	wantNum(t, body, "limit", 20)
	if len(items) != 4 {
		t.Fatalf("items=%d, ждали 4", len(items))
	}
	order := []string{f.r4, f.r3, f.r2, f.r1}
	for i, wantID := range order {
		if got := items[i].(map[string]any)["id"]; got != wantID {
			t.Fatalf("порядок по умолчанию [%d]=%v, ждали %s", i, got, wantID)
		}
	}

	// Полная форма строки контракта T83.
	newest := adminReviewByID(t, items, f.r4)
	wantKeys(t, newest, "id", "courseId", "courseTitle", "courseSlug", "studentId",
		"studentEmail", "studentName", "rating", "body", "createdAt",
		"teacherReply", "teacherReplyAt")
	if newest["courseId"] != f.courseB || newest["courseTitle"] != "Python для аналитиков" ||
		newest["courseSlug"] != f.slugB {
		t.Fatalf("данные курса в строке неверны: %v", newest)
	}
	if newest["studentId"] != f.student1 || newest["studentEmail"] != "t83-anna@example.com" ||
		newest["studentName"] != "Анна Смирнова" {
		t.Fatalf("данные ученика в строке неверны: %v", newest)
	}
	if newest["rating"] != float64(3) || newest["body"] != "Python сложный" {
		t.Fatalf("оценка/текст в строке неверны: %v", newest)
	}
	if newest["teacherReply"] != "" || newest["teacherReplyAt"] != nil {
		t.Fatalf("без ответа teacherReply=%v teacherReplyAt=%v: ждали пусто/null",
			newest["teacherReply"], newest["teacherReplyAt"])
	}

	// Отзыв без текста отдаётся с body "" (не null).
	emptyBody := adminReviewByID(t, items, f.r2)
	if emptyBody["body"] != "" {
		t.Fatalf("body пустого отзыва=%v, ждали пустую строку", emptyBody["body"])
	}
	if _, ok := emptyBody["createdAt"].(string); !ok {
		t.Fatalf("createdAt не строка: %v", emptyBody["createdAt"])
	}

	// Фильтр courseId.
	body, items = getAdminReviews(t, router, cookies, "courseId="+f.courseA)
	wantNum(t, body, "total", 2)
	if len(items) != 2 {
		t.Fatalf("courseId=%s: items=%d, ждали 2", f.courseA, len(items))
	}
	for _, raw := range items {
		if raw.(map[string]any)["courseId"] != f.courseA {
			t.Fatalf("courseId-фильтр вернул чужой курс: %v", raw)
		}
	}

	// Фильтр rating.
	body, items = getAdminReviews(t, router, cookies, "rating=5")
	wantNum(t, body, "total", 2)
	for _, raw := range items {
		if raw.(map[string]any)["rating"] != float64(5) {
			t.Fatalf("rating-фильтр вернул чужую оценку: %v", raw)
		}
	}
	body, _ = getAdminReviews(t, router, cookies, "rating=2")
	wantNum(t, body, "total", 0)

	// Поиск q: название курса, email, имя и текст отзыва, без учёта регистра.
	for _, tc := range []struct {
		name, query string
		want        float64
	}{
		{"название курса", "q=" + url.QueryEscape("PYTHON"), 2},
		{"email ученика", "q=" + url.QueryEscape("T83-ANNA@"), 2},
		{"имя ученика", "q=" + url.QueryEscape("борис"), 1},
		{"текст отзыва", "q=" + url.QueryEscape("ПРО GO"), 1},
		{"нет совпадений", "q=" + url.QueryEscape("нетакого"), 0},
	} {
		body, items = getAdminReviews(t, router, cookies, tc.query)
		wantNum(t, body, "total", tc.want)
		if tc.want == 0 {
			if len(items) != 0 {
				t.Fatalf("q=%q: items=%d, ждали []", tc.query, len(items))
			}
		}
	}

	// Период from/to включительно: сегодня все 4, завтра — 0.
	today := time.Now().UTC().Format("2006-01-02")
	tomorrow := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	body, _ = getAdminReviews(t, router, cookies, "from="+today+"&to="+today)
	wantNum(t, body, "total", 4)
	body, _ = getAdminReviews(t, router, cookies, "from="+tomorrow+"&to="+tomorrow)
	wantNum(t, body, "total", 0)

	// Сортировка по оценке: dir по умолчанию desc.
	_, items = getAdminReviews(t, router, cookies, "sort=rating")
	ratings := make([]float64, 0, len(items))
	for _, raw := range items {
		ratings = append(ratings, raw.(map[string]any)["rating"].(float64))
	}
	if len(ratings) != 4 || ratings[0] != 5 || ratings[1] != 5 || ratings[2] != 4 || ratings[3] != 3 {
		t.Fatalf("sort=rating&dir=desc: %v, ждали 5,5,4,3", ratings)
	}
	_, items = getAdminReviews(t, router, cookies, "sort=rating&dir=asc")
	ratings = ratings[:0]
	for _, raw := range items {
		ratings = append(ratings, raw.(map[string]any)["rating"].(float64))
	}
	if len(ratings) != 4 || ratings[0] != 3 || ratings[1] != 4 || ratings[2] != 5 || ratings[3] != 5 {
		t.Fatalf("sort=rating&dir=asc: %v, ждали 3,4,5,5", ratings)
	}

	// Сортировка createdAt asc — обратный порядок создания.
	_, items = getAdminReviews(t, router, cookies, "sort=createdAt&dir=asc")
	if items[0].(map[string]any)["id"] != f.r1 || items[3].(map[string]any)["id"] != f.r4 {
		t.Fatalf("sort=createdAt&dir=asc: %v", items)
	}

	// Пагинация: total до limit, страницы не пересекаются.
	body, items = getAdminReviews(t, router, cookies, "page=1&limit=2")
	wantNum(t, body, "total", 4)
	wantNum(t, body, "page", 1)
	wantNum(t, body, "limit", 2)
	if len(items) != 2 || items[0].(map[string]any)["id"] != f.r4 || items[1].(map[string]any)["id"] != f.r3 {
		t.Fatalf("page=1&limit=2: %v", items)
	}
	body, items = getAdminReviews(t, router, cookies, "page=2&limit=2")
	wantNum(t, body, "total", 4)
	if len(items) != 2 || items[0].(map[string]any)["id"] != f.r2 || items[1].(map[string]any)["id"] != f.r1 {
		t.Fatalf("page=2&limit=2: %v", items)
	}
	body, items = getAdminReviews(t, router, cookies, "page=3&limit=2")
	wantNum(t, body, "total", 4)
	if len(items) != 0 {
		t.Fatalf("page=3&limit=2: items=%d, ждали []", len(items))
	}
	body, _ = getAdminReviews(t, router, cookies, "limit=500")
	wantNum(t, body, "limit", 100)

	// Неверные параметры — 400.
	for _, query := range []string{
		"courseId=" + url.QueryEscape("не-uuid"),
		"rating=0",
		"rating=6",
		"rating=abc",
		"rating=4.5",
		"from=" + url.QueryEscape("2026-02-30"),
		"to=" + url.QueryEscape("24.10.2026"),
		"from=2026-10-25&to=2026-10-24",
		"sort=body",
		"dir=up",
		"page=abc",
		"limit=abc",
	} {
		rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/reviews?"+query, nil, cookies)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status=%d body=%s", query, rec.Code, rec.Body.String())
		}
	}
}

// TestAdminReviewsDeleteRecalculatesRating: удаление из глобального списка —
// существующий DELETE /admin/reviews/{id}; после него отзыв исчезает, рейтинг
// курса и сводка пересчитаны, повторное удаление — 404.
func TestAdminReviewsDeleteRecalculatesRating(t *testing.T) {
	f := seedAdminReviewsFixture(t)
	router, cookies := f.router, f.adminCookies

	// Курс A: оценки 5 и 4 → среднее 4.5.
	course, ok := f.app.Store.GetCourseByID(f.courseA)
	if !ok || course.AvgRating != 4.5 || course.RatingsCount != 2 {
		t.Fatalf("до удаления рейтинг курса A: ok=%v %+v, ждали 4.5/2", ok, course)
	}

	rec := doRequest(t, router, http.MethodDelete, "/api/v1/admin/reviews/"+f.r1, nil, cookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("204 должен быть без тела: %q", rec.Body.String())
	}

	body, items := getAdminReviews(t, router, cookies, "courseId="+f.courseA)
	wantNum(t, body, "total", 1)
	if len(items) != 1 || items[0].(map[string]any)["id"] != f.r2 {
		t.Fatalf("после удаления список курса A неверен: %v", items)
	}
	course, _ = f.app.Store.GetCourseByID(f.courseA)
	if course.AvgRating != 4 || course.RatingsCount != 1 {
		t.Fatalf("рейтинг курса A не пересчитан: avg=%v count=%d", course.AvgRating, course.RatingsCount)
	}

	// Сводка админки: 3 отзыва, среднее (4+5+3)/3 = 4.0.
	stats := statsSection(t, adminStats(t, router, cookies), "reviews")
	wantNum(t, stats, "total", 3)
	wantNum(t, stats, "avgRating", 4)

	rec = doRequest(t, router, http.MethodDelete, "/api/v1/admin/reviews/"+f.r1, nil, cookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("повторное удаление: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodDelete,
		"/api/v1/admin/reviews/00000000-0000-0000-0000-000000000000", nil, cookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("несуществующий отзыв: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
