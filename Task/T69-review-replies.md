# T69 — Бэкенд: ответы преподавателя на отзывы и разбивка оценок

**Волна:** P8  **Ветка:** `task/T69-review-replies`  **Worktree:** `wt/t69`

## Суть

Отзывы односторонние: преподаватель не может ответить публично, а на странице
курса нет разбивки оценок. Волна P8 добавляет ответ учителя (создание/замена/
удаление), фильтр отзывов по оценке и гистограмму `breakdown`. Раздел
«Эндпоинты волны P8 (T63–T70)» в `Task/00-api-contract.md` — источник правды,
**заморожен**.

Работать только в своём worktree `wt/t69` (ветка `task/T69-review-replies`).

## Что сделать

1. Миграция `backend/internal/migrate/migrations/011_review_reply.sql`:
   `teacher_reply TEXT NOT NULL DEFAULT ''`, `teacher_reply_at TIMESTAMPTZ NULL`
   (идемпотентно, в стиле `007_cover.sql`).
2. `repo/models.go`: `TeacherReply string` (`json:"teacherReply"`) и
   `TeacherReplyAt *time.Time` (`json:"teacherReplyAt"`); маппинг во всех
   выборках/вставках Postgres и in-memory стора, во всех видах ответов с
   отзывами (страница курса, админ-списки, прочие существующие ручки).
3. StoreAPI (блок методов в конце интерфейса): установка ответа (текст + время),
   очистка ответа, статистика оценок курса (по 1..5). Postgres и in-memory;
   parity.
4. `PUT /teacher/courses/{id}/reviews/{reviewId}/reply` — владелец курса или
   admin: тело `{"body":"…"}`, trim 3..2000, иначе `400 «Ответ: от 3 до 2000
   символов»`; повторный PUT заменяет текст и обновляет `teacherReplyAt`;
   ответ `200 {"review": <Review>}`. `403` — чужой курс, `404 «Отзыв не найден»`
   (в т.ч. отзыв другого курса). Регистрация рядом с teacher-маршрутами.
5. `DELETE /teacher/courses/{id}/reviews/{reviewId}/reply` — те же права,
   `204` (если ответа не было — тоже `204`), `403`/`404`.
6. `GET /courses/{id}/reviews`: новый необязательный параметр `rating` (1..5,
   иное → `400 «Оценка должна быть числом от 1 до 5»`); `items`/`total` — по
   фильтру, `avgRating`/`ratingsCount` — по всем отзывам; аддитивное поле
   `breakdown` — объект со строковыми ключами `"1".."5"` и числами (все отзывы
   курса). Форма конверта прежняя.
7. Тесты `backend/tests/contract/`: права (403), 404 для чужого отзыва,
   валидация длины, создание/замена/удаление ответа, `teacherReply` в ответах,
   фильтр по оценке (`total`, `items`), `breakdown` и независимость
   `avgRating`/`ratingsCount` от фильтра. Parity-тесты новых методов стора в
   `backend/tests/postgres/`.
8. Короткая приписка в `docs/API.md`.

Существующие тесты должны остаться зелёными. Правки существующих тестов
допустимы только при изменении контракта — обоснуйте в отчёте.

## Разрешённые файлы

- `backend/internal/migrate/migrations/011_review_reply.sql` (новый)
- `backend/internal/repo/**`
- `backend/internal/handler/**`
- `backend/tests/**`
- `docs/API.md`
- `backend/tests/T69-review-replies.md` (отчёт)

## Запрещено

- `Task/**`, `specs/**`, `frontend/**`, `ru.json`, миграции 001–010,
  `docker-compose*`, `qa-*.ps1`, `.github/**`, `deploy/**`.
- Менять формы существующих ответов (только аддитивные поля).
- `git push`, `git merge`, смена ветки, работа вне `wt/t69`.

## Проверка (только через Docker)

Из корня репозитория:

```
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Postgres-parity: `postgres:16-alpine` на порту 55432 (БД `edu_test`, `edu/edu`),
прогон с `TEST_DATABASE_URL` (хост из контейнера тестов — `host.docker.internal`).
Вывод приложите к отчёту.

## Отчёт

`backend/tests/T69-review-replies.md`: «Что сделано», «Проверка» (команды и
результаты), «Отклонения от постановки», «Открытые вопросы». Закоммитьте отчёт
и код в ветку `task/T69-review-replies`.
