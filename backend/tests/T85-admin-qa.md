# T85 — бэкенд: глобальная модерация Q&A

**Волна:** P10  **Ветка:** `task/T85-admin-qa`  **Worktree:** `wt/t85`

## Что сделано

1. **Типы и фильтр** `backend/internal/repo/admin_questions.go`:
   `AdminQuestionFilter` (`CourseID`, `LessonID`, `Q`, `Page`, `Limit`) и
   строки `AdminQuestionRow`/`AdminAnswerRow` строго по контракту T85:
   `courseTitle/courseSlug/lessonTitle`, `studentEmail/studentName`,
   `authorEmail/authorName/authorRole`, `answers`. Типы вынесены из
   `models.go` и не меняют формы публичной ленты T71.
2. **StoreAPI** (`store_iface.go`, блок T85 в конце интерфейса):
   `ListAdminQuestions(f AdminQuestionFilter) (items, total)`. Контракт
   метода: вопросы новые первыми (`createdAt desc, id desc`), ответы старые
   первыми (`createdAt asc, id asc`), `total` до пагинации, `items` и
   `answers` никогда не `nil`.
3. **In-memory-реализация** `store_admin_questions.go`: один проход по
   картам T71 под `RLock`, фильтры `courseId`/`lessonId`, регистронезависимый
   поиск по вопросу, ответам, email/именам студента и авторов ответов,
   названиям курса и урока, пагинация и total. Ответы собираются из
   `lessonAnswerIDs` и сортируются детерминированно.
4. **Postgres-реализация** `postgres_admin_questions.go`: `COUNT(*)` и
   страница с JOIN `courses/lessons/users`; поиск через `lower(...) LIKE`
   с параметрами, ответы страницы читаются одним запросом без N+1.
   Некорректные UUID на уровне стора дают пустую выборку (handler отвечает
   `400` раньше), SQL-приведение к `uuid` не падает.
5. **Handler** `backend/internal/handler/admin_questions.go`:
   `RegisterAdminQuestionsRoutes` + `AdminListQuestions` для
   `GET /admin/questions`. Query-валидация: `courseId`/`lessonId` — UUID,
   `page` — с 1, `limit` — 1..100, дефолты 1/20; мусор — `400`.
   Ответ — `{items,total,page,limit}`, `items` всегда массив.
   Маршрут подключён в `middleware.go` внутри `/admin`, поэтому
   действуют общие `requireAuth` и `requireRole(admin)`: без сессии `401`,
   teacher/student `403`.
6. **Контрактные тесты** `backend/tests/contract/admin_questions_test.go`:
   - `TestAdminQuestionsGlobalList`: пустой список (`[]`, page=1, limit=20),
     поля курса/урока/студента/автора ответа, порядок вопросов и ответов,
     фильтры `courseId`/`lessonId`, поиск по вопросу, ответу, имени автора
     ответа, имени/email ученика, названиям урока и курса, пагинация/total,
     страница за пределами, `400` на неверные UUID/`page`/`limit`,
     `401`/`403`;
   - `TestAdminQuestionsDeleteOthers`: посторонний teacher получает `403`
     при удалении чужого вопроса; admin удаляет чужой вопрос (ответы уходят
     каскадом) и чужой ответ существующими маршрутами T71.
7. **Postgres parity** `backend/tests/postgres/admin_questions_test.go`:
   `TestPostgresAdminQuestionsParity` (env-gated через
   `TEST_DATABASE_URL`): join-поля, порядок вопросов/ответов, фильтры,
   поиск по всем полям, пагинация/total, пустые `answers` как `[]`,
   мусорные UUID без SQL-ошибки.
8. **Документация** `docs/API.md`: строка `GET /api/v1/admin/questions`
   с query, формой ответа, сортировкой, семантикой `total` и ссылкой на
   существующие маршруты удаления.

## Проверка

Все проверки — в Docker, из корня worktree.

1. Полный прогон:
   ```
   docker run --rm -e HOME=/root -v "${PWD}\backend:/src" \
     -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine \
     sh -c "go vet ./... && go test ./... -count=1"
   ```
   — **успех, exit 0**: `cmd/api` 4.5 s, `internal/auth` 0.3 s,
   `internal/handler` 2.8 s, `internal/mailer` 0.1 s, `internal/payments`
   0.1 s, `internal/repo` 0.02 s, `tests/contract` 80.5 s,
   `tests/postgres` 0.02 s (без `TEST_DATABASE_URL` — skip). Пакеты без
   тестов — `config`, `migrate`, `service`, `storage`.
2. Postgres-parity: контейнер `p10-t85-pg` (`postgres:16-alpine`, порт
   **55452**, edu/edu, БД `edu_test`), миграции применяет `testStore`.
   ```
   docker run --rm --add-host=host.docker.internal:host-gateway \
     -e HOME=/root \
     -e TEST_DATABASE_URL="postgres://edu:edu@host.docker.internal:55452/edu_test?sslmode=disable" \
     -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src \
     golang:1.24-alpine \
     sh -c "go test ./tests/postgres/... -count=1 -v -run TestPostgresAdminQuestionsParity"
   ```
   — **PASS**, `ok … 1.428s`. Контейнер после прогона удалён.
3. `gofmt -l` по шести новым/изменённым Go-файлам T85 — пусто.

## Отклонения от постановки

- В контракте T85 написано `limit (20..100)`, при этом общий раздел
  контракта задаёт пагинацию как «`page` с 1, `limit` по умолчанию 20,
  максимум 100». Реализовано по общему правилу: дефолт 20, допустимый
  диапазон 1..100; `limit=0`, `limit=101` и нечисловые значения дают `400`.
- Глобальный список читает существующие таблицы T71
  (`lesson_questions`/`lesson_answers`), своих миграций и полей нет.
  Для ответа админки используются отдельные плоские строки, потому что
  контракт требует email/slug/имена, которых нет в `LessonQuestion`.
- Права admin на удаление чужих вопросов/ответов уже были реализованы в
  T71 (`canModerateQA`); код удаления не менялся, добавлены только
  контрактные проверки.
- Поиск в Postgres повторяет in-memory-семантику подстроки без учёта
  регистра. Символы `%`/`_` в `q` трактуются как wildcard `LIKE`; для
  обычных поисковых запросов поведение совпадает.

## Открытые вопросы

- Если `limit` в T85 должен быть именно не меньше 20 (буквальное чтение
  «20..100»), потребуется ужесточить валидацию до 20..100. Сейчас
  сохранено общее правило платформы (1..100, дефолт 20), чтобы не
  расходиться с остальными админ-списками.
- Контракт не задаёт поведение `q` с символами `%`/`_`; при необходимости
  буквального поиска их нужно экранировать и в in-memory, и в SQL-ветке.
