# T65 — Бэкенд: категории курсов и серверные фильтры каталога

**Волна:** P8  **Ветка:** `task/T65-catalog-filters`  **Worktree:** `wt/t65`

## Суть

Каталог умеет только поиск по `q`; «пилюли» направлений на фронте — это
захардкоженные поисковые запросы, сортировка — клиентская в пределах страницы.
Волна P8 вводит категорию курса, публичный список категорий и серверные
фильтры/сортировку. Раздел «Эндпоинты волны P8 (T63–T70)» в
`Task/00-api-contract.md` — источник правды, **заморожен**.

Работать только в своём worktree `wt/t65` (ветка `task/T65-catalog-filters`).

## Что сделать

1. Миграция `backend/internal/migrate/migrations/009_course_category.sql`:
   `ALTER TABLE courses ADD COLUMN category TEXT NOT NULL DEFAULT 'other';`
   (идемпотентно, в стиле `007_cover.sql`). Существующие курсы получают `other`.
2. `repo/models.go`: поле `Category string` (`json:"category"`) в `Course`;
   убедитесь, что оно попадает во все выборки/вставки/обновления Postgres и
   in-memory стора, а также во все виды ответов (каталог, страница курса,
   teacher-списки, admin-списки) — аддитивно.
3. Фиксированный список слагов: `programming, design, data, marketing, ai,
   other`. Хелпер валидации в хендлерах (не экспортируйте лишнего).
   `POST /teacher/courses` и `PATCH /teacher/courses/{id}` принимают
   необязательное `category`; неизвестное значение → `400 «Неизвестная
   категория курса»`; создание без поля — `other`; PATCH без поля не меняет.
4. `GET /categories` (публичный, без авторизации): все шесть слагов в
   фиксированном порядке, `coursesCount` — число `published` курсов (0
   допустим). SQL — `GROUP BY category`; in-memory — подсчёт.
5. `GET /courses`: новые необязательные параметры `category`, `price`
   (`free|paid`), `minRating` (`3.5|4|4.5`, строго эти значения), `sort`
   (`popular|new|rating|price_asc|price_desc`, по умолчанию `popular`).
   Ошибки и тексты — по контракту. Семантика сортировок — по контракту,
   вторичный ключ — `createdAt` по убыванию, выдача только `published`.
   Фильтры комбинируются с `q`, `page`, `limit`; `total` — по фильтрам.
   `minRating` исключает курсы с `ratingsCount = 0`.
6. Не ломайте существующее поведение: без новых параметров форма ответа и
   пагинация прежние (порядок по умолчанию теперь `popular` — это осознанно).
7. Тесты: `backend/tests/contract/` — каждый фильтр отдельно и комбинации,
   все сортировки (готовьте данные с разными ценами/рейтингами/датами/
   записями), валидация (400 на неизвестные значения), `category` в ответах,
   teacher create/patch, `GET /categories` (порядок и счётчики, включая 0).
   Добавьте parity-тесты в `backend/tests/postgres/`.
8. Короткая приписка в `docs/API.md` (новые параметры и эндпоинт).

Существующие тесты должны остаться зелёными. Правки существующих тестов
допустимы только при изменении контракта — обоснуйте в отчёте.

## Разрешённые файлы

- `backend/internal/migrate/migrations/009_course_category.sql` (новый)
- `backend/internal/repo/**`
- `backend/internal/handler/**`
- `backend/tests/**`
- `docs/API.md`
- `backend/tests/T65-catalog-filters.md` (отчёт)

## Запрещено

- `Task/**`, `specs/**`, `frontend/**`, `ru.json`, миграции 001–008,
  `docker-compose*`, `qa-*.ps1`, `.github/**`, `deploy/**`.
- `git push`, `git merge`, смена ветки, работа вне `wt/t65`.

## Проверка (только через Docker)

Из корня репозитория:

```
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Postgres-parity: `postgres:16-alpine` на порту 55432 (БД `edu_test`, `edu/edu`),
прогон с `TEST_DATABASE_URL` (хост из контейнера тестов — `host.docker.internal`).
Вывод приложите к отчёту.

## Отчёт

`backend/tests/T65-catalog-filters.md`: «Что сделано», «Проверка» (команды и
результаты), «Отклонения от постановки», «Открытые вопросы». Закоммитьте отчёт
и код в ветку `task/T65-catalog-filters`.
