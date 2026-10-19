// Package repo содержит доменные модели и in-memory реализацию хранилища.
//
// In-memory store выбран для MVP, чтобы backend собирался и работал без БД.
// Методы Store повторяют границы будущих pgx-репозиториев (см. postgres.go),
// поэтому замена на SQL не потребует менять handler/service слои.
package repo

import "time"

type Role string

const (
	RoleStudent Role = "student"
	RoleTeacher Role = "teacher"
	RoleAdmin   Role = "admin"
)

// UserStatus — состояние учётной записи (T24): active — обычный доступ,
// blocked — вход запрещён, ранее выданные сессии не работают.
// Пустая строка у старых объектов in-memory трактуется как active.
type UserStatus string

const (
	UserActive  UserStatus = "active"
	UserBlocked UserStatus = "blocked"
)

type CourseStatus string

const (
	CourseDraft     CourseStatus = "draft"
	CourseReview    CourseStatus = "review"
	CoursePublished CourseStatus = "published"
	CourseArchived  CourseStatus = "archived"
)

type LessonKind string

const (
	LessonVideoText LessonKind = "video_text"
	LessonLive      LessonKind = "live"
	LessonQuiz      LessonKind = "quiz"
	LessonCode      LessonKind = "code"
)

type EnrollmentStatus string

const (
	EnrollmentActive   EnrollmentStatus = "active"
	EnrollmentRefunded EnrollmentStatus = "refunded"
	EnrollmentRevoked  EnrollmentStatus = "revoked"
)

type PaymentStatus string

const (
	PaymentPending   PaymentStatus = "pending"
	PaymentSucceeded PaymentStatus = "succeeded"
	PaymentFailed    PaymentStatus = "failed"
	PaymentRefunded  PaymentStatus = "refunded"
)

type InviteStatus string

const (
	InviteSent     InviteStatus = "sent"
	InviteAccepted InviteStatus = "accepted"
	InviteExpired  InviteStatus = "expired"
	InviteRevoked  InviteStatus = "revoked"
)

type User struct {
	ID           string
	Email        string
	PasswordHash string
	Role         Role
	Status       UserStatus
	DisplayName  string
	CreatedAt    time.Time
}

type TeacherInvite struct {
	ID          string
	Email       string
	TokenHash   string
	TokenPrefix string
	Status      InviteStatus
	ExpiresAt   time.Time
	CreatedBy   string
	CreatedAt   time.Time
}

type Course struct {
	ID          string `json:"id"`
	TeacherID   string `json:"teacherId"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// CoverPath — публичный путь обложки «/uploads/covers/<файл>» (T54).
	// nil — обложки нет: в JSON отдаётся null (контракт волны P7:
	// coverPath string|null во всех видах курса).
	CoverPath         *string      `json:"coverPath"`
	PriceCents        int          `json:"priceCents"`
	Currency          string       `json:"currency"`
	Status            CourseStatus `json:"status"`
	AvgRating         float64      `json:"avgRating"`
	RatingsCount      int          `json:"ratingsCount"`
	ModerationComment string       `json:"moderationComment,omitempty"`
	CreatedAt         time.Time    `json:"createdAt"`
	UpdatedAt         time.Time    `json:"updatedAt"`
}

type Module struct {
	ID       string `json:"id"`
	CourseID string `json:"courseId"`
	Title    string `json:"title"`
	Position int    `json:"position"`
}

type QuizQuestion struct {
	ID          string   `json:"id"`
	Text        string   `json:"text"`
	Options     []string `json:"options"`
	Correct     int      `json:"correct"`
	Explanation string   `json:"explanation,omitempty"`
}

type QuizData struct {
	Questions []QuizQuestion `json:"questions"`
}

type CodeTest struct {
	Name     string `json:"name"`
	Input    string `json:"input"`
	Expected string `json:"expected"`
	Hidden   bool   `json:"hidden,omitempty"`
}

type CodeTask struct {
	Language string     `json:"language"`
	Template string     `json:"template,omitempty"`
	Tests    []CodeTest `json:"tests"`
}

type Lesson struct {
	ID        string     `json:"id"`
	ModuleID  string     `json:"moduleId"`
	Title     string     `json:"title"`
	Kind      LessonKind `json:"kind"`
	Position  int        `json:"position"`
	BodyMD    string     `json:"bodyMd,omitempty"`
	VideoURL  string     `json:"videoUrl,omitempty"`
	LiveURL   string     `json:"liveUrl,omitempty"`
	StartsAt  *time.Time `json:"startsAt,omitempty"`
	Quiz      *QuizData  `json:"quiz,omitempty"`
	CodeTask  *CodeTask  `json:"codeTask,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

type Enrollment struct {
	ID        string
	StudentID string
	CourseID  string
	Status    EnrollmentStatus
	CreatedAt time.Time
}

type LessonProgress struct {
	StudentID   string
	LessonID    string
	CompletedAt time.Time
	Attempts    int
}

type Payment struct {
	ID                 string
	StudentID          string
	CourseID           string
	AmountCents        int
	PlatformFeeCents   int
	TeacherAmountCents int
	Currency           string
	Provider           string
	ProviderPaymentID  string
	Status             PaymentStatus
	CreatedAt          time.Time
	ProcessedAt        *time.Time
}

type Review struct {
	ID        string    `json:"id"`
	StudentID string    `json:"studentId"`
	CourseID  string    `json:"courseId"`
	Rating    int       `json:"rating"`
	Body      string    `json:"body,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type Session struct {
	ID        string
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
}
