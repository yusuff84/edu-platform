package contract_test

// Контрактные тесты T47: возврат платежа (POST /admin/payments/{id}/refund) —
// форма ответа {"payment": <строка журнала>} из Task/00-api-contract.md
// («Эндпоинты волны P6»). Возврат возможен только из succeeded; применение
// сериализуется a.webhookMu, поэтому два параллельных возврата одного платежа
// дают ровно один 200 и один 409.

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"edu-platform/backend/internal/repo"
)

// Счастливый путь: покупка mock → вебхук succeeded → возврат админом →
// 200 (refunded, processedAt заполнен, studentEmail/studentName есть),
// запись стала refunded, курс исчез из /me/courses, повтор → 409.
// Плюс: pending-платёж → 409, случайный UUID → 404, студент → 403.
func TestAdminRefundPaymentFlow(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	adminCookies := loginUser(t, router, "admin@example.com", "password123")
	studentCookies, studentID := registerUser(t, router, "refund-student@example.com", "password123", "")

	// Покупка mock и успешный вебхук — платёж в succeeded.
	pid := beginCheckout(t, router, studentCookies, courseID)
	if rec := webhookMock(t, router, pid, "succeeded"); rec.Code != http.StatusOK {
		t.Fatalf("webhook succeeded: status=%d body=%s", rec.Code, rec.Body.String())
	}
	p, ok := app.Store.GetPaymentByProviderID(pid)
	if !ok || p.Status != repo.PaymentSucceeded {
		t.Fatalf("платёж не в succeeded: %+v", p)
	}

	// Возврат: 200, элемент в форме строки журнала.
	rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/payments/"+p.ID+"/refund", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("refund: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	payment, _ := out["payment"].(map[string]any)
	if payment == nil {
		t.Fatalf("в ответе нет payment: %s", rec.Body.String())
	}
	if payment["id"] != p.ID || payment["status"] != "refunded" {
		t.Fatalf("форма payment неверна: %s", rec.Body.String())
	}
	if payment["processedAt"] == nil {
		t.Fatalf("processedAt должен быть заполнен (момент возврата): %s", rec.Body.String())
	}
	if payment["studentEmail"] != "refund-student@example.com" {
		t.Fatalf("studentEmail отсутствует или неверен: %s", rec.Body.String())
	}
	if _, hasName := payment["studentName"].(string); !hasName {
		t.Fatalf("studentName должен быть строкой (допустимо пустой): %s", rec.Body.String())
	}
	if payment["courseTitle"] != "Go для новичков" || payment["amountCents"] != float64(10000) || payment["providerPaymentId"] != pid {
		t.Fatalf("поля строки журнала в ответе: %s", rec.Body.String())
	}

	// Платёж в хранилище: refunded + processedAt.
	updated, _ := app.Store.GetPaymentByID(p.ID)
	if updated == nil || updated.Status != repo.PaymentRefunded || updated.ProcessedAt == nil {
		t.Fatalf("платёж после возврата: %+v", updated)
	}
	// Запись ученика переведена в refunded, курс исчез из /me/courses.
	if e, ok := app.Store.GetEnrollment(studentID, courseID); !ok || e.Status != repo.EnrollmentRefunded {
		t.Fatalf("запись на курс после возврата: %+v", e)
	}
	if total := myCoursesTotal(t, router, studentCookies); total != 0 {
		t.Fatalf("курс остался в /me/courses: total=%d", total)
	}
	// В журнале платежей видна строка refunded.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/payments?status=refunded", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("журнал refunded: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var journal map[string]any
	decodeBody(t, rec, &journal)
	if total, _ := journal["total"].(float64); total != 1 {
		t.Fatalf("журнал: total=%v, ждали 1 refunded", journal["total"])
	}

	// Повторный возврат → 409 тем же текстом.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/payments/"+p.ID+"/refund", nil, adminCookies)
	if rec.Code != http.StatusConflict {
		t.Fatalf("повторный refund: status=%d body=%s, ждали 409", rec.Code, rec.Body.String())
	}
	var errBody map[string]any
	decodeBody(t, rec, &errBody)
	if errBody["error"] != "Возврат возможен только для успешного платежа" {
		t.Fatalf("текст 409: %v", errBody["error"])
	}

	// Несуществующий платёж → 404.
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/admin/payments/00000000-0000-0000-0000-000000000000/refund", nil, adminCookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("refund несуществующего: status=%d body=%s, ждали 404", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &errBody)
	if errBody["error"] != "Платёж не найден" {
		t.Fatalf("текст 404: %v", errBody["error"])
	}

	// Pending-платёж (без вебхука) → 409: возвращать нечего.
	student2Cookies, _ := registerUser(t, router, "refund-student2@example.com", "password123", "")
	pendingPID := beginCheckout(t, router, student2Cookies, courseID)
	pending, ok := app.Store.GetPaymentByProviderID(pendingPID)
	if !ok || pending.Status != repo.PaymentPending {
		t.Fatalf("ожидали pending-платёж: %+v", pending)
	}
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/payments/"+pending.ID+"/refund", nil, adminCookies)
	if rec.Code != http.StatusConflict {
		t.Fatalf("refund pending: status=%d body=%s, ждали 409", rec.Code, rec.Body.String())
	}

	// Студент → 403, аноним → 401.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/payments/"+p.ID+"/refund", nil, studentCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("студент на refund: status=%d body=%s, ждали 403", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/payments/"+p.ID+"/refund", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("аноним на refund: status=%d, ждали 401", rec.Code)
	}
}

// Ошибка провайдера → 502: бесплатный платёж проведён без провайдера
// (детерминированный providerPaymentId "free-..." не зарегистрирован в
// реестре mock), поэтому Refund отвечает ошибкой. Платёж и доступ остаются
// нетронутыми; повторный refund снова 502 (не 409).
func TestAdminRefundProviderFailure(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 0) // бесплатный курс
	adminCookies := loginUser(t, router, "admin@example.com", "password123")
	studentCookies, _ := registerUser(t, router, "refund-free@example.com", "password123", "")

	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("бесплатная покупка: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var enroll map[string]any
	decodeBody(t, rec, &enroll)
	pid, _ := enroll["providerPaymentId"].(string)
	p, ok := app.Store.GetPaymentByProviderID(pid)
	if !ok || p.Status != repo.PaymentSucceeded {
		t.Fatalf("бесплатный платёж не в succeeded: %+v", p)
	}

	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/payments/"+p.ID+"/refund", nil, adminCookies)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("ошибка провайдера: status=%d body=%s, ждали 502", rec.Code, rec.Body.String())
	}
	var errBody map[string]any
	decodeBody(t, rec, &errBody)
	if errBody["error"] != "Не удалось выполнить возврат у провайдера" {
		t.Fatalf("текст 502: %v", errBody["error"])
	}
	// Состояние не изменилось: платёж succeeded, доступ активен.
	if after, _ := app.Store.GetPaymentByID(p.ID); after == nil || after.Status != repo.PaymentSucceeded {
		t.Fatalf("платёж изменился после 502: %+v", after)
	}
	if total := myCoursesTotal(t, router, studentCookies); total != 1 {
		t.Fatalf("доступ пропал после 502: total=%d", total)
	}
}

// Возврат mock-платежа после «рестарта»: реестр MockProvider пуст, но платёж
// с providerPaymentId mock_... лежит в Store в статусе succeeded. Возврат
// должен пройти локально (денег вне платформы нет), а не падать в 502.
func TestAdminRefundMockAfterRestart(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	adminCookies := loginUser(t, router, "admin@example.com", "password123")
	studentCookies, studentID := registerUser(t, router, "refund-restart@example.com", "password123", "")
	app.Store.EnsureEnrollment(studentID, courseID)

	p, err := app.Store.CreatePayment(&repo.Payment{
		StudentID:          studentID,
		CourseID:           courseID,
		AmountCents:        10000,
		PlatformFeeCents:   1500,
		TeacherAmountCents: 8500,
		Currency:           "RUB",
		Provider:           "mock",
		ProviderPaymentID:  "mock_after_restart",
		Status:             repo.PaymentSucceeded,
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/payments/"+p.ID+"/refund", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("refund после рестарта: status=%d body=%s", rec.Code, rec.Body.String())
	}
	updated, ok := app.Store.GetPaymentByID(p.ID)
	if !ok || updated.Status != repo.PaymentRefunded {
		t.Fatalf("платёж после возврата: ok=%v got=%+v", ok, updated)
	}
	if e, ok := app.Store.GetEnrollment(studentID, courseID); !ok || e.Status != repo.EnrollmentRefunded {
		t.Fatalf("запись после возврата: ok=%v got=%+v", ok, e)
	}
	if total := myCoursesTotal(t, router, studentCookies); total != 0 {
		t.Fatalf("доступ остался после возврата: total=%d", total)
	}
}

// Гонка (для -race): два параллельных возврата одного платежа → ровно один
// 200 и один 409 (применение сериализуется a.webhookMu — прецедент P5).
func TestAdminRefundConcurrentRace(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	adminCookies := loginUser(t, router, "admin@example.com", "password123")
	studentCookies, studentID := registerUser(t, router, "refund-race@example.com", "password123", "")

	pid := beginCheckout(t, router, studentCookies, courseID)
	if rec := webhookMock(t, router, pid, "succeeded"); rec.Code != http.StatusOK {
		t.Fatalf("webhook succeeded: status=%d body=%s", rec.Code, rec.Body.String())
	}
	p, ok := app.Store.GetPaymentByProviderID(pid)
	if !ok || p.Status != repo.PaymentSucceeded {
		t.Fatalf("платёж не в succeeded: %+v", p)
	}

	const parallel = 2
	codes := make([]int, parallel)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Свой request/recorder на каждую горутину: общий httptest-объект
			// сам по себе был бы гонкой.
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/payments/"+p.ID+"/refund", bytes.NewReader(nil))
			req.Header.Set("Content-Type", "application/json")
			for _, c := range adminCookies {
				req.AddCookie(c)
			}
			rec := httptest.NewRecorder()
			<-start // одновременный старт обоих возвратов
			router.ServeHTTP(rec, req)
			codes[i] = rec.Code
		}(i)
	}
	close(start)
	wg.Wait()

	ok200, conflict409 := 0, 0
	for i, code := range codes {
		switch code {
		case http.StatusOK:
			ok200++
		case http.StatusConflict:
			conflict409++
		default:
			t.Fatalf("возврат %d: status=%d, ждали 200 или 409", i, code)
		}
	}
	if ok200 != 1 || conflict409 != 1 {
		t.Fatalf("200=%d, 409=%d: ровно один возврат должен примениться (codes=%v)", ok200, conflict409, codes)
	}

	// Итог однократный: платёж refunded, запись refunded, доступа нет.
	updated, _ := app.Store.GetPaymentByID(p.ID)
	if updated == nil || updated.Status != repo.PaymentRefunded || updated.ProcessedAt == nil {
		t.Fatalf("платёж после гонки: %+v", updated)
	}
	if e, ok := app.Store.GetEnrollment(studentID, courseID); !ok || e.Status != repo.EnrollmentRefunded {
		t.Fatalf("запись на курс после гонки: %+v", e)
	}
	if total := myCoursesTotal(t, router, studentCookies); total != 0 {
		t.Fatalf("курс доступен после возврата: total=%d", total)
	}
	// Второй возврат после завершения гонки → 409 (не 200).
	rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/payments/"+p.ID+"/refund", nil, adminCookies)
	if rec.Code != http.StatusConflict {
		t.Fatalf("refund после гонки: status=%d body=%s, ждали 409", rec.Code, rec.Body.String())
	}
}
