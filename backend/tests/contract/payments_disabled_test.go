package contract_test

import (
	"net/http"
	"testing"
)

func TestDisabledPaymentsKeepFreeCoursesAndRejectPaidCheckouts(t *testing.T) {
	for _, price := range []int{0, 10000} {
		name := "free"
		if price > 0 {
			name = "paid"
		}
		t.Run(name, func(t *testing.T) {
			app, router := newTestApp()
			_, courseID, _ := seedPublishedCourse(t, app, router, price)
			app.Config.PaymentsProvider = "disabled"
			cookies, studentID := registerUser(t, router, "disabled-payments@example.com", "password123", "")
			rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{"courseId": courseID, "provider": "mock"}, cookies)
			if price == 0 {
				if rec.Code != http.StatusCreated {
					t.Fatalf("free enrollment: %d %s", rec.Code, rec.Body.String())
				}
				if enrollment, ok := app.Store.GetEnrollment(studentID, courseID); !ok || enrollment == nil {
					t.Fatal("free enrollment was not persisted")
				}
			} else {
				if rec.Code != http.StatusServiceUnavailable {
					t.Fatalf("paid checkout: %d %s", rec.Code, rec.Body.String())
				}
				if len(app.Store.ListPaymentsByCourse(courseID)) != 0 {
					t.Fatal("disabled checkout created a payment")
				}
				if _, ok := app.Store.GetEnrollment(studentID, courseID); ok {
					t.Fatal("disabled checkout granted access")
				}
			}
		})
	}
}
