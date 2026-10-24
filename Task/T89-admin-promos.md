# T89 — Бэкенд: глобальное управление промокодами

**Волна:** P10  **Ветка:** `task/T89-admin-promos`  **Worktree:** `wt/t89`

## Суть

Промокоды сейчас создаются и отключаются только в конструкторе конкретного
курса. Супер-админу нужен глобальный журнал: поиск, фильтр по курсу/статусу,
редактирование скидки/лимита/срока и включение/отключение. Контракт —
`GET/PATCH /admin/promo-codes` в «Волна P10» `Task/00-api-contract.md`
(заморожен).

Работать только в `wt/t89` (ветка `task/T89-admin-promos`), база —
`integration/p10`.

## Что сделать

1. Новый файл `backend/internal/repo/admin_promos.go`: типы
   `AdminPromoFilter` (`CourseID`, `Q`, `Active *bool`, `Page`, `Limit`),
   `AdminPromoRow` (все поля контракта + `courseTitle`, `courseSlug`),
   `AdminPromoPatch` (`DiscountPct`, `MaxUses`, `ExpiresAt` c признаком
   «не менять»/«очистить», `Active` — указатели).
2. StoreAPI: `ListAdminPromoCodes(f) ([]*AdminPromoRow, int)` и
   `UpdateAdminPromoCode(id string, p AdminPromoPatch) (*AdminPromoRow, error)`.
   Реализуйте в отдельных `store_admin_promos.go` / `postgres_admin_promos.go`;
   Postgres — один UPDATE, затем чтение с join курса.
3. Handler `backend/internal/handler/admin_promos.go`,
   `RegisterAdminPromosRoutes(r chi.Router)`:
   - `GET /promo-codes` — query/валидация, 400/401/403, ответ по контракту;
   - `PATCH /promo-codes/{id}` — `DisallowUnknownFields`, хотя бы одно поле,
     валидация (1..90, maxUses >= usedCount, expiresAt в будущем или null,
     archived course → 409), 404, ответ `{"promoCode": ...}`.
   Подключите внутри `/admin` в `middleware.go`.
4. Тесты `backend/tests/contract/admin_promos_test.go`: список/фильтры/q/
   пагинация/total, частичное обновление каждого поля, очистка expiresAt,
   включение/отключение, 400 (пустое тело, неизвестное поле, неверные
   значения), 404, 409 archived, 401/403, использованный лимит. Postgres
   parity — `backend/tests/postgres/admin_promos_test.go` (порт 55453).
5. Приписка в `docs/API.md`.

## Разрешённые файлы

- `backend/internal/repo/admin_promos.go` (новый)
- `backend/internal/repo/store_admin_promos.go` (новый)
- `backend/internal/repo/postgres_admin_promos.go` (новый)
- `backend/internal/repo/store_iface.go`
- `backend/internal/handler/admin_promos.go` (новый)
- `backend/internal/handler/middleware.go`
- `backend/tests/**`, `docs/API.md`
- `backend/tests/T89-admin-promos.md` (отчёт)

## Запрещено

- `Task/**`, `specs/**`, `frontend/**`, `ru.json`, миграции,
  `docker-compose*`, `qa-*.ps1`, `.github/**`, `deploy/**`.
- Менять существующие teacher-эндпоинты промокодов, кроме общих store-методов.
- `git push`, `git merge`, смена ветки, работа вне `wt/t89`.

## Проверка

```
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Postgres-parity: `postgres:16-alpine`, имя `p10-t89-pg`, порт **55453**
(edu/edu, edu_test), хост `host.docker.internal`; после прогона удалить.

## Отчёт

`backend/tests/T89-admin-promos.md`: что сделано, проверка, отклонения,
открытые вопросы. Коммит явными путями.
