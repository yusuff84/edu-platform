package handler

// Страница mock-оплаты читает платёж из Store: после «рестарта» (пустой
// in-memory реестр провайдера) данные всё равно отображаются.

import (
	"net/http"
	"strings"
	"testing"

	"edu-platform/backend/internal/repo"
)

func TestMockPaymentPageReadsStore(t *testing.T) {
	app, router := newTestRouter(devConfig())
	teacher, err := app.Store.CreateUser("mock-page-teacher@example.test", "hash", repo.RoleTeacher)
	if err != nil {
		t.Fatalf("CreateUser(teacher): %v", err)
	}
	c := app.Store.CreateCourse(&repo.Course{
		TeacherID: teacher.ID, Slug: "mock-page-course", Title: "Курс для mock-оплаты",
		Description: "Описание", PriceCents: 12345, Currency: "RUB",
	})
	app.Store.SetCourseStatus(c.ID, repo.CoursePublished, "")
	if _, err := app.Store.CreatePayment(&repo.Payment{
		StudentID: teacher.ID, CourseID: c.ID, AmountCents: 12345,
		Currency: "RUB", Provider: "mock", ProviderPaymentID: "mock_durable",
		Status: repo.PaymentPending,
	}); err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	rec := doJSON(t, router, http.MethodGet, "/pay/mock/mock_durable", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("Content-Type=%q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{"mock_durable", "Курс для mock-оплаты", "123.45", "pending"} {
		if !strings.Contains(body, want) {
			t.Fatalf("на странице нет %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Платёж не найден") {
		t.Fatalf("известный платёж показан как ненайденный:\n%s", body)
	}
}
