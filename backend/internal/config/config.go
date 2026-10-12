// Package config читает настройки backend из переменных окружения.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config — настройки API.
type Config struct {
	DatabaseURL      string
	SessionSecret    string
	PaymentsProvider string // mock | stripe | yookassa
	CommissionPct    int    // комиссия платформы 0..90, default 15
	Port             string
	RunnerURL        string // базовый URL code-runner, например http://runner:8081
	AllowedOrigins   []string
	AdminEmail       string // опциональный сид админа (создаётся только если задан вместе с паролем)
	AdminPassword    string
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
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("PAYMENTS_PROVIDER")))
	if provider == "" {
		provider = "mock"
	}
	runner := strings.TrimSpace(os.Getenv("RUNNER_URL"))
	if runner == "" {
		runner = "http://runner:8081"
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
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		SessionSecret:    os.Getenv("SESSION_SECRET"),
		PaymentsProvider: provider,
		CommissionPct:    commission,
		Port:             port,
		RunnerURL:        runner,
		AllowedOrigins:   origins,
		AdminEmail:       strings.TrimSpace(os.Getenv("ADMIN_EMAIL")),
		AdminPassword:    os.Getenv("ADMIN_PASSWORD"),
	}
}
