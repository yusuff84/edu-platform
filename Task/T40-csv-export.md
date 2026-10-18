# T40 — Списки админки: выгрузка CSV (frontend)

**Волна:** P4 · **Ветка:** `task/T40-csv-export` · **Worktree:** `wt/t40`

## Зачем

Админу нужны списки пользователей и курсов «наружу» (сверки, отчётность). Сейчас данные видны
только в таблицах с пагинацией по страницам, выгрузки нет.

## Что сделать

1. Новый компонент `frontend/src/components/CsvExportButton.tsx`:
   - пропсы: `filename: string`, `headers: string[]`, `loadRows: () => Promise<Array<Array<string | number | null>>>`,
     `disabled?: boolean`;
   - кнопка: текст `admin.exportCsv`, `title` — `admin.exportCsvHint`;
   - CSV: разделитель `;`, BOM `\uFEFF`, каждое поле в кавычках, внутренние кавычки удваиваются,
     `null`/`undefined` → пустое поле, переводы строк в значениях сохраняются;
   - скачивание через `Blob` + `URL.createObjectURL` (без библиотек), ревокация ссылки после клика;
   - состояния: идёт выгрузка (кнопка `disabled`), успех → `admin.exported`, ошибка → текст ошибки
     (по образцу существующих сообщений), пустой набор → `admin.exportEmpty` (файл не создаётся);
   - экспортируемую функцию преобразования строк **экспортировать** (`export function toCsv(...)`) —
     её позже сможет покрыть тестами T37-подобный набор; сама логика без DOM — принимать данные,
     возвращать строку.
2. `frontend/src/pages/AdminUsers.tsx`: кнопка выгрузки; выгружается **вся выборка по текущим
   фильтрам** (`q`, `role`, `status`) — постранично `limit=100` до `total`, но не более 50 страниц
   (5000 строк; ограничение описать в отчёте). Колонки: Email, Имя, Роль, Статус, Дата регистрации,
   Курсов, Записей; заголовки — из ключей словаря, где есть подходящие (`adminUsers.*`,
   `admin.col*`, `profile.*`); для остальных короткая русская подпись прямо в CSV (это данные,
   а не интерфейс). Имя файла `users-<ГГГГ-ММ-ДД>.csv`.
3. `frontend/src/pages/AdminCourses.tsx`: то же для курсов (фильтры `q`, `status`) —
   колонки: Slug, Название, Преподаватель (email), Статус, Цена, Модулей, Уроков, Учеников,
   Создан, Обновлён; файл `courses-<ГГГГ-ММ-ДД>.csv`. Цена — в рублях (`formatPrice`/`formatMoney`),
   даты — `formatDate`.
4. Не ломать существующее: фильтры, пагинацию, решения по модерации, отзывы, удаление пользователя.

## Проверка

```powershell
cd C:\Users\96352\all_project\eduPlatform\edu-platform\frontend
npx tsc --noEmit
npx vite build
```

Оба `exit 0`. На стенде (`localhost:3001`, `admin@local / admin1234`): `/admin/users` и
`/admin/courses` — нажать «Выгрузить CSV» (`document.body.innerText` должен показать `admin.exported`).
Если проверить не удалось — честно в отчёт.

## Ограничения

- Править только: `frontend/src/components/CsvExportButton.tsx` (новый),
  `frontend/src/pages/AdminUsers.tsx`, `frontend/src/pages/AdminCourses.tsx`,
  `frontend/src/styles/csvExport.css` (новый, при необходимости — импорт из компонента),
  отчёт `frontend/tests/T40-csv-export.md`.
- `frontend/src/i18n/ru.json` (ключи `admin.exportCsv`, `admin.exportCsvHint`, `admin.exported`,
  `admin.exportEmpty` уже добавлены), `frontend/src/api/**`, `Task/**`, `specs/**` — не трогать.
  Новых зависимостей нет.

## Отчёт

`frontend/tests/T40-csv-export.md`: что изменено, как проверено, ограничения (5000 строк,
разделитель/кодировка), честный список непроверенного.
