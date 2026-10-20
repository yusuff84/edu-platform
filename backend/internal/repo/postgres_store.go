package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore — SQL-реализация StoreAPI поверх pgxpool.
// Используется, когда DATABASE_URL достижима и миграции применены.
// При недоступности БД main.go оставляет in-memory Store (dev-режим).
type PostgresStore struct {
	pool *pgxpool.Pool

	mu            sync.RWMutex
	commissionPct int
	// promoBarEnabled — пост-P7, флаг промо-баннера: дефолт «включён»
	// (историческое поведение), в БД пишется только после действий админа.
	promoBarEnabled bool
}

var _ StoreAPI = (*PostgresStore)(nil)

// NewPostgresStore создаёт Postgres-реализацию с fallback-комиссией из env.
func NewPostgresStore(pool *pgxpool.Pool, fallbackCommission int) *PostgresStore {
	if fallbackCommission < 0 {
		fallbackCommission = 0
	}
	if fallbackCommission > 90 {
		fallbackCommission = 90
	}
	return &PostgresStore{pool: pool, commissionPct: fallbackCommission, promoBarEnabled: true}
}

// Pool возвращает underlying пул (для migrate/seed и graceful close).
func (s *PostgresStore) Pool() *pgxpool.Pool { return s.pool }

func (s *PostgresStore) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return true
	}
	return false
}

func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func nilIfEmpty(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

// ---- Users ----

func (s *PostgresStore) CreateUser(email, passwordHash string, role Role) (*User, error) {
	email = normEmail(email)
	ctx, cancel := s.ctx()
	defer cancel()
	// T24: статус пишем явно, а не полагаемся на DEFAULT 'active' миграции 006.
	u := &User{Email: email, PasswordHash: passwordHash, Role: role, Status: UserActive}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, role, status) VALUES ($1,$2,$3,$4)
		 RETURNING id::text, created_at`,
		email, passwordHash, string(role), string(UserActive)).Scan(&u.ID, &u.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrAlreadyExists
		}
		return nil, err
	}
	return u, nil
}

func (s *PostgresStore) GetUserByEmail(email string) (*User, bool) {
	ctx, cancel := s.ctx()
	defer cancel()
	u := &User{}
	var display string
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, email, password_hash, role, COALESCE(display_name,''), created_at, status
		 FROM users WHERE lower(email)=lower($1) LIMIT 1`, normEmail(email)).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &display, &u.CreatedAt, &u.Status)
	if err != nil {
		return nil, false
	}
	u.DisplayName = display
	return u, true
}

func (s *PostgresStore) GetUserByID(id string) (*User, bool) {
	ctx, cancel := s.ctx()
	defer cancel()
	u := &User{}
	var display string
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, email, password_hash, role, COALESCE(display_name,''), created_at, status
		 FROM users WHERE id=$1::uuid`, id).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &display, &u.CreatedAt, &u.Status)
	if err != nil {
		return nil, false
	}
	u.DisplayName = display
	return u, true
}

func (s *PostgresStore) SetUserRole(id string, role Role) bool {
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx, `UPDATE users SET role=$2 WHERE id=$1::uuid`, id, string(role))
	if err != nil {
		return false
	}
	return tag.RowsAffected() > 0
}

// SetUserPasswordHash перезаписывает хеш пароля (починка поддельного хеша
// из миграции 002_seed.sql, T16). false — пользователь не найден или БД отказала.
func (s *PostgresStore) SetUserPasswordHash(id, hash string) bool {
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx, `UPDATE users SET password_hash=$2 WHERE id=$1::uuid`, id, hash)
	if err != nil {
		log.Printf("postgres SetUserPasswordHash: %v", err)
		return false
	}
	return tag.RowsAffected() > 0
}

// ---- Sessions ----

func (s *PostgresStore) CreateSession(userID string, ttl time.Duration) *Session {
	now := time.Now().UTC()
	sess := &Session{ID: uuid.NewString(), UserID: userID, CreatedAt: now, ExpiresAt: now.Add(ttl)}
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO sessions (id, user_id, created_at, expires_at) VALUES ($1::uuid,$2::uuid,$3,$4)`,
		sess.ID, userID, sess.CreatedAt, sess.ExpiresAt)
	if err != nil {
		log.Printf("postgres CreateSession: %v", err)
	}
	return sess
}

func (s *PostgresStore) GetSession(id string) (*Session, bool) {
	ctx, cancel := s.ctx()
	defer cancel()
	sess := &Session{}
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, user_id::text, created_at, expires_at FROM sessions WHERE id=$1::uuid`, id).
		Scan(&sess.ID, &sess.UserID, &sess.CreatedAt, &sess.ExpiresAt)
	if err != nil {
		return nil, false
	}
	if time.Now().UTC().After(sess.ExpiresAt) {
		_, _ = s.pool.Exec(ctx, `DELETE FROM sessions WHERE id=$1::uuid`, id)
		return nil, false
	}
	return sess, true
}

func (s *PostgresStore) DeleteSession(id string) {
	ctx, cancel := s.ctx()
	defer cancel()
	_, _ = s.pool.Exec(ctx, `DELETE FROM sessions WHERE id=$1::uuid`, id)
}

// ---- Invites ----

func (s *PostgresStore) CreateInvite(email, tokenHash, prefix, createdBy string, expiresAt time.Time) *TeacherInvite {
	email = normEmail(email)
	now := time.Now().UTC()
	inv := &TeacherInvite{
		ID: uuid.NewString(), Email: email, TokenHash: tokenHash,
		TokenPrefix: prefix, Status: InviteSent, ExpiresAt: expiresAt,
		CreatedBy: createdBy, CreatedAt: now,
	}
	ctx, cancel := s.ctx()
	defer cancel()
	_, _ = s.pool.Exec(ctx,
		`UPDATE teacher_invites SET status='revoked' WHERE lower(email)=lower($1) AND status='sent'`, email)
	var createdAt time.Time
	var id string
	if strings.TrimSpace(createdBy) == "" {
		err := s.pool.QueryRow(ctx,
			`INSERT INTO teacher_invites (email, token_hash, token_prefix, expires_at, status, created_by)
			 VALUES ($1,$2,$3,$4,'sent',NULL) RETURNING id::text, created_at`,
			email, tokenHash, prefix, expiresAt).Scan(&id, &createdAt)
		if err != nil {
			log.Printf("postgres CreateInvite: %v", err)
			return inv
		}
	} else {
		err := s.pool.QueryRow(ctx,
			`INSERT INTO teacher_invites (email, token_hash, token_prefix, expires_at, status, created_by)
			 VALUES ($1,$2,$3,$4,'sent',$5::uuid) RETURNING id::text, created_at`,
			email, tokenHash, prefix, expiresAt, createdBy).Scan(&id, &createdAt)
		if err != nil {
			log.Printf("postgres CreateInvite: %v", err)
			return inv
		}
	}
	inv.ID = id
	inv.CreatedAt = createdAt
	return inv
}

// scanInvite собирает приглашение из строки выборки teacher_invites:
// порядок колонок — inviteCols (T47).
func scanInvite(scan func(dest ...any) error) (*TeacherInvite, error) {
	inv := &TeacherInvite{}
	var prefix string
	var createdBy *string
	if err := scan(&inv.ID, &inv.Email, &inv.TokenHash, &prefix, &inv.Status,
		&inv.ExpiresAt, &createdBy, &inv.CreatedAt); err != nil {
		return nil, err
	}
	inv.TokenPrefix = prefix
	inv.CreatedBy = strOrEmpty(createdBy)
	return inv, nil
}

// inviteCols — колонки teacher_invites для scanInvite (без token по смыслу:
// сырой токен в БД не хранится, только хеш и префикс).
const inviteCols = `id::text, email, token_hash, COALESCE(token_prefix,''), status,
	expires_at, created_by::text, created_at`

func (s *PostgresStore) GetInviteByTokenHash(hash string) (*TeacherInvite, bool) {
	ctx, cancel := s.ctx()
	defer cancel()
	row := s.pool.QueryRow(ctx,
		`SELECT `+inviteCols+` FROM teacher_invites WHERE token_hash=$1 LIMIT 1`, hash)
	inv, err := scanInvite(row.Scan)
	if err != nil {
		return nil, false
	}
	return inv, true
}

// ListInvites возвращает приглашения новыми первыми с пагинацией (T47):
// COUNT(*) — total до пагинации, страница — LIMIT/OFFSET. Ошибки БД
// логируются и отдают пустой результат — как у остальных list-методов.
func (s *PostgresStore) ListInvites(page, limit int) ([]*TeacherInvite, int) {
	page, limit = clampPageLimit(page, limit)

	ctx, cancel := s.ctx()
	defer cancel()

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM teacher_invites`).Scan(&total); err != nil {
		log.Printf("postgres ListInvites count: %v", err)
		return nil, 0
	}

	offset := (page - 1) * limit
	rows, err := s.pool.Query(ctx,
		`SELECT `+inviteCols+` FROM teacher_invites ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		limit, offset)
	if err != nil {
		log.Printf("postgres ListInvites: %v", err)
		return nil, 0
	}
	defer rows.Close()

	items := make([]*TeacherInvite, 0, limit)
	for rows.Next() {
		if inv, err := scanInvite(rows.Scan); err == nil {
			items = append(items, inv)
		}
	}
	return items, total
}

// RevokeInvite отзывает приглашение из статуса sent (T47). UPDATE с условием
// status='sent' атомарен — отзыв ровно один; RETURNING отдаёт обновлённую
// запись. Если строк не обновлено, различаем 404/409 отдельным SELECT:
// строк нет — ErrNotFound, статус был не sent — ErrConflict. Мусорный id
// (не UUID) трактуется как «не найдено» — как в GetCourseByID.
func (s *PostgresStore) RevokeInvite(id string) (*TeacherInvite, error) {
	if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
		return nil, ErrNotFound
	}
	ctx, cancel := s.ctx()
	defer cancel()

	row := s.pool.QueryRow(ctx,
		`UPDATE teacher_invites SET status='revoked'
		 WHERE id=$1::uuid AND status='sent'
		 RETURNING `+inviteCols, id)
	inv, err := scanInvite(row.Scan)
	if err == nil {
		return inv, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("не удалось отозвать приглашение: %w", err)
	}

	// Строк не обновлено: либо приглашения нет, либо статус не sent.
	var status string
	err = s.pool.QueryRow(ctx,
		`SELECT status FROM teacher_invites WHERE id=$1::uuid`, id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("не удалось проверить статус приглашения: %w", err)
	}
	return nil, ErrConflict
}

func (s *PostgresStore) AcceptInvite(inv *TeacherInvite) {
	if inv == nil {
		return
	}
	ctx, cancel := s.ctx()
	defer cancel()
	_, _ = s.pool.Exec(ctx,
		`UPDATE teacher_invites SET status='accepted' WHERE token_hash=$1`, inv.TokenHash)
	inv.Status = InviteAccepted
}

// ---- Courses ----

func scanCourse(scan func(dest ...any) error) (*Course, error) {
	c := &Course{}
	var teacherID, currency, status, comment string
	// cover_path читается как есть (NULLABLE): NULL → nil указатель → null
	// в JSON/видах (T54). COALESCE тут не нужен, пусто — это отсутствие обложки.
	var cover *string
	var avg float64
	var count int
	var created, updated time.Time
	if err := scan(&c.ID, &teacherID, &c.Slug, &c.Title, &c.Description, &c.Category, &cover,
		&c.PriceCents, &currency, &status, &avg, &count, &comment, &created, &updated); err != nil {
		return nil, err
	}
	c.TeacherID = teacherID
	if strings.TrimSpace(c.Category) == "" {
		c.Category = "other"
	}
	c.CoverPath = cover
	c.Currency = strings.TrimSpace(currency)
	if c.Currency == "" {
		c.Currency = "RUB"
	}
	c.Status = CourseStatus(status)
	c.AvgRating = avg
	c.RatingsCount = count
	c.ModerationComment = comment
	c.CreatedAt = created
	c.UpdatedAt = updated
	return c, nil
}

const courseCols = `id::text, teacher_id::text, slug, title, description, category,
	cover_path, price_cents, currency::text, status,
	COALESCE(avg_rating,0)::float8, ratings_count, COALESCE(moderation_comment,''),
	created_at, updated_at`

func (s *PostgresStore) CreateCourse(c *Course) *Course {
	now := time.Now().UTC()
	currency := strings.ToUpper(strings.TrimSpace(c.Currency))
	if currency == "" {
		currency = "RUB"
	}
	// T65: без явной категории — other, как DEFAULT миграции 009 (передаём
	// значение явно, чтобы поведение не зависело от версии схемы).
	category := strings.TrimSpace(c.Category)
	if category == "" {
		category = "other"
	}
	ctx, cancel := s.ctx()
	defer cancel()
	row := s.pool.QueryRow(ctx,
		`INSERT INTO courses (teacher_id, slug, title, description, category, price_cents, currency, status)
		 VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,'draft')
		 RETURNING `+courseCols,
		c.TeacherID, c.Slug, c.Title, c.Description, category, c.PriceCents, currency)
	if nc, err := scanCourse(row.Scan); err == nil {
		*c = *nc
		return c
	} else {
		log.Printf("postgres CreateCourse: %v", err)
		c.ID = uuid.NewString()
		c.Category = category
		c.Currency = currency
		c.Status = CourseDraft
		c.CreatedAt = now
		c.UpdatedAt = now
		return c
	}
}

func (s *PostgresStore) GetCourseByID(id string) (*Course, bool) {
	ctx, cancel := s.ctx()
	defer cancel()
	row := s.pool.QueryRow(ctx, `SELECT `+courseCols+` FROM courses WHERE id=$1::uuid`, id)
	c, err := scanCourse(row.Scan)
	if err != nil {
		return nil, false
	}
	return c, true
}

func (s *PostgresStore) GetCourseBySlug(slug string) (*Course, bool) {
	ctx, cancel := s.ctx()
	defer cancel()
	row := s.pool.QueryRow(ctx,
		`SELECT `+courseCols+` FROM courses WHERE lower(slug)=lower($1) LIMIT 1`,
		strings.ToLower(strings.TrimSpace(slug)))
	c, err := scanCourse(row.Scan)
	if err != nil {
		return nil, false
	}
	return c, true
}

func (s *PostgresStore) SlugTaken(slug string) bool {
	_, ok := s.GetCourseBySlug(slug)
	return ok
}

func (s *PostgresStore) ListPublishedCourses(q string) []*Course {
	ctx, cancel := s.ctx()
	defer cancel()
	q = strings.TrimSpace(q)
	var rows pgx.Rows
	var err error
	if q == "" {
		rows, err = s.pool.Query(ctx,
			`SELECT `+courseCols+` FROM courses WHERE status='published' ORDER BY created_at DESC`)
	} else {
		rows, err = s.pool.Query(ctx,
			`SELECT `+courseCols+` FROM courses WHERE status='published'
			 AND (title ILIKE '%'||$1||'%' OR description ILIKE '%'||$1||'%')
			 ORDER BY created_at DESC`, q)
	}
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Course
	for rows.Next() {
		if c, err := scanCourse(rows.Scan); err == nil {
			out = append(out, c)
		}
	}
	return out
}

// ListCatalogCourses возвращает опубликованные курсы с серверными фильтрами
// каталога (T65): q (title/description, ILIKE), category, price (free=0,
// paid>0), minRating (только курсы с отзывами). Сортировка — по f.Sort,
// вторичный ключ — created_at DESC, затем id DESC (детерминированный порядок
// при полном совпадении ключей). Пагинацию применяет handler, поэтому LIMIT
// в запросе нет. Значения фильтров — параметры $N; в SQL подставляется только
// выбранный из белого списка ORDER BY.
func (s *PostgresStore) ListCatalogCourses(f CatalogFilter) []*Course {
	where := "c.status='published'"
	args := make([]any, 0, 4)
	if q := strings.TrimSpace(f.Q); q != "" {
		args = append(args, q)
		where += fmt.Sprintf(" AND (c.title ILIKE '%%'||$%d||'%%' OR c.description ILIKE '%%'||$%d||'%%')",
			len(args), len(args))
	}
	if category := strings.TrimSpace(f.Category); category != "" {
		args = append(args, category)
		where += fmt.Sprintf(" AND c.category=$%d", len(args))
	}
	switch f.Price {
	case "free":
		where += " AND c.price_cents=0"
	case "paid":
		where += " AND c.price_cents>0"
	}
	if f.MinRating > 0 {
		args = append(args, f.MinRating)
		where += fmt.Sprintf(" AND c.ratings_count>0 AND COALESCE(c.avg_rating,0)>=$%d", len(args))
	}

	orderBy := `(SELECT COUNT(*) FROM enrollments e
	             WHERE e.course_id = c.id AND e.status = 'active') DESC,
	             c.created_at DESC, c.id DESC`
	switch f.Sort {
	case "new":
		orderBy = "c.created_at DESC, c.id DESC"
	case "rating":
		orderBy = "COALESCE(c.avg_rating,0) DESC, c.created_at DESC, c.id DESC"
	case "price_asc":
		orderBy = "c.price_cents ASC, c.created_at DESC, c.id DESC"
	case "price_desc":
		orderBy = "c.price_cents DESC, c.created_at DESC, c.id DESC"
	}

	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT `+courseColsJoined+` FROM courses c WHERE `+where+` ORDER BY `+orderBy, args...)
	if err != nil {
		log.Printf("postgres ListCatalogCourses: %v", err)
		return nil
	}
	defer rows.Close()
	var out []*Course
	for rows.Next() {
		if c, err := scanCourse(rows.Scan); err == nil {
			out = append(out, c)
		}
	}
	return out
}

// CountPublishedCoursesByCategory — число published-курсов по категориям
// (T65) одним GROUP BY. Слаги без курсов в карту не попадают; хендлер
// разворачивает фиксированный список и подставляет 0.
func (s *PostgresStore) CountPublishedCoursesByCategory() map[string]int {
	ctx, cancel := s.ctx()
	defer cancel()
	out := make(map[string]int)
	rows, err := s.pool.Query(ctx,
		`SELECT category, COUNT(*)::int FROM courses
		 WHERE status='published' GROUP BY category`)
	if err != nil {
		log.Printf("postgres CountPublishedCoursesByCategory: %v", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var category string
		var n int
		if err := rows.Scan(&category, &n); err == nil {
			out[category] = n
		}
	}
	return out
}

func (s *PostgresStore) ListCoursesByTeacher(teacherID string) []*Course {
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT `+courseCols+` FROM courses WHERE teacher_id=$1::uuid ORDER BY created_at DESC`, teacherID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Course
	for rows.Next() {
		if c, err := scanCourse(rows.Scan); err == nil {
			out = append(out, c)
		}
	}
	return out
}

func (s *PostgresStore) ListCoursesByStatus(status CourseStatus) []*Course {
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT `+courseCols+` FROM courses WHERE status=$1 ORDER BY created_at ASC`, string(status))
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Course
	for rows.Next() {
		if c, err := scanCourse(rows.Scan); err == nil {
			out = append(out, c)
		}
	}
	return out
}

func (s *PostgresStore) SetCourseStatus(id string, status CourseStatus, comment string) bool {
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx,
		`UPDATE courses SET status=$2, moderation_comment=$3, updated_at=now() WHERE id=$1::uuid`,
		id, string(status), comment)
	if err != nil {
		return false
	}
	return tag.RowsAffected() > 0
}

func (s *PostgresStore) TouchCourse(id string) {
	ctx, cancel := s.ctx()
	defer cancel()
	_, _ = s.pool.Exec(ctx, `UPDATE courses SET updated_at=now() WHERE id=$1::uuid`, id)
}

// ---- Modules ----

func (s *PostgresStore) CreateModule(courseID, title string) *Module {
	ctx, cancel := s.ctx()
	defer cancel()
	m := &Module{CourseID: courseID, Title: title}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO modules (course_id, title, position)
		 SELECT $1::uuid, $2, COALESCE(MAX(position),0)+1 FROM modules WHERE course_id=$1::uuid
		 RETURNING id::text, position`, courseID, title).Scan(&m.ID, &m.Position)
	if err != nil {
		log.Printf("postgres CreateModule: %v", err)
		m.ID = uuid.NewString()
		m.Position = 1
	}
	return m
}

func (s *PostgresStore) GetModule(id string) (*Module, bool) {
	ctx, cancel := s.ctx()
	defer cancel()
	m := &Module{}
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, course_id::text, title, position FROM modules WHERE id=$1::uuid`, id).
		Scan(&m.ID, &m.CourseID, &m.Title, &m.Position)
	if err != nil {
		return nil, false
	}
	return m, true
}

func (s *PostgresStore) ListModulesByCourse(courseID string) []*Module {
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, course_id::text, title, position FROM modules
		 WHERE course_id=$1::uuid ORDER BY position ASC`, courseID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Module
	for rows.Next() {
		m := &Module{}
		if err := rows.Scan(&m.ID, &m.CourseID, &m.Title, &m.Position); err == nil {
			out = append(out, m)
		}
	}
	return out
}

// ---- Lessons ----

func lessonToJSON(v any) any {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	// Проверяем указатели на пустые структуры: QuizData/CodeTask без вопросов/тестов
	// всё равно сохраняем как есть (валидация — в handler).
	return string(b)
}

func scanLesson(scan func(dest ...any) error) (*Lesson, error) {
	l := &Lesson{}
	var moduleID, title, kind, body, video, live string
	var pos int
	var starts *time.Time
	var quizRaw, codeRaw *string
	var created time.Time
	if err := scan(&l.ID, &moduleID, &title, &kind, &pos, &body, &video, &live,
		&starts, &quizRaw, &codeRaw, &created, &l.IsPreview); err != nil {
		return nil, err
	}
	l.ModuleID = moduleID
	l.Title = title
	l.Kind = LessonKind(kind)
	l.Position = pos
	l.BodyMD = body
	l.VideoURL = video
	l.LiveURL = live
	l.StartsAt = starts
	l.CreatedAt = created
	if quizRaw != nil && strings.TrimSpace(*quizRaw) != "" {
		var qd QuizData
		if err := json.Unmarshal([]byte(*quizRaw), &qd); err == nil {
			l.Quiz = &qd
		}
	}
	if codeRaw != nil && strings.TrimSpace(*codeRaw) != "" {
		var ct CodeTask
		if err := json.Unmarshal([]byte(*codeRaw), &ct); err == nil {
			l.CodeTask = &ct
		}
	}
	return l, nil
}

const lessonCols = `l.id::text, l.module_id::text, l.title, l.kind, l.position,
	COALESCE(l.body_md,''), COALESCE(l.video_url,''), COALESCE(l.live_url,''),
	l.starts_at, l.quiz::text, l.code_task::text, l.created_at, l.is_preview`

func (s *PostgresStore) CreateLesson(l *Lesson) *Lesson {
	ctx, cancel := s.ctx()
	defer cancel()
	var quiz, code any = lessonToJSON(l.Quiz), lessonToJSON(l.CodeTask)
	var body, video, live any = nilIfEmpty(l.BodyMD), nilIfEmpty(l.VideoURL), nilIfEmpty(l.LiveURL)
	var starts any
	if l.StartsAt != nil {
		starts = *l.StartsAt
	}
	nl := &Lesson{
		ModuleID: l.ModuleID, Title: l.Title, Kind: l.Kind,
		BodyMD: l.BodyMD, VideoURL: l.VideoURL, LiveURL: l.LiveURL,
		StartsAt: l.StartsAt, Quiz: l.Quiz, CodeTask: l.CodeTask,
		IsPreview: l.IsPreview,
	}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO lessons (module_id, title, kind, position, body_md, video_url, live_url, starts_at, quiz, code_task, is_preview)
		 SELECT $1::uuid, $2, $3, COALESCE(MAX(position),0)+1, $4, $5, $6, $7, $8::jsonb, $9::jsonb, $10
		 FROM lessons WHERE module_id=$1::uuid
		 RETURNING id::text, position, created_at`,
		l.ModuleID, l.Title, string(l.Kind), body, video, live, starts, quiz, code, l.IsPreview).
		Scan(&nl.ID, &nl.Position, &nl.CreatedAt)
	if err != nil {
		log.Printf("postgres CreateLesson: %v", err)
		nl.ID = uuid.NewString()
		nl.Position = 1
		nl.CreatedAt = time.Now().UTC()
	}
	*l = *nl
	return l
}

func (s *PostgresStore) GetLesson(id string) (*Lesson, bool) {
	ctx, cancel := s.ctx()
	defer cancel()
	row := s.pool.QueryRow(ctx, `SELECT `+lessonCols+` FROM lessons l WHERE l.id=$1::uuid`, id)
	l, err := scanLesson(row.Scan)
	if err != nil {
		return nil, false
	}
	return l, true
}

func (s *PostgresStore) ListLessonsByModule(moduleID string) []*Lesson {
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT `+lessonCols+` FROM lessons l WHERE l.module_id=$1::uuid ORDER BY l.position ASC`, moduleID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Lesson
	for rows.Next() {
		if l, err := scanLesson(rows.Scan); err == nil {
			out = append(out, l)
		}
	}
	return out
}

func (s *PostgresStore) ListLessonsByCourse(courseID string) []*Lesson {
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT `+lessonCols+` FROM lessons l JOIN modules m ON m.id=l.module_id
		 WHERE m.course_id=$1::uuid ORDER BY m.position ASC, l.position ASC`, courseID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Lesson
	for rows.Next() {
		if l, err := scanLesson(rows.Scan); err == nil {
			out = append(out, l)
		}
	}
	return out
}

func (s *PostgresStore) CountLessonsByCourse(courseID string) int {
	ctx, cancel := s.ctx()
	defer cancel()
	var n int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM lessons l JOIN modules m ON m.id=l.module_id WHERE m.course_id=$1::uuid`,
		courseID).Scan(&n); err != nil {
		return 0
	}
	return n
}

// ---- Enrollments ----

func scanEnrollment(scan func(dest ...any) error) (*Enrollment, error) {
	e := &Enrollment{}
	var status string
	if err := scan(&e.ID, &e.StudentID, &e.CourseID, &status, &e.CreatedAt); err != nil {
		return nil, err
	}
	e.Status = EnrollmentStatus(status)
	return e, nil
}

const enrollCols = `id::text, student_id::text, course_id::text, status, created_at`

func (s *PostgresStore) GetEnrollment(studentID, courseID string) (*Enrollment, bool) {
	ctx, cancel := s.ctx()
	defer cancel()
	row := s.pool.QueryRow(ctx,
		`SELECT `+enrollCols+` FROM enrollments WHERE student_id=$1::uuid AND course_id=$2::uuid LIMIT 1`,
		studentID, courseID)
	e, err := scanEnrollment(row.Scan)
	if err != nil {
		return nil, false
	}
	return e, true
}

// EnsureEnrollment создаёт запись (идемпотентно); created=false, если уже была.
// T16 (п.5): вызывается только на путях УСПЕШНОГО платежа (handler/enroll.go),
// поэтому существующая запись со статусом refunded/revoked переводится в
// active — иначе оплата прошла, а доступ не открылся. Семантика created
// («запись только что создана») сохранена как у in-memory Store.
func (s *PostgresStore) EnsureEnrollment(studentID, courseID string) (*Enrollment, bool) {
	ctx, cancel := s.ctx()
	defer cancel()
	row := s.pool.QueryRow(ctx,
		`INSERT INTO enrollments (student_id, course_id, status) VALUES ($1::uuid,$2::uuid,'active')
		 ON CONFLICT (student_id, course_id) DO NOTHING
		 RETURNING `+enrollCols, studentID, courseID)
	if e, err := scanEnrollment(row.Scan); err == nil {
		return e, true
	} else if !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("postgres EnsureEnrollment insert: %v", err)
	}
	e, ok := s.GetEnrollment(studentID, courseID)
	if !ok {
		now := time.Now().UTC()
		return &Enrollment{ID: uuid.NewString(), StudentID: studentID, CourseID: courseID,
			Status: EnrollmentActive, CreatedAt: now}, false
	}
	if e.Status != EnrollmentActive {
		if err := s.SetEnrollmentStatus(studentID, courseID, EnrollmentActive); err != nil {
			log.Printf("postgres EnsureEnrollment: не удалось открыть доступ: %v", err)
			return e, false
		}
		e.Status = EnrollmentActive
	}
	return e, false
}

// SetEnrollmentStatus меняет статус записи на курсе (возврат средств →
// refunded, отзыв доступа → revoked). Ошибки БД НЕ глотаются: ErrNotFound —
// записи ученика на курс нет, иначе — ошибка pgx.
func (s *PostgresStore) SetEnrollmentStatus(studentID, courseID string, status EnrollmentStatus) error {
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx,
		`UPDATE enrollments SET status=$3
		 WHERE student_id=$1::uuid AND course_id=$2::uuid`,
		studentID, courseID, string(status))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) ListEnrollmentsByStudent(studentID string) []*Enrollment {
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT `+enrollCols+` FROM enrollments WHERE student_id=$1::uuid`, studentID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Enrollment
	for rows.Next() {
		if e, err := scanEnrollment(rows.Scan); err == nil {
			out = append(out, e)
		}
	}
	return out
}

// ---- Progress ----

func (s *PostgresStore) CompleteLesson(studentID, lessonID string) (*LessonProgress, bool) {
	ctx, cancel := s.ctx()
	defer cancel()
	var completed *time.Time
	var attempts int
	err := s.pool.QueryRow(ctx,
		`SELECT completed_at, attempts FROM lesson_progress
		 WHERE student_id=$1::uuid AND lesson_id=$2::uuid`,
		studentID, lessonID).Scan(&completed, &attempts)
	now := time.Now().UTC()
	if err == nil {
		attempts++
		if completed == nil {
			completed = &now
		}
		_, _ = s.pool.Exec(ctx,
			`UPDATE lesson_progress SET attempts=$3, completed_at=$4
			 WHERE student_id=$1::uuid AND lesson_id=$2::uuid`,
			studentID, lessonID, attempts, *completed)
		return &LessonProgress{StudentID: studentID, LessonID: lessonID,
			CompletedAt: *completed, Attempts: attempts}, false
	}
	p := &LessonProgress{StudentID: studentID, LessonID: lessonID, CompletedAt: now, Attempts: 1}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO lesson_progress (student_id, lesson_id, completed_at, attempts)
		 VALUES ($1::uuid,$2::uuid,$3,1) ON CONFLICT (student_id, lesson_id)
		 DO UPDATE SET attempts=lesson_progress.attempts+1,
		 completed_at=COALESCE(lesson_progress.completed_at, EXCLUDED.completed_at)`,
		studentID, lessonID, now)
	if err != nil {
		log.Printf("postgres CompleteLesson: %v", err)
	}
	return p, true
}

func (s *PostgresStore) BumpAttempt(studentID, lessonID string) int {
	ctx, cancel := s.ctx()
	defer cancel()
	var attempts int
	err := s.pool.QueryRow(ctx,
		`SELECT attempts FROM lesson_progress WHERE student_id=$1::uuid AND lesson_id=$2::uuid`,
		studentID, lessonID).Scan(&attempts)
	if err == nil {
		attempts++
		_, _ = s.pool.Exec(ctx,
			`UPDATE lesson_progress SET attempts=$3 WHERE student_id=$1::uuid AND lesson_id=$2::uuid`,
			studentID, lessonID, attempts)
		return attempts
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO lesson_progress (student_id, lesson_id, completed_at, attempts)
		 VALUES ($1::uuid,$2::uuid,NULL,1) ON CONFLICT (student_id, lesson_id)
		 DO UPDATE SET attempts=lesson_progress.attempts+1`,
		studentID, lessonID)
	if err != nil {
		log.Printf("postgres BumpAttempt: %v", err)
		return 1
	}
	return 1
}

func (s *PostgresStore) IsLessonCompleted(studentID, lessonID string) bool {
	ctx, cancel := s.ctx()
	defer cancel()
	var completed *time.Time
	if err := s.pool.QueryRow(ctx,
		`SELECT completed_at FROM lesson_progress WHERE student_id=$1::uuid AND lesson_id=$2::uuid`,
		studentID, lessonID).Scan(&completed); err != nil {
		return false
	}
	return completed != nil
}

func (s *PostgresStore) CountCompletedLessons(studentID, courseID string) int {
	ctx, cancel := s.ctx()
	defer cancel()
	var n int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM lesson_progress lp
		 JOIN lessons l ON l.id=lp.lesson_id JOIN modules m ON m.id=l.module_id
		 WHERE lp.student_id=$1::uuid AND m.course_id=$2::uuid AND lp.completed_at IS NOT NULL`,
		studentID, courseID).Scan(&n); err != nil {
		return 0
	}
	return n
}

// ---- Payments ----

func scanPayment(scan func(dest ...any) error) (*Payment, error) {
	p := &Payment{}
	var studentID, courseID, currency, provider, providerID, status string
	var processed *time.Time
	var promoCode string
	var discountCents int
	if err := scan(&p.ID, &studentID, &courseID, &p.AmountCents, &p.PlatformFeeCents,
		&p.TeacherAmountCents, &currency, &provider, &providerID, &status,
		&p.CreatedAt, &processed, &promoCode, &discountCents); err != nil {
		return nil, err
	}
	p.StudentID = studentID
	p.CourseID = courseID
	p.Currency = strings.TrimSpace(currency)
	p.Provider = provider
	p.ProviderPaymentID = providerID
	p.Status = PaymentStatus(status)
	p.ProcessedAt = processed
	p.PromoCode = promoCode
	p.DiscountCents = discountCents
	return p, nil
}

// paymentCols — порядок колонок payments для scanPayment. T63: в конец
// добавлены promo_code и discount_cents (аддитивные поля контракта).
const paymentCols = `id::text, student_id::text, course_id::text, amount_cents, platform_fee_cents,
	teacher_amount_cents, currency::text, provider, COALESCE(provider_payment_id,''),
	status, created_at, processed_at, COALESCE(promo_code,''), discount_cents`

// paymentColsP — то же, что paymentCols, но с алиасом таблицы p
// (для JOIN-запросов журнала платежей).
const paymentColsP = `p.id::text, p.student_id::text, p.course_id::text, p.amount_cents, p.platform_fee_cents,
	p.teacher_amount_cents, p.currency::text, p.provider, COALESCE(p.provider_payment_id,''),
	p.status, p.created_at, p.processed_at, COALESCE(p.promo_code,''), p.discount_cents`

func (s *PostgresStore) CreatePayment(p *Payment) (*Payment, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	currency := strings.TrimSpace(p.Currency)
	if currency == "" {
		currency = "RUB"
	}
	var providerID any
	if strings.TrimSpace(p.ProviderPaymentID) == "" {
		providerID = nil
	} else {
		providerID = p.ProviderPaymentID
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO payments (student_id, course_id, amount_cents, platform_fee_cents,
		  teacher_amount_cents, currency, provider, provider_payment_id, status,
		  promo_code, discount_cents)
		 VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 RETURNING `+paymentCols,
		p.StudentID, p.CourseID, p.AmountCents, p.PlatformFeeCents, p.TeacherAmountCents,
		currency, p.Provider, providerID, string(p.Status), p.PromoCode, p.DiscountCents)
	if res, err := scanPayment(row.Scan); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrAlreadyExists
		}
		return nil, err
	} else {
		*p = *res
		return p, nil
	}
}

func (s *PostgresStore) GetPaymentByProviderID(id string) (*Payment, bool) {
	ctx, cancel := s.ctx()
	defer cancel()
	row := s.pool.QueryRow(ctx,
		`SELECT `+paymentCols+` FROM payments WHERE provider_payment_id=$1 LIMIT 1`, id)
	p, err := scanPayment(row.Scan)
	if err != nil {
		return nil, false
	}
	return p, true
}

// GetPaymentByID ищет платёж по внутреннему id (T47: админский возврат
// средств принимает внутренний UUID платежа). Мусорный id (не UUID) даёт
// ошибку парсинга и трактуется как «не найдено» — как в GetCourseByID.
func (s *PostgresStore) GetPaymentByID(id string) (*Payment, bool) {
	ctx, cancel := s.ctx()
	defer cancel()
	row := s.pool.QueryRow(ctx,
		`SELECT `+paymentCols+` FROM payments WHERE id=$1::uuid`, id)
	p, err := scanPayment(row.Scan)
	if err != nil {
		return nil, false
	}
	return p, true
}

func (s *PostgresStore) FindPendingPayment(studentID, courseID string) (*Payment, bool) {
	ctx, cancel := s.ctx()
	defer cancel()
	row := s.pool.QueryRow(ctx,
		`SELECT `+paymentCols+` FROM payments
		 WHERE student_id=$1::uuid AND course_id=$2::uuid AND status='pending'
		 ORDER BY created_at ASC LIMIT 1`, studentID, courseID)
	p, err := scanPayment(row.Scan)
	if err != nil {
		return nil, false
	}
	return p, true
}

func (s *PostgresStore) SetPaymentStatus(id string, status PaymentStatus) bool {
	ctx, cancel := s.ctx()
	defer cancel()
	// T47: refunded — тоже терминальный статус, processed_at фиксирует
	// момент возврата (контракт требует заполненный processedAt в ответе
	// POST /admin/payments/{id}/refund).
	tag, err := s.pool.Exec(ctx,
		`UPDATE payments SET status=$2,
		 processed_at=CASE WHEN $2 IN ('succeeded','failed','refunded') THEN now() ELSE processed_at END
		 WHERE id=$1::uuid`, id, string(status))
	if err != nil {
		return false
	}
	return tag.RowsAffected() > 0
}

// ListPaymentsJournal возвращает журнал платежей для админки (US6).
// В отличие от большинства методов файла ошибки БД НЕ глотаются, а
// пробрасываются вызывающему (handler отвечает 500). Фильтры status,
// courseId и T31-поиск/период (q — подстрока по email/displayName студента,
// from/to — границы даты создания включительно) комбинируются через AND,
// сортировка created_at DESC; total — количество до пагинации, totals — SUM
// по всем отфильтрованным записям без пагинации. Все новые условия идут
// через тот же динамический WHERE и общий список args, поэтому нумерация
// $N не сбивается, а агрегат и страница строк видят одинаковый набор.
func (s *PostgresStore) ListPaymentsJournal(f PaymentFilter) ([]PaymentJournalRow, int, PaymentTotals, error) {
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

	// Динамический WHERE: значения идут параметрами через $N (без конкатенации
	// пользовательского ввода в SQL), LIMIT/OFFSET — клампнутые int.
	// Условия по периоду: from — «с начала суток», to — «до конца суток»
	// (created_at < следующих суток после to), поэтому границы включительно.
	where := ""
	args := make([]any, 0, 5)
	if f.Status != "" {
		args = append(args, string(f.Status))
		where += fmt.Sprintf(" AND p.status=$%d", len(args))
	}
	if f.CourseID != "" {
		args = append(args, f.CourseID)
		where += fmt.Sprintf(" AND p.course_id=$%d::uuid", len(args))
	}
	if f.Q != "" {
		// Один и тот же плейсхолдер в обоих LIKE: параметр — одна подстрока
		// в нижнем регистре (email хранится как есть, поэтому lower(email)).
		args = append(args, "%"+strings.ToLower(f.Q)+"%")
		where += fmt.Sprintf(" AND (lower(u.email) LIKE $%d OR lower(coalesce(u.display_name,'')) LIKE $%d)",
			len(args), len(args))
	}
	if !f.From.IsZero() {
		args = append(args, f.From)
		where += fmt.Sprintf(" AND p.created_at >= $%d", len(args))
	}
	if !f.To.IsZero() {
		args = append(args, f.To.AddDate(0, 0, 1))
		where += fmt.Sprintf(" AND p.created_at < $%d", len(args))
	}

	ctx, cancel := s.ctx()
	defer cancel()

	// Итоги и count — одним агрегатом по всем записям с учётом фильтров.
	// LEFT JOIN users нужен для условий по q (u.email/u.display_name):
	// строки от него не размножаются (u.id — первичный ключ), а набор для
	// агрегата совпадает с набором страницы строк ниже.
	var totals PaymentTotals
	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(SUM(p.amount_cents),0),
		 COALESCE(SUM(p.platform_fee_cents),0), COALESCE(SUM(p.teacher_amount_cents),0)
		 FROM payments p LEFT JOIN users u ON u.id = p.student_id WHERE 1=1`+where, args...).
		Scan(&total, &totals.AmountCents, &totals.PlatformFeeCents, &totals.TeacherAmountCents); err != nil {
		return nil, 0, PaymentTotals{}, err
	}

	// Страница строк: платежи + название курса + данные студента (T16, п.3).
	// LEFT JOIN users: строка журнала не должна исчезнуть из-за пользователя,
	// иначе total/ totals из агрегата разошлись бы со списком.
	offset := (page - 1) * limit
	rows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s, c.title, COALESCE(u.email,''), COALESCE(u.display_name,'') FROM payments p
		 JOIN courses c ON c.id = p.course_id
		 LEFT JOIN users u ON u.id = p.student_id
		 WHERE 1=1%s ORDER BY p.created_at DESC LIMIT %d OFFSET %d`,
			paymentColsP, where, limit, offset), args...)
	if err != nil {
		return nil, 0, PaymentTotals{}, err
	}
	defer rows.Close()

	items := make([]PaymentJournalRow, 0, limit)
	for rows.Next() {
		var title, studentEmail, studentName string
		// Переиспользуем scanPayment: дописываем title/email/name последними.
		p, err := scanPayment(func(dest ...any) error {
			return rows.Scan(append(dest, &title, &studentEmail, &studentName)...)
		})
		if err != nil {
			return nil, 0, PaymentTotals{}, err
		}
		items = append(items, PaymentJournalRow{
			Payment: *p, CourseTitle: title,
			StudentEmail: studentEmail, StudentDisplayName: studentName,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, PaymentTotals{}, err
	}
	return items, total, totals, nil
}

// ---- Reviews ----

func (s *PostgresStore) CreateReview(studentID, courseID string, rating int, body string) (*Review, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	r := &Review{StudentID: studentID, CourseID: courseID, Rating: rating, Body: body}
	var bodyParam any = nilIfEmpty(body)
	err := s.pool.QueryRow(ctx,
		`INSERT INTO reviews (student_id, course_id, rating, body) VALUES ($1::uuid,$2::uuid,$3,$4)
		 RETURNING id::text, created_at`, studentID, courseID, rating, bodyParam).
		Scan(&r.ID, &r.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrAlreadyExists
		}
		return nil, err
	}
	return r, nil
}

// reviewCols — колонки Review в форме контракта T69 (teacherReply и
// teacherReplyAt — аддитивные поля; NULL времени читается как nil → null).
const reviewCols = `id::text, student_id::text, course_id::text, rating,
	COALESCE(body,''), created_at, teacher_reply, teacher_reply_at`

// scanReview собирает Review из строки выборки reviewCols.
func scanReview(scan func(dest ...any) error) (*Review, error) {
	r := &Review{}
	var replyAt *time.Time
	if err := scan(&r.ID, &r.StudentID, &r.CourseID, &r.Rating, &r.Body,
		&r.CreatedAt, &r.TeacherReply, &replyAt); err != nil {
		return nil, err
	}
	r.TeacherReplyAt = replyAt
	return r, nil
}

func (s *PostgresStore) ListReviewsByCourse(courseID string) []*Review {
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT `+reviewCols+` FROM reviews WHERE course_id=$1::uuid ORDER BY created_at DESC`, courseID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Review
	for rows.Next() {
		if r, err := scanReview(rows.Scan); err == nil {
			out = append(out, r)
		}
	}
	return out
}

func (s *PostgresStore) RecalcCourseRating(courseID string) {
	ctx, cancel := s.ctx()
	defer cancel()
	var avg float64
	var count int
	_ = s.pool.QueryRow(ctx,
		`SELECT COALESCE(ROUND(AVG(rating)::numeric,1),0)::float8, COUNT(*)
		 FROM reviews WHERE course_id=$1::uuid`, courseID).Scan(&avg, &count)
	_, _ = s.pool.Exec(ctx,
		`UPDATE courses SET avg_rating=$2, ratings_count=$3, updated_at=now() WHERE id=$1::uuid`,
		courseID, avg, count)
}

// SetReviewReply устанавливает/заменяет ответ преподавателя на отзыв (T69).
// Мусорный id (не UUID) трактуется как «отзыва нет» — как в DeleteReview,
// чтобы не отдавать 500 на кривом значении пути.
func (s *PostgresStore) SetReviewReply(courseID, reviewID, body string) (*Review, error) {
	if !validUUID(reviewID) || !validUUID(courseID) {
		return nil, ErrNotFound
	}
	ctx, cancel := s.ctx()
	defer cancel()
	row := s.pool.QueryRow(ctx,
		`UPDATE reviews SET teacher_reply=$3, teacher_reply_at=now()
		 WHERE id=$1::uuid AND course_id=$2::uuid
		 RETURNING `+reviewCols, reviewID, courseID, body)
	r, err := scanReview(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("не удалось сохранить ответ на отзыв: %w", err)
	}
	return r, nil
}

// ClearReviewReply очищает ответ преподавателя (T69): пустая строка и NULL
// времени. Повторная очистка — успех; ErrNotFound только если отзыва нет.
func (s *PostgresStore) ClearReviewReply(courseID, reviewID string) error {
	if !validUUID(reviewID) || !validUUID(courseID) {
		return ErrNotFound
	}
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx,
		`UPDATE reviews SET teacher_reply='', teacher_reply_at=NULL
		 WHERE id=$1::uuid AND course_id=$2::uuid`, reviewID, courseID)
	if err != nil {
		return fmt.Errorf("не удалось очистить ответ на отзыв: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ReviewRatingBreakdown — число отзывов курса по оценкам 1..5 (T69).
// Ключи 1..5 всегда в карте; ошибка БД/кривой id — нулевая разбивка
// (сигнатура без error, как у остальных агрегатов чтения).
func (s *PostgresStore) ReviewRatingBreakdown(courseID string) map[int]int {
	out := map[int]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0}
	if !validUUID(courseID) {
		return out
	}
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT rating, COUNT(*)::int FROM reviews
		 WHERE course_id=$1::uuid GROUP BY rating`, courseID)
	if err != nil {
		log.Printf("postgres ReviewRatingBreakdown: %v", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var rating, n int
		if err := rows.Scan(&rating, &n); err == nil && rating >= 1 && rating <= 5 {
			out[rating] = n
		}
	}
	return out
}

// validUUID — true для корректного UUID (с trim). Общий помощник для
// методов отзывов T69 и DeleteReview-подобных путей.
func validUUID(id string) bool {
	_, err := uuid.Parse(strings.TrimSpace(id))
	return err == nil
}

// ---- Settings ----

func clampCommission(n int) int {
	if n < 0 {
		return 0
	}
	if n > 90 {
		return 90
	}
	return n
}

func (s *PostgresStore) GetCommissionPct() int {
	ctx, cancel := s.ctx()
	defer cancel()
	var raw string
	if err := s.pool.QueryRow(ctx,
		`SELECT value FROM app_settings WHERE key='commission_pct'`).Scan(&raw); err != nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		return s.commissionPct
	}
	n, err := parsePct(raw)
	if err != nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		return s.commissionPct
	}
	s.mu.Lock()
	s.commissionPct = n
	s.mu.Unlock()
	return n
}

func parsePct(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, errors.New("пустое значение комиссии")
	}
	// value хранится как обычный текст ("15").
	n, err := strconv.Atoi(raw)
	if err != nil {
		// На случай JSON-обёртки — пробуем декодировать.
		var jn int
		if jerr := json.Unmarshal([]byte(raw), &jn); jerr != nil {
			return 0, errors.New("нечисловая комиссия")
		}
		n = jn
	}
	if n < 0 || n > 90 {
		return 0, errors.New("комиссия вне диапазона")
	}
	return n, nil
}

// SetCommissionPct сохраняет комиссию в app_settings и ТОЛЬКО ПОСЛЕ УСПЕШНОЙ
// записи обновляет кэш в памяти (T16, п.4): раньше запись глоталась через
// `_, _ =`, API отвечал 200, а значение терялось после перезапуска.
func (s *PostgresStore) SetCommissionPct(n int) error {
	n = clampCommission(n)
	ctx, cancel := s.ctx()
	defer cancel()
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO app_settings (key, value) VALUES ('commission_pct',$1)
		 ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`,
		strconv.Itoa(n)); err != nil {
		// Кэш не трогаем: иначе API показывал бы комиссию, которой нет в БД.
		return fmt.Errorf("не удалось сохранить комиссию в app_settings: %w", err)
	}
	s.mu.Lock()
	s.commissionPct = n
	s.mu.Unlock()
	return nil
}

// GetPromoBarEnabled читает флаг промо-баннера из app_settings (пост-P7).
// При отсутствии строки/ошибке БД возвращает кэш в памяти — так работает и
// до первого переключения админом (значение по умолчанию — true).
func (s *PostgresStore) GetPromoBarEnabled() bool {
	ctx, cancel := s.ctx()
	defer cancel()
	var raw string
	if err := s.pool.QueryRow(ctx,
		`SELECT value FROM app_settings WHERE key='promo_bar_enabled'`).Scan(&raw); err != nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		return s.promoBarEnabled
	}
	b, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		return s.promoBarEnabled
	}
	s.mu.Lock()
	s.promoBarEnabled = b
	s.mu.Unlock()
	return b
}

// SetPromoBarEnabled сохраняет флаг промо-баннера в app_settings и обновляет
// кэш только после успешной записи (контракт ошибок как у SetCommissionPct).
func (s *PostgresStore) SetPromoBarEnabled(enabled bool) error {
	ctx, cancel := s.ctx()
	defer cancel()
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO app_settings (key, value) VALUES ('promo_bar_enabled',$1)
		 ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`,
		strconv.FormatBool(enabled)); err != nil {
		return fmt.Errorf("не удалось сохранить настройку промо-баннера в app_settings: %w", err)
	}
	s.mu.Lock()
	s.promoBarEnabled = enabled
	s.mu.Unlock()
	return nil
}

// ---- T19: CRUD конструктора (удаление, порядок, счётчики) ----
//
// Методы вынесены в конец файла, чтобы не конфликтовать при слиянии волн.
// Ошибки БД пробрасываются завёрнутыми (не глотаются, в отличие от
// исторических методов `return nil, false`).

// DeleteModule удаляет модуль; уроки уходят каскадом (ON DELETE CASCADE,
// 001_init). ErrNotFound — модуля нет.
func (s *PostgresStore) DeleteModule(id string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx, `DELETE FROM modules WHERE id=$1::uuid`, id)
	if err != nil {
		return fmt.Errorf("не удалось удалить модуль: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteLesson удаляет урок; прогресс уходит каскадом (001_init).
// ErrNotFound — урока нет.
func (s *PostgresStore) DeleteLesson(id string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx, `DELETE FROM lessons WHERE id=$1::uuid`, id)
	if err != nil {
		return fmt.Errorf("не удалось удалить урок: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteCourse удаляет курс; записи, платежи, отзывы, модули и уроки уходят
// каскадом (001_init). ErrNotFound — курса нет. Handler ограничивает вызов
// черновиком, чтобы не терять историю оплат.
func (s *PostgresStore) DeleteCourse(id string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx, `DELETE FROM courses WHERE id=$1::uuid`, id)
	if err != nil {
		return fmt.Errorf("не удалось удалить курс: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateLesson перезаписывает контент урока; kind, позицию и модуль не меняет
// (T19). ErrNotFound — урока нет.
func (s *PostgresStore) UpdateLesson(l *Lesson) error {
	ctx, cancel := s.ctx()
	defer cancel()
	var quiz, code any = lessonToJSON(l.Quiz), lessonToJSON(l.CodeTask)
	var body, video, live any = nilIfEmpty(l.BodyMD), nilIfEmpty(l.VideoURL), nilIfEmpty(l.LiveURL)
	var starts any
	if l.StartsAt != nil {
		starts = *l.StartsAt
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE lessons SET title=$2, body_md=$3, video_url=$4, live_url=$5,
		 starts_at=$6, quiz=$7::jsonb, code_task=$8::jsonb, is_preview=$9
		 WHERE id=$1::uuid`,
		l.ID, l.Title, body, video, live, starts, quiz, code, l.IsPreview)
	if err != nil {
		return fmt.Errorf("не удалось обновить урок: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ReorderModules применяет полный список позиций модулей курса одной
// транзакцией (T19). Сначала позиции уводятся в отрицательные: иначе обмен
// местами нарушал бы UNIQUE (course_id, position) до завершения транзакции.
// ErrNotFound — пункт списка не относится к курсу.
func (s *PostgresStore) ReorderModules(courseID string, items []PositionItem) error {
	ctx, cancel := s.ctx()
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("перестановка модулей: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // откат после Commit безвреден
	if _, err := tx.Exec(ctx,
		`UPDATE modules SET position = -position WHERE course_id=$1::uuid`, courseID); err != nil {
		return fmt.Errorf("перестановка модулей: %w", err)
	}
	for _, it := range items {
		tag, err := tx.Exec(ctx,
			`UPDATE modules SET position=$2 WHERE id=$1::uuid AND course_id=$3::uuid`,
			it.ID, it.Position, courseID)
		if err != nil {
			return fmt.Errorf("перестановка модулей: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("перестановка модулей: %w", err)
	}
	return nil
}

// ReorderLessons применяет полный список позиций уроков курса одной
// транзакцией (T19). Как и у модулей, позиции сначала уводятся в
// отрицательные — из-за UNIQUE (module_id, position).
// ErrNotFound — пункт списка не относится к курсу.
func (s *PostgresStore) ReorderLessons(courseID string, items []PositionItem) error {
	ctx, cancel := s.ctx()
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("перестановка уроков: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // откат после Commit безвреден
	if _, err := tx.Exec(ctx,
		`UPDATE lessons l SET position = -l.position
		 FROM modules m WHERE m.id = l.module_id AND m.course_id=$1::uuid`, courseID); err != nil {
		return fmt.Errorf("перестановка уроков: %w", err)
	}
	for _, it := range items {
		tag, err := tx.Exec(ctx,
			`UPDATE lessons SET position=$2
			 WHERE id=$1::uuid
			 AND module_id IN (SELECT id FROM modules WHERE course_id=$3::uuid)`,
			it.ID, it.Position, courseID)
		if err != nil {
			return fmt.Errorf("перестановка уроков: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("перестановка уроков: %w", err)
	}
	return nil
}

// CountContentByCourses считает модули и уроки по курсам преподавателя
// одним запросом (T19: убирает N+1 в GET /teacher/courses).
func (s *PostgresStore) CountContentByCourses(teacherID string) map[string]ContentCounts {
	ctx, cancel := s.ctx()
	defer cancel()
	out := make(map[string]ContentCounts)
	rows, err := s.pool.Query(ctx,
		`SELECT c.id::text, COUNT(DISTINCT m.id)::int, COUNT(l.id)::int
		 FROM courses c
		 LEFT JOIN modules m ON m.course_id = c.id
		 LEFT JOIN lessons l ON l.module_id = m.id
		 WHERE c.teacher_id=$1::uuid
		 GROUP BY c.id`, teacherID)
	if err != nil {
		log.Printf("postgres CountContentByCourses: %v", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var cc ContentCounts
		if err := rows.Scan(&id, &cc.Modules, &cc.Lessons); err == nil {
			out[id] = cc
		}
	}
	return out
}

// ---- T63: промокоды ----

// promoCodeCols — порядок колонок promo_codes для scanPromoCode.
const promoCodeCols = `id::text, course_id::text, code, discount_pct, max_uses,
	used_count, expires_at, active, created_at`

// scanPromoCode собирает промокод из строки выборки (T63).
func scanPromoCode(scan func(dest ...any) error) (*PromoCode, error) {
	pc := &PromoCode{}
	if err := scan(&pc.ID, &pc.CourseID, &pc.Code, &pc.DiscountPct, &pc.MaxUses,
		&pc.UsedCount, &pc.ExpiresAt, &pc.Active, &pc.CreatedAt); err != nil {
		return nil, err
	}
	return pc, nil
}

// CreatePromoCode сохраняет промокод (T63). Код нормализуется к верхнему
// регистру (как в in-memory Store), used_count стартует с 0, active=true.
// Дубликат (course_id, code) — ErrAlreadyExists.
func (s *PostgresStore) CreatePromoCode(p *PromoCode) (*PromoCode, error) {
	code := strings.ToUpper(strings.TrimSpace(p.Code))
	ctx, cancel := s.ctx()
	defer cancel()
	row := s.pool.QueryRow(ctx,
		`INSERT INTO promo_codes (course_id, code, discount_pct, max_uses, expires_at, active)
		 VALUES ($1::uuid,$2,$3,$4,$5,TRUE)
		 RETURNING `+promoCodeCols,
		p.CourseID, code, p.DiscountPct, p.MaxUses, p.ExpiresAt)
	pc, err := scanPromoCode(row.Scan)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrAlreadyExists
		}
		return nil, err
	}
	*p = *pc
	return p, nil
}

// ListPromoCodesByCourse возвращает все коды курса (включая неактивные),
// новые первыми; при равном created_at порядок стабилен по id — как в
// in-memory Store. Ошибки БД логируются и дают пустой (не nil) список.
func (s *PostgresStore) ListPromoCodesByCourse(courseID string) []*PromoCode {
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT `+promoCodeCols+` FROM promo_codes
		 WHERE course_id=$1::uuid ORDER BY created_at DESC, id ASC`, courseID)
	if err != nil {
		log.Printf("postgres ListPromoCodesByCourse: %v", err)
		return []*PromoCode{}
	}
	defer rows.Close()
	out := make([]*PromoCode, 0)
	for rows.Next() {
		if pc, err := scanPromoCode(rows.Scan); err == nil {
			out = append(out, pc)
		}
	}
	return out
}

// GetPromoCodeByID ищет промокод по id. Мусорный id (не UUID) трактуется
// как «не найдено» — как в GetCourseByID.
func (s *PostgresStore) GetPromoCodeByID(id string) (*PromoCode, bool) {
	ctx, cancel := s.ctx()
	defer cancel()
	row := s.pool.QueryRow(ctx,
		`SELECT `+promoCodeCols+` FROM promo_codes WHERE id=$1::uuid`, id)
	pc, err := scanPromoCode(row.Scan)
	if err != nil {
		return nil, false
	}
	return pc, true
}

// FindPromoCode ищет промокод курса по коду (без учёта регистра: код
// нормализуется к верхнему, как при создании).
func (s *PostgresStore) FindPromoCode(courseID, code string) (*PromoCode, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	ctx, cancel := s.ctx()
	defer cancel()
	row := s.pool.QueryRow(ctx,
		`SELECT `+promoCodeCols+` FROM promo_codes
		 WHERE course_id=$1::uuid AND code=$2 LIMIT 1`, courseID, code)
	pc, err := scanPromoCode(row.Scan)
	if err != nil {
		return nil, false
	}
	return pc, true
}

// DeactivatePromoCode идемпотентно ставит active=false (повторный вызов —
// тоже успех). ErrNotFound — промокода нет.
func (s *PostgresStore) DeactivatePromoCode(id string) error {
	if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
		return ErrNotFound
	}
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx,
		`UPDATE promo_codes SET active=false WHERE id=$1::uuid`, id)
	if err != nil {
		return fmt.Errorf("не удалось деактивировать промокод: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// IncrementPromoCodeUsage атомарно увеличивает used_count на 1. false —
// промокода нет. Возврат средств счётчик не уменьшает (контракт T63).
func (s *PostgresStore) IncrementPromoCodeUsage(id string) bool {
	if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
		return false
	}
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx,
		`UPDATE promo_codes SET used_count=used_count+1 WHERE id=$1::uuid`, id)
	if err != nil {
		log.Printf("postgres IncrementPromoCodeUsage: %v", err)
		return false
	}
	return tag.RowsAffected() > 0
}
