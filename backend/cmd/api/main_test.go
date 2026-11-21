package main

// Регресс-тесты production-safety (T16/аудит):
//   - seedAdmin создаёт bootstrap-учётки только при явных ADMIN_*/TEACHER_*;
//     дефолтных teacher@local/teacher1 и admin@local больше нет;
//   - существующий валидный пароль не перезаписывается при рестарте;
//   - невалидный bcrypt-хеш (заглушка старой 002_seed.sql) чинится, иначе
//     вход в такую учётку невозможен;
//   - production без DATABASE_URL/с битым DSN не деградирует в in-memory;
//   - dev без БД по-прежнему получает in-memory хранилище с демо-курсом.

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"edu-platform/backend/internal/config"
	"edu-platform/backend/internal/repo"
)

// fakeMigrationHash — заглушка из старой 002_seed.sql: 50 символов «A»
// плюс открытый текст пароля. bcrypt.Cost/CompareHashAndPassword на ней падают.
const fakeMigrationHash = "$2a$10$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAadmin1234"

// seedConfig собирает конфиг явных bootstrap-учёток (пустые — не создаются).
func seedConfig(adminEmail, adminPassword, teacherEmail, teacherPassword string) *config.Config {
	return &config.Config{
		AdminEmail:      adminEmail,
		AdminPassword:   adminPassword,
		TeacherEmail:    teacherEmail,
		TeacherPassword: teacherPassword,
	}
}

// mustCompare сверяет хеш с паролем и падает при расхождении.
func mustCompare(t *testing.T, hash, password string) {
	t.Helper()
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		t.Fatalf("хеш не соответствует паролю %q: %v", password, err)
	}
}

// mustNotCompare проверяет, что пароль к хешу не подходит.
func mustNotCompare(t *testing.T, hash, password string) {
	t.Helper()
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err == nil {
		t.Fatalf("пароль %q не должен подходить к хешу", password)
	}
}

// TestSeedAdminCreatesMissingUser — пользователя нет → создаётся с настоящим
// bcrypt и ролью admin; пароль реально входит.
func TestSeedAdminCreatesMissingUser(t *testing.T) {
	s := repo.NewStore()
	seedAdmin(s, seedConfig("seed-create@example.test", "пароль-админа-1", "", ""))

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
// из старой миграции → хеш перезаписывается настоящим (учётка иначе
// нерабочая). Это единственный случай перезаписи пароля.
func TestSeedAdminRepairsFakeHash(t *testing.T) {
	s := repo.NewStore()
	if _, err := s.CreateUser("admin@local", fakeMigrationHash, repo.RoleAdmin); err != nil {
		t.Fatalf("создание сид-пользователя: %v", err)
	}

	seedAdmin(s, seedConfig("admin@local", "admin1234", "", ""))

	u, ok := s.GetUserByEmail("admin@local")
	if !ok {
		t.Fatal("пользователь пропал")
	}
	if u.PasswordHash == fakeMigrationHash {
		t.Fatal("поддельный хеш не был перезаписан — вход по-прежнему невозможен")
	}
	mustCompare(t, u.PasswordHash, "admin1234")
}

// TestSeedAdminKeepsExistingValidPassword — хеш валиден, но от ДРУГОГО
// пароля: существующий пароль НЕ перезаписывается (рестарт с другим
// ADMIN_PASSWORD не должен молча менять доступ).
func TestSeedAdminKeepsExistingValidPassword(t *testing.T) {
	s := repo.NewStore()
	oldHash, err := bcrypt.GenerateFromPassword([]byte("старый-пароль"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("хеш: %v", err)
	}
	if _, err := s.CreateUser("admin-existing@example.test", string(oldHash), repo.RoleAdmin); err != nil {
		t.Fatalf("создание: %v", err)
	}

	seedAdmin(s, seedConfig("admin-existing@example.test", "новый-пароль", "", ""))

	u, _ := s.GetUserByEmail("admin-existing@example.test")
	if u.PasswordHash != string(oldHash) {
		t.Fatal("валидный хеш существующего пользователя не должен перезаписываться")
	}
	mustCompare(t, u.PasswordHash, "старый-пароль")
	mustNotCompare(t, u.PasswordHash, "новый-пароль")
}

// TestSeedAdminKeepsValidHash — хеш уже корректен → НЕ перезаписывается
// (каждый рестарт не должен менять хеш и ломать параллельные сессии).
func TestSeedAdminKeepsValidHash(t *testing.T) {
	s := repo.NewStore()
	goodHash, err := bcrypt.GenerateFromPassword([]byte("верный-пароль"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("хеш: %v", err)
	}
	if _, err := s.CreateUser("admin-valid@example.test", string(goodHash), repo.RoleAdmin); err != nil {
		t.Fatalf("создание: %v", err)
	}

	seedAdmin(s, seedConfig("admin-valid@example.test", "верный-пароль", "", ""))

	u, _ := s.GetUserByEmail("admin-valid@example.test")
	if u.PasswordHash != string(goodHash) {
		t.Fatal("корректный хеш не должен перезаписываться")
	}
}

// TestSeedAdminFixesRole — роль приводится к требуемой, если пароль из env
// подтверждён; сам валидный хеш при этом не перезаписывается.
func TestSeedAdminFixesRole(t *testing.T) {
	s := repo.NewStore()
	goodHash, _ := bcrypt.GenerateFromPassword([]byte("п-1"), bcrypt.MinCost)
	if _, err := s.CreateUser("admin-role@example.test", string(goodHash), repo.RoleStudent); err != nil {
		t.Fatalf("создание: %v", err)
	}

	if err := seedAdmin(s, seedConfig("admin-role@example.test", "п-1", "", "")); err != nil {
		t.Fatalf("seedAdmin: %v", err)
	}

	u, _ := s.GetUserByEmail("admin-role@example.test")
	if u.Role != repo.RoleAdmin {
		t.Fatalf("роль=%v, ждали admin", u.Role)
	}
	if u.PasswordHash != string(goodHash) {
		t.Fatal("пароль существующего пользователя не должен перезаписываться")
	}
}

// TestSeedAdminRejectsRolePromotionWithoutPassword — критический сценарий:
// атакующий заранее зарегистрировал ADMIN_EMAIL со своим паролем. Рестарт не
// должен повышать его до админа и не должен молча оставлять систему без
// администратора: startup обязан упасть с понятной ошибкой.
func TestSeedAdminRejectsRolePromotionWithoutPassword(t *testing.T) {
	s := repo.NewStore()
	attackerHash, err := bcrypt.GenerateFromPassword([]byte("чужой-пароль-1"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("хеш: %v", err)
	}
	if _, err := s.CreateUser("admin-taken@example.test", string(attackerHash), repo.RoleStudent); err != nil {
		t.Fatalf("создание: %v", err)
	}

	err = seedAdmin(s, seedConfig("admin-taken@example.test", "пароль-оператора", "", ""))
	if err == nil {
		t.Fatal("повышение роли без подтверждения пароля должно быть ошибкой")
	}
	if !strings.Contains(err.Error(), "пароль") {
		t.Fatalf("ошибка не объясняет причину: %v", err)
	}

	u, _ := s.GetUserByEmail("admin-taken@example.test")
	if u.Role != repo.RoleStudent {
		t.Fatalf("роль=%v, атакующий не должен стать админом", u.Role)
	}
	mustCompare(t, u.PasswordHash, "чужой-пароль-1")
	mustNotCompare(t, u.PasswordHash, "пароль-оператора")
}

// TestSeedAdminNoConfigCreatesNoDefaults — без ADMIN_*/TEACHER_* не создаётся
// ни одна учётка, в том числе известные дефолты teacher@local/admin@local.
func TestSeedAdminNoConfigCreatesNoDefaults(t *testing.T) {
	s := repo.NewStore()
	if err := seedAdmin(s, seedConfig("", "", "", "")); err != nil {
		t.Fatalf("seedAdmin без учёток: %v", err)
	}

	if users := s.ListUsers(repo.UserFilter{}); len(users) != 0 {
		t.Fatalf("без явного конфига созданы пользователи: %+v", users)
	}
	for _, e := range []string{"admin@local", "teacher@local", "demo-teacher@local"} {
		if _, ok := s.GetUserByEmail(e); ok {
			t.Fatalf("без явного конфига не должен создаваться %s", e)
		}
	}
}

// TestSeedAdminCreatesExplicitTeacher — преподаватель создаётся только при
// явных TEACHER_EMAIL + TEACHER_PASSWORD.
func TestSeedAdminCreatesExplicitTeacher(t *testing.T) {
	s := repo.NewStore()
	if err := seedAdmin(s, seedConfig("", "", "teacher-explicit@example.test", "teacher-пароль-1")); err != nil {
		t.Fatalf("seedAdmin: %v", err)
	}

	u, ok := s.GetUserByEmail("teacher-explicit@example.test")
	if !ok {
		t.Fatal("преподаватель по TEACHER_* не создан")
	}
	if u.Role != repo.RoleTeacher {
		t.Fatalf("роль=%v, ждали teacher", u.Role)
	}
	mustCompare(t, u.PasswordHash, "teacher-пароль-1")

	for _, e := range []string{"admin@local", "teacher@local"} {
		if _, ok := s.GetUserByEmail(e); ok {
			t.Fatalf("не должен создаваться дефолт %s", e)
		}
	}
}

// TestSeedAdminRequiresBothAdminFields — одной переменной ADMIN_* мало.
func TestSeedAdminRequiresBothAdminFields(t *testing.T) {
	for name, cfg := range map[string]*config.Config{
		"только email":    seedConfig("admin-only@example.test", "", "", ""),
		"только password": seedConfig("", "admin-only-pass", "", ""),
	} {
		t.Run(name, func(t *testing.T) {
			s := repo.NewStore()
			if err := seedAdmin(s, cfg); err == nil {
				t.Fatal("неполный ADMIN_* должен быть ошибкой конфигурации")
			}
			if users := s.ListUsers(repo.UserFilter{}); len(users) != 0 {
				t.Fatalf("созданы пользователи при неполном ADMIN_*: %+v", users)
			}
		})
	}
}

// TestSeedAdminRequiresBothTeacherFields — одной переменной TEACHER_* мало.
func TestSeedAdminRequiresBothTeacherFields(t *testing.T) {
	for name, cfg := range map[string]*config.Config{
		"только email":    seedConfig("", "", "teacher-only@example.test", ""),
		"только password": seedConfig("", "", "", "teacher-only-pass"),
	} {
		t.Run(name, func(t *testing.T) {
			s := repo.NewStore()
			if err := seedAdmin(s, cfg); err == nil {
				t.Fatal("неполный TEACHER_* должен быть ошибкой конфигурации")
			}
			if users := s.ListUsers(repo.UserFilter{}); len(users) != 0 {
				t.Fatalf("созданы пользователи при неполном TEACHER_*: %+v", users)
			}
		})
	}
}

// TestOpenStoreProductionRequiresDatabaseURL — production без DATABASE_URL
// обязан вернуть ошибку (main сделает log.Fatalf), а не in-memory store.
func TestOpenStoreProductionRequiresDatabaseURL(t *testing.T) {
	cfg := &config.Config{AppEnv: "production"}
	store, closeFn, err := openStore(cfg)
	if err == nil {
		t.Fatal("production без DATABASE_URL не должен запускаться")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("ошибка не упоминает DATABASE_URL: %v", err)
	}
	if store != nil {
		t.Fatalf("в production нельзя отдавать in-memory store: %T", store)
	}
	if closeFn != nil {
		t.Fatal("closeFn должен быть nil при ошибке")
	}
}

// TestOpenStoreProductionDoesNotFallbackOnBadDSN — ошибка ConnectPool в
// production не должна приводить к in-memory fallback.
func TestOpenStoreProductionDoesNotFallbackOnBadDSN(t *testing.T) {
	cfg := &config.Config{AppEnv: "production", DatabaseURL: "://invalid-dsn"}
	store, closeFn, err := openStore(cfg)
	if err == nil {
		t.Fatal("битый DATABASE_URL в production должен давать ошибку")
	}
	if store != nil {
		t.Fatalf("в production нельзя отдавать in-memory store: %T", store)
	}
	if closeFn != nil {
		t.Fatal("closeFn должен быть nil при ошибке")
	}
}

// TestOpenStoreDevWithoutDatabaseUsesMemory — dev без DATABASE_URL сохраняет
// прежний удобный режим: in-memory + демо-курс.
func TestOpenStoreDevWithoutDatabaseUsesMemory(t *testing.T) {
	cfg := &config.Config{AppEnv: "development", CommissionPct: 15}
	store, closeFn, err := openStore(cfg)
	if err != nil {
		t.Fatalf("dev без БД не должен падать: %v", err)
	}
	if closeFn != nil {
		t.Fatal("для in-memory closeFn должен быть nil")
	}
	if _, ok := store.(*repo.Store); !ok {
		t.Fatalf("ждали *repo.Store, получили %T", store)
	}
	if !store.SlugTaken("python-s-nulya") {
		t.Fatal("dev in-memory должен содержать демо-курс")
	}
	if _, ok := store.GetUserByEmail("demo-teacher@local"); !ok {
		t.Fatal("dev in-memory должен содержать демо-преподавателя")
	}
}

// TestOpenStoreDevFallsBackOnBadDSN — в dev ошибка ConnectPool допустима:
// деградируем в in-memory с предупреждением.
func TestOpenStoreDevFallsBackOnBadDSN(t *testing.T) {
	cfg := &config.Config{AppEnv: "development", DatabaseURL: "://invalid-dsn", CommissionPct: 15}
	store, closeFn, err := openStore(cfg)
	if err != nil {
		t.Fatalf("dev fallback не должен возвращать ошибку: %v", err)
	}
	if closeFn != nil {
		t.Fatal("для in-memory closeFn должен быть nil")
	}
	if _, ok := store.(*repo.Store); !ok {
		t.Fatalf("ждали *repo.Store, получили %T", store)
	}
}
