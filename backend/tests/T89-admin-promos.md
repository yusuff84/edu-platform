# T89 — глобальное управление промокодами (P10)

**Волна:** P10  **Ветка:** `task/T89-admin-promos`  **Worktree:** `wt/t89`
**Контракт:** `Task/00-api-contract.md`, раздел «T89 — глобальное управление
промокодами» (заморожен)
**Статус:** ✅ готово (код и отчёт закоммичены, ветка не запушена)

## Что сделано

- **Типы** `backend/internal/repo/admin_promos.go` (новый):
  - `AdminPromoFilter` — `CourseID`, `Q`, `Active *bool`, `Page`, `Limit`;
  - `AdminPromoRow` — поля `PromoCode` (id, courseId, code, discountPct,
    maxUses, usedCount, expiresAt, active, createdAt) + `courseTitle`,
    `courseSlug`;
  - `AdminPromoPatch` — указатели `DiscountPct`, `MaxUses`, `Active`;
    `ExpiresAt **time.Time` различает «не менять» (nil), «очистить»
    (указатель на nil) и новый срок (указатель на время).
- **StoreAPI** (`store_iface.go`): добавлены `ListAdminPromoCodes(f)`
  (`[]*AdminPromoRow, int`) и `UpdateAdminPromoCode(id, p)`
  (`*AdminPromoRow, error`).
- **In-memory** `backend/internal/repo/store_admin_promos.go` (новый):
  - фильтры `courseId`, `q` (подстрока без учёта регистра по коду ИЛИ
    названию курса), `active`; AND; `total` до пагинации;
  - порядок `createdAt DESC, id ASC` — как в Postgres;
  - строки — снимки с клонированным `expiresAt`, курс подтягивается из карты
    курсов; частичное обновление трёхзначного срока, `ErrNotFound` для
    несуществующего id.
- **Postgres** `backend/internal/repo/postgres_admin_promos.go` (новый):
  - `ListAdminPromoCodes`: `JOIN courses` (courseTitle/courseSlug), `COUNT(*)`
    по тем же фильтрам, страница `ORDER BY p.created_at DESC, p.id ASC`;
  - `UpdateAdminPromoCode`: один динамический `UPDATE` только по присланным
    полям (`expires_at = NULL` при очистке), затем чтение с join-ом курса;
    не-UUID/несуществующий id — `ErrNotFound`.
- **Handler** `backend/internal/handler/admin_promos.go` (новый),
  `RegisterAdminPromosRoutes`:
  - `GET /admin/promo-codes` — `courseId` (UUID), `q`, `active=true|false`,
    `page/limit` (20 по умолчанию, максимум 100, нечисловые — 400); ответ
    `{items,total,page,limit}`, `items` всегда массив;
  - `PATCH /admin/promo-codes/{id}` — `DisallowUnknownFields`, хотя бы одно
    поле, `discountPct` 1..90, `maxUses >= 0` и `>= usedCount`,
    `expiresAt` RFC3339 в будущем или `null`; 404 — кода нет, 409 — курс в
    архиве (проверка до чтения тела), успех — `200 {"promoCode": <строка>}`;
  - 401/403 — middleware `/admin` + страховка `adminOnly`.
- **Маршруты** (`middleware.go`): `RegisterAdminPromosRoutes(r)` внутри
  существующей группы `r.Route("/admin", ...)`.
- **Тесты**:
  - `backend/tests/contract/admin_promos_test.go` — список, все фильтры,
    поиск по коду/названию без учёта регистра, пагинация/total, форма строки,
    401/403/400; PATCH каждого поля, очистка срока, active=false/true,
    использованный лимит, пустое тело/`{}`/неизвестное поле/неверные
    значения, 404, 409 archived;
  - `backend/tests/postgres/admin_promos_test.go` — SQL-parity: join курса,
    фильтры, порядок, пагинация, частичные UPDATE, очистка `expires_at`,
    active, использованный лимит, `ErrNotFound`.
- **docs/API.md**: строки `GET /admin/promo-codes` и
  `PATCH /admin/promo-codes/{id}` в таблице админ-эндпоинтов.

## Проверка

Все команды — в Docker `golang:1.24-alpine`, Go-код только через контейнер.

### Unit + contract + postgres без БД

```bash
docker run --rm -e HOME=/root \
  -v "${PWD}:/src" -v gomodcache:/go/pkg/mod -w /src \
  golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Результат: `go vet ./...` без замечаний; все пакеты `ok`, включая
`tests/contract`; в `tests/postgres` без `TEST_DATABASE_URL` тесты ожидаемо
пропущены (пакет `ok`).

### Postgres parity

```bash
docker run -d --name p10-t89-pg \
  -e POSTGRES_USER=edu -e POSTGRES_PASSWORD=edu -e POSTGRES_DB=edu_test \
  -p 55453:5432 postgres:16-alpine

docker run --rm -e HOME=/root \
  -e "TEST_DATABASE_URL=postgres://edu:edu@host.docker.internal:55453/edu_test?sslmode=disable" \
  -v "${PWD}:/src" -v gomodcache:/go/pkg/mod -w /src \
  golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Результат: все пакеты `ok`, включая `tests/postgres`; `go vet` без замечаний.
Целевой прогон `go test ./tests/postgres -run TestPostgresAdminPromosParity
-count=1 -v` — PASS. Контейнер `p10-t89-pg` остановлен и удалён после
прогона; порт 55453, отдельная БД `edu_test`.

## Отклонения / трактовки

1. **`maxUses = 0` после использования.** Контракт T89 буквально требует
   `maxUses >= usedCount`, поэтому при `usedCount > 0` установить `0`
   («без лимита») нельзя — это будет `400`. Если бизнес-смысл «0 = без
   лимита» должен побеждать, правило нужно смягчить до
   `maxUses == 0 || maxUses >= usedCount` (потребует правки контракта).
2. **`active=true` при истёкшем сроке.** Реактивация просто ставит
   `active=true`; код с прошедшим `expiresAt` остаётся неприменимым при
   покупке (проверка срока T63), отдельной ошибки контракт не задаёт.
3. **Архивный курс.** Журнал (GET) показывает коды архивных курсов — иначе
   историей нельзя управлять; запрещён только PATCH (`409
   «Курс в архиве: промокоды недоступны»`, текст T63). Проверка архива стоит
   до чтения тела: пустое тело по архивному курсу тоже даёт `409`, а не `400`
   (правило «состояние ресурса важнее валидации тела», как в PATCH карточки).
4. **Пустой PATCH в Store.** Handler отсекает тело без полей `400`; прямой
   вызов `UpdateAdminPromoCode` с пустым патчем — no-op и возвращает текущую
   строку (в Postgres UPDATE не выполняется).
5. **Не-UUID id в PATCH.** В in-memory — обычный `404`; в Postgres мусорный
   id трактуется как «не найдено» до SQL, чтобы не отдавать `500`
   (паттерн `DeleteReview`/`DeactivatePromoCode`).
6. **Пагинация.** «20..100» трактовано как у остальных админ-списков:
   `limit` по умолчанию 20, максимум 100; значения 1..100 принимаются,
   `page < 1` → 1.

## Открытые вопросы

1. Нужно ли разрешать `maxUses = 0` (без лимита) после того, как код уже
   использован? Сейчас — нет, по буквальному правилу контракта (см. п.1
   отклонений).
2. Должен ли `active=true` для кода с истёкшим сроком автоматически очищать
   `expiresAt`? Контракт этого не требует; сейчас срок не меняется.
