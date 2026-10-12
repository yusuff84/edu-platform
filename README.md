# Edu Platform — маркетплейс IT-курсов

Платформа онлайн-курсов для начинающих: студенты покупают курсы, приглашённые преподаватели создают их, админ модерирует публикацию.

## Возможности MVP

- **Каталог курсов** — фильтры, направления, поиск, страница курса с программой и отзывами.
- **Конструктор курсов** — модули, уроки, автопроверка кода и квизы, живые эфиры (Zoom/Meet как внешние ссылки).
- **Модерация** — черновик → заявка → проверка админом → публикация.
- **Оплата** — интерфейс `PaymentProvider` (mock / Stripe / YooKassa), комиссия платформы настраивается, идемпотентный вебхук.
- **Обучение** — прогресс по урокам, кодовые задачи (Python, JS, HTML/CSS) и тесты, runner с таймаутом 5с.
- **Роли** — студент, преподаватель (только по приглашению), админ. Email+пароль, сессии в cookie.

## Стек

| Сервис   | Технологии                          | Порт (host) |
|----------|-------------------------------------|-------------|
| backend  | Go, chi, pgxpool, миграции SQL      | 8082        |
| frontend | React + Vite + TypeScript, nginx    | 3000        |
| runner   | Go (python/js/htmlcss sandbox)      | 8081        |
| db       | Postgres                            | —           |

Бэкенд подключается к Postgres, а при недоступности `DATABASE_URL` работает на in-memory хранилище.

## Быстрый старт

```bash
cp .env.example .env          # задать ADMIN_EMAIL / ADMIN_PASSWORD
docker compose up -d --build
# http://localhost:3000 — веб, http://localhost:8082 — API
```

Миграции применяются автоматически при старте API.

## Тесты и QA

```bash
cd backend && go test ./...
pwsh -File qa-abc.ps1         # 28 проверок сквозного сценария
```

## Спецификации

Разработано по SDD-процессу (Spec Kit):

- `.specify/memory/constitution.md` — конституция проекта
- `specs/001-it-courses-mvp/` — spec, plan, data-model, tasks, quickstart, converge-report
- `specs/001-it-courses-mvp/contracts/openapi.yaml` — контракт API

## Окружение

Основные переменные (см. `.env.example`): `DATABASE_URL`, `ADMIN_EMAIL`, `ADMIN_PASSWORD`, `RUNNER_URL`, `PAYMENTS_PROVIDER`, `PLATFORM_COMMISSION_PCT`.
