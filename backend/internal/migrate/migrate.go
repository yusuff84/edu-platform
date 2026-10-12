// Package migrate — простой runner SQL-миграций для Postgres.
//
// Читает *.sql файлы из каталога migrations по порядку имён и выполняет
// каждый файл целиком через pgxpool. Без внешних зависимостей (golang-migrate
// не используется ради простоты MVP). Файлы идемпотентны (IF NOT EXISTS /
// ON CONFLICT DO NOTHING), поэтому повторный запуск безопасен.
package migrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ApplyMigrations выполняет все *.sql файлы из dir по возрастанию имён.
func ApplyMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("чтение каталога миграций %s: %w", dir, err)
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
		files = append(files, filepath.Join(dir, name))
	}
	sort.Strings(files)
	if len(files) == 0 {
		return fmt.Errorf("в каталоге %s нет *.sql миграций", dir)
	}
	for _, f := range files {
		if err := applyFile(ctx, pool, f); err != nil {
			return err
		}
	}
	return nil
}

func applyFile(ctx context.Context, pool *pgxpool.Pool, path string) error {
	sql, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("чтение миграции %s: %w", path, err)
	}
	if strings.TrimSpace(string(sql)) == "" {
		return nil
	}
	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		return fmt.Errorf("применение миграции %s: %w", filepath.Base(path), err)
	}
	return nil
}

// SeedIfEmpty применяет seedFile, только если таблица courses пуста.
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
