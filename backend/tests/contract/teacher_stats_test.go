package contract_test

// Контрактные тесты T77 — аналитика учителя (волна P9):
// GET /api/v1/teacher/stats и GET /api/v1/teacher/stats/courses.
//
// Проверяются: пустая сводка, агрегаты по нескольким курсам/ученикам,
// выручка только по succeeded-платежам, завершённые записи и средний прогресс,
// рейтинг (только published), last30d, порядок разбивки, limit, доступ
// (ученик → 403) и изоляция курсов чужого преподавателя. Формы ответов —
// Task/00-api-contract.md, раздел «T77 — Аналитика учителя».

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"edu-platform/backend/internal/repo"
)

// t77RegisterRole регистрирует пользователя и выдаёт ему роль напрямую в
// store (API смены ролей нет — как promoteToAdmin в helper_test.go).
func t77RegisterRole(t *testing.T, app interface {
	SetUserRole(id string, role repo.Role) bool
}, router http.Handler, email string, role repo.Role) (cookies []*http.Cookie, id string) {
	t.Helper()
	cookies, id = registerUser(t, router, email, "password123", "")
	if !app.SetUserRole(id, role) {
		t.Fatalf("не удалось выдать роль %s пользователю %s", role, email)
	}
	return cookies, id
}

// t77Course создаёт курс преподавателя с заданным статусом и числом уроков
// (один модуль). CreateCourse всегда создаёт draft — статус меняется
// SetCourseStatus, как на реальном пути модерации.
func t77Course(t *testing.T, store repo.StoreAPI, teacherID, title string, priceCents, lessons int, status repo.CourseStatus) *repo.Course {
	t.Helper()
	c := store.CreateCourse(&repo.Course{
		TeacherID:   teacherID,
		Slug:        "t77-" + uuid.NewString(),
		Title:       title,
		Description: "Курс контрактного теста T77",
		PriceCents:  priceCents,
		Currency:    "RUB",
	})
	if c == nil || c.ID == "" {
		t.Fatalf("CreateCourse(%q) не вернул id", title)
	}
	if status != repo.CourseDraft {
		if !store.SetCourseStatus(c.ID, status, "") {
			t.Fatalf("SetCourseStatus(%q, %s) = false", title, status)
		}
	}
	if lessons > 0 {
		m := store.CreateModule(c.ID, "Модуль T77")
		for i := 0; i < lessons; i++ {
			store.CreateLesson(&repo.Lesson{
				ModuleID: m.ID,
				Title:    fmt.Sprintf("Урок %d", i+1),
				Kind:     repo.LessonVideoText,
				BodyMD:   "текст урока",
			})
		}
	}
	return c
}

// t77Payment создаёт платёж и переводит его в заданный статус. Комиссия —
// 15%, как в проде: platformFeeCents/teacherAmountCents пишутся в платёж и
// потом сверяются в сводке.
func t77Payment(t *testing.T, store repo.StoreAPI, studentID, courseID string, amountCents int, status repo.PaymentStatus) *repo.Payment {
	t.Helper()
	fee := amountCents * 15 / 100
	p, err := store.CreatePayment(&repo.Payment{
		StudentID:          studentID,
		CourseID:           courseID,
		AmountCents:        amountCents,
		PlatformFeeCents:   fee,
		TeacherAmountCents: amountCents - fee,
		Currency:           "RUB",
		Provider:           "mock",
		ProviderPaymentID:  "t77-pay-" + uuid.NewString(),
		Status:             repo.PaymentPending,
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if status != repo.PaymentPending && !store.SetPaymentStatus(p.ID, status) {
		t.Fatalf("SetPaymentStatus(%s) = false", status)
	}
	return p
}

// t77Review оставляет отзыв и пересчитывает карточку курса (как handler).
func t77Review(t *testing.T, store repo.StoreAPI, studentID, courseID string, rating int) {
	t.Helper()
	if _, err := store.CreateReview(studentID, courseID, rating, "Отзыв T77"); err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
	store.RecalcCourseRating(courseID)
}

// t77Overview запрашивает сводку и возвращает разобранный JSON.
func t77Overview(t *testing.T, router http.Handler, cookies []*http.Cookie) map[string]any {
	t.Helper()
	rec := doRequest(t, router, http.MethodGet, "/api/v1/teacher/stats", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /teacher/stats: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	return out
}

// t77CourseStats запрашивает разбивку с query-строкой и возвращает items/total.
func t77CourseStats(t *testing.T, router http.Handler, cookies []*http.Cookie, query string) (items []map[string]any, total int) {
	t.Helper()
	rec := doRequest(t, router, http.MethodGet, "/api/v1/teacher/stats/courses"+query, nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /teacher/stats/courses%s: status=%d body=%s", query, rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	rawItems, ok := out["items"].([]any)
	if !ok {
		t.Fatalf("items должен быть массивом: %s", rec.Body.String())
	}
	totalFloat, _ := out["total"].(float64)
	for _, raw := range rawItems {
		items = append(items, raw.(map[string]any))
	}
	return items, int(totalFloat)
}

// t77Section достаёт вложенный объект сводки с проверкой наличия всех полей.
func t77Section(t *testing.T, out map[string]any, name string, fields []string) map[string]any {
	t.Helper()
	raw, ok := out[name].(map[string]any)
	if !ok {
		t.Fatalf("в сводке нет объекта %q: %v", name, out)
	}
	if len(raw) != len(fields) {
		t.Fatalf("%s: полей %d, ждали %d: %v", name, len(raw), len(fields), raw)
	}
	for _, f := range fields {
		if _, ok := raw[f]; !ok {
			t.Fatalf("%s: нет поля %q: %v", name, f, raw)
		}
	}
	return raw
}

// num проверяет числовое поле и возвращает его значение.
func t77Num(t *testing.T, m map[string]any, key string, want float64) {
	t.Helper()
	got, _ := m[key].(float64)
	if got != want {
		t.Fatalf("%s=%v, ждали %v: %v", key, m[key], want, m)
	}
}

// Пустая сводка: все блоки и поля присутствуют, значения нулевые; пустая
// разбивка — items:[] и total:0. Доступ: без сессии — 401, ученику — 403.
// Учитель без курсов видит нули, даже если у другого учителя курсы есть.
func TestTeacherStatsEmptyAndAccess(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, teacherID := t77RegisterRole(t, app.Store, router, "t77-empty-teacher@example.com", repo.RoleTeacher)
	studentCookies, _ := registerUser(t, router, "t77-empty-student@example.com", "password123", "")

	// У второго учителя есть курс: он не должен просачиваться в чужую сводку.
	otherCookies, otherID := t77RegisterRole(t, app.Store, router, "t77-empty-other@example.com", repo.RoleTeacher)
	t77Course(t, app.Store, otherID, "Чужой курс", 1000, 1, repo.CoursePublished)
	_ = teacherID

	out := t77Overview(t, router, teacherCookies)
	courses := t77Section(t, out, "courses", []string{"total", "published", "draft", "review", "archived"})
	students := t77Section(t, out, "students", []string{"total", "unique"})
	revenue := t77Section(t, out, "revenue", []string{"paidTotalCents", "platformFeeCents", "teacherCents"})
	completion := t77Section(t, out, "completion", []string{"completed", "avgPercent"})
	ratings := t77Section(t, out, "ratings", []string{"avgRating", "ratingsCount"})
	last30d := t77Section(t, out, "last30d", []string{"enrollments", "revenueCents"})
	for _, section := range []map[string]any{courses, students, revenue, completion, ratings, last30d} {
		for key := range section {
			t77Num(t, section, key, 0)
		}
	}

	items, total := t77CourseStats(t, router, teacherCookies, "")
	if len(items) != 0 || total != 0 {
		t.Fatalf("разбивка учителя без курсов: items=%v total=%d", items, total)
	}

	// Без сессии — 401, ученику — 403 (requireAuth/requireRole группы).
	for _, path := range []string{"/api/v1/teacher/stats", "/api/v1/teacher/stats/courses"} {
		rec := doRequest(t, router, http.MethodGet, path, nil, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s без сессии: status=%d, ждали 401: %s", path, rec.Code, rec.Body.String())
		}
		rec = doRequest(t, router, http.MethodGet, path, nil, studentCookies)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s ученику: status=%d, ждали 403: %s", path, rec.Code, rec.Body.String())
		}
	}

	// Для чужого учителя его собственный курс виден, а сводка первого — нет.
	otherOut := t77Overview(t, router, otherCookies)
	if otherOut["courses"].(map[string]any)["total"] != float64(1) {
		t.Fatalf("сводка второго учителя: %v", otherOut)
	}
}

// Основной сценарий: три курса своего учителя (published/published/draft),
// курсы и данные чужого учителя, активные/refunded записи, succeeded/pending/
// refunded платежи, прогресс 100/50/0, отзывы (published и draft), last30d.
func TestTeacherStatsOverviewAndCourses(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, teacherID := t77RegisterRole(t, app.Store, router, "t77-teacher@example.com", repo.RoleTeacher)
	adminCookies, _ := t77RegisterRole(t, app.Store, router, "t77-admin@example.com", repo.RoleAdmin)
	otherCookies, otherID := t77RegisterRole(t, app.Store, router, "t77-other@example.com", repo.RoleTeacher)

	store := app.Store
	c1 := t77Course(t, store, teacherID, "A: Go", 10000, 4, repo.CoursePublished)
	c2 := t77Course(t, store, teacherID, "B: SQL", 5000, 2, repo.CoursePublished)
	c3 := t77Course(t, store, teacherID, "C: Draft", 0, 0, repo.CourseDraft)
	c4 := t77Course(t, store, otherID, "D: Чужой", 7000, 1, repo.CoursePublished)

	_, s1 := registerUser(t, router, "t77-s1@example.com", "password123", "")
	_, s2 := registerUser(t, router, "t77-s2@example.com", "password123", "")
	_, s3 := registerUser(t, router, "t77-s3@example.com", "password123", "")

	// Активные записи: s1/s2 на c1, s3 на c2, s2 на чужом c4.
	for _, pair := range [][2]string{{s1, c1.ID}, {s2, c1.ID}, {s3, c2.ID}, {s2, c4.ID}} {
		store.EnsureEnrollment(pair[0], pair[1])
	}
	// Отчисленная запись s1 на c2 в учеников/прогресс не попадает.
	store.EnsureEnrollment(s1, c2.ID)
	if err := store.SetEnrollmentStatus(s1, c2.ID, repo.EnrollmentRefunded); err != nil {
		t.Fatalf("SetEnrollmentStatus: %v", err)
	}

	// Прогресс: s1 — все 4 урока c1 (100%), s2 — 2/4 (50%),
	// s3 — 1/2 c2 (50%). У чужого курса прогресса нет.
	lessons1 := store.ListLessonsByCourse(c1.ID)
	if len(lessons1) != 4 {
		t.Fatalf("уроков c1=%d, ждали 4", len(lessons1))
	}
	for _, l := range lessons1 {
		store.CompleteLesson(s1, l.ID)
	}
	for _, l := range lessons1[:2] {
		store.CompleteLesson(s2, l.ID)
	}
	lessons2 := store.ListLessonsByCourse(c2.ID)
	store.CompleteLesson(s3, lessons2[0].ID)

	// Платежи: у c1 два succeeded и один pending (не выручка), у c2 succeeded
	// с последующим refunded (не выручка), у чужого c4 — succeeded (не виден).
	t77Payment(t, store, s1, c1.ID, 10000, repo.PaymentSucceeded)
	t77Payment(t, store, s2, c1.ID, 10000, repo.PaymentSucceeded)
	t77Payment(t, store, s3, c1.ID, 10000, repo.PaymentPending)
	t77Payment(t, store, s3, c2.ID, 5000, repo.PaymentRefunded)
	t77Payment(t, store, s2, c4.ID, 7000, repo.PaymentSucceeded)

	// Отзывы: c1 — 5 и 4 (avg 4.5), c2 — 3, draft c3 — 1 (в рейтинг сводки
	// не входит), чужой c4 — 5.
	t77Review(t, store, s1, c1.ID, 5)
	t77Review(t, store, s2, c1.ID, 4)
	t77Review(t, store, s3, c2.ID, 3)
	t77Review(t, store, s2, c3.ID, 1)
	t77Review(t, store, s2, c4.ID, 5)

	// ---- Сводка учителя ----
	out := t77Overview(t, router, teacherCookies)
	courses := t77Section(t, out, "courses", []string{"total", "published", "draft", "review", "archived"})
	t77Num(t, courses, "total", 3)
	t77Num(t, courses, "published", 2)
	t77Num(t, courses, "draft", 1)
	t77Num(t, courses, "review", 0)
	t77Num(t, courses, "archived", 0)

	students := t77Section(t, out, "students", []string{"total", "unique"})
	t77Num(t, students, "total", 3) // refunded-запись s1 на c2 не считается
	t77Num(t, students, "unique", 3)

	// Только succeeded: 10000 + 10000; pending и refunded не в счёте.
	revenue := t77Section(t, out, "revenue", []string{"paidTotalCents", "platformFeeCents", "teacherCents"})
	t77Num(t, revenue, "paidTotalCents", 20000)
	t77Num(t, revenue, "platformFeeCents", 3000)
	t77Num(t, revenue, "teacherCents", 17000)

	// completed: s1 на c1; avgPercent = (100 + 50 + 50) / 3 = 66.7.
	completion := t77Section(t, out, "completion", []string{"completed", "avgPercent"})
	t77Num(t, completion, "completed", 1)
	t77Num(t, completion, "avgPercent", 66.7)

	// Published-курсы: c1 (4.5×2) + c2 (3.0×1) → 12/3 = 4.0; draft c3 не входит.
	ratings := t77Section(t, out, "ratings", []string{"avgRating", "ratingsCount"})
	t77Num(t, ratings, "avgRating", 4.0)
	t77Num(t, ratings, "ratingsCount", 3)

	last30d := t77Section(t, out, "last30d", []string{"enrollments", "revenueCents"})
	t77Num(t, last30d, "enrollments", 3)
	t77Num(t, last30d, "revenueCents", 20000)

	// ---- Разбивка по курсам: порядок revenue desc → students desc → title ----
	items, total := t77CourseStats(t, router, teacherCookies, "")
	if total != 3 || len(items) != 3 {
		t.Fatalf("разбивка: total=%d items=%d, ждали 3: %v", total, len(items), items)
	}
	wantOrder := []string{c1.ID, c2.ID, c3.ID}
	for i, want := range wantOrder {
		if items[i]["courseId"] != want {
			t.Fatalf("порядок курсов: [%d]=%v, ждали %s; все: %v", i, items[i]["courseId"], want, items)
		}
	}
	// Точный набор полей строки.
	for _, row := range items {
		if len(row) != 8 {
			t.Fatalf("в строке курса %d полей, ждали 8: %v", len(row), row)
		}
		for _, key := range []string{"courseId", "title", "status", "students",
			"revenueCents", "avgRating", "ratingsCount", "avgPercent"} {
			if _, ok := row[key]; !ok {
				t.Fatalf("в строке курса нет %q: %v", key, row)
			}
		}
	}
	row1, row2, row3 := items[0], items[1], items[2]
	t77Num(t, row1, "students", 2)
	t77Num(t, row1, "revenueCents", 20000)
	t77Num(t, row1, "avgRating", 4.5)
	t77Num(t, row1, "ratingsCount", 2)
	t77Num(t, row1, "avgPercent", 75) // (100 + 50) / 2

	t77Num(t, row2, "students", 1) // refunded-запись s1 не считается
	t77Num(t, row2, "revenueCents", 0)
	t77Num(t, row2, "avgRating", 3)
	t77Num(t, row2, "ratingsCount", 1)
	t77Num(t, row2, "avgPercent", 50)

	// Draft-курс входит в разбивку (любой статус), но без учеников и выручки.
	t77Num(t, row3, "students", 0)
	t77Num(t, row3, "revenueCents", 0)
	t77Num(t, row3, "avgPercent", 0)
	if row3["status"] != string(repo.CourseDraft) {
		t.Fatalf("статус draft-курса в разбивке: %v", row3["status"])
	}

	// limit: 2 → два первых курса, но total — все 3 курса в области видимости
	// (limit только укорачивает items; это не пагинация).
	limited, limitedTotal := t77CourseStats(t, router, teacherCookies, "?limit=2")
	if limitedTotal != 3 || len(limited) != 2 || limited[0]["courseId"] != c1.ID || limited[1]["courseId"] != c2.ID {
		t.Fatalf("limit=2: total=%d items=%v", limitedTotal, limited)
	}
	for _, q := range []string{"?limit=abc", "?limit=0", "?limit=-5", "?limit=500"} {
		got, gotTotal := t77CourseStats(t, router, teacherCookies, q)
		if gotTotal != 3 || len(got) != 3 {
			t.Fatalf("%s: total=%d items=%d, ждали 3", q, gotTotal, len(got))
		}
	}

	// ---- Admin видит все курсы платформы ----
	adminOut := t77Overview(t, router, adminCookies)
	adminCourses := t77Section(t, adminOut, "courses", []string{"total", "published", "draft", "review", "archived"})
	t77Num(t, adminCourses, "total", 4)
	t77Num(t, adminCourses, "published", 3)
	t77Num(t, adminCourses, "draft", 1)
	adminStudents := t77Section(t, adminOut, "students", []string{"total", "unique"})
	t77Num(t, adminStudents, "total", 4)
	t77Num(t, adminStudents, "unique", 3)
	adminRevenue := t77Section(t, adminOut, "revenue", []string{"paidTotalCents", "platformFeeCents", "teacherCents"})
	t77Num(t, adminRevenue, "paidTotalCents", 27000)
	t77Num(t, adminRevenue, "platformFeeCents", 4050)
	t77Num(t, adminRevenue, "teacherCents", 22950)
	adminCompletion := t77Section(t, adminOut, "completion", []string{"completed", "avgPercent"})
	t77Num(t, adminCompletion, "completed", 1)
	t77Num(t, adminCompletion, "avgPercent", 50) // (100+50+50+0)/4
	adminRatings := t77Section(t, adminOut, "ratings", []string{"avgRating", "ratingsCount"})
	t77Num(t, adminRatings, "avgRating", 4.3) // (4.5×2 + 3.0 + 5.0) / 4 = 4.25
	t77Num(t, adminRatings, "ratingsCount", 4)
	adminLast := t77Section(t, adminOut, "last30d", []string{"enrollments", "revenueCents"})
	t77Num(t, adminLast, "enrollments", 4)
	t77Num(t, adminLast, "revenueCents", 27000)

	adminItems, adminTotal := t77CourseStats(t, router, adminCookies, "")
	if adminTotal != 4 || len(adminItems) != 4 {
		t.Fatalf("admin-разбивка: total=%d items=%d, ждали 4", adminTotal, len(adminItems))
	}
	for i, want := range []string{c1.ID, c4.ID, c2.ID, c3.ID} {
		if adminItems[i]["courseId"] != want {
			t.Fatalf("admin-порядок: [%d]=%v, ждали %s; все: %v", i, adminItems[i]["courseId"], want, adminItems)
		}
	}

	// ---- Чужой учитель видит только свой курс ----
	otherOut := t77Overview(t, router, otherCookies)
	otherCourses := t77Section(t, otherOut, "courses", []string{"total", "published", "draft", "review", "archived"})
	t77Num(t, otherCourses, "total", 1)
	t77Num(t, otherCourses, "published", 1)
	otherStudents := t77Section(t, otherOut, "students", []string{"total", "unique"})
	t77Num(t, otherStudents, "total", 1)
	t77Num(t, otherStudents, "unique", 1)
	otherRevenue := t77Section(t, otherOut, "revenue", []string{"paidTotalCents", "platformFeeCents", "teacherCents"})
	t77Num(t, otherRevenue, "paidTotalCents", 7000)
	t77Num(t, otherRevenue, "platformFeeCents", 1050)
	t77Num(t, otherRevenue, "teacherCents", 5950)
	otherRatings := t77Section(t, otherOut, "ratings", []string{"avgRating", "ratingsCount"})
	t77Num(t, otherRatings, "avgRating", 5)
	t77Num(t, otherRatings, "ratingsCount", 1)
	otherItems, otherTotal := t77CourseStats(t, router, otherCookies, "")
	if otherTotal != 1 || len(otherItems) != 1 || otherItems[0]["courseId"] != c4.ID {
		t.Fatalf("разбивка чужого учителя: total=%d items=%v", otherTotal, otherItems)
	}
}
