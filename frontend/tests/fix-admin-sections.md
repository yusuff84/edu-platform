# fix/admin-sections — разделы админки на отдельных страницах

Дата: 24.10.2026. Ветка: `fix/admin-sections` (от `main` @ a0d5158).

## Маршруты

| Маршрут | Компонент | Назначение |
| --- | --- | --- |
| `/admin` | `pages/Admin.tsx` | хаб админки: только карточки-ссылки, без запросов и якорей |
| `/admin/moderation` | `pages/AdminModeration.tsx` | очередь модерации курсов (бывший `#queue`) |
| `/admin/payments` | `pages/AdminPayments.tsx` | журнал платежей (бывший `#journal`) |
| `/admin/settings` | `pages/AdminSettings.tsx` | комиссия и промо-бар (бывший `#settings`) |
| `/admin/invites` | `pages/AdminInvites.tsx` | приглашения (бывший `#invites`) |

Все четыре маршрута объявлены в `frontend/src/app/router.tsx` как lazy-чанки
и защищены `<RequireRole roles={['admin']}>`. Существующие маршруты
`/admin/users`, `/admin/courses`, `/admin/courses/:id`, `/admin/enrollments`,
`/admin/stats`, `/admin/reviews`, `/admin/qa`, `/admin/promo-codes` не менялись.

## Что вынесено

- `AdminModeration.tsx` — запрос очереди `['review-queue']`, комментарии и
  ошибки по каждому курсу, approve/reject, инвалидация очереди.
- `AdminPayments.tsx` — журнал `['admin-payments']`, фильтры в URL
  (`page/limit/status/courseId/q/from/to`), пагинация, выгрузка CSV с
  предупреждением об усечении, копирование ID платежа, модалка деталей,
  возврат средств и сообщения об успехе/ошибке.
- `AdminSettings.tsx` — `['admin-settings']`, сохранение комиссии с
  подтверждением, тогл промо-бара и инвалидация `['public-settings']`.
- `AdminInvites.tsx` — создание приглашения, одноразовый показ и копирование
  токена, список `['admin-invites']` с пагинацией, отзыв и статусы
  (включая вычисляемое «Истекло»).
- `adminShared.ts` — общие константы, типы и утилиты: `errorText`, `shortId`,
  `copyToClipboard`, `isInviteExpired`, `inviteDisplayStatus`,
  `isPaymentStatus`, `paymentsCsv`, `csvCell`, `todayIsoDate`,
  `downloadTextFile`, `STATUS_LABEL`, `INVITE_STATUS_LABEL`,
  `PAYMENT_STATUSES`, лимиты выгрузки и тип `QueueCourse`. Модуль зависит
  только от `api/*` и `i18n/*` — циклических импортов нет.

## Старые ссылки и редиректы

`Admin.tsx` читает `useLocation().hash` и делает `<Navigate replace />`:

- `/admin#queue` → `/admin/moderation`
- `/admin#journal` → `/admin/payments`
- `/admin#settings` → `/admin/settings`
- `/admin#invites` → `/admin/invites`

Обновлены ссылки в `AdminStats.tsx` (быстрые переходы на журнал и очередь) и
футер в `router.tsx` («Модерация» и «Приглашения»). В `frontend/src` не
осталось рабочих ссылок `/admin#...` и `<a href="#queue|#journal|#settings|#invites">`;
единственное упоминание — комментарий с описанием редиректа.

## Вид страниц

Каждая новая страница обёрнута в
`<div className="wrap"><div className="page"><main className="admin">`, имеет
заголовок и ссылку «Назад в админку» (`p10.nav.backToAdmin`), а содержимое —
один блок `.admin-section.panel`. Классы таблиц, фильтров, модалки и
пагинации сохранены без изменений. Для хаба в `styles/admin.css` добавлены
`.admin-hub`, `.admin-hub-group`, `.admin-hub-cards`, `.admin-hub-card` с
адаптивной сеткой. Новые ключи в `frontend/src/i18n/ru.json` не добавлялись:
подписи групп хаба — короткие русские заголовки, остальные тексты — из
существующего словаря.

## Проверки

- `npm run build` — PASS (tsc + vite build).
- `npm test` — PASS, 235/235 тестов API-контракта.
- Поиск по `frontend/src`: рабочих `/admin#queue|#journal|#settings|#invites`
  и `href="#..."` для этих разделов нет.
- Все новые маршруты объявлены и защищены `RequireRole roles={['admin']}`.

## Ограничения

- Визуальные скриншоты не снимались (в окружении нет браузерной проверки
  интерфейса); вёрстка проверена по DOM-структуре и успешной сборке.
- Редиректы обрабатывают только четыре исторических якоря; произвольные
  hash-значения на `/admin` просто игнорируются.
- Старые отчёты в `frontend/tests/T*.md` по-прежнему описывают якорную
  структуру — это исторические документы, они не переписывались.
- `Task/**`, `specs/**`, `frontend/src/i18n/ru.json` и backend не менялись.
