# T33 — Правка карточки курса: заморозка в архиве (backend)

**Волна:** P4 · **Ветка:** `task/T33-course-card-guard` · **Worktree:** `wt/t33`
**Контракт:** `Task/00-api-contract.md`, раздел «Эндпоинты волны P4» → «Правка карточки курса (T33)». Контракт заморожен.

## Зачем

`PATCH /teacher/courses/{id}` меняет карточку в любом статусе, включая `archived` — то есть
можно переписать архивный курс, который уже снят с продажи и заморожен. Остальные операции
(`submit`, структура, архивация) статус проверяют. Это последняя дырка в правилах жизненного цикла.

## Что сделать

1. В `TeacherUpdateCourse` (`backend/internal/handler/teacher_edit.go`) сразу после `ownCourse`
   (до валидации тела) добавить:
   `if c.Status == repo.CourseArchived { 409 «Курс в архиве: карточка заморожена» }` —
   состояние ресурса важнее валидации тела (контракт).
2. Поведение в `draft | review | published` не меняется (осознанное исключение: карточка не
   проходит модерацию заново) — это надо явно указать в комментарии функции, чтобы следующий
   читатель не «исправил» его на `requireDraft`.
3. Админ правит чужие курсы тем же маршрутом — guard действует и для него.
4. Найти тест, фиксирующий текущее (всеядное) поведение:
   `grep` по `backend --include=*_test.go -rn "TeacherUpdateCourse\|courses/{id}" .`
   (вероятно `backend/internal/handler/teacher_crud_test.go` и/или
   `backend/tests/contract/teacher_test.go`). Обновить его под новый контракт **и** добавить
   проверку `409` в архиве. Если существующий тест лежит в файле, где рядом много чужих проверок —
   правь аккуратно только относящееся к PATCH карточки.

## Тесты (обязательные случаи)

- `draft` → `200`, поля обновились;
- `review` → `200`;
- `published` → `200`;
- `archived` → `409`, курс в сторе **не изменился** (проверить `title`/`priceCents`);
- `archived` + пустое тело → `409` (guard раньше валидации);
- занятый slug → `400`, пустое название → `400` (регрессия не сломана).

## Ограничения

- Править только: `backend/internal/handler/teacher_edit.go`, тестовые файлы, где живут проверки
  PATCH карточки, при необходимости новый `backend/tests/contract/teacher_card_status_test.go`,
  отчёт `backend/tests/T33-course-card-guard.md`.
- `Task/**`, `frontend/**`, `specs/**` — не трогать. Новых зависимостей нет. По-русски.

## Проверка (Docker)

```powershell
docker run --rm -v "C:\Users\96352\all_project\eduPlatform\edu-platform\backend:/src" -v gomodcache:/go/pkg/mod -w /src -e GOFLAGS=-mod=mod golang:1.24-alpine sh -c "go build ./... && go vet ./... && go test ./... -count=1"
```

## Отчёт

`backend/tests/T33-course-card-guard.md`: что изменено, какие тесты обновлены (файл + имена),
вывод прогона, честный список непроверенного.
