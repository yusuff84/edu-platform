package repo

import (
	"errors"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ErrAlreadyExists возвращается при нарушении уникальности (email, отзыв, запись).
var ErrAlreadyExists = errors.New("запись уже существует")

// Store — потокобезопасное in-memory хранилище MVP.
// Границы методов повторяют будущие pgx-репозитории.
type Store struct {
	mu                 sync.RWMutex
	users              map[string]*User
	usersByEmail       map[string]*User
	invites            map[string]*TeacherInvite // tokenHash -> invite
	courses            map[string]*Course
	modules            map[string]*Module
	lessons            map[string]*Lesson
	enrollments        map[string]*Enrollment
	enrollIdx          map[string]string          // studentID|courseID -> enrollmentID
	progress           map[string]*LessonProgress // studentID|lessonID -> progress
	payments           map[string]*Payment
	paymentsByProvider map[string]*Payment // providerPaymentID -> payment
	reviews            map[string]*Review
	reviewIdx          map[string]string // studentID|courseID -> reviewID
	sessions           map[string]*Session
	commissionPct      int
}

// NewStore создаёт пустое хранилище.
func NewStore() *Store {
	return &Store{
		users:              make(map[string]*User),
		usersByEmail:       make(map[string]*User),
		invites:            make(map[string]*TeacherInvite),
		courses:            make(map[string]*Course),
		modules:            make(map[string]*Module),
		lessons:            make(map[string]*Lesson),
		enrollments:        make(map[string]*Enrollment),
		enrollIdx:          make(map[string]string),
		progress:           make(map[string]*LessonProgress),
		payments:           make(map[string]*Payment),
		paymentsByProvider: make(map[string]*Payment),
		reviews:            make(map[string]*Review),
		reviewIdx:          make(map[string]string),
		sessions:           make(map[string]*Session),
		commissionPct:      15,
	}
}

func normEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ---- Users ----

// CreateUser создаёт пользователя; email уникален без учёта регистра.
func (s *Store) CreateUser(email, passwordHash string, role Role) (*User, error) {
	email = normEmail(email)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.usersByEmail[email]; ok {
		return nil, ErrAlreadyExists
	}
	u := &User{
		ID:           uuid.NewString(),
		Email:        email,
		PasswordHash: passwordHash,
		Role:         role,
		CreatedAt:    time.Now().UTC(),
	}
	s.users[u.ID] = u
	s.usersByEmail[email] = u
	return u, nil
}

// GetUserByEmail ищет пользователя по email.
func (s *Store) GetUserByEmail(email string) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.usersByEmail[normEmail(email)]
	return u, ok
}

// GetUserByID ищет пользователя по id.
func (s *Store) GetUserByID(id string) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[id]
	return u, ok
}

// SetUserRole меняет роль пользователя.
func (s *Store) SetUserRole(id string, role Role) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return false
	}
	u.Role = role
	return true
}

// ---- Sessions ----

// CreateSession создаёт сессию пользователя с TTL.
func (s *Store) CreateSession(userID string, ttl time.Duration) *Session {
	now := time.Now().UTC()
	sess := &Session{
		ID:        uuid.NewString(),
		UserID:    userID,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	s.mu.Lock()
	s.sessions[sess.ID] = sess
	s.mu.Unlock()
	return sess
}

// GetSession возвращает сессию, если она не истекла.
func (s *Store) GetSession(id string) (*Session, bool) {
	s.mu.RLock()
	sess, ok := s.sessions[id]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().UTC().After(sess.ExpiresAt) {
		s.mu.Lock()
		delete(s.sessions, id)
		s.mu.Unlock()
		return nil, false
	}
	return sess, true
}

// DeleteSession удаляет сессию.
func (s *Store) DeleteSession(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

// ---- Invites ----

// CreateInvite создаёт приглашение учителя, отзывая предыдущие активные на тот же email.
func (s *Store) CreateInvite(email, tokenHash, prefix, createdBy string, expiresAt time.Time) *TeacherInvite {
	email = normEmail(email)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, inv := range s.invites {
		if inv.Email == email && inv.Status == InviteSent {
			inv.Status = InviteRevoked
		}
	}
	inv := &TeacherInvite{
		ID:          uuid.NewString(),
		Email:       email,
		TokenHash:   tokenHash,
		TokenPrefix: prefix,
		Status:      InviteSent,
		ExpiresAt:   expiresAt,
		CreatedBy:   createdBy,
		CreatedAt:   time.Now().UTC(),
	}
	s.invites[tokenHash] = inv
	return inv
}

// GetInviteByTokenHash ищет приглашение по хешу токена.
func (s *Store) GetInviteByTokenHash(hash string) (*TeacherInvite, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	inv, ok := s.invites[hash]
	return inv, ok
}

// AcceptInvite помечает приглашение использованным.
func (s *Store) AcceptInvite(inv *TeacherInvite) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inv.Status = InviteAccepted
}

// ---- Courses ----

// CreateCourse создаёт курс в статусе draft.
func (s *Store) CreateCourse(c *Course) *Course {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	c.ID = uuid.NewString()
	if c.Currency == "" {
		c.Currency = "RUB"
	}
	c.Status = CourseDraft
	c.CreatedAt = now
	c.UpdatedAt = now
	s.courses[c.ID] = c
	return c
}

// GetCourseByID ищет курс по id.
func (s *Store) GetCourseByID(id string) (*Course, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.courses[id]
	return c, ok
}

// GetCourseBySlug ищет курс по slug (без учёта регистра).
func (s *Store) GetCourseBySlug(slug string) (*Course, bool) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.courses {
		if strings.ToLower(c.Slug) == slug {
			return c, true
		}
	}
	return nil, false
}

// SlugTaken проверяет занятость slug.
func (s *Store) SlugTaken(slug string) bool {
	_, ok := s.GetCourseBySlug(slug)
	return ok
}

// ListPublishedCourses возвращает опубликованные курсы с опциональным поиском.
func (s *Store) ListPublishedCourses(q string) []*Course {
	q = strings.ToLower(strings.TrimSpace(q))
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Course
	for _, c := range s.courses {
		if c.Status != CoursePublished {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(c.Title+" "+c.Description), q) {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// ListCoursesByTeacher возвращает курсы преподавателя.
func (s *Store) ListCoursesByTeacher(teacherID string) []*Course {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Course
	for _, c := range s.courses {
		if c.TeacherID == teacherID {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// ListCoursesByStatus возвращает курсы в заданном статусе.
func (s *Store) ListCoursesByStatus(status CourseStatus) []*Course {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Course
	for _, c := range s.courses {
		if c.Status == status {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// SetCourseStatus меняет статус курса (и комментарий модерации).
func (s *Store) SetCourseStatus(id string, status CourseStatus, comment string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.courses[id]
	if !ok {
		return false
	}
	c.Status = status
	c.ModerationComment = comment
	c.UpdatedAt = time.Now().UTC()
	return true
}

// TouchCourse обновляет updated_at курса.
func (s *Store) TouchCourse(id string) {
	s.mu.Lock()
	if c, ok := s.courses[id]; ok {
		c.UpdatedAt = time.Now().UTC()
	}
	s.mu.Unlock()
}

// ---- Modules ----

// CreateModule создаёт модуль в конце списка курса.
func (s *Store) CreateModule(courseID, title string) *Module {
	s.mu.Lock()
	defer s.mu.Unlock()
	pos := 1
	for _, m := range s.modules {
		if m.CourseID == courseID && m.Position >= pos {
			pos = m.Position + 1
		}
	}
	m := &Module{ID: uuid.NewString(), CourseID: courseID, Title: title, Position: pos}
	s.modules[m.ID] = m
	return m
}

// GetModule ищет модуль по id.
func (s *Store) GetModule(id string) (*Module, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.modules[id]
	return m, ok
}

// ListModulesByCourse возвращает модули курса по порядку.
func (s *Store) ListModulesByCourse(courseID string) []*Module {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Module
	for _, m := range s.modules {
		if m.CourseID == courseID {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out
}

// ---- Lessons ----

// CreateLesson создаёт урок в конце модуля.
func (s *Store) CreateLesson(l *Lesson) *Lesson {
	s.mu.Lock()
	defer s.mu.Unlock()
	pos := 1
	for _, e := range s.lessons {
		if e.ModuleID == l.ModuleID && e.Position >= pos {
			pos = e.Position + 1
		}
	}
	l.ID = uuid.NewString()
	l.Position = pos
	l.CreatedAt = time.Now().UTC()
	s.lessons[l.ID] = l
	return l
}

// GetLesson ищет урок по id.
func (s *Store) GetLesson(id string) (*Lesson, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.lessons[id]
	return l, ok
}

// ListLessonsByModule возвращает уроки модуля по порядку.
func (s *Store) ListLessonsByModule(moduleID string) []*Lesson {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Lesson
	for _, l := range s.lessons {
		if l.ModuleID == moduleID {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out
}

// ListLessonsByCourse возвращает все уроки курса в порядке (модуль, урок).
func (s *Store) ListLessonsByCourse(courseID string) []*Lesson {
	mods := s.ListModulesByCourse(courseID)
	var out []*Lesson
	for _, m := range mods {
		out = append(out, s.ListLessonsByModule(m.ID)...)
	}
	return out
}

// CountLessonsByCourse считает уроки курса.
func (s *Store) CountLessonsByCourse(courseID string) int {
	return len(s.ListLessonsByCourse(courseID))
}

// ---- Enrollments ----

func enrollKey(studentID, courseID string) string { return studentID + "|" + courseID }

// GetEnrollment ищет запись ученика на курс.
func (s *Store) GetEnrollment(studentID, courseID string) (*Enrollment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.enrollIdx[enrollKey(studentID, courseID)]
	if !ok {
		return nil, false
	}
	e, ok := s.enrollments[id]
	return e, ok
}

// EnsureEnrollment создаёт запись (идемпотентно); created=false если уже была.
func (s *Store) EnsureEnrollment(studentID, courseID string) (*Enrollment, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.enrollIdx[enrollKey(studentID, courseID)]; ok {
		return s.enrollments[id], false
	}
	e := &Enrollment{
		ID:        uuid.NewString(),
		StudentID: studentID,
		CourseID:  courseID,
		Status:    EnrollmentActive,
		CreatedAt: time.Now().UTC(),
	}
	s.enrollments[e.ID] = e
	s.enrollIdx[enrollKey(studentID, courseID)] = e.ID
	return e, true
}

// ListEnrollmentsByStudent возвращает записи ученика.
func (s *Store) ListEnrollmentsByStudent(studentID string) []*Enrollment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Enrollment
	for _, e := range s.enrollments {
		if e.StudentID == studentID {
			out = append(out, e)
		}
	}
	return out
}

// ---- Progress ----

// CompleteLesson отмечает урок пройденным (повтор увеличивает attempts).
func (s *Store) CompleteLesson(studentID, lessonID string) (*LessonProgress, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := studentID + "|" + lessonID
	if p, ok := s.progress[key]; ok {
		p.Attempts++
		if p.CompletedAt.IsZero() {
			p.CompletedAt = time.Now().UTC()
		}
		return p, false
	}
	p := &LessonProgress{
		StudentID:   studentID,
		LessonID:    lessonID,
		CompletedAt: time.Now().UTC(),
		Attempts:    1,
	}
	s.progress[key] = p
	return p, true
}

// BumpAttempt увеличивает счётчик попыток без завершения урока.
func (s *Store) BumpAttempt(studentID, lessonID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := studentID + "|" + lessonID
	if p, ok := s.progress[key]; ok {
		p.Attempts++
		return p.Attempts
	}
	s.progress[key] = &LessonProgress{StudentID: studentID, LessonID: lessonID, Attempts: 1}
	return 1
}

// IsLessonCompleted проверяет прохождение урока.
func (s *Store) IsLessonCompleted(studentID, lessonID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.progress[studentID+"|"+lessonID]
	return ok && !p.CompletedAt.IsZero()
}

// CountCompletedLessons считает пройденные уроки курса.
func (s *Store) CountCompletedLessons(studentID, courseID string) int {
	lessons := s.ListLessonsByCourse(courseID)
	n := 0
	for _, l := range lessons {
		if s.IsLessonCompleted(studentID, l.ID) {
			n++
		}
	}
	return n
}

// ---- Payments ----

// CreatePayment сохраняет платёж; providerPaymentID уникален.
func (s *Store) CreatePayment(p *Payment) (*Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ProviderPaymentID != "" {
		if _, ok := s.paymentsByProvider[p.ProviderPaymentID]; ok {
			return nil, ErrAlreadyExists
		}
	}
	p.ID = uuid.NewString()
	p.CreatedAt = time.Now().UTC()
	s.payments[p.ID] = p
	if p.ProviderPaymentID != "" {
		s.paymentsByProvider[p.ProviderPaymentID] = p
	}
	return p, nil
}

// GetPaymentByProviderID ищет платёж по внешнему id (идемпотентность webhook).
func (s *Store) GetPaymentByProviderID(id string) (*Payment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.paymentsByProvider[id]
	return p, ok
}

// FindPendingPayment ищет незавершённый платёж ученика за курс.
func (s *Store) FindPendingPayment(studentID, courseID string) (*Payment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.payments {
		if p.StudentID == studentID && p.CourseID == courseID && p.Status == PaymentPending {
			return p, true
		}
	}
	return nil, false
}

// SetPaymentStatus меняет статус платежа, проставляя processed_at.
func (s *Store) SetPaymentStatus(id string, status PaymentStatus) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.payments[id]
	if !ok {
		return false
	}
	p.Status = status
	if status == PaymentSucceeded || status == PaymentFailed {
		now := time.Now().UTC()
		p.ProcessedAt = &now
	}
	return true
}

// ---- Reviews ----

// CreateReview создаёт отзыв; один ученик — один отзыв на курс.
func (s *Store) CreateReview(studentID, courseID string, rating int, body string) (*Review, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := studentID + "|" + courseID
	if _, ok := s.reviewIdx[key]; ok {
		return nil, ErrAlreadyExists
	}
	r := &Review{
		ID:        uuid.NewString(),
		StudentID: studentID,
		CourseID:  courseID,
		Rating:    rating,
		Body:      body,
		CreatedAt: time.Now().UTC(),
	}
	s.reviews[r.ID] = r
	s.reviewIdx[key] = r.ID
	return r, nil
}

// ListReviewsByCourse возвращает отзывы курса (новые первыми).
func (s *Store) ListReviewsByCourse(courseID string) []*Review {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Review
	for _, r := range s.reviews {
		if r.CourseID == courseID {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// RecalcCourseRating пересчитывает avg_rating/ratings_count курса.
func (s *Store) RecalcCourseRating(courseID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sum, n := 0, 0
	for _, r := range s.reviews {
		if r.CourseID == courseID {
			sum += r.Rating
			n++
		}
	}
	c, ok := s.courses[courseID]
	if !ok {
		return
	}
	c.RatingsCount = n
	if n == 0 {
		c.AvgRating = 0
		return
	}
	c.AvgRating = math.Round(float64(sum)/float64(n)*10) / 10
}

// ---- Settings ----

// GetCommissionPct возвращает действующую комиссию платформы.
func (s *Store) GetCommissionPct() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.commissionPct
}

// SetCommissionPct задаёт комиссию платформы (0..90).
func (s *Store) SetCommissionPct(n int) {
	if n < 0 {
		n = 0
	}
	if n > 90 {
		n = 90
	}
	s.mu.Lock()
	s.commissionPct = n
	s.mu.Unlock()
}
