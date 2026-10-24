# T94 — Фронтенд: сессии в карточке пользователя

**Волна:** P10  **Ветка:** `task/T94-admin-sessions-ui`  **Worktree:** `wt/t94`

## Суть

В модалке пользователя (`/admin/users`) нужно показать активные сессии и дать
супер-админу кнопку «Завершить все сессии». Бэкенд — T93, контракт
`GET/DELETE /admin/users/{id}/sessions` заморожен.

Работать только в `wt/t94` (ветка `task/T94-admin-sessions-ui`), база —
`integration/p10`.

## Что сделать

1. Новый API-модуль `frontend/src/api/adminSessions.ts`: типы
   `AdminSession`, `AdminSessionsResponse`; `listAdminUserSessions(userId)`,
   `revokeAdminUserSessions(userId)`. Используйте `apiFetch`; `client.ts` не
   править.
2. В `frontend/src/pages/AdminUsers.tsx` в компоненте `UserDetails`:
   - секция `p10.sessions.title` с пояснением `hint`;
   - запрос сессий (`queryKey: ['admin-user-sessions', user.id]`);
   - список: создана/истекает (formatDate), пустое состояние,
     loading/error;
   - кнопка `revokeAll` с confirm `confirmRevokeAll`, pending, success
     `revoked`; после успеха инвалидировать запрос сессий;
   - если сессий нет — кнопку не показывать/disabled.
3. Стили — в `frontend/src/styles/adminUsers.css`, используя существующие
   классы `au-details-section`, `au-details-list`, `au-details-item-*`,
   `btn`/`btn-danger`; адаптивность и `aria-live`.
4. Только ключи `p10.sessions.*`, `common.*`, `adminUsers.*` из `ru.json`;
   `ru.json` не править.
5. Тест `frontend/tests/api-contract/admin-sessions.test.mjs`: URL, методы,
   204-ответ, типы.

## Разрешённые файлы

- `frontend/src/api/adminSessions.ts` (новый)
- `frontend/src/pages/AdminUsers.tsx`
- `frontend/src/styles/adminUsers.css`
- `frontend/tests/api-contract/admin-sessions.test.mjs` (новый)
- `frontend/tests/T94-admin-sessions-ui.md` (отчёт)

## Запрещено

- `Task/**`, `specs/**`, `backend/**`, `ru.json`, `app/router.tsx`,
  `client.ts`, `types.ts`, `Admin.tsx`, `docker-compose*`, `qa-*.ps1`,
  `.github/**`, `deploy/**`.
- `git push`, `git merge`, смена ветки, работа вне `wt/t94`.

## Проверка

```
cd frontend
npm run build
npm test
```

При проблеме с каталогом тестов используйте
`node --test tests/api-contract/*.test.mjs` и укажите это в отчёте.

## Отчёт

`frontend/tests/T94-admin-sessions-ui.md`: что сделано, проверка, отклонения,
открытые вопросы. Коммит явными путями.
