# T90 — фронтенд глобальной страницы промокодов

**Статус:** выполнено.
**Ветка:** `task/T90-admin-promos-ui`.
**Worktree:** `wt/t90`.

## Что сделано

- `frontend/src/api/adminConsole.ts` — типы `AdminPromoCode`,
  `AdminPromoCodesResponse`, `AdminPromoCodesQuery`, `AdminPromoCodePatch`
  и функции `listAdminPromoCodes()` / `updateAdminPromoCode(id, patch)`
  (GET/PATCH `/admin/promo-codes`). `active=false` передаётся явно,
  `expiresAt: null` в PATCH уходит как `null` (снять срок), ответ-конверт
  `{promoCode}` распаковывается в запись.
- `frontend/src/pages/AdminPromoCodes.tsx` — страница `/admin/promo-codes`:
  заголовок/подзаголовок, «Назад в админку», фильтры `q`/`courseId`/`active`
  с debounce и URL-состоянием, сброс, пагинация, таблица (код, курс со
  ссылкой, скидка, `used / max` с «без лимита» при `maxUses=0`, срок или
  «Без срока», статус, действия), инлайн-редактор
  `discountPct`/`maxUses`/`expiresAt`/`active` с валидацией и pending,
  включение/отключение через `confirm`, `aria-live`-уведомления, состояния
  loading/error/empty, доступные label.
- `frontend/src/styles/adminPromos.css` — изолированные стили `.ap-*`,
  адаптивность (карточки на узком экране), подключены из страницы.
- `frontend/src/app/router.tsx` — lazy-маршрут `/admin/promo-codes` под
  `RequireRole roles={['admin']}`; `useSeo` вызывается на странице.
- `frontend/src/pages/Admin.tsx` — ссылка «Промокоды» в навигации подразделов.
- `frontend/tests/api-contract/admin-promos.test.mjs` — 12 контрактных тестов:
  query string (включая `active=false` и пустые параметры), PATCH-тело,
  явный `expiresAt=null`, частичное тело, кодирование id, ответ `promoCode`,
  ошибки 400/403/404/409.

i18n: использованы только существующие замороженные ключи `p10.promo.*`,
`p10.nav.*`, `p8.promo.*`, `common.*`, `admin.*`. `frontend/src/i18n/ru.json`
не изменялся.

## Проверки

- `cd frontend && npm ci` — в worktree отсутствовал `node_modules`
  (каталог в gitignore, в коммит не входит).
- `npm run build` — успешно (`tsc --noEmit` + `vite build`, 162 модуля).
- `npm run test:build` — собран `.tmp-api-test/client.js`, нужный старым
  контрактным тестам.
- `node --test "tests/api-contract/*.test.mjs"` — **205/205 pass**
  (включая 12 новых тестов T90).
- `node --test tests/api-contract/admin-promos.test.mjs` — **12/12 pass**.

## Отклонения

1. **База ветки.** Фактический HEAD ветки — `80facc7` (docs-коммит P10);
   `integration/p10` (`8c7fd48`) содержит влитые T89/T91/T92/T93, но не
   является предком ветки. Merge/rebase не выполнялся (запрет постановки).
   Фронтенд реализован по замороженному контракту T89, backend не трогался.
2. **Расположение API.** По указанию оркестратора типы и функции добавлены
   в существующий `frontend/src/api/adminConsole.ts`, а не в новый
   `frontend/src/api/adminPromoCodes.ts`, который упомянут в постановке T90.
3. **Редактирование.** По указанию оркестратора сделан инлайн-редактор в
   строке таблицы; в постановке T90 описана модалка. Набор полей, валидация,
   pending и обработка ошибок сохранены.
4. **`Admin.tsx`.** Добавлена одна ссылка навигации по указанию оркестратора;
   в списке запрещённых файлов постановки T90 `Admin.tsx` указан.
5. **Частичный PATCH при редактировании.** В тело уходят только изменённые
   поля: неизменённый прошедший `expiresAt` не блокирует правку скидки/лимита
   и не провоцирует лишний 400. Если срок меняют, он проверяется на будущее.

## Открытые вопросы

- Справочник курсов для фильтра — первые 100 (потолок `limit`), как в T49;
  выбранный курс остаётся в select, даже если не попал в первые 100.
- Активация истёкшего кода не блокируется на клиенте: ответ 400/409 бэкенда
  показывается в строке.
- Коммит: один коммит в ветке `task/T90-admin-promos-ui`; hash — в финальном
  ответе оркестратору.
