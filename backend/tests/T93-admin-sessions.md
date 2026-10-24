# T93 — Бэкенд: сессии пользователя и принудительный выход

**Волна:** P10  **Ветка:** `task/T93-admin-sessions`  **Worktree:** `wt/t93`

## Что сделано

1. **StoreAPI** (`backend/internal/repo/store_iface.go`): в блок Sessions
   добавлены методы:
   - `ListSessionsByUser(userID string) []*Session` — только не истёкшие
     сессии, новые первыми; срез никогда не nil;
   - `DeleteSessionsByUser(userID string) int` — удаляет все сессии
     пользователя (включая уже истёкшие), возвращает количество; 0 — не
     ошибка.
2. **In-memory** (`backend/internal/repo/store_admin_sessions.go`, новый):
   `ListSessionsByUser` — один проход по карте под `RLock`, фильтр
   `ExpiresAt > now`, сортировка `CreatedAt desc`, при равенстве — `id desc`
   (детерминизм при грубых системных часах Windows); `DeleteSessionsByUser`
   — проход под `Lock`, удаление по `UserID`, возврат счётчика.
3. **Postgres** (`backend/internal/repo/postgres_admin_sessions.go`, новый):
   `SELECT ... WHERE user_id=$1::uuid AND expires_at > now()
   ORDER BY created_at DESC, id DESC` (срез инициализируется `make`, ошибка
   чтения логируется и даёт пустой не-nil срез); удаление —
   `DELETE FROM sessions WHERE user_id=$1::uuid` с `RowsAffected()`.
4. **Хендлер** (`backend/internal/handler/admin_sessions.go`, новый):
   `RegisterAdminSessionsRoutes`:
   - `GET /users/{id}/sessions` — `adminOnly`, 404 «Пользователь не найден»
     для отсутствующего/некорректного id, ответ
     `200 {"items":[{"id","createdAt","expiresAt"}]}`, `items` всегда массив;
   - `DELETE /users/{id}/sessions` — `adminOnly`, 404, успех `204` без тела;
     удаляет и текущую сессию админа, если он завершает свои. В лог пишется
     только количество отозванных сессий (без id пользователя и сессий).
5. **Регистрация** (`backend/internal/handler/middleware.go`): вызов
   `RegisterAdminSessionsRoutes(r)` внутри `r.Route("/admin", ...)` рядом с
   `RegisterAdminUsersRoutes`; `requireAuth` + `requireRole(admin)` уже
   навешаны группой.
6. **Контрактные тесты** (`backend/tests/contract/admin_sessions_test.go`):
   - вход создаёт сессию, админ видит её; регистрация + повторный логин дают
     две сессии, порядок «новые первыми», форма элемента ровно
     `{id,createdAt,expiresAt}`, `items` не null;
   - истёкшая сессия (создана напрямую в store с отрицательным TTL) в ответ
     не попадает;
   - после DELETE обе cookie не проходят `requireAuth` (`/auth/me`,
     `/me/courses` → 401), список пуст, повторный DELETE — 204;
   - сессии другого ученика и админа не затрагиваются;
   - 401 анониму, 403 ученику (даже на свои сессии), 404 для
     несуществующего и некорректного id в обоих методах;
   - удаление собственных сессий админом: старая cookie → 401, повторный вход
     создаёт новую видимую сессию.
7. **Postgres parity** (`backend/tests/postgres/admin_sessions_test.go`):
   пустой не-nil список и 0 при удалении без сессий; две активные + истёкшая
   + чужая сессии; проверка `expires_at > now()`, порядка `created_at DESC`,
   сохранности времён (микросекундная точность), удаления ровно 3 сессий
   своего пользователя, идемпотентного повтора (0) и неприкосновенности
   чужой сессии.
8. **Документация** (`docs/API.md`): блок «Волна P10 (T93)» после таблицы
   админских эндпоинтов — формы ответов, сортировка, 204, 401/403/404.
9. **Отчёт** — этот файл.

## Проверка

1. Полный прогон (Docker, `golang:1.24-alpine`):

```
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod \
  -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Результат: `go vet` без замечаний; все пакеты `ok`, в том числе
`internal/handler`, `internal/repo`, `tests/contract` (39.6 s) и
`tests/postgres` (без `TEST_DATABASE_URL` — штатный SKIP).

2. Postgres parity (контейнер `p10-t93-pg`, порт **55454**,
`postgres:16-alpine`, `edu/edu`, `edu_test`):

```
docker run --rm -e HOME=/root \
  -e "TEST_DATABASE_URL=postgres://edu:edu@host.docker.internal:55454/edu_test?sslmode=disable" \
  -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine \
  sh -c "go test ./tests/postgres/... -count=1 -v -run 'AdminSessions|AdminUsers|ListUsers|UserDependencies'"
```

`TestPostgresAdminSessions`, `TestPostgresListUsersFilters`,
`TestPostgresUserDependenciesAndDelete` — PASS. Дополнительно весь пакет
`./tests/postgres/...` — `ok` (14.9 s). Контейнер `p10-t93-pg` после прогона
удалён.

3. Форматирование: новые файлы `gofmt`-чистые. Существующие файлы в рабочем
   дереве имеют CRLF, поэтому `gofmt -l` перечисляет их и до правок; массовое
   переформатирование не выполнялось.

## Отклонения

- В сортировку добавлен вторичный ключ `id DESC` после `created_at DESC` —
  контрактом не оговорён, нужен для детерминированного порядка сессий,
  созданных в одну микросекунду, и полного parity in-memory/Postgres.
- `DeleteSessionsByUser` удаляет и уже истёкшие записи, оставшиеся в
  хранилище, и включает их в возвращаемое количество: контракт говорит «все
  сессии пользователя». Наружу количество не отдаётся (только лог).
- Проверка «истёкшие не возвращаются» сделана в контрактном тесте через
  прямой вызов `CreateSession` с отрицательным TTL (разрешено постановкой);
  отдельный unit-тест в `internal/repo` не добавлялся, чтобы не расширять
  список разрешённых файлов.
- В `docs/API.md` не было раздела `/admin/users` (T24 не документировал свои
  эндпоинты); приписка T93 добавлена отдельным блоком после таблицы админских
  маршрутов.

## Открытые вопросы

Нет. `Task/**`, `specs/**`, `frontend/**`, i18n, миграции,
`docker-compose*`, `qa-*.ps1`, `.github/**`, `deploy/**` не затрагивались;
формат cookie и аутентификации не менялся.
