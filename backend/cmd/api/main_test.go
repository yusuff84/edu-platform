package main

// Регресс-тесты P11-7 (критические риски старта C3/C4).
//
// C4: в production API не должен молча откатываться в in-memory, если
// DATABASE_URL пуст или Postgres недоступен. В dev fallback с demo-сидом
// сохраняется.
//
// C3: seedAdmin не создаёт teacher@local/teacher1 без явных TEACHER_*,
// не сбрасывает пароль и роль существующих пользователей; точные
// placeholder-хеши из 002_seed.sql заменяются паролем из env, чтобы свежая
// Postgres-БД не оставалась без входа. demo-учитель demo-teacher@local
// создаётся только в in-memory сиде seedDemo.

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"edu-platform/backend/internal/config"
	"edu-platform/backend/internal/repo"
)

// fakeMigrationHash / fakeTeacherMigrationHash — точные заглушки из
// 002_seed.sql: 50 символов «A»/«B» плюс открытый текст пароля.
// bcrypt.Cost на них падает.
const (
	fakeMigrationHash        = "$2a$10$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAadmin1234"
	fakeTeacherMigrationHash = "$2a$10$BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBteacher1"
)

// newSeedConfig собирает конфиг для seedAdmin и по умолчанию отключает
// сид учителя: без явных TEACHER_* никакая учётка учителя создаваться не должна.
func newSeedConfig(t *testing.T, adminEmail, adminPassword string) *config.Config {
	t.Helper()
	t.Setenv("TEACHER_EMAIL", "")
	t.Setenv("TEACHER_PASSWORD", "")
	return &config.Config{AdminEmail: adminEmail, AdminPassword: adminPassword}
}

// setTeacherEnv задаёт переменные сида учителя для конкретного теста.
func setTeacherEnv(t *testing.T, email, password string) {
	t.Helper()
	t.Setenv("TEACHER_EMAIL", email)
	t.Setenv("TEACHER_PASSWORD", password)
}

// mustCompare сверяет хеш с паролем и падает при расхождении.
func mustCompare(t *testing.T, hash, password string) {
	t.Helper()
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		t.Fatalf("хеш не соответствует паролю %q: %v", password, err)
	}
}

// mustNotCompare проверяет, что пароль больше не подходит к хешу.
func mustNotCompare(t *testing.T, hash, password string) {
	t.Helper()
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err == nil {
		t.Fatalf("пароль %q не должен подходить к хешу", password)
	}
}

// ---- seedAdmin (C3) ----

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

// TestSeedAdminKeepsExistingPassword — существующий пользователь не должен
// получать новый пароль при старте (C3): ADMIN_PASSWORD задан, но хеш
// остаётся прежним, старый пароль продолжает работать.
func TestSeedAdminKeepsExistingPassword(t *testing.T) {
	s := repo.NewStore()
	oldHash, err := bcrypt.GenerateFromPassword([]byte("старый-пароль"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("хеш: %v", err)
	}
	if _, err := s.CreateUser("admin-existing@example.test", string(oldHash), repo.RoleAdmin); err != nil {
		t.Fatalf("создание: %v", err)
	}

	seedAdmin(s, newSeedConfig(t, "admin-existing@example.test", "новый-пароль"))

	u, ok := s.GetUserByEmail("admin-existing@example.test")
	if !ok {
		t.Fatal("пользователь пропал")
	}
	if u.PasswordHash != string(oldHash) {
		t.Fatal("пароль существующего пользователя не должен меняться при старте")
	}
	mustCompare(t, u.PasswordHash, "старый-пароль")
	mustNotCompare(t, u.PasswordHash, "новый-пароль")
	if u.Role != repo.RoleAdmin {
		t.Fatalf("роль=%v, ждали admin", u.Role)
	}
}

// TestSeedAdminRepairsAdminPlaceholder — точная заглушка admin@local из
// 002_seed.sql заменяется настоящим bcrypt-хешем пароля из ADMIN_PASSWORD:
// иначе на свежей Postgres-БД войти под admin@local невозможно.
func TestSeedAdminRepairsAdminPlaceholder(t *testing.T) {
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
		t.Fatal("точная seed-заглушка должна быть заменена при старте")
	}
	mustCompare(t, u.PasswordHash, "admin1234")
	if u.Role != repo.RoleAdmin {
		t.Fatalf("роль=%v, ждали admin (замена хеша не меняет роль)", u.Role)
	}
}

// TestSeedAdminRepairsTeacherPlaceholder — та же замена для точной заглушки
// teacher@local при явных TEACHER_*.
func TestSeedAdminRepairsTeacherPlaceholder(t *testing.T) {
	s := repo.NewStore()
	if _, err := s.CreateUser("teacher@local", fakeTeacherMigrationHash, repo.RoleTeacher); err != nil {
		t.Fatalf("создание сид-учителя: %v", err)
	}

	cfg := newSeedConfig(t, "", "")
	setTeacherEnv(t, "teacher@local", "teacher1")
	seedAdmin(s, cfg)

	u, ok := s.GetUserByEmail("teacher@local")
	if !ok {
		t.Fatal("пользователь пропал")
	}
	if u.PasswordHash == fakeTeacherMigrationHash {
		t.Fatal("точная seed-заглушка учителя должна быть заменена при старте")
	}
	mustCompare(t, u.PasswordHash, "teacher1")
	if u.Role != repo.RoleTeacher {
		t.Fatalf("роль=%v, ждали teacher (замена хеша не меняет роль)", u.Role)
	}
}

// TestSeedAdminDoesNotRepairSimilarFakeHash — похожий, но не точный
// placeholder не трогаем: иначе можно случайно сбросить реальный пароль.
func TestSeedAdminDoesNotRepairSimilarFakeHash(t *testing.T) {
	s := repo.NewStore()
	similar := fakeMigrationHash + "x"
	if _, err := s.CreateUser("admin@local", similar, repo.RoleAdmin); err != nil {
		t.Fatalf("создание пользователя: %v", err)
	}

	seedAdmin(s, newSeedConfig(t, "admin@local", "admin1234"))

	u, ok := s.GetUserByEmail("admin@local")
	if !ok {
		t.Fatal("пользователь пропал")
	}
	if u.PasswordHash != similar {
		t.Fatal("незнакомый битый хеш не должен перезаписываться при старте")
	}
}

// TestSeedAdminDoesNotChangeRole — существующей учётке роль не меняется:
// иначе можно неявно повысить до admin уже зарегистрированного пользователя.
func TestSeedAdminDoesNotChangeRole(t *testing.T) {
	s := repo.NewStore()
	hash, err := bcrypt.GenerateFromPassword([]byte("пароль-студента"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("хеш: %v", err)
	}
	if _, err := s.CreateUser("admin-role@example.test", string(hash), repo.RoleStudent); err != nil {
		t.Fatalf("создание: %v", err)
	}

	seedAdmin(s, newSeedConfig(t, "admin-role@example.test", "пароль-студента"))

	u, ok := s.GetUserByEmail("admin-role@example.test")
	if !ok {
		t.Fatal("пользователь пропал")
	}
	if u.Role != repo.RoleStudent {
		t.Fatalf("роль=%v, ждали student (старт не должен менять роль)", u.Role)
	}
}

// TestSeedAdminNoDefaults — без ADMIN_* и TEACHER_* не создаётся ни админ,
// ни teacher@local/teacher1 (главный риск C3).
func TestSeedAdminNoDefaults(t *testing.T) {
	s := repo.NewStore()
	seedAdmin(s, newSeedConfig(t, "", ""))

	if users := s.ListUsers(repo.UserFilter{}); len(users) != 0 {
		t.Fatalf("без явных env не должно создаваться учёток, создано %d", len(users))
	}
	for _, e := range []string{"", "admin@local", "teacher@local", "teacher@example.test"} {
		if _, ok := s.GetUserByEmail(e); ok {
			t.Fatalf("без явных env не должен создаваться %s", e)
		}
	}
}

// TestSeedAdminTeacherRequiresBothEnv — учитель создаётся только когда заданы
// И TEACHER_EMAIL, И TEACHER_PASSWORD.
func TestSeedAdminTeacherRequiresBothEnv(t *testing.T) {
	cases := []struct {
		name     string
		email    string
		password string
	}{
		{name: "только email", email: "teacher@example.test", password: ""},
		{name: "только пароль", email: "", password: "пароль-учителя"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := repo.NewStore()
			cfg := newSeedConfig(t, "", "")
			setTeacherEnv(t, tc.email, tc.password)
			seedAdmin(s, cfg)

			if users := s.ListUsers(repo.UserFilter{}); len(users) != 0 {
				t.Fatalf("при неполных TEACHER_* учётки создаваться не должны, создано %d", len(users))
			}
		})
	}
}

// TestSeedAdminCreatesTeacherWithExplicitEnv — при полной паре TEACHER_*
// учитель создаётся с настоящим хешем и ролью teacher.
func TestSeedAdminCreatesTeacherWithExplicitEnv(t *testing.T) {
	s := repo.NewStore()
	cfg := newSeedConfig(t, "", "")
	setTeacherEnv(t, "teacher@example.test", "сильный-пароль-учителя")
	seedAdmin(s, cfg)

	u, ok := s.GetUserByEmail("teacher@example.test")
	if !ok {
		t.Fatal("учитель по явным TEACHER_* не создан")
	}
	if u.Role != repo.RoleTeacher {
		t.Fatalf("роль=%v, ждали teacher", u.Role)
	}
	mustCompare(t, u.PasswordHash, "сильный-пароль-учителя")
}

// TestSeedAdminTeacherKeepsExistingPassword — если учитель уже есть, старт
// с другими TEACHER_* не сбрасывает его пароль.
func TestSeedAdminTeacherKeepsExistingPassword(t *testing.T) {
	s := repo.NewStore()
	oldHash, err := bcrypt.GenerateFromPassword([]byte("старый-пароль-учителя"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("хеш: %v", err)
	}
	if _, err := s.CreateUser("teacher@example.test", string(oldHash), repo.RoleTeacher); err != nil {
		t.Fatalf("создание: %v", err)
	}

	cfg := newSeedConfig(t, "", "")
	setTeacherEnv(t, "teacher@example.test", "новый-пароль-учителя")
	seedAdmin(s, cfg)

	u, _ := s.GetUserByEmail("teacher@example.test")
	if u.PasswordHash != string(oldHash) {
		t.Fatal("пароль существующего учителя не должен меняться при старте")
	}
	mustCompare(t, u.PasswordHash, "старый-пароль-учителя")
	mustNotCompare(t, u.PasswordHash, "новый-пароль-учителя")
}

// TestSeedAdminAdminRequiresBothEnv — админ не создаётся при неполной паре
// ADMIN_EMAIL/ADMIN_PASSWORD.
func TestSeedAdminAdminRequiresBothEnv(t *testing.T) {
	cases := []struct {
		name     string
		email    string
		password string
	}{
		{name: "только email", email: "admin-only@example.test", password: ""},
		{name: "только пароль", email: "", password: "пароль-админа"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := repo.NewStore()
			seedAdmin(s, newSeedConfig(t, tc.email, tc.password))

			if users := s.ListUsers(repo.UserFilter{}); len(users) != 0 {
				t.Fatalf("при неполных ADMIN_* учётки создаваться не должны, создано %d", len(users))
			}
		})
	}
}

// ---- openStore (C4) ----

// TestOpenStoreCheckedDevFallsBackToMemory — в dev без DATABASE_URL остаётся
// прежний fallback: in-memory с demo-сидом (курс и demo-teacher@local).
func TestOpenStoreCheckedDevFallsBackToMemory(t *testing.T) {
	store, closeDB, err := openStoreChecked(&config.Config{AppEnv: "development", CommissionPct: 15})
	if err != nil {
		t.Fatalf("в dev fallback должен работать, получена ошибка: %v", err)
	}
	if store == nil {
		t.Fatal("store не создан")
	}
	if closeDB != nil {
		t.Fatal("для in-memory close должен быть nil")
	}
	if _, ok := store.GetCourseBySlug("python-s-nulya"); !ok {
		t.Fatal("demo-сид не применён: курса python-s-nulya нет")
	}
	u, ok := store.GetUserByEmail("demo-teacher@local")
	if !ok {
		t.Fatal("demo-учитель не создан в in-memory сиде")
	}
	mustCompare(t, u.PasswordHash, "teach1234")
}

// TestOpenStoreCheckedDevFallsBackOnUnreachableDB — в dev при недоступном
// Postgres fallback сохраняется (порт 1 гарантированно закрыт, реальная БД
// не нужна).
func TestOpenStoreCheckedDevFallsBackOnUnreachableDB(t *testing.T) {
	cfg := &config.Config{
		AppEnv:        "development",
		DatabaseURL:   "postgres://edu:edu@127.0.0.1:1/edu?sslmode=disable",
		CommissionPct: 15,
	}
	store, closeDB, err := openStoreChecked(cfg)
	if err != nil {
		t.Fatalf("в dev при недоступной БД ожидали fallback, получена ошибка: %v", err)
	}
	if store == nil {
		t.Fatal("store не создан")
	}
	if closeDB != nil {
		t.Fatal("для in-memory close должен быть nil")
	}
	if _, ok := store.GetCourseBySlug("python-s-nulya"); !ok {
		t.Fatal("demo-сид не применён при dev-fallback")
	}
}

// TestOpenStoreCheckedProductionRequiresDatabase — в production пустой
// DATABASE_URL — ошибка, а не in-memory: вызывающий завершит процесс.
func TestOpenStoreCheckedProductionRequiresDatabase(t *testing.T) {
	store, closeDB, err := openStoreChecked(&config.Config{AppEnv: "production", CommissionPct: 15})
	if err == nil {
		t.Fatal("в production без DATABASE_URL должна быть ошибка")
	}
	if store != nil || closeDB != nil {
		t.Fatal("при критической ошибке store/close должны быть nil")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("ошибка должна упоминать DATABASE_URL: %v", err)
	}
	if !strings.Contains(err.Error(), "production") {
		t.Fatalf("ошибка должна упоминать production: %v", err)
	}
}

// TestOpenStoreCheckedProductionNoFallbackOnUnreachableDB — в production
// недоступный Postgres — ошибка, а не in-memory с demo-сидом.
func TestOpenStoreCheckedProductionNoFallbackOnUnreachableDB(t *testing.T) {
	cfg := &config.Config{
		AppEnv:        "production",
		DatabaseURL:   "postgres://edu:edu@127.0.0.1:1/edu?sslmode=disable",
		CommissionPct: 15,
	}
	store, closeDB, err := openStoreChecked(cfg)
	if err == nil {
		t.Fatal("в production при недоступной БД должна быть ошибка, а не in-memory fallback")
	}
	if store != nil || closeDB != nil {
		t.Fatal("при критической ошибке store/close должны быть nil")
	}
	if !strings.Contains(err.Error(), "Postgres") {
		t.Fatalf("ошибка должна упоминать Postgres: %v", err)
	}
	if !strings.Contains(err.Error(), "production") {
		t.Fatalf("ошибка должна упоминать production: %v", err)
	}
}
