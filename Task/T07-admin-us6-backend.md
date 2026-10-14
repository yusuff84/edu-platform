# T07 — US6: журнал платежей и настройка комиссии (backend)

**Ключ сессии:** `T07-admin-us6-backend`
**Волна:** 1 · **Слой:** backend (Go)
**Приоритет:** P0 — US6 в `converge-report.md` помечен как «partial»

## Проблема

Спека US6 требует журнал платежей и управление комиссией платформы
(`spec.md:96`, `converge-report.md:96-97`). Фактически:

- маршрута `GET /admin/payments` **нет**;
- маршрута `PUT /admin/settings` **нет**;
- `StoreAPI.SetCommissionPct` (`repo/store_iface.go`, `repo/store.go:636`,
  `repo/postgres_store.go:967`) реализован в обоих сторах, но **не вызывается ни одним хендлером**;
- запроса «все платежи» нет ни в одном сторе;
- на фронте `Admin.tsx:100-103` рисует заглушку «журнал платежей пуст».

## Владение файлами (жёсткая граница)

```
backend/internal/repo/store_iface.go      — расширить интерфейс
backend/internal/repo/store.go            — in-memory реализация
backend/internal/repo/postgres_store.go   — pgx реализация
backend/internal/handler/admin_payments.go — СОЗДАТЬ (новый файл)
```
`backend/internal/handler/middleware.go` **не трогать** — маршруты зарегистрирует T13.
Вместо этого в `admin_payments.go` добавь экспортируемый метод:

```go
func (a *App) RegisterAdminPaymentsRoutes(r chi.Router) {
    r.Get("/admin/payments", a.AdminPaymentsJournal)
    r.Get("/admin/settings", a.AdminGetSettings)
    r.Put("/admin/settings", a.AdminUpdateSettings)
}
```

## Что сделать

### 1. Слой repo

Добавить в `StoreAPI`:
```go
type PaymentFilter struct {
    Page, Limit int
    Status      PaymentStatus // "" = любой
    CourseID    string        // "" = любой
}
type PaymentJournalRow struct {
    Payment
    CourseTitle string
}
ListPaymentsJournal(f PaymentFilter) (items []PaymentJournalRow, total int, err error)
```
Требования к обеим реализациям:
- сортировка по `created_at DESC`;
- `total` — количество **до** пагинации;
- пагинация 1-based, `limit` по умолч. 20, максимум 100 (переиспользуй логику `handler.paginate` или вынеси её — но `middleware.go` не трогай, сделай локальный хелпер в `admin_payments.go`);
- фильтры `status` и `courseId` должны комбинироваться через `AND`;
- `nil`-срезы заменять на пустые, чтобы JSON был `[]`, а не `null`;
- **не глотать ошибки**: если запрос упал — вернуть `err`. В `postgres_store.go`
  сейчас почти все методы возвращают `return nil, false` при ошибке;
  в новом методе так делать **нельзя**.

### 2. Слой handler — `admin_payments.go`

- `AdminPaymentsJournal`: `requireRole(admin)` через существующий middleware
  (он вешается в `NewRouter`, проверку роли делает middleware-цепочка —
  вызывай `a.currentUser(r)` и проверяй `u.Role == repo.RoleAdmin` вручную,
  если цепочка ещё не добавлена).
  Query-параметры: `page`, `limit`, `status`, `courseId`.
  Ответ ровно по контракту `Task/00-api-contract.md`, раздел «Новые эндпоинты US6»:
  `{ items, total, page, limit, totals: { amountCents, platformFeeCents, teacherAmountCents } }`.
  `totals` считаются **по всем платежам с учётом фильтров, но без пагинации**.
- `AdminGetSettings` → `{ "commissionPct": N }`
- `AdminUpdateSettings` → тело `{ "commissionPct": N }`, валидация `0..90`,
  запись в `app_settings` (`SetCommissionPct`) и **обновление кэша в памяти**
  (проверь, как `GetCommissionPct` работает в обоих сторах — значение должно
  применяться немедленно, без перезапуска).
- Ответы об ошибках — через существующие `writeJSON`/`writeErr`.
- Документируй каждый хендлер в стиле остальных файлов (комментарий `// X — GET /path: ...`).

### 3. Обновить контракт

В `specs/001-it-courses-mvp/contracts/openapi.yaml` **не редактировать** —
это артефакт SDD. Вместо этого отметь в отчёте, что T13 должен будет
подтянуть новые пути.

## Критерии приёмки

- [ ] `cd backend && go build ./...` без ошибок
- [ ] `cd backend && go vet ./...` чисто
- [ ] `cd backend && go test ./...` — существующие тесты не сломаны
- [ ] `var _ StoreAPI = (*Store)(nil)` и `var _ StoreAPI = (*PostgresStore)(nil)`
      в `store_iface.go` по-прежнему компилируются (значит, интерфейс
      реализован полностью в обоих сторах)
- [ ] `SetCommissionPct` теперь вызывается из хендлера
- [ ] Новый метод не глотает ошибки БД
- [ ] `items` никогда не `null`

## Проверка

```bash
cd backend
go build ./...
go vet ./...
go test ./...
```

Дополнительно вручную (при поднятой БД или in-memory):
```bash
curl -s localhost:8082/api/v1/healthz
# затем с cookie админа:
curl -s -b cookies.txt localhost:8082/api/v1/admin/payments
curl -s -b cookies.txt localhost:8082/api/v1/admin/settings
curl -s -X PUT -b cookies.txt -H 'Content-Type: application/json' \
     -d '{"commissionPct":20}' localhost:8082/api/v1/admin/settings
```

## Отчёт

Опиши сигнатуры добавленных методов, схему ответа журнала,
и точные координаты (файл:строка), куда T13 должен вставить вызов
`a.RegisterAdminPaymentsRoutes(...)` в `NewRouter`.
