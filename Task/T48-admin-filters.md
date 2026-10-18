# T48 — Админка, бэкенд: фильтры/сортировка курсов и записей, unarchive, автор в очереди

**Волна:** P6 · **Ветка:** `task/T48-admin-filters` · **Worktree:** `wt/t48`

## Зачем

Раздел «Курсы» должен стать максимально удобным: сортировка по колонкам,
фильтр по преподавателю и датам создания, возврат из архива, а в очереди
модерации — email автора и объём курса. Плюс фильтры списка записей (поиск по
ученику и статус). Всё заморожено в `Task/00-api-contract.md`, раздел
«Эндпоинты волны P6».

## Что сделать

1. `GET /admin/courses` (handler/admin_console.go + repo):
   - `CourseFilter` (repo/admin_console.go): + `Sort`, `Dir`, `TeacherEmail`,
     `From`, `To`;
   - валидация `sort`/`dir` в хендлере с текстами контракта (400);
     `from/to` — через существующий `parseJournalPeriod` (переиспользовать,
     не дублировать);
   - сортировки: `createdAt`, `updatedAt`, `title` (без учёта регистра), `price`
     (priceCents), `students` (число учеников курса; в памяти — `CourseContentCounts`,
     в SQL — подзапрос); вторичный ключ — `createdAt DESC`;
   - `teacherEmail` — подстрока по email преподавателя без учёта регистра
     (в памяти — через `GetUserByID`, в SQL — `lower(u.email) LIKE lower('%...%')`);
   - реализации in-memory (`repo/admin_console.go`) и Postgres
     (`repo/postgres_admin_console.go`); старые параметры и формы ответов
     не меняются.
2. `POST /admin/courses/{id}/unarchive` (handler/admin_console.go):
   `archived → draft` (комментарий модерации сохраняется), ответ
   `{"course": <Course>}` (как archive/unpublish, T25), 404/409 по контракту;
   регистрация в `RegisterAdminConsoleRoutes`.
3. `GET /admin/courses/review` (handler/admin.go): аддитивно добавить каждому
   элементу `teacherEmail` ("" если автор не найден), `modulesCount`,
   `lessonsCount` — как в списке курсов (переиспользуй `CourseContentCounts`).
4. `GET /admin/enrollments` (handler/admin_console.go + repo): параметры
   `q` (подстрока без учёта регистра по email ИЛИ displayName ученика) и
   `status` (`active|refunded|revoked`; иное → 400 по контракту);
   `EnrollmentFilter` + in-memory и Postgres реализации.
5. Тесты `backend/tests/contract/admin_filters_test.go` (новый, стиль существующих):
   - сортировки asc/desc (`title`, `price`, `students` — на двух подготовленных
     курсах), вторичный ключ, невалидные `sort`/`dir` → 400;
   - `teacherEmail` сужает выборку; `from/to` (валидные + 400 на кривую дату и
     `from > to`);
   - unarchive: 200 `draft`, повтор → 409, случайный id → 404, студент → 403;
   - очередь: курс на модерации → `teacherEmail`, `modulesCount`, `lessonsCount`;
   - записи: `q` по email и по имени, `status`, невалидный статус → 400.
   Существующие тесты не ломать (в т.ч. фиксаторы форм ответов).

## Правила волны

- Работать только в своём worktree (`C:\Users\96352\all_project\eduPlatform\wt\t48`);
  коммит — в ветку `task/T48-admin-filters`; **push не делать** — ветки
  публикует координатор.
- Править только: `backend/internal/handler/{admin_console.go, admin.go}`,
  `backend/internal/repo/{store_iface.go, admin_console.go, postgres_admin_console.go}`,
  новые файлы тестов `backend/tests/contract/*` и отчёт.
- Go собирать и запускать **только в Docker**; новых внешних зависимостей нет;
  `specs/` и `ru.json` не трогать; контейнеры `olymp-*` не останавливать.
- SQL-реализации PG-методов обязательны; по возможности прогнать
  postgres-тесты по рецепту README (контейнер `edu-pgtest` на 55432 +
  `TEST_DATABASE_URL`) — если прогон не получится (сеть/Docker), честно
  отметить в отчёте.
- Формы ответов и тексты ошибок — строго по `Task/00-api-contract.md`;
  комментарии в коде — по-русски.

## Проверка

```powershell
# Полный прогон (alpine):
docker run --rm -v "C:\Users\96352\all_project\eduPlatform\wt\t48\backend:/src" -v gomodcache:/go/pkg/mod -v gocache:/root/.cache/go-build -w /src -e GOFLAGS=-mod=mod golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"

# Опционально: Postgres-паритет (рецепт README; сначала поднять edu-pgtest):
docker run --rm --network host -v "C:\Users\96352\all_project\eduPlatform\wt\t48\backend:/src" -v gomodcache:/go/pkg/mod -w /src -e GOFLAGS=-mod=mod -e TEST_DATABASE_URL="postgres://edu:edu@127.0.0.1:55432/edu_test?sslmode=disable" golang:1.24-alpine sh -c "go test ./tests/postgres/... -count=1 -v"
```

Хвосты вывода — в отчёт.

## Отчёт

`backend/tests/T48-admin-filters.md`: что сделано, команды, итоги, что осталось
непроверенным (если PG-прогон не выполнялся).
