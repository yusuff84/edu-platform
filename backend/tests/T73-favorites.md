# T73 — Бэкенд: избранные курсы (отчёт)

**Ветка:** `task/T73-favorites`  **Worktree:** `wt/t73`
**Контракт:** `Task/00-api-contract.md`, раздел «T73 — Избранное» (заморожен).

## Что сделано

1. **Миграция** `backend/internal/migrate/migrations/013_favorites.sql`:
   таблица `favorites (user_id, course_id, created_at)`, PK `(user_id, course_id)`,
   FK на `users`/`courses` с `ON DELETE CASCADE`, индекс
   `(user_id, created_at DESC)`. Идемпотентна (`CREATE TABLE/INDEX IF NOT EXISTS`) —
   как соседние миграции, раннер выполняет файл целиком при каждом старте.
2. **StoreAPI** (`repo/store_iface.go`, блок методов в конце интерфейса):
   `AddFavorite`, `RemoveFavorite`, `ListFavoriteCoursesByUser`.
   - in-memory (`repo/favorites.go`): `Map[userID]map[courseID]time`; повторное
     добавление не дублирует запись и не сдвигает время; список — только
     `published`, новые первыми, при равном времени — по id по убыванию
     (детерминированный порядок); никогда не `nil`. Удаление пользователя и
     курса вычищает избранное (вызовы из `DeleteUser`/`DeleteCourse` — зеркало
     `ON DELETE CASCADE`).
   - Postgres (`repo/postgres_favorites.go`): `INSERT ... ON CONFLICT DO NOTHING`,
     `DELETE`, `JOIN courses` с фильтром `status='published'` и
     `ORDER BY created_at DESC, c.id DESC`. FK-нарушение и мусорный UUID
     (22P02) в `AddFavorite` — `ErrNotFound`; в `RemoveFavorite` мусорный id —
     успешный no-op (идемпотентность).
3. **Хендлеры** `backend/internal/handler/favorites.go`:
   - `PUT /api/v1/me/favorites/{courseId}` — только `published`, иначе
     `404 «Курс не найден»`; идемпотентно `204`;
   - `DELETE /api/v1/me/favorites/{courseId}` — идемпотентно `204`, в т.ч.
     если курса/пары уже нет;
   - `GET /api/v1/me/favorites` — `200 {"items":[CourseItem],"total":N}`,
     элементы через существующий `toCourseItem`, `items` всегда массив.
   Маршруты регистрируются `RegisterFavoritesRoutes` внутри общей группы
   `/api/v1` через `r.With(app.requireAuth)` (вызов в `middleware.go` рядом
   с остальными `/me`-маршрутами). Признак `isFavorite` в публичные виды
   курса не добавлен.
4. **Тесты:**
   - `backend/tests/contract/favorites_test.go`: 401 без входа для всех трёх
     маршрутов; добавление/повторное добавление; удаление/повторное удаление и
     удаление несуществующей пары; порядок «новые первыми»; черновик/архив/
     модерация → 404 «Курс не найден», неудачный PUT не сохраняется; архивный
     курс исчезает из списка; изоляция избранного разных учеников; форма
     пустого конверта (`items: []`, `total: 0`, без `isFavorite`).
   - `backend/tests/postgres/favorites_test.go`: parity стора на реальном
     Postgres (идемпотентность, порядок, фильтр published, изоляция, ошибки
     `ErrNotFound`), каскад по таблице `favorites` при удалении курса и
     пользователя.
5. **Документация** `docs/API.md`: три строки в таблице раздела
   «Прогресс обучения (`me`)».

## Проверка

Окружение — Docker, как в постановке.

```
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod \
  -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Результат: `go vet` без замечаний, все пакеты `ok`:
`cmd/api`, `internal/auth`, `internal/handler`, `internal/payments`,
`internal/repo`, `tests/contract` (52.1 s), `tests/postgres` (без
`TEST_DATABASE_URL` — skip).

Postgres parity: контейнер `p9-t73-pg` (`postgres:16-alpine`, порт `55443`,
`edu/edu`, БД `edu_test`), тесты запускались из `golang:1.24-alpine` с
`TEST_DATABASE_URL=postgres://edu:edu@host.docker.internal:55443/edu_test?sslmode=disable`:

```
go test ./tests/postgres -count=1 -v
```

Результат: весь пакет `ok` (7.9 s), включая `TestFavoritesParity` и
`TestFavoritesCascadeParity`; существующие parity-тесты зелёные. Контейнер
после прогона остановлен и удалён.

## Отклонения

- На уровне стора `AddFavorite` принимает любой существующий курс, включая
  черновик: проверку `published` и `404` по контракту выполняет хендлер
  `PUT`, а `ListFavoriteCoursesByUser` фильтрует список по `published`.
  Это соответствует формулировке постановки («только published-курсы
  попадают в список»); в parity-тесте черновик добавлен напрямую в стор и
  проверено, что в список он не попадает.
- Для стабильного порядка при совпадающем `created_at` добавлен вторичный
  ключ `id DESC` (в in-memory и Postgres). Контракт задаёт только «новые
  первыми», вторичный ключ лишь убирает недетерминированность.
- Существующие тесты и их ожидания не менялись.

## Открытые вопросы

Нет. Миграция `013` аддитивна и не зависит от `012`/`014`/`015` других задач
волны; раннер сортирует файлы по имени, поэтому при интеграции порядок
применения сохранится.
