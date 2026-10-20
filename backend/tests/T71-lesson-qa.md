# T71 — бэкенд: вопросы и ответы по урокам

**Волна:** P9  **Ветка:** `task/T71-lesson-qa`  **Worktree:** `wt/t71`

## Что сделано

1. **Миграция** `backend/internal/migrate/migrations/012_lesson_questions.sql`:
   таблицы `lesson_questions` и `lesson_answers` по контракту, FK на
   `courses/lessons/users/lesson_questions` с `ON DELETE CASCADE`,
   индексы `(course_id, lesson_id, created_at)` и
   `(question_id, created_at)`. Файл идемпотентен (`IF NOT EXISTS`) —
   раннер выполняет его при каждом запуске.
2. **Модели** `repo/models.go`: `LessonQuestion` и `LessonAnswer` с
   JSON-тегами контракта, включая вычисляемые `lessonTitle`, `studentName`,
   `authorName`, `authorRole`, `isTeacher`, `answers` (всегда не-nil).
3. **StoreAPI** (`store_iface.go`, блок T71 в конце интерфейса):
   `CreateLessonQuestion`, `ListLessonQuestions`, `GetLessonQuestion`,
   `CreateLessonAnswer`, `GetLessonAnswer`, `DeleteLessonQuestion`,
   `DeleteLessonAnswer`. Реализации:
   - in-memory `Store` (`store.go`, блок в конце файла): вопросы/ответы
     хранятся «сырыми», вычисляемые поля собираются при чтении из
     `users.display_name` и `lessons.title` (смена displayName сразу видна),
     наружу отдаются снимки; порядок — вопросы новые первыми, ответы старые
     первыми; удаление вопроса каскадом чистит ответы;
   - `PostgresStore` (`postgres_store.go`, блок в конце файла): JOIN-ы
     `users/lessons/courses`, CTE `INSERT … SELECT` проверяет принадлежность
     урока курсу и вопроса курсу, `isTeacher` = автор курса или admin;
     ответы ленты читаются одним запросом без N+1; мусорные/чужие id —
     `ErrNotFound`, как в T69.
4. **Хендлеры** `backend/internal/handler/lesson_qa.go`:
   публичный `GET /courses/{id}/questions` (фильтр `lessonId`, пагинация
   `page/limit` с дефолтами 1/20, максимум 100, нечисловые/вне диапазона —
   дефолт), `POST /courses/{id}/questions` (trim 10..2000, 201
   `{"question":…}`), `POST …/questions/{questionId}/answers` (trim 3..2000,
   201 `{"answer":…}`), `DELETE` вопроса/ответа (204). Права: активная запись,
   владелец курса, admin; удаление — автор, владелец, admin. Коды и тексты
   ошибок — строго по контракту; неопубликованный курс в публичной ленте —
   404 «Курс не найден» (владелец/admin видят).
5. **Маршруты** зарегистрированы в `handler/middleware.go` рядом с отзывами:
   лента публичная, мутации — под `requireAuth`.
6. **Тесты**:
   - `backend/tests/contract/lesson_qa_test.go` — 3 теста: публичная лента
     (пустой массив, порядок, фильтр урока, пагинация/дефолты, 404 чужого
     урока), права и валидация (401/403/201, владелец/admin, длины 10/2000,
     черновик), ответы и удаление (роли, `isTeacher`, 403 чужому, 404
     чужого вопроса/ответа, каскад);
   - `backend/tests/postgres/lesson_qa_test.go` — parity-roundtrip:
     вычисляемые поля, порядок, фильтр, пагинация, роли ответов, каскад,
     `ErrNotFound` для чужих/несуществующих id.
7. **Документация** `docs/API.md`: строка публичной ленты в таблице курсов и
   блок «Волна P9 (T71)» с формами моделей, запросов и ошибок.

## Проверка

Все проверки — в Docker, из корня worktree.

1. `go vet ./... && go test ./... -count=1` (golang:1.24-alpine) — **успех,
   exit 0**. Пакеты с тестами: `cmd/api`, `internal/auth`, `internal/handler`,
   `internal/payments`, `internal/repo`, `tests/contract` (46.2 s),
   `tests/postgres` (без TEST_DATABASE_URL — skip, 0.009 s); 4 пакета без
   тестов. Все существующие тесты зелёные, правки чужих тестов не
   потребовались.
2. Postgres-parity: контейнер `p9-t71-pg` (`postgres:16-alpine`, порт
   **55441**, edu/edu, БД `edu_test`), миграции применяет сам `testStore`.
   ```
   TEST_DATABASE_URL=postgres://edu:edu@host.docker.internal:55441/edu_test?sslmode=disable \
     go test ./tests/postgres -count=1
   ```
   — **успех, exit 0**, `ok … 6.730s`, в т.ч. новый
   `TestPostgresLessonQARoundtrip` PASS. Контейнер после прогона удалён.

## Отклонения от постановки

- Для проверки владения курсом не вызывался `ownCourse`: он жёстко требует
  роль teacher/admin и отвечает `403 «Это чужой курс»`, тогда как T71
  разрешает удаление автору-ученику и требует контрактные тексты
  `403 «Нет прав на удаление»` / `403 «Задавать вопросы…»`. Использована та
  же предикатная проверка (`Course.TeacherID == user.ID || role == admin`) в
  локальных хелперах `canParticipateInQA`/`canModerateQA`; активная запись
  проверяется существующим `Store.GetEnrollment` + `EnrollmentActive`.
- Методы стора и блоки интерфейса добавлены в конец файлов, чужие участки не
  переформатировались (gofmt затронул только добавленные строки и выравнивание
  полей структуры `Store`).
- Миграция 012 содержит только таблицы/индексы; сиды не добавлялись.

## Открытые вопросы

- Контракт не описывает поведение POST-мутаций для неопубликованного
  (в частности архивного) курса: сейчас вопрос/ответ разрешены по правам
  (активная запись/владелец/admin) без проверки статуса, а публичная лента
  для не-владельца/не-админа отдаёт 404. Если архивный курс должен запрещать
  новые сообщения — потребуется отдельное правило и текст ошибки.
- `studentName`/`authorName` — это `users.display_name` как есть; при пустом
  displayName в JSON уходит пустая строка (контракт fallback на email не
  задаёт). Фронтенду нужно быть готовым к пустому имени.
