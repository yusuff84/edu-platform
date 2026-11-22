// Package migrate — простой runner SQL-миграций для Postgres.
//
// Применяет новые схемные файлы по порядку имён, записывает их контрольные
// суммы и сериализует старты PostgreSQL advisory lock. Схема и журнал
// миграций изменяются одной транзакцией; изменённый применённый файл — ошибка.
package migrate

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SeedFileName — демо-сид. ApplyMigrations его не применяет (файлы с seed в
// имени всегда пропускаются); в dev cmd/api вызывает SeedIfEmpty отдельно,
// в production демо-данные не создаются.
const SeedFileName = "002_seed.sql"

// ApplyMigrations выполняет только ещё не применённые схемные файлы.
// Демо-сиды и историческая правка цены демо-курса в production не выполняются.
func ApplyMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	files, err := migrationFiles(dir)
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(753251624773)`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		name text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	for _, f := range files {
		sql, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		// Normalize Git's platform-specific line endings before checksumming.
		checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ReplaceAll(string(sql), "\r\n", "\n"))))
		name := filepath.Base(f)
		var existing string
		if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT checksum FROM schema_migrations WHERE name=$1),'')`, name).Scan(&existing); err != nil {
			return err
		}
		if existing != "" {
			if existing != checksum {
				return fmt.Errorf("миграция %s изменена после применения; создайте новую миграцию", name)
			}
			continue
		}
		if strings.TrimSpace(string(sql)) != "" {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return fmt.Errorf("применение миграции %s: %w", name, err)
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(name,checksum) VALUES($1,$2)`, name, checksum); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// migrationFiles возвращает отсортированные пути схемных миграций, отбрасывая
// seed-файлы и не-SQL файлы. Вынесено из ApplyMigrations для unit-тестов без
// живого Postgres.
func migrationFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("чтение каталога миграций %s: %w", dir, err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".sql") {
			continue
		}
		if isSeedFile(name) {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("в каталоге %s нет схемных *.sql миграций", dir)
	}
	return files, nil
}

// isSeedFile сообщает, является ли SQL-файл демо-сидом (в имени есть "seed",
// регистр не важен).
func isSeedFile(name string) bool {
	base := strings.ToLower(filepath.Base(name))
	return strings.Contains(base, "seed") || base == "005_price_fix.sql"
}

// SeedIfEmpty применяет seedFile, только если таблица courses пуста.
// Вызывается cmd/api ТОЛЬКО в dev-режиме: в production демо-данные запрещены.
// Файл должен быть идемпотентным (ON CONFLICT DO NOTHING).
// Если таблица отсутствует (миграции не применены) — возвращает ошибку.
func SeedIfEmpty(ctx context.Context, pool *pgxpool.Pool, seedFile string) error {
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM courses`).Scan(&count); err != nil {
		return fmt.Errorf("проверка таблицы courses: %w", err)
	}
	if count > 0 {
		return nil
	}
	sql, err := os.ReadFile(seedFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("чтение сида %s: %w", seedFile, err)
	}
	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		return fmt.Errorf("применение сида %s: %w", filepath.Base(seedFile), err)
	}
	return nil
}

// GetCommissionPct читает commission_pct из app_settings (ключ commission_pct).
// Возвращает fallback, если ключа нет, значение не парсится или вне 0..90.
func GetCommissionPct(ctx context.Context, pool *pgxpool.Pool, fallback int) int {
	var raw string
	if err := pool.QueryRow(ctx, `SELECT value FROM app_settings WHERE key='commission_pct'`).Scan(&raw); err != nil {
		return fallback
	}
	raw = strings.TrimSpace(raw)
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 || n > 90 {
		return fallback
	}
	return n
}

// FindMigrationsDir возвращает первый существующий каталог из кандидатов.
func FindMigrationsDir(candidates ...string) (string, error) {
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("каталог миграций не найден (проверены: %s)", strings.Join(candidates, ", "))
}
