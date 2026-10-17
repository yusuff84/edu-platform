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

// ErrNotFound возвращается, когда запись не найдена (SetEnrollmentStatus).
var ErrNotFound = errors.New("запись не найдена")

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

// SetUserPasswordHash перезаписывает хеш пароля (починка поддельного хеша
// из миграции 002_seed.sql, T16).
func (s *Store) SetUserPasswordHash(id, hash string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return false
	}
	u.PasswordHash = hash
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

// DeleteCourse удаляет курс и всё, что на него ссылается (уроки, прогресс,
// записи, отзывы, платежи) — зеркало ON DELETE CASCADE из 001_init.sql (T19).
// Ограничение «только draft» накладывает handler: каскадное удаление
// платежей недопустимо для курсов с историей оплат.
func (s *Store) DeleteCourse(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.courses[id]; !ok {
		return ErrNotFound
	}
	delete(s.courses, id)
	mods := make(map[string]bool)
	for mid, m := range s.modules {
		if m.CourseID == id {
			mods[mid] = true
			delete(s.modules, mid)
		}
	}
	for lid, l := range s.lessons {
		if mods[l.ModuleID] {
			delete(s.lessons, lid)
			s.dropProgressLocked(lid)
		}
	}
	for key, e := range s.enrollments {
		if e.CourseID == id {
			delete(s.enrollIdx, enrollKey(e.StudentID, e.CourseID))
			delete(s.enrollments, key)
		}
	}
	for key, r := range s.reviews {
		if r.CourseID == id {
			delete(s.reviewIdx, enrollKey(r.StudentID, r.CourseID))
			delete(s.reviews, key)
		}
	}
	for pid, p := range s.payments {
		if p.CourseID == id {
			if p.ProviderPaymentID != "" {
				delete(s.paymentsByProvider, p.ProviderPaymentID)
			}
			delete(s.payments, pid)
		}
	}
	return nil
}

// CountContentByCourses считает модули и уроки по курсам преподавателя
// одним проходом по картам (T19: GET /teacher/courses без N+1).
func (s *Store) CountContentByCourses(teacherID string) map[string]ContentCounts {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]ContentCounts)
	for _, c := range s.courses {
		if c.TeacherID == teacherID {
			out[c.ID] = ContentCounts{}
		}
	}
	for _, m := range s.modules {
		if cc, ok := out[m.CourseID]; ok {
			cc.Modules++
			out[m.CourseID] = cc
		}
	}
	for _, l := range s.lessons {
		if m, ok := s.modules[l.ModuleID]; ok {
			if cc, ok := out[m.CourseID]; ok {
				cc.Lessons++
				out[m.CourseID] = cc
			}
		}
	}
	return out
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

// DeleteModule удаляет модуль вместе с его уроками и прогрессом по ним —
// зеркало ON DELETE CASCADE в Postgres (T19). ErrNotFound — модуля нет.
// Позиции оставшихся модулей НЕ уплотняются (так же ведёт себя Postgres
// без триггера): дырки в порядках допустимы, их закроет reorder.
func (s *Store) DeleteModule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.modules[id]
	if !ok {
		return ErrNotFound
	}
	delete(s.modules, id)
	for lid, l := range s.lessons {
		if l.ModuleID == id {
			delete(s.lessons, lid)
			s.dropProgressLocked(lid)
		}
	}
	return nil
}

// ReorderModules применяет полный список позиций модулей курса одной
// операцией под локом (T19: повтор с тем же списком идемпотентен).
// ErrNotFound — среди пунктов есть модуль не этого курса.
func (s *Store) ReorderModules(courseID string, items []PositionItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, it := range items {
		m, ok := s.modules[it.ID]
		if !ok || m.CourseID != courseID {
			return ErrNotFound
		}
	}
	for _, it := range items {
		s.modules[it.ID].Position = it.Position
	}
	return nil
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

// dropProgressLocked удаляет прогресс всех студентов по уроку;
// вызывать только под держащимся локом (T19).
func (s *Store) dropProgressLocked(lessonID string) {
	for key := range s.progress {
		if strings.HasSuffix(key, "|"+lessonID) {
			delete(s.progress, key)
		}
	}
}

// UpdateLesson перезаписывает контент урока (title/данные по kind); позицию,
// модуль и kind не меняет — kind фиксируется при создании (T19).
func (s *Store) UpdateLesson(l *Lesson) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.lessons[l.ID]
	if !ok {
		return ErrNotFound
	}
	cur.Title = l.Title
	cur.BodyMD = l.BodyMD
	cur.VideoURL = l.VideoURL
	cur.LiveURL = l.LiveURL
	cur.StartsAt = l.StartsAt
	cur.Quiz = l.Quiz
	cur.CodeTask = l.CodeTask
	return nil
}

// DeleteLesson удаляет урок и прогресс по нему (каскад; T19).
// ErrNotFound — урока нет.
func (s *Store) DeleteLesson(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.lessons[id]; !ok {
		return ErrNotFound
	}
	delete(s.lessons, id)
	s.dropProgressLocked(id)
	return nil
}

// ReorderLessons применяет полный список позиций уроков курса одной
// операцией под локом (T19). ErrNotFound — среди пунктов есть урок не
// этого курса.
func (s *Store) ReorderLessons(courseID string, items []PositionItem) error {
	mods := s.ListModulesByCourse(courseID)
	inCourse := make(map[string]bool, len(mods))
	for _, m := range mods {
		inCourse[m.ID] = true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, it := range items {
		l, ok := s.lessons[it.ID]
		if !ok || !inCourse[l.ModuleID] {
			return ErrNotFound
		}
	}
	for _, it := range items {
		s.lessons[it.ID].Position = it.Position
	}
	return nil
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
// T16 (п.5): вызывается только на путях УСПЕШНОГО платежа (handler/enroll.go:
// бесплатный checkout и webhook succeeded), поэтому существующая запись со
// статусом refunded/revoked переводится в active — иначе оплата прошла, а
// доступ не открылся.
func (s *Store) EnsureEnrollment(studentID, courseID string) (*Enrollment, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.enrollIdx[enrollKey(studentID, courseID)]; ok {
		e := s.enrollments[id]
		if e.Status != EnrollmentActive {
			e.Status = EnrollmentActive
		}
		return e, false
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

// SetEnrollmentStatus меняет статус записи на курсе (возврат средств →
// refunded, отзыв доступа → revoked). ErrNotFound — записи нет.
func (s *Store) SetEnrollmentStatus(studentID, courseID string, status EnrollmentStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.enrollIdx[enrollKey(studentID, courseID)]
	if !ok {
		return ErrNotFound
	}
	e, ok := s.enrollments[id]
	if !ok {
		return ErrNotFound
	}
	e.Status = status
	return nil
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

// ListPaymentsJournal возвращает журнал платежей для админки (US6).
// Фильтры Status и CourseID комбинируются через AND, сортировка created_at DESC,
// total — количество до пагинации, totals — суммы по всем отфильтрованным
// записям (без пагинации). Пагинация 1-based, limit 20..100 (защитный
// clamp дублирует проверку на уровне handler). Ошибок не бывает — nil.
func (s *Store) ListPaymentsJournal(f PaymentFilter) ([]PaymentJournalRow, int, PaymentTotals, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	page := f.Page
	if page <= 0 {
		page = 1
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	matched := make([]*Payment, 0, len(s.payments))
	for _, p := range s.payments {
		if f.Status != "" && p.Status != f.Status {
			continue
		}
		if f.CourseID != "" && p.CourseID != f.CourseID {
			continue
		}
		matched = append(matched, p)
	}
	// Новые платежи первыми.
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].CreatedAt.After(matched[j].CreatedAt)
	})

	total := len(matched)
	var totals PaymentTotals
	for _, p := range matched {
		totals.AmountCents += p.AmountCents
		totals.PlatformFeeCents += p.PlatformFeeCents
		totals.TeacherAmountCents += p.TeacherAmountCents
	}

	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	rows := make([]PaymentJournalRow, 0, end-start)
	for _, p := range matched[start:end] {
		title := ""
		if c, ok := s.courses[p.CourseID]; ok {
			title = c.Title
		}
		// T16 (п.3): email и имя студента — из таблицы пользователей.
		email, name := "", ""
		if u, ok := s.users[p.StudentID]; ok {
			email = u.Email
			name = u.DisplayName
		}
		// Копируем значение: в JSON-слой уходит снимок, а не живой указатель.
		rows = append(rows, PaymentJournalRow{
			Payment: *p, CourseTitle: title,
			StudentEmail: email, StudentDisplayName: name,
		})
	}
	return rows, total, totals, nil
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

// SetCommissionPct задаёт комиссию платформы (0..90). In-memory запись
// всегда успешна, поэтому nil — но сигнатура совпадает с StoreAPI: вызывающий
// обязан обработать ошибку (T16, п.4).
func (s *Store) SetCommissionPct(n int) error {
	if n < 0 {
		n = 0
	}
	if n > 90 {
		n = 90
	}
	s.mu.Lock()
	s.commissionPct = n
	s.mu.Unlock()
	return nil
}
