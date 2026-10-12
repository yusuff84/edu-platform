package repo

import (
	"context"
	"encoding/json"
	"errors"
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
	return &PostgresStore{pool: pool, commissionPct: fallbackCommission}
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
	u := &User{Email: email, PasswordHash: passwordHash, Role: role}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, role) VALUES ($1,$2,$3)
		 RETURNING id::text, created_at`,
		email, passwordHash, string(role)).Scan(&u.ID, &u.CreatedAt)
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
		`SELECT id::text, email, password_hash, role, COALESCE(display_name,''), created_at
		 FROM users WHERE lower(email)=lower($1) LIMIT 1`, normEmail(email)).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &display, &u.CreatedAt)
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
		`SELECT id::text, email, password_hash, role, COALESCE(display_name,''), created_at
		 FROM users WHERE id=$1::uuid`, id).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &display, &u.CreatedAt)
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

func (s *PostgresStore) GetInviteByTokenHash(hash string) (*TeacherInvite, bool) {
	ctx, cancel := s.ctx()
	defer cancel()
	inv := &TeacherInvite{}
	var prefix string
	var createdBy *string
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, email, token_hash, COALESCE(token_prefix,''), status, expires_at,
		        created_by::text, created_at
		 FROM teacher_invites WHERE token_hash=$1 LIMIT 1`, hash).
		Scan(&inv.ID, &inv.Email, &inv.TokenHash, &prefix, &inv.Status,
			&inv.ExpiresAt, &createdBy, &inv.CreatedAt)
	if err != nil {
		return nil, false
	}
	inv.TokenPrefix = prefix
	inv.CreatedBy = strOrEmpty(createdBy)
	return inv, true
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
	var teacherID, cover, currency, status, comment string
	var avg float64
	var count int
	var created, updated time.Time
	if err := scan(&c.ID, &teacherID, &c.Slug, &c.Title, &c.Description, &cover,
		&c.PriceCents, &currency, &status, &avg, &count, &comment, &created, &updated); err != nil {
		return nil, err
	}
	c.TeacherID = teacherID
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

const courseCols = `id::text, teacher_id::text, slug, title, description,
	COALESCE(cover_path,''), price_cents, currency::text, status,
	COALESCE(avg_rating,0)::float8, ratings_count, COALESCE(moderation_comment,''),
	created_at, updated_at`

func (s *PostgresStore) CreateCourse(c *Course) *Course {
	now := time.Now().UTC()
	currency := strings.ToUpper(strings.TrimSpace(c.Currency))
	if currency == "" {
		currency = "RUB"
	}
	ctx, cancel := s.ctx()
	defer cancel()
	row := s.pool.QueryRow(ctx,
		`INSERT INTO courses (teacher_id, slug, title, description, price_cents, currency, status)
		 VALUES ($1::uuid,$2,$3,$4,$5,$6,'draft')
		 RETURNING `+courseCols,
		c.TeacherID, c.Slug, c.Title, c.Description, c.PriceCents, currency)
	if nc, err := scanCourse(row.Scan); err == nil {
		*c = *nc
		return c
	} else {
		log.Printf("postgres CreateCourse: %v", err)
		c.ID = uuid.NewString()
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
		&starts, &quizRaw, &codeRaw, &created); err != nil {
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
	l.starts_at, l.quiz::text, l.code_task::text, l.created_at`

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
	}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO lessons (module_id, title, kind, position, body_md, video_url, live_url, starts_at, quiz, code_task)
		 SELECT $1::uuid, $2, $3, COALESCE(MAX(position),0)+1, $4, $5, $6, $7, $8::jsonb, $9::jsonb
		 FROM lessons WHERE module_id=$1::uuid
		 RETURNING id::text, position, created_at`,
		l.ModuleID, l.Title, string(l.Kind), body, video, live, starts, quiz, code).
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
	return e, false
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
	if err := scan(&p.ID, &studentID, &courseID, &p.AmountCents, &p.PlatformFeeCents,
		&p.TeacherAmountCents, &currency, &provider, &providerID, &status,
		&p.CreatedAt, &processed); err != nil {
		return nil, err
	}
	p.StudentID = studentID
	p.CourseID = courseID
	p.Currency = strings.TrimSpace(currency)
	p.Provider = provider
	p.ProviderPaymentID = providerID
	p.Status = PaymentStatus(status)
	p.ProcessedAt = processed
	return p, nil
}

const paymentCols = `id::text, student_id::text, course_id::text, amount_cents, platform_fee_cents,
	teacher_amount_cents, currency::text, provider, COALESCE(provider_payment_id,''),
	status, created_at, processed_at`

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
		  teacher_amount_cents, currency, provider, provider_payment_id, status)
		 VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9)
		 RETURNING `+paymentCols,
		p.StudentID, p.CourseID, p.AmountCents, p.PlatformFeeCents, p.TeacherAmountCents,
		currency, p.Provider, providerID, string(p.Status))
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
	tag, err := s.pool.Exec(ctx,
		`UPDATE payments SET status=$2,
		 processed_at=CASE WHEN $2 IN ('succeeded','failed') THEN now() ELSE processed_at END
		 WHERE id=$1::uuid`, id, string(status))
	if err != nil {
		return false
	}
	return tag.RowsAffected() > 0
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

func (s *PostgresStore) ListReviewsByCourse(courseID string) []*Review {
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, student_id::text, course_id::text, rating, COALESCE(body,''), created_at
		 FROM reviews WHERE course_id=$1::uuid ORDER BY created_at DESC`, courseID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Review
	for rows.Next() {
		r := &Review{}
		if err := rows.Scan(&r.ID, &r.StudentID, &r.CourseID, &r.Rating, &r.Body, &r.CreatedAt); err == nil {
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

func (s *PostgresStore) SetCommissionPct(n int) {
	n = clampCommission(n)
	s.mu.Lock()
	s.commissionPct = n
	s.mu.Unlock()
	ctx, cancel := s.ctx()
	defer cancel()
	_, _ = s.pool.Exec(ctx,
		`INSERT INTO app_settings (key, value) VALUES ('commission_pct',$1)
		 ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`,
		strconv.Itoa(n))
}
