# T93 — Бэкенд: сессии пользователя и принудительный выход

**Волна:** P10  **Ветка:** `task/T93-admin-sessions`  **Worktree:** `wt/t93`

## Суть

Супер-админ может заблокировать пользователя, но активные cookie-сессии
продолжают работать до истечения. Нужен просмотр активных сессий и
принудительный выход. Контракт — `GET/DELETE /admin/users/{id}/sessions` в
«Волна P10» `Task/00-api-contract.md` (заморожен).

Работать только в `wt/t93` (ветка `task/T93-admin-sessions`), база —
`integration/p10`.

## Что сделать

1. StoreAPI (`backend/internal/repo/store_iface.go`) добавьте:
   - `ListSessionsByUser(userID string) []*Session` — только не истёкшие,
     новые первыми;
   - `DeleteSessionsByUser(userID string) int` — удаляет все сессии, возвращает
     количество (для логов/тестов), не падает при нуле.
   Реализуйте в in-memory (`store_admin_sessions.go`) и Postgres
   (`postgres_admin_sessions.go`). В Postgres используйте `expires_at > now()`
   и `ORDER BY created_at DESC`; удаление — по `user_id`.
2. Новый handler `backend/internal/handler/admin_sessions.go`,
   `RegisterAdminSessionsRoutes(r chi.Router)`:
   - `GET /users/{id}/sessions` — admin, 404 если пользователя нет, ответ
     `{items:[{id,createdAt,expiresAt}]}`, `items` не nil;
   - `DELETE /users/{id}/sessions` — admin, 404, `204`.
   Подключите внутри `/admin` в `middleware.go` (рядом с admin users).
3. Не логируйте токены/ID сессий в открытом виде сверх ответа; не добавляйте
   секреты в JSON.
4. Тесты `backend/tests/contract/admin_sessions_test.go`: логин пользователя
   создаёт сессию, админ видит её; после DELETE сессия не проходит
   `requireAuth`; чужая сессия не затрагивается; 404/401/403; истёкшие не
   возвращаются (можно проверить через короткий TTL/прямой store в unit-тесте).
   Postgres parity — `backend/tests/postgres/admin_sessions_test.go` (порт
   55454).
5. Приписка в `docs/API.md`.

## Разрешённые файлы

- `backend/internal/repo/store_iface.go`
- `backend/internal/repo/store_admin_sessions.go` (новый)
- `backend/internal/repo/postgres_admin_sessions.go` (новый)
- `backend/internal/handler/admin_sessions.go` (новый)
- `backend/internal/handler/middleware.go`
- `backend/tests/**`, `docs/API.md`
- `backend/tests/T93-admin-sessions.md` (отчёт)

## Запрещено

- `Task/**`, `specs/**`, `frontend/**`, `ru.json`, миграции (таблица sessions
  уже есть), `docker-compose*`, `qa-*.ps1`, `.github/**`, `deploy/**`.
- Менять формат cookie/аутентификации.
- `git push`, `git merge`, смена ветки, работа вне `wt/t93`.

## Проверка

```
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Postgres-parity: `postgres:16-alpine`, имя `p10-t93-pg`, порт **55454**
(edu/edu, edu_test), хост `host.docker.internal`; после прогона удалить.

## Отчёт

`backend/tests/T93-admin-sessions.md`: что сделано, проверка, отклонения,
открытые вопросы. Коммит явными путями.
