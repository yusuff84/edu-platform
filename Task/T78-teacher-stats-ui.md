# T78 — Фронтенд: страница аналитики учителя

**Волна:** P9  **Ветка:** `task/T78-teacher-stats-ui`  **Worktree:** `wt/t78`

## Суть

Нужна наглядная аналитика в кабинете преподавателя: KPI, доход, завершаемость
и таблица по курсам. API — по разделу «Волна P9» в `Task/00-api-contract.md`
(**заморожен**); бэкенд T77 идёт параллельно.

Работать только в своём worktree `wt/t78` (ветка `task/T78-teacher-stats-ui`).

## Что сделать

1. `frontend/src/api/types.ts`: `TeacherOverview`, `TeacherCourseStat`,
   `TeacherCourseStatsPage` по контракту.
2. `frontend/src/api/client.ts`: `teacherOverview()`, `teacherCourseStats(limit?)`.
3. Новая страница `frontend/src/pages/TeacherStats.tsx`:
   - KPI-карточки: курсы (всего/опубликовано), ученики (всего/уникальные),
     выручка (оплачено/ваш доход/комиссия), завершаемость (завершено,
     средний прогресс), рейтинг, за 30 дней (записи/выручка);
   - таблица по курсам: название, статус, ученики, выручка, прогресс, рейтинг;
     строка кликабельна → `/teach/{courseId}` (кнопка «Открыть»);
   - состояния загрузки/ошибки/пустое («Курсов пока нет»), форматирование
     денег и процентов как в существующих страницах (`formatPrice`/`formatDate`
     из `client.ts` или локально);
   - адаптив: на узких экранах карточки в колонку, таблица скроллится.
4. `frontend/src/app/router.tsx`: маршрут `/teach/stats` под
   `RequireRole(['teacher','admin'])` (до `/teach/:id`, иначе `:id` перехватит
   `stats` — проверьте порядок маршрутов).
5. `frontend/src/pages/Cabinets.tsx`: ссылка/кнопка «Аналитика» в кабинете
   преподавателя.
6. Стили — новый `frontend/src/styles/teacherStats.css` (импорт в странице), на
   переменных дизайн-системы. Чужие css не править.
7. i18n — только готовые ключи `p9.stats.*`; словарь не править.
8. Тесты: новый `frontend/tests/api-contract/teacher-stats.test.mjs`: сводка,
   разбивка с limit, ошибки 403.

## Разрешённые файлы

- `frontend/src/pages/TeacherStats.tsx` (новый)
- `frontend/src/pages/Cabinets.tsx`
- `frontend/src/app/router.tsx`
- `frontend/src/api/client.ts`, `frontend/src/api/types.ts`
- `frontend/src/styles/teacherStats.css` (новый)
- `frontend/tests/api-contract/teacher-stats.test.mjs` (новый)
- `frontend/tests/T78-teacher-stats-ui.md` (отчёт)

## Запрещено

- `backend/**`, `Task/**`, `specs/**`, `frontend/src/i18n/ru.json`, чужие
  страницы/компоненты/стили, `qa-*.ps1`, `docker-compose*`.
- Менять существующие маршруты и ломать сигнатуры клиентских функций.
- `git push`, `git merge`, смена ветки, работа вне `wt/t78`.

## Проверка

Из папки `frontend/`:

```
npm run build
npm test
```

Сборка и все тесты (старые + новые) зелёные. При таймаутах порта 8799 —
подождите 30–60 секунд и повторите `npm test`.

## Отчёт

`frontend/tests/T78-teacher-stats-ui.md`: «Что сделано», «Проверка»,
«Отклонения», «Открытые вопросы». Закоммитьте отчёт и код в ветку
`task/T78-teacher-stats-ui`.
