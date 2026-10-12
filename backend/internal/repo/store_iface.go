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
	SetUserRole(id string, role Role) bool
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
	// Modules
	CreateModule(courseID, title string) *Module
	GetModule(id string) (*Module, bool)
	ListModulesByCourse(courseID string) []*Module
	// Lessons
	CreateLesson(l *Lesson) *Lesson
	GetLesson(id string) (*Lesson, bool)
	ListLessonsByModule(moduleID string) []*Lesson
	ListLessonsByCourse(courseID string) []*Lesson
	CountLessonsByCourse(courseID string) int
	// Enrollments
	GetEnrollment(studentID, courseID string) (*Enrollment, bool)
	EnsureEnrollment(studentID, courseID string) (*Enrollment, bool)
	ListEnrollmentsByStudent(studentID string) []*Enrollment
	// Progress
	CompleteLesson(studentID, lessonID string) (*LessonProgress, bool)
	BumpAttempt(studentID, lessonID string) int
	IsLessonCompleted(studentID, lessonID string) bool
	CountCompletedLessons(studentID, courseID string) int
	// Payments
	CreatePayment(p *Payment) (*Payment, error)
	GetPaymentByProviderID(id string) (*Payment, bool)
	FindPendingPayment(studentID, courseID string) (*Payment, bool)
	SetPaymentStatus(id string, status PaymentStatus) bool
	// Reviews
	CreateReview(studentID, courseID string, rating int, body string) (*Review, error)
	ListReviewsByCourse(courseID string) []*Review
	RecalcCourseRating(courseID string)
	// Settings
	GetCommissionPct() int
	SetCommissionPct(n int)
}

// Проверка, что in-memory Store реализует интерфейс.
var _ StoreAPI = (*Store)(nil)
