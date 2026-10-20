# T75 — Бэкенд: превью-уроки

**Ветка:** `task/T75-lesson-preview`  **Worktree:** `wt/t75`  **Волна:** P9

## Что сделано

1. **Миграция** `backend/internal/migrate/migrations/014_lesson_preview.sql`:
   `ALTER TABLE lessons ADD COLUMN IF NOT EXISTS is_preview BOOLEAN NOT NULL
   DEFAULT false;` — идемпотентно, старые уроки читаются как не-превью.
2. **Модель** `repo.Lesson`: аддитивное поле `IsPreview bool` с тегом
   `json:"isPreview"` (без `omitempty` — присутствует всегда).
3. **Postgres** (`internal/repo`):
   - `lessonCols` и `scanLesson` читают `l.is_preview`;
   - `CreateLesson` пишет `is_preview` (параметр `$10`);
   - `UpdateLesson` обновляет `is_preview=$9`;
   - `CloneCourseAs` копирует `is_preview` вместе с остальными полями урока.
4. **In-memory** (`internal/repo`):
   - `CreateLesson` сохраняет поле как часть урока;
   - `UpdateLesson` перезаписывает `IsPreview` (PATCH без поля не меняет —
     handler передаёт слитый урок с текущим значением);
   - `cloneLesson` копирует флаг в дубль курса.
5. **Виды урока** (`handler/courses.go`): `lessonView.isPreview` заполняется
   в `lessonToView` и присутствует и в публичном (`full=false`), и в полном
   (`full=true`) виде; публичный вид по-прежнему без `correct` и скрытых
   тестов code-задания.
6. **Teacher API**: `createLessonRequest` и `updateLessonRequest` принимают
   необязательный `isPreview` (`*bool`): создание без поля — `false`, PATCH
   без поля значение не меняет, явный `false` снимает флаг. Валидация по
   `kind` не менялась.
7. **Публичный эндпоинт** `GET /api/v1/courses/{slug}/preview/{lessonId}`
   (маршрут зарегистрирован в публичной секции `middleware.go`, без входа):
   - курс не найден или не `published` → `404 «Курс не найден»`;
   - урок не найден / не принадлежит курсу / `isPreview=false` →
     `404 «Урок не найден»`;
   - успех → `200 {"course":{"id","slug","title"},"lesson":<lessonView>}`,
     урок — публичный вид (`lessonToView(l, false)`).
8. **Тесты**:
   - `backend/tests/contract/lesson_preview_test.go` — флаг в teacher
     create/patch, сохранение при PATCH без поля, флаг в публичной странице
     курса и полном виде владельца, превью `200` без входа для quiz/code,
     отсутствие `"correct"` и скрытых тестов, `404` для не-превью урока,
     чужого урока, неопубликованного курса, неизвестного slug и мусорного id.
   - `backend/tests/postgres/lesson_preview_test.go` — parity SQL-ветки:
     `DEFAULT false` у сидовых уроков, запись/чтение при `CreateLesson`,
     прямой `SELECT is_preview` (миграция 014), флаг в `GetLesson`/
     `ListLessonsByModule`/`ListLessonsByCourse`, переключение в
     `UpdateLesson`, сохранение флага при `CloneCourse`.
9. **Документация**: короткая приписка о поле и эндпоинте в `docs/API.md`
   (блок «Волна P9 (T75)»).

## Проверка

Все команды — только через Docker, из корня worktree `wt/t75`.

```
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod \
  -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Результат: `go vet` без замечаний, все пакеты `ok` (включая
`tests/contract` и `tests/postgres`; Postgres-тесты без env штатно
`SKIP`).

Целевой прогон новых контрактных тестов:

```
go test ./tests/contract -run 'TestLessonPreview' -count=1 -v
```

Результат: `TestLessonPreviewFlagInTeacherAndPublicViews` — PASS,
`TestLessonPreviewPublicEndpoint` — PASS.

Postgres parity (контейнер `p9-t75-pg`, порт 55445, `postgres:16-alpine`,
`edu/edu`, `edu_test`):

```
docker run --rm --add-host=host.docker.internal:host-gateway \
  -e TEST_DATABASE_URL="postgres://edu:edu@host.docker.internal:55445/edu_test?sslmode=disable" \
  -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine \
  sh -c "go test ./tests/postgres/... -count=1 -v"
```

Результат: все тесты пакета PASS, в том числе
`TestPostgresLessonPreviewRoundtrip` и `TestPostgresMigrationsIdempotent`.
Контейнер после прогона удалён (`docker rm -f p9-t75-pg`).

## Отклонения

- Карточка урока в `GET /admin/courses/{id}` (T55) не менялась: это
  отдельная замороженная форма «только метаданные» (`id/title/kind/position`),
  а не `lessonView`; её SQL-выборка в `postgres_course_detail.go` поле
  `is_preview` не читает. На контракт T75 это не влияет.
- Формы существующих тестов не правились: аддитивное `isPreview` не сломало
  ни одного фиксатора, все старые тесты зелёные.
- Предсуществующее выравнивание полей `Store` в `internal/repo/store.go`
  оставлено как было (не переформатировал чужие строки).

## Открытые вопросы

Нет. Фронтенд (бейдж/окно превью) — вне зоны T75; контракт API для него
готов: `lessonView.isPreview` и публичный эндпоинт.
