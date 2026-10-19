# T65 — Категории курсов и серверные фильтры каталога

Волна P8, ветка `task/T65-catalog-filters`. Контракт
`Task/00-api-contract.md` (раздел P8 T65) не менялся.

## Что сделано

### Миграция и модель

- `backend/internal/migrate/migrations/009_course_category.sql` —
  идемпотентно, в стиле `007_cover.sql`:
  `ALTER TABLE courses ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT 'other';`.
  Существующие курсы получают `other`; повторный запуск миграций безопасен.
- `repo.Course` получил аддитивное поле `Category string` (`json:"category"`).
  Поле присутствует во всех выборках/вставках/обновлениях и во всех видах
  ответа: каталог, страница курса, teacher-список/деталь, admin-список/деталь,
  очередь модерации, копия курса.
- In-memory `CreateCourse` и Postgres `CreateCourse` нормализуют пустую
  категорию в `other` (та же семантика, что у DEFAULT миграции).

### Категории

- Новый публичный `GET /api/v1/categories` (`handler/categories.go`) отдаёт
  `{items:[{slug,coursesCount}]}`: ровно шесть слагов в фиксированном порядке
  `programming, design, data, marketing, ai, other`; `coursesCount` считает
  только `published`, отсутствующая категория — `0`.
- Валидация слагов — внутренний хелпер хендлеров `validCourseCategory`
  (не экспортируется); список слагов задан один раз.

### Каталог `GET /api/v1/courses`

- Добавлены параметры `category`, `price` (`free|paid`), `minRating`
  (`3.5|4|4.5` строго), `sort` (`popular|new|rating|price_asc|price_desc`,
  по умолчанию `popular`). Фильтры комбинируются с `q/page/limit` через AND;
  `total` считается по отфильтрованному набору до пагинации.
- Тексты ошибок — строго по контракту: `Неизвестная категория`,
  `Недопустимый фильтр цены`, `Недопустимый фильтр рейтинга`,
  `Недопустимая сортировка`.
- Хранилище: добавлены `ListCatalogCourses(CatalogFilter)` и
  `CountPublishedCoursesByCategory()` в `StoreAPI`, in-memory `Store` и
  `PostgresStore`. В Postgres фильтры передаются параметрами, в `ORDER BY`
  подставляется только выбранная из белого списка сортировка.
- Сортировки: `popular` — число `active` записей desc, `new` — `createdAt`
  desc, `rating` — `avgRating` desc, `price_asc/price_desc` — `priceCents`.
  Вторичный ключ — `createdAt desc`. Участвуют только `published`;
  `minRating` исключает курсы с `ratingsCount = 0`.
- `ListPublishedCourses` не изменён — новый метод используется каталогом,
  остальные вызовы остались прежними.

### Teacher API

- `POST /teacher/courses`: необязательное поле `category`; без поля (или
  `null`) — `other`; неизвестное/пустое значение — `400 «Неизвестная категория
  курса»`.
- `PATCH /teacher/courses/{id}`: поле необязательное, без поля категория не
  меняется; неизвестное/пустое значение — тот же `400`.

### Тесты и документация

- `backend/tests/contract/catalog_filters_test.go`:
  - `TestCategoriesList` — порядок шести слагов, счётчики только `published`,
    нули для пустых категорий, черновик не считается;
  - `TestCatalogFiltersAndSorts` — каждый фильтр, комбинации, `q/page/limit`,
    все пять сортировок, вторичный ключ `createdAt`, исключение черновиков,
    `minRating` без отзывов, точные тексты ошибок;
  - `TestTeacherCourseCategory` — create без category/с category, ошибки,
    PATCH без поля/со сменой/с ошибкой, `category` в teacher-списке и детали,
    странице курса, каталоге и admin-списке.
- `backend/tests/postgres/catalog_filters_test.go` — parity SQL-ветки:
  DEFAULT миграции 009, roundtrip категории, `UpdateCourse`/`CloneCourse`/
  `AdminCourseDetail`, фильтры, все сортировки, изоляция черновиков и
  счётчики категорий (дельтами, чтобы не зависеть от чужих данных стенда).
- `admin_console_test.go` и `admin_course_detail_p7_test.go` обновлены под
  аддитивное поле `category` (точные наборы ключей).
- `docs/API.md`: `GET /categories`, новые параметры `GET /courses`, поле
  `category` в create/PATCH teacher и во всех видах курса.

## Проверка

### Полная проверка Go (Docker, golang:1.24-alpine)

```powershell
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Результат: `go vet` без замечаний; все пакеты зелёные:

```
ok  edu-platform/backend/cmd/api          5.463s
ok  edu-platform/backend/internal/auth    0.129s
ok  edu-platform/backend/internal/handler 5.433s
ok  edu-platform/backend/internal/repo    0.042s
ok  edu-platform/backend/internal/payments 0.231s
ok  edu-platform/backend/tests/contract   46.160s
ok  edu-platform/backend/tests/postgres   0.040s (без TEST_DATABASE_URL — SKIP)
```

### Интеграционные тесты Postgres

Временный контейнер на отдельном порту `55434` (стенд и порт 55432 не
затрагивались):

```powershell
docker run -d --rm --name p8-t65-pg -e POSTGRES_USER=edu -e POSTGRES_PASSWORD=edu -e POSTGRES_DB=edu_test -p 55434:5432 postgres:16-alpine
docker run --rm --add-host=host.docker.internal:host-gateway -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src -e GOFLAGS=-mod=mod -e TEST_DATABASE_URL="postgres://edu:edu@host.docker.internal:55434/edu_test?sslmode=disable" golang:1.24-alpine sh -c "go test ./tests/postgres/... -count=1 -v"
```

Результат: `PASS`, 16 тестов пакета (включая новые
`TestPostgresCourseCategory`, `TestPostgresCategoryColumnDefault`,
`TestPostgresCatalogFiltersAndSorts` с 14 подтестами сортировок/фильтров).
После прогона контейнер остановлен и удалён:

```powershell
docker stop p8-t65-pg
```

### Новые контрактные тесты отдельно

```powershell
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go test ./tests/contract/ -run 'TestCategoriesList|TestCatalogFiltersAndSorts|TestTeacherCourseCategory' -count=1 -v"
```

Результат: `PASS` (3 теста, 15 подтестов каталога).

## Отклонения от постановки

1. **Третичный ключ `id DESC`.** Контракт задаёт только вторичный ключ
   `createdAt desc`. При полном совпадении всех ключей (одинаковые цена,
   рейтинг, дата) порядок в SQL без третьего ключа недетерминирован, а
   in-memory и Postgres могли бы расходиться. Добавлен `id DESC` как
   технический tie-breaker; пользовательская семантика сортировок не меняется.
2. **Пустая строка категории при PATCH — `400`,** как и неизвестный слаг.
   На создании отсутствие поля даёт `other`, но явно переданная пустая строка
   трактуется как некорректное значение. Это следует из «неизвестное
   значение → 400» и не позволяет обнулить категорию в обход списка.
3. **`ListPublishedCourses` оставлен без изменений.** Вместо расширения
   старого метода добавлен отдельный `ListCatalogCourses`, чтобы не менять
   поведение существующих вызовов (сортировка по `createdAt`) и не протаскивать
   фильтры туда, где они не нужны.
4. **`gofmt -w` массово не применялся.** В рабочем дереве файлы с CRLF, из-за
   чего `gofmt -l` показывает почти весь репозиторий. Форматирование
   изменённых фрагментов выдержано в стиле окружающего кода; массовая
   перезапись окончаний строк не делалась, чтобы не раздувать diff.
5. **Postgres-тесты задают `created_at` через пул.** Публичного метода для
   этого нет; прямой `UPDATE` нужен только тесту, чтобы детерминированно
   проверить вторичный ключ сортировок. Данные изолированы уникальным
   `token` в заголовке и префиксом прогона.

## Открытые вопросы

Нет. Контракт P8 T65 реализован полностью; смежные эндпоинты и схемы
(`Task/**`, `specs/**`, `frontend/**`, миграции 001–008) не затрагивались.
