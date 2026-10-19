# T63 — Бэкенд: промокоды (создание, проверка, покупка)

**Волна:** P8  **Ветка:** `task/T63-promo-codes`  **Worktree:** `wt/t63`

## Суть

У учителя нет возможности давать скидки: цена курса фиксирована. Волна P8
вводит промокоды: учитель создаёт код со скидкой в процентах на свой курс,
студент проверяет код на странице курса и покупает со скидкой; платёж хранит
код и размер скидки. Раздел «Эндпоинты волны P8 (T63–T70)» в
`Task/00-api-contract.md` — источник правды, **заморожен**.

Работать только в своём worktree `wt/t63` (ветка `task/T63-promo-codes`).

## Что сделать

1. Миграция `backend/internal/migrate/migrations/008_promo_codes.sql` — таблица
   `promo_codes` по контракту; идемпотентно, в стиле `007_cover.sql`.
2. `repo/models.go`: структура `PromoCode` с JSON-тегами по контракту
   (`expiresAt` — `*time.Time` → `null`; `maxUses=0` — без лимита).
   В модель платежа добавьте аддитивные `PromoCode string` и `DiscountCents int`
   (JSON-теги `promoCode`, `discountCents`).
3. StoreAPI (`store_iface.go`, блок методов добавляйте отдельным блоком в конец
   интерфейса): создание, список по курсу, получение по id, поиск по
   `(courseId, code)`, деактивация, атомарный инкремент `usedCount`. Реализуйте
   в `store.go` (in-memory) и `postgres_store.go`; соблюдайте parity.
4. Хендлеры (новый файл рядом с `teacher_edit.go`, регистрация маршрутов —
   рядом с другими teacher-маршрутами):
   - `POST /teacher/courses/{id}/promo-codes` — `201 {"promoCode":…}`,
     нормализация кода к верхнему регистру, все проверки и тексты ошибок — по
     контракту, права владельца/admin, архивный курс → `409`;
   - `GET /teacher/courses/{id}/promo-codes` — `200 {"items":[…]}`, новые первыми;
   - `DELETE /teacher/courses/{id}/promo-codes/{promoId}` — деактивация
     (`active=false`), `204`, повтор — тоже `204`.
5. `POST /courses/{id}/promo/validate` (только вошедший): расчёт
   `finalPriceCents = priceCents − ⌊priceCents·discountPct/100⌋`; проверки
   активности/срока/лимита и тексты — по контракту.
6. Покупка `POST /enrollments`: необязательное `promoCode`. Проверка кода — до
   создания платежа, те же тексты; `amountCents` — со скидкой; в платёж пишутся
   `promoCode`/`discountCents`; сплит комиссии — от фактической суммы (как
   сейчас). Бесплатный курс + код → `400`.
7. `usedCount` увеличивается ровно один раз — в момент первого перехода платежа
   в `succeeded` (найдите существующую точку: вебхук/подтверждение mock).
   Идемпотентность сохраните (повторный вебхук не должен увеличивать счётчик).
8. Журнал админа (`admin_payments.go`): аддитивные `promoCode` (пустая строка,
   если не было) и `discountCents`.
9. Тесты: `backend/tests/contract/` — валидация всех полей, права (403),
   дубликат кода (400), архив (409), validate (скидка/ошибки), покупка со
   скидкой (сумма, платёж, комиссия), идемпотентность `usedCount`, журнал.
   Добавьте parity-тесты для новых методов стора в `backend/tests/postgres/`.
10. Короткая приписка в `docs/API.md` по новым эндпоинтам (без переписывания
    чужих разделов).

Существующие тесты должны остаться зелёными. Правки существующих тестов
допустимы только при изменении контракта — обоснуйте в отчёте.

## Разрешённые файлы

- `backend/internal/migrate/migrations/008_promo_codes.sql` (новый)
- `backend/internal/repo/**`
- `backend/internal/handler/**` (новые файлы + точечные правки существующих)
- `backend/tests/**`
- `docs/API.md`
- `backend/tests/T63-promo-codes.md` (отчёт)

## Запрещено

- `Task/**`, `specs/**`, `frontend/**`, `ru.json`, миграции 001–007,
  `docker-compose*`, `qa-*.ps1`, `.github/**`, `deploy/**`.
- Менять формы существующих ответов (только аддитивные поля).
- `git push`, `git merge`, смена ветки, работа вне `wt/t63`.

## Проверка (только через Docker)

Из корня репозитория:

```
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Postgres-parity: поднимите `postgres:16-alpine` на порту 55432 (БД `edu_test`,
`edu/edu`) и прогоните тесты с `TEST_DATABASE_URL` (из контейнера тестов хост —
`host.docker.internal`). Вывод команд приложите к отчёту.

## Отчёт

`backend/tests/T63-promo-codes.md`: «Что сделано», «Проверка» (команды и
результаты), «Отклонения от постановки», «Открытые вопросы». Закоммитьте отчёт
и код в ветку `task/T63-promo-codes`.
