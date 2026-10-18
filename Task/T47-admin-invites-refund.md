# T47 — Админка, бэкенд: список приглашений и возврат платежа

**Волна:** P6 · **Ветка:** `task/T47-admin-invites-refund` · **Worktree:** `wt/t47`

## Зачем

«Полный функционал» админки упирается в два белых пятна бэкенда:
`GET /admin/invites` (приглашение видно только в момент создания — токен показан
один раз, потом его не найти и не отозвать) и возврат платежа (метод `Refund`
есть в пакете `payments` с T13, но ни один хендлер его не вызывает). Обе формы
заморожены в `Task/00-api-contract.md`, раздел «Эндпоинты волны P6».

## Что сделать

1. Хранилище (`backend/internal/repo`):
   - `store_iface.go`: добавить в интерфейс
     `ListInvites(page, limit int) ([]*TeacherInvite, int)`,
     `RevokeInvite(id string) (*TeacherInvite, error)`,
     `GetPaymentByID(id string) (*Payment, bool)`;
     `RevokeInvite` различает `repo.ErrNotFound` (нет такого) и `repo.ErrConflict`
     (статус не `sent`); если `ErrConflict` ещё нет — завести рядом с
     существующими sentinel-ошибками repo;
   - in-memory (`store.go`): список — `createdAt` DESC + пагинация (+total);
     отзыв — только из статуса `sent`, атомарно (под существующим мьютексом стора);
   - Postgres (`postgres_store.go`): `SELECT ... ORDER BY created_at DESC
     LIMIT $1 OFFSET $2` + `COUNT(*)`; отзыв — `UPDATE teacher_invites SET
     status='revoked' WHERE id=$1 AND status='sent' RETURNING ...` с различением
     404/409 (после UPDATE без строк — проверка существования).
2. Хендлеры:
   - новый `backend/internal/handler/admin_invites.go`: `AdminListInvites`
     (`GET /admin/invites` — ответ строго по контракту: `items/total/page/limit`,
     в элементе `tokenPrefix` и НЕ отдавать `token_hash`/`token`),
     `AdminRevokeInvite` (`POST /admin/invites/{id}/revoke` — 200/404/409);
     не забыть `a.adminOnly(w, r)` как в соседних хендлерах;
   - `backend/internal/handler/admin_payments.go`: `AdminRefundPayment`
     (`POST /admin/payments/{id}/refund`):
     * `GetPaymentByID` → нет → 404 «Платёж не найден»;
     * статус не `succeeded` → 409 «Возврат возможен только для успешного платежа»;
     * `payments.NewProvider(p.Provider)` + `Refund(ctx, p.ProviderPaymentID)`;
       ошибка провайдера (включая `ErrNotConfigured`) → 502 «Не удалось выполнить
       возврат у провайдера»;
     * применение результата — ПОД `a.webhookMu` (тот же мьютекс, что у вебхука,
       см. фикс гонки P5 b9a84df): повторная проверка статуса под замком,
       `SetPaymentStatus(refunded)` + перевод записи ученика в `refunded`
       (семантика ветки `refunded` в `PaymentWebhook`, enroll.go);
     * ответ `{"payment": <paymentJournalItem>}` — переиспользуй маппинг журнала
       (studentEmail/studentName обязательны), свежие значения читай после записи.
3. Маршруты: `middleware.go` — в группе `/admin` добавить
   `r.Get("/invites", app.AdminListInvites)` и
   `r.Post("/invites/{id}/revoke", app.AdminRevokeInvite)`;
   `RegisterAdminPaymentsRoutes` (admin_payments.go) — добавить
   `Post("/admin/payments/{id}/refund", a.AdminRefundPayment)`.
4. Тесты `backend/tests/contract/` (новые файлы, стиль существующих):
   - `admin_invites_test.go`: создание через `POST /admin/invites` → список
     (`GET /admin/invites`) содержит запись с нужными полями, без токена;
     `limit=1&page=2` даёт вторую страницу; revoke → 200 `status=revoked`;
     повторный revoke → 409; случайный UUID → 404; студент/учитель → 403;
   - `admin_refund_test.go`: покупка mock → вебхук `succeeded` → refund → 200
     (`status=refunded`, `processedAt` заполнен), запись стала `refunded` и курс
     исчез из `/me/courses`; повтор → 409; случайный UUID → 404; студент → 403;
     конкурентность: два параллельных refund одного платежа → ровно один 200 и
     один 409 (для `-race`).
   Существующие тесты не ломать.

## Правила волны

- Работать только в своём worktree (`C:\Users\96352\all_project\eduPlatform\wt\t47`);
  коммит — в ветку `task/T47-admin-invites-refund`; **push не делать** — ветки
  публикует координатор при слиянии.
- Править только: `backend/internal/handler/admin_invites.go` (новый),
  `backend/internal/handler/{admin_payments.go, middleware.go}`,
  `backend/internal/repo/{store_iface.go, store.go, postgres_store.go, ошибки}`,
  новые файлы тестов `backend/tests/contract/*` и отчёт.
- Go собирать и запускать **только в Docker**; новых внешних зависимостей нет;
  `specs/` и `ru.json` не трогать; контейнеры `olymp-*` не останавливать.
- Формы ответов и тексты ошибок — строго по `Task/00-api-contract.md`
  («Эндпоинты волны P6»); комментарии в коде — по-русски.

## Проверка

```powershell
# Полный прогон (alpine; volumes — общий кэш модулей и сборки):
docker run --rm -v "C:\Users\96352\all_project\eduPlatform\wt\t47\backend:/src" -v gomodcache:/go/pkg/mod -v gocache:/root/.cache/go-build -w /src -e GOFLAGS=-mod=mod golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"

# Гонка возврата (bookworm: -race требует gcc):
docker run --rm -v "C:\Users\96352\all_project\eduPlatform\wt\t47\backend:/src" -v gomodcache:/go/pkg/mod -v gocache:/root/.cache/go-build -w /src -e GOFLAGS=-mod=mod golang:1.24-bookworm sh -c "go test ./tests/contract/... -run Refund -race -count=3"
```

Хвосты вывода обеих команд — в отчёт.

## Отчёт

`backend/tests/T47-admin-invites-refund.md`: что сделано, команды, итоги
(сколько тестов, статусы), ограничения (реестр mock-провайдера живёт в памяти
процесса — после перезапуска api возврат старого платежа даст 502).
