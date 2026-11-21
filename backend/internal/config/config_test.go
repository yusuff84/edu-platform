package config

// Тесты production-safety конфигурации: Load больше не допускает production
// с SESSION_SECRET=change-me/пустым, mock-оплатой (даже с секретом вебхука) и
// SMTP_TLS=none. Dev-режим сохраняет удобные дефолты.

import (
	"strings"
	"testing"
)

// productionConfig — заведомо безопасный production-конфиг.
func productionConfig() *Config {
	return &Config{
		AppEnv:           "production",
		DatabaseURL:      "postgres://edu:edu@db:5432/edu?sslmode=disable",
		SessionSecret:    "случайный-прод-секрет-123",
		PaymentsProvider: "yookassa",
		SMTPTLS:          "starttls",
	}
}

func TestProductionAllowsExplicitlyDisabledPayments(t *testing.T) {
	cfg := productionConfig()
	cfg.PaymentsProvider = "disabled"
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountEmailRequiresSafeProductionURL(t *testing.T) {
	for _, value := range []string{"", "http://localhost:3000", "https://user:password@edu.example", "https://edu.example?token=x", "https://edu.example#token=x"} {
		cfg := productionConfig()
		cfg.SMTPAddr = "smtp.example:587"
		cfg.SMTPFrom = "support@example.com"
		cfg.PublicAppURL = value
		if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), "PUBLIC_APP_URL") {
			t.Fatalf("unsafe account link base accepted: %q %v", value, err)
		}
	}
	cfg := productionConfig()
	cfg.SMTPAddr = "smtp.example:587"
	cfg.SMTPFrom = "support@example.com"
	cfg.PublicAppURL = "https://edu.example"
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
}

// TestValidateDevAllowsDefaults — dev не ломаем: change-me, mock и SMTP_TLS=none
// допустимы (с предупреждениями в логе).
func TestValidateDevAllowsDefaults(t *testing.T) {
	cfg := &Config{
		AppEnv:           "development",
		SessionSecret:    defaultSessionSecret,
		PaymentsProvider: "mock",
		SMTPTLS:          "none",
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("dev-конфиг не должен отвергаться: %v", err)
	}
}

// TestValidateProductionRejectsWeakSessionSecret — в production пустой секрет
// и известный дефолт change-me фатальны.
func TestValidateProductionRejectsWeakSessionSecret(t *testing.T) {
	for name, secret := range map[string]string{
		"пустой":      "",
		"дефолт":      defaultSessionSecret,
		"дефолт CAPS": "CHANGE-ME",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := productionConfig()
			cfg.SessionSecret = secret
			err := cfg.validate()
			if err == nil {
				t.Fatal("production с небезопасным SESSION_SECRET не должен проходить валидацию")
			}
			if !strings.Contains(err.Error(), "SESSION_SECRET") {
				t.Fatalf("ошибка не упоминает SESSION_SECRET: %v", err)
			}
		})
	}
}

// TestValidateProductionRejectsMockWithoutWebhookSecret — mock-провайдер в
// production запрещён независимо от MOCK_WEBHOOK_SECRET.
func TestValidateProductionRejectsMockWithoutWebhookSecret(t *testing.T) {
	for name, secret := range map[string]string{
		"пусто":   "",
		"пробелы": "   ",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := productionConfig()
			cfg.PaymentsProvider = "mock"
			cfg.MockWebhookSecret = secret
			err := cfg.validate()
			if err == nil {
				t.Fatal("mock в production не должен проходить валидацию")
			}
			if !strings.Contains(err.Error(), "PAYMENTS_PROVIDER=mock") {
				t.Fatalf("ошибка не упоминает PAYMENTS_PROVIDER=mock: %v", err)
			}
		})
	}
}

// TestValidateProductionRejectsMockEvenWithWebhookSecret — mock-страница не
// умеет подписывать вебхук, поэтому mock запрещён в production при любом
// MOCK_WEBHOOK_SECRET; для закрытого staging используйте APP_ENV=development.
func TestValidateProductionRejectsMockEvenWithWebhookSecret(t *testing.T) {
	cfg := productionConfig()
	cfg.PaymentsProvider = "mock"
	cfg.MockWebhookSecret = "whsec-случайный"
	err := cfg.validate()
	if err == nil {
		t.Fatal("mock в production должен быть запрещён даже с MOCK_WEBHOOK_SECRET")
	}
	if !strings.Contains(err.Error(), "PAYMENTS_PROVIDER=mock") {
		t.Fatalf("ошибка не упоминает PAYMENTS_PROVIDER=mock: %v", err)
	}
}

// TestValidateProductionRejectsUnknownProvider — опечатка в
// PAYMENTS_PROVIDER должна валить старт, а не каждую покупку с 400/502.
func TestValidateProductionRejectsUnknownProvider(t *testing.T) {
	cfg := productionConfig()
	cfg.PaymentsProvider = "stripe-typo"
	err := cfg.validate()
	if err == nil {
		t.Fatal("неизвестный провайдер в production не должен проходить валидацию")
	}
	if !strings.Contains(err.Error(), "PAYMENTS_PROVIDER") {
		t.Fatalf("ошибка не упоминает PAYMENTS_PROVIDER: %v", err)
	}
}

// TestValidateRejectsSameAdminAndTeacherEmail — одна учётка не должна быть
// одновременно bootstrap-админом и преподавателем: второй вызов сменил бы ей
// роль и/или пароль.
func TestValidateRejectsSameAdminAndTeacherEmail(t *testing.T) {
	cfg := productionConfig()
	cfg.AdminEmail = " same@example.test "
	cfg.TeacherEmail = "SAME@example.test"
	if err := cfg.validate(); err == nil {
		t.Fatal("совпадающие ADMIN_EMAIL/TEACHER_EMAIL не должны проходить валидацию")
	}
}

// TestValidateProductionRejectsSMTPNone — незашифрованный SMTP и опечатки в
// режиме TLS в production запрещены.
func TestValidateProductionRejectsSMTPNone(t *testing.T) {
	for name, mode := range map[string]string{
		"none":     "NONE",
		"опечатка": "startls",
		"пусто":    "",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := productionConfig()
			cfg.SMTPTLS = mode
			err := cfg.validate()
			if err == nil {
				t.Fatal("некорректный SMTP_TLS в production не должен проходить валидацию")
			}
			if !strings.Contains(err.Error(), "SMTP_TLS") {
				t.Fatalf("ошибка не упоминает SMTP_TLS: %v", err)
			}
		})
	}
}

// TestValidateProductionRejectsSMTPWithoutFrom — при заданном SMTP_ADDR
// отправитель обязателен, иначе письма молча отключатся.
func TestValidateProductionRejectsSMTPWithoutFrom(t *testing.T) {
	cfg := productionConfig()
	cfg.SMTPAddr = "smtp.example.test:587"
	cfg.SMTPFrom = ""
	if err := cfg.validate(); err == nil {
		t.Fatal("SMTP_ADDR без SMTP_FROM не должен проходить валидацию")
	}
}

// TestValidateProductionAcceptsSecureConfig — корректный production-конфиг
// проходит без ошибок.
func TestValidateProductionAcceptsSecureConfig(t *testing.T) {
	cfg := productionConfig()
	if err := cfg.validate(); err != nil {
		t.Fatalf("безопасный production-конфиг отвергнут: %v", err)
	}
}

// TestValidateInfersProductionFromDatabaseURL — без APP_ENV наличие
// DATABASE_URL означает production: дефолтный секрет там тоже запрещён.
func TestValidateInfersProductionFromDatabaseURL(t *testing.T) {
	cfg := &Config{
		DatabaseURL:      "postgres://edu:edu@db:5432/edu?sslmode=disable",
		SessionSecret:    defaultSessionSecret,
		PaymentsProvider: "yookassa",
		SMTPTLS:          "starttls",
	}
	if cfg.IsDev() {
		t.Fatal("конфиг с DATABASE_URL без APP_ENV должен считаться production")
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("дефолтный SESSION_SECRET при неявном production должен отвергаться")
	}
}

// TestLoadFromEnvReadsBootstrapAccounts — TEACHER_* и MOCK_WEBHOOK_SECRET
// читаются из окружения (без log.Fatal), email тримится, пароли — как есть.
func TestLoadFromEnvReadsBootstrapAccounts(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("ADMIN_EMAIL", " admin@example.test ")
	t.Setenv("ADMIN_PASSWORD", "admin-pass")
	t.Setenv("TEACHER_EMAIL", " teacher@example.test ")
	t.Setenv("TEACHER_PASSWORD", "teacher-pass ")
	t.Setenv("MOCK_WEBHOOK_SECRET", " whsec-test ")

	cfg := loadFromEnv()
	if cfg.AdminEmail != "admin@example.test" || cfg.AdminPassword != "admin-pass" {
		t.Fatalf("ADMIN_* прочитаны неверно: %q/%q", cfg.AdminEmail, cfg.AdminPassword)
	}
	if cfg.TeacherEmail != "teacher@example.test" || cfg.TeacherPassword != "teacher-pass " {
		t.Fatalf("TEACHER_* прочитаны неверно: %q/%q", cfg.TeacherEmail, cfg.TeacherPassword)
	}
	if cfg.MockWebhookSecret != "whsec-test" {
		t.Fatalf("MOCK_WEBHOOK_SECRET=%q, ждали whsec-test", cfg.MockWebhookSecret)
	}
}

// TestLoadFromEnvDefaults — dev-дефолты не меняются: mock-провайдер и starttls.
func TestLoadFromEnvDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("PAYMENTS_PROVIDER", "")
	t.Setenv("SMTP_TLS", "")

	cfg := loadFromEnv()
	if cfg.PaymentsProvider != "mock" {
		t.Fatalf("PAYMENTS_PROVIDER по умолчанию=%q, ждали mock", cfg.PaymentsProvider)
	}
	if cfg.SMTPTLS != "starttls" {
		t.Fatalf("SMTP_TLS по умолчанию=%q, ждали starttls", cfg.SMTPTLS)
	}
	if !cfg.IsDev() {
		t.Fatal("APP_ENV=development должен включать dev-режим")
	}
}
