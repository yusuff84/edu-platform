# T90 — Фронтенд: глобальная страница промокодов

**Волна:** P10  **Ветка:** `task/T90-admin-promos-ui`  **Worktree:** `wt/t90`

## Суть

Нужна страница `/admin/promo-codes`: все промокоды платформы, фильтры по
курсу/статусу/поиску, пагинация, редактирование скидки/лимита/срока и
включение/отключение. Бэкенд — T89, контракт заморожен.

Работать только в `wt/t90` (ветка `task/T90-admin-promos-ui`), база —
`integration/p10`.

## Что сделать

1. Новый API-модуль `frontend/src/api/adminPromoCodes.ts`: типы
   `AdminPromoCode`, `AdminPromoCodesResponse`, `AdminPromoCodesQuery`,
   `AdminPromoCodePatch`; `listAdminPromoCodes(query)`,
   `updateAdminPromoCode(id, patch)`. Используйте `apiFetch`/`buildQuery`,
   не правьте `client.ts`.
2. Новая страница `frontend/src/pages/AdminPromoCodes.tsx`:
   - заголовок/подзаголовок, «Назад в админку»;
   - фильтры: поиск, курс (`listAdminCourses` из `adminConsole.ts`), статус
     все/активные/неактивные; сброс; URL-состояние; пагинация;
   - таблица: код, курс (ссылка), скидка, использовано `used / max` (если
     `maxUses=0` — «без лимита»), срок (или «Без срока»), статус, действия;
   - модалка редактирования: скидка 1..90, лимит использований, дата/время
     (datetime-local → RFC3339), активность; валидация, pending, ошибки;
   - быстрое включение/отключение с confirm; success/error `aria-live`;
   - empty/loading/error, доступные label.
3. Новый CSS `frontend/src/styles/adminPromoCodes.css` в стиле
   `adminUsers.css`/`adminConsole.css`; адаптивность.
4. Роут `/admin/promo-codes` в `frontend/src/app/router.tsx` с
   `RequireRole roles={['admin']}`, lazy import, `useSeo`.
5. Ключи только `p10.promo.*`, `p8.promo.*` (поля кода/скидки/лимита/срока),
   `common.*`, `admin.*`; `ru.json` не править.
6. Тест `frontend/tests/api-contract/admin-promos.test.mjs`: query string,
   PATCH-тело, обработка null expiresAt, ответ `promoCode`.

## Разрешённые файлы

- `frontend/src/api/adminPromoCodes.ts` (новый)
- `frontend/src/pages/AdminPromoCodes.tsx` (новый)
- `frontend/src/styles/adminPromoCodes.css` (новый)
- `frontend/src/app/router.tsx`
- `frontend/tests/api-contract/admin-promos.test.mjs` (новый)
- `frontend/tests/T90-admin-promos-ui.md` (отчёт)

## Запрещено

- `Task/**`, `specs/**`, `backend/**`, `ru.json`, `client.ts`, `types.ts`,
  `Admin.tsx`, `docker-compose*`, `qa-*.ps1`, `.github/**`, `deploy/**`.
- `git push`, `git merge`, смена ветки, работа вне `wt/t90`.

## Проверка

```
cd frontend
npm run build
npm test
```

При проблеме с каталогом тестов используйте
`node --test tests/api-contract/*.test.mjs` и укажите это в отчёте.

## Отчёт

`frontend/tests/T90-admin-promos-ui.md`: что сделано, проверка, отклонения,
открытые вопросы. Коммит явными путями.
