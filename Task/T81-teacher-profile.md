# T81 — Бэкенд: публичный профиль преподавателя

**Волна:** P9  **Ветка:** `task/T81-teacher-profile`  **Worktree:** `wt/t81`

## Суть

У преподавателя нет публичной страницы: ученик видит только имя рядом с
курсом. Волна P9 добавляет публичный профиль со статистикой и курсами. Раздел
«Волна P9 (T71–T82)» в `Task/00-api-contract.md` — источник правды,
**заморожен**.

Работать только в своём worktree `wt/t81` (ветка `task/T81-teacher-profile`).

## Что сделать

1. StoreAPI (блок методов в конце `store_iface.go`):
   `GetTeacherProfile(teacherID string) (*TeacherProfileData, bool)` —
   пользователь + агрегаты + published-курсы. Реализация в `store.go` и
   `postgres_store.go` (агрегирующий запрос, без N+1); parity.
2. Модели в `repo/models.go`: `TeacherPublic` (`id, displayName, coursesCount,
   studentsCount, avgRating, ratingsCount`) и `TeacherProfileData`
   (`teacher`, `courses []*Course`).
3. Хендлер в новом `handler/teachers.go`:
   `GET /api/v1/teachers/{id}` — публичный, без авторизации. Если пользователя
   нет или роль не teacher/admin → `404 «Преподаватель не найден»`. Ответ
   `200 {"teacher":{…},"courses":[CourseItem]}`; курсы — только published,
   сортировка: активные записи desc, затем `createdAt` desc. `avgRating` и
   `studentsCount` считайте только по published-курсам; `coursesCount` — их
   число. Регистрация — в публичной секции `middleware.go`.
4. Тесты `backend/tests/contract/teacher_profile_test.go`: профиль без входа,
   счётчики (курсы/ученики/рейтинг), только published-курсы, порядок, 404 для
   несуществующего id, 404 для student-аккаунта, пустой displayName. Parity в
   `backend/tests/postgres/teacher_profile_test.go`.
5. Короткая приписка в `docs/API.md`.

Существующие тесты должны остаться зелёными. Правки существующих тестов
допустимы только при изменении контракта — обоснуйте.

## Разрешённые файлы

- `backend/internal/repo/**`
- `backend/internal/handler/**`
- `backend/tests/**`
- `docs/API.md`
- `backend/tests/T81-teacher-profile.md` (отчёт)

## Запрещено

- `Task/**`, `specs/**`, `frontend/**`, `ru.json`, миграции 001–015,
  `docker-compose*`, `qa-*.ps1`, `.github/**`, `deploy/**`.
- `git push`, `git merge`, смена ветки, работа вне `wt/t81`.

## Проверка (только через Docker)

```
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Postgres-parity: контейнер `p9-t81-pg` на порту **55451** (edu/edu, edu_test),
хост из тестового контейнера — `host.docker.internal`; после прогона удалите
контейнер.

## Отчёт

`backend/tests/T81-teacher-profile.md`: «Что сделано», «Проверка»,
«Отклонения», «Открытые вопросы». Закоммитьте отчёт и код в ветку
`task/T81-teacher-profile`.
