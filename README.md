# Edu Platform — маркетплейс IT-курсов

Платформа онлайн-курсов для начинающих: студенты покупают курсы, приглашённые преподаватели создают их, админ модерирует публикацию.

## Возможности MVP

- **Каталог курсов** — фильтры, направления, поиск, страница курса с программой и отзывами.
- **Конструктор курсов** — модули, уроки, автопроверка кода и квизы, живые эфиры (Zoom/Meet как внешние ссылки).
- **Модерация** — черновик → заявка → проверка админом → публикация.
- **Оплата** — интерфейс `PaymentProvider` (mock / Stripe / YooKassa), комиссия платформы настраивается, идемпотентный вебхук.
- **Обучение** — прогресс по урокам, кодовые задачи (Python, JS, HTML/CSS) и тесты, runner с таймаутом 5с.
- **Роли** — студент, преподаватель (только по приглашению), админ. Email+пароль, сессии в cookie (подписанные HMAC), rate-limit, жёсткий CSRF.
- **Админ-US6** — журнал платежей с разбивкой комиссия/преподаватель, настройка комиссии через API.

## Стек

| Сервис   | Технологии                          | Порт (host) |
|----------|-------------------------------------|-------------|
| backend  | Go, chi, pgxpool, миграции SQL      | 8082 (внутри контейнера 8080) |
| frontend | React + Vite + TypeScript, nginx    | 3001 (в dev-`.env`; шаблон — `3000`) |
| runner   | Go (python/js/htmlcss sandbox)      | — (внутренняя сеть `runner_net`, 8081; наружу не публикуется) |
| db       | Postgres 16                         | — (наружу не публикуется) |

Бэкенд подключается к Postgres, а при недоступности `DATABASE_URL` работает на in-memory хранилище.

## Быстрый старт

В репозитории уже лежит dev-`.env` (отслеживается git): `ADMIN_EMAIL=admin@local`,
`ADMIN_PASSWORD=admin1234`, `WEB_PORT=3001`. Для локального запуска копировать
шаблон не нужно:

```bash
docker compose up -d --build
# http://localhost:3001 — веб, http://localhost:8082 — API
# http://localhost:8082/healthz → {"status":"ok"}
# admin@local / admin1234 — из .env
# teacher@local / teacher1 создаётся автоматически — это dev-учётка,
# см. «Демо-учётки → Преподаватель».
```

Миграции применяются автоматически при старте API.

Шаблон для своего окружения — [.env.example](.env.example): там `WEB_PORT=3000`
и пустые `ADMIN_EMAIL`/`ADMIN_PASSWORD`. Скопируйте его в отдельный env-файл,
заполните секреты и запускайте `docker compose --env-file .env.local up -d`
(имя файла — любое, в git его не добавлять).

### Смена порта web

`WEB_PORT` в `.env` задаёт host-порт web-контейнера; в текущем dev-`.env` — `3001`.
**CSRF и CORS привязаны к origin'у** — просто перенаправить порт недостаточно,
браузер получит отказ на любых не-GET запросах (вход, регистрация). Меняйте
`WEB_PORT` и `ALLOWED_ORIGINS` одной парой. Пример — переезд на `3002`:

```bash
# PowerShell
$env:WEB_PORT="3002"
# Важно: список — это allowlist. Оставьте в нём :3000/:3001, иначе сквозные
# скрипты получат 403 на не-GET: qa-abc.ps1 всегда шлёт Origin :3000,
# qa-p10.ps1 по умолчанию — :3001.
$env:ALLOWED_ORIGINS="http://localhost:3000,http://127.0.0.1:3000,http://localhost:3001,http://127.0.0.1:3001,http://localhost:3002,http://127.0.0.1:3002"
docker compose up -d --build --force-recreate api web

# bash
WEB_PORT=3002 ALLOWED_ORIGINS="http://localhost:3000,http://127.0.0.1:3000,http://localhost:3001,http://127.0.0.1:3001,http://localhost:3002,http://127.0.0.1:3002" \
  docker compose up -d --build --force-recreate api web
```

`ALLOWED_ORIGINS` по умолчанию (когда переменная не задана) —
`http://localhost:3000,http://127.0.0.1:3000` (см. `docker-compose.yml`,
`internal/config/config.go`). Если фронтенд отдаётся на другом порту, а
`ALLOWED_ORIGINS` не поправлен, UI отвечает `403 «Недопустимый источник
запроса»` на любой не-GET — вход, сохранение, архивацию курса. Сквозные скрипты
принимают адрес фронтенда отдельно: `powershell -File qa-abc.ps1 -WebUrl
http://localhost:3001` (на заголовок `Origin` это не влияет — см. «Тесты и QA»).

## Демо-учётки

### Админ — работает сразу

`.env` уже содержит dev-заглушки `ADMIN_EMAIL=admin@local` / `ADMIN_PASSWORD=admin1234`,
поэтому после `docker compose up` админ входит **без ручных действий**:

```bash
docker compose up -d --build db api web runner
# http://localhost:3001  →  admin@local / admin1234
```

В чём была проблема и как она закрыта (T16): в
`backend/internal/migrate/migrations/002_seed.sql` лежат **невалидные
bcrypt-заглушки** — `$2a$10$AAA…Aadmin1234`, то есть 50 символов-заглушка плюс
открытый текст пароля. Настоящий bcrypt — 60 символов псевдослучайных.
Первоначальный `seedAdmin` видел, что пользователь уже создан миграцией, и
выходил, не исправляя хеш — войти было нельзя никогда.

Сейчас `seedAdmin` **перезаписывает битый хеш** настоящим bcrypt при явно
заданных `ADMIN_EMAIL` + `ADMIN_PASSWORD`. Миграции не правились — она уже
применена и является артефактом SDD-процесса.

Для прода задавайте свои значения через свой env-file:
```bash
docker compose --env-file .env.prod up -d
```

### Преподаватель — создаётся автоматически (dev)

`seedAdmin` в `backend/cmd/api/main.go` при **каждом** старте API вызывает
`ensureAccount` для преподавателя. Если `TEACHER_EMAIL`/`TEACHER_PASSWORD`
пусты, используются `teacher@local` / `teacher1`. Если пользователь уже есть,
несовпадающий bcrypt-хеш перезаписывается хешем заданного пароля, а роль
принудительно ставится `teacher`. Поэтому `teacher@local / teacher1` работает
и на Postgres-сиде, хотя в `002_seed.sql` лежит поддельный хеш.

Это **известные dev-учётки, а не безопасный прод-дефолт**:

- сид преподавателя нельзя выключить переменной — он выполняется всегда;
- при расхождении хеша пароль сбрасывается к `TEACHER_PASSWORD` при старте;
- `docker-compose.yml` сейчас **не передаёт** `TEACHER_*` в `api`, поэтому в
  контейнере действуют дефолты; свои значения требуют правки compose
  (отдельная задача) или прямого запуска API с env;
- не задавайте `TEACHER_EMAIL` равным админу: роль существующей учётки будет
  понижена до `teacher`.

Для реального преподавателя используйте приглашение: админ → `/admin/invites`
→ создайте приглашение. Если настроен SMTP, письмо со ссылкой уйдёт
автоматически; иначе скопируйте `inviteLink` из UI/API и откройте
`/register?inviteToken=...` вручную. Регистрация по приглашению даёт роль
преподавателя (T17 починил чтение токена из query).

В in-memory режиме (без `DATABASE_URL`) дополнительно есть
`demo-teacher@local / teach1234` — только для локальной разработки.

### Вход через Яндекс

Маршруты `GET /api/v1/auth/yandex/start` и `.../callback` зарегистрированы
(см. `backend/internal/handler/middleware.go`, `auth_yandex.go`):

- без `YANDEX_CLIENT_ID` `start` отвечает `503 «Вход через Яндекс не настроен
  на сервере»`; при заданном `CLIENT_ID` — `302` на `oauth.yandex.ru` и
  httpOnly cookie `oauth_state` (10 минут);
- `callback` сверяет `state` (иначе `400`), меняет `code` на профиль и ставит
  `session_id`; ошибка провайдера или незаданные `YANDEX_CLIENT_SECRET` —
  `502`; заблокированный пользователь — `403`; при успехе — `302` на `/`;
- новый пользователь создаётся с ролью `student`; если Яндекс не вернул email,
  используется синтетический `yandex-<id>@oauth.invalid`.

Для рабочего входа нужны все три переменные `YANDEX_*` и Redirect URI,
совпадающий с настройками приложения. Живой провайдер в тестах не
использовался — проверены только `503` без ключей и юнит-сценарии.

## Тесты и QA

Go-тесты запускаются в Docker: на хосте Go может быть не установлен, а версия
должна совпадать с `go.mod` (1.24). Из корня репозитория (PowerShell):

```powershell
# Бэкенд: сборка, vet и все тесты (включая contract и handler)
docker run --rm -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src -e GOFLAGS=-mod=mod golang:1.24-alpine sh -c "go build ./... && go vet ./... && go test ./... -count=1"

# Runner: те же проверки в bookworm-образе
docker run --rm -v "${PWD}\runner:/src" -v gomodcache:/go/pkg/mod -w /src -e GOFLAGS=-mod=mod golang:1.24-bookworm sh -c "go build ./... && go vet ./... && go test ./... -count=1"

# Frontend: установка зависимостей, сборка (tsc --noEmit + vite build) и контрактные тесты
cd frontend
npm ci
npm run build
npm test        # vite build SSR-клиента + node --test tests/api-contract/*.test.mjs
```

В bash те же Go-команды: замените `${PWD}` на `"$PWD"`.

### Проверка runner (python3/nodejs)

Тесты runner в контейнере пропускают (`SKIP`) реальный запуск python/js,
если интерпретаторов нет в окружении, а слой сборки `runner/Dockerfile` может
быть взят из кэша — поэтому песочницу целиком проверяем так (из корня репозитория):

```powershell
docker run --rm -v "${PWD}\runner:/src" -w /src golang:1.24-bookworm sh -c "apt-get update -qq && apt-get install -y -qq --no-install-recommends python3 nodejs >/dev/null && go vet ./... && go test ./... -count=1 -v"
```

Ожидаемый итог — `ok runner` без единого `SKIP`/`FAIL`. Если репозиторий Debian
недоступен из вашей сети, замените `deb.debian.org` на рабочее зеркало в
apt-источниках контейнера — добавьте в начало `sh -c` команду
`sed -i 's|deb.debian.org|mirror.yandex.ru|g' /etc/apt/sources.list.d/debian.sources &&`.

Сквозные QA-скрипты (код возврата 0/1; каждый создаёт свои сущности с меткой
времени, поэтому повторный прогон на той же базе не ломается):

| Скрипт | Волна | Проверок | Что покрывает |
|---|---|---|---|
| `qa-abc.ps1` | 1–C | 49 | базовый сценарий: auth, каталог, покупка, конструктор, US6 |
| `qa-p3.ps1` | P3 | 80 | конструктор, ученики курса, кабинет ученика, супер-админ |
| `qa-p4.ps1` | P4 | 37 | журнал платежей, guard архива, blocked |
| `qa-p5.ps1` | P5 | 19 | slug-транслитерация, CORS-preflight |
| `qa-p6.ps1` | P6 | 43 | приглашения/возврат, фильтры, записи на курсы |
| `qa-p7.ps1` | P7 | 59 | unpublish/валидация квиза, обложки, детали курса |
| `qa-p8.ps1` | P8 | 66 | промокоды, фильтры каталога, сертификаты, ответы на отзывы |
| `qa-p9.ps1` | P9 | 93 | Q&A, избранное, превью, аналитика, похожие, профиль |
| `qa-p10.ps1` | P10 | 69 | глобальные отзывы/Q&A/промокоды, супер-админ, сессии |

```bash
# Требует запущенного стенда. web (фронтенд) не обязателен для API-скриптов:
docker compose up -d db api runner

powershell -File qa-abc.ps1 -AdminEmail admin@local -AdminPassword admin1234
pwsh -File qa-abc.ps1 -AdminEmail admin@local -AdminPassword admin1234  # PS 7

# Скрипт идемпотентен: каждый запуск регистрирует qa-<timestamp>@example.test,
# второй прогон на «грязной» БД тоже даёт 49/49.
# Значения -AdminEmail/-AdminPassword — из ADMIN_EMAIL/ADMIN_PASSWORD в .env
# (там уже подставлены dev-заглушки, см. «Демо-учётки»).
powershell -File qa-abc.ps1 -SkipAdmin         # без админ-блока (US6)
powershell -File qa-abc.ps1 -BaseUrl http://127.0.0.1:8082 -WebUrl http://127.0.0.1:3001

# Остальные волны — тем же админом; qa-p7..qa-p10 проверяют только API.
powershell -File qa-p3.ps1  -AdminEmail admin@local -AdminPassword admin1234
powershell -File qa-p4.ps1  -AdminEmail admin@local -AdminPassword admin1234
powershell -File qa-p5.ps1  -AdminEmail admin@local -AdminPassword admin1234
powershell -File qa-p6.ps1  -AdminEmail admin@local -AdminPassword admin1234
powershell -File qa-p7.ps1  -AdminEmail admin@local -AdminPassword admin1234
powershell -File qa-p8.ps1  -AdminEmail admin@local -AdminPassword admin1234
powershell -File qa-p9.ps1  -AdminEmail admin@local -AdminPassword admin1234
powershell -File qa-p10.ps1 -AdminEmail admin@local -AdminPassword admin1234
```

Все не-GET запросы шлют `Origin`, иначе CSRF-защита (T10) вернёт `403`:

- `qa-abc.ps1` всегда шлёт `http://localhost:3000` — параметра `-Origin` у него
  нет; `-WebUrl` меняет только адрес проверки фронтенда, но не `Origin`;
- `qa-p3..qa-p10` принимают `-Origin` (по умолчанию `http://localhost:3000`,
  у `qa-p10` — `http://localhost:3001`). При стенде на другом порту передайте
  `-Origin http://localhost:<порт>`.

У `qa-p3..qa-p6` есть `-SkipWeb`; у `qa-p7..qa-p10` проверок web нет. Лимиты
rate-limit на `/auth/login` — 10/мин на IP; при частых повторных запусках задайте
`RATE_LIMIT_AUTH_PER_MIN=0` на время прогона (сами лимиты покрыты контрактными
тестами).

Автосид qa-админа: при неудачном входе под `-AdminEmail` `qa-abc.ps1` один раз запускает
`qa-seed-admin.ps1` — тот заводит учётку через сид-админа (`-SeedAdminEmail`/`-SeedAdminPassword`,
дефолты `admin@local`/`admin1234` — как в .env) и выдаёт роль admin, затем вход повторяется.
Отключается switch'ем `-NoAutoSeed`; отдельный запуск — `powershell -File qa-seed-admin.ps1`.

### Интеграционные тесты Postgres (`backend/tests/postgres`)

Методы волны P3 и счётчики сводки проверяются не только сквозным QA, но и на
**живом Postgres**: пакет `backend/tests/postgres` включается при заданном
`TEST_DATABASE_URL`, а без него каждый тест делает `t.Skip` — обычный
`go test ./...` остаётся зелёным и без базы. Тесты создают свои сущности
с уникальными email/slug и не трогают чужие данные, поэтому запускать их можно
и рядом со стендом.

```powershell
# 1. Временный Postgres: отдельный контейнер и порт, стенд не трогаем
docker run -d --rm --name edu-pgtest -e POSTGRES_USER=edu -e POSTGRES_PASSWORD=edu -e POSTGRES_DB=edu_test -p 55432:5432 postgres:16-alpine

# 2. Прогон из контейнера с Go; миграции применяет сам тест
docker run --rm --network host -v "$PWD\backend:/src" -v gomodcache:/go/pkg/mod -w /src -e GOFLAGS=-mod=mod -e TEST_DATABASE_URL="postgres://edu:edu@127.0.0.1:55432/edu_test?sslmode=disable" golang:1.24-alpine sh -c "go test ./tests/postgres/... -count=1 -v"

# 3. Убрать за собой
docker stop edu-pgtest
```

Если Docker недоступен, но Go и клиент Postgres установлены нативно:

```powershell
createdb -h 127.0.0.1 -p 5432 -U postgres edu_test
$env:TEST_DATABASE_URL="postgres://postgres:пароль@127.0.0.1:5432/edu_test?sslmode=disable"
go test ./tests/postgres/... ./internal/repo -count=1 -v
```

Покрыто: пользователи (фильтры, блокировка/разблокировка, зависимости при
удалении), сводка админки (в том числе `users.blocked` — до T34 счётчик
на Postgres всегда показывал 0), журнал платежей (фильтры, тоталы, пагинация),
ученики курса и их прогресс, кабинет ученика (записи, платежи, `nextLessonId`),
Q&A и правка ответов, избранное, похожие курсы с fallback, приглашения и
`processed_at` платежей (T47).

`repo.ConnectPool` принудительно переводит SQL-сессию и декодирование
`timestamptz` в UTC — API не зависит от часового пояса сервера Postgres
(регрессия `startsAt` `+03:00` на Europe/Moscow закрыта тестом
`TestConnectPoolScansTimestamptzUTC`). PG-тесты похожих курсов используют
уникальную категорию прогона, поэтому их можно повторять на наполненной БД.

## Документация API

- [`docs/API.md`](docs/API.md) — сводный обзор эндпоинтов, формы запросов/ответов и таблица кодов ошибок.
- `specs/001-it-courses-mvp/contracts/openapi.yaml` — машинно-читаемый контракт
  (OpenAPI 3.0). **Отстаёт от кода**: покрывает ранние волны; админ-консоль,
  журнал платежей и настройки, mock-страница оплаты, Яндекс OAuth, Q&A,
  промокоды, избранное, сертификаты, сессии и часть teacher-маршрутов в него
  не внесены. Актуальные пути и формы — `docs/API.md` и код
  `backend/internal/handler`.

## Доработки (папка `Task/`)

`Task/` — единый источник правды по доработкам репозитория:
зафиксированный контракт API (`Task/00-api-contract.md`), описание каждой
задачи (один файл = один агент = один git-worktree) и карта владения файлами.

- Как читать: начни с [`Task/README.md`](Task/README.md), затем
  `Task/00-api-contract.md` и нужный файл `Task/T**-*.md`.
- Как добавить задачу: создай `Task/T<номер>-<ключ>.md` по образцу существующих
  (проблема, владение файлами, что сделать, критерии приёмки, проверка, отчёт)
  и добавь строку в таблицу «Задачи» `Task/README.md`. Работай только
  в своём worktree и только со своими файлами.

## Окружение

Все переменные — в [.env.example](.env.example) с комментариями. Сводка
(значения по умолчанию — из `config.Load()`, `internal/payments/*` и
`docker-compose.yml`):

| Переменная | Смысл | По умолчанию | Обязательна в проде |
|---|---|---|---|
| `DATABASE_URL` | DSN Postgres; пусто → in-memory без сохранности | — (пусто) | **да**, иначе in-memory |
| `SESSION_SECRET` | HMAC-подпись сессионных cookie | `change-me` (dev) | **да**; пустой в проде → API не стартует |
| `APP_ENV` | `development` \| `production`; пусто — дедукция по `DATABASE_URL` | — (пусто) | рекомендуется `production` |
| `PORT` | порт API внутри контейнера (host — 8082) | `8080` | нет |
| `WEB_PORT` | порт web-контейнера на хосте (читает `docker-compose.yml`, не Go) | `3000` | нет (свой при публикации) |
| `MIGRATIONS_DIR` | override каталога миграций | — (авто-поиск) | нет |
| `PAYMENTS_PROVIDER` | `mock` \| `stripe` \| `yookassa` | `mock` | да, если не mock |
| `COMMISSION_PCT` | комиссия платформы, 0..90 | `15` | нет (меняется админом через API) |
| `STRIPE_SECRET_KEY` | ключ API Stripe | — | **да при `PAYMENTS_PROVIDER=stripe`** |
| `STRIPE_WEBHOOK_SECRET` | HMAC-секрет `Stripe-Signature` | — | **да при stripe** |
| `YOOKASSA_SHOP_ID` | идентификатор магазина YooKassa | — | **да при yookassa** |
| `YOOKASSA_SECRET_KEY` | секрет YooKassa (Basic-auth) | — | **да при yookassa** |
| `YOOKASSA_TRUSTED_IPS` | CIDR/IP доверенных источников вебхуков, через запятую | — (проверка выключена) | рекомендуется при yookassa |
| `MOCK_WEBHOOK_SECRET` | если задан — mock-вебхук требует `X-Mock-Signature` | — (подпись не нужна) | нет |
| `PAYMENTS_RETURN_URL` | абсолютный URL возврата после оплаты | `http://localhost:3000/` | да для stripe/yookassa |
| `RATE_LIMIT_AUTH_PER_MIN` | лимит `/auth/login`, `/auth/register` на IP; ≤0 — выкл. | `10` | нет |
| `RATE_LIMIT_GLOBAL_PER_MIN` | глобальный лимит `/api/v1` на IP | `300` | нет |
| `RATE_LIMIT_WEBHOOK_PER_MIN` | лимит `/payments/webhook/{provider}` | `120` | нет |
| `COOKIE_SAMESITE` | `lax` \| `strict` \| `none` для сессионной cookie | `lax` | нет |
| `CSRF_ALLOW_MISSING_ORIGIN` | разрешить не-GET без `Origin`/`Referer` (для локальных qa-скриптов) | `false` | держать `false` |
| `ALLOWED_ORIGINS` | CORS-источники через запятую | `http://localhost:3000,http://127.0.0.1:3000` | да (свои домены) |
| `RUNNER_URL` | базовый URL песочницы кода | `http://runner:8081` | нет (в compose уже зашит) |
| `YANDEX_CLIENT_ID` | OAuth-приложение Яндекса | — | да, если включён вход через Яндекс |
| `YANDEX_CLIENT_SECRET` | секрет OAuth-приложения Яндекса | — | да, если включён вход через Яндекс |
| `YANDEX_REDIRECT_URI` | Redirect URI, должен совпадать с зарегистрированным | — | да, если включён вход через Яндекс |
| `ADMIN_EMAIL` + `ADMIN_PASSWORD` | опциональный сид админа (создаётся только пара) | — (пусто) | нет |
| `TEACHER_EMAIL` + `TEACHER_PASSWORD` | dev-сид преподавателя при каждом старте API; compose эти переменные пока не передаёт | `teacher@local` / `teacher1` | нет (известные dev-учётки, не для прода) |
| `PUBLIC_APP_URL` | публичная база ссылок в письмах-приглашениях | `http://localhost:3000` | да (свой домен) |
| `SMTP_ADDR` | SMTP `host:port` для приглашений; пусто — письма выключены | — (пусто) | нет |
| `SMTP_FROM` | адрес отправителя писем | — | да, если включён SMTP |
| `SMTP_USERNAME` + `SMTP_PASSWORD` | SMTP-аутентификация | — (без auth) | зависит от провайдера |
| `SMTP_TLS` | `starttls` (587) \| `implicit` (465) \| `none` (локальный тест) | `starttls` | рекомендуется `starttls`/`implicit` |
| `VITE_API_URL` | базовый URL API для фронтенда; нужен только локальному `npm run dev` (в docker nginx проксирует `/api` сам) | `/api/v1` (в коде клиента) | нет |
| `VITE_API_PROXY` | target dev-прокси Vite для `/api` и `/uploads` (`frontend/vite.config.ts`) | `http://localhost:8082` | нет |
| `TEST_DATABASE_URL` | DSN для интеграционных тестов `backend/tests/postgres`; пусто — тесты пропускаются | — (пусто) | нет |

`PAYMENTS_RETURN_URL` по умолчанию — `http://localhost:3000/`
(`payments.DefaultReturnURL`), а в отслеживаемом dev-`.env` веб отдаётся на
`:3001`. Для Stripe/YooKassa задайте URL возврата явно; mock-страница
`/pay/mock/{id}` эту настройку не использует.

SMTP проверяется тестами на локальном SMTP-сервере (формат письма, UTF-8,
обработка ошибок), но не против реального провайдера. Для продакшена задайте
`SMTP_ADDR`, `SMTP_FROM`, при необходимости логин/пароль и `PUBLIC_APP_URL`,
затем создайте тестовое приглашение и проверьте доставку. Если SMTP не задан,
`POST /admin/invites` продолжает работать: админ получает `inviteLink` и
`emailSent: false`.

`inviteLink` строится из `PUBLIC_APP_URL`. В отслеживаемом dev-`.env` переменная
не задана, поэтому compose подставляет `http://localhost:3000`, хотя web в dev
отдаётся на `:3001`; для корректных ссылок задайте
`PUBLIC_APP_URL=http://localhost:3001` (или свой публичный адрес).

### Про `.env` в репозитории

Файл `.env` **отслеживается git намеренно** и содержит только несекретные
dev-значения (`SESSION_SECRET=change-me` и т.п.) — иначе не работает
`docker compose up` из «Быстрого старта» (compose читает `.env` автоматически).
В `.gitignore` добавлено правило `!.env.example` и `.env`, поэтому **новые
локальные env-файлы не коммитятся**. Для прода: свой env-file с настоящими
секретами через `docker compose --env-file .env.prod up -d` — его в git не добавлять.

## Статус реализации

В `main @ df9ed31` влиты волны 1–3, A–C и P3–P10, затем пост-P10 правки (PR #11/#12).

- **Волны 1–3 и A–C (T01–T20):** API-клиент и i18n, экран обучения, каталог и отзывы,
  роутинг и гварды, конструктор, журнал платежей и комиссия, Stripe/YooKassa,
  cookie/CSRF/rate-limit, runner, контрактные тесты и `qa-abc.ps1`, демо-учётки,
  CRUD конструктора.
- **P3 (T21–T30):** жизненный цикл курса, ученики и статистика, кабинет ученика,
  супер-админка (пользователи, сводка, курсы, записи).
- **P4 (T31–T40):** поиск и период в журнале платежей, блокировка OAuth, guard архива,
  Postgres-тесты, hardening вебхуков, конструктор по `id`, CSV-выгрузки.
- **P5 (T41–T46):** транслитерация slug, CORS-preflight, предупреждение об обрезке CSV,
  тесты runner с интерпретаторами, автосид qa-админа, каскад Postgres.
- **P6 (T47–T52):** приглашения и возвраты, фильтры админки, записи на курсы, UI админки.
- **P7 (T53–T62):** unpublish и валидация квиза, обложки курсов, детали курса, UI/стили.
- **P8 (T63–T70):** промокоды, категории и фильтры каталога, сертификаты, ответы на отзывы.
- **P9 (T71–T82):** Q&A по урокам, избранное, превью, аналитика, похожие курсы,
  публичный профиль преподавателя.
- **P10 (T83–T94):** глобальные отзывы/Q&A/промокоды, супер-админ в конструкторе,
  принудительная публикация, сессии и принудительный выход.
- **Пост-P10:** подсветка синтаксиса (PR #11), отдельные страницы админки (PR #12).

Подробности, коммиты и ограничения волн — [`Task/STATUS.md`](Task/STATUS.md).
P11 (ограничение mock-платежей dev/test) идёт отдельными PR и в `df9ed31` не входит.

## Известные ограничения

- **Mock-платежи доступны в любом окружении.** `payments.NewProvider("mock")` не
  проверяет `APP_ENV`; при `PAYMENTS_PROVIDER=mock` (дефолт) оплата проходит и в
  production. Ограничение «mock только dev/test» — целевая модель P11, в этот
  срез не входит.
- **Вебхук выбирает провайдера по пути.** `POST /payments/webhook/{provider}`
  проверяет подпись/источник выбранного провайдера, но не сверяет `{provider}` с
  `PAYMENTS_PROVIDER`.
- **`App.webhookMu` — process-local.** Несколько инстансов API с общим Postgres
  не сериализуют применение одного события; для боевого контура нужен атомарный
  переход статуса на уровне БД.
- **Автосид `teacher@local / teacher1`** выполняется при каждом старте API,
  перезаписывает несовпадающий хеш и принудительно ставит роль `teacher`;
  `docker-compose.yml` не передаёт `TEACHER_*`, поэтому в контейнере действуют
  дефолты (см. «Демо-учётки»).
- **Удаление курса — только `draft`** для всех ролей: каскад в Postgres удаляет
  платежи и записи, что сломало бы журнал. Архивация — мягкий статус `archived`;
  `PATCH` карточки намеренно работает в `review`/`published`, в архиве —
  `409 «Курс в архиве: карточка заморожена»`. Контентные операции: `teacher` —
  только `draft`, `admin` — любой неархивный курс (T87).
- **Обложки хранятся локально** в каталоге `uploads/` (в compose — volume
  `uploads`), внешнего object storage (S3 и т.п.) и CDN нет: без общего volume
  инстансы не видят файлы друг друга.
- **Внешние интеграции не проверены вживую.** Stripe/YooKassa требуют реальных
  ключей и публичного HTTPS-вебхука, SMTP — реального провайдера; в репозитории
  проверены mock-провайдер, подпись/идемпотентность вебхуков и локальный
  SMTP-сервер. Перед публикацией задайте ключи и прогоните тестовую оплату и
  письмо-приглашение.
- **Docker Hub по TLS.** На машине, где готовился этот срез, `docker pull` к
  Docker Hub периодически падал с TLS-ошибкой; рабочие образы уже были в
  локальном кэше. Если `docker compose build` не может скачать базовый образ,
  проверьте сеть/прокси и повторите pull.
- **Живой Yandex OAuth не проверялся:** покрыты `503` без ключей, `400` по
  `state` и юнит-сценарии; реальный редирект и обмен кода с провайдером — нет.
- **CSV-выгрузки админки** собираются на клиенте по 100 строк и обрезаются на
  5000 строках; при обрезке UI показывает предупреждение.

## Безопасность платежей (целевая модель P11)

Текущее поведение `main @ df9ed31`:

- mock-провайдер не зависит от окружения: `NewProvider("mock")` возвращает
  рабочий провайдер и в production;
- `POST /payments/webhook/{provider}` проверяет подпись/источник провайдера из
  URL, но не требует совпадения с `PAYMENTS_PROVIDER`;
- `webhookMu` сериализует применение события только внутри одного процесса API.

Целевая модель P11 — запрет mock-платежей в production. Правки идут отдельными
PR и в `df9ed31` не входят; до их мержа не публикуйте стенд с
`PAYMENTS_PROVIDER=mock` в интернет.

## Спецификации

Разработано по SDD-процессу (Spec Kit):

- `.specify/memory/constitution.md` — конституция проекта
- `specs/001-it-courses-mvp/` — spec, plan, data-model, tasks, quickstart, converge-report
- `specs/001-it-courses-mvp/contracts/openapi.yaml` — контракт API (может отставать, см. «Документация API»)
