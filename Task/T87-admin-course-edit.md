# T87 — Бэкенд: редактирование курса супер-админом

**Волна:** P10  **Ветка:** `task/T87-admin-course-edit`  **Worktree:** `wt/t87`

## Суть

Админ уже может открыть `/teacher/courses/{id}` (хелпер `ownCourse` пускает
`RoleAdmin`), но изменять содержимое опубликованного/на-модерации курса нельзя:
`requireDraft` и проверки статуса режут операции. Нужно, чтобы супер-админ мог
править любой **неархивный** курс, а также принудительно публиковать его.
Контракт — раздел T87 в «Волна P10» `Task/00-api-contract.md` (заморожен).

Работать только в `wt/t87` (ветка `task/T87-admin-course-edit`), база —
`integration/p10`.

## Что сделать

1. В `teacher_crud.go` замените `requireDraft` на хелпер, учитывающий роль:
   `requireEditable(w, u, c)` / `canEditContent(u, c)`:
   - `admin`: разрешено, если `c.Status != repo.CourseArchived` (архив — 409);
   - остальные: только `draft` (текущее поведение и тексты 409 сохранить).
   Проверьте все вызовы `requireDraft` в `teacher_crud.go` (update lesson,
   delete module/lesson/course?, reorder modules/lessons).
2. В `teacher.go` (создание модуля/урока) и `teacher_lifecycle.go` (если есть
   проверки статуса для контентных операций) добавьте тот же admin-bypass.
   Не меняйте правила `submit` для teacher; админ публикует через новый
   маршрут.
3. Новый handler `backend/internal/handler/admin_course_publish.go`:
   `POST /admin/courses/{id}/publish` — только admin; `draft|review` →
   `published`, `published` идемпотентно 200, `archived` 409, нет курса 404;
   очищает `moderationComment`; ответ `{"course": ...}` в форме `moderate`.
   Зарегистрируйте внутри `/admin` в `middleware.go`.
4. В `admin_course_detail.go` добавьте аддитивное поле
   `course.moderationComment` (пустая строка, если нет). Не ломайте текущие
   JSON-поля.
5. Тесты `backend/tests/contract/admin_course_edit_test.go`:
   - admin правит урок/модуль published и review-курса, создаёт/удаляет
     модуль и урок, меняет порядок;
   - teacher по-прежнему получает 409 на published/review;
   - archived — 409 для admin;
   - force publish из draft/review, идемпотентность published, 404/403/401;
   - `moderationComment` в `GET /admin/courses/{id}`.
6. Приписка в `docs/API.md`.

## Разрешённые файлы

- `backend/internal/handler/teacher.go`
- `backend/internal/handler/teacher_crud.go`
- `backend/internal/handler/teacher_lifecycle.go`
- `backend/internal/handler/admin_course_detail.go`
- `backend/internal/handler/admin_course_publish.go` (новый)
- `backend/internal/handler/middleware.go`
- `backend/tests/**`, `docs/API.md`
- `backend/tests/T87-admin-course-edit.md` (отчёт)

## Запрещено

- `Task/**`, `specs/**`, `frontend/**`, `ru.json`, миграции,
  `repo/**` (если без этого можно обойтись), `docker-compose*`, `qa-*.ps1`,
  `.github/**`, `deploy/**`.
- Менять публичные формы ответов, кроме аддитивного `moderationComment`.
- `git push`, `git merge`, смена ветки, работа вне `wt/t87`.

## Проверка

```
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Postgres-parity не требуется, если не менялся store; иначе — отдельный
контейнер на порту **55455** и parity-тесты.

## Отчёт

`backend/tests/T87-admin-course-edit.md`: что сделано, проверка, отклонения,
открытые вопросы. Коммит явными путями.
