package handler

// P11: точечные проверки платёжной безопасности на уровне handler/payments,
// которые неудобно наблюдать только через HTTP: dev-режим mock-провайдера и
// безопасный дефолт PAYMENTS_PROVIDER в config.Load.

import (
	"errors"
	"testing"

	"edu-platform/backend/internal/config"
	"edu-platform/backend/internal/payments"
)

// Mock-провайдер существует только в dev: фабрика вне dev возвращает
// ErrNotConfigured, в dev — рабочий MockProvider.
func TestP11MockProviderDevOnly(t *testing.T) {
	if _, err := payments.NewProvider("mock", false); !errors.Is(err, payments.ErrNotConfigured) {
		t.Fatalf("NewProvider(mock, false): err=%v, ждали ErrNotConfigured", err)
	}
	p, err := payments.NewProvider("mock", true)
	if err != nil || p.Name() != "mock" {
		t.Fatalf("NewProvider(mock, true): p=%v err=%v", p, err)
	}
}

// config.Load в prod с пустым PAYMENTS_PROVIDER не оставляет mock: безопасный
// явный дефолт — stripe (без ключей checkout вернёт 502, а не создаст
// тестовый платёж).
func TestP11ConfigProdEmptyProviderIsNotMock(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://edu:edu@db:5432/edu")
	t.Setenv("SESSION_SECRET", "p11-config-secret")
	t.Setenv("PAYMENTS_PROVIDER", "")

	cfg := config.Load()
	if cfg.IsDev() {
		t.Fatal("конфиг с APP_ENV=production распознан как dev")
	}
	if cfg.PaymentsProvider != "stripe" {
		t.Fatalf("PAYMENTS_PROVIDER пуст в prod: %q, ждали безопасный дефолт stripe", cfg.PaymentsProvider)
	}
}

// В dev пустой PAYMENTS_PROVIDER по-прежнему означает mock — локальная
// разработка и контрактные тесты не должны требовать ключей.
func TestP11ConfigDevEmptyProviderIsMock(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("SESSION_SECRET", "p11-config-secret")
	t.Setenv("PAYMENTS_PROVIDER", "")

	cfg := config.Load()
	if !cfg.IsDev() {
		t.Fatal("конфиг с APP_ENV=development не распознан как dev")
	}
	if cfg.PaymentsProvider != "mock" {
		t.Fatalf("PAYMENTS_PROVIDER пуст в dev: %q, ждали mock", cfg.PaymentsProvider)
	}
}
