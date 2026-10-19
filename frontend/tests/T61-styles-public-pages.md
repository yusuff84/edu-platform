# T61 — публичные страницы: `<main>` и стилевая уборка

**Волна:** P7 · **Ветка:** `task/T61-styles-public-pages` · **Дата:** 19.10.2026

## 1. Разметка: `<main className="page">` (только обёртка, контент не правлен)

| Файл | Что обёрнуто |
|---|---|
| `src/pages/Catalog.tsx` | Корневой `div` → `<main className="page catalog-page">` (hero, фильтры, сетка, промо-блоки, LeadBlock). `catalog-page` — локальный scoped-класс; без него `main.page` дал бы 30/48px полей (styles.css) и hero отлип бы от шапки: раньше его не было (корневой div без полей). |
| `src/pages/MyCourses.tsx` | Все 4 варианта рендера: loading / error / empty / основной — `div.page` → `main.page`. Полей не добавилось: класс `.page` был на этом же узле раньше. |
| `src/pages/Learn.tsx` | Все 7 вариантов: loading, error, no-access, course-loading, course-error, «нет уроков», основной (крошки + прогресс + панель «Курс пройден» + lesson-shell). Здесь же inline-style `margin: 14px 0 20px` заменён на класс `learn-progress` (мелкое class-дополнение, вид не менялся). |
| `src/pages/Profile.tsx` | `div.page` → `main.page`; email в `pf-facts` получил класс-модификатор `pf-email` (для ellipsis, см. стили). |
| `src/pages/Cabinets.tsx` | Только учительский кабинет: `TeacherCab` — `div.page` → `main.page` (заголовок и список курсов внутри). Локальные guard-состояния `RequireRole` (как и `AuthSkeleton` в RouteGuards) не тронуты — это транзитные скелетоны, не контент страницы. |
| `src/pages/Auth.tsx` | `Login` и `Register`: `div.page` → `main.page` (2 шт.). Разметка форм/панелей не менялась. |

Не тронуты (по инструкции): `Course.tsx` (T57), `Constructor.tsx`, `Admin*.tsx`, `CourseStudents.tsx`, `NotFound.tsx` (у них main уже есть: `admin`, `wrap`, `ac-page`, `ae-admin`, `au-admin`, `shell-center`, `course-students-page`).

Центрирование «как раньше»: у Catalog добавлен `padding: 0` (hero был заподлицо с шапкой, а `.page` даёт 30px сверху); у остальных страниц `.page` уже был в разметке, центр/отступы не изменились. Побочный эффект обёртки — селектор `.page p a:not(.btn)` (styles.css) перекрасил ссылку «консультант» в блоке дипломов; возвращено scoped-правилом `.catalog-page .diploma p a { color: inherit }` (hover — фирменный красный, как в styles.css).

## 2. Стили — только `catalog.css`, `learn.css`, `profile.css`

Глобальные `styles.css`/`shell.css` НЕ менялись. Все новые правила скоупятся корнем (`main.page` / `.catalog-page` / `.pf-*`) и переопределяют только перечисленные ниже.

### `styles/catalog.css` (плюс 128 строк, минус мёртвый блок)

**Удалено (мёртвое, греп по *.tsx — 0 попаданий):**
- `.catalog-search`, `.catalog-search input`, `.catalog-search button` — разметка пользуется `.search-row` из styles.css.

**Добавлено/изменено:**

| Селектор | Зачем |
|---|---|
| `.catalog-page { padding: 0 }` | Возврат вида «как до обёртки» (см. п.1) |
| `.catalog-page .diploma p a` / `...:hover` | Возврат цвета ссылки «консультант» после появления `.page p a` |
| `.catalog-page .support`, `.dreams`, `.diploma` → `padding-top: 48px` | Ритм промо-секций: был 44/56/60px разнобой, теперь единый шаг 48px (`.cta` и hero оставлены крупными — финальный акцент) |
| `.catalog-page .lead-zone { margin-top: 48px }` | Тот же шаг перед лид-формой (был 52px) |
| `.catalog-page .count-line { margin-left/right: 0 }` | Убраны случайные боковые поля 2px у «Найдено: N из M» |
| `.page .grid > * { display:flex; flex-direction:column; min-width:0 }` | Карточка — колонка; ряды одной высоты, длинный контент не распирает колонку |
| `.page .grid .tile { flex:none; height:300px }` | Плитка держит height 300px из styles.css внутри flex-колонки |
| `.page .grid .card-row { display:flex; justify-content:space-between; align-items:center; gap:8px; margin:8px 0 0 }` | Первый `.card-row` класса нет в styles.css (звёзды/отзывы и цена были вообще без стиля) — появился ритм; цена и счётчик отзывов разъехались по краям |
| `.page .grid .card-row:last-child { margin-top:auto }` | Цена прижата к низу карточки → низ ряда ровный |
| `.page .grid > p { margin:12px 0 4px; …clamp 3 строки }` | Превью описания не переполняет карточку (был UA-маржин 16px) |
| `.catalog-page .tile-title { clamp 3 строки, overflow:hidden }` | Длинные названия — многоточие; на ≤620px — 4 строки |
| `.catalog-page .catalog-skeletons` → 3/2/1 колонки (+2 media) | Скелетоны повторяли auto-fill 4 колонки против 3 карточек — теперь совпадают с сеткой на 1440/1024/768/360 |

### `styles/learn.css` (переписан: минус мёртвые селекторы, плюс защита ширины)

**Удалено (мёртвое от разметки T03 — греп даёт 0 попаданий):** `.learn-page` (и его фокус-правила), `.learn-grid` (+ media-блок 768px), `.learn-sidebar`, `.learn-module-title`, `.learn-sidebar section:first-child …`, `.learn-content`, `.lesson h3`, `.lesson a/button/textarea/input:focus-visible`. Разметка сейчас живёт на `.lesson-shell/.side-nav/.lesson-pane/.learn-link*`.

**Добавлено/изменено:**

| Селектор | Зачем |
|---|---|
| `.learn-progress { margin:14px 0 20px }` | Бывший inline-style из Learn.tsx («Ход обучения: N%» + полоса) |
| `.lesson-pane { min-width:0 }` + `.lesson-pane .panel { min-width:0 }` | Колонка урока не растягивается широкими `<pre>/<video>`: **сайдбар на узких экранах больше не «выпадает» из грида** и нет горизонтального скролла |
| `.lesson-pane video { width:100%; max-width:100% }` | Видео — аккуратно по ширине панели (aspect-ratio 16/9 из styles.css) |
| Фокус-правила: `.learn-link`, `.lesson-pane a/button/textarea/input:focus-visible` → зелёный outline | Возврат задуманной (T03) зелёной фокус-обводки на актуальной разметке; раньше сработал бы только `.learn-link` |

Сохранено как было: `.learn-link*`, `.learn-notice`, `.md-*`, `.quiz`, `.code-*`, `.panel.learn-finished`. Адаптив сетки — styles.css (`.lesson-shell` → 1fr на ≤960, sticky гасится): при узком экране программа уроков идёт первой, отдельного сброса не требуется.

### `styles/profile.css` (плюс 41 строка)

| Селектор | Зачем |
|---|---|
| `.cab-stats > div { box-shadow: var(--shadow-sm) }` | Метрики «Курсов / Средний прогресс / Завершено» — та же тень, что у `.panel` записей: единый стиль страницы /me (радиус 18px уже совпадал) |
| `.enrolled-grid > .panel { display:flex; flex-direction:column; min-width:0 }` | Карточки курса — колонка, высоты в ряду ровные; длинный контент не распирает сетку |
| `.enrolled-grid .panel h3 { min-width:0 }` + `.enrolled-grid .panel h3 a { clamp 2 строки }` | Длинные названия курса — многоточие, сетка /me не ломается |
| `.enrolled-grid .btn-row { margin-top:auto; padding-top:10px }` | Кнопки «Продолжить» прижаты к низу карточки |
| `.pf-email { overflow:hidden; text-overflow:ellipsis; white-space:nowrap }` | EMAIL/РОЛЬ/ДАТА: длинный email — одна строка с многоточием, не переносит блок и не сдвигает историю платежей |

История платежей (`.pf-table-wrap/.pf-table/.pf-badge*/.pf-pager` + карточный адаптив ≤720px) уже была выровнена — не менялась.

## 3. Замеры (расчёт по CSS, фактическая сетка mock-данных из 12 курсов)

Браузерной приёмки в сессии не было (desktop browser не подключён), заменяющие замеры — расчёт по правилам styles.css/catalog.css: `.wrap` = min(1200px, 100%−56px), gap 20px.

| Ширина | Колонки сетки | Ширина карточки | Проверка |
|---|---|---|---|
| 1440 | 3 | (1144−40)/3 ≈ 368px | ок; скелетоны теперь тоже 3 (было 4) |
| 1024 | 3 | (968−40)/3 ≈ 309px | ок |
| 768 | 2 | (712−20)/2 ≈ 346px | ок (брейкпоинт styles.css 960px) |
| 360 | 1 | 304px | ок (брейкпоинт styles.css 620px) |

- Высота карточек каталога: ряд stretch, цена прижата `margin-top:auto` → нижняя кромка ровная; описание — clamp 3 строк; название — clamp 3 (4 на 360px): mock-курс «Оочень длинное одно слово…» и «Курс с очень длинным заголовком…» не выпирают.
- Ритм: mini-head 34 → count-line 26 → сетка 26/10 → pager 16 → support/dreams/diploma по 48 → lead 48 (CTA-блок 84/96 остаётся «паузой»).
- /me: 3 метрики; на ≤620px сетка 2×(146px) — не ломается; карточки — flex-колонки с кнопками у нижней кромки.
- /learn: >960px — сайдбар 300px (sticky top 20), урок 1fr; ≤960px — стек (программа сверху). С `.lesson-pane { min-width:0 }` даже `<pre>`-строкой-без-пробелов страница не уводится в горизонтальный скролл (проверка по сценариям длинных md-pre).
- /profile: `pf-facts` auto-fit minmax(200px,1fr) → при 1440 3 ячейки в 1 ряд (~340px каждая), email ellipsis; ≤360px — стопки; таблица платежей ≤720px → карточки с подписями data-label (без изменений).

## 4. Проверки

- `npm install` — чисто (node_modules до этого отсутствовали).
- `npm run build` (tsc --noEmit + vite build) — **EXIT=0**, 138 модулей.
- `npm test` — **61/61 зелёный**.
- Diff ограничен списком задачи: 6 tsx-страниц + 3 css (+ этот отчёт). `styles.css`, `shell.css`, admin/constructor-файлы, `CourseCard.tsx` не тронуты.

## 5. Что осталось (кривые места, чужие файлы — словами)

1. **Topbar `.user-email`** (`styles.css`/shell, T60): длинный email в шапке не имеет ellipsis — при очень длинном email может распирать `top-actions`. Полечить надо в shell.css.
2. **`/teach`**: статус-строки `l-item` тоже не имеют ellipsis на длинных названиях курсов (`.l-item` — flex, ссылка не сжимается до 0). Правится только в global styles.css — вне моего списка.
3. **Курсовые стили** (`.course-banner`, `.course-module`, `.reviews`, `.review-form`, `.rating-group`, `.card-row` у отзывов) живут в моём `catalog.css`, но рисуют страницу `/courses/:slug` — их пейдж T57: при очистке сознательно сохранены 1:1, включая все `course-*`. Если T57 переносит отзывы в отдельный css — блок «Отзывы» можно будет вырезать из catalog.css.
4. **`.tile` height 300px фиксированная**: карточкам не хватает «дышащего» относительного роста на очень узких экранах (плитка выше текста). Оставлено как есть — это styles.css, а clamp уже спасает от переполнения.
5. **Catalog — «отзывы»**: отдельного блока отзывов на главной нет (отзывы только на странице курса) — ритм-задача выполнена по фактическим секциям (hero/промо/фильтры/сетка/лид-форма).

## 6. Окружение

- На момент финиша на диске C: практически не осталось места (<0.05 ГБ) — последние dev-прогоны vite для визуальной проверки не смогли записать кэш `(node_modules/.vite)`. Сборка `npm run build` и `npm test` прошли до этого и зелёные; артефакты в репозиторий не попали. Координатору: может потребовать почистить диски перед волной P8.
- Временный мок-бэкенд/структурные серверы проверки были запущены на портах 3007/3009/8099 и после проверки остановлены; чужие процессы (3001/8082 других задач) не трогались.
