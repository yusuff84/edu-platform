package repo

// T85 — глобальная модерация Q&A (волна P10).
//
// Контракт — Task/00-api-contract.md, раздел «T85 — GET /admin/questions».
// Здесь только типы и фильтр: in-memory-реализация — store_admin_questions.go,
// SQL-ветка — postgres_admin_questions.go. Существующие LessonQuestion/
// LessonAnswer (T71) для админ-выдачи не переиспользуются: контракт требует
// email студента и автора ответа, slug/название курса и плоский набор полей
// без questionId/isTeacher, поэтому у строк админки своя форма.
//
// Типы намеренно не лежат в models.go: они относятся только к T85 и не
// участвуют в публичных формах урока.

import "time"

// AdminQuestionFilter — фильтры глобального списка Q&A для админки (T85).
//
// CourseID/LessonID — точное совпадение по UUID ("" — любой). LessonID
// используется вместе с CourseID или отдельно; урок принадлежит курсу по
// схеме, поэтому отдельной проверки в хранилище не требуется. Q — подстрока
// без учёта регистра по тексту вопроса, текстам ответов, email/именам
// студента и авторов ответов, названиям курса и урока; "" — без поиска.
// Пагинация 1-based: Page по умолчанию 1, Limit — 20, максимум 100.
type AdminQuestionFilter struct {
	CourseID string
	LessonID string
	Q        string
	Page     int
	Limit    int
}

// AdminAnswerRow — ответ в глобальном списке Q&A (T85). Авторские поля
// вычисляются из users (email, displayName, role) при каждом чтении.
type AdminAnswerRow struct {
	ID          string    `json:"id"`
	AuthorID    string    `json:"authorId"`
	AuthorEmail string    `json:"authorEmail"`
	AuthorName  string    `json:"authorName"`
	AuthorRole  string    `json:"authorRole"`
	Body        string    `json:"body"`
	CreatedAt   time.Time `json:"createdAt"`
}

// AdminQuestionRow — строка глобального списка Q&A (T85): вопрос с данными
// курса/урока/студента и ответами. Answers — всегда не-nil срез (в JSON []),
// ответы отсортированы старые первыми.
type AdminQuestionRow struct {
	ID           string            `json:"id"`
	CourseID     string            `json:"courseId"`
	CourseTitle  string            `json:"courseTitle"`
	CourseSlug   string            `json:"courseSlug"`
	LessonID     string            `json:"lessonId"`
	LessonTitle  string            `json:"lessonTitle"`
	StudentID    string            `json:"studentId"`
	StudentEmail string            `json:"studentEmail"`
	StudentName  string            `json:"studentName"`
	Body         string            `json:"body"`
	CreatedAt    time.Time         `json:"createdAt"`
	Answers      []*AdminAnswerRow `json:"answers"`
}
