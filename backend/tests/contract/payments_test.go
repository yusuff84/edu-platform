package contract_test

import (
	"net/http"
	"strings"
	"testing"
)

// Покупка: enroll -> pending+redirect, повторный enroll до оплаты идемпотентен,
// webhook -> active, дубль webhook -> duplicate, повторная покупка -> 409.
func TestPaymentsEnrollAndWebhook(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)

	studentCookies, _ := registerUser(t, router, "buyer@example.com", "password123", "")

	// Покупка.
	var enroll map[string]any
	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &enroll)
	providerPaymentID, _ := enroll["providerPaymentId"].(string)
	if providerPaymentID == "" {
		t.Fatalf("нет providerPaymentId: %s", rec.Body.String())
	}
	if url, _ := enroll["redirectUrl"].(string); url == "" {
		t.Fatalf("нет redirectUrl: %s", rec.Body.String())
	}

	// До оплаты курса нет в «моих».
	rec = doRequest(t, router, http.MethodGet, "/api/v1/me/courses", nil, studentCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("me/courses: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var mine map[string]any
	decodeBody(t, rec, &mine)
	if total, _ := mine["total"].(float64); total != 0 {
		t.Fatalf("курс доступен до оплаты: %s", rec.Body.String())
	}

	// Webhook: оплата прошла.
	webhook := map[string]any{"providerPaymentId": providerPaymentID, "status": "succeeded"}
	rec = doRequest(t, router, http.MethodPost, "/api/v1/payments/webhook/mock", webhook, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var wh map[string]any
	decodeBody(t, rec, &wh)
	if wh["status"] != "processed" {
		t.Fatalf("webhook status=%v body=%s", wh["status"], rec.Body.String())
	}

	// Дубль webhook — идемпотентный duplicate.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/payments/webhook/mock", webhook, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook duplicate: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &wh)
	if wh["status"] != "duplicate" {
		t.Fatalf("webhook duplicate status=%v body=%s", wh["status"], rec.Body.String())
	}

	// Курс появился в «моих» с прогрессом 0 и nextLesson.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/me/courses", nil, studentCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("me/courses после оплаты: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &mine)
	if total, _ := mine["total"].(float64); total != 1 {
		t.Fatalf("me/courses total=%v body=%s", mine["total"], rec.Body.String())
	}

	// Повторная покупка — 409.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusConflict {
		t.Fatalf("re-enroll: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Сплит: комиссия 15% от 10000 = 1500 платформе, 8500 учителю.
	p, ok := app.Store.GetPaymentByProviderID(providerPaymentID)
	if !ok {
		t.Fatal("платёж не найден в store")
	}
	if p.PlatformFeeCents != 1500 || p.TeacherAmountCents != 8500 {
		t.Fatalf("сплит неверный: fee=%d teacher=%d", p.PlatformFeeCents, p.TeacherAmountCents)
	}
}

// Тесты «прохождение квиза» и «отзывы» перенесены в progress_test.go и
// reviews_test.go (T12): логика проверок не менялась, файлы разнесены по
// задачам specs (T034, T040).

// Mock-покупка сквозь страницу оплаты: enroll → redirectUrl /pay/mock/{id} →
// GET страницы (кнопка «Оплатить») → POST webhook со страницы → страница
// показывает «уже обработан», курс активируется. Раньше маршрут /pay/mock/{id}
// не был смонтирован и покупка в mock-режиме не работала вовсе (T13).
func TestMockPayPageEndToEnd(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	studentCookies, _ := registerUser(t, router, "mockpage@example.com", "password123", "")

	// Покупка: получаем redirectUrl.
	var enroll map[string]any
	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &enroll)
	redirectURL, _ := enroll["redirectUrl"].(string)
	if redirectURL == "" {
		t.Fatalf("нет redirectUrl: %s", rec.Body.String())
	}

	// Страница оплаты открывается и показывает кнопку «Оплатить».
	rec = doRequest(t, router, http.MethodGet, redirectURL, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("страница оплаты: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `id="pay"`) {
		t.Fatalf("на странице pending-платежа нет кнопки «Оплатить»: %s", rec.Body.String())
	}

	// Имитируем нажатие кнопки: страница шлёт этот же webhook.
	providerPaymentID, _ := enroll["providerPaymentId"].(string)
	rec = doRequest(t, router, http.MethodPost, "/api/v1/payments/webhook/mock", map[string]any{
		"providerPaymentId": providerPaymentID, "status": "succeeded",
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Страница после оплаты не показывает кнопки.
	rec = doRequest(t, router, http.MethodGet, redirectURL, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("страница оплаты после webhook: status=%d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `id="pay"`) {
		t.Fatalf("после оплаты кнопка не должна отображаться: %s", rec.Body.String())
	}

	// Доступ к курсу открыт.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/me/courses", nil, studentCookies)
	var mine map[string]any
	decodeBody(t, rec, &mine)
	if total, _ := mine["total"].(float64); total != 1 {
		t.Fatalf("курс не активировался после оплаты: %s", rec.Body.String())
	}
}
