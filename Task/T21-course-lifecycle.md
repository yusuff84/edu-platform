# T21 — Бэкенд: жизненный цикл курса в конструкторе

Волна: P3. Зависимости: нет. Потребитель: T28 (UI конструктора).

## Зачем

Конструктор умеет только «создать → отправить на модерацию». Нет способа скопировать
курс (учителя делают это руками), нет архивации (`archived` в модели есть, но недостижим),
нет отзыва с модерации (передумал — правь в БД), нет переноса урока между модулями
(`UpdateLesson` намеренно не меняет `moduleId`). Это четыре дыры в базовом сценарии
«составить курс».

## Владеет файлами (менять можно только их)

- `backend/internal/handler/teacher_lifecycle.go` (новый)
- `backend/internal/repo/course_clone.go` (новый — методы на `*Store`)
- `backend/internal/repo/postgres_course_clone.go` (новый — методы на `*PostgresStore`)
- `backend/internal/repo/store_iface.go` (+2 метода в `StoreAPI`)
- `backend/internal/handler/middleware.go` (+1 строка монтирования внутри `r.Route("/teacher", ...)`)
- `backend/tests/contract/teacher_lifecycle_test.go` (новый)

## API (внутри `/api/v1/teacher`, requireAuth + teacher|admin уже навешены)

Все пути относительные, владение курсом проверяет существующий `ownCourse`.

| Метод | Путь | Успех | Ошибки |
|---|---|---|---|
| POST | `/courses/{id}/duplicate` | 201 `{"course":{...}}` | 404 нет курса, 403 чужой |
| POST | `/courses/{id}/archive` | 200 `{"course":{...}}` | 409 уже archived, 404, 403 |
| POST | `/courses/{id}/unarchive` | 200 `{"course":{...}}` | 409 не archived, 404, 403 |
| POST | `/courses/{id}/withdraw` | 200 `{"course":{...}}` | 409 не review, 404, 403 |
| POST | `/courses/{id}/lessons/{lessonId}/move` | 200 `{"lesson":{...}}` | 400, 409, 404, 403 |

Правило кодов проекта: **400** — валидация тела/параметров, **409** — конфликт с
машиной состояний, **403** — нет прав, **404** — нет сущности.

### duplicate
- Копия: `status=draft`, `title` = `<title> (копия)`, `slug` = свободный `<slug>-copy-2`,
  `-copy-3`, … (нумерация начинается с 2, проверка через `SlugTaken`).
- Копируются **все** модули (позиции сохранены) и уроки (позиции сохранены,
  `bodyMd`/`videoUrl`/`liveUrl`/`startsAt`/`quiz`/`codeTask` — целиком, глубоко).
- Сбрасывается: `status`, `moderationComment`, `avgRating`=0, `ratingsCount`=0,
  `createdAt`/`updatedAt` = сейчас. `coverPath` копируется.
- Записи, платежи, отзывы и прогресс НЕ копируются.
- Копия принадлежит вызывающему (`teacherID` = текущий пользователь), даже если
  копируют админ чужой курс.

### archive / unarchive / withdraw
- `archive`: разрешён из `published` и `draft` и `review`; 409 если уже `archived`.
- `unarchive`: `archived` → `draft`; 409 иначе.
- `withdraw`: `review` → `draft`, `moderationComment` очищается; 409 иначе.
- Использовать существующий `SetCourseStatus`. После смены — вернуть свежий курс.

### move
- Тело: `{"moduleId":"..."}`. `moduleId` пустой → 400 «Не указан модуль».
- Модуль должен принадлежать **тому же** курсу → иначе 400.
- Урок должен принадлежать этому курсу → иначе 404.
- Разрешено только при `status == draft` → иначе 409 «Менять структуру можно только в черновике».
- Новая позиция — конец целевого модуля (max+1).
- Если урок уже в этом модуле — 200, ничего не меняется (идемпотентность).

## Хранилище

Добавить в `StoreAPI` ровно два метода:

```go
// CloneCourse — глубокая копия курса с модулями и уроками (T21).
// Копия создаётся черновиком, slug подбирается по свободному (<slug>-copy-N).
// ErrNotFound — исходного курса нет.
CloneCourse(courseID string) (*Course, error)

// MoveLessonToModule переносит урок в конец другого модуля (T21).
// ErrNotFound — урока или модуля нет.
MoveLessonToModule(lessonID, moduleID string) error
```

- In-memory реализация — `course_clone.go`.
- Postgres-реализация — `postgres_course_clone.go` **одной транзакцией** (копирование
  модулей и уроков с новыми id; id генерировать так же, как в остальном проекте).
- Дубль `slug`/`title` формируется в repo или handler — на твоё усмотрение, но
  одно место, не в двух.

## Тесты (обязательно)

`backend/tests/contract/teacher_lifecycle_test.go` — сквозь `NewRouter` (как в
`teacher_crud_test.go`), не менее:

1. duplicate: копия в draft, модули/уроки совпадают по количеству и содержимому,
   slug отличается, исходный курс не изменился.
2. duplicate чужого курса другим учителем → 403.
3. archive: published → archived; повторный archive → 409.
4. unarchive: archived → draft; из draft → 409.
5. withdraw: review → draft; из published → 409.
6. move: урок переехал в другой модуль, позиция в конце; чужой модуль → 400;
   перенос в published → 409.

## Критерии приёмки

- `go build ./... && go vet ./... && go test ./... -count=1` — зелёные (в Docker).
- `go test -race` — зелёный в образе `golang:1.24-bookworm`.
- Ни один существующий тест не изменён.
- Контракт из таблицы соблюдён буквально (коды и тела).

## Проверка (только в Docker, Go на хосте нет)

```
docker run --rm -v "<путь>\backend:/src" -v gomodcache:/go/pkg/mod -w /src ^
  -e GOFLAGS=-mod=mod golang:1.24-alpine sh -c "go build ./... && go vet ./... && go test ./... -count=1"
```
