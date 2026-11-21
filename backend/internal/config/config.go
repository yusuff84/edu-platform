// Package config читает настройки backend из переменных окружения.
package config

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// defaultSessionSecret — дефолт из .env.example/compose: известен публично,
// поэтому подпись cookie с ним предсказуема. В dev предупреждаем, в
// production validate() запрещает запуск.
const defaultSessionSecret = "change-me"

// Config — настройки API.
type Config struct {
	DatabaseURL       string
	SessionSecret     string
	AppEnv            string // APP_ENV: development | production (пусто — дедукция по DATABASE_URL)
	PaymentsProvider  string // mock | stripe | yookassa | disabled
	CommissionPct     int    // комиссия платформы 0..90, default 15
	Port              string
	RunnerURL         string // базовый URL code-runner, например http://runner:8081
	AllowedOrigins    []string
	TrustedProxyCIDRs []string
	AdminEmail        string // опциональный bootstrap админа (создаётся только если задан вместе с паролем)
	AdminPassword     string
	TeacherEmail      string // опциональный bootstrap преподавателя (только вместе с паролем)
	TeacherPassword   string
	MockWebhookSecret string // MOCK_WEBHOOK_SECRET: только dev; production с mock запрещён
	PublicAppURL      string // PUBLIC_APP_URL: база ссылок в письмах, default http://localhost:3000

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

// Load собирает Config из окружения и завершает процесс при небезопасной
// конфигурации (production без секретов, mock-оплата, SMTP без TLS и т.п.).
func Load() *Config {
	cfg := loadFromEnv()
	if err := cfg.validate(); err != nil {
		log.Fatalf("некорректная конфигурация: %v", err)
	}
	return cfg
}

// loadFromEnv только читает переменные окружения и не завершает процесс —
// так конфигурацию можно тестировать без log.Fatal.
func loadFromEnv() *Config {
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
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("PAYMENTS_PROVIDER")))
	if provider == "" {
		provider = "mock"
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

	return &Config{
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		SessionSecret:     strings.TrimSpace(os.Getenv("SESSION_SECRET")),
		AppEnv:            strings.TrimSpace(os.Getenv("APP_ENV")),
		PaymentsProvider:  provider,
		CommissionPct:     commission,
		Port:              port,
		RunnerURL:         runner,
		AllowedOrigins:    origins,
		TrustedProxyCIDRs: strings.FieldsFunc(os.Getenv("TRUSTED_PROXY_CIDRS"), func(r rune) bool { return r == ',' || r == ' ' }),
		AdminEmail:        strings.TrimSpace(os.Getenv("ADMIN_EMAIL")),
		AdminPassword:     os.Getenv("ADMIN_PASSWORD"),
		TeacherEmail:      strings.TrimSpace(os.Getenv("TEACHER_EMAIL")),
		TeacherPassword:   os.Getenv("TEACHER_PASSWORD"),
		MockWebhookSecret: strings.TrimSpace(os.Getenv("MOCK_WEBHOOK_SECRET")),
		PublicAppURL:      publicAppURL,

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
}

// validate проверяет инварианты production. В dev дефолты разрешены (с
// предупреждениями), в production возвращённая ошибка превращается Load в
// log.Fatalf — сервис не стартует с предсказуемыми секретами/mock-оплатой.
func (c *Config) validate() error {
	for _, entry := range c.TrustedProxyCIDRs {
		if net.ParseIP(entry) == nil {
			if _, _, err := net.ParseCIDR(entry); err != nil {
				return fmt.Errorf("TRUSTED_PROXY_CIDRS содержит некорректный IP/CIDR: %s", entry)
			}
		}
	}
	adminEmail := strings.TrimSpace(c.AdminEmail)
	teacherEmail := strings.TrimSpace(c.TeacherEmail)
	if adminEmail != "" && teacherEmail != "" && strings.EqualFold(adminEmail, teacherEmail) {
		return fmt.Errorf("ADMIN_EMAIL и TEACHER_EMAIL не должны совпадать: одна учётка не может быть одновременно админом и преподавателем")
	}

	secret := strings.TrimSpace(c.SessionSecret)
	if c.IsDev() {
		switch {
		case secret == "":
			log.Printf("ПРЕДУПРЕЖДЕНИЕ: SESSION_SECRET не задан — cookie сессии не подписываются (dev-режим). Задайте секрет перед прод-запуском")
		case strings.EqualFold(secret, defaultSessionSecret):
			// Compose из .env.example поднимается с дефолтом: работаем, но громко предупреждаем.
			log.Printf("ПРЕДУПРЕЖДЕНИЕ: SESSION_SECRET остался дефолтным %q — подпись cookie предсказуема; задайте свой секрет", defaultSessionSecret)
		}
		return nil
	}

	if secret == "" {
		return fmt.Errorf("SESSION_SECRET не задан, а режим продовый (APP_ENV=%q): запуск без секрета запрещён; задайте SESSION_SECRET или APP_ENV=development", c.AppEnv)
	}
	if strings.EqualFold(secret, defaultSessionSecret) {
		return fmt.Errorf("SESSION_SECRET остался дефолтным %q: в production задайте уникальный случайный секрет", defaultSessionSecret)
	}
	provider := strings.ToLower(strings.TrimSpace(c.PaymentsProvider))
	if provider == "mock" {
		// Mock-страница подтверждения не умеет подписывать вебхук секретом,
		// который нельзя отдавать браузеру. Поэтому mock — только dev-режим,
		// даже если MOCK_WEBHOOK_SECRET задан: production обязан использовать
		// реального провайдера.
		return fmt.Errorf("PAYMENTS_PROVIDER=mock запрещён в production: используйте stripe/yookassa, а mock оставьте для APP_ENV=development")
	}
	if provider != "stripe" && provider != "yookassa" && provider != "disabled" {
		// Иначе сервис стартовал бы и падал только на первой покупке;
		// ошибка конфигурации должна обнаруживаться при запуске.
		return fmt.Errorf("PAYMENTS_PROVIDER=%q неизвестен: допустимы stripe, yookassa или disabled", c.PaymentsProvider)
	}
	smtpTLS := strings.ToLower(strings.TrimSpace(c.SMTPTLS))
	if smtpTLS == "none" {
		return fmt.Errorf("SMTP_TLS=none запрещён в production: используйте starttls или implicit")
	}
	if smtpTLS != "starttls" && smtpTLS != "implicit" {
		return fmt.Errorf("SMTP_TLS=%q неизвестен: допустимы starttls или implicit", c.SMTPTLS)
	}
	if strings.TrimSpace(c.SMTPAddr) != "" && strings.TrimSpace(c.SMTPFrom) == "" {
		return fmt.Errorf("SMTP_ADDR задан, но SMTP_FROM пуст: в production письма не должны отключаться молча")
	}
	if strings.TrimSpace(c.SMTPAddr) != "" {
		appURL, err := url.Parse(c.PublicAppURL)
		if err != nil || appURL.Scheme != "https" || appURL.Host == "" || appURL.User != nil || appURL.RawQuery != "" || appURL.Fragment != "" {
			return fmt.Errorf("PUBLIC_APP_URL должен быть HTTPS URL приложения без query/fragment: он используется в письмах аккаунта")
		}
	}
	return nil
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
