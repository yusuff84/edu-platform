package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"edu-platform/backend/internal/config"
	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/mailer"
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

	smtpSender, err := mailer.NewSMTP(mailer.Config{
		Addr:     cfg.SMTPAddr,
		From:     cfg.SMTPFrom,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
		TLSMode:  cfg.SMTPTLS,
	})
	if err != nil {
		log.Printf("ПРЕДУПРЕЖДЕНИЕ: SMTP отключён: %v", err)
	} else if smtpSender != nil {
		log.Printf("SMTP включён: приглашения уходят на email")
	} else {
		log.Printf("SMTP не задан: приглашения создаются со ссылкой в API/UI")
	}
	var mailerSender mailer.Sender
	if smtpSender != nil {
		mailerSender = smtpSender
	}

	app := &handler.App{Store: store, Config: cfg, Mailer: mailerSender}
	addr := ":" + cfg.Port
	log.Printf("edu-platform api слушает %s (провайдер оплаты: %s)", addr, cfg.PaymentsProvider)
	if err := http.ListenAndServe(addr, handler.NewRouter(app)); err != nil {
		log.Fatalf("api остановлен: %v", err)
	}
}

// openStore выбирает хранилище: Postgres при достижимой DATABASE_URL.
// В dev-режиме при недоступной БД разрешён откат в in-memory с demo-сидом.
// В production любой сбой — критическая ошибка: log.Fatalf завершает процесс,
// контейнер перезапускается, но API не начинает молча работать на пустой
// памяти (иначе данные «теряются», а demo-сид подменяет реальную БД).
// Возвращает store и опциональный close для пула (nil для in-memory).
func openStore(cfg *config.Config) (repo.StoreAPI, func()) {
	store, closeDB, err := openStoreChecked(cfg)
	if err != nil {
		log.Fatalf("КРИТИЧЕСКАЯ ОШИБКА старта: %v", err)
	}
	return store, closeDB
}

// openStoreChecked — реализация openStore с возвратом ошибки вместо
// log.Fatalf: решение «fallback или падать» покрывается тестами без
// реального Postgres. В dev ошибки БД логируются и приводят к in-memory
// с demo-сидом, в production возвращается ошибка (openStore завершает
// процесс через log.Fatalf).
func openStoreChecked(cfg *config.Config) (repo.StoreAPI, func(), error) {
	newMem := func() (repo.StoreAPI, func(), error) {
		s := repo.NewStore()
		if err := s.SetCommissionPct(cfg.CommissionPct); err != nil {
			log.Printf("ПРЕДУПРЕЖДЕНИЕ: не удалось задать комиссию в памяти: %v", err)
		}
		seedDemo(s)
		return s, nil, nil
	}
	fallback := func(reason string) (repo.StoreAPI, func(), error) {
		if !cfg.IsDev() {
			return nil, nil, fmt.Errorf("%s; в production (APP_ENV=%q) откат в in-memory и demo-сид запрещены — задайте рабочий DATABASE_URL (контейнер будет перезапущен)", reason, cfg.AppEnv)
		}
		log.Printf("ПРЕДУПРЕЖДЕНИЕ: %s — работаем в in-memory режиме без БД (данные сбросятся при перезапуске)", reason)
		return newMem()
	}

	dsn := strings.TrimSpace(cfg.DatabaseURL)
	if dsn == "" {
		return fallback("DATABASE_URL не задан")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	pool, err := repo.ConnectPool(ctx, dsn)
	if err != nil {
		return fallback(fmt.Sprintf("не удалось создать пул Postgres: %v", err))
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fallback(fmt.Sprintf("Postgres по DATABASE_URL недоступен: %v", err))
	}

	dir, err := findMigrationsDir()
	if err != nil {
		pool.Close()
		return fallback(err.Error())
	}
	if err := migrate.ApplyMigrations(ctx, pool, dir); err != nil {
		pool.Close()
		return fallback(fmt.Sprintf("не удалось применить миграции: %v", err))
	}
	// Сид идемпотентен (ON CONFLICT DO NOTHING): применяется только если courses пуста.
	if err := migrate.SeedIfEmpty(ctx, pool, filepath.Join(dir, "002_seed.sql")); err != nil {
		pool.Close()
		return fallback(fmt.Sprintf("не удалось применить сид: %v", err))
	}

	// Комиссия: при доступной БД — из app_settings (ключ commission_pct), fallback — env.
	effective := migrate.GetCommissionPct(ctx, pool, cfg.CommissionPct)
	pg := repo.NewPostgresStore(pool, effective)
	log.Printf("Postgres подключён: миграции применены, хранилище — pgxpool, комиссия=%d%%", effective)
	return pg, func() { pool.Close() }, nil
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
// из openStoreChecked для dev-режима без БД (в production fallback запрещён);
// при рестарте память пуста, сид идемпотентен по slug. В Postgres демо-учитель
// demo-teacher@local не создаётся.
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
		PriceCents:  490000,
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

// seedAdmin создаёт учётные записи только по явно заданным env.
// Пароли существующих пользователей при старте НЕ меняются: рестарт API —
// не операция сброса доступа. Демо-учитель demo-teacher@local/teach1234
// создаётся только в in-memory сиде seedDemo и в Postgres не попадает.
//
// ВАЖНО: раньше функция всегда создавала/сбрасывала teacher@local с паролем
// teacher1, даже без TEACHER_* (критический риск C3). Теперь:
//   - учитель — только если заданы И TEACHER_EMAIL, И TEACHER_PASSWORD;
//   - админ — только если заданы И ADMIN_EMAIL, И ADMIN_PASSWORD;
//   - существующая учётка не получает новый пароль и новую роль.
func seedAdmin(store repo.StoreAPI, cfg *config.Config) {
	adminEmail := strings.TrimSpace(cfg.AdminEmail)
	adminPassword := cfg.AdminPassword
	switch {
	case adminEmail != "" && adminPassword != "":
		ensureAccount(store, adminEmail, adminPassword, repo.RoleAdmin, "админ")
	case adminEmail != "" || adminPassword != "":
		log.Printf("ПРЕДУПРЕЖДЕНИЕ: сид админа пропущен — нужны одновременно ADMIN_EMAIL и ADMIN_PASSWORD")
	}

	// TEACHER_* читаем напрямую: в config.Config этих полей нет (config.go —
	// зона P11-1), а дефолтных учёток с известным паролем в коде быть не должно.
	teacherEmail := strings.TrimSpace(os.Getenv("TEACHER_EMAIL"))
	teacherPassword := os.Getenv("TEACHER_PASSWORD")
	switch {
	case teacherEmail != "" && teacherPassword != "":
		ensureAccount(store, teacherEmail, teacherPassword, repo.RoleTeacher, "преподаватель")
	case teacherEmail != "" || teacherPassword != "":
		log.Printf("ПРЕДУПРЕЖДЕНИЕ: сид преподавателя пропущен — нужны одновременно TEACHER_EMAIL и TEACHER_PASSWORD")
	}
}

// Точные placeholder-хеши из миграции 002_seed.sql. Это не настоящие
// bcrypt-хеши: после 50 символов «A»/«B» в строке стоит открытый пароль,
// поэтому bcrypt.Cost на них падает, а вход невозможен. Чиним только эти
// два конкретных значения, чтобы не сбросить реальный пароль пользователя
// (C3): любое другое значение, включая похожие подделки, не трогаем.
const (
	seedAdminPlaceholderHash   = "$2a$10$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAadmin1234"
	seedTeacherPlaceholderHash = "$2a$10$BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBteacher1"
)

// isSeedPlaceholderHash сообщает, что хеш — точная заглушка из 002_seed.sql.
func isSeedPlaceholderHash(hash string) bool {
	return hash == seedAdminPlaceholderHash || hash == seedTeacherPlaceholderHash
}

// ensureAccount создаёт учётку, только если её ещё нет. Существующим
// пользователям пароль и роль при старте не меняются — это исключает
// неявный сброс доступа (C3) и повышение роли уже зарегистрированного
// пользователя. Исключение — точные placeholder-хеши из 002_seed.sql:
// они не являются bcrypt и без замены оставляют свежую БД без входа.
// label — имя роли по-русски для логов.
func ensureAccount(store repo.StoreAPI, email, password string, role repo.Role, label string) {
	email = strings.TrimSpace(email)
	if email == "" || password == "" {
		return
	}

	if user, exists := store.GetUserByEmail(email); exists {
		// Заменяем только известную заглушку миграции на пароль из env.
		// Реальный bcrypt-хеш (и любой незнакомый битый хеш) не трогаем.
		if isSeedPlaceholderHash(user.PasswordHash) {
			hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if err != nil {
				log.Printf("не удалось захешировать пароль %s: %v", email, err)
				return
			}
			if !store.SetUserPasswordHash(user.ID, string(hash)) {
				log.Printf("не удалось заменить seed-заглушку пароля для %s", email)
				return
			}
			log.Printf("%s %s: seed-заглушка пароля заменена паролем из env", label, email)
			return
		}
		if _, err := bcrypt.Cost([]byte(user.PasswordHash)); err != nil {
			log.Printf("ПРЕДУПРЕЖДЕНИЕ: %s %s уже существует с невалидным bcrypt-хешем; пароль при старте не сбрасывается — задайте его вручную", label, email)
		} else {
			log.Printf("%s %s уже существует — пароль и роль при старте не меняются", label, email)
		}
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("не удалось захешировать пароль %s: %v", email, err)
		return
	}
	if _, err := store.CreateUser(email, string(hash), role); err != nil {
		log.Printf("не удалось создать %s: %v", email, err)
		return
	}
	log.Printf("создан %s %s", label, email)
}
