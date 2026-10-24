# T91 — Стили: разделы квиза и кода в конструкторе

**Волна:** P10  **Ветка:** `task/T91-constructor-styles`  **Worktree:** `wt/t91`

## Итог

Разделы `lesson.kind === 'quiz'` и `lesson.kind === 'code'` приведены к общему
ритму конструктора: компактные действия 32px вместо «гигантских» 40px,
единые поля `.input`/`.select`, сетка строки варианта, карточки тестов с
заголовком и рядом действий, аккуратный перенос на узком экране. Бизнес-логика,
валидация, API-вызовы и i18n-ключи не менялись.

## Корневые причины

1. **Слишком широкое `.teach-section button:not(.btn)`** (`constructor.css`,
   ~127) задавало всем вложенным кнопкам `min-height: 40px`, `padding: 8px 16px`,
   `border-radius: 12px`. В строке варианта оказывалось 5–8 таких кнопок
   («выше/ниже/удалить» × варианты + «добавить»), из-за чего строка распирала
   карточку, а кнопки визуально доминировали над полем.
2. **Поля quiz/code были «нативными»**: `input/select/textarea` в этих блоках
   не имели классов `.input`/`.select`, поэтому получали браузерную высоту,
   рамку и `width: auto`, а не общую геометрию проекта.
3. **Подписи и fieldset**: `.teach-section .form-grid > label` стилизовал
   только прямые дочерние label формы; вложенные label вопросов/тестов и
   `legend` оставались с браузерной типографикой. `fieldset` имел собственный
   `margin`, а строка варианта — `flex` с `label { flex: 1 }`, из-за чего
   подпись и input «разъезжались».
4. **Карточка теста была плоским стеком** без заголовка; действия лежали в
   конце карточки отдельным рядом и тоже были 40px.
5. **Коллизия классов превью**: `LessonPreview` уже использует `.ce-quiz` и
   `.ce-code`. Чтобы маркеры формы не подхватили стили списка превью, правила
   превью в `constructorExtra.css` заскоуплены `.ce-preview-pane`, а форма
   получила маркеры `.ce-quiz`/`.ce-code` без конфликта.

## Что изменено

### `frontend/src/pages/Constructor.tsx` (только разметка/классы)

- Контейнер квиза: `teach-focusable ce-quiz`; контейнер кода: `ce-code`.
- Карточки: `teach-q-box ce-q-box`, `teach-test-box ce-test-box`.
- Компактные действия: `ce-btn-sm`, удаление — `ce-btn-sm ce-btn-danger`
  (вопросы, варианты, тесты).
- Кнопки добавления: `ce-add` (обычные вторичные, 40px).
- Строка варианта: действия обёрнуты в `span.ce-option-actions`
  (`radio | label | input | действия`).
- Тест: шапка `ce-test-head` с `ce-test-title` («Тест N») и
  `ce-reorder ce-test-actions`; действия перенесены из низа карточки в шапку.
- Поля quiz/code получили общие классы `.input`/`.select`
  (вопрос, варианты, пояснение, язык, шаблон, поля тестов).
- Логика `moveQuestion/moveOption/moveTest/remove*`, обработчики, валидация,
  id/name/aria — без изменений.

### `frontend/src/styles/constructor.css`

- Базовый селектор сужен:
  `.teach-section button:not(.btn):not(.ce-btn-sm)` — остальные кнопки
  конструктора (обложка, промокоды, структура, lifecycle, карточка) сохранили
  геометрию T62 (40px / radius 12 / 8×16).
- `.ce-btn-sm`: 32px высоты, `padding: 0 10px`, radius 8, 12.5px/700,
  `white-space: nowrap`; hover/disabled/`focus-visible`; `.ce-btn-danger` —
  красный контур и красная заливка на hover.
- Карточки `.teach-q-box`/`.teach-test-box`: `border: 1px solid var(--line)`,
  radius 8, padding 12, margin 10, `background: var(--surface)`.
- Подписи внутри карточек — 13px/700/`var(--muted)`, как
  `.form-grid > label`; `legend` — та же типографика; `fieldset` без рамки.
- Поля: `width: 100%`, `max-width: 100%`, `box-sizing: border-box`,
  `min-width: 0`; высота/радиус/граница — из общих `.input`/`.select`.
- Строка варианта: grid
  `auto minmax(72px, max-content) minmax(0, 1fr) auto` (radio | label | input |
  действия), без переносов на десктопе.
- `focus-visible` для полей/кнопок/групп квиза и кода — общий
  `--focus-ring-color`; `.input:focus` box-shadow подавлен, чтобы не было
  двух колец. `disabled` полей — из `.input:disabled`, компактных кнопок —
  `opacity: .5`.
- `@media (max-width: 760px)`: radio и подпись в первой строке, input — во всю
  ширину, действия — отдельной строкой справа с переносом при нехватке места.
- `min-width: 0` у `.ce-quiz`/`.ce-code` и карточек — поля не выходят за
  контейнер/панель.

### `frontend/src/styles/constructorExtra.css`

- Превью-селекторы `.ce-quiz*` заскоуплены `.ce-preview-pane` (вид превью не
  изменился, редактор больше не получает `display: grid` списка превью).
- `.ce-reorder`: `gap: 6px`, `flex: 0 0 auto`.
- Новые `.ce-option-actions`, `.ce-q-head` (с разделителем), `.ce-test-head`,
  `.ce-test-title`, `.ce-test-actions`.
- Удалён конфликтующий `.ce-option-row { flex-wrap: wrap }` — раскладку строки
  теперь задаёт grid в `constructor.css`; `.ce-option-row` остаётся
  аддитивным маркером.

## Проверка DOM/селекторов (без скриншотов)

Статический аудит по AST TypeScript (внешний временный скрипт, в репозиторий
не коммитился) разобрал JSX `Constructor.tsx` и проверил поддеревья
`.ce-quiz`/`.ce-code`, а также наличие/свойства CSS-правил:

- обёртки `.ce-quiz` и `.ce-code` найдены;
- кнопок в quiz-блоке — 8, в code-блоке — 4; все имеют либо `ce-btn-sm`
  (компактные), либо `ce-add` (добавление);
- 9 компактных кнопок, 3 кнопки добавления; удаления используют
  `ce-btn-danger`;
- все текстовые `input`, `select`, `textarea` в quiz/code имеют
  `.input`/`.select` (radio/checkbox исключены);
- маркеры `ce-q-box`, `ce-option-row`, `ce-option-actions`, `ce-test-box`,
  `ce-test-head` присутствуют;
- CSS: старый широкий селектор отсутствует; `.ce-btn-sm` — 32px;
  `.teach-option-row` — grid с колонками
  `auto minmax(72px, max-content) minmax(0, 1fr) auto`;
  `.ce-test-head` и `.ce-option-actions` объявлены; превью `.ce-quiz`
  заскоуплено `.ce-preview-pane`.

Итог аудита: `ALL CHECKS PASSED`.

## Сборка и тесты

- `cd frontend && npm ci` — `node_modules` в worktree отсутствовал,
  установлено 76 пакетов (lock-файл не менялся).
- `cd frontend && npm run build` — ✓ `tsc --noEmit` + `vite build`, 0 ошибок.
- `npm test` в этой среде падает на запуске `node --test tests/api-contract/`
  (Node 22.16/Windows принимает каталог за модуль:
  `Cannot find module ...\tests\api-contract`). При явном перечислении файлов
  `node --test <все *.test.mjs>` — ✓ 193/193. К T91 отношения не имеет,
  `package.json` не менялся (не входит в разрешённые файлы).

## Риски

- Селектор кнопок сужен только исключением `.ce-btn-sm`; все прочие кнопки
  конструктора продолжают получать 40px-геометрию T62. Риск регрессии вне
  quiz/code минимален.
- `.ce-quiz`/`.ce-code` — общие имена с превью; конфликт снят скоупом
  `.ce-preview-pane`. Если компонент `LessonPreview` когда-нибудь отрендерят
  вне `.ce-preview-pane`, стили списка превью не применятся — это осознанное
  сужение, а не ошибка формы.
- На экранах ≤760px действия варианта уходят на отдельную строку; на очень
  узких экранах (≈320px) три компактные кнопки могут переноситься внутри
  `.ce-option-actions` — это предусмотрено.
- Визуальная проверка скриншотами не выполнялась по условию задачи; проверены
  DOM-структура, классы и CSS-селекторы. Финальный browserreview —
  за координатором.
- Новых i18n-ключей нет: «Тест N» — литерал рядом с уже существующими
  литералами «Тесты»/«Добавить тест»; `ru.json` не менялся.

## Файлы

- `frontend/src/pages/Constructor.tsx`
- `frontend/src/styles/constructor.css`
- `frontend/src/styles/constructorExtra.css`
- `frontend/tests/T91-constructor-styles.md`
