# T75 — Бэкенд: превью-уроки

**Волна:** P9  **Ветка:** `task/T75-lesson-preview`  **Worktree:** `wt/t75`

## Суть

У курса нет бесплатного превью: гость не может оценить содержание до покупки.
Волна P9 добавляет флаг `isPreview` у урока и публичный эндпоинт просмотра.
Раздел «Волна P9 (T71–T82)» в `Task/00-api-contract.md` — источник правды,
**заморожен**.

Работать только в своём worktree `wt/t75` (ветка `task/T75-lesson-preview`).

## Что сделать

1. Миграция `backend/internal/migrate/migrations/014_lesson_preview.sql`:
   `ALTER TABLE lessons ADD COLUMN IF NOT EXISTS is_preview BOOLEAN NOT NULL
   DEFAULT false;` идемпотентно.
2. `repo/models.go`: `Lesson.IsPreview bool` (`json:"isPreview"`); добавьте
   поле во все SQL-выборки/вставки/обновления `lessons` в `postgres_store.go`
   и в in-memory `store.go` (создание, клонирование курса, обновление).
3. `handler/courses.go`: `lessonView` получает `isPreview` (всегда, и в
   публичном, и в полном виде); `lessonToView` заполняет его.
4. Приём поля в teacher-хендлерах: `POST /teacher/courses/{id}/lessons` и
   `PATCH /teacher/courses/{id}/lessons/{lessonID}` — необязательное
   `isPreview` (указатель в input; создание без поля — `false`, PATCH без поля
   не меняет). Не ломайте валидацию по kind.
5. Публичный `GET /api/v1/courses/{slug}/preview/{lessonId}` (регистрация в
   публичной секции `middleware.go`): курс published, урок принадлежит курсу и
   `isPreview=true`; ответ
   `200 {"course":{"id","slug","title"},"lesson":<lessonView>}`; иначе
   `404 «Курс не найден»` / `404 «Урок не найден»`. Используйте публичный вид
   урока (без правильных ответов теста и скрытых тестов).
6. Тесты `backend/tests/contract/lesson_preview_test.go`: флаг в публичной
   странице курса, teacher create/patch с `isPreview`, превью 200 без входа,
   404 для не-preview/чужого урока/неопубликованного курса, отсутствие
   `correct` и hidden-тестов в ответе. Parity-тесты в
   `backend/tests/postgres/lesson_preview_test.go`.
7. Короткая приписка в `docs/API.md`.

Существующие тесты должны остаться зелёными (правки фиксаторов формы уроков
допустимы — поле аддитивное, обоснуйте).

## Разрешённые файлы

- `backend/internal/migrate/migrations/014_lesson_preview.sql` (новый)
- `backend/internal/repo/**`
- `backend/internal/handler/**`
- `backend/tests/**`
- `docs/API.md`
- `backend/tests/T75-lesson-preview.md` (отчёт)

## Запрещено

- `Task/**`, `specs/**`, `frontend/**`, `ru.json`, миграции 001–013 и 015,
  `docker-compose*`, `qa-*.ps1`, `.github/**`, `deploy/**`.
- `git push`, `git merge`, смена ветки, работа вне `wt/t75`.

## Проверка (только через Docker)

```
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Postgres-parity: контейнер `p9-t75-pg` на порту **55445** (edu/edu, edu_test),
хост из тестового контейнера — `host.docker.internal`; после прогона удалите
контейнер.

## Отчёт

`backend/tests/T75-lesson-preview.md`: «Что сделано», «Проверка», «Отклонения»,
«Открытые вопросы». Закоммитьте отчёт и код в ветку `task/T75-lesson-preview`.
