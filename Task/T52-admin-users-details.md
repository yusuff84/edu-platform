# T52 — Админка: пользователи — детали, подтверждения и починка вёрстки таблицы

**Волна:** P6 · **Ветка:** `task/T52-admin-users-details` · **Worktree:** `wt/t52`

## Зачем

Пользователь прислал скриншот `/admin/users`: таблица вылезает за карточку,
колонка «Действия» обрезана по краю (видны «Забл…»/«Удал…»), селект роли сжат
до ~43 px (вместо «ученик» — «у» и стрелка). Замеры на живом стенде: таблица
1194 px при контейнере 1094 px; email-колонка 357 px (длинные qa-адреса), имя
227 px, селект роли 43 px, действия 163 px. Плюс разделу не хватает деталей
пользователя и подтверждений опасных действий.

## Что сделать

1. **Починка вёрстки `/admin/users`** (первоочередное, `styles/adminUsers.css`
   + при необходимости разметка `AdminUsers.tsx`):
   - `.au-role-select` — минимальная ширина ~110–120 px, чтобы полностью
     помещался самый длинный вариант («администратор»);
   - email-колонка: `max-width: ~220px` + перенос длинных адресов
     (`overflow-wrap: anywhere`/`word-break: break-all`), а не растягивание таблицы;
   - имя: `max-width: ~150px` + `overflow: hidden; text-overflow: ellipsis`
     (полное значение — в `title` ячейки);
   - действия: компактная колонка (~≤150 px), кнопки целиком (столбик или wrap),
     ничего не обрезается;
   - цель: при контейнере ~1100 px таблица с колонкой действий помещается
     целиком; на узких экранах продолжает работать `.au-table-wrap { overflow-x:
     auto }` и мобильная раскладка (медиазапрос уже есть — проверь, что правки
     её не ломают).
2. **Детали пользователя** (`AdminUsers.tsx`, `api/adminUsers.ts`):
   - кнопка `adminUsers.open` («Профиль») в строке → модальное окно
     (`adminUsers.detailsTitle`): email, имя, роль, статус, дата регистрации,
     ID (`adminUsers.detailsId`); `role="dialog"`, закрытие по Esc/клику вне;
   - ниже — секция «Записи на курсы» (`adminUsers.detailsEnrollments`):
     `GET /admin/enrollments?studentId=...&limit=10`; секция «Последние платежи»
     (`adminUsers.detailsPayments`): `GET /admin/payments?q=<email>&limit=10`;
     fetch — маленькими локальными хелперами в `adminUsers.ts` через `apiFetch`
     из `client.ts` (модуль `adminEnrollments.ts` появится отдельной веткой —
     от него не зависеть); loading (`common.loading`), пусто
     (`adminUsers.detailsEmpty`).
3. **Подтверждения** (в стиле уже существующего инлайн-подтверждения удаления
   в этой таблице):
   - смена роли: `onChange` селекта → подтверждение (`adminUsers.confirmRole`,
     показать старую и новую роль) → только потом `setUserRole`;
   - блокировка/разблокировка — `adminUsers.confirmBlock`/`confirmUnblock`;
   - удаление — существующее подтверждение (ключ `adminUsers.confirmRemove`);
     при ошибке 409 (есть зависимости) показывай текст с сервера;
   - после успеха — тосты/уведомление и refetch списка; ошибки строкой, как сейчас.
4. Прогнать проверки (ниже) и написать отчёт.

## Правила волны

- Работать только в своём worktree (`C:\Users\96352\all_project\eduPlatform\wt\t52`);
  коммит — в ветку `task/T52-admin-users-details`; **push не делать**.
- Править только: `frontend/src/pages/AdminUsers.tsx`,
  `frontend/src/api/adminUsers.ts`, `frontend/src/styles/adminUsers.css` и отчёт.
- `ru.json` не менять (ключи заморожены), `client.ts`, `router.tsx`, другие
  страницы не трогать, новые внешние зависимости не добавлять, backend не трогать.
- Итоговые ширины колонок/правила — описать в отчёте; финальную визуальную
  приёмку на стенде (в т.ч. ~1250 px и уже) сделает координатор.

## Проверка

```powershell
cd C:\Users\96352\all_project\eduPlatform\wt\t52\frontend
npm ci --prefer-offline   # первый раз в этом worktree
npm run build             # tsc --noEmit + vite build
npm test                  # контрактные тесты клиента — должны остаться зелёными
```

## Отчёт

`frontend/tests/T52-admin-users-details.md`: что сделано, как починена вёрстка
(какие ширины/правила заданы), вывод команд, ограничения.
