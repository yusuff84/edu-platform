package repo

import "time"

// StoreAPI — общий интерфейс хранилища для handler/service слоёв.
//
// *Store (in-memory) — dev-режим без БД.
// *PostgresStore (pgxpool) — прод-режим с реальным Postgres.
// Выбор реализации выполняется в cmd/api/main.go: если DATABASE_URL
// достижима — мигрируем и используем Postgres, иначе — fallback на in-memory
// с предупреждением в логе. Сигнатуры повторяют границы pgx-репозиториев,
// поэтому handler/service слои не зависят от конкретной реализации.
type StoreAPI interface {
	// Users
	CreateUser(email, passwordHash string, role Role) (*User, error)
	GetUserByEmail(email string) (*User, bool)
	GetUserByID(id string) (*User, bool)
	// SetUserRole реализован, но не вызывается ни одним хендлером:
	// API для будущего (админ-панель управления ролями). См. отчёт T13.
	SetUserRole(id string, role Role) bool
	// SetUserDisplayName задаёт отображаемое имя (OAuth-профиль, T13).
	SetUserDisplayName(id, name string) bool
	// SetUserPasswordHash перезаписывает хеш пароля. Нужен T16 для починки
	// поддельных bcrypt-хешей из миграции 002_seed.sql (см. ensureAccount
	// в cmd/api/main.go). false — пользователь не найден.
	SetUserPasswordHash(id, hash string) bool
	// Sessions
	CreateSession(userID string, ttl time.Duration) *Session
	GetSession(id string) (*Session, bool)
	DeleteSession(id string)
	// Invites
	CreateInvite(email, tokenHash, prefix, createdBy string, expiresAt time.Time) *TeacherInvite
	GetInviteByTokenHash(hash string) (*TeacherInvite, bool)
	AcceptInvite(inv *TeacherInvite)
	// Courses
	CreateCourse(c *Course) *Course
	GetCourseByID(id string) (*Course, bool)
	GetCourseBySlug(slug string) (*Course, bool)
	SlugTaken(slug string) bool
	ListPublishedCourses(q string) []*Course
	ListCoursesByTeacher(teacherID string) []*Course
	ListCoursesByStatus(status CourseStatus) []*Course
	SetCourseStatus(id string, status CourseStatus, comment string) bool
	TouchCourse(id string)
	// UpdateCourse — частичное обновление карточки (PATCH, T13/бэклог T06).
	UpdateCourse(id string, u CourseUpdate) bool
	// Modules
	CreateModule(courseID, title string) *Module
	GetModule(id string) (*Module, bool)
	ListModulesByCourse(courseID string) []*Module
	// UpdateModuleTitle — переименование модуля (PATCH, T13/бэклог T06).
	UpdateModuleTitle(id, title string) bool
	// Lessons
	CreateLesson(l *Lesson) *Lesson
	GetLesson(id string) (*Lesson, bool)
	ListLessonsByModule(moduleID string) []*Lesson
	ListLessonsByCourse(courseID string) []*Lesson
	CountLessonsByCourse(courseID string) int
	// Enrollments
	GetEnrollment(studentID, courseID string) (*Enrollment, bool)
	EnsureEnrollment(studentID, courseID string) (*Enrollment, bool)
	SetEnrollmentStatus(studentID, courseID string, status EnrollmentStatus) error
	ListEnrollmentsByStudent(studentID string) []*Enrollment
	// Progress
	CompleteLesson(studentID, lessonID string) (*LessonProgress, bool)
	BumpAttempt(studentID, lessonID string) int
	IsLessonCompleted(studentID, lessonID string) bool
	// CountCompletedLessons реализован, но не вызывается ни одним хендлером:
	// API для будущего (прогресс-бар «M из N уроков»). См. отчёт T13.
	CountCompletedLessons(studentID, courseID string) int
	// Payments
	CreatePayment(p *Payment) (*Payment, error)
	GetPaymentByProviderID(id string) (*Payment, bool)
	FindPendingPayment(studentID, courseID string) (*Payment, bool)
	SetPaymentStatus(id string, status PaymentStatus) bool
	ListPaymentsJournal(f PaymentFilter) (items []PaymentJournalRow, total int, totals PaymentTotals, err error)
	// Reviews
	CreateReview(studentID, courseID string, rating int, body string) (*Review, error)
	ListReviewsByCourse(courseID string) []*Review
	RecalcCourseRating(courseID string)
	// Settings
	GetCommissionPct() int
	// SetCommissionPct сохраняет комиссию (0..90) и возвращает ошибку записи
	// (T16, п.4): раньше Postgres глотал её, API отвечал 200, а в базу ничего
	// не попадало. Кэш в памяти обновляется ТОЛЬКО после успешной записи.
	SetCommissionPct(n int) error
}

// PaymentFilter — параметры журнала платежей для админки (US6).
// Пагинация 1-based: Limit по умолчанию 20, максимум 100; Page по умолчанию 1.
// Status "" и CourseID "" означают «любой»; фильтры комбинируются через AND.
type PaymentFilter struct {
	Page, Limit int
	Status      PaymentStatus
	CourseID    string
}

// PaymentJournalRow — строка журнала платежей: платёж + название курса
// (название подтягивается из courses, в in-memory — из map, в PG — через JOIN)
// и данные студента (email и отображаемое имя, T16 п.3: без email колонка
// «Студент» в админке была непригодной — там светился обрезанный UUID).
type PaymentJournalRow struct {
	Payment
	CourseTitle        string
	StudentEmail       string
	StudentDisplayName string
}

// PaymentTotals — итоговые суммы журнала по ВСЕМ записям с учётом фильтров,
// но без пагинации (контракт US6: totals в ответе GET /admin/payments).
type PaymentTotals struct {
	AmountCents        int
	PlatformFeeCents   int
	TeacherAmountCents int
}

// Проверка, что in-memory Store реализует интерфейс.
var _ StoreAPI = (*Store)(nil)
