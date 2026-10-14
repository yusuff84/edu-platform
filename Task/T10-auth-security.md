# T10 — Безопасность аутентификации: Yandex OAuth, cookie, rate-limit

**Ключ сессии:** `T10-auth-security`
**Волна:** 1 · **Слой:** backend (Go)
**Приоритет:** P2 — `converge-report.md` подтверждает, что T051 (rate-limit) не выполнен

## Проблема

1. `backend/internal/auth/yandex.go` — **пустой интерфейс без реализации**
   (`TODO: реализовать обмен code->token и получение профиля`).
2. `config.Load()` читает `SESSION_SECRET` (`config.go:13,55`), compose его передаёт,
   но **он нигде не используется**: сессия — голый UUID в cookie без подписи
   (`middleware.go:128-138`). Подмена значения cookie = подмена пользователя,
   если UUID угадать/перехватить.
3. Cookie выставляется **без флага `Secure`** (`middleware.go:130-137`).
4. **Rate-limit отсутствует полностью.** `POST /auth/login` перебирается без ограничений.
5. CSRF-защита минимальна: проверка Origin/Referer только для не-GET
   (`middleware.go:173-178`), причём `originOf` пропускает запрос, если заголовка нет вовсе.
6. `tasks.md:39` (T009) обещал argon2id — фактически bcrypt,
   что зафиксировано в `handler/auth.go:14-16`. Задокументировано, менять не нужно.

## Владение файлами (жёсткая граница)

```
backend/internal/auth/yandex.go
backend/internal/auth/oauth.go          — можно создать
backend/internal/handler/auth.go
backend/internal/handler/middleware.go
backend/internal/config/config.go
```

## Что сделать

### 1. Rate-limit (`middleware.go`)

Реализуй `rateLimit(max int, window time.Duration)` middleware на
`token bucket` в памяти (без новых зависимостей), с периодической
очисткой карты, чтобы не течь по памяти. Повесь:
- на `/auth/login` и `/auth/register` — жёстче (например 10/мин на IP);
- на `/payments/webhook/{provider}` — отдельный, более высокий порог;
- глобально на `/api/v1` — мягкий (например 300/мин), чтобы защититься от флуда.

Ответ при превышении: `429` + заголовок `Retry-After` + поле `error` по-русски.

### 2. Cookie и подпись сессии (`middleware.go`, `config.go`)

- Сделай `SESSION_SECRET` рабочим: значение cookie должно быть
  `<sessionID>.<base64url(HMAC-SHA256(secret, sessionID))>`,
  проверка — `hmac.Equal` (защита от тайминг-атак).
- Если `SESSION_SECRET` пустой или равен дефолту `change-me` —
  в dev-режиме (`APP_ENV=development` или отсутствии `DATABASE_URL`) пиши
  предупреждение в лог и **не** подписывай; в остальных случаях —
  отказывайся стартовать. Не сломай `docker compose up` из `.env.example`.
- Добавь флаг `Secure` на cookie, когда запрос пришёл по HTTPS
  (`r.TLS != nil` или `X-Forwarded-Proto: https`).
- Сделай `SameSite` настраиваемым через конфиг.

### 3. CSRF (`middleware.go`)

- Ужесточи `originOf`: если на не-GET запросе нет **ни** `Origin`, **ни** `Referer`,
  и запрос не является заведомо безопасным (`multipart/form-data`, bearer-заголовок) —
  отклоняй с `403`. Не сломай `curl`-сценарии из `qa-abc.ps1` (T12) —
  опиши в отчёке, как их адаптировать, либо добавь env-флаг `CSRF_ALLOW_MISSING_ORIGIN=true`
  для локальных скриптов.

### 4. Yandex OAuth (`auth/yandex.go`, новый `auth/oauth.go`)

Реализуй полностью, без внешних SDK:
- `ExchangeCode(ctx, code) (accessToken string, err error)` —
  `POST https://oauth.yandex.ru/token` (`grant_type=authorization_code`,
  `client_id`, `client_secret`, `code`), ответ JSON `{ access_token, token_type, expires_in }`;
- `FetchProfile(ctx, accessToken) (Profile, error)` —
  `GET https://login.yandex.ru/info?format=json` с заголовком `Authorization: OAuth <token>`,
  разобрать `default_email`, `display_name`, `id`.
- Тип ошибок — свой, с понятным русским текстом; таймауты на обоих запросах.
- Добавь в `config`: `YANDEX_CLIENT_ID`, `YANDEX_CLIENT_SECRET`, `YANDEX_REDIRECT_URI`.
- **Регистрацию/логин через OAuth в хендлере не добавляй** —
  добавь только клиентскую часть и опиши в отчёте, какой эндпоинт нужен
  (`GET /auth/yandex/start` → редирект, `GET /auth/yandex/callback` → сессия).
  Если решишь добавить хендлеры — опиши точные координаты для T13,
  но сам маршрут в `middleware.go` не регистрируй.

### 5. Прочее

- Проверь, что `decodeJSON` с `DisallowUnknownFields` даёт понятное сообщение
  пользователю, а не «json: unknown field».
- Убедись, что сообщения об ошибках не раскрывают внутренности
  (не возвращай наружу текст SQL-ошибок).

## Критерии приёмки

- [ ] `cd backend && go build ./...` без ошибок
- [ ] `cd backend && go vet ./...` чисто
- [ ] `cd backend && go test ./...` — существующие 6 тестов проходят
- [ ] Есть тесты: rate-limit срабатывает и возвращает `429` + `Retry-After`;
      подпись cookie проверяется; подделанная подпись отклоняется
- [ ] Подделанное значение cookie приводит к `401`, а не к доступу
- [ ] `SESSION_SECRET` реально используется в коде
- [ ] `docker compose up` из `.env.example` по-прежнему поднимается

## Проверка

```bash
cd backend
go build ./... && go vet ./... && go test ./...
```

## Отчёт

Опиши: формат cookie, алгоритм подписи, параметры rate-limit для каждого
префикса маршрутов, env-переменные Yandex OAuth, и точные координаты
(file:line) для T13, если нужны новые маршруты.
