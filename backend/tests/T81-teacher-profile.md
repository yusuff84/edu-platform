# T81 — Бэкенд: публичный профиль преподавателя

**Волна:** P9  **Ветка:** `task/T81-teacher-profile`  **Worktree:** `wt/t81`

## Что сделано

- **Модели** (`backend/internal/repo/models.go`): `TeacherPublic`
  (`id, displayName, coursesCount, studentsCount, avgRating, ratingsCount`) и
  `TeacherProfileData` (`teacher`, `courses []*Course`).
- **StoreAPI** (`backend/internal/repo/store_iface.go`): в конец интерфейса
  добавлен `GetTeacherProfile(teacherID string) (*TeacherProfileData, bool)`.
- **In-memory** (`backend/internal/repo/store.go`): профиль собирается под одним
  `RLock`; роль не teacher/admin → `false`; агрегаты считаются одним проходом
  по курсам и записям (без N+1); курсы отдаются снимками. Порядок: активные
  записи desc, затем `createdAt` desc, затем `id` desc; максимум 100 курсов.
  `coursesCount` — полное число published-курсов (до ограничения 100),
  `studentsCount` — активные записи на published-курсы, `avgRating` —
  средневзвешенное по `avgRating * ratingsCount` с округлением до 1 знака
  (как `RecalcCourseRating`).
- **Postgres** (`backend/internal/repo/postgres_store.go`): три запроса
  постоянного числа (без N+1): пользователь, агрегаты по published-курсам
  (COUNT/SUM + активные записи через `GROUP BY`) и published-курсы с
  подсчётом активных записей в `LEFT JOIN` и `ORDER BY ... LIMIT 100`.
  Мусорный/несуществующий id — `false`.
- **Хендлер** (`backend/internal/handler/teachers.go`): публичный
  `GET /api/v1/teachers/{id}` → `200 {"teacher":{...},"courses":[CourseItem]}`;
  иначе `404 «Преподаватель не найден»`. Курсы конвертируются существующим
  `toCourseItem`.
- **Маршрут** (`backend/internal/handler/middleware.go`): публичная секция
  `/api/v1`, без `requireAuth`.
- **Тесты:**
  - `backend/tests/contract/teacher_profile_test.go`: профиль без входа,
    счётчики (курсы/ученики/рейтинг), только published-курсы, порядок по
    активным записям и `createdAt`, 404 для несуществующего id, student и
    мусорного id, admin допускается, пустой `displayName`, `courses` — массив
    (не `null`).
  - `backend/tests/postgres/teacher_profile_test.go`: parity SQL-ветки —
    те же агрегаты/состав/порядок, пустой профиль, 404-условия.
- **Документация** (`docs/API.md`): строка про `GET /api/v1/teachers/{id}` в
  таблице «Курсы и отзывы».

## Проверка

1. Полный прогон (Docker, `golang:1.24-alpine`):

```
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Результат: `go vet` без замечаний, все пакеты `ok`
(`cmd/api`, `internal/auth`, `internal/handler`, `internal/payments`,
`internal/repo`, `tests/contract`, `tests/postgres` — без
`TEST_DATABASE_URL` parity-тесты штатно пропущены).

2. Postgres parity (контейнер `p9-t81-pg`, порт 55451,
`postgres:16-alpine`, `edu/edu`, `edu_test`):

```
docker run --rm -e HOME=/root \
  -e TEST_DATABASE_URL="postgres://edu:edu@host.docker.internal:55451/edu_test?sslmode=disable" \
  -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine \
  sh -c "go test ./tests/postgres/... -count=1 -v -run 'TeacherProfile'"
```

Результат: `TestPostgresTeacherProfile` и
`TestPostgresTeacherProfileEmptyAndNotFound` — PASS. Дополнительно весь пакет
`./tests/postgres/...` — `ok`. Контейнер после прогона удалён.

## Отклонения

- Контракт говорит «без пагинации (максимум 100)» — ответ ограничен 100
  курсами после сортировки; `coursesCount` при этом остаётся полным числом
  published-курсов.
- В порядок добавлен третичный ключ `id` desc (после активных записей и
  `createdAt`) — для детерминированной выдачи и полного parity in-memory и
  Postgres; первичные ключи контракта не изменены.
- Postgres-реализация использует три запроса постоянного числа вместо одного
  монолитного SQL; N+1 отсутствует.

## Открытые вопросы

Нет.
