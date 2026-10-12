package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"edu-platform/backend/internal/config"
	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/migrate"
	"edu-platform/backend/internal/repo"
)

func main() {
	cfg := config.Load()

	store, closeDB := openStore(cfg)
	if closeDB != nil {
		defer closeDB()
	}
	seedAdmin(store, cfg)

	app := &handler.App{Store: store, Config: cfg}
	addr := ":" + cfg.Port
	log.Printf("edu-platform api слушает %s (провайдер оплаты: %s)", addr, cfg.PaymentsProvider)
	if err := http.ListenAndServe(addr, handler.NewRouter(app)); err != nil {
		log.Fatalf("api остановлен: %v", err)
	}
}

// openStore выбирает хранилище: Postgres при достижимой DATABASE_URL,
// иначе — in-memory с предупреждением в логе на русском.
// Возвращает store и опциональный close для пула (nil для in-memory).
func openStore(cfg *config.Config) (repo.StoreAPI, func()) {
	newMem := func() (repo.StoreAPI, func()) {
		s := repo.NewStore()
		s.SetCommissionPct(cfg.CommissionPct)
		seedDemo(s)
		return s, nil
	}

	dsn := strings.TrimSpace(cfg.DatabaseURL)
	if dsn == "" {
		log.Printf("ПРЕДУПРЕЖДЕНИЕ: DATABASE_URL не задан — работаем в in-memory режиме без БД (данные сбросятся при перезапуске)")
		return newMem()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	pool, err := repo.ConnectPool(ctx, dsn)
	if err != nil {
		log.Printf("ПРЕДУПРЕЖДЕНИЕ: не удалось создать пул Postgres (%v) — работаем в in-memory режиме без БД", err)
		return newMem()
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		log.Printf("ПРЕДУПРЕЖДЕНИЕ: Postgres по DATABASE_URL недоступен (%v) — работаем в in-memory режиме без БД", err)
		return newMem()
	}

	dir, err := findMigrationsDir()
	if err != nil {
		pool.Close()
		log.Printf("ПРЕДУПРЕЖДЕНИЕ: %v — работаем в in-memory режиме без БД", err)
		return newMem()
	}
	if err := migrate.ApplyMigrations(ctx, pool, dir); err != nil {
		pool.Close()
		log.Printf("ПРЕДУПРЕЖДЕНИЕ: не удалось применить миграции (%v) — работаем в in-memory режиме без БД", err)
		return newMem()
	}
	// Сид идемпотентен (ON CONFLICT DO NOTHING): применяется только если courses пуста.
	if err := migrate.SeedIfEmpty(ctx, pool, filepath.Join(dir, "002_seed.sql")); err != nil {
		pool.Close()
		log.Printf("ПРЕДУПРЕЖДЕНИЕ: не удалось применить сид (%v) — работаем в in-memory режиме без БД", err)
		return newMem()
	}

	// Комиссия: при доступной БД — из app_settings (ключ commission_pct), fallback — env.
	effective := migrate.GetCommissionPct(ctx, pool, cfg.CommissionPct)
	pg := repo.NewPostgresStore(pool, effective)
	log.Printf("Postgres подключён: миграции применены, хранилище — pgxpool, комиссия=%d%%", effective)
	return pg, func() { pool.Close() }
}

func findMigrationsDir() (string, error) {
	candidates := []string{
		"internal/migrate/migrations",         // запуск из backend/: go run ./cmd/api
		"backend/internal/migrate/migrations", // запуск из корня проекта
		"./migrations",
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(exe), "migrations"),
			filepath.Join(filepath.Dir(exe), "internal/migrate/migrations"),
		)
	}
	if env := strings.TrimSpace(os.Getenv("MIGRATIONS_DIR")); env != "" {
		candidates = append([]string{env}, candidates...)
	}
	return migrate.FindMigrationsDir(candidates...)
}

// seedDemo наполняет in-memory хранилище демо-курсом для локальной проверки
// сценариев quickstart (каталог, покупка, прохождение). Вызывается только
// для dev-режима без БД; при рестарте память пуста, сид идемпотентен по slug.
func seedDemo(s *repo.Store) {
	if s.SlugTaken("python-s-nulya") {
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("teach1234"), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("демо-сид: не удалось захешировать пароль: %v", err)
		return
	}
	teacher, err := s.CreateUser("demo-teacher@local", string(hash), repo.RoleTeacher)
	if err != nil {
		log.Printf("демо-сид: не удалось создать учителя: %v", err)
		return
	}
	c := s.CreateCourse(&repo.Course{
		TeacherID:   teacher.ID,
		Slug:        "python-s-nulya",
		Title:       "Python с нуля",
		Description: "Первый курс для новичков: установка, переменные, ветвления.",
		PriceCents:  4900,
		Currency:    "RUB",
		Status:      repo.CoursePublished,
	})
	// CreateCourse всегда создаёт черновик — переводим сид-курс в публикацию.
	s.SetCourseStatus(c.ID, repo.CoursePublished, "")
	m := s.CreateModule(c.ID, "Основы")
	s.CreateLesson(&repo.Lesson{
		ModuleID: m.ID,
		Title:    "Установка Python",
		Kind:     repo.LessonVideoText,
		BodyMD:   "Установи Python 3.12 с python.org и проверь `python --version`.",
	})
	s.CreateLesson(&repo.Lesson{
		ModuleID: m.ID,
		Title:    "Переменные: квиз",
		Kind:     repo.LessonQuiz,
		Quiz: &repo.QuizData{Questions: []repo.QuizQuestion{
			{ID: "q1", Text: "Что делает `x = 5`?", Options: []string{"Присваивает 5", "Сравнивает с 5", "Ошибка"}, Correct: 0, Explanation: "Одиночное `=` — присваивание."},
			{ID: "q2", Text: "Какой тип у `3.14`?", Options: []string{"int", "float", "str"}, Correct: 1, Explanation: "Дробные числа — float."},
		}},
	})
	s.CreateLesson(&repo.Lesson{
		ModuleID: m.ID,
		Title:    "Сумма: код",
		Kind:     repo.LessonCode,
		CodeTask: &repo.CodeTask{
			Language: "python",
			Template: "a, b = map(int, input().split())\nprint(a + b)\n",
			Tests: []repo.CodeTest{
				{Name: "пример", Input: "2 3", Expected: "5"},
				{Name: "скрытый", Input: "-1 1", Expected: "0", Hidden: true},
			},
		},
	})
	log.Printf("демо-сид: курс «Python с нуля» (учитель demo-teacher@local / teach1234)")
}

// seedAdmin создаёт админа только если заданы ADMIN_EMAIL и ADMIN_PASSWORD.
// Без этих переменных сидирование не выполняется — секретов в коде нет.
func seedAdmin(store repo.StoreAPI, cfg *config.Config) {
	if cfg.AdminEmail == "" || cfg.AdminPassword == "" {
		return
	}
	if _, ok := store.GetUserByEmail(cfg.AdminEmail); ok {
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("не удалось захешировать пароль админа: %v", err)
		return
	}
	if _, err := store.CreateUser(cfg.AdminEmail, string(hash), repo.RoleAdmin); err != nil {
		log.Printf("не удалось создать админа: %v", err)
		return
	}
	log.Printf("создан админ %s", cfg.AdminEmail)
}
