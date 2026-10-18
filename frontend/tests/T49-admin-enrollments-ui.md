# T49 — Админка: экран «Записи на курсы» (frontend). Отчёт

Дата: 2026-10-19. Ветка: `task/T49-admin-enrollments`. Волна: P6.
Контракт: `Task/00-api-contract.md` — `GET /admin/enrollments` (T25) +
новые параметры `q`/`status` (T48, бэкенд — параллельная задача волны) +
`POST .../revoke|unrevoke` (T16). Живой стенд и браузер не использовались —
визуальную приёмку делает координатор (по ТЗ это допустимо).

## Что сделано

### Файлы

| Файл | Что |
|---|---|
| `frontend/src/pages/AdminEnrollments.tsx` | новый экран `/admin/enrollments` |
| `frontend/src/api/adminEnrollments.ts` | новый API-модуль раздела (транспорт — `apiFetch`/`buildQuery` из `client.ts`) |
| `frontend/src/styles/adminEnrollments.css` | новые стили, все классы с префиксом `.ae-` |
| `frontend/src/app/router.tsx` | +2 строки: `lazy`-импорт и маршрут `/admin/enrollments` под `RequireRole roles={['admin']}` |
| `frontend/src/pages/Admin.tsx` | +1 строка: ссылка на экран в `admin-nav admin-nav--pages` |

Другие файлы не тронуты: `ru.json`, `client.ts`, `AdminUsers.tsx`,
`AdminCourses.tsx`, `AdminStats.tsx`, `package.json` — без изменений
(проверено `git status --short`).

### Экран (`AdminEnrollments.tsx`)

- Шапка как у AdminUsers: крошка-ссылка на `/admin` (`cabinet.adminTitle`),
  заголовок `adminEnrollments.title`, содержимое в `.panel`.
- Фильтры:
  - поиск `q` (по email/имени ученика) — debounce 300 мс, ввод возвращает на 1-ю страницу;
  - select курса: опции из `listAdminCourses({ limit: 100 })` (`adminConsole.ts`),
    отсортированы по названию; первый пункт — «Все» (`adminEnrollments.all`);
    если выбранный курс не попал в первые 100, в список добавляется пункт с его `id`
    (иначе фильтр «невидим» и его не снять — приём из `Admin.tsx`);
  - select статуса: «Все» + `statusActive|statusRefunded|statusRevoked`;
  - кнопка `adminEnrollments.reset` — очищает оба select и поиск, возвращает страницу 1;
    недоступна, когда фильтров нет.
- Таблица: ученик (имя + email; без имени — email), курс (`colCourse`),
  статус-бейдж (`colStatus`), прогресс (`colProgress`: полоса `.progress` +
  «N из M · P%»), дата (`colDate`, `formatDate`), действия (`colActions`).
- Действия по статусу: `active` → «Отозвать доступ» (`revoke`) с подтверждением
  (`confirmRevoke`); `revoked` → «Вернуть доступ» (`unrevoke`, `confirmUnrevoke`);
  `refunded` — без действий (прочерк «—»). Подтверждение — `window.confirm`
  (паттерн `AdminCourses.tsx`/T27): под вопросом второй строкой подставляются
  ученик и курс, т.к. ключи словаря без интерполяции.
- Мутации: `useMutation` + инвалидация по префиксу `['admin-enrollments']`;
  ответ (`status`) точечно применяется к кэшу (`setQueriesData`), чтобы кнопка
  не «откатывалась» до рефетча. После успеха — тост `revokedDone`/`unrevokedDone`
  (`role="status"`) и refetch. Ошибки — строком в строке (`role="alert"`,
  `errorText`: 404/400 — текст бэкенда, 401/403, сеть — словарь).
- На время запроса строка помечается `aria-busy`, визуально приглушается
  (`.ae-row--busy`), кнопка в ней `disabled`.
- Состояния: загрузка — `common.loading` (`role="status"`); ошибка списка +
  `common.retry`; пусто — `adminEnrollments.empty`; «Всего записей: N»
  (`adminEnrollments.total`); в пагинаторе — «Показано N–M из total»
  (`adminEnrollments.showing` + `admin.of`), кнопки `admin.prev`/`admin.next`.
- Пагинация: `page`/`limit=20`, `placeholderData: keepPreviousData` — старые
  строки остаются до прихода новых; пагинатор прячется только на пустой 1-й странице.
- Доступность: `<label>` у всех фильтров, `<th scope="col">`, `role="region"` +
  `aria-label` у обёртки таблицы, `role="progressbar"` с `aria-valuenow`/`aria-valuetext`
  у прогресса, `role="status"` у тоста, `role="alert"` у ошибок.
- Вёрстка (по образцу вылеченной в T52 таблицы пользователей): обёртка
  `.ae-table-wrap { overflow-x: auto }`; email переносится (`overflow-wrap: anywhere`,
  ячейка ≤15 rem), имя обрезается с многоточием (полное — в `title`), название
  курса переносится (ячейка ≤15 rem), колонка действий компактная (≥9.5 rem,
  кнопка целиком). Таблица `min-width: 960px` — при типовой ширине карточки
  (~1100 px) все колонки, включая действия, видны без прокрутки; на ≤720 px
  строки разворачиваются в карточки (`data-label`, как в соседних разделах).

### API (`adminEnrollments.ts`)

- Типы: `AdminEnrollmentStatus` (`active|refunded|revoked`), `AdminEnrollmentRow`
  (id, studentId, studentEmail, studentName, courseId, courseTitle, status,
  createdAt, completedLessons, totalLessons, percent), `AdminEnrollmentPage`
  (`{items,total,page,limit}`), `AdminEnrollmentStatusResponse`.
- `listAdminEnrollments({ q?, courseId?, status?, page?, limit? })` →
  `GET /admin/enrollments?...` (пустые значения `buildQuery` пропускает).
- `revokeEnrollment(studentId, courseId)` →
  `POST /admin/enrollments/{studentId}/courses/{courseId}/revoke`
  (без тела; ответ читается как `status`).
- `unrevokeEnrollment(studentId, courseId)` → тот же путь с `/unrevoke`.
- ID в путях кодируются `encodeURIComponent`; новых зависимостей и изменений
  транспорта нет.

## Ключи i18n (все — из замороженного `ru.json`, файл не менялся)

`adminEnrollments.*` — все 24 существующих ключа раздела: title, empty,
colStudent, colCourse, colStatus, colProgress, colDate, colActions, search,
filterCourse, filterStatus, all, statusActive, statusRefunded, statusRevoked,
revoke, unrevoke, confirmRevoke, confirmUnrevoke, revokedDone, unrevokedDone,
total, showing, reset.
Прочие: `common.loading`, `common.retry`, `common.error`, `cabinet.adminTitle`,
`cabinet.needRole`, `error.needLogin`, `error.network`, `admin.of`, `admin.prev`,
`admin.next`.

Полнота ключей проверена компилятором: `t()` принимает только ключи из `ru.json`,
`tsc --noEmit` зелёный.

### Литералы вне `t()`

1. `—` — прочерк для действия при статусе `refunded` (как пустое `displayName`
   в AdminUsers).
2. `·` и `%` в подписи прогресса, типографский тире `–` в диапазоне страниц —
   нетекстовые разделители/единицы.
3. Композиция вопроса `window.confirm`: перевод строки и `— «курс»` вокруг данных
   ученика/курса (интерполяции в словаре нет — тот же приём, что в T27).
4. В CSS: `content: attr(data-label)`, цвета — не текст UI.

## Проверка

```
cd C:\Users\96352\all_project\eduPlatform\wt\t49\frontend
npm ci --prefer-offline   # added 76 packages, and audited 77 packages (реестр доступен)
npm run build             # tsc --noEmit — 0 ошибок; vite build: ✓ 137 modules,
                          #   чанки AdminEnrollments-*.js (~8.6 КБ) и AdminEnrollments-*.css (~3.2 КБ)
npm test                  # tests 61 / pass 61 / fail 0 (контрактные тесты клиента)
```

Новых зависимостей в `package.json` нет (npm ci ставил строго по `package-lock.json`).

## Известные ограничения (честно)

1. **Фильтры `q`/`status` заработают после T48.** В замороженном коммите волны
   `GET /admin/enrollments` принимает только `courseId/studentId/page/limit`:
   `q` и `status` — аддитивные параметры задачи T48 (бэкенд, параллельная ветка).
   Фронтенд шлёт их с первого дня; до сборки веток P6 стенд будет молча
   игнорировать эти два фильтра (без 400 — лишние query-параметры бэкенд не
   отвергает).
2. **В select курсов — первые 100** (`limit=100`, новые первыми): более старый
   курс в фильтре не появится. Выбранный курс, не попавший в первые 100,
   показывается в select по `id` (чтобы фильтр можно было снять).
3. Живого прогона в браузере не было (по ТЗ T49 не обязателен): визуальные
   ширины/переносы и поведение `window.confirm` финально проверяет координатор.
4. Фильтры и страница живут в React-state, не в URL (как в AdminUsers/T26):
   ссылка на выборку не шарится, F5 сбрасывает фильтры.
5. Сортировка — серверная («новые первыми»), клиентской нет; CSV-выгрузки на
   экране нет (ТЗ T49 её не требует).
6. Мутации `revoke`/`unrevoke` — раздельные: две строки теоретически можно
   менять параллельно; одна и та же строка на время запроса блокируется.
7. `percent` для полосы зажимается в 0..100 (`clampPercent`, как в
   CourseStudents); в тексте показывается значение с сервера.
