# T54 — Бэкенд: обложка курса (upload, backend)

**Волна:** P7  **Ветка:** `task/T54-course-cover-upload`  **Worktree:** `wt/t54`

## Суть

У курса нет визуальной обложки: каталог и публичная страница показывают только
текст-плитки. Контракт волны P7 вводит обложку:
`POST/DELETE /teacher/courses/{id}/cover` + поле `coverPath` в видах (аддитивно).
Раздел «T54 — обложка курса (cover)» в `Task/00-api-contract.md` — источник
правды, **заморожен**.

Реализация хранилища уже есть в проекте: `storage.SaveLocal` (см. в бэке,
папка uploads) и маршрут `/uploads/*` — файлы отдаются стендом.

## Что сделать

1. `backend/internal/migrate/migrations/007_cover.sql`:
   `ALTER TABLE courses ADD COLUMN cover_path TEXT;` (+ идемпотентность по
   стилю соседних миграций).
2. `backend/internal/repo/models.go` — поле `CoverPath *string` (`json:"coverPath"`),
   маппинг в SQL-слоях: `postgres_store.go` / `postgres_update.go` /
   `store.go` (in-memory) / везде, где строится `Course` (вставка/обновление);
   обновление `courses` не должно сбрасывать `cover_path` в NULL при правке
   карточки и статусов (добавь только в UPDATE при явной смене).
3. `backend/internal/handler/teacher_cover.go` (НОВЫЙ):
   - `RegisterTeacherCoverRoutes(r)` — монтировать в `middleware.go` рядом с
     другими teacher-группами;
   - `POST /courses/{id}/cover`: multipart `file`; тип по байтам тела
     (`http.DetectContentType`) — только `image/jpeg|png|webp`; ≤5 МБ; иначе
     `400 «Обложка: допустимы JPEG, PNG или WebP не больше 5 МБ»`; права —
     владелец/admin (копия из `teacher_edit.go` `loadOwnCourse`); `409` —
     archived («Курс в архиве: обложка заморожена»); успех `201
     {"coverPath":"/uploads/covers/<имя>"}`; файл — через `storage.SaveLocal`
     в `uploads/covers/`, имя вида `<courseId>.<ext>` (перезапись при повторе);
   - `DELETE /courses/{id}/cover`: `204` (обложки нет — так же `204`), сброс
     `cover_path` в NULL; `409` archived; `404`.
   - ВАЖНО: глобальный лимит тела 1 МБ не должен убивать multipart — для этого
     маршрута нужно разрешить ≥6 МБ (порядок middleware: лимит применяется на
     ветке `/api/v1`; совместимо с остальными маршрутами; если лимит ставится
     глобально в `middleware.go` — исключение только для cover).
4. `coverPath` аддитивно в ответах: `GET /courses` (каталог, `courses.go`),
   `GET /courses/{slug}` (course view), `GET /teacher/courses`,
   `GET /teacher/courses/{id}`, `GET /admin/courses` (T48 список). JSON-имя —
   `coverPath`, значение `string|null` (пусто → `null`).
5. Публичная форма курса: убедиться, что `Course` сериализует обложку во все
   listed ответах без переименований.

## Разрешённые файлы

- НОВЫЕ: `backend/internal/handler/teacher_cover.go`,
  `backend/internal/migrate/migrations/007_cover.sql`,
  `backend/tests/contract/cover_p7_test.go`;
- ПРАВКИ: `backend/internal/handler/middleware.go` (маршруты + лимит тела),
  `backend/internal/handler/courses.go` (coverPath во view),
  `backend/internal/repo/models.go`, `store.go`, `postgres_store.go`,
  `postgres_update.go` (+ сопутствующие тестовые фикстуры golbp);
- `docs/API.md` — короткая приписка.

## Запрещено

- Трогать `frontend/**`, `Task/**`, `specs/**`, `ru.json`;
- Менять существующие поля/структуры ответов (только аддитивно);
- Останавливать чужие контейнеры `olymp-*`;
- Использовать базу стенда для экспериментов дальше миграции LOCAL-тестов
  (тестовая БД по README: контейнер `edu-pgtest`, порт 55432, env
  `TEST_DATABASE_URL`).

## Проверка (только через Docker)

```
docker run --rm -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
docker run -d --rm --name edu-pgtest54 -e POSTGRES_USER=edu -e POSTGRES_PASSWORD=edu -e POSTGRES_DB=edu_test -p 55432:5432 postgres:16-alpine
docker run --rm --network host -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src -e GOFLAGS=-mod=mod -e TEST_DATABASE_URL="postgres://edu:edu@127.0.0.1:55432/edu_test?sslmode=disable" golang:1.24-alpine sh -c "go test ./tests/postgres/... -count=1"
docker rm -f edu-pgtest54
```

Контрактные тесты: happy-path загрузки (тип + размер), перезапись, удаление,
400 (мусорный тип), 400 (5,5 МБ), 401, 403 (не владелец), 409 (archived),
`coverPath` в видах каталога/курса/списков. PG-набор убедиться, что миграция
007 накатывается и старые тесты зелёные.

## Отчёт

`backend/tests/T54-course-cover-upload.md`:те же правила, что в T53.
Коммит `T54: ...`, push `task/T54-course-cover-upload`.
