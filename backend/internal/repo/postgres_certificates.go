package repo

// T67 — Postgres-реализация сертификатов (in-memory зеркало — certificates.go).
// Номер генерируется общим newCertificateNumber; уникальность пары
// (user_id, course_id) и номера обеспечивает схема 010_certificates.sql.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

// isForeignKeyViolation — FK-нарушение (23503): нет ученика или курса.
// Для контракта IssueCertificate это ErrNotFound, как и в in-memory Store.
func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

// certificateByPair читает сертификат по уникальной паре (ученик, курс).
func (s *PostgresStore) certificateByPair(ctx context.Context, studentID, courseID string) (*Certificate, bool) {
	c := &Certificate{}
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, user_id::text, course_id::text, number, issued_at
		 FROM certificates WHERE user_id=$1::uuid AND course_id=$2::uuid`,
		studentID, courseID).Scan(&c.ID, &c.UserID, &c.CourseID, &c.Number, &c.IssuedAt)
	if err != nil {
		return nil, false
	}
	return c, true
}

// IssueCertificate идемпотентно выдаёт сертификат: при конфликте пары
// (user_id, course_id) возвращается уже существующий; при коллизии номера
// (UNIQUE number) генерация повторяется.
func (s *PostgresStore) IssueCertificate(studentID, courseID string) (*Certificate, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	if c, ok := s.certificateByPair(ctx, studentID, courseID); ok {
		return c, nil
	}
	for attempt := 0; attempt < certificateNumberAttempts; attempt++ {
		number, err := newCertificateNumber(nil)
		if err != nil {
			return nil, err
		}
		c := &Certificate{UserID: studentID, CourseID: courseID, Number: number}
		err = s.pool.QueryRow(ctx,
			`INSERT INTO certificates (user_id, course_id, number)
			 VALUES ($1::uuid,$2::uuid,$3)
			 RETURNING id::text, issued_at`,
			studentID, courseID, number).Scan(&c.ID, &c.IssuedAt)
		if err == nil {
			return c, nil
		}
		if isUniqueViolation(err) {
			// Либо параллельный вызов уже выдал сертификат за эту пару,
			// либо номер совпал — тогда просто пробуем новый.
			if existing, ok := s.certificateByPair(ctx, studentID, courseID); ok {
				return existing, nil
			}
			continue
		}
		if isForeignKeyViolation(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("не удалось выдать сертификат: %w", err)
	}
	return nil, errCertificateNumber
}

// GetCertificateByNumber — публичная карточка сертификата по номеру.
func (s *PostgresStore) GetCertificateByNumber(number string) (*CertificateDetails, bool) {
	ctx, cancel := s.ctx()
	defer cancel()
	d := &CertificateDetails{}
	err := s.pool.QueryRow(ctx,
		`SELECT c.number, c.issued_at, COALESCE(u.display_name,''), co.title, co.slug
		 FROM certificates c
		 JOIN users u ON u.id = c.user_id
		 JOIN courses co ON co.id = c.course_id
		 WHERE c.number=$1`, number).
		Scan(&d.Number, &d.IssuedAt, &d.StudentName, &d.CourseTitle, &d.CourseSlug)
	if err != nil {
		return nil, false
	}
	return d, true
}

// ListCertificatesByStudent — сертификаты ученика, новые первыми; при равной
// issued_at порядок стабилизируется номером по убыванию. Никогда не nil.
func (s *PostgresStore) ListCertificatesByStudent(studentID string) []*CertificateListItem {
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT c.number, c.issued_at, c.course_id::text, co.title, co.slug
		 FROM certificates c
		 JOIN courses co ON co.id = c.course_id
		 WHERE c.user_id=$1::uuid
		 ORDER BY c.issued_at DESC, c.number DESC`, studentID)
	if err != nil {
		return []*CertificateListItem{}
	}
	defer rows.Close()
	items := make([]*CertificateListItem, 0)
	for rows.Next() {
		it := &CertificateListItem{}
		if err := rows.Scan(&it.Number, &it.IssuedAt, &it.CourseID, &it.CourseTitle, &it.CourseSlug); err == nil {
			items = append(items, it)
		}
	}
	return items
}
