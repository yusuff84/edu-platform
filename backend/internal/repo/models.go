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
	// Category — категория каталога (T65): один из слагов
	// programming/design/data/marketing/ai/other. Валидация — в хендлерах;
	// у старых курсов и при создании без поля — "other".
	Category string `json:"category"`
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

// Lesson — урок программы курса. IsPreview (T75) — урок открыт для
// бесплатного публичного просмотра: GET /courses/{slug}/preview/{lessonId}.
// Поле аддитивное и всегда присутствует во всех видах урока (не omitempty).
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
	IsPreview bool       `json:"isPreview"`
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
	// T63: промокод, применённый при покупке. Пустая строка — обычная
	// покупка без скидки (аддитивные поля контракта, старые платежи
	// читаются как "").
	PromoCode string `json:"promoCode"`
	// DiscountCents — размер скидки в копейках: полная цена минус
	// фактически оплаченная сумма. 0 — скидки не было.
	DiscountCents int `json:"discountCents"`
}

// PromoCode — промокод курса (T63): процентная скидка, лимит использований
// и срок действия. Код хранится и сравнивается в верхнем регистре;
// MaxUses = 0 — без ограничения. ExpiresAt = nil — без срока действия
// (в JSON отдаётся null).
type PromoCode struct {
	ID          string     `json:"id"`
	CourseID    string     `json:"courseId"`
	Code        string     `json:"code"`
	DiscountPct int        `json:"discountPct"`
	MaxUses     int        `json:"maxUses"`
	UsedCount   int        `json:"usedCount"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	Active      bool       `json:"active"`
	CreatedAt   time.Time  `json:"createdAt"`
}

type Review struct {
	ID        string    `json:"id"`
	StudentID string    `json:"studentId"`
	CourseID  string    `json:"courseId"`
	Rating    int       `json:"rating"`
	Body      string    `json:"body,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	// TeacherReply — публичный ответ преподавателя (T69). Контракт: всегда
	// строка, пустая — ответа нет (не omitempty).
	TeacherReply string `json:"teacherReply"`
	// TeacherReplyAt — время последней установки/замены ответа (T69);
	// nil — ответа нет (в JSON null). Очистка ответа сбрасывает в nil.
	TeacherReplyAt *time.Time `json:"teacherReplyAt"`
}

type Session struct {
	ID        string
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// Certificate — сертификат о завершении курса (T67). Number — публичный
// идентификатор «EDU-<ГГГГ>-<8 hex ВЕРХНИМ регистром>»; пара (UserID,
// CourseID) уникальна, поэтому повторное достижение 100% прогресса второй
// сертификат не создаёт.
type Certificate struct {
	ID       string    `json:"id"`
	UserID   string    `json:"userId"`
	CourseID string    `json:"courseId"`
	Number   string    `json:"number"`
	IssuedAt time.Time `json:"issuedAt"`
}

// CertificateListItem — элемент GET /me/certificates (T67): номер, дата
// выдачи и данные курса, новые сертификаты первыми.
type CertificateListItem struct {
	Number      string    `json:"number"`
	IssuedAt    time.Time `json:"issuedAt"`
	CourseID    string    `json:"courseId"`
	CourseTitle string    `json:"courseTitle"`
	CourseSlug  string    `json:"courseSlug"`
}

// CertificateDetails — публичная карточка GET /certificates/{number} (T67):
// StudentName — displayName студента (пустая строка, если не задан; e-mail
// наружу не отдаётся).
type CertificateDetails struct {
	Number      string    `json:"number"`
	IssuedAt    time.Time `json:"issuedAt"`
	StudentName string    `json:"studentName"`
	CourseTitle string    `json:"courseTitle"`
	CourseSlug  string    `json:"courseSlug"`
}
