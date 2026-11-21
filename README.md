# Edu Platform — маркетплейс IT-курсов

Платформа онлайн-курсов для начинающих: студенты покупают курсы, приглашённые преподаватели создают их, админ модерирует публикацию.

## Возможности MVP

- **Каталог курсов** — фильтры, направления, поиск, страница курса с программой и отзывами.
- **Конструктор курсов** — модули, уроки, автопроверка кода и квизы, живые эфиры (Zoom/Meet как внешние ссылки).
- **Модерация** — черновик → заявка → проверка админом → публикация.
- **Оплата** — интерфейс `PaymentProvider` (mock / Stripe / YooKassa), комиссия платформы настраивается, идемпотентный вебхук.
- **Обучение** — прогресс по урокам, кодовые задачи (Python, JS, HTML/CSS) и тесты, runner с таймаутом 5с.
- **Роли** — студент, преподаватель (только по приглашению), админ. Email+пароль, сессии в cookie (подписанные HMAC), rate-limit, жёсткий CSRF.
- **Безопасность аккаунта** — сброс пароля по одноразовой ссылке, подтверждение email, просмотр и завершение активных сессий в профиле. Для писем нужен SMTP.
- **Админ-US6** — журнал платежей с разбивкой комиссия/преподаватель, настройка комиссии через API.

## Стек

| Сервис   | Технологии                          | Порт (host) |
|----------|-------------------------------------|-------------|
| backend  | Go, chi, pgxpool, миграции SQL      | 8082 (внутри контейнера 8080) |
| frontend | React + Vite + TypeScript, nginx    | 3000        |
| runner   | Go (python/js/htmlcss sandbox)      | — (внутренняя сеть `runner_net`, 8081; наружу не публикуется) |
| db       | Postgres 16                         | — (наружу не публикуется) |

В dev бэкенд может работать на in-memory хранилище без `DATABASE_URL`; в production пустой или недоступный Postgres — фатальная ошибка старта, а не тихий fallback.

## Быстрый старт

```bash
# в репозитории уже есть dev .env с ADMIN_EMAIL/ADMIN_PASSWORD;
# если копируете .env.example — задайте эти переменные сами
docker compose up -d --build
# http://localhost:3001 — веб (в репозиторном .env WEB_PORT=3001; без него 3000),
# http://localhost:8082 — API
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

### Админ в dev

`.env` содержит dev-заглушки `ADMIN_EMAIL=admin@local` /
`ADMIN_PASSWORD=admin1234`, поэтому после `docker compose up` админ входит
**без ручных действий**:

```bash
docker compose up -d --build db api runner
# http://localhost:3001  →  admin@local / admin1234
```

`seedAdmin` создаёт админа только при явно заданной паре
`ADMIN_EMAIL` + `ADMIN_PASSWORD`; дефолтных учёток в коде нет. В production
задайте свои значения через отдельный env-file:

```bash
docker compose --env-file .env.prod up -d
```

### Преподаватель

Дефолтного `teacher@local` больше нет. Новый преподаватель создаётся только
двумя способами:

1. **Приглашение (рекомендуется).** Войдите админом → `/admin` → создайте
   приглашение. Если настроен SMTP, письмо со ссылкой уйдёт учителю
   автоматически; иначе скопируйте `inviteLink`/токен из UI и откройте
   `/register?inviteToken=...` вручную. Регистрация сразу даст роль
   преподавателя (T17 починил чтение токена из query).
2. **Явный bootstrap.** Задайте пару `TEACHER_EMAIL` + `TEACHER_PASSWORD` в
   окружении `api` (compose пробрасывает обе переменные). Пароль хешируется
   bcrypt при старте; существующий валидный пароль не перезаписывается.

В dev-режиме с Postgres сид создаёт `demo-teacher@local / teach1234` и
демо-курс «Python с нуля». В in-memory режиме (без `DATABASE_URL`) эти же
данные создаются в памяти. В production демо-сид не применяется.

### Ручная установка пароля (запасной вариант)

Если нужно сменить пароль вручную:

```bash
# сгенерировать хеш (нужен Docker; $2y$ принимается backend'ом — проверено)
docker run --rm httpd:alpine sh -c "htpasswd -bnBC 10 '' '<НОВЫЙ_ПАРОЛЬ>' | tr -d ':\n'; echo"
# применить (в PowerShell хеш — в одинарных кавычках!)
docker compose exec db psql -U edu -d edu \
  -c "UPDATE users SET password_hash='<ХЕШ>' WHERE email='<EMAIL>';"
```

## Тесты и QA

```bash
cd backend  && go test ./...     # юнит-тесты бэкенда
cd runner   && go test ./...     # тесты песочницы
cd frontend && npm ci && npm run build   # tsc --noEmit + vite build
```

### Проверка runner (python3/nodejs)

Тесты `cd runner && go test ./...` пропускают (`SKIP`) реальный запуск python/js,
если интерпретаторов нет в окружении, а слой сборки `runner/Dockerfile` может
быть взят из кэша — поэтому песочницу целиком проверяем так (из корня репозитория):

```powershell
docker run --rm -v "${PWD}\runner:/src" -w /src golang:1.24-bookworm sh -c "apt-get update -qq && apt-get install -y -qq --no-install-recommends python3 nodejs >/dev/null && go vet ./... && go test ./... -count=1 -v"
```

Ожидаемый итог — `ok runner` без единого `SKIP`/`FAIL`. Если репозиторий Debian
недоступен из вашей сети, замените `deb.debian.org` на рабочее зеркало
(например, `mirror.yandex.ru`) в команде `apt-get update`.

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
powershell -File qa-abc.ps1 -BaseUrl http://127.0.0.1:8082 -WebUrl http://127.0.0.1:3001
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

Если Postgres установлен нативно и Docker недоступен:

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

Переменные окружения — в [.env.example](.env.example) с комментариями.
Сводка (значения по умолчанию взяты из `config.Load()` и
`internal/payments/*`):

| Переменная | Смысл | По умолчанию | Обязательна в проде |
|---|---|---|---|
| `DATABASE_URL` | DSN Postgres; пусто/недоступен → in-memory только в dev, в production API не стартует | — (пусто) | **да** |
| `SESSION_SECRET` | HMAC-подпись сессионных cookie | `change-me` (dev) | **да**; пустой/`change-me` в проде → API не стартует |
| `APP_ENV` | `development` \| `production`; в Compose обязателен, вне Compose пусто — дедукция по `DATABASE_URL` | — (пусто) | **да**, `production` |
| `PORT` | порт API внутри контейнера (host — 8082) | `8080` | нет |
| `MIGRATIONS_DIR` | override каталога миграций | — (авто-поиск) | нет |
| `PAYMENTS_PROVIDER` | `mock` \| `stripe` \| `yookassa` | `mock` | **да**; `mock` разрешён только в dev |
| `COMMISSION_PCT` | комиссия платформы, 0..90 | `15` | нет (меняется админом через API) |
| `STRIPE_SECRET_KEY` | ключ API Stripe | — | **да при `PAYMENTS_PROVIDER=stripe`** |
| `STRIPE_WEBHOOK_SECRET` | HMAC-секрет `Stripe-Signature` | — | **да при stripe** |
| `YOOKASSA_SHOP_ID` | идентификатор магазина YooKassa | — | **да при yookassa** |
| `YOOKASSA_SECRET_KEY` | секрет YooKassa (Basic-auth) | — | **да при yookassa** |
| `YOOKASSA_TRUSTED_IPS` | CIDR/IP доверенных источников вебхуков, через запятую | — (проверка выключена) | рекомендуется при yookassa; невалидный список отклоняет все вебхуки |
| `MOCK_WEBHOOK_SECRET` | в dev: если задан, mock-вебхук требует `X-Mock-Signature` | — (подпись не нужна) | нет; mock в production запрещён |
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
| `ADMIN_EMAIL` + `ADMIN_PASSWORD` | опциональный bootstrap админа; обе переменные или ни одной, неполная пара — ошибка старта | — (пусто) | да для первого запуска |
| `TEACHER_EMAIL` + `TEACHER_PASSWORD` | опциональный bootstrap преподавателя; обе переменные или ни одной, неполная пара — ошибка старта | — (пусто) | нет; обычно приглашение |
| `PUBLIC_APP_URL` | публичная база ссылок в письмах-приглашениях | `http://localhost:3000` | да (свой домен) |
| `SMTP_ADDR` | SMTP `host:port` для приглашений; пусто — письма выключены | — (пусто) | нет |
| `SMTP_FROM` | адрес отправителя писем | — | да, если включён SMTP |
| `SMTP_USERNAME` + `SMTP_PASSWORD` | SMTP-аутентификация | — (без auth) | зависит от провайдера |
| `SMTP_TLS` | `starttls` (587) \| `implicit` (465) \| `none` (локальный тест) | `starttls` | `starttls`/`implicit`; `none` и опечатки в production запрещены |
| `VITE_API_URL` | базовый URL API для фронтенда; нужен только локальному `npm run dev` (в docker nginx проксирует `/api` сам) | `/api/v1` (в коде клиента) | нет |

SMTP проверяется тестами на локальном SMTP-сервере (формат письма, UTF-8,
обработка ошибок), но не против реального провайдера. Для продакшена задайте
`SMTP_ADDR`, `SMTP_FROM`, при необходимости логин/пароль и `PUBLIC_APP_URL`,
затем создайте тестовое приглашение и проверьте доставку. Если SMTP не задан,
`POST /admin/invites` продолжает работать: админ получает `inviteLink` и
`emailSent: false`.

### Чек-лист production-проверки

- [ ] `APP_ENV=production`, заданы `DATABASE_URL` и `SESSION_SECRET` (не
      dev-значение `change-me`); остальные секреты — только в env-файле вне git.
- [ ] В `docker-compose.yml` заменены демо-креды Postgres (`edu/edu`) и
      `DATABASE_URL` (или подключён override-файл); затем
      `docker compose --env-file .env.prod config --quiet` и
      `docker compose --env-file .env.prod up -d --build`;
      `GET http://localhost:8082/healthz` отвечает `200 {"status":"ok"}`,
      миграции применились.
- [ ] Postgres: данные переживают перезапуск `api`; том БД не удаляется при
      пересоздании контейнеров. `TEST_DATABASE_URL` для тестов указывает на
      отдельную базу, не на production.
- [ ] SMTP: `SMTP_ADDR`, `SMTP_FROM`, `PUBLIC_APP_URL` заданы; тестовое
      приглашение доставлено, API вернул `emailSent: true`.
- [ ] Платежи: `PAYMENTS_PROVIDER` не `mock`; ключи провайдера,
      `STRIPE_WEBHOOK_SECRET`/`YOOKASSA_*` и публичный `PAYMENTS_RETURN_URL`
      заданы; тестовый платёж, вебхук и возврат прошли.
- [ ] CORS/cookies: `ALLOWED_ORIGINS` содержит только реальные домены
      (HTTPS), `CSRF_ALLOW_MISSING_ORIGIN=false`; при cross-site cookie —
      `COOKIE_SAMESITE=none` и HTTPS.
- [ ] Демо-данные не используются: `ADMIN_EMAIL`/`ADMIN_PASSWORD` заданы
      своими значениями, `PAYMENTS_PROVIDER` — `stripe` или `yookassa`,
      `SMTP_TLS` — `starttls`/`implicit`; `APP_ENV=production` подтверждён в
      окружении контейнера `api`.

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

Закрыто (волны P3–P9): жизненный цикл курса и супер-админка (T21–T30),
платежи/CSV и устойчивость Postgres (P4–P5), админка и возвраты (P6),
жизненный цикл учителя и обложки (P7), промокоды/сертификаты/отзывы (P8),
Q&A/избранное/превью/аналитика/похожие курсы/профиль учителя (P9).

Текущее состояние и история волн — [`Task/STATUS.md`](Task/STATUS.md),
актуальный статус исправлений и остатки — [`Task/REVIEW-2026-11-17.md`](Task/REVIEW-2026-11-17.md).

Известные ограничения:

- удаление курса (`DELETE /teacher/courses/{id}`) доступно только в `draft`:
  каскад в Postgres удаляет записи и платежи и сломал бы журнал US6. Для
  опубликованных курсов есть `archive`/`unarchive`: курс уходит в `archived`,
  возврат — в `draft`;
- CSV-выгрузки (платежи, пользователи, курсы, записи) отдают не больше
  5000 строк за один файл; при обрезке UI предупреждает;
- обложки курсов хранятся на локальном диске API (`/uploads`, volume
  `uploads`); для нескольких хостов нужен общий storage или перенос файлов;
- платёжные операции сериализуются PostgreSQL advisory lock между репликами;
  статус, доступ и использование промокода применяются транзакционно;
  общая блокировка требует нагрузочной проверки перед масштабированием;
- `PAYMENTS_PROVIDER=mock` доступен только в dev: production обязан
  использовать Stripe или YooKassa;
- демо-курс и `demo-teacher@local` создаются только в dev; в production
  bootstrap-учётки задаются явно через `ADMIN_*`/`TEACHER_*`;
- сброс пароля и подтверждение email требуют настроенного SMTP и `PUBLIC_APP_URL`;
  локальная тестовая почта включается через `docker-compose.dev-mail.yml`.

## Спецификации

Приёмка, browser E2E, sandbox runner, резервирование и восстановление:
[`docs/OPERATIONS.md`](docs/OPERATIONS.md).

Разработано по SDD-процессу (Spec Kit):

- `.specify/memory/constitution.md` — конституция проекта
- `specs/001-it-courses-mvp/` — spec, plan, data-model, tasks, quickstart, converge-report
- `specs/001-it-courses-mvp/contracts/openapi.yaml` — контракт API (может отставать, см. «Документация API»)
