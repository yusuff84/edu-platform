# T59 — Фронтенд: сводка с быстрыми переходами + CSV записей (frontend)

**Волна:** P7  **Ветка:** `task/T59-admin-dashboard-exports`  **Worktree:** `wt/t59`

## Суть

Сводка (`/admin/stats`) — мёртвые числа без ссылок; CSV-экспорт есть у
пользователей/курсов/платежей (T40), но отсутствует у «Записей на курсы»
(T49-страница). Нужны: быстрые переходы из сводки в разделы с фильтрами и
экспорт CSV на T49-странице. Ключи: `p7.stats.*`, `p7.enroll.*` (заморожены).

## Что сделать (только `frontend/**`)

1. `frontend/src/pages/AdminStats.tsx`:
   - под заголовком блок «Быстрые переходы» (`t('p7.stats.quickLinks')`) —
     табличка-ссылки в существующие разделы (Link со стрелком):
     `linksUsers` → `/admin/users`, `linksCourses` → `/admin/courses`,
     `linksEnrollments` → `/admin/enrollments`, `linksJournal` → `/admin`
     (окнраг со секцией `#journal` — смотри, как Admin.tsx навигирует к
     секциям; если нет хэшей — плоские Link в разделы),
     `linksQueue` → `/admin` (секция `#queue`);
     стиль — сетка карточек (переиспользуй `.as-cards` или соседние классы
     AdminStats, БЕЗ изменения css глобалки);
   - цифры оставляем как есть (данные из того же GET /admin/stats).
2. `frontend/src/pages/AdminEnrollments.tsx`:
   - кнопка «Экспорт CSV» (`t('p7.enroll.exportCsv')`) рядом с фильтрами:
     использовать существующий общий компонент `CsvExportButton`
     (`frontend/src/components/CsvExportButton.tsx`, T40) — экспортирует
     ТЕКУЩИЙ отфильтрованный список (до 1000 строк, как в T43);
     тост по завершению — `exportDone`; при ошибке — общий тост erro.
   - формат колонок CSV: id, email (studentEmail), имя (studentName),
     курс (courseTitle), courseId, статус (.childNodes? нет — status),
     прогресс (completed/total, percent), createdAt (ISO);
   - semicolon-разделитель? читай CsvExportButton (T40) — придерживайся его
     общих правил (уже реализованы).
3. Стили: только точечные в `adminConsole.css`/`adminEnrollments.css` в своей
   области (nullable, не ломает `styles: Т60/T61/T62` другие катлливые стили);
   без новых файлов если не нужно.

## Разрешённые файлы

- `frontend/src/pages/AdminStats.tsx`,
  `frontend/src/pages/AdminEnrollments.tsx`,
  `frontend/src/styles/adminConsole.css`, `adminEnrollments.css`
  (точечно), `frontend/tests/T59-admin-dashboard-exports.md`;
- НЕЛЬЗЯ: `ru.json`, `client.ts`, `CsvExportButton.tsx` (использовать как есть;
  расширения — только если граница T40 не выйдет: расширения Animator но
  писать в отчёт), Admin.tsx, AdminUsers/AdminCourses/T58-причастные.

## Проверка

`npm run build && npm test` — зелёные. (CSV-клиентские тесты T40 должны
продолжить проходить — компонент не меняем.)

## Отчёт

`frontend/tests/T59-admin-dashboard-exports.md`. Коммит `T59: ...`, push
`task/T59-admin-dashboard-exports`.
