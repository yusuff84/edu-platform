# T13 — Сквозная проводка: маршруты, compose, долги репозитория

**Ключ сессии:** `T13-integration-wiring`
**Волна:** 3 (после T07, T09, T10, T11) · **Слой:** сквозной
**Приоритет:** P1 — без него половина сделанного не подключена

## Зачем эта задача

T07, T09, T10, T11 намеренно **не регистрировали** свои маршруты и
не меняли `docker-compose.yml`, чтобы не конфликтовать при параллельной работе.
Эта задача — единственная, которая имеет право трогать эти файлы.

## Владение файлами (жёсткая граница)

```
backend/internal/handler/middleware.go     — регистрация всех новых маршрутов
backend/internal/handler/admin.go          — если требуется
backend/internal/handler/teacher.go         — если требуется
backend/internal/handler/main.go*           — если появится
backend/internal/repo/postgres.go           — убрать устаревший TODO
docker-compose.yml                          — sandbox runner, лимиты, env
```

## Что сделать

### 1. Маршруты (`middleware.go`)

Собери в `NewRouter` всё, что подготовили другие задачи. Ожидаемые кандидаты:

| Источник | Что зарегистрировать |
|---|---|
| T07 | `a.RegisterAdminPaymentsRoutes(...)` → `GET /admin/payments`, `GET/PUT /admin/settings` под `requireRole(admin)` |
| T09 | **`r.Get("/pay/mock/{id}", payments.MockConfirmHandler())`** — см. ниже |
| T10 | `GET /auth/yandex/start`, `GET /auth/yandex/callback` (если описаны в отчёте T10); вешай `rateLimit` на `/auth/login`, `/auth/register` |
| T13 | `path`-обслуживание `/uploads/*` — если `internal/storage` решено подключить (см. п.4) |

### Требование T09 — mock-страница оплаты (точные координаты)

В `backend/internal/handler/middleware.go`, функция `NewRouter`, **после строки 44**
(`r.Get("/healthz", healthz)`), на верхнем уровне роутера — **не под `/api/v1`**,
потому что `redirectUrl = "/pay/mock/{id}"`:

```go
r.Get("/pay/mock/{id}", payments.MockConfirmHandler())
```
плюс импорт `"edu-platform/backend/internal/payments"`.

Без этого маршрута **мок-покупка не работает вообще** — `redirectUrl`,
который возвращает провайдер, ведёт в никуда. Это блокирует
сквозной сценарий покупки из `qa-abc.ps1` (T12).

Обязательно проверь, что:
- `mock.go` из T09 экспортирует именно `MockConfirmHandler()`;
- страница шлёт `POST /api/v1/payments/webhook/mock` (маршрут уже есть в `NewRouter`);
- `payments.MockConfirmPath(id)` — то, что возвращает `CreatePayment` как `redirectUrl`.

### Env-переменные, добавленные T09 — передать в сервис `api`

| Переменная | Обязательна | Назначение |
|---|---|---|
| `STRIPE_SECRET_KEY` | для stripe | Bearer-авторизация |
| `STRIPE_WEBHOOK_SECRET` | для stripe | HMAC-секрет `Stripe-Signature` |
| `YOOKASSA_SHOP_ID` | для yookassa | Basic-авторизация |
| `YOOKASSA_SECRET_KEY` | для yookassa | Basic-авторизация |
| `YOOKASSA_TRUSTED_IPS` | нет | CIDR/IP доверенных источников вебхуков |
| `MOCK_WEBHOOK_SECRET` | нет | если задан — mock-вебхук требует `X-Mock-Signature` |
| `PAYMENTS_RETURN_URL` | нет | URL возврата после оплаты |

Передавай через `${VAR:-}` из `.env`, чтобы не падать без ключей.

Обязательно проверь и исправь:
- `POST /api/v1/payments/webhook/{provider}` — должен идти **после** rate-limit,
  но **без** `requireAuth` (это внешний вызов провайдера);
- все `/admin/*` — только под `requireRole(admin)`;
- `/teacher/*` — под `requireAuth` + `requireRole(teacher, admin)`.

### 2. Docker Compose

3. Сервис `runner` (`docker-compose.yml:43-47`) — применить требования T11:
   - `network_mode: none` **или** внутренняя сеть без выхода наружу;
     выбери `none` и проверь, что runner всё ещё доступен от `api`
     (если `none` — API не сможет до него обращаться; тогда нужен
     `internal: true` bridge-сеть. **Проверь фактически и опиши решение**);
   - `mem_limit`, `cpus`, `pids_limit`;
   - `read_only: true` + `tmpfs` для `/tmp`;
   - `cap_drop: [ALL]`, `security_opt: [no-new-privileges:true]`;
   - `restart: unless-stopped`.
4. Передай в сервис `api` переменные, которые читает `config.Load()`,
   но которых нет в compose: `ALLOWED_ORIGINS`, `PLATFORM_*`-эквиваленты
   для новых переменных T09 (`STRIPE_*`, `YOOKASSA_*`) и T10 (`YANDEX_*`,
   `SESSION_SECRET` уже есть — проверь), `CSRF_*` если такие появятся.
   Незаданные значения передавай как `${VAR:-}` из `.env`.
5. Добавь `healthcheck` для `runner` (`GET /health`) и для `api`
   (`GET /api/v1/healthz`), с `depends_on: condition: service_healthy`.

### 3. Устаревшие долги

6. `backend/internal/repo/postgres.go:12` — удали устаревший
   `TODO (инфра/БД): заменить in-memory Store на SQL-репозитории...`
   и `postgres.go:13` про golang-migrate: SQL-репозиторий **уже реализован**
   в `postgres_store.go`, миграции — на собственном раннере.
   Замени на актуальный комментарий.
7. Пройдись по `grep -rn "TODO\|FIXME\|заглушк\|не настроен в MVP" backend runner`
   и для каждого вхождения реши: закрыто другими задачами → удалить;
   актуально → оставить с уточнением.
8. `backend/internal/handler/admin.go` и `teacher.go` — если T07/T09/T10
   оставили там что-то несогласованное, приведи к общему стилю.

### 4. Мёртвый код — реши и обоснуй

9. `backend/internal/storage/store.go` — пакет не импортируется нигде.
   Варианты: (а) подключить `LocalStore` + отдавать `/uploads/*`
   статикой и добавить загрузку обложек; (б) удалить пакет.
   **Выбери одно и обоснуй.** Учти: у задач нет времени на полноценный
   upload-пайплайн — если выберешь (а), ограничься обслуживанием
   уже существующих файлов из каталога `uploads/`.
10. `StoreAPI.SetUserRole` и `CountCompletedLessons` реализованы,
    но не вызываются ни одним хендлером. Либо найди им применение,
    либо отметь в отчёте как API для будущего.
11. `payments.PaymentProvider.Refund` / `GetStatus` — после T09 проверь,
    достижимы ли они. Если нет и не нужны — оставь в интерфейсе,
    но задокументируй.

### 5. Проверка целостности

12. Убедись, что `specs/001-it-courses-mvp/tasks.md` **не редактировался**
    (это артефакт процесса). Вместо правки — в отчёте перечисли,
    какие задачи из `tasks.md` фактически были закрыты этой волной.
13. Прогони `go build`, `go vet`, `go test` для `backend` и `runner`.

## Критерии приёмки

- [ ] `cd backend && go build ./... && go vet ./... && go test ./...` — чисто
- [ ] `cd runner && go build ./... && go vet ./... && go test ./...` — чисто
- [ ] `docker compose config` валиден, все сервисы стартуют
- [ ] `GET /api/v1/admin/payments` отдаёт `401` без админской cookie
      и `200` с ней (проверь вручную или опиши в отчёте, как проверено)
- [ ] `GET /api/v1/healthz` возвращает `ok`
- [ ] Rate-limit на `/auth/login` реально срабатывает
- [ ] Устаревший TODO в `postgres.go` удалён
- [ ] По каждому решению из раздела «Мёртвый код» есть обоснование в отчёте

## Проверка

```bash
cd backend && go build ./... && go vet ./... && go test ./...
cd ../runner && go build ./... && go vet ./... && go test ./...
cd .. && docker compose config && docker compose build
```

## Отчёт

1. Итоговая таблица **всех** маршрутов: метод, путь, middleware, обработчик.
2. Итоговый YAML-блок сервиса `runner` и список env-переменных,
   добавленных в сервис `api`.
3. Решения по п.9–11 с обоснованием.
4. Список задач из `specs/001-it-courses-mvp/tasks.md`, фактически закрытых
   этой волной (для будущей правки `tasks.md`).
