# T43 — CSV: предупреждение об обрезке 5000 строк (frontend)

**Волна:** P5 · **Ветка:** `task/T43-csv-limit-warning` · **Worktree:** `wt/t43`
**Контракт:** API не меняется. Ключ `admin.exportTruncated` уже добавлен
в `ru.json` координатором (правки словаря — вне объёма задачи).

## Зачем

Ограничение P4: выгрузки (журнал платежей, пользователи, курсы) молча обрезаются
на 5000 строках (50 страниц × 100). Нужно честно показывать, что выборка усечена.

## Что сделать

1. `frontend/src/components/CsvExportButton.tsx`:
   - `loadRows` возвращает `Promise<{ rows: Array<Array<CsvCell>>; truncated: boolean }>`
     (обновить тип пропса и вызовы в `AdminUsers.tsx`, `AdminCourses.tsx`);
   - тон сообщения: добавить `'warn'`; при `truncated && rows.length > 0` —
     текст `admin.exportTruncated` (файл всё равно скачивается); пустой набор —
     как было (`admin.exportEmpty`, файл не создаётся); успех без обрезки —
     `admin.exported`;
   - экспортировать чистую `export function exportTruncated(total: number, loaded: number): boolean`
     (`total > loaded`) — по образцу экспортируемой `toCsv`;
   - стиль `.ce-msg--warn` в `frontend/src/styles/csvExport.css`
     (цвет — как у предупреждений проекта; найди образец).
2. `frontend/src/pages/AdminUsers.tsx` и `AdminCourses.tsx`:
   - в циклах выгрузки уже известен `total` первой страницы — вернуть
     `truncated = exportTruncated(total, загружено)` в новом формате результата.
3. `frontend/src/pages/Admin.tsx` (`onExportCsv`, строки ~261–300 и рендер ~624–626):
   - запоминать `total` из ответов; после скачивания при `total > rows.length`
     показать `admin.exportTruncated` тоном `warn` (расширить `exportMsg` до
     `tone: 'ok' | 'warn' | 'error'`, рендер: `role` — `status` для warn,
     класс — `ce-msg ce-msg--warn`, импорт `../styles/csvExport.css`);
     иначе — прежний `admin.exported`.
4. Если харнесс T37 (`frontend/tests/**`, `npm test`) позволяет дёшево покрыть
   `exportTruncated` — добавь тест; иначе честно напиши в отчёте.

## Правила волны

- Работать только в своём worktree; коммит — в свою ветку `task/T43-csv-limit-warning`.
- Править только: `frontend/src/components/CsvExportButton.tsx`,
  `frontend/src/pages/{Admin,AdminUsers,AdminCourses}.tsx`,
  `frontend/src/styles/csvExport.css`, при желании `frontend/tests/**` (новый тест),
  отчёт `frontend/tests/T43-csv-limit-warning.md`.
- `frontend/src/i18n/ru.json` (ключ уже есть), `Task/**`, `specs/**` — не трогать.
  Новых зависимостей нет.

## Проверка (из каталога worktree)

```powershell
cd frontend
npx tsc --noEmit
npx vite build
npm test
```

Все — `exit 0`. На стенде (`http://localhost:3001`, `admin@local / admin1234`):
выгрузка на `/admin`, `/admin/users`, `/admin/courses` работает, без обрезки
показывается `admin.exported` (реальную обрезку на стенде не воспроизвести —
честно напиши об этом в отчёте).

## Отчёт

`frontend/tests/T43-csv-limit-warning.md`: что изменено, где показывается
предупреждение, как проверено, честный список непроверенного.
