# T46 — Postgres: тесты каскада удаления курса и миграций (backend)

**Волна:** P5 · **Ветка:** `task/T46-pg-cascade` · **Worktree:** `wt/t46`

## Зачем

Хвост честного списка T20 («Postgres-стор целиком») и отчёта T34: каскад
удаления курса и идемпотентность миграций на живом Postgres автотестами не
покрыты. Закрываем — без изменения API.

## Что сделать

1. Новый `backend/tests/postgres/cascade_test.go` (пакет `postgres_test`;
   хелперы брать из `helper_test.go` — `testStore`, `seedCourseWithLessons`,
   `uniqueSlug`, создание пользователей/платежей; фактические имена посмотри там):
   - `TestPostgresDeleteCourseCascade`: преподаватель + курс с модулями и уроками,
     ученик, запись (`enroll`), платёж (`CreatePayment` + `SetPaymentStatus(..., succeeded)`);
     затем `s.DeleteCourse(course.ID)` и проверки:
     - `GetCourseByID` → `ok=false`; уроки курса — пусто (по доступному методу);
     - запись ученика на курс — снята (`GetEnrollment`/список записей ученика);
     - `ListPaymentsByCourse(course.ID)` → пусто; `ListPaymentsByStudent(student)`
       не содержит платежей удалённого курса;
   - `TestPostgresMigrationsIdempotent`: `migrate.ApplyMigrations` на том же пуле
     повторно (2 раза) → без ошибок; дополнительные проверки — на твоё усмотрение.
   - Правила `helper_test.go` соблюдать: только свои данные (префикс прогона),
     никаких `TRUNCATE`; без `TEST_DATABASE_URL` — `t.Skip`.
2. Если каскад в Postgres отличается от ожиданий — тест фиксирует **фактическое**
   поведение, расхождение с in-memory описать в отчёте (код хранилища не чинить).
3. Отчёт `backend/tests/T46-postgres-cascade.md`.

## Правила волны

- Работать только в своём worktree; коммит — в свою ветку `task/T46-pg-cascade`.
- Править только: `backend/tests/postgres/cascade_test.go` (новый),
  отчёт `backend/tests/T46-postgres-cascade.md`. `helper_test.go` — только читать.
- `backend/internal/**`, `frontend/**`, `Task/**`, `specs/**` — не трогать.
  Новых зависимостей нет.

## Проверка (Postgres — временный контейнер, после — прибрать)

```powershell
docker network create t46-net
docker run -d --name t46-pg --network t46-net -e POSTGRES_USER=edu -e POSTGRES_PASSWORD=edu -e POSTGRES_DB=edu postgres:16-alpine
# дождаться готовности: docker exec t46-pg pg_isready -U edu -d edu
docker run --rm --network t46-net -v "${PWD}\backend:/src" -v p5-gomod:/go/pkg/mod -w /src -e GOFLAGS=-mod=mod -e TEST_DATABASE_URL="postgres://edu:edu@t46-pg:5432/edu?sslmode=disable" golang:1.24-alpine sh -c "go build ./... && go vet ./... && go test ./tests/postgres -count=1 -v"
docker rm -f t46-pg
docker network rm t46-net
```

В отчёт: PASS новых тестов; плюс прогон без `TEST_DATABASE_URL`
(`go test ./... -count=1`) — `exit 0`, пропуски PG-тестов — ок.

## Отчёт

`backend/tests/T46-postgres-cascade.md`: какие каскады/методы проверены, вывод
прогонов, честный список непроверенного.
