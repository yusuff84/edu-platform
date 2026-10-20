# T71 — Бэкенд: вопросы и ответы по урокам

**Волна:** P9  **Ветка:** `task/T71-lesson-qa`  **Worktree:** `wt/t71`

## Суть

Ученики не могут задать вопрос по уроку, а преподаватель — ответить публично.
Волна P9 добавляет Q&A: вопросы к конкретному уроку, ответы учеников и
преподавателя, удаление своих сообщений. Раздел «Волна P9 (T71–T82)» в
`Task/00-api-contract.md` — источник правды, **заморожен**.

Работать только в своём worktree `wt/t71` (ветка `task/T71-lesson-qa`).

## Что сделать

1. Миграция `backend/internal/migrate/migrations/012_lesson_questions.sql`:
   таблицы `lesson_questions` и `lesson_answers` по контракту, FK каскадом,
   индексы `(course_id, lesson_id, created_at)`, `(question_id, created_at)`;
   идемпотентно, в стиле `011_review_reply.sql`.
2. `repo/models.go`: `LessonQuestion`, `LessonAnswer` с JSON-тегами по
   контракту (включая вычисляемые `lessonTitle`, `studentName`, `authorName`,
   `authorRole`, `isTeacher`, `answers`).
3. StoreAPI (блок методов в конце `store_iface.go`): создание вопроса, список
   вопросов курса (фильтр по lessonID, пагинация, total), получение вопроса,
   создание ответа, получение ответа, удаление вопроса/ответа. Реализуйте в
   `store.go` и `postgres_store.go`; parity. Имена/время — из существующих
   сущностей (users.display_name, lessons.title).
4. Хендлеры в новом `handler/lesson_qa.go`, регистрация в `middleware.go`:
   публичный `GET /courses/{id}/questions`, авторизованные
   `POST /courses/{id}/questions`, `POST /courses/{id}/questions/{questionId}/answers`,
   `DELETE /courses/{id}/questions/{questionId}`,
   `DELETE /courses/{id}/questions/{questionId}/answers/{answerId}`.
   Все проверки прав, коды и тексты ошибок — строго по контракту. Для проверки
   записи переиспользуйте существующий метод стора/хелпер активной записи; для
   владения курсом — существующий хелпер teacher/admin.
5. Тесты `backend/tests/contract/lesson_qa_test.go`: публичный список,
   пагинация/фильтр по уроку, вопрос без записи (403), вопрос владельцем,
   ответ учеником и преподавателем (`isTeacher`), удаление автором/владельцем/
   админом, 403 чужому, 404 чужого вопроса, валидация длин, неопубликованный
   курс. Parity-тесты в `backend/tests/postgres/lesson_qa_test.go`.
6. Короткая приписка в `docs/API.md` по новым эндпоинтам.

Существующие тесты должны остаться зелёными. Правки существующих тестов
допустимы только при изменении контракта — обоснуйте в отчёте.

## Разрешённые файлы

- `backend/internal/migrate/migrations/012_lesson_questions.sql` (новый)
- `backend/internal/repo/**`
- `backend/internal/handler/**`
- `backend/tests/**`
- `docs/API.md`
- `backend/tests/T71-lesson-qa.md` (отчёт)

## Запрещено

- `Task/**`, `specs/**`, `frontend/**`, `ru.json`, миграции 001–011 и 013–015,
  `docker-compose*`, `qa-*.ps1`, `.github/**`, `deploy/**`.
- Менять формы существующих ответов (только аддитивные поля).
- `git push`, `git merge`, смена ветки, работа вне `wt/t71`.

## Проверка (только через Docker)

Из корня worktree:

```
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Postgres-parity: контейнер `postgres:16-alpine` с именем `p9-t71-pg` на порту
**55441** (edu/edu, БД edu_test); из контейнера тестов хост —
`host.docker.internal`; после прогона остановите и удалите контейнер. Не
используйте порт 55432 — он занят.

## Отчёт

`backend/tests/T71-lesson-qa.md`: «Что сделано», «Проверка» (команды и
результаты), «Отклонения от постановки», «Открытые вопросы». Закоммитьте отчёт
и код в ветку `task/T71-lesson-qa`.
