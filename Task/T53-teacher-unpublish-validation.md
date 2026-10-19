# T53 — Бэкенд: жизненный цикл учителя и жёсткая валидация (backend)

**Волна:** P7  **Ветка:** `task/T53-teacher-unpublish-validation`  **Worktree:** `wt/t53`

## Суть

1. Учитель не может снять опубликованный курс с публикации (у админа есть
   `/admin/courses/{id}/unpublish`, у автора — только archive). Из-за этого
   «удалить курс» в списке курсов бьётся об `409` и плохо объяснён в UI.
2. `PATCH` урока принимает `startsAt` только в RFC3339, а форма в UI
   (`datetime-local`) шлёт `ГГГГ-ММ-ДДТЧЧ:ММ` — противоречие, закрываем бэком.
3. Quiz-уроки на бэкенде не валидируются (можно сохранить вопрос без текста,
   с одним вариантом, без правильного ответа).
4. В `handler/teacher.go` остался мёртвый дубль регистрации `GET /teacher/courses`
   (перекрывается `RegisterTeacherCRUDRoutes`) — почистить.

Контракт: `Task/00-api-contract.md`, раздел «Эндпоинты волны P7 (T53–T55)».
**Поля и тексты ошибок менять нельзя — раздел заморожен.**

## Что сделать

1. `backend/internal/handler/teacher_lifecycle.go`:
   - `POST /courses/{id}/unpublish` (хендлер `TeacherUnpublishCourse`):
     владелец или admin (модель прав — как в `TeacherArchiveCourse`);
     `published → draft` через `a.Store.SetCourseStatus(...)`; ответ
     `200 {"course":{...}}` (прецедент — `respondFreshCourse`);
     `409 «Снять с публикации можно только опубликованный курс»` при статусе
     не `published`; `404 «Курс не найден»`; `403` — чужой курс. Тело запроса
     игнорируется, пустые тела допустимы (как у archive/withdraw).
2. `backend/internal/handler/teacher_edit.go` (PATCH урока):
   - `startsAt` грамматика: RFC3339 **или** `YYYY-MM-DDTHH:mm` (datetime-local
     без зоны → UTC). Парсер общий с POST (вынести в функцию
     `parseLessonStartsAt`), ответ/ошибки — как было (`400` на мусор).
3. Валидация quiz (POST в `teacher.go`/`teacher_edit.go`, где создаются уроки,
   и PATCH): вопрос без текста, <2 вариантов, пустой вариант текста,
   `correctIndex` вне границ или неопределённый → `400 «Некорректный тест: …»`
   (текст свободный, код — 400). Старые уроки не рестриктурируются.
4. Мёртвый код: удалить дублирующую регистрацию `GET /teacher/courses` в
   `teacher.go` (оставить одну в `RegisterTeacherCRUDRoutes`); убедиться, что
   `go vet ./...` чистый.
5. Маршрут добавить в `backend/internal/handler/middleware.go` внутри
   `RegisterTeacherLifecycleRoutes`.

## Разрешённые файлы

- `backend/internal/handler/teacher_lifecycle.go`, `teacher_edit.go`,
  `teacher.go` (только мёртвый код + валидация), `middleware.go` (только строка
  маршрута), `docs/API.md` (короткая приписка в разделе teacher);
- НОВЫЕ контрактные тесты: `backend/tests/contract/teacher_p7_test.go`.

## Запрещено

- Трогать `frontend/**`, `Task/**`, `specs/**`, `ru.json`, чужие handler-файлы,
  репозиторий (`internal/repo/**`) — методы уже есть (`SetCourseStatus`);
- Менять формы ответов существующих эндпоинтов;
- Запускать `docker compose` от чужого стенда и трогать контейнеры `olymp-*`.

## Проверка (только через Docker)

```
docker run --rm -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Тесты должны быть зелёные ДО push; контрактные тесты написать обязательно:
unpublish happy-path + 409 (draft/review/archived) + 403 чужой + PATCH
`startsAt` datetime-local(→UTC) + RFC3339 + 400 мусор + quiz-валидация (3
негатива).

## Отчёт

`backend/tests/T53-teacher-unpublish-validation.md`: что сделано, тесты, числа
(кол-во PASS), найденные расхождения с контрактом (писать, не править).
Коммит: `T53: ...`; push `task/T53-teacher-unpublish-validation` (с 2–3
ретраями при сетевых сбоях). Не мержить в main/integration — это делает координатор.
