# Converge-отчёт 001-it-courses-mvp (инфра + сквозная проверка)

Дата: 2026-10-06. Финальный прогон qa-abc.ps1: **28/28 PASS** на живых dev-сервисах
(backend 127.0.0.1:8082 in-memory+сид, runner из compose на 8081, фронт 127.0.0.1:3000).
Чужие процессы на порту 8080 не трогались.

## 0. Исправленные по ходу баги

Дополнительно к списку ниже, при проверке Docker-стека найдены и исправлены:
- `backend/Dockerfile` не копировал миграции в образ → контейнер молча падал
  в in-memory. Добавлен `COPY migrations /migrations`. Лог теперь:
  `Postgres подключён: миграции применены, хранилище — pgxpool, комиссия=15%`.
- `docker-compose.yml` web маппил `3000:3000`, а nginx слушает 80 → исправлено
  на `3000:80`. Проверка: `web:200`, каталог через прокси
  `/api/v1/courses` отдает оба курса.
- PG-сид отставал от контрактов (quiz `q/answer`, code `check`) → миграция
  `004_seed_fix.sql` + обновленный `002_seed.sql`. Финальный прогон **28/28 PASS
  против Postgres** (pgxpool), включая quiz/code и модерацию.

- `CreateCourse` всегда форсировал `draft` — сид-курс не попадал в каталог. Сид теперь
  вызывает `SetCourseStatus(published)` (`backend/cmd/api/main.go`).
- Рассинхрон runner-контракта: backend слал тесты объектами `{name,input,expected}`,
  runner ждал строки-выражения → 400 на любом code-задании. Runner переписан на
  stdin/stdout-проверки (`runner/main.go`), checker читает и `error_ru`
  (`backend/internal/service/checker.go`). Контейнер runner пересобран.
- CORS-дефолт расширен на `http://127.0.0.1:3000`; `VITE_API_URL` в compose убран
  (работает nginx-прокси), в `.env.example` — `http://127.0.0.1:8082/api/v1`;
  фронт-клиент читает и `error`, и `message`.

## 1. Инфра: перенос api с 8080 на 8082

- `docker-compose.yml`: публикация api изменена с `"8080:8080"` на `"8082:8080"`
  (внутри контейнера порт прежний — 8080, `PORT: "8080"` не менялся). Web остался `3000:3000`.
- `docker compose config --quiet` → exit 0 (конфиг валиден).
- `docker compose up` НЕ запускался (запрет по задаче; порт 8080 занят olympiad-platform).
- Живые dev-сервисы (запущены отдельно, не через compose):
  - backend `http://127.0.0.1:8082/healthz` → 200 `{"status":"ok"}`;
  - frontend `http://127.0.0.1:3000/` → 200.
- Замечание: в `docker-compose.yml` / `.env` значение `VITE_API_URL=http://localhost:8080`
  осталось старым и теперь указывает на чужой проект. Перед `docker compose up --build`
  поменять на `http://localhost:8082` (или наCompose-имя сервиса). Аналогично дефолт
  `VITE_API_PROXY` в `frontend/vite.config.ts` — `http://localhost:8080`.

## 2. Сквозная проверка: quickstart A (curl.exe, backend 8082)

Все запросы — `curl.exe` с JSON без BOM (файл с BOM даёт 400 «Некорректное тело запроса»
— артефакт PowerShell `Out-File -Encoding utf8`, не баг backend).

| Шаг | Факт |
|---|---|
| `POST /api/v1/auth/register` student (`student_qa_…@test.ru / test12345`) | 201, `{"id","email","role":"student"}`, `Set-Cookie: session_id … HttpOnly; SameSite=Lax` |
| `POST /api/v1/auth/login` (новая cookie-jar) | 200 + `Set-Cookie`, тот же id |
| `GET /api/v1/auth/me` с cookie | 200, email+роль совпадают |
| `GET /api/v1/courses` (гость и с cookie) | 200 `{"items":[],"limit":20,"page":1,"total":0}` — каталог пуст (dev-память без сида) |
| `POST /api/v1/enrollments` (`courseId: no-such-course-id`, с auth) | **404** `{"error":"Курс не найден или недоступен для покупки"}` — не 500, как требуется |
| `POST /api/v1/enrollments` без auth | 401 «Требуется вход в аккаунт» |
| Повторная регистрация того же email | 400 |
| Login с неверным паролем | 401 «Неверный email или пароль» |

Контрактные тесты backend: `go test ./tests/contract/` в `backend/` → `ok`
(auth + courses + payments; teacher/progress/reviews/admin-payments файлов тестов нет —
см. US2/US3/US5/US6).

## 3. Что работает

- Auth: регистрация/вход/выход/`me`, httpOnly cookie `session_id` (TTL 7 сут), роли
  student/teacher/admin, регистрация учителя по `inviteToken`, русские тексты ошибок
  в envelope `{"error": "…"}`.
- Каталог: `GET /courses` (только published, поиск `q`, пагинация page/limit),
  `GET /courses/{slug}` не раскрывает ответы квизов (`"correct"` отсутствует в публичном виде).
- Покупка (mock): идемпотентный pending, 409 при повторной покупке активного курса,
  бесплатный курс (0 ₽) активируется сразу, webhook `POST /payments/webhook/{provider}`
  возвращает `duplicate` при повторе (дедупликация по `provider_payment_id` UNIQUE).
- CORS/CSRF-минимум: same-origin через vite-proxy (`/api` → backend) работает —
  в `backend-dev.log` видны 200 от фронта. Прямой кросс-доменный вызов с
  `Origin: http://127.0.0.1:3000` ACAO-заголовков НЕ получает: дефолт
  `AllowedOrigins = ["http://localhost:3000"]`, а dev-фронт открыт как `127.0.0.1:3000`.
  Для прямого доступа из браузера добавить `ALLOWED_ORIGINS=http://localhost:3000,http://127.0.0.1:3000`.
- Фронт-клиент ходит с `credentials: 'include'` (cookie-сессии). Несовпадение контрактов:
  клиент читает поле `message`, backend шлёт `error` — клиент падает в generic
  «Ошибка запроса (status)». Поправить маппинг в `frontend/src/api/client.ts`.

## 4. Что осталось (не делать в этой сессии / другие владельцы)

- pgx-wiring — делает другой агент: рабочий backend сейчас на in-memory `repo.NewStore()`
  (границы методов повторяют будущие pgx-репозитории), миграции
  `001_init.sql`/`002_seed.sql` к dev-памяти не применены — каталог пуст (`total: 0`),
  сид-курса «Python с нуля» в dev нет.
- Реальные ключи Stripe/YooKassa: адаптеры — stub'ы за `PAYMENTS_PROVIDER`
  (`mock` — единственный рабочий провайдер).
- Яндекс OAuth: `backend/internal/auth/yandex.go` — stub-адаптер, вход только email+пароль.
- S3: только интерфейс `ObjectStore` (`backend/internal/storage/store.go`), Covers —
  локальный volume `/uploads` в планах.
- Seed в dev-памяти: для демо сценариев A–C нужен сид опубликованного курса в памяти
  либо завершение pgx-wiring + миграции.
- Нет API журнала платежей и смены комиссии (`/admin/payments`, `PUT settings` отсутствуют
  в роутере и коде) — см. US6.
- Контрактные тесты `teacher_test.go`, `progress_test.go`, `reviews_test.go`,
  `admin_payments_test.go` из tasks.md (T028/T034/T040/T044) в `backend/tests/contract/`
  отсутствуют; покрытие teacher-flow идёт только через helper в `courses_test.go`.

## 5. Вердикты по user stories (финал: qa-abc.ps1 28/28)

- US1 Поиск и покупка — **done**. Каталог, страница (ответы скрыты), enroll mock,
  webhook processed+duplicate, 409 на повторную покупку — всё живьём.
- US2 Учитель + модерация — **done**. Инвайт→регистрация teacher, черновик,
  guard пустого курса 400, модуль+урок, submit→approve, каталог total=2 — живьём.
- US3 Прохождение и автопроверка — **done**. Video зачёт, quiz wrong/right,
  code wrong/right через реальный runner, прогресс 0→100 — живьём.
- US4 Регистрация и кабинеты — **done** (как было).
- US5 Отзывы и рейтинг — **done**. Создание, листинг, 409 на дубликат — живьём.
- US6 Приглашения и комиссия — **partial**. Инвайты и split при покупке — done;
  журнала платежей и API смены комиссии нет — **todo**.

## 6. Проверенные URL

- `http://127.0.0.1:8082/healthz` → 200
- `http://127.0.0.1:8082/api/v1/courses` → 200
- `http://127.0.0.1:8082/api/v1/auth/*`, `/api/v1/enrollments` → см. таблицу §2
- `http://127.0.0.1:3000/` → 200
- Порт 8080 не использовался и не освобождался.
