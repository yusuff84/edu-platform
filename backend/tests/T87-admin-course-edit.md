# T87 — редактирование курса супер-админом

**Волна:** P10  **Ветка:** `task/T87-admin-course-edit`  **Worktree:** `wt/t87`
**Статус:** ✅ готово (коммит в ветке, без push/merge)

## Что сделано

- **`backend/internal/handler/teacher_crud.go`**
  - добавлены хелперы `canEditContent(u, c)` и `requireEditable(w, u, c)`:
    `admin` правит любой курс, кроме `archived` (409); остальные — только
    `draft` (409 с прежним текстом T19
    «Контент курса можно редактировать только в режиме черновика»);
  - на `requireEditable` переведены: `PATCH` урока, удаление модуля/урока,
    reorder модулей/уроков. У `PATCH` урока сохранён T75-исключение:
    teacher может менять только `isPreview` у published-курса; для admin
    архив закрыт и в этом случае;
  - удаление всего курса (`DELETE /teacher/courses/{id}`) осознанно осталось
    `draft`-only для всех (см. «Отклонения»), поэтому `requireDraft` сохранён
    только для него.
- **`backend/internal/handler/teacher.go`**
  - создание модуля/урока: для admin архивный курс → 409 до `unarchive`;
  - поведение teacher не менялось: у создания модуля/урока исторически нет
    `draft`-guard (это используют существующие контрактные тесты, например
    добавление уроков в published-курс в T22/T16).
- **`backend/internal/handler/teacher_lifecycle.go`**
  - перенос урока: admin может переносить в любом неархивном курсе; для
    teacher сохранён прежний 409 с текстом T21
    «Менять структуру можно только в черновике»; архив — 409 и для admin.
  - `submit`, `archive`, `unarchive`, `withdraw`, `unpublish` не менялись.
- **`backend/internal/handler/admin_course_publish.go`** (новый)
  - `POST /admin/courses/{id}/publish` — только admin (`adminOnly` +
    middleware группы `/admin`);
  - `draft|review → published` без очереди модерации, `moderationComment`
    очищается; `published` — идемпотентный `200`; `archived` — `409`;
    нет курса — `404`; ответ — `200 {"course": {...}}`;
  - пустой draft публикуется: force publish не повторяет submit-guard
    «минимум 1 модуль и 1 урок».
- **`backend/internal/handler/middleware.go`** — маршрут
  `POST /admin/courses/{id}/publish` зарегистрирован внутри
  `r.Route("/admin", ...)` рядом с `moderate`.
- **`backend/internal/handler/admin_course_detail.go`** — в `course` добавлено
  аддитивное `moderationComment: string` (всегда ключ; пустая строка, если
  комментария нет). Остальные поля не менялись; `repo.Course` с `omitempty`
  не используется, чтобы ключ был стабильным.
- **`docs/API.md`** — блок T87 в разделе «Преподаватель», строка
  `/admin/courses/{id}/publish` и обновлённая форма
  `GET /admin/courses/{id}` (+`moderationComment`).
- **`backend/tests/contract/admin_course_detail_p7_test.go`** — ожидаемый набор
  полей `course` дополнен `moderationComment` (иначе точный `wantKeys` T55
  падал бы на аддитивном поле).

## Контрактные тесты

`backend/tests/contract/admin_course_edit_test.go` (пакет `contract_test`,
сквозь `NewRouter`; фикстуры — существующие `newTestApp`, `doRequest`,
`createDraftCourse`, `adminCourseDetail`):

| Тест | Проверяет |
|---|---|
| `TestAdminCourseEditPublished` | admin правит урок и модуль published-курса, создаёт модуль/урок, меняет порядок модулей и уроков, переносит урок между модулями, удаляет урок и пустой модуль; статус остаётся `published` |
| `TestAdminCourseEditReview` | те же операции на review-курсе; статус остаётся `review` |
| `TestTeacherContentGuardsPreserved` | teacher на published/review: PATCH урока, DELETE урока/модуля, reorder модулей/уроков и перенос — 409 с прежними текстами T19/T21 |
| `TestAdminCourseEditArchived` | admin на archived: все контентные операции и force publish — 409; после `unarchive` правка снова работает |
| `TestAdminCoursePublish` | publish из draft (комментарий очищен) и review; идемпотентность published; пустой draft; archived — 409; 404 (UUID и не-UUID); teacher — 403; аноним — 401 |
| `TestAdminCourseDetailModerationComment` | `moderationComment` всегда присутствует: пустой без замечаний, текст после reject, снова пустой после force publish |

## Проверка (Docker)

```
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Результат: `go vet` — без замечаний; `go test ./...` — все пакеты `ok`
(`tests/contract` — ok). Отдельный прогон T87-тестов
(`go test ./tests/contract -run 'TestAdminCourse(EditPublished|EditReview|EditArchived|Publish|DetailModerationComment)|TestTeacherContentGuardsPreserved' -count=1 -v`)
— 6/6 PASS.

## Отклонения / решения

1. **Удаление курса не расширено на admin.** Задача помечала
   `delete course?` вопросом; контракт T87 говорит про содержимое (карточка,
   модули, уроки, квиз, codeTask, порядок, перенос), а существующий
   `TestTeacherDeleteCourse` прямо закрепляет `409` для admin на published.
   Каскадное удаление курса с платежами/отзывами оставлено `draft`-only.
2. **Создание модуля/урока у teacher не получило новый draft-guard.**
   В `teacher.go` этого guard не было, и несколько существующих контрактных
   тестов (T22 `TestTeacherStudentsProgressAndOrder`, T16
   `TestStartsAtPersisted`) добавляют уроки в published-курс. По условию
   «правила teacher не меняются» оставлено как было; для admin добавлена
   только защита архива (409).
3. **Идемпотентный publish уже опубликованного курса** не пишет статус
   повторно, если `moderationComment` пуст (не двигает `updatedAt`); если
   комментарий остался — очищается.
4. **Store не менялся** (`AdminCourseDetail` уже отдаёт
   `ModerationComment` и в in-memory, и в Postgres; `SetCourseStatus` есть в
   обоих). Postgres-parity не запускался — постановка требует его только при
   изменении store.

## Открытые вопросы

Нет. Поведение teacher на создание модуля/урока вне draft зафиксировано как
существующее (см. отклонение 2); если продукт захочет закрыть и его — это
отдельная задача с правкой существующих контрактных тестов.
