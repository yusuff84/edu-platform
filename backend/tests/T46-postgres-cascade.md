# T46 — отчёт: каскад удаления курса и идемпотентность миграций на Postgres

**Ветка:** `task/T46-pg-cascade` · **Worktree:** `wt/t46`

## Что изменено

| Файл | Что |
|---|---|
| `backend/tests/postgres/cascade_test.go` (новый) | 2 интеграционных теста: `TestPostgresDeleteCourseCascade`, `TestPostgresMigrationsIdempotent` + локальный помощник `countRows` (прямые COUNT по своим id) |
| `backend/tests/T46-postgres-cascade.md` (этот файл) | отчёт |

Код хранилища, handler'ы и `helper_test.go` не менялись. Новых зависимостей нет.

## Что покрыто

### `TestPostgresDeleteCourseCascade`

Сценарий: преподаватель + курс-черновик с модулем и 2 уроками (`seedCourseWithLessons`),
ученик, запись на курс (`enroll`), платёж (`createPayment` → `SetPaymentStatus(succeeded)`),
отзыв (`CreateReview`), прогресс по уроку (`CompleteLesson`). Затем
`s.DeleteCourse(course.ID)` и проверки — **двумя способами**: публичными методами
хранилища (как их видит API) и прямыми `COUNT(*)` по своим id (факт удаления строк в БД).

| Каскад (001_init.sql) | Проверка методами | Проверка COUNT в БД |
|---|---|---|
| `courses` | `GetCourseByID` → `ok=false`; курс не остался в `ListCoursesByTeacher` | — |
| `modules` | `ListModulesByCourse` → пусто; `GetModule(id)` → not found | `modules WHERE course_id` = 0 |
| `lessons` | `ListLessonsByCourse` → пусто; `GetLesson(id)` по каждому → not found | `lessons WHERE id` = 0 по каждому уроку |
| `enrollments` | `GetEnrollment` → `ok=false`; в `ListEnrollmentsByStudent` записей на курс нет | `enrollments WHERE course_id` = 0 |
| `payments` | `ListPaymentsByCourse` → пусто; в `ListPaymentsByStudent` платежей курса нет | `payments WHERE course_id` = 0 |
| `reviews` (доп. к заданию) | до удаления отзыв создан и виден (`ListReviewsByCourse`) | `reviews WHERE course_id` = 0 |
| `lessons → lesson_progress` | `CompleteLesson` до удаления | `lesson_progress WHERE student_id` = 0 |

Дополнительно: ученик и преподаватель после удаления курса целы (`GetUserByID` ok) —
каскад по `users` не задет; повторный `DeleteCourse` → `repo.ErrNotFound`.

**Фактическое поведение совпало с ожиданиями и с in-memory `Store.DeleteCourse`
(`store.go`):** удаляются модули, уроки, прогресс, записи, отзывы и платежи
(включая индекс `provider_payment_id` — на PG его освобождает UNIQUE-строка);
пользователи не задеваются; повторное удаление — `ErrNotFound`. Расхождений
с in-memory реализацией не обнаружено.

### `TestPostgresMigrationsIdempotent`

- До повторных прогонов создаются свои пользователь и курс.
- `migrate.ApplyMigrations` на том же пуле (тот же раннер, что в `cmd/api`)
  вызывается ещё **2 раза** (3-й и 4-й прогон набора 001–006 после прогона
  помощником) — оба без ошибок.
- После повторных прогонов: свои курс и пользователь читаются (`GetCourseByID`,
  `GetUserByID`); 11 ключевых таблиц читаются (`COUNT(*)` без ошибки);
  CHECK-констрейнт `users_status_check` существует ровно один раз (006 не
  пытается добавить его повторно); строка `commission_pct` из 001 на месте.

## Прогоны (Docker, временный Postgres, стенд не тронут)

```
docker network create t46-net
docker run -d --name t46-pg --network t46-net -e POSTGRES_USER=edu -e POSTGRES_PASSWORD=edu -e POSTGRES_DB=edu postgres:16-alpine
# pg_isready: ready
docker run --rm --network t46-net -v "<wt>\t46\backend:/src" -v p5-gomod:/go/pkg/mod -w /src \
  -e GOFLAGS=-mod=mod -e TEST_DATABASE_URL="postgres://edu:edu@t46-pg:5432/edu?sslmode=disable" \
  golang:1.24-alpine sh -c "go build ./... && go vet ./... && go test ./tests/postgres -count=1 -v"
```

Вывод (сокращённо — по строке на тест); `go build` и `go vet` прошли (exit 0):

```
=== RUN   TestPostgresAdminOverviewUsersBlocked
--- PASS: TestPostgresAdminOverviewUsersBlocked (2.94s)
--- PASS: TestPostgresAdminOverviewRoleAndPaymentAggregates (0.21s)
--- PASS: TestPostgresListUsersFilters (0.19s)
--- PASS: TestPostgresUserDependenciesAndDelete (0.43s)
--- PASS: TestPostgresDeleteCourseCascade (0.53s)      <- новый T46
--- PASS: TestPostgresMigrationsIdempotent (0.88s)     <- новый T46
--- PASS: TestPostgresCourseStudentsProgress (0.43s)
--- PASS: TestPostgresCourseStatsData (0.56s)
--- PASS: TestPostgresStudentDashboard (0.41s)
--- PASS: TestPostgresPaymentsJournalFiltersTotalsAndPaging (0.60s)
--- PASS: TestPostgresUpdateCoursePartialFields (0.22s)
PASS
ok  	edu-platform/backend/tests/postgres	7.650s
```

Прогон без `TEST_DATABASE_URL` — весь набор (`exit 0`, PG-тесты SKIP):

```
ok  	edu-platform/backend/cmd/api	4.353s
ok  	edu-platform/backend/internal/auth	0.068s
ok  	edu-platform/backend/internal/handler	1.976s
ok  	edu-platform/backend/internal/payments	0.090s
ok  	edu-platform/backend/internal/repo	0.022s
ok  	edu-platform/backend/tests/contract	27.270s
ok  	edu-platform/backend/tests/postgres	0.038s
T46_ALL_EXIT=0
```

Пакет postgres без DSN подробно: все 11 тестов (включая оба T46) уходят в
`--- SKIP` с текстом «TEST_DATABASE_URL не задан…», `ok` пакета, exit 0
(`T46_SKIP_EXIT=0`).

После прогона: `docker rm -f t46-pg`, `docker network rm t46-net` — своих
контейнеров/сетей не осталось; постгрес общего стенда (`edu-platform-db`) не
использовался.

## Не проверено / риски

- `-race` на Postgres не запускался (в тестах нет конкурентных сценариев).
- Параллельный запуск `ApplyMigrations` из нескольких процессов не проверялся:
  у раннера нет advisory-lock, но и код миграций в задаче менять нельзя —
  риск вне рамок T46.
- Ограничение «удалять только draft» живёт в handler'е и здесь не проверяется
  (тест — уровень хранилища: `DeleteCourse` удаляет курс в любом статусе —
  так же, как in-memory).
- Каскады `sessions`, `teacher_invites`, удаление `users` и прогресс по урокам
  с `completed_at IS NULL` (BumpAttempt) в T46 не проверялись — это вне
  списка задачи.
- Изоляция на «чужом» непустом Postgres держится на уникальности своих данных
  (префикс `pgtest-<unix>`), а не на транзакциях; на временном контейнере
  таблицы были непустыми (сид 001/002), но боевую нагрузку не эмулировали.
- Платёж в каскадном тесте один и в статусе `succeeded`; каскад для
  `pending/failed/refunded` отдельно не гонялся (FK-каскад от статуса не
  зависит).
