# T51 — Админка: приглашения (список/отзыв), платежи (детали/возврат), автор в очереди

**Волна:** P6 · **Ветка:** `task/T51-admin-invites-payments-ui` · **Worktree:** `wt/t51`

## Зачем

Главная страница админки (`/admin`, `pages/Admin.tsx`) создаёт приглашения, но
не показывает уже выданные (отозвать нельзя); журнал платежей ведёт пассивно —
детали не раскрыть, возврат средств не сделать; очередь модерации показывает
курс анонимно, без автора и объёма. Бэкенд для всего готовится в T47/T48
(контракт P6), ключи `admin.*` заморожены в `ru.json`.

## Что сделать

1. `frontend/src/api/adminActions.ts` (новый): типы `AdminInviteRow` /
   `AdminInvitePage`; функции:
   - `listInvites({ page?, limit? })` → `GET /admin/invites`;
   - `revokeInvite(id)` → `POST /admin/invites/{id}/revoke`;
   - `refundPayment(id)` → `POST /admin/payments/{id}/refund`.
2. `frontend/src/pages/Admin.tsx`:
   - секция `#invites`: под формой создания — таблица приглашений (email,
     статус-бейдж, создано, действует до, действия). Статус: `sent` с истёкшим
     `expiresAt` показывать как «Истекло» (`admin.inviteStatusExpired`); остальные —
     `inviteStatusSent|Accepted|Revoked`. Кнопка «Отозвать» (`admin.inviteRevoke`)
     только для `sent` с подтверждением (`admin.confirmInviteRevoke`); после
     успеха — тост `admin.inviteRevokedDone` и refetch. Итог (`admin.invitesTotal`),
     пусто (`admin.invitesEmpty`), пагинация Назад/Вперёд (limit 10–20);
   - секция `#journal`: в каждой строке — кнопка «Детали» (`admin.paymentOpen`),
     открывающая модалку «Детали платежа» (`admin.paymentDetails`): ученик, курс,
     провайдер, ID у провайдера, сумма/комиссия платформы/преподавателю
     (`admin.payment*`), статус, создан/обработан. Для `status === 'succeeded'` —
     кнопка «Вернуть средства» (`admin.paymentRefund`) с подтверждением
     (`admin.confirmRefund`): вызов `refundPayment(id)` → тост `admin.refundDone`,
     закрытие модалки и refetch журнала; ошибка API (409/502) — текстом в модалке;
   - секция `#queue`: показывать автора и объём — `teacherEmail` (`admin.queueTeacher`),
     `modulesCount` (`admin.queueModules`), `lessonsCount` (`admin.queueLessons`)
     и ссылку «Страница курса» (`admin.queueOpen`) на `/courses/{slug}` (slug —
     из объекта курса). Поля приходят после T48 — типы объяви локально как
     опциональные (`Course & { teacherEmail?: string; modulesCount?: number;
     lessonsCount?: number }`), без падения при их отсутствии.
3. Вёрстка: таблица приглашений и журнал не вылезают за карточку (длинные email/
   названия переносить/обрезать, контейнер с `overflow-x: auto`); модалка деталей —
   по образцу существующих панелей/модалок проекта: `role="dialog"`,
   закрытие по Esc и клику вне, фокус на заголовке.
4. Прогнать проверки (ниже) и написать отчёт.

## Правила волны

- Работать только в своём worktree (`C:\Users\96352\all_project\eduPlatform\wt\t51`);
  коммит — в ветку `task/T51-admin-invites-payments-ui`; **push не делать**.
- Править только: `frontend/src/pages/Admin.tsx`,
  `frontend/src/api/adminActions.ts` (новый),
  `frontend/src/styles/admin.css` и отчёт.
- `ru.json` не менять (ключи заморожены), `client.ts` не менять (там уже есть
  `createInvite`/`reviewQueue`/`paymentJournal`/`moderateCourse`), `router.tsx`
  не трогать, новые внешние зависимости не добавлять, backend не трогать.
- Комментарии — по-русски; стиль — как у соседних секций `Admin.tsx`.

## Проверка

```powershell
cd C:\Users\96352\all_project\eduPlatform\wt\t51\frontend
npm ci --prefer-offline   # первый раз в этом worktree
npm run build             # tsc --noEmit + vite build
npm test                  # контрактные тесты клиента — должны остаться зелёными
```

Живую приёмку (в т.ч. возврат средств на mock-платеже) сделает координатор.

## Отчёт

`frontend/tests/T51-admin-invites-payments-ui.md`: что сделано, вывод команд,
ограничения (mock-возврат возможен только для платежа, созданного после
последнего перезапуска api).
