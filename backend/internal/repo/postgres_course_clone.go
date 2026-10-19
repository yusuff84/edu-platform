package repo

// T21 — жизненный цикл курса: Postgres-реализация глубокого копирования
// курса и переноса урока между модулями. Правила формирования названия/slug
// копии — в course_clone.go (copyTitle, copySlugCandidate), там же
// in-memory-реализация с комментариями к семантике.
//
// Копия пишется одной транзакцией: либо курс с полной программой, либо
// ничего. Новые id выдаёт БД (DEFAULT gen_random_uuid()) — как в
// CreateModule/CreateLesson. UNIQUE (course_id, position) и
// UNIQUE (module_id, position) при копировании не нарушаются: позиции
// переносятся как есть, а модули/уроки попадают в новый (пустой) курс.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// CloneCourse — см. StoreAPI (T21).
func (s *PostgresStore) CloneCourse(courseID string) (*Course, error) {
	return s.CloneCourseAs(courseID, "")
}

// CloneCourseAs — копия курса с явным владельцем (T21); teacherID == "" —
// владелец копии наследуется от исходного курса. ErrNotFound — исходного
// курса нет.
func (s *PostgresStore) CloneCourseAs(courseID, teacherID string) (*Course, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("копирование курса: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // откат после Commit безвреден

	// FOR SHARE: исходный курс не должен быть удалён (каскадом) или изменён
	// другой транзакцией, пока копируем его программу.
	src, err := scanCourse(tx.QueryRow(ctx,
		`SELECT `+courseCols+` FROM courses WHERE id=$1::uuid FOR SHARE`, courseID).Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("копирование курса: %w", err)
	}

	owner := strings.TrimSpace(teacherID)
	if owner == "" {
		owner = src.TeacherID
	}
	slug, err := freeCopySlugTx(ctx, tx, src.Slug)
	if err != nil {
		return nil, err
	}
	// status='draft', avg_rating=0, ratings_count=0, moderation_comment=NULL,
	// created_at/updated_at=now() — значения по умолчанию схемы.
	nc, err := scanCourse(tx.QueryRow(ctx,
		`INSERT INTO courses (teacher_id, slug, title, description, cover_path, price_cents, currency, status)
		 VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,'draft')
		 RETURNING `+courseCols,
		owner, slug, copyTitle(src.Title), src.Description, src.CoverPath,
		src.PriceCents, src.Currency).Scan)
	if err != nil {
		return nil, fmt.Errorf("копирование курса: %w", err)
	}

	// Модули копируем по одному: нужен маппинг «модуль источника → модуль
	// копии», чтобы перепривязать к нему уроки. Читаем все строки сразу —
	// на одном соединении нельзя держать открытый pgx.Rows и слать запросы.
	rows, err := tx.Query(ctx,
		`SELECT id::text, title, position FROM modules WHERE course_id=$1::uuid ORDER BY position ASC`,
		courseID)
	if err != nil {
		return nil, fmt.Errorf("копирование курса: %w", err)
	}
	type srcModule struct {
		id       string
		title    string
		position int
	}
	mods := make([]srcModule, 0, 4)
	for rows.Next() {
		var m srcModule
		if err := rows.Scan(&m.id, &m.title, &m.position); err != nil {
			rows.Close()
			return nil, fmt.Errorf("копирование курса: %w", err)
		}
		mods = append(mods, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("копирование курса: %w", err)
	}

	for _, m := range mods {
		var newModuleID string
		if err := tx.QueryRow(ctx,
			`INSERT INTO modules (course_id, title, position) VALUES ($1::uuid,$2,$3) RETURNING id::text`,
			nc.ID, m.title, m.position).Scan(&newModuleID); err != nil {
			return nil, fmt.Errorf("копирование курса: %w", err)
		}
		// Уроки переносим одним INSERT..SELECT: jsonb-поля quiz/code_task
		// копируются как значение, starts_at — как есть; id выдаёт БД.
		if _, err := tx.Exec(ctx,
			`INSERT INTO lessons (module_id, title, kind, position, body_md, video_url,
			  live_url, starts_at, quiz, code_task)
			 SELECT $1::uuid, l.title, l.kind, l.position, l.body_md, l.video_url,
			  l.live_url, l.starts_at, l.quiz, l.code_task
			 FROM lessons l WHERE l.module_id=$2::uuid`,
			newModuleID, m.id); err != nil {
			return nil, fmt.Errorf("копирование курса: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("копирование курса: %w", err)
	}
	return nc, nil
}

// freeCopySlugTx подбирает свободный slug копии в рамках транзакции: та же
// проверка занятости, что в SlugTaken (без учёта регистра), но видит и
// незакоммиченные изменения текущей транзакции.
func freeCopySlugTx(ctx context.Context, tx pgx.Tx, srcSlug string) (string, error) {
	for n := 2; n <= 10000; n++ {
		cand := copySlugCandidate(srcSlug, n)
		var exists bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM courses WHERE lower(slug)=lower($1))`, cand).Scan(&exists); err != nil {
			return "", fmt.Errorf("копирование курса: %w", err)
		}
		if !exists {
			return cand, nil
		}
	}
	// Практически недостижимо (10 000 копий): uuid-основа гарантирует
	// свободный slug настолько же, насколько уникален uuid.
	return copySlugCandidate(uuid.NewString(), 2), nil
}

// MoveLessonToModule переносит урок в конец другого модуля (T21) одной
// транзакцией. ErrNotFound — урока или модуля нет. Проверки «модуль того же
// курса» и «курс в черновике» накладывает handler.
func (s *PostgresStore) MoveLessonToModule(lessonID, moduleID string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("перенос урока: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // откат после Commit безвреден

	// Блокируем целевой модуль: MAX(position)+1 должен считаться под локом,
	// иначе два параллельных переноса в один модуль получат одну позицию и
	// нарушат UNIQUE (module_id, position). Заодно проверяем существование.
	var target string
	err = tx.QueryRow(ctx,
		`SELECT id::text FROM modules WHERE id=$1::uuid FOR UPDATE`, moduleID).Scan(&target)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("перенос урока: %w", err)
	}

	var pos int
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(position),0)+1 FROM lessons WHERE module_id=$1::uuid`,
		moduleID).Scan(&pos); err != nil {
		return fmt.Errorf("перенос урока: %w", err)
	}

	tag, err := tx.Exec(ctx,
		`UPDATE lessons SET module_id=$2::uuid, position=$3 WHERE id=$1::uuid`,
		lessonID, moduleID, pos)
	if err != nil {
		return fmt.Errorf("перенос урока: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("перенос урока: %w", err)
	}
	return nil
}
