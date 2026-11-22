package repo

// Postgres-реализация расширенных операций администратора (in-memory — admin_ops.go).

import (
	"fmt"
	"log"
	"strings"
)

// SetUserEmail меняет email; ErrAlreadyExists — занят, ErrNotFound — нет пользователя.
func (s *PostgresStore) SetUserEmail(id, email string) error {
	if !validUUID(id) {
		return ErrNotFound
	}
	ctx, cancel := s.ctx()
	defer cancel()
	// Одним выражением со сменой адреса гасим выданные письмами токены:
	// иначе ссылка подтверждения, ушедшая на старый адрес, подтверждала бы
	// новый, а ссылка сброса пароля на старом адресе продолжала бы работать.
	var updated int
	err := s.pool.QueryRow(ctx,
		`WITH upd AS (
		   UPDATE users SET email=$2, email_verified_at=NULL WHERE id=$1::uuid RETURNING id
		 ), del AS (
		   DELETE FROM account_tokens WHERE user_id IN (SELECT id FROM upd)
		 )
		 SELECT count(*)::int FROM upd`,
		id, normEmail(email)).Scan(&updated)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrAlreadyExists
		}
		return fmt.Errorf("postgres SetUserEmail: %w", err)
	}
	if updated == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteLessonProgress удаляет прогресс ученика по указанным урокам.
func (s *PostgresStore) DeleteLessonProgress(studentID string, lessonIDs []string) int {
	if !validUUID(studentID) || len(lessonIDs) == 0 {
		return 0
	}
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM lesson_progress WHERE student_id=$1::uuid AND lesson_id = ANY($2::uuid[])`,
		studentID, lessonIDs)
	if err != nil {
		log.Printf("postgres DeleteLessonProgress: %v", err)
		return 0
	}
	return int(tag.RowsAffected())
}

// ListAllCertificates — сертификаты всех учеников с поиском, новые первыми.
func (s *PostgresStore) ListAllCertificates(q string, limit int) []*AdminCertificateItem {
	out := make([]*AdminCertificateItem, 0)
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	ctx, cancel := s.ctx()
	defer cancel()
	like := "%" + strings.ToLower(strings.TrimSpace(q)) + "%"
	rows, err := s.pool.Query(ctx,
		`SELECT c.number, c.issued_at, u.id::text, u.email, coalesce(u.display_name,''), co.id::text, co.title
		   FROM certificates c JOIN users u ON u.id=c.user_id JOIN courses co ON co.id=c.course_id
		  WHERE $1 = '%%' OR lower(u.email || ' ' || coalesce(u.display_name,'') || ' ' || c.number || ' ' || co.title) LIKE $1
		  ORDER BY c.issued_at DESC LIMIT $2`, like, limit)
	if err != nil {
		log.Printf("postgres ListAllCertificates: %v", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var it AdminCertificateItem
		if err := rows.Scan(&it.Number, &it.IssuedAt, &it.UserID, &it.Email, &it.StudentName, &it.CourseID, &it.CourseTitle); err != nil {
			log.Printf("postgres ListAllCertificates scan: %v", err)
			continue
		}
		out = append(out, &it)
	}
	return out
}

// DeleteCertificate отзывает сертификат по номеру.
func (s *PostgresStore) DeleteCertificate(number string) bool {
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx, `DELETE FROM certificates WHERE number=$1`, number)
	if err != nil {
		log.Printf("postgres DeleteCertificate: %v", err)
		return false
	}
	return tag.RowsAffected() > 0
}

// SetCourseTeacher передаёт курс другому преподавателю.
func (s *PostgresStore) SetCourseTeacher(courseID, teacherID string) bool {
	if !validUUID(courseID) || !validUUID(teacherID) {
		return false
	}
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx, `UPDATE courses SET teacher_id=$2::uuid, updated_at=now() WHERE id=$1::uuid`,
		courseID, teacherID)
	if err != nil {
		log.Printf("postgres SetCourseTeacher: %v", err)
		return false
	}
	return tag.RowsAffected() > 0
}

const siteSettingPrefix = "site."

// GetSiteSettings — редактируемые тексты сайта из app_settings (ключи site.*).
func (s *PostgresStore) GetSiteSettings() map[string]string {
	out := map[string]string{}
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx, `SELECT key, value FROM app_settings WHERE key LIKE 'site.%'`)
	if err != nil {
		log.Printf("postgres GetSiteSettings: %v", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) == nil {
			out[strings.TrimPrefix(k, siteSettingPrefix)] = v
		}
	}
	return out
}

// SetSiteSettings сохраняет тексты сайта одной транзакцией; пустое значение удаляет ключ.
func (s *PostgresStore) SetSiteSettings(kv map[string]string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres SetSiteSettings: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for k, v := range kv {
		key := siteSettingPrefix + k
		if v == "" {
			_, err = tx.Exec(ctx, `DELETE FROM app_settings WHERE key=$1`, key)
		} else {
			_, err = tx.Exec(ctx, `INSERT INTO app_settings (key, value) VALUES ($1,$2)
				ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`, key, v)
		}
		if err != nil {
			return fmt.Errorf("postgres SetSiteSettings %s: %w", k, err)
		}
	}
	return tx.Commit(ctx)
}
