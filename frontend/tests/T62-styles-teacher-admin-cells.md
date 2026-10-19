# T62 — Стили: кабинет учителя, конструктор и админ-записи (локальные css)

**Волна:** P7  **Ветка:** `task/T62-styles-teacher-admin-cells`  **Worktree:** `wt/t62`

## Проверка

- `npm install` — node_modules присутствовал, установка не требовалась.
- `npm run build` — ✓ 0 ошибок (tsc --noEmit + vite build).
- `npm test` — ✓ 61/61 (api-contract, `node --test`).

## Что изменено

### 1. ADMIN ENROLLMENTS — ячейка ученика `/admin/enrollments` (фикс измеренной проблемы)

Замер координатора: `td.ae-td-student` 253px, а `.ae-student` / `.ae-student__email`
309–321px с overflow visible — текст вылезал из ячейки. Фикс: email и имя —
каждая в **одну строку** с `text-overflow: ellipsis` (полное значение — по
`title` в разметке), ширина ограничена **двумя независимыми уровнями**, чтобы
гарантия не зависела от причуд авторазметки таблицы:

1. жёсткий cap на самом элементе: `max-width: 14rem` + `overflow: hidden` —
   содержимое не может рисоваться дальше 224px от левого края ячейки
   (элементный overflow всегда считается в пределах самого бокса);
2. трек `minmax(0, 1fr)` у контейнера + `min-width: 0` + `max-width: 100%` +
   `max-width: 15rem` у td — колонка не раздувается до длины текста.

Классы T49/T52 не переименовывались и не удалялись (`.ae-td-student`,
`.ae-student`, `.ae-student__name`, `.ae-student__email`) — только точечные
декларации внутри той же системы. В разметке TSX правок не потребовалось:
атрибуты `title` (имя/email) уже стоят (T49).

Горизонтальный скролл самой таблицы внутри `.ae-table-wrap` сохранён как был
(допустим); вылезание содержимого за пределы td устранено на уровне боксов.

### 2. COURSE STUDENTS — `/teach/:id/students`

- **Плитки метрик одной высоты**: `.cs-stats` получил `grid-auto-rows: 1fr`
  (все ряды сетки одной высоты), `.cs-stat` переведён на flex-колонку с
  `min-width: 0`; числам задан `line-height: 1.2` + одностороння строка с
  ellipsis (защита от «прыжков», когда денежное значение длинное).
- **Поиск + таблица — единым блоком**: в разметке добавлена обёртка
  `<div className="cs-list-panel">` вокруг поиска, пустого состояния,
  таблицы и пагинации (только обёртка — маршруты/состояния/логика не тронуты).
  Плитки метрик остались *вне* этой карточки.
- **Таблица учеников**: имя/email — односторонняя строка с многоточием
  (`title` — полный текст); та же двухуровневая гарантия ширины, что и в
  п.1. В разметке добавлены `title`-атрибуты на `<b>` и `<span class="muted">`
  (только презентация, без логики).
- Узкий экран (≤720px, режим карточек): имя/email возвращаются к переносу
  (`white-space: normal` + `overflow-wrap: anywhere`), односторонняя строка —
  только для десктопной таблицы. То же в админ-записях.

### 3. КОНСТРУКТОР — панели, заголовки, кнопки

- **Паддинги панелей**: конфликт `.panel` (24px, styles.css) против
  `.teach-section` (16px) зафиксирован одним значением `1.25rem 1.5rem`,
  вертикальный ритм `1.25rem 0` (как у системных секций).
- **Заголовки**: у всех панелей х2 — `21px` и `margin: 0 0 12px`;
  контент сразу после заголовка (форма/подсказка/список модулей) больше не
  добавляет собственный верхний `margin ~1em` — начало контента во всех
  панелях («Название курса», «Отправка на модерацию», «Добавить модуль»,
  «Урок»/«Программа») на одном расстоянии от заголовка.
- **Высота панелей**: последний блок панели больше не несёт нижний margin —
  воздух внизу задаётся паддингом секции («высота панелей сомнительна»).
- **Шапка редактора урока**: `.ce-editor-head` получил `margin: 0 0 12px`
  (у заголовка х2 внутри flex-строки margin 0 — зазор держит шапка), чтобы
  отступ «заголовок → контент» был как в других панелях.
- **Кнопки перемещения/рядов — один размер**: голые нативные кнопки
  («вверх/вниз», удаление, переименование, «Тесты», варианты) приведены к
  единой геометрии `40px / radius 12 / 8px 16px / 13.5px` (`.teach-section
  button:not(.btn)`); первичные `.btn` в тех же рядах — той же геометрии
  (иерархию несёт цвет `btn-primary`); ссылка-кнопка «Редактировать»
  (`.teach-link-btn`) и «Публичная страница» (`.ce-public-link`) и кнопки
  жизненного цикла (`.ce-lifecycle button`) — той же высоты/радиуса.
- **Destructive-кнопки**: усиленный селектор перекрывал красный контур
  `.teach-danger` — добавлены равноспецифичные повторения
  (`.teach-section button.teach-danger` + hover красным заливом + disabled).
- **Заголовок блока «Тесты»** в код-задаче — та же типографика, что у
  заголовков вопросов (`.ce-tests-box h3`, 0.95rem, зазор 0.5rem); в
  разметке Constructor.tsx добавлен только className `ce-tests-box`
  (логика не тронута).
- Подписи полей форм (`.teach-section .form-grid > label`) — единые 13px/700.

## Таблица: селектор → что изменено → где нужна визуальная проверка

| Селектор | Что изменено | Страницы для визуальной проверки |
| --- | --- | --- |
| `main.ae-admin` *(без изменений)* | — | `/admin/enrollments` |
| `.ae-table th, .ae-table td` *(без изменений)* | nowrap-база таблицы сохранена | `/admin/enrollments` |
| `.ae-td-student` | без новых деклараций (min-width 11rem / max-width 15rem как в T49) | `/admin/enrollments` |
| `.ae-student` | + `grid-template-columns: minmax(0,1fr)`, `min-width: 0`, `max-width: 100%` | `/admin/enrollments` |
| `.ae-student__name` | + `min-width: 0`; `max-width: 14rem`; односторонняя строка + ellipsis (title из T49) | `/admin/enrollments` |
| `.ae-student__email` | вместо переноса (`overflow-wrap/word-break`) — `white-space: nowrap` + `overflow: hidden` + `text-overflow: ellipsis` + `max-width: 14rem` | `/admin/enrollments` |
| `@media ≤720px: .ae-student__name/.ae-student__email` | в карточном режиме возвращён перенос (`white-space: normal`, `overflow-wrap: anywhere`, `overflow: visible`) | `/admin/enrollments` на телефоне |
| `.cs-stats` | + `grid-auto-rows: 1fr` | `/teach/:id/students` |
| `.cs-stat` | flex-колонка, `gap: 4px`, `min-width: 0` | `/teach/:id/students` |
| `.cs-stat b` | `line-height: 1.2` + nowrap/hidden/ellipsis числа | `/teach/:id/students` |
| `.cs-stat span` | `font-size: 12.5px`, `line-height: 1.4` | `/teach/:id/students` |
| `.cs-list-panel` (НОВЫЙ) | единая карточка «поиск + пустое состояние + таблица + пагинация»; обёртка в TSX | `/teach/:id/students` |
| `.cs-list-panel > .cs-empty` | padding 40px 16px (компактнее внутри карточки) | `/teach/:id/students` (пустой курс) |
| `.cs-table-wrap` | `border-radius: r-sm` (внутри карточки) | `/teach/:id/students` |
| `.cs-td-student` | + `max-width: 15rem` | `/teach/:id/students` |
| `.cs-student` | flex-колонка → grid `minmax(0,1fr)`, `min-width: 0`, `max-width: 100%` | `/teach/:id/students` |
| `.cs-student b` | односторонняя строка + ellipsis, `max-width: 14rem` (title из TSX) | `/teach/:id/students` |
| `.cs-student .muted` | односторонняя строка + ellipsis, `max-width: 14rem`, цвет `--muted` (title из TSX) | `/teach/:id/students` |
| `@media ≤720px: .cs-student b/.muted` | перенос восстановлен в карточном режиме | `/teach/:id/students` на телефоне |
| `.teach-banner` | `margin: 1.25rem 0` (ритм секций) | `/teach/:id` (отклонённый курс) |
| `.teach-section` | `padding: 1.25rem 1.5rem`, `margin: 1.25rem 0` — фикс конфликта с `.panel` (24px) | `/teach/:id`, `/teach/new` |
| `.teach-section > h2` | `margin: 0 0 12px`, `font-size: 21px` — одна типографика заголовков | `/teach/:id`, `/teach/new` |
| `.teach-section > h2 + form/p/ol/.teach-card-actions/.teach-note` | `margin-top: 0` — начало контента на одном расстоянии | `/teach/:id`, `/teach/new` |
| `.teach-section > h2 + p.teach-note/p.muted` | `margin: 0 0 .25rem` (подсказка-подпись) | `/teach/:id` (замороженная карточка, пустая программа) |
| `.teach-section > :last-child` | `margin-bottom: 0` — единая высота воздуха внизу панелей | `/teach/:id`, `/teach/new` |
| `.teach-section .form-grid > label` (НОВОЕ) | подписи полей 13px/700/muted | `/teach/:id`, `/teach/new` (карточка, модуль, урок) |
| `.teach-section button:not(.btn)` (НОВОЕ) | единая геометрия рядовых кнопок: 40px / radius 12 / 8px 16px / 13.5px | `/teach/:id` (ряды модулей, уроков, quiz, тестов), `/teach/new` |
| `.teach-section button:not(.btn):hover/:disabled` (НОВОЕ) | hover-фон, disabled 0.55 | те же ряды |
| `.teach-card-actions button` | 40px / 8px 16px / radius 12 — ряды с `.btn`, сохраняющая высоту («Отмена» рядом с «Сохранить») | `/teach/:id`, `/teach/new` |
| `.teach-rename-row button` | та же геометрия в строке переименования | `/teach/:id` (переименование модуля) |
| `.teach-section .btn` (НОВОЕ) | первичные кнопки к той же геометрии (цветовой иерархии не изменено) | `/teach/:id`, `/teach/new` |
| `.teach-link-btn` | 40px / radius 12 / 8px 16px / 13.5px — как соседние кнопки | `/teach/new` (карточка курса) и `/teach/:id` |
| `.teach-section button.teach-danger` (+hover/disabled) (НОВОЕ) | красный контур/залив restored против усиленной базовой кнопки | `/teach/:id` (удаление карточки/модуля/урока), `/teach/new` |
| `.teach-lesson-list` | `margin: 0.25rem 0 0`, `gap: 0.4rem` | `/teach/:id` (программа курса) |
| `.ce-lifecycle button` + hover/disabled (НОВОЕ) | та же геометрия 40px/12 у кнопок жизненного цикла | `/teach/:id` (draft/review/published/archived) |
| `.ce-public-link` |как `.teach-link-btn` (40px / 12) | `/teach/:id` (published) |
| `.ce-editor-head` | `margin: 0 0 12px` — единый зазор «заголовок → контент» | `/teach/:id` (панель урока) |
| `.ce-tests-box h3` (НОВОЕ, класс добавлен в TSX) | заголовок «Тесты» 0.95rem как `.teach-q-box h3` | `/teach/:id` (код-задача) |

## Разметка (минимальные правки, без логики)

- `frontend/src/pages/CourseStudents.tsx`:
  - обёртка `<div className="cs-list-panel">` вокруг toolbar/empty/table/pager
    (протянуто раньше: фикс «поиск+таблица единым блоком»);
  - `title` на `b` (имя) и `span.muted` (email) в таблице учеников.
- `frontend/src/pages/Constructor.tsx`:
  - только `className="teach-focusable ce-tests-box"` у блока тестов код-задачи.

## НЕ тронуты (по договору)

`styles.css`, `shell.css`, `catalog.css`, `learn.css`, `profile.css`,
`adminUsers.css`, `admin.css`, `adminConsole.css`, `csvExport.css`,
`AdminStats/AdminCourses/AdminCourseDetail/AdminUsers.tsx`, `ru.json`,
`client.ts`, `backend/**`, `Task/**`, `specs/**`.

## Примечания для координатора (browserreview)

Замер против 1495px подтверждается косвенно; если после выката будут найдены
новые slot' по подбору в моей зоне — только ДОБАВИТЬ в отчёт, не менять:
- `/admin/enrollments`: email в одну строку с «…» и `title`; вылезание за td
  отсутствует; горизонтальный скролл таблицы — только внутри `.ae-table-wrap`.
- `/teach/:id/students`: плитки метрик одной высоты; поиск+таблица в одной
  карточке (`cs-list-panel`); имя/email ученика с «…» и `title`.
- Конструктор `/teach/:id` (+ `/teach/new`): панели с одинаковыми
  паддингами/заголовками, у всех рядов кнопки одной высоты; проверить
  взглядом разницу до/после в «Название курса» vs «Программа курса» (загол-
  овки/паддинги), кнопки жизненного цикла, панель урока с предпросмотром
  (зазор над формой, панель справа), строка переименования модуля,
  кнопки перемещения уроков (вверх/вниз).

## Как проверить локально

```
cd frontend
npm run build   # ✓ 0 ошибок
npm test        # ✓ 61/61
```
