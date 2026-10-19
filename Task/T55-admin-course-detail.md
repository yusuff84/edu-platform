# T55 — Бэкенд: детали курса для админа (backend)

**Волна:** P7  **Ветка:** `task/T55-admin-course-detail`  **Worktree:** `wt/t55`

## Суть

У админа нет страницы «одним экраном»: программа курса (модули/уроки),
ученики, отзывы и деньги. Контракт P7 вводит `GET /admin/courses/{id}` —
замороженный раздел «T55» в `Task/00-api-contract.md`.

## Что сделать

1. `backend/internal/handler/admin_course_detail.go` (НОВЫЙ):
   - `AdminCourseDetail` — `GET /admin/courses/{id}` (role admin; в стеке
     `requireAuth + requireRole(admin)` уже есть на группе `/admin` в
     `middleware.go` — подключи аналогично соседям);
   - ответ строго по контракту: `course` (форма элемента
     `GET /admin/courses` T25/T48 + `coverPath` из T54; если T54 ещё не влита,
     читай форму из смежных полей — `cover_path` кол. через миграцию может
     отсутствовать: тогда `coverPath` пиши `null`, поле оставить, polling
     после merge T54 мониторится координатором);
   - `modules` — по `position`, внутри `lessons` по `position`, урок — только
     `{id,title,kind,position}`; пустой массив вместо null;
   - `stats`: `studentsCount` (все записи), `activeCount` (статус active),
     `reviewsCount`, `avgRating`, `paymentsCount`, `paidTotalCents`
     (только `succeeded`).
   - `404 «Курс не найден»` на несуществующий id; `403` — не админ;
   - `kind` — тот же enum, что в уроках (`video_text|live|quiz|code`).
2. Постройка SQL — переиспользуй существующие запросы из
   `postgres_admin_console.go` (`AdminListCourseDetails`), NEW SQL в
   `postgres_course_detail.go` (один запрос/несколько + склейка),
   метод задекларировать в `store_iface.go` + in-memory `store.go`.
3. Группы права: НЕ переносить эндпоинт в чужие файлы; отзывы по курсу не
   трогать (уже есть T25 `GET /admin/courses/{id}/reviews`
   и `DELETE /admin/reviews/{id}`).

## Разрешённые файлы

- НОВЫЕ: `backend/internal/handler/admin_course_detail.go`,
  `backend/internal/repo/postgres_course_detail.go`,
  `backend/tests/contract/admin_course_detail_p7_test.go`;
- ПРАВКИ: `backend/internal/handler/middleware.go` (только регистрация
  маршрута), `backend/internal/repo/store_iface.go`, `store.go`
  (метод + in-memory реализация), `backend/internal/repo/postgres_admin_console.go`
  — только если доступен переиспользуемый SQL для средней склейки;
- `docs/API.md` — приписка в разделе admin.

## Запрещено

- Трогать `frontend/**`, `Task/**`, `specs/**`, `ru.json`, чужие хендлеры;
- Менять существующие ответы (только добавление нового endpoint);
- Останавливать контейнеры `olymp-*`.

## Проверка (только через Docker)

```
docker run --rm -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Контрактные тесты: happy path (курс с 2 модулями и уроками разных kinds —
порядок position, формы полей); 404; 403 (teacher-у сессия); 401; stats на
ин-memory фиксarbonе (0 при пустом курсе); админ-идентичность — поля
`teacherEmail` не пустые у создателя.

## Отчёт

`backend/tests/T55-admin-course-detail.md`: те же правила (что в T53).
Коммит `T55: ...`, push `task/T55-admin-course-detail`.
