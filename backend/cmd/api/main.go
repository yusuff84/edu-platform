package main

import (
	"context"
	"errors"
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
	"edu-platform/backend/internal/payments"
	"edu-platform/backend/internal/repo"
)

func main() {
	cfg := config.Load()

	store, closeDB, err := openStore(cfg)
	if err != nil {
		log.Fatalf("хранилище не инициализировано: %v", err)
	}
	if closeDB != nil {
		defer closeDB()
	}
	if err := seedAdmin(store, cfg); err != nil {
		log.Fatalf("bootstrap-учётки не созданы: %v", err)
	}

	// В production проверяем ключи выбранного провайдера при старте, а не
	// отдаём 502 на первую покупку. В dev mock/stripe без ключей допустимы.
	if !cfg.IsDev() && cfg.PaymentsProvider != "disabled" {
		if _, err := payments.NewProvider(cfg.PaymentsProvider); err != nil {
			log.Fatalf("платёжный провайдер %q не настроен: %v", cfg.PaymentsProvider, err)
		}
	}

	smtpSender, err := mailer.NewSMTP(mailer.Config{
		Addr:     cfg.SMTPAddr,
		From:     cfg.SMTPFrom,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
		TLSMode:  cfg.SMTPTLS,
	})
	if err != nil {
		if !cfg.IsDev() && strings.TrimSpace(cfg.SMTPAddr) != "" {
			log.Fatalf("SMTP настроен некорректно: %v", err)
		}
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
	server := &http.Server{
		Addr: addr, Handler: handler.NewRouter(app),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second,
		MaxHeaderBytes: 16 * 1024,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("api остановлен: %v", err)
	}
}

// openStore выбирает хранилище. В production ошибки DATABASE_URL/Postgres,
// миграций или сида фатальны: молчаливый in-memory fallback означал бы потерю
// данных. В dev при пустом или недоступном DATABASE_URL разрешён in-memory
// режим с демо-данными.
func openStore(cfg *config.Config) (repo.StoreAPI, func(), error) {
	dsn := strings.TrimSpace(cfg.DatabaseURL)
	if dsn == "" {
		if !cfg.IsDev() {
			return nil, nil, fmt.Errorf("DATABASE_URL не задан: production требует Postgres")
		}
		log.Printf("ПРЕДУПРЕЖДЕНИЕ: DATABASE_URL не задан — dev-режим, in-memory без сохранения данных")
		store, closeFn := newMemoryStore(cfg)
		return store, closeFn, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	pool, err := repo.ConnectPool(ctx, dsn)
	if err != nil {
		return fallbackToMemory(cfg, fmt.Errorf("не удалось создать пул Postgres: %w", err))
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fallbackToMemory(cfg, fmt.Errorf("postgres по DATABASE_URL недоступен: %w", err))
	}

	dir, err := findMigrationsDir()
	if err != nil {
		pool.Close()
		return fallbackToMemory(cfg, err)
	}
	if err := migrate.ApplyMigrations(ctx, pool, dir); err != nil {
		pool.Close()
		return fallbackToMemory(cfg, fmt.Errorf("не удалось применить миграции: %w", err))
	}
	// Демо-учётки и курс — только dev. В production сид не применяется вовсе;
	// ApplyMigrations к тому же всегда пропускает файлы с seed в имени.
	if cfg.IsDev() {
		if err := migrate.SeedIfEmpty(ctx, pool, filepath.Join(dir, migrate.SeedFileName)); err != nil {
			pool.Close()
			return fallbackToMemory(cfg, fmt.Errorf("не удалось применить демо-сид: %w", err))
		}
	}

	// Комиссия: при доступной БД — из app_settings (ключ commission_pct), fallback — env.
	effective := migrate.GetCommissionPct(ctx, pool, cfg.CommissionPct)
	pg := repo.NewPostgresStore(pool, effective)
	log.Printf("Postgres подключён: миграции применены, хранилище — pgxpool, комиссия=%d%%", effective)
	return pg, func() { pool.Close() }, nil
}

// fallbackToMemory — единственная точка деградации до in-memory. В production
// возвращает исходную ошибку (main превращает её в log.Fatalf), в dev —
// предупреждает и отдаёт демо-хранилище.
func fallbackToMemory(cfg *config.Config, cause error) (repo.StoreAPI, func(), error) {
	if !cfg.IsDev() {
		return nil, nil, cause
	}
	log.Printf("ПРЕДУПРЕЖДЕНИЕ: %v — dev-режим, работаем в in-memory без БД", cause)
	store, closeFn := newMemoryStore(cfg)
	return store, closeFn, nil
}

// newMemoryStore создаёт dev-хранилище с демо-курсом. Вызывается только после
// проверки cfg.IsDev(): в production in-memory не используется.
func newMemoryStore(cfg *config.Config) (repo.StoreAPI, func()) {
	s := repo.NewStore()
	if err := s.SetCommissionPct(cfg.CommissionPct); err != nil {
		log.Printf("ПРЕДУПРЕЖДЕНИЕ: не удалось задать комиссию в памяти: %v", err)
	}
	seedDemo(s)
	return s, nil
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

// seedAdmin создаёт bootstrap-учётки только при явных ADMIN_* / TEACHER_*.
// Дефолтных логинов (teacher@local/teacher1 и т.п.) больше нет: без переменных
// окружения не создаётся ни одна учётная запись. В production это гарантирует
// отсутствие известных паролей «из коробки».
func seedAdmin(store repo.StoreAPI, cfg *config.Config) error {
	if err := seedConfiguredAccount(store, cfg.AdminEmail, cfg.AdminPassword, repo.RoleAdmin, "админ"); err != nil {
		return err
	}
	return seedConfiguredAccount(store, cfg.TeacherEmail, cfg.TeacherPassword, repo.RoleTeacher, "преподаватель")
}

// seedConfiguredAccount требует обе переменные — email и пароль. Если задана
// только одна, это ошибка конфигурации: молчаливый пропуск bootstrap-учётки
// оставил бы production без администратора.
func seedConfiguredAccount(store repo.StoreAPI, email, password string, role repo.Role, label string) error {
	email = strings.TrimSpace(email)
	if email == "" && password == "" {
		return nil
	}
	if email == "" || password == "" {
		return fmt.Errorf("%s: задайте и email, и пароль", label)
	}
	if err := ensureAccount(store, email, password, role, label); err != nil {
		return fmt.Errorf("%s %s: %w", label, email, err)
	}
	return nil
}

// bcryptHashLen — длина канонического bcrypt-хеша ($2a$10$ + 53 символа).
// bcrypt.Cost проверяет только префикс/стоимость и принимает мусорные хвосты
// вроде заглушки старой 002_seed.sql, поэтому длину проверяем отдельно.
const bcryptHashLen = 60

// ensureAccount создаёт учётку, если её нет. Пароль существующего
// пользователя НЕ перезаписывается, если его хеш валиден, — иначе рестарт API
// молча менял бы пароль. Исключение — невалидный bcrypt-хеш (например,
// заглушка из старой 002_seed.sql): с ним вход невозможен, поэтому он чинится
// явно заданным паролем.
//
// Роль меняется только при подтверждённом пароле (совпал с env или хеш был
// невалиден и перезаписан). Иначе публичная регистрация могла бы заранее
// занять ADMIN_EMAIL, а следующий рестарт повысил бы атакующего до админа.
func ensureAccount(store repo.StoreAPI, email, password string, role repo.Role, label string) error {
	user, exists := store.GetUserByEmail(email)
	if !exists {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("захешировать пароль: %w", err)
		}
		if _, err := store.CreateUser(email, string(hash), role); err != nil {
			return fmt.Errorf("создать пользователя: %w", err)
		}
		log.Printf("создан %s %s", label, email)
		return nil
	}

	cmpErr := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	// bcrypt.Cost — надёжная проверка формата хеша (в отличие от длины:
	// существуют легитимные 59-символьные $2$ хеши).
	_, costErr := bcrypt.Cost([]byte(user.PasswordHash))
	hashValid := costErr == nil && len(user.PasswordHash) == bcryptHashLen
	passwordConfirmed := false
	switch {
	case cmpErr == nil:
		passwordConfirmed = true
		log.Printf("%s %s уже существует — пароль не изменён", label, email)
	case !hashValid:
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("захешировать пароль для невалидного хеша: %w", err)
		}
		if !store.SetUserPasswordHash(user.ID, string(hash)) {
			return errors.New("не удалось исправить пароль: пользователь не найден")
		}
		passwordConfirmed = true
		log.Printf("исправлен невалидный хеш пароля: %s %s", label, email)
	case errors.Is(cmpErr, bcrypt.ErrMismatchedHashAndPassword):
		// Валидный пароль другого владельца не трогаем; роль ниже менять
		// нельзя, иначе это повышение чужой учётки.
		log.Printf("%s %s уже существует — пароль не изменён (пароль из env не совпадает)", label, email)
	case errors.Is(cmpErr, bcrypt.ErrPasswordTooLong):
		log.Printf("ПРЕДУПРЕЖДЕНИЕ: %s %s: пароль из env длиннее лимита bcrypt — пароль не изменён", label, email)
	default:
		return fmt.Errorf("проверить пароль: %w", cmpErr)
	}

	if user.Role != role {
		if !passwordConfirmed {
			return fmt.Errorf("роль %s→%s не изменена: пароль существующей учётки не совпадает с env", user.Role, role)
		}
		oldRole := user.Role
		if !store.SetUserRole(user.ID, role) {
			return errors.New("не удалось изменить роль: пользователь не найден")
		}
		log.Printf("исправлена роль: %s %s → %s", email, oldRole, role)
	}
	return nil
}
