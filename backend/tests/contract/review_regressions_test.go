package contract_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/repo"
)

type checkoutFailureStore struct {
	repo.StoreAPI
	fail bool
}

func TestReviewUnpublishedCourseCannotEraseBillingHistory(t *testing.T) {
	app, router := newTestApp()
	teacher, courseID, _ := seedPublishedCourse(t, app, router, 0)
	student, _ := registerUser(t, router, "course-history-review@example.com", "password123", "")
	enrolled := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{"courseId": courseID}, student)
	if enrolled.Code != 201 {
		t.Fatalf("enroll: %d %s", enrolled.Code, enrolled.Body.String())
	}
	unpublished := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/unpublish", nil, teacher)
	if unpublished.Code != 200 {
		t.Fatalf("unpublish: %d %s", unpublished.Code, unpublished.Body.String())
	}
	deleted := doRequest(t, router, http.MethodDelete, "/api/v1/teacher/courses/"+courseID, nil, teacher)
	if deleted.Code != 409 {
		t.Fatalf("billing history erased: %d %s", deleted.Code, deleted.Body.String())
	}
	if len(app.Store.ListPaymentsByCourse(courseID)) != 1 {
		t.Fatal("payment history lost")
	}
}

func (s *checkoutFailureStore) SetPaymentCheckout(id, providerID, redirect string) error {
	if s.fail {
		s.fail = false
		return errors.New("injected checkout write failure")
	}
	return s.StoreAPI.SetPaymentCheckout(id, providerID, redirect)
}

func TestReviewCheckoutRecoversAfterAttachmentFailure(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	student, studentID := registerUser(t, router, "checkout-write-review@example.com", "password123", "")
	app.Store = &checkoutFailureStore{StoreAPI: app.Store, fail: true}
	first := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{"courseId": courseID}, student)
	if first.Code != 500 {
		t.Fatalf("write failure: %d %s", first.Code, first.Body.String())
	}
	attempts := app.Store.ListPaymentsByStudent(studentID)
	if len(attempts) != 1 || attempts[0].ProviderPaymentID != "" {
		t.Fatalf("durable attempt missing: %+v", attempts)
	}
	admin := loginUser(t, router, "admin@example.com", "password123")
	result := doRequest(t, router, http.MethodPost, "/api/v1/admin/payments/"+attempts[0].ID+"/sync", nil, admin)
	if result.Code != 200 {
		t.Fatalf("reconcile: %d %s", result.Code, result.Body.String())
	}
	second := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{"courseId": courseID}, student)
	if second.Code != 200 {
		t.Fatalf("resume: %d %s", second.Code, second.Body.String())
	}
	var checkout map[string]any
	decodeBody(t, second, &checkout)
	if checkout["paymentId"] != attempts[0].ID || checkout["redirectUrl"] == "" || len(app.Store.ListPaymentsByStudent(studentID)) != 1 {
		t.Fatalf("attempt duplicated: %s", second.Body.String())
	}
	denied := doRequest(t, router, http.MethodPost, "/api/v1/admin/payments/"+attempts[0].ID+"/sync", nil, student)
	if denied.Code != 403 {
		t.Fatalf("student sync: %d", denied.Code)
	}
}

type refundFailureStore struct {
	repo.StoreAPI
	fail bool
}

func (s *refundFailureStore) SetEnrollmentStatus(student, course string, status repo.EnrollmentStatus) error {
	if s.fail && status == repo.EnrollmentRefunded {
		s.fail = false
		return errors.New("injected access write failure")
	}
	return s.StoreAPI.SetEnrollmentStatus(student, course, status)
}

func TestReviewRefundRetryRepairsPartialFailure(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	student, studentID := registerUser(t, router, "refund-write-review@example.com", "password123", "")
	providerID := beginCheckout(t, router, student, courseID)
	webhookMock(t, router, providerID, "succeeded")
	p, _ := app.Store.GetPaymentByProviderID(providerID)
	admin := loginUser(t, router, "admin@example.com", "password123")
	app.Store = &refundFailureStore{StoreAPI: app.Store, fail: true}
	path := "/api/v1/admin/payments/" + p.ID + "/refund"
	if result := doRequest(t, router, http.MethodPost, path, nil, admin); result.Code != 500 {
		t.Fatalf("first refund: %d %s", result.Code, result.Body.String())
	}
	if result := doRequest(t, router, http.MethodPost, path, nil, admin); result.Code != 200 {
		t.Fatalf("refund repair: %d %s", result.Code, result.Body.String())
	}
	if e, _ := app.Store.GetEnrollment(studentID, courseID); e.Status != repo.EnrollmentRefunded {
		t.Fatal("access not revoked")
	}
	if result := doRequest(t, router, http.MethodPost, path, nil, admin); result.Code != 409 {
		t.Fatalf("completed refund: %d", result.Code)
	}
}

func TestReviewPaidContentAndArchivedAccess(t *testing.T) {
	app, router := newTestApp()
	_, courseID, slug := seedPublishedCourse(t, app, router, 10000)
	module := app.Store.ListModulesByCourse(courseID)[0]
	app.Store.CreateLesson(&repo.Lesson{ModuleID: module.ID, Title: "Private", Kind: repo.LessonVideoText, BodyMD: "PRIVATE_BODY", VideoURL: "https://example.com/PRIVATE_VIDEO"})
	app.Store.CreateLesson(&repo.Lesson{ModuleID: module.ID, Title: "Preview", Kind: repo.LessonVideoText, BodyMD: "PUBLIC_PREVIEW", IsPreview: true})
	student, studentID := registerUser(t, router, "content-review@example.com", "password123", "")
	for _, cookies := range [][]*http.Cookie{nil, student} {
		rec := doRequest(t, router, http.MethodGet, "/api/v1/courses/"+slug, nil, cookies)
		if rec.Code != 200 || strings.Contains(rec.Body.String(), "PRIVATE_") || !strings.Contains(rec.Body.String(), "PUBLIC_PREVIEW") {
			t.Fatalf("content access: %d %s", rec.Code, rec.Body.String())
		}
	}
	app.Store.EnsureEnrollment(studentID, courseID)
	for _, status := range []repo.CourseStatus{repo.CoursePublished, repo.CourseArchived, repo.CourseDraft} {
		app.Store.SetCourseStatus(courseID, status, "")
		rec := doRequest(t, router, http.MethodGet, "/api/v1/courses/"+slug, nil, student)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "PRIVATE_BODY") {
			t.Fatalf("purchased %s: %d %s", status, rec.Code, rec.Body.String())
		}
		if status != repo.CoursePublished && doRequest(t, router, http.MethodGet, "/api/v1/courses/"+slug, nil, nil).Code != 404 {
			t.Fatal("unpublished course visible to guest")
		}
	}
}

type failedProgressStore struct{ repo.StoreAPI }

func (s failedProgressStore) CompleteLesson(string, string) (*repo.LessonProgress, bool) {
	return nil, false
}

func TestReviewFailedProgressAndLanguageOverride(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 0)
	student, studentID := registerUser(t, router, "progress-review@example.com", "password123", "")
	app.Store.EnsureEnrollment(studentID, courseID)
	module := app.Store.ListModulesByCourse(courseID)[0]
	video := app.Store.CreateLesson(&repo.Lesson{ModuleID: module.ID, Title: "Video", Kind: repo.LessonVideoText})
	app.Store = failedProgressStore{app.Store}
	rec := doRequest(t, router, http.MethodPost, "/api/v1/me/courses/"+courseID+"/lessons/"+video.ID+"/complete", nil, student)
	if rec.Code != 500 || strings.Contains(rec.Body.String(), `"completed":true`) {
		t.Fatalf("false completion: %d %s", rec.Code, rec.Body.String())
	}
	code := app.Store.CreateLesson(&repo.Lesson{ModuleID: module.ID, Title: "Python", Kind: repo.LessonCode, CodeTask: &repo.CodeTask{Language: "python"}})
	rec = doRequest(t, router, http.MethodPost, "/api/v1/me/courses/"+courseID+"/lessons/"+code.ID+"/complete", map[string]any{"code": "print(1)", "language": "htmlcss"}, student)
	if rec.Code != 400 {
		t.Fatalf("language override: %d %s", rec.Code, rec.Body.String())
	}
}

func TestReviewOldWebhooksPreserveCurrentAccess(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	student, studentID := registerUser(t, router, "replay-review@example.com", "password123", "")
	first := beginCheckout(t, router, student, courseID)
	webhookMock(t, router, first, "succeeded")
	app.Store.SetEnrollmentStatus(studentID, courseID, repo.EnrollmentRevoked)
	webhookMock(t, router, first, "succeeded")
	if e, _ := app.Store.GetEnrollment(studentID, courseID); e.Status != repo.EnrollmentRevoked {
		t.Fatal("success replay undid manual revoke")
	}
	webhookMock(t, router, first, "refunded")
	app.Store.SetEnrollmentStatus(studentID, courseID, repo.EnrollmentRefunded)
	second := beginCheckout(t, router, student, courseID)
	webhookMock(t, router, second, "succeeded")
	webhookMock(t, router, first, "refunded")
	if e, _ := app.Store.GetEnrollment(studentID, courseID); e.Status != repo.EnrollmentActive {
		t.Fatal("old refund revoked new purchase")
	}
}

func TestReviewCheckoutResumeAcrossRouterRestart(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	student, studentID := registerUser(t, router, "resume-review@example.com", "password123", "")
	// A stored external checkout is resumed without contacting its provider.
	app.Config.PaymentsProvider = "stripe"
	p, err := app.Store.CreatePayment(&repo.Payment{StudentID: studentID, CourseID: courseID, Provider: "stripe", ProviderPaymentID: "pi_resume", Status: repo.PaymentPending, RedirectURL: "https://example.com/checkout"})
	if err != nil {
		t.Fatal(err)
	}
	router = handler.NewRouter(&handler.App{Store: app.Store, Config: app.Config})
	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{"courseId": courseID}, student)
	var out map[string]any
	decodeBody(t, rec, &out)
	if rec.Code != 200 || out["paymentId"] != p.ID || out["redirectUrl"] != p.RedirectURL {
		t.Fatalf("resume: %d %s", rec.Code, rec.Body.String())
	}
}

func TestReviewConcurrentCheckoutAndPromoReservations(t *testing.T) {
	app, router := newTestApp()
	teacher, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	seedPromo(t, router, courseID, "ONLYONE", 20, 1, teacher)
	first, firstID := registerUser(t, router, "reservation-one@example.com", "password123", "")
	second, _ := registerUser(t, router, "reservation-two@example.com", "password123", "")
	var wg sync.WaitGroup
	responses := make([]*httptest.ResponseRecorder, 2)
	for i := range responses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			responses[i] = doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{"courseId": courseID, "promoCode": "ONLYONE"}, first)
		}(i)
	}
	wg.Wait()
	for _, rec := range responses {
		if rec.Code != 200 && rec.Code != 201 {
			t.Fatalf("concurrent checkout: %d %s", rec.Code, rec.Body.String())
		}
	}
	if len(app.Store.ListPaymentsByStudent(firstID)) != 1 {
		t.Fatal("duplicate payment attempts")
	}
	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{"courseId": courseID, "promoCode": "ONLYONE"}, second)
	if rec.Code != 400 {
		t.Fatalf("reserved promo reused: %d %s", rec.Code, rec.Body.String())
	}
}
