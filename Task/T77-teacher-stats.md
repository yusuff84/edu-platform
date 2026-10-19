# T77 — Бэкенд: аналитика учителя

**Волна:** P9  **Ветка:** `task/T77-teacher-stats`  **Worktree:** `wt/t77`

## Суть

У преподавателя нет сводки по бизнесу: выручка, ученики, завершаемость,
рейтинг и разбивка по курсам. Волна P9 добавляет два эндпоинта аналитики.
Раздел «Волна P9 (T71–T82)» в `Task/00-api-contract.md` — источник правды,
**заморожен**.

Работать только в своём worktree `wt/t77` (ветка `task/T77-teacher-stats`).

## Что сделать

1. Миграция `backend/internal/migrate/migrations/015_analytics_indexes.sql` —
   индексы под агрегаты: `enrollments(course_id, status)`,
   `payments(course_id, status, created_at)`, `reviews(course_id, rating)`;
   `CREATE INDEX IF NOT EXISTS`, идемпотентно.
2. StoreAPI (блок методов в конце `store_iface.go`): сводка и разбивка по
   курсам одним вызовом каждая:
   - `TeacherOverview(teacherID string, all bool) (*TeacherOverview, error)`;
   - `TeacherCourseStats(teacherID string, all bool, limit int) []*TeacherCourseStat`.
   `all=true` — все курсы платформы (для admin), иначе только курсы учителя.
   Реализуйте в `store.go` (in-memory, без SQL) и `postgres_store.go`
   (агрегирующие запросы, без N+1); parity.
3. Модели в `repo/models.go` с JSON-тегами по контракту. Средний прогресс
   считайте как `completed_lessons / total_lessons * 100` по активным записям
   (у курса без уроков — 0). `completed` — активные записи с прогрессом 100 %.
4. Хендлеры в новом `handler/teacher_stats.go`:
   `GET /teacher/stats`, `GET /teacher/stats/courses?limit=50` — teacher/admin,
   регистрация в teacher-группе `middleware.go`. Контроль `limit` (дефолт 50,
   максимум 200). Права: teacher — только свои курсы, admin — все.
5. Тесты `backend/tests/contract/teacher_stats_test.go`: пустая сводка,
   несколько курсов/учеников, выручка только по succeeded, завершённые записи
   и средний прогресс, рейтинг, last30d, разбивка и порядок, limit, доступ
   ученику → 403, чужой учитель не видит чужие курсы. Parity в
   `backend/tests/postgres/teacher_stats_test.go`.
6. Короткая приписка в `docs/API.md`.

Существующие тесты должны остаться зелёными. Правки существующих тестов
допустимы только при изменении контракта — обоснуйте.

## Разрешённые файлы

- `backend/internal/migrate/migrations/015_analytics_indexes.sql` (новый)
- `backend/internal/repo/**`
- `backend/internal/handler/**`
- `backend/tests/**`
- `docs/API.md`
- `backend/tests/T77-teacher-stats.md` (отчёт)

## Запрещено

- `Task/**`, `specs/**`, `frontend/**`, `ru.json`, миграции 001–014,
  `docker-compose*`, `qa-*.ps1`, `.github/**`, `deploy/**`.
- `git push`, `git merge`, смена ветки, работа вне `wt/t77`.

## Проверка (только через Docker)

```
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Postgres-parity: контейнер `p9-t77-pg` на порту **55447** (edu/edu, edu_test),
хост из тестового контейнера — `host.docker.internal`; после прогона удалите
контейнер.

## Отчёт

`backend/tests/T77-teacher-stats.md`: «Что сделано», «Проверка», «Отклонения»,
«Открытые вопросы». Закоммитьте отчёт и код в ветку `task/T77-teacher-stats`.
