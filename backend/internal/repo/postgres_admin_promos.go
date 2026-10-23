package repo

// T89 — SQL-реализация глобального журнала промокодов (см. admin_promos.go —
// типы, store_admin_promos.go — in-memory-ветка и комментарии к семантике).
//
// Список читает promo_codes с JOIN courses (courseTitle/courseSlug в строке
// журнала), total — отдельным COUNT по тем же фильтрам. Обновление — один
// динамический UPDATE только по переданным полям, затем чтение строки с
// join-ом курса.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// adminPromoCols — колонки AdminPromoRow в порядке scanAdminPromoRow.
const adminPromoCols = `p.id::text, p.course_id::text, c.title, c.slug, p.code,
	p.discount_pct, p.max_uses, p.used_count, p.expires_at, p.active, p.created_at`

// scanAdminPromoRow собирает строку журнала из выборки adminPromoCols.
func scanAdminPromoRow(scan func(dest ...any) error) (*AdminPromoRow, error) {
	row := &AdminPromoRow{}
	if err := scan(&row.ID, &row.CourseID, &row.CourseTitle, &row.CourseSlug,
		&row.Code, &row.DiscountPct, &row.MaxUses, &row.UsedCount,
		&row.ExpiresAt, &row.Active, &row.CreatedAt); err != nil {
		return nil, err
	}
	return row, nil
}

// ListAdminPromoCodes возвращает коды всех курсов для админ-журнала (T89).
// Фильтры — параметрами $N (без конкатенации пользовательского ввода в SQL):
// courseId, q (подстрока без учёта регистра по коду ИЛИ названию курса),
// active. total считается до пагинации теми же условиями, что и страница.
func (s *PostgresStore) ListAdminPromoCodes(f AdminPromoFilter) ([]*AdminPromoRow, int) {
	page, limit := clampPageLimit(f.Page, f.Limit)

	where := ""
	args := make([]any, 0, 3)
	if f.CourseID != "" {
		args = append(args, f.CourseID)
		where += fmt.Sprintf(" AND p.course_id=$%d::uuid", len(args))
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		args = append(args, "%"+strings.ToLower(q)+"%")
		where += fmt.Sprintf(
			" AND (lower(p.code) LIKE $%d OR lower(c.title) LIKE $%d)",
			len(args), len(args))
	}
	if f.Active != nil {
		args = append(args, *f.Active)
		where += fmt.Sprintf(" AND p.active=$%d", len(args))
	}

	ctx, cancel := s.ctx()
	defer cancel()

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM promo_codes p
		 JOIN courses c ON c.id = p.course_id WHERE 1=1`+where, args...).Scan(&total); err != nil {
		log.Printf("postgres ListAdminPromoCodes count: %v", err)
		return nil, 0
	}

	offset := (page - 1) * limit
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT %s FROM promo_codes p
		JOIN courses c ON c.id = p.course_id
		WHERE 1=1%s
		ORDER BY p.created_at DESC, p.id ASC
		LIMIT %d OFFSET %d`, adminPromoCols, where, limit, offset), args...)
	if err != nil {
		log.Printf("postgres ListAdminPromoCodes: %v", err)
		return nil, 0
	}
	defer rows.Close()

	items := make([]*AdminPromoRow, 0, limit)
	for rows.Next() {
		if row, err := scanAdminPromoRow(rows.Scan); err == nil {
			items = append(items, row)
		}
	}
	return items, total
}

// adminPromoRowByID читает строку журнала с данными курса; ErrNotFound —
// кода нет (в том числе мусорный/не-UUID id, чтобы не отдавать 500).
func (s *PostgresStore) adminPromoRowByID(ctx context.Context, id string) (*AdminPromoRow, error) {
	if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
		return nil, ErrNotFound
	}
	row := s.pool.QueryRow(ctx, `
		SELECT `+adminPromoCols+` FROM promo_codes p
		JOIN courses c ON c.id = p.course_id
		WHERE p.id=$1::uuid`, id)
	promo, err := scanAdminPromoRow(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return promo, nil
}

// UpdateAdminPromoCode применяет частичное обновление (T89) одним UPDATE по
// переданным полям ($2, $3…; id — $1 в WHERE), затем перечитывает строку с
// join-ом курса. ExpiresAt: nil-указатель — не менять, указатель на nil —
// записать NULL (срок очищен), указатель на время — новый срок.
// ErrNotFound — кода нет (включая не-UUID id).
func (s *PostgresStore) UpdateAdminPromoCode(id string, p AdminPromoPatch) (*AdminPromoRow, error) {
	id = strings.TrimSpace(id)
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrNotFound
	}

	sets := make([]string, 0, 4)
	args := []any{id}
	add := func(col string, v any) {
		args = append(args, v)
		sets = append(sets, col+"=$"+strconv.Itoa(len(args)))
	}
	if p.DiscountPct != nil {
		add("discount_pct", *p.DiscountPct)
	}
	if p.MaxUses != nil {
		add("max_uses", *p.MaxUses)
	}
	if p.ExpiresAt != nil {
		if *p.ExpiresAt == nil {
			add("expires_at", nil)
		} else {
			add("expires_at", **p.ExpiresAt)
		}
	}
	if p.Active != nil {
		add("active", *p.Active)
	}

	ctx, cancel := s.ctx()
	defer cancel()

	if len(sets) > 0 {
		tag, err := s.pool.Exec(ctx,
			"UPDATE promo_codes SET "+strings.Join(sets, ", ")+" WHERE id=$1::uuid",
			args...)
		if err != nil {
			return nil, fmt.Errorf("не удалось обновить промокод: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil, ErrNotFound
		}
	}
	// Пустой патч (все поля nil) handler отсекает 400-й; прямой вызов стора
	// просто возвращает текущую строку — как no-op.
	return s.adminPromoRowByID(ctx, id)
}
