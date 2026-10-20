# T77 — Бэкенд: аналитика учителя (отчёт)

Ветка: `task/T77-teacher-stats` (worktree `wt/t77`). Контракт:
`Task/00-api-contract.md`, раздел «Волна P9 (T71–T82)» → «T77 — Аналитика
учителя».

## Что сделано

1. **Миграция `015_analytics_indexes.sql`** (новая): идемпотентные
   `CREATE INDEX IF NOT EXISTS` под агрегаты —
   `enrollments(course_id, status)`,
   `payments(course_id, status, created_at)`,
   `reviews(course_id, rating)`.
2. **Модели** (`repo/models.go`): `TeacherOverview` с вложенными
   `courses/students/revenue/completion/ratings/last30d` и
   `TeacherCourseStat`; JSON-теги — ровно по контракту P9.
3. **StoreAPI** (`repo/store_iface.go`, блок методов в конце интерфейса):
   - `TeacherOverview(teacherID string, all bool) (*TeacherOverview, error)`;
   - `TeacherCourseStats(teacherID string, all bool, limit int) []*TeacherCourseStat`.
4. **In-memory** (`repo/store.go`, блок в конце файла): оба метода собирают
   агрегаты одним проходом по картам под `RLock` — без SQL и без N+1.
   Активными считаются только записи `active`; выручка — только
   `succeeded`-платежи; прогресс записи —
   `completed_lessons / total_lessons * 100` (у курса без уроков 0,
   ограничение сверху 100); `completed` — активные записи с прогрессом
   100 %; рейтинг — средневзвешенный по `published`-курсам
   (`avg_rating × ratings_count`); `last30d` — записи и succeeded-платежи
   за 30 дней.
5. **Postgres** (`repo/postgres_store.go`, блок в конце файла): каждый метод —
   **один** агрегирующий SQL-запрос. Сводка собрана скалярными подзапросами
   по CTE `scoped`/`active` (LATERAL-подсчёт уроков и пройденного по каждой
   активной записи); разбивка — `LEFT JOIN LATERAL` по ученикам/прогрессу и
   выручке на курс. Сортировка разбивки — `revenue_cents DESC`,
   `students DESC`, `lower(title) ASC`, `id` как детерминированный
   tie-break; `LIMIT` применяется в SQL. Ошибка сводки возвращается наверх
   (handler → 500), ошибка разбивки логируется и даёт пустой список — как у
   остальных методов StoreAPI без `error` в сигнатуре.
6. **Хендлеры** (`handler/teacher_stats.go`, новый):
   - `GET /api/v1/teacher/stats` → `200` `TeacherOverview`;
   - `GET /api/v1/teacher/stats/courses?limit=50` →
     `200 {"items":[TeacherCourseStat],"total":N}`.
   Регистрация — в teacher-группе `middleware.go`
   (`RegisterTeacherStatsRoutes`), поэтому доступ только teacher/admin
   (ученику — 403, без сессии — 401). Teacher вызывает store с `all=false`
   (свои курсы), admin — с `all=true` (все курсы платформы). `limit`:
   дефолт 50, максимум 200; нечисловое/`<=0` — дефолт.
7. **Тесты**:
   - `backend/tests/contract/teacher_stats_test.go` — пустая сводка и
     структура ответа, несколько курсов/учеников, выручка только по
     `succeeded` (pending/refunded не считаются), завершённые записи и
     средний прогресс, рейтинг только опубликованных курсов, `last30d`,
     разбивка и её порядок, `limit` (в т.ч. мусор/0/500), ученик → 403,
     изоляция курсов чужого учителя, admin видит все курсы;
   - `backend/tests/postgres/teacher_stats_test.go` — parity in-memory и
     Postgres на одинаковом наборе данных (полное совпадение сводки и
     разбивки), порядок и limit, `all=true` для admin, наличие индексов
     миграции 015, нулевая сводка для несуществующего учителя.
8. **Документация**: короткая приписка о двух эндпоинтах в `docs/API.md`
   (раздел «Преподаватель»).

## Проверка

1. Основная проверка (Docker, без БД — Postgres-тесты корректно skip):

   ```
   docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
   ```

   Результат: `go vet` без замечаний; все пакеты `ok`
   (`cmd/api`, `internal/auth`, `internal/handler`, `internal/payments`,
   `internal/repo`, `tests/contract`, `tests/postgres`).

2. Postgres parity: контейнер `p9-t77-pg` (`postgres:16-alpine`, порт
   `55447`, `edu/edu`, `edu_test`), тесты из Docker с
   `TEST_DATABASE_URL=postgres://edu:edu@host.docker.internal:55447/edu_test?sslmode=disable`.

   ```
   docker run --rm -e HOME=/root \
     -e TEST_DATABASE_URL="postgres://edu:edu@host.docker.internal:55447/edu_test?sslmode=disable" \
     -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine \
     sh -c "go test ./... -count=1"
   ```

   Результат: все пакеты `ok`, включая `tests/postgres`
   (`TestPostgresTeacherStatsParity`, `TestPostgresTeacherStatsAdminAll`).
   После прогона контейнер `p9-t77-pg` удалён.

## Отклонения

- **`total` в `/teacher/stats/courses`** — число возвращённых элементов
  после применения `limit` (пагинации у эндпоинта нет; сигнатура store
  получает только `limit`). Зафиксировано в `docs/API.md`.
- **`avgPercent`** округляется до 1 знака (`math.Round` / `ROUND(...,1)`),
  чтобы ответы были стабильными и совпадали между in-memory и Postgres;
  контракт требует лишь «дробное 0..100».
- **`limit`**: нечисловое, нулевое и отрицательное значения трактуются как
  отсутствие параметра (дефолт 50), больше 200 — обрезается до 200. Для
  T77 контракт не задаёт код ошибки на мусор; выбран мягкий дефолт, как у
  `limit` в T71.
- **Рейтинг сводки** считается по полям карточки курса
  (`avg_rating`/`ratings_count`), а не пересчётом сырых отзывов: это тот же
  источник, что у публичной карточки курса (`RecalcCourseRating` вызывается
  на каждом изменении отзывов), и он одинаков в обеих реализациях.
- В ветке присутствует только миграция `015`; миграции `012–014` — в
  параллельных ветках P9. Runner применяет файлы по имени, зависимостей у
  `015` от них нет.

## Открытые вопросы

- Нет блокирующих. Семантику `total` после `limit` стоит подтвердить при
  интеграции с фронтендом T78: если ему нужно общее число курсов до
  ограничения, потребуется отдельный счётчик (сейчас контрактная сигнатура
  store его не предусматривает).
