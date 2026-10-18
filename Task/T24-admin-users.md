# T24 — Бэкенд: супер-админ — управление пользователями

Волна: P3. Зависимости: нет. Потребитель: T26 (UI «Пользователи»).

## Зачем

В админке **нет раздела пользователей вообще**. При этом `SetUserRole` в хранилище
уже реализован и, по комментарию в `store_iface.go:18-20`, «не вызывается ни одним
хендлером: API для будущего». Роль учителя выдаётся только через инвайт. Заблокировать
нарушителя, разжаловать, удалить мусорный аккаунт — невозможно.

Это ядро раздела «супер-админ».

## Владеет файлами (менять можно только их)

- `backend/internal/handler/admin_users.go` (новый)
- `backend/internal/repo/users_admin.go` (новый — методы на `*Store`)
- `backend/internal/repo/postgres_users_admin.go` (новый — методы на `*PostgresStore`)
- `backend/internal/repo/store_iface.go` (+4 метода в `StoreAPI`)
- `backend/internal/repo/models.go` (добавить `UserStatus` и поле `Status` у `User`)
- `backend/internal/repo/store.go` (заполнить `Status` при создании пользователя)
- `backend/internal/repo/postgres_store.go` (читать/писать колонку `status` в запросах пользователя)
- `backend/internal/handler/auth.go` (отказ во входе заблокированному)
- `backend/internal/handler/middleware.go` (+1 строка монтирования в `r.Route("/admin", ...)`
  и проверка блокировки в `requireAuth`)
- `backend/internal/migrate/migrations/006_user_status.sql` (новый)
- `backend/tests/contract/admin_users_test.go` (новый)

## Модель и миграция

```go
type UserStatus string

const (
	UserActive  UserStatus = "active"
	UserBlocked UserStatus = "blocked"
)
```

`User` получает поле `Status UserStatus`. В in-memory пустая строка `""`
трактуется как активный (для старых объектов) — но `CreateUser` должен явно
ставить `UserActive`.

Миграция `006_user_status.sql` — **идемпотентная** (runner выполняет файл целиком,
повторный запуск безопасен):

```sql
ALTER TABLE users ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
-- CHECK добавляем отдельно: ALTER TABLE ... ADD CONSTRAINT не идемпотентен.
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_status_check') THEN
    ALTER TABLE users ADD CONSTRAINT users_status_check CHECK (status IN ('active','blocked'));
  END IF;
END $$;
```

## Хранилище

```go
// ListUsers — фильтрованный список пользователей для админки (T24).
// Q — подстрока по email/displayName (регистронезависимо), Role/Status — точное
// совпадение, "" — любой. Новые первыми (CreatedAt desc).
ListUsers(f UserFilter) []*User

// SetUserStatus активен/заблокирован; false — пользователя нет (T24).
SetUserStatus(id string, status UserStatus) bool

// DeleteUser удаляет пользователя; ErrNotFound — нет такого (T24).
// Каскад по сессиям/отзывам/прогрессу; курсы и платежи НЕ удаляются —
// handler обязан отказать (409), если они есть (см. UserDependencies).
DeleteUser(id string) error

// UserDependencies — счётчики связей пользователя для защиты от удаления (T24).
UserDependencies(id string) UserDeps

// CountUsersByRole — сколько пользователей с ролью (защита последнего админа, T24).
CountUsersByRole(role Role) int
```

```go
type UserFilter struct {
	Q      string
	Role   Role
	Status UserStatus
}

type UserDeps struct {
	Courses     int
	Payments    int
	Enrollments int
}
```

Итого **5 методов** (в списке выше опечатка округления нет — считай по факту).
Проверка блокировки: `requireAuth` после `GetUserByID` — если `Status == UserBlocked`,
ответ `403` «Учётная запись заблокирована». Ту же проверку добавь при входе
(`auth.go`) — иначе пользователь получит сессию, которая сразу не работает.

## API (внутри `/api/v1/admin`)

### GET `/users?q=&role=&status=&page=&limit=`

200:

```json
{
  "items": [
    {
      "id": "u-1", "email": "ivan@example.com", "role": "student",
      "displayName": "Иван", "status": "active", "createdAt": "2026-01-01T00:00:00Z",
      "coursesCount": 0, "enrollmentsCount": 3
    }
  ],
  "total": 1, "page": 1, "limit": 20
}
```

- `role` вне `student|teacher|admin` → 400. `status` вне `active|blocked` → 400.
  Пустые значения = «любой».
- `items` — всегда массив (не `null`).
- Пагинация — существующим `paginate`.
- `coursesCount` (курсы, где пользователь преподаватель) и `enrollmentsCount`
  берутся из `UserDependencies` по странице.

### PATCH `/users/{id}/role`

Тело: `{"role":"teacher"}`.
- 200 → `{"user":{...}}` в форме строки списка (без `coursesCount`/`enrollmentsCount`
  можно, но лучше те же поля — фронт переиспользует тип).
- 400 — роль вне трёх допустимых.
- 404 — нет пользователя.
- 409 — «Нельзя менять собственную роль» (защита от запирания себя).
- 409 — «Нельзя снять роль администратора с последнего администратора».
- Идемпотентность: постановка уже действующей роли — 200, без ошибки.

### PATCH `/users/{id}/status`

Тело: `{"status":"blocked"}`.
- 200 → `{"user":{...}}`.
- 400 — статус вне `active|blocked`.
- 404 — нет пользователя.
- 409 — «Нельзя заблокировать собственную учётную запись».
- 409 — «Нельзя заблокировать последнего администратора».
- Идемпотентность: уже такой статус → 200.
- При блокировке — не удалять сессии вручную: `requireAuth` уже отсечёт.

### DELETE `/users/{id}`

- 204 без тела.
- 404 — нет пользователя.
- 409 — «Нельзя удалить собственную учётную запись».
- 409 — «Нельзя удалить последнего администратора».
- 409 — «Нельзя удалить пользователя с курсами» (если `userDeps.Courses > 0`).
- 409 — «Нельзя удалить пользователя с платежами: история оплат должна сохраниться».
- Каскадно чистятся сессии, прогресс, отзывы, записи (`enrollments`).

Правило кодов: 400 — валидация тела, 409 — конфликт с состоянием/защита,
403 — заблокированный вход, 404 — нет сущности.

## Тесты (обязательно)

`backend/tests/contract/admin_users_test.go` сквозь `NewRouter` (админ авторизуется
через `POST /auth/login`), не менее:

1. Список: `items` не `null`, пагинация работает, `q` фильтрует по email и имени.
2. Фильтр по `role` и по `status`; мусорный `role` → 400.
3. Смена роли: student → teacher, роль реально изменилась (проверить `GET /users`).
4. Смена собственной роли → 409; блокировка себя → 409; удаление себя → 409.
5. Последний админ: разжаловать/заблокировать/удалить → 409.
6. Блокировка ученика → его `POST /auth/login` возвращает **403**, а существующая
   сессия перестаёт пускать (`GET /auth/me` → 403). Разблокировка возвращает доступ.
7. Удаление свободного пользователя → 204, он пропадает из списка.
8. Удаление пользователя с платежом → 409.

## Критерии приёмки

- `go build ./... && go vet ./... && go test ./... -count=1` — зелёные (Docker).
- `go test -race` — зелёный в `golang:1.24-bookworm`.
- Миграция применяется на живом Postgres и повторный запуск не падает.
- Существующие тесты не изменены; `seedAdmin`/`seedDemo` не сломаны.

## Проверка

```
docker run --rm -v "<путь>\backend:/src" -v gomodcache:/go/pkg/mod -w /src ^
  -e GOFLAGS=-mod=mod golang:1.24-alpine sh -c "go build ./... && go vet ./... && go test ./... -count=1"
```
