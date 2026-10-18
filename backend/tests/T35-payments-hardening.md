# T35 — отчёт задачи: негативные и конкурентные тесты платёжного контура (backend)

**Волна:** P4 · **Ветка:** `task/T35-payments-hardening` · **Worktree:** `wt/t35`
**Файл тестов:** `backend/tests/contract/payments_hardening_test.go` (новый, пакет `contract_test`)
**Контракты:** `Task/00-api-contract.md` §16 + `Task/T09-payments-providers.md`

*(Сессия агента T35 дважды умерла на инфраструктурной ошибке провайдера (HTTP 500); тесты, диагноз бага и отчёт — его работа, фикс, прогоны и фиксацию довёл координатор.)*

## Что проверяется

Тесты бьют по реальному пути: HTTP-слой (`newTestApp` / `newTestAppWithCfg`,
помощники `helper_test.go`), `handler/enroll.go` (`CreateEnrollment` и вебхук),
`internal/payments/{mock,stripe,yookassa}.go`, CORS-middleware `handler/middleware.go`.
Существующие файлы (`payments_test.go`, `auth_hardening_test.go`) не менялись.

| Проверка | Ожидание | Факт |
|---|---|---|
| `TestPaymentsHardeningForgedSignatureRejected` (4 подслучая: подпись не совпадает, заголовка нет, подпись от другого тела, подпись не hex) | не `2xx`, платёж не меняется, записи не появляются | PASS (`400`; статус и записи не тронуты) |
| `TestPaymentsHardeningWebhookDuplicateDelivery` | повторная доставка → ровно один платёж, одна запись, те же суммы | PASS |
| `TestPaymentsHardeningConcurrentWebhookRace` (8 параллельных вебхуков на один `providerPaymentId`) | ровно один платёж и одна запись, без гонок | PASS (в том числе под `-race`) |
| `TestPaymentsHardeningUnknownPaymentAndProvider` | неизвестный платёж → `404`, чужой провайдер → `400` | PASS |
| `TestPaymentsHardeningPreflightCORS` | `OPTIONS` с разрешённым origin → `204` + `ACAO`/`ACAM`/`ACAH`; с чужим origin — без `ACAO` | PASS |
| `TestPaymentsHardeningFreeCourseRepurchaseAfterRefund` | повторная покупка бесплатного курса после возврата → `201`, тот же платёж, доступ активен | PASS (после фикса, см. ниже) |

## Найденные баги и правки

**Баг: паника → `500` при повторной покупке бесплатного курса после возврата средств.**
`providerPaymentId` бесплатного платежа детерминирован (`free-<ученик>-<курс>`),
поэтому вторая покупка после возврата снова доходила до `CreatePayment`, который
возвращал `(nil, ErrAlreadyExists)`; обработчик продолжал с `p == nil` и падал
на `p.ID` → `500` с пустым телом. Наблюдение из первого прогона:
`POST /api/v1/enrollments → 500 0B`.

**Фикс** — `internal/handler/enroll.go`, минимальный: при `ErrAlreadyExists`
обработчик находит уже выданный бесплатный платёж (`freePaymentByCourse` через
`ListPaymentsByStudent`, поиск по ключу `free-<ученик>-<курс>`), переиспользует
его, возвращает в `succeeded` (`SetPaymentStatus`) и открывает доступ
(`EnsureEnrollment`); если создание упало по другой причине или платёж не найден —
прежний ответ `500`. Новых полей и сущностей не появилось, платные пути не тронуты.

## Полный прогон (Docker, `golang:1.24-alpine`)

```
ok  edu-platform/backend/cmd/api             5.088s
ok  edu-platform/backend/internal/auth       0.344s
ok  edu-platform/backend/internal/handler    3.341s
ok  edu-platform/backend/internal/payments   0.386s
ok  edu-platform/backend/internal/repo       0.016s
ok  edu-platform/backend/tests/contract     42.501s
T35_TESTS_EXIT=0
```

## Вывод `-race` (Docker, `golang:1.24-bookworm`, только hardening)

```
--- PASS: TestPaymentsHardeningForgedSignatureRejected (7.02s)
--- PASS: TestPaymentsHardeningWebhookDuplicateDelivery (8.16s)
--- PASS: TestPaymentsHardeningConcurrentWebhookRace (10.08s)
--- PASS: TestPaymentsHardeningUnknownPaymentAndProvider (13.01s)
--- PASS: TestPaymentsHardeningPreflightCORS (0.00s)
--- PASS: TestPaymentsHardeningFreeCourseRepurchaseAfterRefund (10.67s)
ok  edu-platform/backend/tests/contract  49.999s
T35_RACE_EXIT=0
```

## Что не проверено

- **Реальные провайдеры** (Stripe/YooKassa): подписи и тела проверены на подставном
  mock-провайдере, сетевых обращений к внешним API нет.
- **Postgres-путь**: все проверки — на in-memory store; повторная покупка после
  возврата на живом Postgres отдельным тестом не покрыта (SQL-путь читался глазами,
  поведение должно совпадать, но это не доказано прогоном).
- Гонка при одновременных **разных** вебхуках на один платёж (не дубликатах) не эмулировалась.
- Реальный браузер и живой стенд в этой задаче не участвовали.
