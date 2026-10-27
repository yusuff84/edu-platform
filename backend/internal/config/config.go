// Package config читает настройки backend из переменных окружения.
package config

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// defaultSessionSecret — дефолт из .env.example/compose: известен публично,
// поэтому подпись cookie с ним предсказуема (предупреждаем в Load).
const defaultSessionSecret = "change-me"

// Config — настройки API.
type Config struct {
	DatabaseURL      string
	SessionSecret    string
	AppEnv           string // APP_ENV: development | production (пусто — дедукция по DATABASE_URL)
	PaymentsProvider string // mock | stripe | yookassa
	CommissionPct    int    // комиссия платформы 0..90, default 15
	Port             string
	RunnerURL        string // базовый URL code-runner, например http://runner:8081
	AllowedOrigins   []string
	AdminEmail       string // опциональный сид админа (создаётся только если задан вместе с паролем)
	AdminPassword    string
	PublicAppURL     string // PUBLIC_APP_URL: база ссылок в письмах, default http://localhost:3000

	// ---- SMTP для приглашений (опционально) ----
	SMTPAddr     string // SMTP_ADDR: host:port; пусто — письма выключены
	SMTPFrom     string // SMTP_FROM: адрес отправителя
	SMTPUsername string // SMTP_USERNAME
	SMTPPassword string // SMTP_PASSWORD
	SMTPTLS      string // SMTP_TLS: starttls | implicit | none

	// ---- Безопасность аутентификации (T10) ----

	// CookieSameSite — SameSite cookie сессии: COOKIE_SAMESITE=lax|strict|none.
	// Пустое значение в Config (тесты) означает lax.
	CookieSameSite http.SameSite
	// CSRFAllowMissingOrigin — разрешить не-GET запросы без Origin/Referer
	// (CSRF_ALLOW_MISSING_ORIGIN=true). Нужно локальным qa-скриптам в prod-режиме.
	CSRFAllowMissingOrigin bool

	// Rate-limit запросов в минуту на IP; <=0 — лимит выключен.
	// Load выставляет дефолты: auth 10, global 300, webhook 120.
	RateLimitAuthPerMin    int // RATE_LIMIT_AUTH_PER_MIN: /auth/login, /auth/register
	RateLimitGlobalPerMin  int // RATE_LIMIT_GLOBAL_PER_MIN: всё /api/v1
	RateLimitWebhookPerMin int // RATE_LIMIT_WEBHOOK_PER_MIN: /payments/webhook/{provider}

	// ---- Яндекс OAuth (T10) ----

	YandexClientID     string // YANDEX_CLIENT_ID
	YandexClientSecret string // YANDEX_CLIENT_SECRET
	YandexRedirectURI  string // YANDEX_REDIRECT_URI
}

// IsDev определяет dev-режим: явный APP_ENV=development|dev|local|test
// либо отсутствие DATABASE_URL (запуск без БД = локальная разработка).
func (c *Config) IsDev() bool {
	switch strings.ToLower(strings.TrimSpace(c.AppEnv)) {
	case "development", "dev", "local", "test":
		return true
	case "production", "prod":
		return false
	}
	return strings.TrimSpace(c.DatabaseURL) == ""
}

// Load собирает Config из окружения с безопасными значениями по умолчанию.
func Load() *Config {
	commission := 15
	if v := strings.TrimSpace(os.Getenv("COMMISSION_PCT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 90 {
			commission = n
		}
	}
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8080"
	}
	env := strings.TrimSpace(os.Getenv("APP_ENV"))
	databaseURL := os.Getenv("DATABASE_URL")
	dev := (&Config{AppEnv: env, DatabaseURL: databaseURL}).IsDev()
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("PAYMENTS_PROVIDER")))
	if provider == "" {
		if dev {
			provider = "mock"
		} else {
			// P11: в prod пустой PAYMENTS_PROVIDER не должен молча
			// превращаться в mock (иначе тестовый платёж может стать
			// боевым). Безопасный явный дефолт — реальный провайдер:
			// без ключей checkout вернёт 502, а не создаст mock-платёж.
			provider = "stripe"
			log.Printf("ПРЕДУПРЕЖДЕНИЕ: PAYMENTS_PROVIDER не задан в prod-режиме — используем stripe; mock вне dev запрещён. Задайте провайдера явно")
		}
	}
	if provider == "mock" && !dev {
		// Явный mock в prod не фатален (конфиг может быть общим для стендов),
		// но handler отклонит mock-платежи: checkout/webhook вернут 502/404.
		log.Printf("ПРЕДУПРЕЖДЕНИЕ: PAYMENTS_PROVIDER=mock в prod-режиме — mock-платежи отключены; задайте stripe или yookassa")
	}
	runner := strings.TrimSpace(os.Getenv("RUNNER_URL"))
	if runner == "" {
		runner = "http://runner:8081"
	}
	publicAppURL := strings.TrimSpace(os.Getenv("PUBLIC_APP_URL"))
	if publicAppURL == "" {
		publicAppURL = "http://localhost:3000"
	}
	smtpTLS := strings.ToLower(strings.TrimSpace(os.Getenv("SMTP_TLS")))
	if smtpTLS == "" {
		smtpTLS = "starttls"
	}
	origins := []string{"http://localhost:3000", "http://127.0.0.1:3000"}
	if v := strings.TrimSpace(os.Getenv("ALLOWED_ORIGINS")); v != "" {
		parts := strings.Split(v, ",")
		origins = origins[:0]
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				origins = append(origins, p)
			}
		}
	}

	secret := strings.TrimSpace(os.Getenv("SESSION_SECRET"))
	switch {
	case secret == "" && dev:
		log.Printf("ПРЕДУПРЕЖДЕНИЕ: SESSION_SECRET не задан — cookie сессии не подписываются (dev-режим). Задайте секрет перед прод-запуском")
	case secret == "" && !dev:
		// Прод без секрета: подписывать нечем, подделка cookie становится тривиальной — не стартуем.
		log.Fatalf("SESSION_SECRET не задан, а режим продовый (APP_ENV=%q, DATABASE_URL задан): запуск без секрета запрещён. Задайте SESSION_SECRET или APP_ENV=development", env)
	case secret == defaultSessionSecret:
		// Комpose из .env.example поднимается с дефолтом: работаем, но громко предупреждаем.
		log.Printf("ПРЕДУПРЕЖДЕНИЕ: SESSION_SECRET остался дефолтным %q — подпись cookie предсказуема; задайте свой секрет", defaultSessionSecret)
	}

	cfg := &Config{
		DatabaseURL:      databaseURL,
		SessionSecret:    secret,
		AppEnv:           env,
		PaymentsProvider: provider,
		CommissionPct:    commission,
		Port:             port,
		RunnerURL:        runner,
		AllowedOrigins:   origins,
		AdminEmail:       strings.TrimSpace(os.Getenv("ADMIN_EMAIL")),
		AdminPassword:    os.Getenv("ADMIN_PASSWORD"),
		PublicAppURL:     publicAppURL,

		SMTPAddr:     strings.TrimSpace(os.Getenv("SMTP_ADDR")),
		SMTPFrom:     strings.TrimSpace(os.Getenv("SMTP_FROM")),
		SMTPUsername: strings.TrimSpace(os.Getenv("SMTP_USERNAME")),
		SMTPPassword: os.Getenv("SMTP_PASSWORD"),
		SMTPTLS:      smtpTLS,

		CookieSameSite:         parseSameSite(os.Getenv("COOKIE_SAMESITE")),
		CSRFAllowMissingOrigin: parseBool(os.Getenv("CSRF_ALLOW_MISSING_ORIGIN")),

		RateLimitAuthPerMin:    parseRateLimit("RATE_LIMIT_AUTH_PER_MIN", 10),
		RateLimitGlobalPerMin:  parseRateLimit("RATE_LIMIT_GLOBAL_PER_MIN", 300),
		RateLimitWebhookPerMin: parseRateLimit("RATE_LIMIT_WEBHOOK_PER_MIN", 120),

		YandexClientID:     strings.TrimSpace(os.Getenv("YANDEX_CLIENT_ID")),
		YandexClientSecret: strings.TrimSpace(os.Getenv("YANDEX_CLIENT_SECRET")),
		YandexRedirectURI:  strings.TrimSpace(os.Getenv("YANDEX_REDIRECT_URI")),
	}
	return cfg
}

// parseSameSite переводит COOKIE_SAMESITE=lax|strict|none в http.SameSite.
func parseSameSite(v string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		// Браузеры требуют Secure при SameSite=none — handler всегда ставит флаг.
		return http.SameSiteNoneMode
	case "":
		return http.SameSiteLaxMode
	default:
		log.Printf("ПРЕДУПРЕЖДЕНИЕ: COOKIE_SAMESITE=%q не распознан, используем lax", v)
		return http.SameSiteLaxMode
	}
}

// parseBool понимает "true"/"1"/"yes" (регистронезависимо).
func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on":
		return true
	}
	return false
}

// parseRateLimit читает лимит из env: невалидное значение → дефолт,
// явный 0 или отрицательное — выключает лимитер.
func parseRateLimit(env string, def int) int {
	v := strings.TrimSpace(os.Getenv(env))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("ПРЕДУПРЕЖДЕНИЕ: %s=%q не число, используем дефолт %d", env, v, def)
		return def
	}
	return n
}
