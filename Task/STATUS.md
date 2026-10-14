# Task/STATUS — ключи сессий агентов

Каждая задача = ключ сессии. Ключ нужен для продолжения работы
конкретным агентом (`sessionID`) и для отслеживания, кто что сделал.

## Волна 1 — запущена

| Ключ сессии | Задача | Worktree | Ветка | Session ID | Статус |
|---|---|---|---|---|---|
| `T01-api-client-contract` | Контракт API-клиента | `../wt-T01-api-client-contract` | `task/T01-api-client-contract` | `ses_ec3cf8177ffeIHA35StIW9HilK` | 🔄 в работе |
| `T02-i18n-contract` | Словарь и типы ключей | `../wt-T02-i18n-contract` | `task/T02-i18n-contract` | `ses_ec3cf8170ffeJctWWn443BCxLZ` | 🔄 в работе |
| `T07-admin-us6-backend` | US6 backend (журнал, комиссия) | `../wt-T07-admin-us6-backend` | `task/T07-admin-us6-backend` | `ses_ec3cccbd4ffeM8Io5nlWhYCZCH` | 🔄 в работе |
| `T09-payments-providers` | Stripe, YooKassa, подписи | `../wt-T09-payments-providers` | `task/T09-payments-providers` | `ses_ec3cccbd1ffe3v8XWcVatQ2lnQ` | 🔄 в работе |
| `T10-auth-security` | OAuth, cookie, rate-limit | `../wt-T10-auth-security` | `task/T10-auth-security` | `ses_ec3ca8f63ffejNIndhuvCsgwUf` | 🔄 в работе |
| `T11-runner-sandbox` | Песочница runner | `../wt-T11-runner-sandbox` | `task/T11-runner-sandbox` | `ses_ec3ca8f5affedrpc8wAoe2M43W` | 🔄 в работе |

## Волна 2 — ожидает T01 и T02

| Ключ сессии | Задача | Зависит от |
|---|---|---|
| `T03-learn-experience` | Экран обучения, просмотр урока | T01, T02 |
| `T04-catalog-reviews` | Каталог, страница курса, отзывы | T01, T02 |
| `T05-routing-guards` | Роутинг, гварды, 404, error boundary | T01, T02 |
| `T06-constructor-editor` | Конструктор курса: редактирование | T01, T02 |
| `T08-admin-us6-frontend` | Админ-панель US6 | T01, T02, T07 |

## Волна 3 — ожидает все

| Ключ сессии | Задача | Зависит от |
|---|---|---|
| `T12-contract-tests` | Контрактные тесты + qa-abc.ps1 | T03…T11 |
| `T13-integration-wiring` | Проводка маршрутов, compose | T07, T09, T10, T11 |
| `T14-docs-cleanup` | README, env, документация | все |

---

## Инфраструктура окружения

| Компонент | Статус |
|---|---|
| Go на хосте | ❌ нет — **сборка и тесты идут в Docker** |
| Docker | ✅ 28.3.2, linux/x86_64 |
| Node / npm | ✅ установлены, `npm ci` прогнан в `edu-platform/frontend` |
| Python | ✅ 3.14 (не нужен для задач) |
| `gh` CLI | ❌ нет — push потребует кредов git |

Образы Docker (уже скачаны):
`golang:1.24-alpine`, `golang:1.24-bookworm`, `debian:bookworm-slim`,
`node:20-alpine`, `nginx:1.27-alpine`, `postgres:16`

Volume `gomodcache` — кеш Go-модулей, чтобы не качать их на каждый запуск.

### Команды проверки

Go-бэкенд:
```
docker run --rm -v "<worktree>\backend:/src" -v gomodcache:/go/pkg/mod -w /src -e GOFLAGS=-mod=mod golang:1.24-alpine sh -c "go build ./... && go vet ./... && go test ./... -count=1 && echo ALL_OK"
```

Go-runner:
```
docker run --rm -v "<worktree>\runner:/src" -v gomodcache:/go/pkg/mod -w /src -e GOFLAGS=-mod=mod golang:1.24-bookworm sh -c "go build ./... && go vet ./... && go test ./... -count=1 && echo ALL_OK"
```

Frontend:
```
cd <worktree>\frontend
npm ci
npx tsc --noEmit
npm run build
```

---

## Договорённости

- Каждый агент работает **только в своём worktree** и **только со своими файлами**.
- `backend/internal/handler/middleware.go` в волне 1 правит **только T10**;
  новые маршруты остальные регистрируют экспортируемым методом,
  T13 подключает их в волне 3.
- `docker-compose.yml` в волне 1 не трогает никто — готовый YAML
  отдаёт T13 в отчёте T11.
- `frontend/src/i18n/ru.json` и `frontend/src/api/client.ts` — зона T02 и T01,
  остальные второволновые задачи только читают их результат.
