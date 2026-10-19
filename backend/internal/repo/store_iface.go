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
	// ListUsers — фильтрованный список пользователей для админки (T24).
	// Q — подстрока по email/displayName (регистронезависимо), Role/Status —
	// точное совпадение, "" — любой. Новые первыми (CreatedAt desc).
	ListUsers(f UserFilter) []*User
	// SetUserStatus активен/заблокирован; false — пользователя нет (T24).
	SetUserStatus(id string, status UserStatus) bool
	// DeleteUser удаляет пользователя; ErrNotFound — нет такого (T24).
	// Каскад по сессиям/отзывам/прогрессу; курсы и платежи НЕ удаляются —
	// handler обязан отказать (409), если они есть (см. UserDependencies).
	DeleteUser(id string) error
	// UserDependencies — счётчики связей пользователя для защиты от удаления (T24).
	UserDependencies(id string) UserDeps
	// CountUsersByRole — сколько пользователей с ролью (защита последнего
	// администратора, T24).
	CountUsersByRole(role Role) int
	// Sessions
	CreateSession(userID string, ttl time.Duration) *Session
	GetSession(id string) (*Session, bool)
	DeleteSession(id string)
	// Invites
	CreateInvite(email, tokenHash, prefix, createdBy string, expiresAt time.Time) *TeacherInvite
	GetInviteByTokenHash(hash string) (*TeacherInvite, bool)
	AcceptInvite(inv *TeacherInvite)
	// ListInvites — список приглашений для админки (T47): createdAt DESC,
	// пагинация 1-based (limit 1..100, по умолчанию 20); total — до пагинации.
	ListInvites(page, limit int) ([]*TeacherInvite, int)
	// RevokeInvite отзывает приглашение из статуса sent (T47).
	// ErrNotFound — приглашения нет; ErrConflict — статус не sent.
	RevokeInvite(id string) (*TeacherInvite, error)
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
	// DeleteCourse удаляет курс и всё, что на него ссылается (уроки, записи,
	// прогресс, отзывы, платежи — каскад; T19). ErrNotFound — курса нет.
	// Ограничение «только draft» накладывает handler: каскадное удаление
	// платежей недопустимо для курсов с историей оплат.
	DeleteCourse(id string) error
	// CountContentByCourses — счётчики «модули/уроки» по курсам преподавателя
	// одним запросом (T19: убирает N+1 в GET /teacher/courses).
	CountContentByCourses(teacherID string) map[string]ContentCounts
	// CloneCourse — глубокая копия курса с модулями и уроками (T21).
	// Копия создаётся черновиком, slug подбирается по свободному (<slug>-copy-N).
	// ErrNotFound — исходного курса нет.
	CloneCourse(courseID string) (*Course, error)
	// Modules
	CreateModule(courseID, title string) *Module
	GetModule(id string) (*Module, bool)
	ListModulesByCourse(courseID string) []*Module
	// UpdateModuleTitle — переименование модуля (PATCH, T13/бэклог T06).
	UpdateModuleTitle(id, title string) bool
	// DeleteModule удаляет модуль (уроки — каскадом в Postgres; T19).
	// ErrNotFound — модуля нет. Непустые уроки проверяет handler (409).
	DeleteModule(id string) error
	// ReorderModules применяет полный список позиций модулей курса одной
	// транзакцией (T19; идемпотентность порядка).
	ReorderModules(courseID string, items []PositionItem) error
	// Lessons
	CreateLesson(l *Lesson) *Lesson
	GetLesson(id string) (*Lesson, bool)
	ListLessonsByModule(moduleID string) []*Lesson
	ListLessonsByCourse(courseID string) []*Lesson
	CountLessonsByCourse(courseID string) int
	// UpdateLesson перезаписывает контент урока по переданному уроку (PATCH,
	// T19): title/данные по kind; позицию, модуль и kind не меняет.
	// ErrNotFound — урока нет.
	UpdateLesson(l *Lesson) error
	// DeleteLesson удаляет урок и его прогресс (каскад в Postgres; T19).
	// ErrNotFound — урока нет.
	DeleteLesson(id string) error
	// ReorderLessons применяет полный список позиций уроков курса одной
	// транзакцией (T19; позиция — порядковый номер урока внутри модуля).
	ReorderLessons(courseID string, items []PositionItem) error
	// MoveLessonToModule переносит урок в конец другого модуля (T21).
	// ErrNotFound — урока или модуля нет.
	MoveLessonToModule(lessonID, moduleID string) error
	// Enrollments
	GetEnrollment(studentID, courseID string) (*Enrollment, bool)
	EnsureEnrollment(studentID, courseID string) (*Enrollment, bool)
	SetEnrollmentStatus(studentID, courseID string, status EnrollmentStatus) error
	ListEnrollmentsByStudent(studentID string) []*Enrollment
	// ListEnrollmentsByCourse — все записи на курс, новые первыми (T22).
	// Записи во всех статусах: учитель видит и отчисления (refunded, revoked),
	// статус отдаётся наружу полем.
	ListEnrollmentsByCourse(courseID string) []*Enrollment
	// Progress
	CompleteLesson(studentID, lessonID string) (*LessonProgress, bool)
	BumpAttempt(studentID, lessonID string) int
	IsLessonCompleted(studentID, lessonID string) bool
	// CountCompletedLessons реализован, но не вызывается ни одним хендлером:
	// API для будущего (прогресс-бар «M из N уроков»). См. отчёт T13.
	CountCompletedLessons(studentID, courseID string) int
	// StudentProgress — карта studentID → прогресс по курсу одним запросом (T22).
	// Ключи — только ученики, у которых есть записи на курс.
	StudentProgress(courseID string) map[string]StudentProgress
	// ListCompletedLessonIDs — id пройденных уроков курса в порядке программы (T23).
	// Никогда не nil (пустой слайс — если ничего не пройдено).
	ListCompletedLessonIDs(studentID, courseID string) []string
	// Payments
	CreatePayment(p *Payment) (*Payment, error)
	GetPaymentByProviderID(id string) (*Payment, bool)
	// GetPaymentByID — платёж по внутреннему id (T47: возврат средств).
	GetPaymentByID(id string) (*Payment, bool)
	FindPendingPayment(studentID, courseID string) (*Payment, bool)
	SetPaymentStatus(id string, status PaymentStatus) bool
	ListPaymentsJournal(f PaymentFilter) (items []PaymentJournalRow, total int, totals PaymentTotals, err error)
	// ListPaymentsByCourse — платежи по курсу; фильтрацию по статусу делает handler (T22).
	ListPaymentsByCourse(courseID string) []*Payment
	// ListPaymentsByStudent — платежи ученика, новые первыми (T23).
	ListPaymentsByStudent(studentID string) []*Payment
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

	// ---- T25: админ-консоль (сводка, курсы, отзывы, записи) ----

	// AdminOverview — агрегаты для сводки супер-админа одним вызовом (T25).
	AdminOverview() AdminTotals
	// ListAllCourses — курсы всех статусов с фильтрами для админки (T25),
	// новые первыми; total — до пагинации. T48: сортировка (Sort/Dir),
	// фильтр по email преподавателя (TeacherEmail) и период по createdAt
	// (From/To) — в CourseFilter.
	ListAllCourses(f CourseFilter) (items []*Course, total int)
	// DeleteReview удаляет отзыв и пересчитывает рейтинг курса (T25).
	// ErrNotFound — отзыва нет.
	DeleteReview(id string) error
	// ListEnrollmentRows — все записи на курсы с данными ученика и курса (T25),
	// новые первыми; total — до пагинации. T48: поиск по ученику (Q) и статус
	// записи (Status) — в EnrollmentFilter.
	ListEnrollmentRows(f EnrollmentFilter) (items []EnrollmentRow, total int)
	// CourseContentCounts — курсы → (модули, уроки, ученики) одним вызовом
	// (T25, без N+1 в списке курсов).
	CourseContentCounts() map[string]ContentCounts

	// ---- T55: детали курса для админа ----

	// AdminCourseDetail — карточка курса «одним экраном» (T55) одним вызовом:
	// курс (с cover_path) + email автора, программа (модули по position,
	// уроки каждого модуля по position — только метаданные) и статистика
	// (все записи, активные, отзывы, успешные платежи). false — курса нет.
	AdminCourseDetail(courseID string) (*AdminCourseDetailData, bool)
}

// PaymentFilter — параметры журнала платежей для админки (US6, T31).
// Пагинация 1-based: Limit по умолчанию 20, максимум 100; Page по умолчанию 1.
// Status "" и CourseID "" означают «любой»; фильтры комбинируются через AND.
//
// T31: Q — подстрока (без учёта регистра) по email ИЛИ displayName студента,
// пусто — без фильтра. From/To — границы периода по дате создания платежа
// (нулевое значение — границы нет); обе границы включительно, To — «до конца
// суток», то есть платежи дня To попадают в выборку. Total и Totals считаются
// по одному и тому же отфильтрованному набору, пагинация — после фильтрации.
type PaymentFilter struct {
	Page, Limit int
	Status      PaymentStatus
	CourseID    string
	Q           string
	From        time.Time
	To          time.Time
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

// PositionItem — пункт reorder-запроса (T19): сущность и её новая позиция.
type PositionItem struct {
	ID       string
	Position int
}

// ContentCounts — счётчики программы курса в списке «мои курсы» (T19).
// T25 добавил Students: studentsCount в списке курсов админки
// (CourseContentCounts). CountContentByCourses (T19) новое поле не
// заполняет — в «моих курсах» преподавателя оно остаётся нулевым, и
// форма ответа /teacher/courses не меняется.
type ContentCounts struct {
	Modules int
	Lessons int
	// Students — число АКТИВНЫХ учеников курса (revoked/refunded доступ
	// отозван и в счёт не идёт).
	Students int
}

// AdminCourseDetailStats — статистика курса в карточке админа (T55).
//
// StudentsCount/ActiveCount записей и PaymentsCount/PaidTotalCents берутся
// по прямым COUNT/SUM; AvgRating/ReviewsCount повторяют публичную карточку
// курса (COALESCE(avg_rating,0)/ratings_count из courses — это единый
// источник обоих срезов, пересчёт от сырых отзывов совпадает с ним,
// т.к. RecalcCourseRating выполняется на каждом изменении отзывов).
type AdminCourseDetailStats struct {
	StudentsCount int // ВСЕ записи на курс (any статус: active/refunded/revoked)
	ActiveCount   int // записи в статусе active
	ReviewsCount  int // отзывы курса (= ratings_count курса)
	AvgRating     float64
	PaymentsCount int // платежи курса в статусе succeeded
	PaidTotalCents int // сумма amount_cents успешных платежей курса
}

// AdminCourseDetailData — всё содержимое карточки курса админа (T55)
// одним вызовом хранилища: без N+1 в хендлере и без повторных взятий лока
// (in-memory) или лишних скан-запросов (Postgres).
//
// Modules/Lessons — программа курса: модули по position, уроки каждого
// модуля по position. Lessons[moduleID] — слайс для модуля; модуль без
// уроков в карту не попадает (хендлер отдаёт пустой массив lessons).
type AdminCourseDetailData struct {
	// Course — снимок курса; CoverPath внутри (колонка cover_path из
	// 001_init.sql; T54 добавляет только эндпоинты загрузки).
	Course       Course
	TeacherEmail string    // email автора; "" — автор не найден
	Modules      []*Module // по position (не nil — пустой слайс, если модулей нет)
	Lessons      map[string][]*Lesson // moduleId → уроки по position (не nil)
	// Counts — счётчики в форме элемента GET /admin/courses (T25):
	// Students — только активные записи.
	Counts ContentCounts
	// Stats — статистика карточки T55: StudentsCount — ВСЕ записи
	// (в отличие от Counts.Students), ActiveCount — только active.
	Stats AdminCourseDetailStats
}

// StudentProgress — агрегат по одному ученику курса (T22).
type StudentProgress struct {
	Completed    int       // пройдено уроков
	LastActivity time.Time // максимум CompletedAt; нулевое — не начинал
}

// UserFilter — фильтры списка пользователей в админке (T24).
// Пустые значения означают «любой»; Q ищет подстроку по email и displayName
// без учёта регистра.
type UserFilter struct {
	Q      string
	Role   Role
	Status UserStatus
}

// UserDeps — счётчики связей пользователя (T24): удаление запрещено, если
// есть курсы (пользователь — преподаватель) или платежи (история оплат).
type UserDeps struct {
	Courses     int
	Payments    int
	Enrollments int
}

// Проверка, что in-memory Store реализует интерфейс.
var _ StoreAPI = (*Store)(nil)
