# T73 — Бэкенд: избранные курсы

**Волна:** P9  **Ветка:** `task/T73-favorites`  **Worktree:** `wt/t73`

## Суть

Ученик не может сохранить курс на будущее. Волна P9 добавляет избранное:
идемпотентные добавление/удаление и список сохранённых курсов. Раздел
«Волна P9 (T71–T82)» в `Task/00-api-contract.md` — источник правды,
**заморожен**.

Работать только в своём worktree `wt/t73` (ветка `task/T73-favorites`).

## Что сделать

1. Миграция `backend/internal/migrate/migrations/013_favorites.sql`: таблица
   `favorites (user_id, course_id, created_at)`, PK `(user_id, course_id)`,
   FK каскадом, индекс по `(user_id, created_at DESC)`; идемпотентно.
2. StoreAPI (блок методов в конце `store_iface.go`): `AddFavorite`,
   `RemoveFavorite`, `ListFavoriteCoursesByUser` (возвращает курсы в порядке
   добавления, новые первыми). Реализация в `store.go` и `postgres_store.go`;
   parity. Только published-курсы попадают в список.
3. Хендлеры в новом `handler/favorites.go`, регистрация в `middleware.go`
   (внутри общей группы `/api/v1` через `r.With(app.requireAuth)`):
   - `PUT /me/favorites/{courseId}` → `204` (идемпотентно; только published,
     иначе `404 «Курс не найден»`);
   - `DELETE /me/favorites/{courseId}` → `204` (идемпотентно);
   - `GET /me/favorites` → `200 {"items":[CourseItem],"total":N}` (элементы —
     через существующий `toCourseItem`).
4. Тесты `backend/tests/contract/favorites_test.go`: 401 без входа, добавление/
   повторное добавление, удаление/повторное удаление, список и порядок,
   архивный/неопубликованный курс → 404, чужое избранное не видно.
   Parity-тесты в `backend/tests/postgres/favorites_test.go` (включая каскад).
5. Короткая приписка в `docs/API.md`.

Существующие тесты должны остаться зелёными. Правки существующих тестов
допустимы только при изменении контракта — обоснуйте в отчёте.

## Разрешённые файлы

- `backend/internal/migrate/migrations/013_favorites.sql` (новый)
- `backend/internal/repo/**`
- `backend/internal/handler/**`
- `backend/tests/**`
- `docs/API.md`
- `backend/tests/T73-favorites.md` (отчёт)

## Запрещено

- `Task/**`, `specs/**`, `frontend/**`, `ru.json`, миграции 001–012 и 014–015,
  `docker-compose*`, `qa-*.ps1`, `.github/**`, `deploy/**`.
- `git push`, `git merge`, смена ветки, работа вне `wt/t73`.

## Проверка (только через Docker)

Из корня worktree:

```
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Postgres-parity: контейнер `postgres:16-alpine` с именем `p9-t73-pg` на порту
**55443** (edu/edu, БД edu_test); хост из контейнера тестов —
`host.docker.internal`; после прогона остановите и удалите контейнер.

## Отчёт

`backend/tests/T73-favorites.md`: «Что сделано», «Проверка», «Отклонения»,
«Открытые вопросы». Закоммитьте отчёт и код в ветку `task/T73-favorites`.
