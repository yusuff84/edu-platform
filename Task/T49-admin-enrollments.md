# T49 — Админка: экран «Записи на курсы» (frontend)

**Волна:** P6 · **Ветка:** `task/T49-admin-enrollments` · **Worktree:** `wt/t49`

## Зачем

Бэкенд умеет всё (список записей — T25, ручной отзыв/возврат доступа — T16),
но в UI этого нет вовсе: «полный функционал» админки требует экран
`/admin/enrollments`. Ключи `adminEnrollments.*` уже заморожены в `ru.json`.

## Что сделать

1. `frontend/src/api/adminEnrollments.ts` (новый): типы
   `AdminEnrollmentRow` / `AdminEnrollmentPage` (поля — как в
   `handler/admin_console.go`: `id, studentId, studentEmail, studentName,
   courseId, courseTitle, status, createdAt, completedLessons, totalLessons,
   percent`); функции:
   - `listAdminEnrollments({ q?, courseId?, status?, page?, limit? })` →
     `GET /admin/enrollments` (используй `buildQuery` из `client.ts`);
   - `revokeEnrollment(studentId, courseId)` и `unrevokeEnrollment(studentId, courseId)` →
     `POST /admin/enrollments/{studentId}/courses/{courseId}/revoke|unrevoke`
     (существующие эндпоинты T16 — ничего не выдумывать).
2. `frontend/src/pages/AdminEnrollments.tsx` (новый) + `frontend/src/styles/adminEnrollments.css`
   (новый; посмотри, как подключают стили соседние страницы, и повтори паттерн):
   - шапка/крошка как у AdminUsers (ссылка на `/admin`), заголовок — `adminEnrollments.title`;
   - фильтры: поиск `q` (debounce ~300 мс), select курса (опции —
     `listAdminCourses({ limit: 100 })` из `adminConsole.ts`, показывать название
     курса; первый пункт — «все курсы», `adminEnrollments.all`), select статуса
     («все» + `adminEnrollments.statusActive|statusRefunded|statusRevoked`),
     кнопка сброса `adminEnrollments.reset`;
   - таблица: ученик (email + имя), курс (`adminEnrollments.colCourse`),
     статус-бейдж, прогресс (`completedLessons` из `totalLessons` + `percent`),
     дата, действия (`adminEnrollments.colActions`);
   - действия: `active` → «Отозвать доступ» (`adminEnrollments.revoke`) с
     подтверждением (`adminEnrollments.confirmRevoke`); `revoked` → «Вернуть доступ»
     (`adminEnrollments.unrevoke`, `confirmUnrevoke`); `refunded` — без действий.
     После успеха — тост (`revokedDone` / `unrevokedDone`) и refetch; ошибки —
     строкой; на время запроса строка/кнопки disabled;
   - состояние: loading (`common.loading`), ошибка + `common.retry`, пусто
     (`adminEnrollments.empty`), «Всего записей» / «Показано N–M из total»
     (`adminEnrollments.total` / `adminEnrollments.showing`), пагинация
     Назад/Вперёд (`keepPreviousData`, как в AdminUsers);
   - вёрстка: таблица не вылезает за карточку — длинные email и названия курсов
     переносить/обрезать (как вылеченная таблица пользователей в T52), контейнер
     с `overflow-x: auto`, колонка действий видна при типовой ширине (~1100 px).
3. `frontend/src/app/router.tsx`: маршрут
   `/admin/enrollments` под `RequireRole roles={['admin']}`;
   в `frontend/src/pages/Admin.tsx` в блоке `admin-nav admin-nav--pages`
   добавить ссылку `<Link to="/admin/enrollments">{t('adminEnrollments.title')}</Link>`.
   Другие страницы (`AdminUsers.tsx`, `AdminCourses.tsx`, `AdminStats.tsx`)
   не трогать.
4. Прогнать проверки (ниже) и написать отчёт.

## Правила волны

- Работать только в своём worktree (`C:\Users\96352\all_project\eduPlatform\wt\t49`);
  коммит — в ветку `task/T49-admin-enrollments`; **push не делать**.
- Править только: `frontend/src/pages/AdminEnrollments.tsx` (новый),
  `frontend/src/api/adminEnrollments.ts` (новый),
  `frontend/src/styles/adminEnrollments.css` (новый), `frontend/src/app/router.tsx`
  (одна строка), `frontend/src/pages/Admin.tsx` (одна строка ссылки) и отчёт.
- `ru.json` не менять (ключи заморожены), `client.ts` не менять, новые внешние
  зависимости не добавлять, backend не трогать.
- Стиль и паттерны (react-query, тосты, обработка ошибок) — как у соседних
  админ-страниц (AdminUsers/AdminCourses); комментарии — по-русски.

## Проверка

```powershell
cd C:\Users\96352\all_project\eduPlatform\wt\t49\frontend
npm ci --prefer-offline   # первый раз в этом worktree (node_modules нет)
npm run build             # tsc --noEmit + vite build
npm test                  # контрактные тесты клиента — должны остаться зелёными
```

Живой стенд и браузер — не обязательны: визуальную приёмку сделает координатор.

## Отчёт

`frontend/tests/T49-admin-enrollments-ui.md`: что сделано (файлы, маршрут,
фильтры/действия), вывод команд, известные ограничения (например, в select
курсов попадают первые 100).
