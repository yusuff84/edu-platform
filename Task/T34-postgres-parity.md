# T34 — Postgres-паритет: интеграционные тесты волны P3 + счётчик блокировок (backend)

**Волна:** P4 · **Ветка:** `task/T34-postgres-parity` · **Worktree:** `wt/t34`
**Контракт:** `Task/00-api-contract.md`, раздел «Эндпоинты волны P4» → «Закрываемые расхождения».

## Зачем

Новые методы P3 (`admin_users.go`, `me_extra.go`, `teacher_students.go`, `admin_console.go`)
проверяются только сквозным QA на живом стенде; автотесты гоняют in-memory store, а Postgres-ветка
(`postgres_store.go`, `postgres_admin_console.go`, `postgres_course_students.go`, `postgres_me_extra.go`)
ни разу не исполнялась в CI. Это и есть главный оставшийся риск волны.

Плюс конкретный баг: `PostgresStore.AdminOverview()`
(`backend/internal/repo/postgres_admin_console.go:27`) не считает `users.blocked`
(в SQL нет условия `status='blocked'`) — на Postgres сводка админки всегда показывает 0
заблокированных. Комментарии «в схеме нет признака блокировки» устарели: колонка есть
(`006_user_status.sql`).

## Что сделать

1. **Фикс сводки**: в `AdminOverview()` добавить подсчёт заблокированных пользователей
   (`status = 'blocked'`) в `AdminTotals.UsersBlocked`. Обновить устаревшие комментарии в
   `backend/internal/repo/postgres_admin_console.go` (строка ~31) и
   `backend/internal/repo/admin_console.go` (~57, ~113) — блокировка в MVP уже есть (T24).
2. **Интеграционные тесты Postgres** — новый пакет `backend/tests/postgres` (все файлы внутри —
   зона T34). Тесты **env-gated**: если `TEST_DATABASE_URL` не задан, каждый тест делает `t.Skip`
   с понятным текстом — `go test ./...` на машине без Postgres остаётся зелёным.
3. Помощник (`backend/tests/postgres/helper_test.go`): поднять `pgxpool` из `TEST_DATABASE_URL`,
   применить миграции (`internal/migrate` из репозитория, как это делает `cmd/api`:
   `ApplyMigrations`), вернуть `*repo.PostgresStore`. Все данные тестов — со своим префиксом
   (`pgtest-<unix-ts>-…` в email/slug), **никаких** `TRUNCATE`/`DELETE` по общим таблицам:
   тесты обязаны быть безопасными на живом стенде.
4. Покрыть (repo-уровень, через `PostgresStore`, а не HTTP):
   - пользователи: `ListUsers` с `q`/`role`/`status`, `UpdateUserStatus` (блокировка и
     разблокировка), `CountAdmins`, удаление пользователя со своими курсами → отказ;
   - сводка: `AdminOverview()` — `UsersTotal`/`UsersStudents` растут после создания,
     **`UsersBlocked` увеличивается после блокировки** (главная проверка фикса);
   - журнал платежей: `ListPaymentsJournal` с `status=`/`courseId=`; пагинация и `totals`;
   - ученики курса: `ListCourseStudents` (прогресс/percent/lastActivity) и `CourseStats`;
   - кабинет ученика: `ListEnrollmentsByStudent`, `ListPaymentsByStudent`, прогресс по курсу
     (`completedLessonIds`/`nextLessonId`).
   Проверки делать на своих сущностях; ассерты — по конкретным ожидаемым значениям, не «непусто».
5. **README**: раздел «Интеграционные тесты Postgres» — как поднять Postgres и как прогнать тесты
   ровно теми командами, которыми пользовались сами (см. шаблон ниже), включая предупреждение,
   что тесты можно запускать и против стенда, но они не должны его портить.

## Шаблон запуска (проверь на себе и уточни в README)

```powershell
docker run -d --rm --name edu-pgtest -e POSTGRES_USER=edu -e POSTGRES_PASSWORD=edu -e POSTGRES_DB=edu_test -p 55432:5432 postgres:16-alpine
docker run --rm --network host -v "C:\Users\96352\all_project\eduPlatform\edu-platform\backend:/src" -v gomodcache:/go/pkg/mod -w /src -e GOFLAGS=-mod=mod -e TEST_DATABASE_URL="postgres://edu:edu@127.0.0.1:55432/edu_test?sslmode=disable" golang:1.24-alpine sh -c "go test ./tests/postgres/... -count=1 -v"
docker stop edu-pgtest
```

(`--network host` работает в Docker Desktop Windows; если нет — свяжи контейнеры своей сетью.)
Контейнеры стенда (`olymp-*`, `edu-platform-*`) не выключать и не пересоздавать.

## Ограничения

- Править только: `backend/tests/postgres/**` (новое), `backend/internal/repo/postgres_admin_console.go`
  (только `AdminOverview` + комментарии), `backend/internal/repo/admin_console.go` (только комментарии),
  `README.md` (только новый раздел), отчёт `backend/tests/T34-postgres.md`.
- `Task/**`, `frontend/**`, `specs/**`, миграции — не трогать. Новых Go-зависимостей нет
  (`pgxpool` уже в проекте).

## Проверка

1. `go build ./... && go vet ./... && go test ./... -count=1` в alpine-контейнере → `exit 0`,
   пакет `tests/postgres` должен быть `ok` (тесты пропущены — это нормально без env).
2. С `TEST_DATABASE_URL` (postgres-контейнер) — все интеграционные тесты выполняются и проходят;
   в отчёте привести вывод (`-v`) с `PASS` и реальными проверками, а не `SKIP`.

## Отчёт

`backend/tests/T34-postgres.md`: список методов, покрытых интеграционными тестами; доказательство
фикса `UsersBlocked` (до/после, вывод теста); команды запуска; честный список непокрытого.
