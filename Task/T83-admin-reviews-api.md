# T83 — Бэкенд: глобальная модерация отзывов

**Волна:** P10  **Ветка:** `task/T83-admin-reviews`  **Worktree:** `wt/t83`

## Суть

Сейчас админ видит отзывы только внутри карточки конкретного курса. Нужен
глобальный список отзывов с фильтрами и поиском, чтобы супер-админ мог быстро
найти и удалить проблемный отзыв. Контракт — раздел «Волна P10» в
`Task/00-api-contract.md`, **заморожен**.

Работать только в своём worktree `wt/t83` (ветка `task/T83-admin-reviews`),
базовая ветка — `integration/p10`.

## Что сделать

1. Новый файл `backend/internal/repo/admin_reviews.go`: тип
   `AdminReviewFilter` (`CourseID`, `Rating *int`, `Q`, `From`, `To`,
   `Sort`, `Dir`, `Page`, `Limit`) и `AdminReviewRow` (поля строго по
   контракту T83, включая `courseTitle`, `courseSlug`, `studentEmail`,
   `studentName`). Не добавляйте новые типы в `models.go`, чтобы не
   конфликтовать с другими задачами P10.
2. В `StoreAPI` (`backend/internal/repo/store_iface.go`) добавьте метод
   `ListAdminReviews(f AdminReviewFilter) (items []*AdminReviewRow, total int)`.
3. Реализуйте метод в in-memory сторе (отдельный файл
   `store_admin_reviews.go`) и Postgres (отдельный файл
   `postgres_admin_reviews.go`). Фильтры/поиск/сортировка/пагинация — как в
   контракте; `total` считается до `limit`, `items` — не `nil`.
4. Новый handler `backend/internal/handler/admin_reviews.go` с
   `RegisterAdminReviewsRoutes(r chi.Router)`:
   - `GET /reviews` — только admin, разбор query, коды 400/401/403, ответ по
     контракту;
   - не дублируйте существующий `DELETE /reviews/{id}` из `admin_console.go`.
   Подключите регистрацию внутри `r.Route("/admin", ...)` в `middleware.go`.
5. Тесты `backend/tests/contract/admin_reviews_test.go`: пустой список,
   фильтры course/rating/q/from-to, сортировки, пагинация/total, 400 на
   неверные UUID/rating/даты/sort/dir, 401/403, удаление отзыва админом и
   пересчёт рейтинга. Postgres-parity —
   `backend/tests/postgres/admin_reviews_test.go` (если в проекте принят
   такой паттерн; иначе добавьте в существующий parity-набор).
6. Короткая приписка в `docs/API.md`.

## Разрешённые файлы

- `backend/internal/repo/admin_reviews.go` (новый)
- `backend/internal/repo/store_admin_reviews.go` (новый)
- `backend/internal/repo/postgres_admin_reviews.go` (новый)
- `backend/internal/repo/store_iface.go`
- `backend/internal/handler/admin_reviews.go` (новый)
- `backend/internal/handler/middleware.go`
- `backend/tests/**`, `docs/API.md`
- `backend/tests/T83-admin-reviews.md` (отчёт)

## Запрещено

- `Task/**`, `specs/**`, `frontend/**`, `frontend/src/i18n/ru.json`,
  миграции, `docker-compose*`, `qa-*.ps1`, `.github/**`, `deploy/**`.
- Менять существующие формы ответов, кроме аддитивных полей.
- `git push`, `git merge`, смена ветки, работа вне `wt/t83`.

## Проверка

Go — только через Docker, из корня worktree:

```
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Postgres-parity: контейнер `postgres:16-alpine` с именем `p10-t83-pg` на порту
**55451** (edu/edu, БД edu_test), хост из контейнера тестов —
`host.docker.internal`; после прогона остановите и удалите контейнер. Порт
55432/55441–55450 не использовать.

## Отчёт

`backend/tests/T83-admin-reviews.md`: «Что сделано», «Проверка» (команды и
результаты), «Отклонения», «Открытые вопросы». Закоммитьте код и отчёт явными
путями (не `git add .`).
