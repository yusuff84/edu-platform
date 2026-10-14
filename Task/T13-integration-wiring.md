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

Собери в `NewRouter` всё, что подготовили другие задачи. **Прочитай раздел
«Проверенные координаты вставки» в самом конце этой задачи — там верные
точки вставки, проверенные по исходникам после смержения T07/T09/T10.**

| Источник | Что зарегистрировать | Где |
|---|---|---|
| T07 | `app.RegisterAdminPaymentsRoutes(r)` | **на уровне `/api/v1`**, между строками 95 и 97 (после `POST /enrollments`) |
| T09 | `r.Get("/pay/mock/{id}", payments.MockConfirmHandler())` | **на верхнем уровне роутера**, сразу после `r.Get("/healthz", healthz)` — **не** под `/api/v1` |
| T10 | `GET /auth/yandex/start`, `GET /auth/yandex/callback` | внутри `/api/v1`; лимитер auth уже навешен T10 |
| T13 | `path`-обслуживание `/uploads/*` — если `internal/storage` решено подключить (см. п.4) | по усмотрению |

### ⚠️ Исправленные координаты — не следуй старым комментариям-маркерам

Агент T10 оставил в `middleware.go` два маркера-подсказки. **Оба неверны**,
проверено по исходникам. Не повторяй ошибку:

1. **Маркер в `r.Route("/admin", ...)` предлагал `app.RegisterAdminPaymentsRoutes(r)` внутри блока `/admin`.** Это неверно: метод регистрирует **абсолютные** пути `/admin/payments`, `/admin/settings` (`handler/admin_payments.go:20-27`, строки `Get("/admin/payments", ...)`). Монтирование внутри `r.Route("/admin", ...)` даст `/admin/admin/payments`. Плюс middleware `requireAuth` + `requireRole(admin)` метод вешает **сам** через `.With(...)` — двойная обёртка не нужна.
2. **Маркер под webhook предлагал `app.PaymentCheckoutPage` по пути `/payments/{provider}/checkout`.** Такого хендлера **не существует**. T09 экспортирует `payments.MockConfirmHandler() http.Handler` (`payments/mock.go:246`), а путь — `/pay/mock/{id}`, см. `payments.MockConfirmPath(id)` (`mock.go:50`).

Эти маркеры я исправил в `middleware.go` на верные инструкции.

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

3. Сервис `runner` (`docker-compose.yml:43-47`) — применить YAML из T13, раздел «YAML для T13» **дословно**. Там же — блоки для `api` и верхнеуровневый `networks`.
4. Передай в сервис `api` переменные, которые читает `config.Load()`,
   но которых нет в compose: `ALLOWED_ORIGINS`, плюс таблицу переменных
   из T09 (раздел «Env-переменные, добавленные T09») и переменные
   Yandex OAuth из T10. Незаданные значения передавай как `${VAR:-}` из `.env`.

**Важные оговорки от T11, не отменяй их:**
- **не** добавлять `depends_on: runner` у `api` — api деградирует мягко
  («сервис проверки недоступен»), а не падает при старте;
- `wget` в образе `runner` **нет** — healthcheck только через `python3`;
- `runner_net` должна быть `internal: true` — egress из неё блокируется
  (T11 проверил вживую: `URLError`);
- `runner` **не публикует** порт 8081 наружу — на internal-сети Docker
  и так его не публикует;
- если очень нужен `curl localhost:8081` для отладки — временно перевести
  runner на default-сеть, но это **вернёт ему выход в интернет**.

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

### 6. Бэклог от T06 — недостающие эндпоинты конструктора

T06 честно отказался рисовать кнопки переименования/удаления/перестановки,
потому что API этого не умеет. Реализуй хотя бы первые три — они разблокируют
основные сценарии преподавателя. Остальные — по остаточному принципу.

| Эндпоинт | Тело | Зачем | Приоритет |
|---|---|---|---|
| `GET /teacher/courses/{id}` | — → Course + modules | редактор по id без обхода «список → slug → /courses/{slug}» | высокий |
| `PATCH /teacher/courses/{id}` | `{ title?, description?, priceCents?, slug? }` → Course | правка карточки курса | высокий |
| `PATCH /teacher/courses/{id}/modules/{moduleId}` | `{ title }` → Module | переименование модуля (повторный POST создаёт новый) | высокий |
| `PATCH /teacher/courses/{id}/lessons/{lessonId}` | тело как в POST lessons (по kind) → Lesson | редактирование урока после создания | средний |
| `DELETE /teacher/courses/{id}/modules/{moduleId}` | — → 204 (409 при непустых уроках) | удаление модуля | средний |
| `DELETE /teacher/courses/{id}/lessons/{lessonId}` | — → 204 | удаление урока | средний |
| `POST /teacher/courses/{id}/modules/reorder` | `{ items: [{ id, position }] }` | порядок модулей | низкий |
| `POST /teacher/courses/{id}/lessons/reorder` | `{ items: [{ id, position }] }` | порядок уроков | низкий |
| `DELETE /teacher/courses/{id}` | — → 204 | удаление курса | низкий |
| обогатить `GET /teacher/courses` | + `modulesCount`, `lessonsCount` | счётчики в списках без N+1 | средний |

Каждый новый эндпоинт — **в отдельном файле** с `Register...Routes`,
как сделал T07, чтобы не раздувать `middleware.go` и не создавать конфликтов.

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

---

## Приложение A — YAML от T11 (применять дословно)

```yaml
  runner:
    build: ./runner
    restart: unless-stopped
    networks:
      - runner_net            # только внутренняя сеть — у runner нет выхода в интернет
    # Порт НЕ публикуется: на internal-сети Docker всё равно его не публикует
    # (проверено: NetworkSettings.Ports = {"8081/tcp":[]}). api ходит по http://runner:8081.
    read_only: true                      # корневая ФС только на чтение
    tmpfs:
      - /tmp:size=32m,mode=1777          # сюда пишется код ученика (WORKDIR=/tmp)
    cap_drop:
      - ALL
    security_opt:
      - no-new-privileges:true
    pids_limit: 64                       # защита от форк-бомбы
    deploy:
      resources:
        limits:
          cpus: "0.50"
          memory: 256M
    healthcheck:
      test: ["CMD", "python3", "-c", "import urllib.request;urllib.request.urlopen('http://127.0.0.1:8081/health',timeout=3)"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 5s
```

Два дополнения (compose не даёт сервису собственную «внутреннюю» сеть
без верхнеуровневого объявления):

```yaml
  api:
    networks:
      - default
      - runner_net
    # RUNNER_URL: http://runner:8081 уже есть в сервисе.
    # healthcheck (образ alpine → busybox wget):
    healthcheck:
      test: ["CMD", "wget", "-q", "-O", "/dev/null", "http://127.0.0.1:8080/healthz"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 10s
```

```yaml
# в самый низ файла, рядом с volumes:
networks:
  runner_net:
    internal: true                       # контейнеры общаются внутри, интернета нет
```

## Приложение B — бюджет времени runner и checker.go

T11 выставил общий бюджет выполнения `requestBudget = 4 с`, потому что
`backend/internal/service/checker.go:71` держит `http.Client{Timeout: 5*time.Second}`.
Если T13/T14 поднимут таймаут в `checker.go` — подними и `requestBudget`
в `runner/main.go` (константа помечена комментарием).
