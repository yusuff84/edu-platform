# T63 — промокоды курсов (P8)

**Волна:** P8  **Ветка:** `task/T63-promo-codes`  **Worktree:** `wt/t63`
**Контракт:** `Task/00-api-contract.md`, раздел «Эндпоинты волны P8 (T63–T70)» (заморожен)
**Статус:** ✅ готово (код и отчёт закоммичены, ветка не запушена)

## Что сделано

- **Миграция** `backend/internal/migrate/migrations/008_promo_codes.sql`:
  - таблица `promo_codes` (`course_id → courses ON DELETE CASCADE`,
    `UNIQUE (course_id, code)`, `discount_pct 1..90`, `max_uses >= 0`,
    `used_count >= 0`, `max_uses = 0` — без лимита, `expires_at NULL` — без срока);
  - индекс `(course_id, created_at DESC)` под список;
  - аддитивные колонки `payments.promo_code TEXT NOT NULL DEFAULT ''` и
    `payments.discount_cents INT NOT NULL DEFAULT 0` (старые строки читаются
    как обычная покупка без скидки).
- **Модели и Store API** (`models.go`, `store_iface.go`): `PromoCode`, поля
  `Payment.PromoCode`/`Payment.DiscountCents`; методы `CreatePromoCode`,
  `ListPromoCodesByCourse`, `GetPromoCodeByID`, `FindPromoCode`,
  `DeactivatePromoCode`, `IncrementPromoCodeUsage`.
- **In-memory Store** (`store.go`): промокоды в `map[id]*PromoCode` под общим
  `RWMutex`; дубликат `(courseId, code)` → `ErrAlreadyExists`; код
  нормализуется trim+upper; список — `createdAt DESC, id ASC`; удаление курса
  каскадно удаляет его промокоды; деактивация идемпотентна; инкремент
  `usedCount` атомарный.
- **Postgres Store** (`postgres_store.go`): те же операции SQL-запросами,
  тот же порядок списка, нормализация и `ErrAlreadyExists` по unique violation;
  мусорный/не-UUID id в `GetPromoCodeByID`/`Deactivate` трактуется как
  «не найдено»; `paymentCols`/`scanPayment`/`CreatePayment` расширены полями
  промокода, журнал админа получает их без изменения формы.
- **Handler** `backend/internal/handler/promo_codes.go`:
  - `POST /api/v1/teacher/courses/{id}/promo-codes` → `201 {"promoCode": …}`,
    права владельца/admin через `ownCourse`, архив — `409`, дубликат — `400`,
    валидация полей с дословными текстами контракта;
  - `GET /api/v1/teacher/courses/{id}/promo-codes` → `200 {"items": […]}`,
    включая неактивные коды; архивный курс читать можно;
  - `DELETE /api/v1/teacher/courses/{id}/promo-codes/{promoId}` → `204`
    (деактивация, повтор — тоже `204`), чужой/несуществующий — `404`;
  - `POST /api/v1/courses/{id}/promo/validate` (только вошедший) →
    `200 {code,discountPct,priceCents,finalPriceCents}`; скидка округляется
    вниз до копеек.
- **Покупка** (`enroll.go`): `POST /enrollments` принимает необязательный
  `promoCode`; проверка кода идёт после идемпотентности pending-платежа и до
  создания платежа; `amountCents` — цена со скидкой, `discountCents` — разница
  с полной ценой, комиссия и выплата преподавателю считаются от фактической
  суммы. В webhook `usedCount` увеличивается ровно один раз при первом переходе
  платежа в `succeeded` (под существующим `webhookMu`); повторный webhook —
  `duplicate`, возврат средств счётчик не уменьшает.
- **Журнал админа** (`admin_payments.go`): аддитивные `promoCode` и
  `discountCents` в строке платежа.
- **Маршруты** (`middleware.go`): публичная проверка кода с `requireAuth`,
  teacher-маршруты — внутри существующей группы `/teacher` (роли уже навешены).
- **docs/API.md**: краткое описание эндпоинтов T63 и новых полей платежа.

## Проверка

Все команды запускались в Docker `golang:1.24-alpine`, Go-код — только через
контейнер.

### Unit + contract + postgres (без БД)

```bash
docker run --rm -e HOME=/root \
  -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src \
  golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Результат:

```
ok  edu-platform/backend/cmd/api
ok  edu-platform/backend/internal/auth
ok  edu-platform/backend/internal/handler
ok  edu-platform/backend/internal/payments
ok  edu-platform/backend/internal/repo
ok  edu-platform/backend/tests/contract
ok  edu-platform/backend/tests/postgres
```

`go vet ./...` — без замечаний. В пакете `tests/postgres` без
`TEST_DATABASE_URL` тесты ожидаемо пропускаются.

### Postgres parity

```bash
docker run -d --name p8-t63-pg \
  -e POSTGRES_USER=edu -e POSTGRES_PASSWORD=edu -e POSTGRES_DB=edu_test \
  -p 55433:5432 postgres:16-alpine

docker run --rm -e HOME=/root \
  -e "TEST_DATABASE_URL=postgres://edu:edu@host.docker.internal:55433/edu_test?sslmode=disable" \
  -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src \
  golang:1.24-alpine sh -c "go test ./... -count=1"
```

Результат: все пакеты `ok`, включая `tests/postgres` (5.2 с). В verbose-прогоне
`go test ./tests/postgres -count=1 -v` прошли все 14 тестов, в том числе:

- `TestPostgresPromoCodesCRUD` — CRUD, дубликат, порядок списка, поиск,
  идемпотентная деактивация, инкремент, каскад при удалении курса;
- `TestPostgresPaymentPromoFields` — `promo_code`/`discount_cents` в платеже и
  строке журнала, аддитивность для обычного платежа.

Контейнер `p8-t63-pg` остановлен и удалён после прогона; порт 55433, отдельная
БД `edu_test`, общий стенд на 3001/8082 не использовался.

## Отклонения

1. **Архивный курс.** Контракт запрещает только создание промокода в архиве
   (`409`). Список и деактивация в архиве разрешены — иначе нельзя было бы
   управлять историей; закреплено тестом.
2. **`DELETE` идемпотентен:** повторный вызов возвращает `204`, запись остаётся
   в списке с `active=false`. Контракт перечисляет для DELETE только `403/404`.
3. **Порядок проверок `resolvePromo`:** бесплатный курс → наличие кода →
   `active` → срок → лимит. Пустой код обрабатывается отдельно («Укажите
   промокод»); в покупке без `promoCode` проверка не выполняется.
4. **Идемпотентность покупки:** при существующем pending-платеже возвращается
   та же ссылка до проверки промокода — повторный запрос не меняет сумму и не
   расходует лимит.
5. **`usedCount`:** увеличивается только в ветке webhook `succeeded` и только
   при первом переходе (duplicate отсекается выше); возврат средств и
   `failed` счётчик не меняют. Это соответствует формулировке контракта
   «при успешной оплате».
6. **Скидка и комиссия:** `finalPriceCents = price - floor(price·pct/100)`;
   `discountCents = price - finalPrice`; комиссия платформы и выплата
   преподавателю считаются от фактически оплаченной суммы, а не от полной
   цены.
7. **Дубликат кода:** и предварительная проверка в handler, и уникальный
   индекс/проверка в Store дают `400` с текстом
   «Такой промокод уже существует на этом курсе» (в Postgres — через
   `ErrAlreadyExists`).
8. **`expiresAt`:** принимается строго RFC3339; невалидная или прошедшая дата —
   `400` «Срок действия должен быть в будущем» (отдельного текста про формат
   контракт не задаёт).
9. **Аддитивные поля платежа:** `promoCode`/`discountCents` добавлены в JSON
   платежа и журнала; для старых записей — `""` и `0`. Форма существующих
   ответов не менялась.

## Открытые вопросы

Нет. Поведение в спорных местах зафиксировано выше и покрыто контрактными/
Postgres-тестами; при необходимости координатор может уточнить трактовку
архива и момента инкремента `usedCount`.
