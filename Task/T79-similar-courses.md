# T79 — Бэкенд: похожие курсы

**Волна:** P9  **Ветка:** `task/T79-similar-courses`  **Worktree:** `wt/t79`

## Суть

На странице курса нечего предложить кроме самого курса. Волна P9 добавляет
подборку похожих курсов той же категории. Раздел «Волна P9 (T71–T82)» в
`Task/00-api-contract.md` — источник правды, **заморожен**.

Работать только в своём worktree `wt/t79` (ветка `task/T79-similar-courses`).

## Что сделать

1. StoreAPI (блок методов в конце `store_iface.go`):
   `ListSimilarCourses(courseID string, limit int) []*Course` — published-курсы
   той же `category`, кроме самого курса; сортировка: число активных записей
   по убыванию, затем `avgRating` по убыванию, затем `createdAt` по убыванию.
   Реализация в `store.go` и `postgres_store.go` (один агрегирующий запрос, без
   N+1); parity.
2. Хендлер в новом `handler/similar_courses.go`:
   `GET /api/v1/courses/{id}/similar?limit=4` — публичный. `id` — UUID курса;
   курс должен существовать и быть published, иначе `404 «Курс не найден»`.
   `limit` по умолчанию 4, максимум 12, нечисловой/<=0 — дефолт. Ответ:
   `200 {"items":[CourseItem],"total":N}` (элементы через `toCourseItem`).
   Регистрация — в публичной секции `middleware.go`.
3. Тесты `backend/tests/contract/similar_courses_test.go`: подборка только той
   же категории и без самого курса, порядок (записи → рейтинг → дата), пустая
   категория, limit/дефолт, 404 для неопубликованного/несуществующего курса.
   Parity в `backend/tests/postgres/similar_courses_test.go`.
4. Короткая приписка в `docs/API.md`.

Существующие тесты должны остаться зелёными. Правки существующих тестов
допустимы только при изменении контракта — обоснуйте.

## Разрешённые файлы

- `backend/internal/repo/**`
- `backend/internal/handler/**`
- `backend/tests/**`
- `docs/API.md`
- `backend/tests/T79-similar-courses.md` (отчёт)

## Запрещено

- `Task/**`, `specs/**`, `frontend/**`, `ru.json`, миграции 001–015,
  `docker-compose*`, `qa-*.ps1`, `.github/**`, `deploy/**`.
- `git push`, `git merge`, смена ветки, работа вне `wt/t79`.

## Проверка (только через Docker)

```
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Postgres-parity: контейнер `p9-t79-pg` на порту **55449** (edu/edu, edu_test),
хост из тестового контейнера — `host.docker.internal`; после прогона удалите
контейнер.

## Отчёт

`backend/tests/T79-similar-courses.md`: «Что сделано», «Проверка»,
«Отклонения», «Открытые вопросы». Закоммитьте отчёт и код в ветку
`task/T79-similar-courses`.
