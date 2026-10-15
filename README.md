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

## Демо-учётки: важное предупреждение

`specs/001-it-courses-mvp/quickstart.md` обещает `admin@local / admin1234` и
`teacher@local / teacher1`, но в `backend/internal/migrate/migrations/002_seed.sql`
лежат **невалидные bcrypt-заглушки** — войти под этими учётками **нельзя**,
пока пароль не установить вручную. Миграция намеренно не правится.

Рабочие варианты:

1. **Рекомендуемый — сид админа через env.** Задайте `ADMIN_EMAIL` и
   `ADMIN_PASSWORD` в `.env` и перезапустите API: при старте создаётся админ
   с настоящим bcrypt-хешем (если email ещё не занят). Дальше учителей заводите
   приглашениями (`/invites`), студентов — обычной регистрацией.
2. **Установить пароль для seed-пользователя.** Сгенерируйте bcrypt-хеш и
   обновите строку в БД:

   ```bash
   # сгенерировать хеш (нужен Docker; $2y$ принимается backend'ом — проверено)
   docker run --rm httpd:alpine sh -c "htpasswd -bnBC 10 '' admin1234 | tr -d ':\n'; echo"
   # применить к демо-админу (подставить <ХЕШ>; в PowerShell хеш — в одинарных кавычках!)
   docker compose exec db psql -U edu -d edu \
     -c "UPDATE users SET password_hash='<ХЕШ>' WHERE email='admin@local';"
   # то же для преподавателя: email 'teacher@local'
   ```

Без этих действий демо-пользователи из SQL-сида входить не могут.
В **in-memory режиме** (без `DATABASE_URL`, только разработка) есть рабочая
демо-учётка преподавателя: `demo-teacher@local / teach1234`.

## Тесты и QA

```bash
cd backend  && go test ./...     # юнит-тесты бэкенда
cd runner   && go test ./...     # тесты песочницы
cd frontend && npm ci && npm run build   # tsc --noEmit + vite build
```

Сквозной QA-скрипт (39 проверок, код возврата 0/1):

```bash
# Требует запущенного стенда. web (фронтенд) не обязателен — недоступность
# считается SKIP, а не провалом:
docker compose up -d db api runner

powershell -File qa-abc.ps1                    # PowerShell 5.1
pwsh -File qa-abc.ps1                          # или PowerShell 7

# Скрипт идемпотентен: каждый запуск регистрирует qa-<timestamp>@example.test.
# Для блока US6 (журнал платежей, комиссия, приглашения) нужен админ —
# задайте пару ADMIN_EMAIL/ADMIN_PASSWORD в .env перед запуском стенда.
powershell -File qa-abc.ps1 -SkipAdmin         # без админ-блока
powershell -File qa-abc.ps1 -BaseUrl http://127.0.0.1:8082 -WebUrl http://127.0.0.1:3000
```

Все не-GET запросы в сценарии шлют `Origin: http://localhost:3000` — иначе
CSRF-защита (T10) вернёт `403`. Лимиты rate-limit на `/auth/login` составляют
10/мин на IP; при частых повторных запусках задайте `RATE_LIMIT_AUTH_PER_MIN=0`
на время прогона ( сами лимиты покрыты контрактными тестами).

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
Статус всех 15 задач — [`Task/STATUS.md`](Task/STATUS.md).

Известные ограничения:

- конструктор: созданы создание курсов/модулей/уроков, плюс (T13) чтение
  курса по id, правка карточки курса и переименование модуля. Удаление,
  перестановка и редактирование урока после создания — в бэклоге;
- `POST /teacher/courses/{id}/lessons` принимает `startsAt`, но не сохраняет
  его в урок — планирование эфиров пока не работает. Закреплено тестом-фиксатором
  `teacher_test.go:TestStartsAtNotPersistedDiscrepancy4`, который упадёт при починке;
- `GET /admin/payments` отдаёт только `studentId`, без email/имени студента —
  в админке колонка «Студент» показывает обрезанный UUID;
- при провале квиза/кода в ответе `POST .../complete` нет `percent`/`nextLesson`;
- демо-пользователи SQL-сида не могут войти без ручной настройки пароля
  (см. раздел «Демо-учётки»).

## Спецификации

Разработано по SDD-процессу (Spec Kit):

- `.specify/memory/constitution.md` — конституция проекта
- `specs/001-it-courses-mvp/` — spec, plan, data-model, tasks, quickstart, converge-report
- `specs/001-it-courses-mvp/contracts/openapi.yaml` — контракт API (может отставать, см. «Документация API»)
