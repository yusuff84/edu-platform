package main

// Регресс-тесты T16 (п.7): seedAdmin/ensureAccount чинят демо-учётки с
// ПОДЕЛЬНЫМИ bcrypt-хешами из миграции 002_seed.sql. Это уже однажды тихо
// сломалось (админ из миграции не мог войти, qa-abc падал на блоке B),
// поэтому поведение закреплено тестами: поддельный или несовпадающий хеш
// перезаписывается настоящим, корректный хеш не трогается, отсутствующий
// пользователь создаётся.

import (
	"testing"

	"golang.org/x/crypto/bcrypt"

	"edu-platform/backend/internal/config"
	"edu-platform/backend/internal/repo"
)

// fakeMigrationHash — точная заглушка из 002_seed.sql: 50 символов «A»
// плюс открытый текст пароля. bcrypt.ParseHash на ней падает.
const fakeMigrationHash = "$2a$10$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAadmin1234"

// newSeedConfig собирает конфиг для seedAdmin, изолируя env учителя.
func newSeedConfig(t *testing.T, email, password string) *config.Config {
	t.Helper()
	// Учителя в тестах не сидируем: env явно указывает на несуществующие
	// значения, чтобы ensureAccount для teacher не мешал проверкам админа.
	t.Setenv("TEACHER_EMAIL", "teacher-test-seed@example.test")
	t.Setenv("TEACHER_PASSWORD", "teacher-test-seed-1")
	return &config.Config{AdminEmail: email, AdminPassword: password}
}

// mustCompare сверяет хеш с паролем и падает при расхождении.
func mustCompare(t *testing.T, hash, password string) {
	t.Helper()
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		t.Fatalf("хеш не соответствует паролю %q: %v", password, err)
	}
}

// TestSeedAdminCreatesMissingUser — пользователя нет → создаётся с настоящим
// bcrypt и ролью admin; пароль реально входит.
func TestSeedAdminCreatesMissingUser(t *testing.T) {
	s := repo.NewStore()
	seedAdmin(s, newSeedConfig(t, "seed-create@example.test", "пароль-админа-1"))

	u, ok := s.GetUserByEmail("seed-create@example.test")
	if !ok {
		t.Fatal("админ не создан")
	}
	if u.Role != repo.RoleAdmin {
		t.Fatalf("роль=%v, ждали admin", u.Role)
	}
	mustCompare(t, u.PasswordHash, "пароль-админа-1")
}

// TestSeedAdminRepairsFakeHash — пользователь ЕСТЬ с поддельным хешем
// из миграции (баг 002_seed.sql) → хеш перезаписывается настоящим.
// Ровно этот сценарий блокировал сквозной прогон до T16.
func TestSeedAdminRepairsFakeHash(t *testing.T) {
	s := repo.NewStore()
	if _, err := s.CreateUser("admin@local", fakeMigrationHash, repo.RoleAdmin); err != nil {
		t.Fatalf("создание сид-пользователя: %v", err)
	}

	seedAdmin(s, newSeedConfig(t, "admin@local", "admin1234"))

	u, ok := s.GetUserByEmail("admin@local")
	if !ok {
		t.Fatal("пользователь пропал")
	}
	if u.PasswordHash == fakeMigrationHash {
		t.Fatal("поддельный хеш не был перезаписан — вход по-прежнему невозможен")
	}
	mustCompare(t, u.PasswordHash, "admin1234")
}

// TestSeedAdminRepairsWrongPasswordHash — хеш валидный, но от ДРУГОГО пароля:
// ADMIN_PASSWORD задан и не совпадает → хеш перезаписывается (семантика:
// ADMIN_* — источник истины при старте).
func TestSeedAdminRepairsWrongPasswordHash(t *testing.T) {
	s := repo.NewStore()
	otherHash, err := bcrypt.GenerateFromPassword([]byte("старый-пароль"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("хеш: %v", err)
	}
	if _, err := s.CreateUser("admin-wrong@example.test", string(otherHash), repo.RoleAdmin); err != nil {
		t.Fatalf("создание: %v", err)
	}

	seedAdmin(s, newSeedConfig(t, "admin-wrong@example.test", "новый-пароль"))

	u, _ := s.GetUserByEmail("admin-wrong@example.test")
	mustCompare(t, u.PasswordHash, "новый-пароль")
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("старый-пароль")); err == nil {
		t.Fatal("старый пароль не должен больше подходить")
	}
}

// TestSeedAdminKeepsValidHash — хеш уже корректен → НЕ перезаписывается
// (иначе каждый рестарт менял бы хеш и ломал параллельные сессии).
func TestSeedAdminKeepsValidHash(t *testing.T) {
	s := repo.NewStore()
	goodHash, err := bcrypt.GenerateFromPassword([]byte("верный-пароль"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("хеш: %v", err)
	}
	if _, err := s.CreateUser("admin-valid@example.test", string(goodHash), repo.RoleAdmin); err != nil {
		t.Fatalf("создание: %v", err)
	}

	seedAdmin(s, newSeedConfig(t, "admin-valid@example.test", "верный-пароль"))

	u, _ := s.GetUserByEmail("admin-valid@example.test")
	if u.PasswordHash != string(goodHash) {
		t.Fatal("корректный хеш не должен перезаписываться")
	}
}

// TestSeedAdminFixesRole — роль приводится к требуемой даже при корректном
// хеше (миграция могла создать пользователя с другой ролью).
func TestSeedAdminFixesRole(t *testing.T) {
	s := repo.NewStore()
	goodHash, _ := bcrypt.GenerateFromPassword([]byte("п-1"), bcrypt.MinCost)
	if _, err := s.CreateUser("admin-role@example.test", string(goodHash), repo.RoleStudent); err != nil {
		t.Fatalf("создание: %v", err)
	}

	seedAdmin(s, newSeedConfig(t, "admin-role@example.test", "п-1"))

	u, _ := s.GetUserByEmail("admin-role@example.test")
	if u.Role != repo.RoleAdmin {
		t.Fatalf("роль=%v, ждали admin", u.Role)
	}
}

// TestSeedAdminNoConfigNoAdmin — без ADMIN_EMAIL/ADMIN_PASSWORD админ не
// сидируется (секретов в коде нет); при этом вызов не падает и трогает
// только учётку учителя.
func TestSeedAdminNoConfigNoAdmin(t *testing.T) {
	s := repo.NewStore()
	seedAdmin(s, newSeedConfig(t, "", ""))

	if _, ok := s.GetUserByEmail(""); ok {
		t.Fatal("пользователь с пустым email создан — ошибка")
	}
	for _, e := range []string{"admin@local", "admin@example.test"} {
		if _, ok := s.GetUserByEmail(e); ok {
			t.Fatalf("без ADMIN_EMAIL не должен создаваться %s", e)
		}
	}
	// Учитель из env (в тестах — тестовые значения) создаётся с настоящим хешем.
	u, ok := s.GetUserByEmail("teacher-test-seed@example.test")
	if !ok {
		t.Fatal("учитель по TEACHER_EMAIL не создан")
	}
	mustCompare(t, u.PasswordHash, "teacher-test-seed-1")
}
