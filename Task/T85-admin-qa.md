# T85 — Бэкенд: глобальная модерация Q&A

**Волна:** P10  **Ветка:** `task/T85-admin-qa`  **Worktree:** `wt/t85`

## Суть

Сейчас вопросы/ответы видны только на странице урока; админ не может найти
проблемные обсуждения по всей платформе. Нужен глобальный список Q&A с
фильтрами и поиском. Контракт — `GET /admin/questions` в разделе «Волна P10»
`Task/00-api-contract.md` (заморожен). Удаление использует существующие
маршруты урока; admin уже имеет право удалять чужие вопросы/ответы.

Работать только в `wt/t85` (ветка `task/T85-admin-qa`), база —
`integration/p10`.

## Что сделать

1. Новый файл `backend/internal/repo/admin_questions.go`: типы
   `AdminQuestionFilter` (`CourseID`, `LessonID`, `Q`, `Page`, `Limit`) и
   `AdminQuestionRow`/`AdminAnswerRow` с полями строго по контракту T85
   (включая courseTitle/courseSlug/lessonTitle, studentEmail/studentName,
   authorEmail/authorName/authorRole, answers). Не кладите типы в
   `models.go`.
2. В `StoreAPI` добавьте
   `ListAdminQuestions(f AdminQuestionFilter) (items []*AdminQuestionRow, total int)`.
3. Реализуйте в `store_admin_questions.go` и
   `postgres_admin_questions.go`: фильтры, поиск без учёта регистра по
   вопросам/ответам/email/именам/названиям, пагинация, total до limit,
   сортировка вопросы новые первыми, ответы старые первыми, `answers` — не
   `nil`.
4. Новый handler `backend/internal/handler/admin_questions.go`,
   `RegisterAdminQuestionsRoutes(r chi.Router)`:
   `GET /questions` только admin; query-валидация 400, 401/403, ответ по
   контракту. Подключите внутри `r.Route("/admin", ...)` в `middleware.go`.
5. Тесты `backend/tests/contract/admin_questions_test.go`: пустой список,
   фильтры course/lesson/q, поиск по ответу и автору, пагинация/total,
   сортировка, 400 на UUID/пагинацию, 401/403. Проверьте, что admin удаляет
   чужой вопрос и ответ существующими маршрутами, teacher — нет. Postgres
   parity — `backend/tests/postgres/admin_questions_test.go` (или в
   существующем parity-наборе).
6. Приписка в `docs/API.md`.

## Разрешённые файлы

- `backend/internal/repo/admin_questions.go` (новый)
- `backend/internal/repo/store_admin_questions.go` (новый)
- `backend/internal/repo/postgres_admin_questions.go` (новый)
- `backend/internal/repo/store_iface.go`
- `backend/internal/handler/admin_questions.go` (новый)
- `backend/internal/handler/middleware.go`
- `backend/tests/**`, `docs/API.md`
- `backend/tests/T85-admin-qa.md` (отчёт)

## Запрещено

- `Task/**`, `specs/**`, `frontend/**`, `ru.json`, миграции,
  `docker-compose*`, `qa-*.ps1`, `.github/**`, `deploy/**`.
- Менять существующие формы ответов, кроме аддитивных полей.
- `git push`, `git merge`, смена ветки, работа вне `wt/t85`.

## Проверка

```
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Postgres-parity: `postgres:16-alpine`, имя `p10-t85-pg`, порт **55452**
(edu/edu, edu_test); хост `host.docker.internal`; после прогона удалить
контейнер.

## Отчёт

`backend/tests/T85-admin-qa.md`: что сделано, проверка, отклонения, открытые
вопросы. Коммит явными путями.
