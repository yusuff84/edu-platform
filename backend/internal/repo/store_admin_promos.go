package repo

// T89 — in-memory-реализация глобального журнала промокодов (см.
// admin_promos.go — типы и семантика, postgres_admin_promos.go — SQL-ветка).
//
// Журнал читает те же promo_codes, что и teacher-эндпоинты T63; отдельного
// хранилища нет. Порядок строк — createdAt DESC, при равном времени id ASC:
// так же сортирует Postgres-реализация, поэтому выдача детерминирована.

import (
	"sort"
	"strings"
)

// adminPromoRowLocked собирает снимок строки журнала по промокоду. Вызывать
// только под s.mu (RLock или Lock): читает карту курсов и копирует время.
// Указатели наружу не отдаются — после снятия лока мутировать их нельзя.
func (s *Store) adminPromoRowLocked(pc *PromoCode) *AdminPromoRow {
	row := &AdminPromoRow{PromoCode: *pc}
	row.ExpiresAt = cloneTimePtr(pc.ExpiresAt)
	if c, ok := s.courses[pc.CourseID]; ok {
		row.CourseTitle = c.Title
		row.CourseSlug = c.Slug
	}
	return row
}

// ListAdminPromoCodes возвращает коды всех курсов для админ-журнала (T89):
// фильтры комбинируются через AND, total считается до пагинации. Q —
// подстрока без учёта регистра по коду ИЛИ названию курса. Курс без записи
// в карте (не должно случаться: удаление каскадно чистит коды) даёт пустые
// courseTitle/courseSlug и по Q не находится.
func (s *Store) ListAdminPromoCodes(f AdminPromoFilter) ([]*AdminPromoRow, int) {
	page, limit := clampPageLimit(f.Page, f.Limit)
	q := strings.ToLower(strings.TrimSpace(f.Q))

	s.mu.RLock()
	defer s.mu.RUnlock()

	matched := make([]*PromoCode, 0, len(s.promoCodes))
	for _, pc := range s.promoCodes {
		if f.CourseID != "" && pc.CourseID != f.CourseID {
			continue
		}
		if f.Active != nil && pc.Active != *f.Active {
			continue
		}
		if q != "" {
			title := ""
			if c, ok := s.courses[pc.CourseID]; ok {
				title = c.Title
			}
			if !strings.Contains(strings.ToLower(pc.Code), q) &&
				!strings.Contains(strings.ToLower(title), q) {
				continue
			}
		}
		matched = append(matched, pc)
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].CreatedAt.Equal(matched[j].CreatedAt) {
			return matched[i].ID < matched[j].ID
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
	items := make([]*AdminPromoRow, 0, end-start)
	for _, pc := range matched[start:end] {
		items = append(items, s.adminPromoRowLocked(pc))
	}
	return items, total
}

// UpdateAdminPromoCode применяет частичное обновление (T89): меняются только
// переданные поля, ExpiresAt различает «не менять» (nil-указатель) и
// «очистить» (указатель на nil). ErrNotFound — кода с таким id нет.
// Валидацию значений (диапазоны, usedCount, архив) выполняет handler: Store
// остаётся механическим слоем, как и у остальных частичных обновлений.
func (s *Store) UpdateAdminPromoCode(id string, p AdminPromoPatch) (*AdminPromoRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pc, ok := s.promoCodes[id]
	if !ok {
		return nil, ErrNotFound
	}
	if p.DiscountPct != nil {
		pc.DiscountPct = *p.DiscountPct
	}
	if p.MaxUses != nil {
		pc.MaxUses = *p.MaxUses
	}
	if p.ExpiresAt != nil {
		if *p.ExpiresAt == nil {
			pc.ExpiresAt = nil
		} else {
			pc.ExpiresAt = cloneTimePtr(*p.ExpiresAt)
		}
	}
	if p.Active != nil {
		pc.Active = *p.Active
	}
	return s.adminPromoRowLocked(pc), nil
}
