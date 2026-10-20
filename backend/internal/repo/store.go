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

// ErrConflict возвращается при конфликте с целевым состоянием ресурса
// (например, отзыв приглашения не из статуса sent, T47). Handler мапит его
// на 409 — как конфликты «вне статуса» в остальном API.
var ErrConflict = errors.New("конфликт состояния записи")

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
	paymentsByProvider map[string]*Payment   // providerPaymentID -> payment
	promoCodes         map[string]*PromoCode // T63: id -> промокод
	reviews            map[string]*Review
	reviewIdx          map[string]string // studentID|courseID -> reviewID
	sessions           map[string]*Session
	// T67: сертификаты. certIdx — уникальная пара (userID|courseID),
	// certByNumber — публичный номер.
	certificates map[string]*Certificate // id -> certificate
	certIdx      map[string]string       // userID|courseID -> certificate id
	certByNumber map[string]string       // number -> certificate id
	// T71: вопросы и ответы по урокам.
	lessonQuestions map[string]*LessonQuestion // id -> вопрос
	lessonAnswers   map[string]*LessonAnswer   // id -> ответ
	lessonAnswerIDs map[string][]string        // questionID -> id ответов в порядке создания
	commissionPct   int
	promoBarEnabled bool
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
		promoCodes:         make(map[string]*PromoCode),
		reviews:            make(map[string]*Review),
		reviewIdx:          make(map[string]string),
		sessions:           make(map[string]*Session),
		certificates:       make(map[string]*Certificate),
		certIdx:            make(map[string]string),
		certByNumber:       make(map[string]string),
		lessonQuestions:    make(map[string]*LessonQuestion),
		lessonAnswers:      make(map[string]*LessonAnswer),
		lessonAnswerIDs:    make(map[string][]string),
		commissionPct:      15,
		promoBarEnabled:    true,
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
		Status:       UserActive, // T24: явный дефолт (пустая строка = active только у старых объектов)
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

// ListInvites возвращает приглашения новыми первыми с пагинацией (T47):
// createdAt DESC, page 1-based, limit 1..100 (по умолчанию 20); total —
// количество до пагинации. Наружу уходят снимки, а не живые указатели карты.
func (s *Store) ListInvites(page, limit int) ([]*TeacherInvite, int) {
	page, limit = clampPageLimit(page, limit)

	s.mu.RLock()
	defer s.mu.RUnlock()

	matched := make([]*TeacherInvite, 0, len(s.invites))
	for _, inv := range s.invites {
		matched = append(matched, inv)
	}
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].CreatedAt.After(matched[j].CreatedAt)
	})

	total := len(matched)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	items := make([]*TeacherInvite, 0, end-start)
	for _, inv := range matched[start:end] {
		snapshot := *inv
		items = append(items, &snapshot)
	}
	return items, total
}

// RevokeInvite переводит приглашение из статуса sent в revoked (T47).
// Проверка статуса и запись — одна критическая секция под мьютексом стора.
// ErrNotFound — приглашения с таким id нет; ErrConflict — статус не sent
// (accepted/expired/revoked — отзывать нечего).
func (s *Store) RevokeInvite(id string) (*TeacherInvite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, inv := range s.invites {
		if inv.ID != id {
			continue
		}
		if inv.Status != InviteSent {
			return nil, ErrConflict
		}
		inv.Status = InviteRevoked
		snapshot := *inv
		return &snapshot, nil
	}
	return nil, ErrNotFound
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
	// T65: без явной категории (или пустой) курс получает other — как
	// NOT NULL DEFAULT 'other' в миграции 009. Валидация слага — в handler.
	if strings.TrimSpace(c.Category) == "" {
		c.Category = "other"
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

// ListCatalogCourses возвращает опубликованные курсы с серверными фильтрами
// каталога (T65): q, category, price, minRating; сортировка — по f.Sort,
// вторичный ключ — createdAt по убыванию. Пагинацию применяет handler.
//
// popular — число АКТИВНЫХ записей; rating — avgRating; у minRating курсы
// без отзывов (ratingsCount = 0) в выдачу не попадают. В сравнении участвуют
// только published: остальные статусы отфильтрованы до сортировки.
func (s *Store) ListCatalogCourses(f CatalogFilter) []*Course {
	q := strings.ToLower(strings.TrimSpace(f.Q))
	category := strings.TrimSpace(f.Category)

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Число активных записей нужно только для popular — не считаем зря.
	active := make(map[string]int)
	if f.Sort == "" || f.Sort == "popular" {
		for _, e := range s.enrollments {
			if e.Status == EnrollmentActive {
				active[e.CourseID]++
			}
		}
	}

	out := make([]*Course, 0, len(s.courses))
	for _, c := range s.courses {
		if c.Status != CoursePublished {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(c.Title+" "+c.Description), q) {
			continue
		}
		if category != "" && c.Category != category {
			continue
		}
		switch f.Price {
		case "free":
			if c.PriceCents != 0 {
				continue
			}
		case "paid":
			if c.PriceCents <= 0 {
				continue
			}
		}
		if f.MinRating > 0 {
			if c.RatingsCount == 0 || c.AvgRating < f.MinRating {
				continue
			}
		}
		out = append(out, c)
	}

	sortKey := f.Sort
	if sortKey == "" {
		sortKey = "popular"
	}
	sort.SliceStable(out, func(i, j int) bool {
		return catalogCourseBefore(out[i], out[j], sortKey, active)
	})
	return out
}

// catalogCourseBefore — порядок курсов каталога (T65): первичный ключ по
// sortKey, затем createdAt по убыванию (вторичный ключ контракта) и id по
// убыванию — детерминированный порядок при полном совпадении ключей.
func catalogCourseBefore(a, b *Course, sortKey string, active map[string]int) bool {
	switch sortKey {
	case "new":
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.After(b.CreatedAt)
		}
	case "rating":
		if a.AvgRating != b.AvgRating {
			return a.AvgRating > b.AvgRating
		}
	case "price_asc":
		if a.PriceCents != b.PriceCents {
			return a.PriceCents < b.PriceCents
		}
	case "price_desc":
		if a.PriceCents != b.PriceCents {
			return a.PriceCents > b.PriceCents
		}
	default: // popular (и страховка для неизвестного значения)
		if active[a.ID] != active[b.ID] {
			return active[a.ID] > active[b.ID]
		}
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.After(b.CreatedAt)
	}
	return a.ID > b.ID
}

// CountPublishedCoursesByCategory считает published-курсы по категориям
// (T65). В карту попадают только существующие категории; отсутствие слага
// означает 0 (хендлер разворачивает фиксированный список).
func (s *Store) CountPublishedCoursesByCategory() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]int)
	for _, c := range s.courses {
		if c.Status != CoursePublished {
			continue
		}
		category := c.Category
		if category == "" {
			category = "other"
		}
		out[category]++
	}
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

// SetCourseCover записывает путь обложки курса (T54): cover != nil — новый
// путь «/uploads/covers/<файл>», nil — сброс (обложки нет). false — курса нет.
// Отдельный метод принципиален: UpdateCourse (PATCH карточки) cover_path
// не затрагивает, обложка меняется только загрузкой/удалением файла.
func (s *Store) SetCourseCover(id string, cover *string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.courses[id]
	if !ok {
		return false
	}
	c.CoverPath = cloneStringPtr(cover)
	c.UpdatedAt = time.Now().UTC()
	return true
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
	// T67: сертификаты за курс уходят каскадом (010_certificates.sql).
	s.dropCertificatesByCourseLocked(id)
	// T63: промокоды курса — зеркало ON DELETE CASCADE (008_promo_codes.sql).
	for cid, pc := range s.promoCodes {
		if pc.CourseID == id {
			delete(s.promoCodes, cid)
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

// GetPaymentByID ищет платёж по внутреннему id (T47: админский возврат
// средств принимает внутренний UUID платежа, а не providerPaymentId).
// Возвращается снимок, а не живой указатель карты: в Postgres scanPayment и
// так создаёт новую структуру, а in-memory копия защищает читателя от гонки
// с параллельной записью статуса (возврат/вебхук) — проверка гонки T47
// (-race) читает статус ВНЕ a.webhookMu, когда провайдер ещё не вызван.
func (s *Store) GetPaymentByID(id string) (*Payment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.payments[id]
	if !ok {
		return nil, false
	}
	snapshot := *p
	return &snapshot, true
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
// T47: refunded тоже терминальный статус — processedAt = момент возврата
// (контракт «Возврат средств» требует заполненный processedAt в ответе).
func (s *Store) SetPaymentStatus(id string, status PaymentStatus) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.payments[id]
	if !ok {
		return false
	}
	p.Status = status
	if status == PaymentSucceeded || status == PaymentFailed || status == PaymentRefunded {
		now := time.Now().UTC()
		p.ProcessedAt = &now
	}
	return true
}

// ListPaymentsJournal возвращает журнал платежей для админки (US6).
// Фильтры Status, CourseID, Q (подстрока по email ИЛИ displayName студента,
// регистронезависимо) и период From/To комбинируются через AND, сортировка
// created_at DESC, total — количество до пагинации, totals — суммы по всем
// отфильтрованным записям (без пагинации). Границы периода включительно:
// From — с начала суток, To — до конца суток (сравнение со следующими сутками).
// Пагинация 1-based, limit 20..100 (защитный clamp дублирует проверку на
// уровне handler). Ошибок не бывает — nil.
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
	searchQuery := strings.ToLower(f.Q)
	for _, p := range s.payments {
		if f.Status != "" && p.Status != f.Status {
			continue
		}
		if f.CourseID != "" && p.CourseID != f.CourseID {
			continue
		}
		// T31: поиск по студенту — подстрока без учёта регистра по email
		// ИЛИ displayName. Пользователя может не быть в users (платёж
		// остаётся в журнале): тогда оба поля пустые и запись не совпадёт.
		if searchQuery != "" {
			email, name := "", ""
			if u, ok := s.users[p.StudentID]; ok {
				email, name = u.Email, u.DisplayName
			}
			if !strings.Contains(strings.ToLower(email), searchQuery) &&
				!strings.Contains(strings.ToLower(name), searchQuery) {
				continue
			}
		}
		// T31: период по дате создания, границы включительно. To — «до конца
		// суток»: отсекаем всё, что не раньше следующих суток после To.
		if !f.From.IsZero() && p.CreatedAt.Before(f.From) {
			continue
		}
		if !f.To.IsZero() && !p.CreatedAt.Before(f.To.AddDate(0, 0, 1)) {
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
			// T69: отзыв теперь изменяемый (ответ преподавателя), поэтому
			// наружу уходит снимок — чтение после снятия лока не гоняется с PUT.
			snapshot := *r
			out = append(out, &snapshot)
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

// SetReviewReply устанавливает/заменяет ответ преподавателя на отзыв (T69).
// ErrNotFound — отзыва с таким id в этом курсе нет (в том числе отзыв,
// принадлежащий другому курсу). Наружу отдаётся снимок: живой указатель
// после снятия лока читать нельзя.
func (s *Store) SetReviewReply(courseID, reviewID, body string) (*Review, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.reviews[reviewID]
	if !ok || r.CourseID != courseID {
		return nil, ErrNotFound
	}
	now := time.Now().UTC()
	r.TeacherReply = body
	r.TeacherReplyAt = &now
	snapshot := *r
	return &snapshot, nil
}

// ClearReviewReply очищает ответ преподавателя (T69); отсутствие ответа —
// не ошибка (идемпотентно), ErrNotFound — только если отзыва нет в курсе.
func (s *Store) ClearReviewReply(courseID, reviewID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.reviews[reviewID]
	if !ok || r.CourseID != courseID {
		return ErrNotFound
	}
	r.TeacherReply = ""
	r.TeacherReplyAt = nil
	return nil
}

// ReviewRatingBreakdown — число отзывов курса по каждой оценке (T69).
// Ключи 1..5 присутствуют всегда, чтобы handler отдавал стабильную форму.
func (s *Store) ReviewRatingBreakdown(courseID string) map[int]int {
	out := map[int]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.reviews {
		if r.CourseID == courseID && r.Rating >= 1 && r.Rating <= 5 {
			out[r.Rating]++
		}
	}
	return out
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

// GetPromoBarEnabled возвращает состояние промо-баннера (пост-P7).
// В NewStore флаг включён: поведение «как раньше» по умолчанию.
func (s *Store) GetPromoBarEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.promoBarEnabled
}

// SetPromoBarEnabled переключает промо-баннер. In-memory запись всегда
// успешна, поэтому nil — но сигнатура совпадает со StoreAPI (см.
// SetCommissionPct).
func (s *Store) SetPromoBarEnabled(enabled bool) error {
	s.mu.Lock()
	s.promoBarEnabled = enabled
	s.mu.Unlock()
	return nil
}

// ---- T55: детали курса для админа ----

// AdminCourseDetail собирает карточку курса админа (T55) одним проходом под
// одним RLock: вложенные вызовы методов Store взяли бы RWMutex повторно
// (самоблокировка) — тот же приём, что в ListEnrollmentRows (T25). Наружу
// уходят снимки (копии значений), а не живые указатели на карты хранилища.
func (s *Store) AdminCourseDetail(courseID string) (*AdminCourseDetailData, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c, ok := s.courses[courseID]
	if !ok {
		return nil, false
	}

	d := &AdminCourseDetailData{
		Modules: []*Module{},
		Lessons: map[string][]*Lesson{},
	}

	// Email автора: справочник пользователей — та же карта под тем же локом
	// (GetUserByID взял бы лок повторно). "" — автора нет.
	if u, ok := s.users[c.TeacherID]; ok {
		d.TeacherEmail = u.Email
	}

	// Программа: модули курса по position. Counts — счётчики элемента списка
	// курсов админки (T25): модули, уроки и число АКТИВНЫХ учеников.
	for _, m := range s.modules {
		if m.CourseID != courseID {
			continue
		}
		d.Modules = append(d.Modules, m)
		d.Counts.Modules++
	}
	sort.Slice(d.Modules, func(i, j int) bool {
		return d.Modules[i].Position < d.Modules[j].Position
	})

	// Уроки каждого модуля по position: группировка от карты lessons.
	inCourse := make(map[string]bool, len(d.Modules))
	for _, m := range d.Modules {
		inCourse[m.ID] = true
	}
	for _, l := range s.lessons {
		if !inCourse[l.ModuleID] {
			continue
		}
		d.Lessons[l.ModuleID] = append(d.Lessons[l.ModuleID], l)
		d.Counts.Lessons++
	}
	for _, ls := range d.Lessons {
		sort.Slice(ls, func(i, j int) bool { return ls[i].Position < ls[j].Position })
	}

	// Снимок курса (включается поле cover_path) — живой указатель не отдаём.
	d.Course = *c

	// Статистика (T55): записи всех статусов и отдельно активные,
	// успешные платежи. Рейтинг и число отзывов берём из полей курса —
	// это тот же источник, что у публичной карточки (RecalcCourseRating
	// пересчитывает их при каждом изменении отзывов).
	for _, e := range s.enrollments {
		if e.CourseID != courseID {
			continue
		}
		d.Stats.StudentsCount++
		if e.Status == EnrollmentActive {
			d.Stats.ActiveCount++
			d.Counts.Students++
		}
	}
	d.Stats.ReviewsCount = c.RatingsCount
	d.Stats.AvgRating = c.AvgRating
	for _, p := range s.payments {
		if p.CourseID == courseID && p.Status == PaymentSucceeded {
			d.Stats.PaymentsCount++
			d.Stats.PaidTotalCents += p.AmountCents
		}
	}
	return d, true
}

// ---- T63: промокоды ----

// CreatePromoCode сохраняет промокод (T63). Дубликат (courseId, code) —
// ErrAlreadyExists: правило уникальности значений контракта (400 у handler).
// Новый код всегда активен, usedCount стартует с нуля.
func (s *Store) CreatePromoCode(p *PromoCode) (*PromoCode, error) {
	code := strings.ToUpper(strings.TrimSpace(p.Code))
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, cur := range s.promoCodes {
		if cur.CourseID == p.CourseID && cur.Code == code {
			return nil, ErrAlreadyExists
		}
	}
	pc := &PromoCode{
		ID:          uuid.NewString(),
		CourseID:    p.CourseID,
		Code:        code,
		DiscountPct: p.DiscountPct,
		MaxUses:     p.MaxUses,
		UsedCount:   0,
		ExpiresAt:   cloneTimePtr(p.ExpiresAt),
		Active:      true,
		CreatedAt:   time.Now().UTC(),
	}
	s.promoCodes[pc.ID] = pc
	*p = *pc
	return p, nil
}

// ListPromoCodesByCourse возвращает все коды курса (включая неактивные —
// история), новые первыми. При равном createdAt порядок стабилен по id —
// так же сортирует Postgres-реализация.
func (s *Store) ListPromoCodesByCourse(courseID string) []*PromoCode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*PromoCode, 0)
	for _, pc := range s.promoCodes {
		if pc.CourseID == courseID {
			out = append(out, pc)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// GetPromoCodeByID ищет промокод по id.
func (s *Store) GetPromoCodeByID(id string) (*PromoCode, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pc, ok := s.promoCodes[id]
	return pc, ok
}

// FindPromoCode ищет промокод курса по коду (без учёта регистра).
func (s *Store) FindPromoCode(courseID, code string) (*PromoCode, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, pc := range s.promoCodes {
		if pc.CourseID == courseID && pc.Code == code {
			return pc, true
		}
	}
	return nil, false
}

// DeactivatePromoCode идемпотентно выключает код (active=false), сохраняя
// запись для истории. ErrNotFound — промокода с таким id нет.
func (s *Store) DeactivatePromoCode(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pc, ok := s.promoCodes[id]
	if !ok {
		return ErrNotFound
	}
	pc.Active = false
	return nil
}

// IncrementPromoCodeUsage атомарно увеличивает usedCount на 1. false —
// промокода нет. Возврат средств счётчик не уменьшает (контракт T63).
func (s *Store) IncrementPromoCodeUsage(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	pc, ok := s.promoCodes[id]
	if !ok {
		return false
	}
	pc.UsedCount++
	return true
}

// ---- T71: вопросы и ответы по урокам ----
//
// Блок вынесен в конец файла, чтобы не конфликтовать при слиянии волн.
// Вопросы/ответы хранятся «сырыми» (только id и текст), а вычисляемые поля
// (lessonTitle, studentName, authorName, authorRole, isTeacher) собираются
// при каждом чтении — как JOIN в Postgres, поэтому смена display_name сразу
// видна в ленте. Наружу уходят снимки: живые указатели после снятия лока
// читать нельзя.

// normalizeQAPageLimit приводит пагинацию ленты вопросов к контракту P9:
// page с 1, limit 20 по умолчанию, максимум 100. Используется обеими
// реализациями стора (in-memory и Postgres) для одинакового поведения.
func normalizeQAPageLimit(page, limit int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return page, limit
}

// lessonInCourseLocked проверяет, что урок принадлежит курсу (через модуль).
// Вызывающий держит s.mu.
func (s *Store) lessonInCourseLocked(courseID, lessonID string) (*Lesson, bool) {
	l, ok := s.lessons[lessonID]
	if !ok {
		return nil, false
	}
	m, ok := s.modules[l.ModuleID]
	if !ok || m.CourseID != courseID {
		return nil, false
	}
	return l, true
}

// lessonAnswerSnapshotLocked собирает ответ с вычисляемыми полями автора.
// Вызывающий держит s.mu (RLock достаточно).
func (s *Store) lessonAnswerSnapshotLocked(a *LessonAnswer) *LessonAnswer {
	cp := *a
	if u, ok := s.users[a.AuthorID]; ok {
		cp.AuthorName = u.DisplayName
		cp.AuthorRole = string(u.Role)
		cp.IsTeacher = u.Role == RoleAdmin
	}
	if q, ok := s.lessonQuestions[a.QuestionID]; ok {
		if c, ok := s.courses[q.CourseID]; ok && c.TeacherID == a.AuthorID {
			cp.IsTeacher = true
		}
	}
	return &cp
}

// lessonQuestionSnapshotLocked собирает вопрос с заголовком урока, именем
// студента и ответами (старые первыми). Вызывающий держит s.mu (RLock).
func (s *Store) lessonQuestionSnapshotLocked(q *LessonQuestion) *LessonQuestion {
	cp := *q
	cp.Answers = make([]*LessonAnswer, 0, len(s.lessonAnswerIDs[q.ID]))
	if l, ok := s.lessons[q.LessonID]; ok {
		cp.LessonTitle = l.Title
	}
	if u, ok := s.users[q.StudentID]; ok {
		cp.StudentName = u.DisplayName
	}
	answers := make([]*LessonAnswer, 0, len(s.lessonAnswerIDs[q.ID]))
	for _, id := range s.lessonAnswerIDs[q.ID] {
		if a, ok := s.lessonAnswers[id]; ok {
			answers = append(answers, s.lessonAnswerSnapshotLocked(a))
		}
	}
	sort.Slice(answers, func(i, j int) bool {
		if answers[i].CreatedAt.Equal(answers[j].CreatedAt) {
			return answers[i].ID < answers[j].ID
		}
		return answers[i].CreatedAt.Before(answers[j].CreatedAt)
	})
	cp.Answers = answers
	return &cp
}

// CreateLessonQuestion создаёт вопрос к уроку (T71). ErrNotFound — урока нет
// или он принадлежит другому курсу.
func (s *Store) CreateLessonQuestion(courseID, lessonID, studentID, body string) (*LessonQuestion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.lessonInCourseLocked(courseID, lessonID); !ok {
		return nil, ErrNotFound
	}
	q := &LessonQuestion{
		ID:        uuid.NewString(),
		CourseID:  courseID,
		LessonID:  lessonID,
		StudentID: studentID,
		Body:      body,
		CreatedAt: time.Now().UTC(),
		Answers:   []*LessonAnswer{},
	}
	s.lessonQuestions[q.ID] = q
	return s.lessonQuestionSnapshotLocked(q), nil
}

// ListLessonQuestions возвращает вопросы курса, новые первыми; lessonID "" —
// все вопросы. Ответы внутри — старые первыми. total считается до пагинации.
func (s *Store) ListLessonQuestions(courseID, lessonID string, page, limit int) ([]*LessonQuestion, int) {
	page, limit = normalizeQAPageLimit(page, limit)
	s.mu.RLock()
	defer s.mu.RUnlock()
	matched := make([]*LessonQuestion, 0)
	for _, q := range s.lessonQuestions {
		if q.CourseID != courseID {
			continue
		}
		if lessonID != "" && q.LessonID != lessonID {
			continue
		}
		matched = append(matched, q)
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].CreatedAt.Equal(matched[j].CreatedAt) {
			return matched[i].ID > matched[j].ID
		}
		return matched[i].CreatedAt.After(matched[j].CreatedAt)
	})
	total := len(matched)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	items := make([]*LessonQuestion, 0, end-start)
	for _, q := range matched[start:end] {
		items = append(items, s.lessonQuestionSnapshotLocked(q))
	}
	return items, total
}

// GetLessonQuestion возвращает вопрос курса с ответами; false — вопроса нет
// или он принадлежит другому курсу.
func (s *Store) GetLessonQuestion(courseID, questionID string) (*LessonQuestion, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q, ok := s.lessonQuestions[questionID]
	if !ok || q.CourseID != courseID {
		return nil, false
	}
	return s.lessonQuestionSnapshotLocked(q), true
}

// CreateLessonAnswer создаёт ответ на вопрос (T71). ErrNotFound — вопроса нет
// или он принадлежит другому курсу.
func (s *Store) CreateLessonAnswer(courseID, questionID, authorID, body string) (*LessonAnswer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q, ok := s.lessonQuestions[questionID]
	if !ok || q.CourseID != courseID {
		return nil, ErrNotFound
	}
	a := &LessonAnswer{
		ID:         uuid.NewString(),
		QuestionID: questionID,
		AuthorID:   authorID,
		Body:       body,
		CreatedAt:  time.Now().UTC(),
	}
	s.lessonAnswers[a.ID] = a
	s.lessonAnswerIDs[questionID] = append(s.lessonAnswerIDs[questionID], a.ID)
	return s.lessonAnswerSnapshotLocked(a), nil
}

// GetLessonAnswer возвращает ответ вопроса курса; false — ответа нет или он
// относится к другому вопросу/курсу.
func (s *Store) GetLessonAnswer(courseID, questionID, answerID string) (*LessonAnswer, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.lessonAnswers[answerID]
	if !ok || a.QuestionID != questionID {
		return nil, false
	}
	q, ok := s.lessonQuestions[questionID]
	if !ok || q.CourseID != courseID {
		return nil, false
	}
	return s.lessonAnswerSnapshotLocked(a), true
}

// DeleteLessonQuestion удаляет вопрос; ответы уходят каскадом.
// ErrNotFound — вопроса нет в этом курсе.
func (s *Store) DeleteLessonQuestion(courseID, questionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	q, ok := s.lessonQuestions[questionID]
	if !ok || q.CourseID != courseID {
		return ErrNotFound
	}
	for _, id := range s.lessonAnswerIDs[questionID] {
		delete(s.lessonAnswers, id)
	}
	// Страховка: ответы, созданные в обход индекса, тоже не должны осиротеть.
	for id, a := range s.lessonAnswers {
		if a.QuestionID == questionID {
			delete(s.lessonAnswers, id)
		}
	}
	delete(s.lessonAnswerIDs, questionID)
	delete(s.lessonQuestions, questionID)
	return nil
}

// DeleteLessonAnswer удаляет ответ. ErrNotFound — ответа нет в этом
// вопросе/курсе.
func (s *Store) DeleteLessonAnswer(courseID, questionID, answerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.lessonAnswers[answerID]
	if !ok || a.QuestionID != questionID {
		return ErrNotFound
	}
	q, ok := s.lessonQuestions[questionID]
	if !ok || q.CourseID != courseID {
		return ErrNotFound
	}
	delete(s.lessonAnswers, answerID)
	ids := s.lessonAnswerIDs[questionID]
	for i, id := range ids {
		if id == answerID {
			s.lessonAnswerIDs[questionID] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	if len(s.lessonAnswerIDs[questionID]) == 0 {
		delete(s.lessonAnswerIDs, questionID)
	}
	return nil
}
