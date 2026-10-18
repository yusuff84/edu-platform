# T35 — Платёжный контур: негативные и конкурентные тесты (backend)

**Волна:** P4 · **Ветка:** `task/T35-payments-hardening` · **Worktree:** `wt/t35`
**Контракт:** `Task/00-api-contract.md` §16 (`POST /payments/webhook/{provider}`) + `Task/T09-payments-providers.md`.

## Зачем

Деньги — самый дорогой участок. Сейчас не проверено автотестами: подделка подписи вебхука,
повторная доставка, гонка на одном `providerPaymentId`, preflight `OPTIONS`. Если что-то из этого
сломано — на живом стенде это выглядит как «пользователь оплатил, но записи нет» или наоборот.

## Что сделать

Новый файл `backend/tests/contract/payments_hardening_test.go` (пакет `contract_test`,
помощники из `helper_test.go`, образцы — `backend/tests/contract/payments_test.go`,
`backend/internal/handler/payments_test.go`; сначала прочитать реализацию
`backend/internal/handler/payments*.go` и `internal/payments/**`, чтобы тесты били по реальному коду).

Покрыть:

1. **Подделка подписи.** Успешный вебхук с неправильной подписью → не `2xx` (`400`/`401`/`403` —
   зафиксировать фактический код в отчёте), статус платежа не изменился, записи на курс не появились.
   Отдельно: вебхук **без** заголовка подписи и с подписью от другого тела.
2. **Повторная доставка.** Тот же успешный вебхук дважды → ровно один платёж, ровно одна запись
   на курс, суммы/комиссия не удвоились; второй ответ — идемпотентный `2xx`.
3. **Гонка.** 8 параллельных одинаковых вебхуков (`sync.WaitGroup` + `httptest`) → ровно один
   платёж и одна запись; если найдена гонка — починить минимально (мьютекс/атомарная вставка
   с уникальным ключом) и описать правку.
4. **Неизвестный платёж.** Вебхук со ссылкой на несуществующий платёж → `404`;
   провайдер из URL, которого нет в конфиге → `400`.
5. **Preflight.** `OPTIONS` на `/api/v1/auth/me` с `Origin` из разрешённых и
   `Access-Control-Request-Method: POST` → `204` (или `200`, зафиксировать факт) и заголовки
   `Access-Control-Allow-Origin` = этот origin, `Access-Control-Allow-Methods` содержит `POST`,
   `Access-Control-Allow-Headers` содержит `Content-Type`; `OPTIONS` с неразрешённым origin →
   без `Access-Control-Allow-Origin`. Тест собирается через `newTestAppWithCfg` с
   `AllowedOrigins: []string{"http://localhost:3000"}` (см. `auth_hardening_test.go`).

Найденные баги чинить **минимально** и только в платёжном контуре; всё, что вне скоупа, — в отчёт.

## Ограничения

- Править только: `backend/tests/contract/payments_hardening_test.go` (новый), при необходимости
  `backend/internal/handler/payments*.go` / `backend/internal/payments/*.go` / `backend/internal/repo/store.go`
  (`CreatePayment`-путь) — только по факту найденного бага, с объяснением в отчёте,
  отчёт `backend/tests/T35-payments-hardening.md`.
- Существующие тесты (`payments_test.go`, `auth_hardening_test.go`) не переписывать: если нужен
  образец — читать, не менять.
- `Task/**`, `frontend/**`, `specs/**` — не трогать. Новых зависимостей нет. По-русски.

## Проверка (Docker)

```powershell
docker run --rm -v "C:\Users\96352\all_project\eduPlatform\edu-platform\backend:/src" -v gomodcache:/go/pkg/mod -w /src -e GOFLAGS=-mod=mod golang:1.24-alpine sh -c "go build ./... && go vet ./... && go test ./... -count=1"
```

Дополнительно прогнать свой пакет под `-race` (`-race` требует gcc → образ `golang:1.24-bookworm`,
как в примере ниже) и привести вывод:

```powershell
docker run --rm -v "C:\Users\96352\all_project\eduPlatform\edu-platform\backend:/src" -v gomodcache:/go/pkg/mod -w /src -e GOFLAGS=-mod=mod golang:1.24-bookworm sh -c "go test ./tests/contract/ -run 'Hardening|Webhook|Preflight' -race -count=1 -v"
```

## Отчёт

`backend/tests/T35-payments-hardening.md`: таблица «проверка → ожидание → факт», найденные баги и
правки, вывод `-race`, честный список непроверенного (например: поведение реальных провайдеров).
