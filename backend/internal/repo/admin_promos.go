package repo

// T89 — типы глобального журнала промокодов для супер-админа.
//
// Контракт заморожен в Task/00-api-contract.md (раздел «T89 — глобальное
// управление промокодами»): список всех кодов платформы с фильтрами по курсу,
// строке поиска и статусу, частичное обновление скидки/лимита/срока/активности.
// Реализации StoreAPI: store_admin_promos.go (in-memory) и
// postgres_admin_promos.go (SQL).

import "time"

// AdminPromoFilter — параметры GET /admin/promo-codes.
//
// CourseID "" — любой курс; Q ищет подстроку без учёта регистра по коду ИЛИ
// названию курса; Active nil — любой статус. Пагинация 1-based: Limit по
// умолчанию 20, максимум 100; Page по умолчанию 1. Total считается до
// пагинации.
type AdminPromoFilter struct {
	CourseID string
	Q        string
	Active   *bool
	Page     int
	Limit    int
}

// AdminPromoRow — строка журнала: все поля PromoCode из контракта T63 плюс
// данные курса (courseTitle/courseSlug) для таблицы админки. Курс подтягивается
// join-ом в Postgres и из карты курсов в in-memory Store.
type AdminPromoRow struct {
	PromoCode
	CourseTitle string `json:"courseTitle"`
	CourseSlug  string `json:"courseSlug"`
}

// AdminPromoPatch — частичное обновление промокода (PATCH /admin/promo-codes/{id}).
// nil-указатель означает «поле не менять».
//
// ExpiresAt — трёхзначное значение: nil — срок не менять; указатель на nil —
// очистить срок (expiresAt=null); указатель на время — установить новый срок.
// Такая форма позволяет отличить отсутствующее поле от явного null, которое
// обычный *time.Time не различает.
type AdminPromoPatch struct {
	DiscountPct *int
	MaxUses     *int
	ExpiresAt   **time.Time
	Active      *bool
}
