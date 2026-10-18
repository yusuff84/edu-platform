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
| frontend | React + Vite + TypeScript, nginx    | 3000        |
| runner   | Go (python/js/htmlcss sandbox)      | — (внутренняя сеть `runner_net`, 8081; наружу не публикуется) |
| db       | Postgres 16                         | — (наружу не публикуется) |

Бэкенд подключается к Postgres, а при недоступности `DATABASE_URL` работает на in-memory хранилище.

## Быстрый старт

```bash
cp .env.example .env          # задать ADMIN_EMAIL / ADMIN_PASSWORD — создастся админ при старте
docker compose up -d --build
# http://localhost:3000 — веб, http://localhost:8082 — API
# http://localhost:8082/healthz → {"status":"ok"}
```

Миграции применяются автоматически при старте API.

### Если порт 3000 занят другим проектом

Порт фронта настраивается через `WEB_PORT`, но **CSRF и CORS привязаны к
origin'у** — просто перенаправить 3000 на другой порт недостаточно, браузер
получит отказ на любых не-GET запросах (вход, регистрация). Меняйте обе
переменные одной парой:

```bash
# PowerShell
$env:WEB_PORT="3001"
# Важно: список — это allowlist. Оставьте в нём и :3000, иначе сквозные
# скрипты (они шлют Origin http://localhost:3000) получат 403 на не-GET.
$env:ALLOWED_ORIGINS="http://localhost:3000,http://127.0.0.1:3000,http://localhost:3001,http://127.0.0.1:3001"
docker compose up -d --build --force-recreate api web

# bash
WEB_PORT=3001 ALLOWED_ORIGINS="http://localhost:3000,http://127.0.0.1:3000,http://localhost:3001,http://127.0.0.1:3001" \
  docker compose up -d --build --force-recreate api web
```

`ALLOWED_ORIGINS` по умолчанию — `http://localhost:3000,http://127.0.0.1:3000`
(см. `docker-compose.yml`, `internal/config/config.go`). Если фронтенд отдаётся
на другом порту, а `ALLOWED_ORIGINS` не поправлен, UI отвечает
`403 «Недопустимый источник запроса»` на любой не-GET — вход, сохранение,
архивацию курса. Сквозные скрипты принимают порт отдельно:
`powershell -File qa-abc.ps1 -WebUrl http://localhost:3001`.

## Демо-учётки

### Админ — работает сразу

`.env` уже содержит dev-заглушки `ADMIN_EMAIL=admin@local` / `ADMIN_PASSWORD=admin1234`,
поэтому после `docker compose up` админ входит **без ручных действий**:

```bash
docker compose up -d --build db api runner
# http://localhost:3000  →  admin@local / admin1234
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

### Преподаватель

`teacher@local` из SQL-сида по-прежнему с поддельным хешем — переменных под
него нет, чтобы не расширять scope. **Два рабочих способа получить учителя:**

1. **Рекомендуемый — приглашение.** Войдите админом → `/admin` → создайте
   приглашение → откройте скопированную ссылку `/register?inviteToken=...`.
   Регистрация сразу даст роль преподавателя (T17 починил чтение токена из query).
2. **In-memory режим** (без `DATABASE_URL`, только разработка):
   `demo-teacher@local / teach1234`.

### Ручная установка пароля (запасной вариант)

Если нужен именно `teacher@local`:

```bash
# сгенерировать хеш (нужен Docker; $2y$ принимается backend'ом — проверено)
docker run --rm httpd:alpine sh -c "htpasswd -bnBC 10 '' teacher1 | tr -d ':\n'; echo"
# применить (в PowerShell хеш — в одинарных кавычках!)
docker compose exec db psql -U edu -d edu \
  -c "UPDATE users SET password_hash='<ХЕШ>' WHERE email='teacher@local';"
```

## Тесты и QA

```bash
cd backend  && go test ./...     # юнит-тесты бэкенда
cd runner   && go test ./...     # тесты песочницы
cd frontend && npm ci && npm run build   # tsc --noEmit + vite build
```

Сквозной QA-скрипт (**49 проверок**, код возврата 0/1):

```bash
# Требует запущенного стенда. web (фронтенд) не обязателен — недоступность
# считается SKIP, а не провалом:
docker compose up -d db api runner

powershell -File qa-abc.ps1 -AdminEmail admin@local -AdminPassword admin1234
pwsh -File qa-abc.ps1 -AdminEmail admin@local -AdminPassword admin1234  # PS 7

# Скрипт идемпотентен: каждый запуск регистрирует qa-<timestamp>@example.test,
# второй прогон на «грязной» БД тоже даёт 49/49.
# Значения -AdminEmail/-AdminPassword — из ADMIN_EMAIL/ADMIN_PASSWORD в .env
# (там уже подставлены dev-заглушки, см. «Демо-учётки»).
powershell -File qa-abc.ps1 -SkipAdmin         # без админ-блока (US6)
powershell -File qa-abc.ps1 -BaseUrl http://127.0.0.1:8082 -WebUrl http://127.0.0.1:3000
```

Все не-GET запросы в сценарии шлют `Origin: http://localhost:3000` — иначе
CSRF-защита (T10) вернёт `403`. Лимиты rate-limit на `/auth/login` составляют
10/мин на IP; при частых повторных запусках задайте `RATE_LIMIT_AUTH_PER_MIN=0`
на время прогона ( сами лимиты покрыты контрактными тестами).

Автосид qa-админа: при неудачном входе под `-AdminEmail` `qa-abc.ps1` один раз запускает
`qa-seed-admin.ps1` — тот заводит учётку через сид-админа (`-SeedAdminEmail`/`-SeedAdminPassword`,
дефолты `admin@local`/`admin1234` — как в .env) и выдаёт роль admin, затем вход повторяется.
Отключается switch'ем `-NoAutoSeed`; отдельный запуск — `powershell -File qa-seed-admin.ps1`.

### Волна P3: `qa-p3.ps1` (**80 проверок**)

Разделы «конструктор курса», «ученики», «супер-админ» проверяет отдельный
скрипт — прежние 49 проверок остаются в `qa-abc.ps1` как регрессия:

```bash
docker compose up -d --build db api web runner

# Блоки: D — конструктор (T21), E — ученики курса (T22),
#        F — кабинет ученика (T23), G — супер-админ (T24/T25).
powershell -File qa-p3.ps1 -AdminEmail admin@local -AdminPassword admin1234
```

Скрипт создаёт свои сущности с меткой времени (`p3-teacher-<stamp>@example.test`
и т. д.), поэтому повторный прогон на той же базе тоже даёт 80/80. Требуется
поднятый `web` — но только для проверки `P0`; без него она уходит в `SKIP`
(`-SkipWeb`, чтобы не проверять вовсе).

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

Покрыто: пользователи (фильтры, блокировка/разблокировка, зависимости при
удалении), сводка админки (в том числе `users.blocked` — до T34 счётчик
на Postgres всегда показывал 0), журнал платежей (фильтры, тоталы, пагинация),
ученики курса и их прогресс, кабинет ученика (записи, платежи, `nextLessonId`).

## Документация API

- [`docs/API.md`](docs/API.md) — сводный обзор эндпоинтов, формы запросов/ответов и таблица кодов ошибок.
- `specs/001-it-courses-mvp/contracts/openapi.yaml` — машинно-читаемый контракт
  (OpenAPI 3.0). **Может отставать от кода**: эндпоинты US6 (`/admin/payments`,
  `/admin/settings`) и mock-страница оплаты добавлены позже и в yaml не отражены.

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
(значения по умолчанию взяты из `config.Load()` и `internal/payments/*`):

| Переменная | Смысл | По умолчанию | Обязательна в проде |
|---|---|---|---|
| `DATABASE_URL` | DSN Postgres; пусто → in-memory без сохранности | — (пусто) | **да**, иначе in-memory |
| `SESSION_SECRET` | HMAC-подпись сессионных cookie | `change-me` (dev) | **да**; пустой в проде → API не стартует |
| `APP_ENV` | `development` \| `production`; пусто — дедукция по `DATABASE_URL` | — (пусто) | рекомендуется `production` |
| `PORT` | порт API внутри контейнера (host — 8082) | `8080` | нет |
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
| `ALLOWED_ORIGINS` | CORS-источники через запятую | `http://localhost:3000, http://127.0.0.1:3000` | да (свои домены) |
| `RUNNER_URL` | базовый URL песочницы кода | `http://runner:8081` | нет (в compose уже зашит) |
| `YANDEX_CLIENT_ID` | OAuth-приложение Яндекса | — | да, если включён вход через Яндекс |
| `YANDEX_CLIENT_SECRET` | секрет OAuth-приложения Яндекса | — | да, если включён вход через Яндекс |
| `YANDEX_REDIRECT_URI` | Redirect URI, должен совпадать с зарегистрированным | — | да, если включён вход через Яндекс |
| `ADMIN_EMAIL` + `ADMIN_PASSWORD` | опциональный сид админа (создаётся только пара) | — (пусто) | нет |
| `VITE_API_URL` | базовый URL API для фронтенда; нужен только локальному `npm run dev` (в docker nginx проксирует `/api` сам) | `/api/v1` (в коде клиента) | нет |

### Про `.env` в репозитории

Файл `.env` **отслеживается git намеренно** и содержит только несекретные
dev-значения (`SESSION_SECRET=change-me` и т.п.) — иначе не работает
`docker compose up` из «Быстрого старта» (compose читает `.env` автоматически).
В `.gitignore` добавлено правило `!.env.example` и `.env`, поэтому **новые
локальные env-файлы не коммитятся**. Для прода: свой env-file с настоящими
секретами через `docker compose --env-file .env.prod up -d` — его в git не добавлять.

## Статус реализации

Закрыто (волны 1–2): новый API-клиент и типобезопасный i18n (T01/T02),
экран обучения (T03), каталог и отзывы (T04), роутинг и гварды (T05),
конструктор (T06), журнал платежей и комиссия (T07/T08), Stripe/YooKassa
и подпись вебхуков (T09), подпись cookie, rate-limit, CSRF, displayName (T10),
песочница runner (T11), редирект после входа (T15).

Завершено (волна 3): контрактные тесты и qa-скрипт `qa-abc.ps1` (T12),
сквозная проводка маршрутов и compose (T13), документация (T14).

Закрыто (волны A–C): 7 бэкенд-багов включая **демо-учётки** (T16),
`inviteToken` из ссылки и редирект со входа (T17), code-splitting и
дедупликация стилей (T18), CRUD конструктора (T19), +15 тестов и сверка
контракта (T20).

Статус всех 20 задач — [`Task/STATUS.md`](Task/STATUS.md).

Известные ограничения:

- конструктор: создание, правка карточки курса и модуля, правка/удаление
  урока, удаление модуля и курса, перестановка модулей и уроков, счётчики
  в списке курсов — **закрыто T19**. В бэклоге осталась только **архивация
  опубликованных курсов**: `DELETE` работает только в `draft`, потому что
  каскад в Postgres удаляет платежи и записи, что сломало бы журнал US6;
- `PATCH /teacher/courses/{id}` не имеет guard по статусу и работает для
  `review`/`published`, тогда как остальные операции конструктора возвращают
  `409` вне `draft`. Метаданные правятся в любом статусе — единообразия нет;
- `GET /admin/payments` отдаёт `studentEmail`/`studentName` (T16), но
  **фильтра по email студента нет**;
- `teacher@local` из SQL-сида по-прежнему с поддельным хешем — переменных под
  него нет. Учителя заводите приглашением (см. «Демо-учётки»);
- `slugRe` пропускает только латиницу, цифры и дефис: кириллический slug
  отклоняется `400`, а публичный каталог не транслитерирует.

## Спецификации

Разработано по SDD-процессу (Spec Kit):

- `.specify/memory/constitution.md` — конституция проекта
- `specs/001-it-courses-mvp/` — spec, plan, data-model, tasks, quickstart, converge-report
- `specs/001-it-courses-mvp/contracts/openapi.yaml` — контракт API (может отставать, см. «Документация API»)
