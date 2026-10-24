package postgres_test

// T89 на Postgres: глобальный журнал промокодов (ListAdminPromoCodes) и
// частичное обновление (UpdateAdminPromoCode) SQL-ветки. Env-gated через
// TEST_DATABASE_URL, как остальные тесты пакета: без него — t.Skip.
//
// Проверяются та же семантика, что у in-memory Store и контрактных тестов:
// join курса (courseTitle/courseSlug), фильтры courseId/q/active, порядок
// createdAt DESC + id ASC, total до пагинации, частичный UPDATE каждого поля,
// очистка expiresAt через указатель на nil и ErrNotFound.

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"edu-platform/backend/internal/repo"
)

func TestPostgresAdminPromosParity(t *testing.T) {
	s := testStore(t)
	teacher := createUser(t, s, "t89-teacher", repo.RoleTeacher)
	courseA := createCourse(t, s, teacher.ID, "t89-a", 10000)
	courseB := createCourse(t, s, teacher.ID, "t89-b", 5000)

	suffix := strings.ToUpper(shortID())
	codeA1 := "T89A1-" + suffix
	codeA2 := "T89A2-" + suffix
	codeB1 := "T89B1-" + suffix
	expiresA := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Microsecond)

	pA1, err := s.CreatePromoCode(&repo.PromoCode{
		CourseID: courseA.ID, Code: codeA1, DiscountPct: 20,
		MaxUses: 100, ExpiresAt: &expiresA,
	})
	if err != nil {
		t.Fatalf("CreatePromoCode A1: %v", err)
	}
	time.Sleep(2 * time.Millisecond) // createdAt следующего кода строго позже
	pA2, err := s.CreatePromoCode(&repo.PromoCode{
		CourseID: courseA.ID, Code: codeA2, DiscountPct: 10,
	})
	if err != nil {
		t.Fatalf("CreatePromoCode A2: %v", err)
	}
	if err := s.DeactivatePromoCode(pA2.ID); err != nil {
		t.Fatalf("DeactivatePromoCode A2: %v", err)
	}
	pB1, err := s.CreatePromoCode(&repo.PromoCode{
		CourseID: courseB.ID, Code: codeB1, DiscountPct: 50, MaxUses: 200,
	})
	if err != nil {
		t.Fatalf("CreatePromoCode B1: %v", err)
	}
	if pA1.ID == "" || pA2.ID == "" || pB1.ID == "" {
		t.Fatalf("CreatePromoCode не вернул id: %+v %+v %+v", pA1, pA2, pB1)
	}

	// Список по курсу: новые первыми (A2 создан позже A1), total до пагинации,
	// курс подтянут join-ом.
	items, total := s.ListAdminPromoCodes(repo.AdminPromoFilter{CourseID: courseA.ID})
	if total != 2 || len(items) != 2 {
		t.Fatalf("courseA: total=%d len=%d, ждали 2", total, len(items))
	}
	if items[0].ID != pA2.ID || items[1].ID != pA1.ID {
		t.Fatalf("порядок списка: %s, %s (ждали %s, %s)",
			items[0].ID, items[1].ID, pA2.ID, pA1.ID)
	}
	row := items[1]
	if row.CourseID != courseA.ID || row.CourseTitle != courseA.Title ||
		row.CourseSlug != courseA.Slug || row.Code != codeA1 ||
		row.DiscountPct != 20 || row.MaxUses != 100 || row.UsedCount != 0 ||
		!row.Active || row.ExpiresAt == nil || !sameMicro(*row.ExpiresAt, expiresA) {
		t.Fatalf("строка журнала A1: %+v", row)
	}
	if items[0].Active {
		t.Fatalf("A2 должен быть неактивен: %+v", items[0])
	}

	// q — подстрока без учёта регистра по коду...
	items, total = s.ListAdminPromoCodes(repo.AdminPromoFilter{
		CourseID: courseA.ID, Q: strings.ToLower(codeA1),
	})
	if total != 1 || len(items) != 1 || items[0].ID != pA1.ID {
		t.Fatalf("q по коду: total=%d items=%+v", total, items)
	}
	// ...и по названию курса (верхний регистр — проверка регистронезависимости).
	items, total = s.ListAdminPromoCodes(repo.AdminPromoFilter{
		CourseID: courseA.ID, Q: strings.ToUpper(courseA.Title),
	})
	if total != 2 || len(items) != 2 {
		t.Fatalf("q по названию курса: total=%d len=%d", total, len(items))
	}
	items, total = s.ListAdminPromoCodes(repo.AdminPromoFilter{Q: strings.ToLower(codeB1)})
	if total != 1 || len(items) != 1 || items[0].ID != pB1.ID {
		t.Fatalf("q по коду B1: total=%d items=%+v", total, items)
	}

	// active — точное совпадение, комбинируется с courseId.
	yes, no := true, false
	items, total = s.ListAdminPromoCodes(repo.AdminPromoFilter{
		CourseID: courseA.ID, Active: &yes,
	})
	if total != 1 || len(items) != 1 || items[0].ID != pA1.ID {
		t.Fatalf("courseA+active=true: total=%d items=%+v", total, items)
	}
	items, total = s.ListAdminPromoCodes(repo.AdminPromoFilter{
		CourseID: courseA.ID, Active: &no,
	})
	if total != 1 || len(items) != 1 || items[0].ID != pA2.ID {
		t.Fatalf("courseA+active=false: total=%d items=%+v", total, items)
	}

	// Пагинация: total не зависит от страницы; page за пределами — пусто.
	items, total = s.ListAdminPromoCodes(repo.AdminPromoFilter{
		CourseID: courseA.ID, Page: 1, Limit: 1,
	})
	if total != 2 || len(items) != 1 || items[0].ID != pA2.ID {
		t.Fatalf("page=1&limit=1: total=%d items=%+v", total, items)
	}
	items, total = s.ListAdminPromoCodes(repo.AdminPromoFilter{
		CourseID: courseA.ID, Page: 2, Limit: 1,
	})
	if total != 2 || len(items) != 1 || items[0].ID != pA1.ID {
		t.Fatalf("page=2&limit=1: total=%d items=%+v", total, items)
	}
	items, total = s.ListAdminPromoCodes(repo.AdminPromoFilter{
		CourseID: courseA.ID, Page: 99, Limit: 1,
	})
	if total != 2 || len(items) != 0 {
		t.Fatalf("page=99: total=%d len=%d", total, len(items))
	}
	// Несуществующий курс — пустой (не nil) список.
	items, total = s.ListAdminPromoCodes(repo.AdminPromoFilter{CourseID: uuid.NewString()})
	if total != 0 || items == nil || len(items) != 0 {
		t.Fatalf("несуществующий курс: total=%d items=%#v", total, items)
	}

	// Частичное обновление: discountPct — остальные поля не меняются.
	discount := 25
	row, err = s.UpdateAdminPromoCode(pA1.ID, repo.AdminPromoPatch{DiscountPct: &discount})
	if err != nil {
		t.Fatalf("Update discountPct: %v", err)
	}
	if row.DiscountPct != 25 || row.MaxUses != 100 || row.UsedCount != 0 ||
		row.ExpiresAt == nil || !sameMicro(*row.ExpiresAt, expiresA) || !row.Active ||
		row.CourseTitle != courseA.Title || row.CourseSlug != courseA.Slug {
		t.Fatalf("после discountPct: %+v", row)
	}

	// maxUses — только лимит.
	maxUses := 50
	row, err = s.UpdateAdminPromoCode(pA1.ID, repo.AdminPromoPatch{MaxUses: &maxUses})
	if err != nil {
		t.Fatalf("Update maxUses: %v", err)
	}
	if row.MaxUses != 50 || row.DiscountPct != 25 || row.Active != true {
		t.Fatalf("после maxUses: %+v", row)
	}

	// expiresAt — новый срок, затем очистка (указатель на nil).
	newExpires := time.Now().UTC().Add(96 * time.Hour).Truncate(time.Microsecond)
	expPtr := &newExpires
	row, err = s.UpdateAdminPromoCode(pA1.ID, repo.AdminPromoPatch{ExpiresAt: &expPtr})
	if err != nil {
		t.Fatalf("Update expiresAt: %v", err)
	}
	if row.ExpiresAt == nil || !sameMicro(*row.ExpiresAt, newExpires) {
		t.Fatalf("после expiresAt: %+v", row.ExpiresAt)
	}
	var nilTime *time.Time
	row, err = s.UpdateAdminPromoCode(pA1.ID, repo.AdminPromoPatch{ExpiresAt: &nilTime})
	if err != nil {
		t.Fatalf("Clear expiresAt: %v", err)
	}
	if row.ExpiresAt != nil {
		t.Fatalf("expiresAt не очищен: %+v", row.ExpiresAt)
	}

	// active=false / true; отключение не удаляет запись.
	active := false
	row, err = s.UpdateAdminPromoCode(pA1.ID, repo.AdminPromoPatch{Active: &active})
	if err != nil {
		t.Fatalf("Update active=false: %v", err)
	}
	if row.Active {
		t.Fatalf("active=false не применился: %+v", row)
	}
	items, total = s.ListAdminPromoCodes(repo.AdminPromoFilter{
		CourseID: courseA.ID, Active: &active,
	})
	if total != 2 {
		t.Fatalf("после отключения A1: active=false total=%d, ждали 2", total)
	}
	active = true
	row, err = s.UpdateAdminPromoCode(pA1.ID, repo.AdminPromoPatch{Active: &active})
	if err != nil {
		t.Fatalf("Update active=true: %v", err)
	}
	if !row.Active {
		t.Fatalf("active=true не применился: %+v", row)
	}

	// Обновление всех полей одним патчем + использованный лимит.
	for i := 0; i < 3; i++ {
		if !s.IncrementPromoCodeUsage(pA1.ID) {
			t.Fatal("IncrementPromoCodeUsage вернул false")
		}
	}
	discount, maxUses = 30, 3
	row, err = s.UpdateAdminPromoCode(pA1.ID, repo.AdminPromoPatch{
		DiscountPct: &discount, MaxUses: &maxUses, Active: &active,
	})
	if err != nil {
		t.Fatalf("Update all fields: %v", err)
	}
	if row.DiscountPct != 30 || row.MaxUses != 3 || row.UsedCount != 3 ||
		!row.Active || row.ExpiresAt != nil || row.Code != codeA1 {
		t.Fatalf("после общего патча: %+v", row)
	}
	got, ok := s.GetPromoCodeByID(pA1.ID)
	if !ok || got.DiscountPct != 30 || got.MaxUses != 3 || got.UsedCount != 3 {
		t.Fatalf("GetPromoCodeByID после патча: ok=%v p=%+v", ok, got)
	}

	// Пустой патч — no-op: строка возвращается без ошибки (handler такой
	// запрос отсекает 400-й до стора).
	row, err = s.UpdateAdminPromoCode(pA1.ID, repo.AdminPromoPatch{})
	if err != nil || row == nil || row.ID != pA1.ID {
		t.Fatalf("пустой патч: err=%v row=%+v", err, row)
	}

	// Несуществующий и мусорный id — ErrNotFound.
	if _, err := s.UpdateAdminPromoCode(uuid.NewString(), repo.AdminPromoPatch{Active: &active}); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("несуществующий id: err=%v, ждали ErrNotFound", err)
	}
	if _, err := s.UpdateAdminPromoCode("not-a-uuid", repo.AdminPromoPatch{Active: &active}); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("мусорный id: err=%v, ждали ErrNotFound", err)
	}
}
