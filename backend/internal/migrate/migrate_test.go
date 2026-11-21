package migrate

// Unit-тесты раннера миграций без Postgres: ApplyMigrations обязан пропускать
// seed-файлы (демо-данные только dev), а FindMigrationsDir — находить каталог
// в списке кандидатов.

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeSQL(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("запись %s: %v", name, err)
	}
}

// TestMigrationFilesSkipsSeedFiles — в список схемных миграций не попадают
// 002_seed.sql/004_seed_fix.sql, не-SQL файлы и подкаталоги; порядок — по имени.
func TestMigrationFilesSkipsSeedFiles(t *testing.T) {
	dir := t.TempDir()
	writeSQL(t, dir, "015_analytics_indexes.sql", "CREATE INDEX IF NOT EXISTS x ON y(z);")
	writeSQL(t, dir, "001_init.sql", "CREATE TABLE x();")
	writeSQL(t, dir, "002_seed.sql", "INSERT INTO x VALUES (1);")
	writeSQL(t, dir, "004_seed_fix.sql", "UPDATE x SET y=1;")
	writeSQL(t, dir, "README.md", "not a migration")
	if err := os.Mkdir(filepath.Join(dir, "010_nested"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got, err := migrationFiles(dir)
	if err != nil {
		t.Fatalf("migrationFiles: %v", err)
	}
	want := []string{
		filepath.Join(dir, "001_init.sql"),
		filepath.Join(dir, "015_analytics_indexes.sql"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("migrationFiles=%v, ждали %v (seed-файлы должны быть пропущены)", got, want)
	}
}

// TestMigrationFilesOnlySeeds — каталог без схемных миграций (только сиды)
// считается ошибкой, а не «нечего применять».
func TestMigrationFilesOnlySeeds(t *testing.T) {
	dir := t.TempDir()
	writeSQL(t, dir, "002_seed.sql", "INSERT INTO x VALUES (1);")
	writeSQL(t, dir, "004_seed_fix.sql", "UPDATE x SET y=1;")

	if _, err := migrationFiles(dir); err == nil {
		t.Fatal("ожидали ошибку: схемных миграций нет")
	}
}

// TestIsSeedFile — фильтр по подстроке seed без учёта регистра.
func TestIsSeedFile(t *testing.T) {
	for name, want := range map[string]bool{
		"002_seed.sql":              true,
		"004_seed_fix.sql":          true,
		"005_price_fix.sql":         true,
		"SEED_data.SQL":             true,
		"seed.sql":                  true,
		"001_init.sql":              false,
		"003_app_compat.sql":        false,
		"015_analytics_indexes.sql": false,
	} {
		if got := isSeedFile(name); got != want {
			t.Errorf("isSeedFile(%q)=%v, ждали %v", name, got, want)
		}
	}
}

// TestFindMigrationsDir — возвращает первый существующий каталог и ошибка,
// если ни одного нет.
func TestFindMigrationsDir(t *testing.T) {
	dir := t.TempDir()
	got, err := FindMigrationsDir("", filepath.Join(dir, "missing"), dir)
	if err != nil {
		t.Fatalf("FindMigrationsDir: %v", err)
	}
	if got != dir {
		t.Fatalf("FindMigrationsDir=%q, ждали %q", got, dir)
	}

	if _, err := FindMigrationsDir("", filepath.Join(dir, "missing")); err == nil {
		t.Fatal("ожидали ошибку, когда ни один каталог не существует")
	}
}
