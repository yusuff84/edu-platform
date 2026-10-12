# Research: IT-курсы MVP

**Дата**: 2026-10-06
**Фича**: specs/001-it-courses-mvp/spec.md

## R1. Go HTTP-фреймворк

- **Decision**: Go 1.22+ std `net/http` + `chi` для роутинга, `pgx` для Postgres, `golang-migrate` для миграций.
- **Rationale**: std в 1.22 закрывает большинство нужд, chi — тонкий роутер без магии, pgx — стандарт де-факто для Postgres. Минимум зависимостей = проще Docker и поддержка.
- **Alternatives considered**: Gin (тяжелее, своя валидация), Echo (похож на Gin), чистый std без роутера (много бойлерплейта для групп /api/v1 + middleware).

## R2. Frontend-сборка

- **Decision**: React 18 + TypeScript + Vite, React Router, TanStack Query, React Hook Form + zod. Без Next.js в MVP.
- **Rationale**: SPA достаточно для кабинетов/каталога, деплой — статика через Nginx в Docker. TanStack Query убирает кэш-ручники для каталога/прогресса. SSR не нужен новичкам в MVP.
- **Alternatives considered**: Next.js (SSR/ISR избыточен, усложняет Docker), CRA (deprecated), Remix (меньше кадров на рынке).

## R3. БД и файлы

- **Decision**: PostgreSQL 16 основная БД. Файлы MVP: обложки — локальный volume `/uploads` за Nginx, видео — внешняя ссылка (YouTube/RuTube/файл), без транскодинга.
- **Rationale**: Курсы/платежи/прогресс — реляционные, нужны транзакции запись+платеж. Локальный volume проще S3 в MVP, интерфейс `ObjectStore` позволит переехать на S3 позже.
- **Alternatives considered**: SQLite (не держит конкурентную запись платежей), Mongo (теряем транзакции и JOIN прогресса), сразу S3 (лишняя инфра для MVP).

## R4. Auth: сессии vs JWT

- **Decision**: Email+пароль (argon2), stateful-сессии в httpOnly cookie + CSRF, refresh-ротация. Яндекс OAuth — позже отдельным адаптером `AuthProvider`.
- **Rationale**: Сессии проще отзывать при смене роли/блокировке, безопаснее для новичков (нет токенов в localStorage). OAuth позже не ломает ядро.
- **Alternatives considered**: Чистый JWT в localStorage (XSS-риск), opaque + Redis (лишний сервис в MVP, сессии можно в Postgres).

## R5. Гибкие платежи

- **Decision**: Домен `PaymentProvider` интерфейс: `CreatePayment`, `HandleWebhook`, `Refund`, `GetStatus`. В MVP: `MockProvider` (для e2e) + один реальный адаптер `Stripe test` (или ЮKassa — выбор одной строкой в `.env`: `PAYMENTS_PROVIDER=mock|stripe|yookassa`). Webhook идемпотентен по `provider_payment_id` (unique). Split учитель/платформа считается в домене, не в провайдере.
- **Rationale**: Выполняет требование "максимально гибко": новый провайдер = новый файл адаптера + регистрация в фабрике, ядро не меняется. Идемпотентность закрывает двойные webhook.
- **Alternatives considered**: Жесткая привязка к одному SDK в хендлерах (нарушает конституцию IV), вынос в отдельный биллинг-сервис (оверкилл для MVP).

## R6. Автопроверка кода

- **Decision**: В MVP — изолированный `code-runner` как отдельный Docker-сервис (Go, очередь в памяти + воркеры, таймаут 5с, memory/CPU limits, без сети). Python 3.12 + Node 20 + HTML/CSS-проверка (DOM-assert без браузера: parse + проверки тегов/селекторов). Квизы проверяются в API без раннера.
- **Rationale**: Judge0 — лишний внешний сервис для MVP; свой минимальный раннер проще, аудит безопасности (no-net, timeout) достаточен для учебных задач. HTML/CSS не нужен браузер в MVP.
- **Alternatives considered**: Judge0 self-hosted (тяжелый, но путь миграции сохранен через интерфейс `CodeRunner`), WASM в браузере (небезопасно для скрытых тестов), ручная проверка учителем (не масштабируется).

## R7. Live-уроки

- **Decision**: В MVP — поле `live_url` + `starts_at`, кнопка "Перейти к встрече", напоминание. Без встроенного WebRTC.
- **Rationale**: Соответствует решению пользователя (ссылкой). Jitsi/WebRTC — после ядра.
- **Alternatives considered**: Jitsi-embed (модерация, нагрузка, запись — вне MVP).

## Итоговые риски

- Двойные webhook → unique constraint + `processed_at`.
- Бесконечные циклы → timeout + kill контейнера-задачи.
- Удаление курса с учениками → только архив, FK restrict.
