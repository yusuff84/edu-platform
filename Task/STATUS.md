# Task/STATUS — ключи сессий агентов

Каждая задача = ключ сессии. Ключ нужен для продолжения работы
конкретным агентом (`sessionID`) и для отслеживания, кто что сделал.

## Сводная ведомость

| Ключ сессии | Задача | Волна | Статус | Коммит |
|---|---|---|---|---|
| `T01-api-client-contract` | Контракт API-клиента | 1 | ✅ смержен | `390b87a` |
| `T02-i18n-contract` | Словарь и типы ключей | 1 | ✅ смержен | `63d1bb4` |
| `T09-payments-providers` | Stripe, YooKassa, подписи | 1 | ✅ смержен | `d25706e` |
| `T11-runner-sandbox` | Песочница runner | 1 | ✅ смержен | `2ae7ab4` |
| `T10-auth-security` | OAuth, cookie, rate-limit | 1 | ✅ смержен | `771dfc1` |
| `T07-admin-us6-backend` | US6 backend: журнал, комиссия | 1 | ✅ смержен | `65e548e` |
| `T03-learn-experience` | Экран обучения, просмотр урока | 2 | ✅ смержен | `8d70b8d` |
| `T04-catalog-reviews` | Каталог, курс, отзывы | 2 | ✅ смержен | `4c28016` |
| `T05-routing-guards` | Роутинг, гварды, 404 | 2 | ✅ смержен | `d3408dc` |
| `T06-constructor-editor` | Конструктор курса | 2 | ✅ смержен | `00bb9db` |
| `T08-admin-us6-frontend` | Админ-панель US6 | 2 | ✅ смержен | `2132da3` |
| `T15-auth-flow-redirect` | Редирект после входа | 2 | ✅ смержен | `3d5d5ec` |
| `T14-docs-cleanup` | README, env, `docs/API.md` | 3 | ✅ смержен | `efe7fc8` |
| `T13-integration-wiring` | Проводка маршрутов, compose | 3 | 🔄 в работе | — |
| `T12-contract-tests` | Контрактные тесты + `qa-abc.ps1` | 3 | ⏸ ждёт T13 | — |

## Ключи сессий активных агентов

| Задача | Session ID | Worktree |
|---|---|---|
| `T13-integration-wiring` | `ses_ec378cc65ffe05LDvP2i0mrgHn` | `../wt-T13-integration-wiring` |
| `T14-docs-cleanup` | `ses_ec3773bb0ffeupzpYUChJtheJD` | `../wt-T14-docs-cleanup` |

T12 запускается после смержения T13 — его контрактные тесты проверяют
маршруты, которые регистрирует именно T13.

## Проверенное состояние интеграционной ветки

На коммите `65e534d` (после смержения всех задач волн 1–2):

| Проверка | Результат |
|---|---|
| `cd backend && go build ./...` | ✅ |
| `cd backend && go vet ./...` | ✅ |
| `cd backend && go test ./... -count=1` | ✅ `ok internal/payments`, `ok tests/contract 2.200s` |
| `cd frontend && npx tsc --noEmit` | ✅ **0 ошибок** |
| `cd frontend && npx vite build` | ✅ 413.67 КБ JS, 12.14 КБ CSS |
| `cd runner && go test ./...` | ✅ `ok runner 1.600s` |

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

## Базовая линия `tsc` — компилятор как список задач

После смержения T01 и T02 `npx tsc --noEmit` в `frontend/` даёт **41 ошибку**
(43 строки вывода, минус 2 строки-продолжения ошибки `map`). Все они — в файлах
второй волны. Компилятор буквально перечисляет оставшуюся работу: каждая ошибка —
это поле, которого не существует в реальном контракте API.

| Файл | Ошибок | Владелец |
|---|---|---|
| `LessonView.tsx` | 15 | T03 |
| `MyCourses.tsx` | 10 | T05 |
| `Reviews.tsx` | 5 | T04 |
| `Course.tsx` | 4 | T04 |
| `Learn.tsx` | 4 | T03 |
| `CourseCard.tsx` | 2 | T04 |
| `Catalog.tsx` | 1 | T04 |
| `Constructor.tsx` | 0 (переписывается целиком) | T06 |
| `Admin.tsx` | 0 (типам не ломается, но рендерит заглушку) | T08 |

Итого T03 — 19, T04 — 12, T05 — 10, T06 — 0, T08 — 0.
Задача T08 не видна компилятору: `queue.data?.length` типобезопасен,
но в рантайме рендерит пустую заглушку вместо журнала платежей.
Каждая задача второй волна должна свести ошибок в своих файлах к нулю;
чужие файлы не трогать, иначе конфликт при слиянии.

## Найденные расхождения с контрактом

Полная версия — в `Task/00-api-contract.md`, раздел «Расхождения, найденные агентами».

| # | Расхождение | Статус |
|---|---|---|
| 1 | `User.displayName` обещан контрактом, `handler/auth.go` не отдаёт | ✅ закрыто T10 — добавлен в `Register`/`Login`/`Me` |
| 2 | `POST /auth/logout` возвращает `200 {"status":"ok"}`, а не `204` | ✅ решено оставить `200` — существующий тест `tests/contract/auth_test.go:44` требует `StatusOK`. **Контракт нужно поправить на 200** (T14) |
| 3 | `listReviews` теряет `avgRating`/`ratingsCount` | ✅ учтено — T04 берёт рейтинг из `getCourse()` |
| 4 | `POST .../lessons` принимает `startsAt`, но **не сохраняет** его в урок | ❌ открыт — подтверждено T03 и T06. Бэкенд-баг, файл `handler/teacher.go` у T13 |
| 5 | `GET /admin/payments` не отдаёт email/имя студента — только `studentId` | ❌ открыт — T08 показывает укороченный UUID. Требует расширения контракта |
| 6 | `formatPrice(0)` возвращает «Бесплатно», что неверно для итогов журнала | ⚠️ T08 обошёл локальным форматтером, `client.ts` не тронут |

## Опасности, которые удалось предотвратить

**T10 оставил два неверных маркера-подсказки для T13** в `middleware.go`.
Проверено по исходникам и исправлено координатором перед запуском T13:

1. Предлагал `app.RegisterAdminPaymentsRoutes(r)` **внутри** `r.Route("/admin", ...)`.
   Метод регистрирует **абсолютные** пути `/admin/payments`, `/admin/settings`
   (`handler/admin_payments.go:20-27`) — монтирование дало бы `/admin/admin/payments`.
2. Предлагал `app.PaymentCheckoutPage` по пути `/payments/{provider}/checkout`.
   Такого хендлера **не существует**; T09 экспортирует `payments.MockConfirmHandler()`
   на верхнем уровне роутера по пути `/pay/mock/{id}`.

Правка внесена в `middleware.go` комментариями и в `Task/T13-integration-wiring.md`
разделом «⚠️ Исправленные координаты вставки».

## Прочие отложенные находки

| Находка | Кто нашёл | Куда девать |
|---|---|---|
| `Auth.tsx` читает `inviteToken` как поле формы, ссылка `/register?inviteToken=...` никуда не ведёт | T15 | бэклог |
| `Auth.tsx` не редиректит уже авторизованного с `/login` | T15 | бэклог |
| При провале квиза/кода в ответе нет `percent`/`nextLesson` | T03 | бэклог фронта |
| `EnsureEnrollment` при повторной покупке не сбрасывает `refunded` в `active` | T07 | бэклог |
| `SetCommissionPct` в Postgres глотает ошибку записи (`_, _ =`) | T07 | бэклог |
| Эндпоинта `POST /admin/enrollments/revoke` не существует, а `EnrollmentRevoked` никем не выставляется | T07 | бэклог T13 |
| 10 недостающих эндпоинтов CRUD конструктора | T06 | частично в T13 |
