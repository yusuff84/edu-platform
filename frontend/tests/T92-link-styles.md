# T92 — глобальные стили текстовых ссылок

**Ветка:** `task/T92-link-styles`  
**Worktree:** `wt/t92`  
**База:** `integration/p10`

## Что сделано

Единое поведение обычных текстовых ссылок: в покое ссылка наследует цвет
родительского текста и не подчёркнута; при `:hover` и `:focus-visible` —
фирменный акцент `var(--brand)` и подчёркивание с отступом 3px. Для
отличимости без постоянного underline добавлен `font-weight: 600`
(курсор `pointer` у ссылок остаётся браузерным). Кнопкообразные ссылки
(`.btn`), nav, карточки и ссылки конструктора не затронуты.

### До / после по правилам

| Файл | Правило | Было | Стало |
|---|---|---|---|
| `frontend/src/styles.css` (140–158) | `.page p a:not(.btn)`, `.page li a:not(.btn)`, `.empty a:not(.btn)` | `color: var(--cta-blue)`, постоянный `underline`, hover `#1f3ae0` | `color: inherit; font-weight: 600; text-decoration: none; text-underline-offset: 3px`; hover/focus-visible — `var(--brand)` + `underline` |
| `frontend/src/styles.css` (1776–1792) | `.perk p a`, `.perk.dark p a` | постоянный `underline`; у тёмной карточки hover белый | без underline в покое; hover/focus — `var(--brand)` + underline, у `.perk.dark` — `#fff` + underline |
| `frontend/src/styles.css` (1958–1968) | `.diploma p a` | постоянный `underline`; hover `var(--brand)` | без underline в покое; hover/focus — `var(--brand)` + underline |
| `frontend/src/styles/shell.css` (40–51) | `.shell-center p a` | `var(--cta-blue)`, постоянный underline, hover `#1f3ae0` | `color: inherit`, без underline в покое; hover/focus — `var(--brand)` + underline; `font-weight: 600` сохранён |
| `frontend/src/styles/catalog.css` (26–36) | `.catalog-page .diploma p a` | `color: inherit`, hover `var(--brand)`; underline наследовался от общего правила | явно `text-decoration: none`; hover/focus — `var(--brand)` + underline |
| `frontend/src/styles/promo.css` (128–147) | `.promo-toggle` | постоянный underline | `text-decoration: none`; hover/focus — `var(--brand-dark)` + underline |
| `frontend/src/styles/promo.css` (199–217) | `.promo-remove` | постоянный underline | `text-decoration: none`; hover/focus — `var(--danger)` + underline |
| `frontend/src/styles/reviews-extra.css` (63–80) | `.reviews-reset` | underline только на hover (оставалось как было) | явно `text-decoration: none` + `text-underline-offset: 3px`; underline также на `:focus-visible` |

### Исключения и совместимость с T91

- `.page li` больше не применяется к карточкам конструктора:
  селектор `.page li:not(.teach-card) a:not(.btn):not(.teach-link-btn)`.
  Это сохраняет стили `.teach-card` / `.teach-link-btn` из
  `constructor.css`, который правит T91; сам `constructor.css` и
  `constructorExtra.css` не изменялись.
- `.btn` исключён во всех общих текстовых правилах, поэтому кнопки-ссылки
  (включая `.btn-primary`/`.btn-ghost`) не меняются.
- Nav, side-nav, admin-nav, footer, карточки курсов (`CourseCard`,
  `.enrolled-grid`, `a.sr-only`) не затрагивались: у них собственные
  правила цвета/оформления.
- `admin*.css` не изменялись. Ссылки-назад в админке (`.acd-back`,
  `.ae-back`, `.au-back`) теперь наследуют цвет текста вместо синего —
  это следствие общего правила, отдельные admin-стили не переопределялись.
- `profile.css` не изменялся: постоянного underline/синего цвета ссылок
  там не найдено (карточная ссылка `.enrolled-grid .panel h3 a` имеет
  собственное оформление и не является текстовой ссылкой в абзаце).

### Доступность

- `:focus-visible` не убирался: общее правило `a:focus-visible` /
  `button:focus-visible` в `styles.css` даёт outline 3px
  (`--focus-ring-color`) с offset 2px; у `.reviews-reset` остаётся свой
  outline. Для текстовых ссылок focus-visible дополнительно включает
  underline и акцентный цвет.
- Ссылки отличимы от обычного текста: курсор `pointer`, `font-weight: 600`,
  смена цвета и underline при hover/focus.
- Контраст: `#141414`/`#6f6e6a` в покое — 18.4:1 / 4.6–5.1:1 (AA/AAA).
  Акцент `var(--brand)` `#e63223` на белом — 4.33:1, на `--bg` `#f4f3ef` —
  3.90:1 (чуть ниже 4.5:1 для обычного текста; это существующий фирменный
  цвет проекта). Для текстовых кнопок, где важен именно читаемый текст,
  hover использует более тёмные `--brand-dark` (6.52:1) и `--danger`
  (4.52:1). Остаточный риск по акценту зафиксирован ниже.

## Проверка

Из `frontend/`:

```
npm ci
npm run build
```

- `npm ci` — зависимости в worktree отсутствовали, установлены по
  `package-lock.json` (76 пакетов).
- `npm run build` — успешно: `tsc --noEmit` без ошибок, Vite собрал
  production-бандл (160 модулей, `dist/assets/index-*.css` 49.74 kB).
- Статическая проверка собранного CSS: в `dist/assets/*.css` найдены
  правила с `text-decoration:none` в состоянии покоя и `underline` только
  в блоках `:hover` / `:focus-visible`:
  `.page p a:not(.btn)`, `.page li:not(.teach-card) ...`,
  `.empty a:not(.btn)`, `.perk p a`, `.perk.dark p a`,
  `.diploma p a`, `.catalog-page .diploma p a`, `.shell-center p a`,
  `.promo-toggle`, `.promo-remove`, `.reviews-reset`.
- Проверка исходников: во всех затронутых файлах `text-decoration:
  underline` остался только внутри hover/focus-visible правил; в
  `styles.css`/`shell.css` больше нет `var(--cta-blue)` и `#1f3ae0` для
  обычных текстовых ссылок (переменная и синие кнопки `.btn-blue` остались
  для кнопкообразных элементов).
- DOM-аудит: ссылки в `.page p` (описание курса, логин в отзывах/Q&A,
  `acd-back` и т.п.), `.perk p`, `.diploma p`, `.empty`, `.shell-center p`
  попадают под новые правила; `.btn`, `.teach-link-btn`, `.teach-card`,
  nav/footer/карточки — нет.

## Риски

1. **Контраст hover-акцента.** `var(--brand)` на белом/`--bg` даёт
   4.33:1 / 3.90:1 — формально ниже AA 4.5:1 для обычного текста.
   Ссылка остаётся различимой за счёт underline, веса 600 и focus-outline,
   но если потребуется строгий AA для hover, стоит заменить акцент
   текстовых ссылок на `--brand-dark` (6.52:1) отдельной задачей.
2. **Визуальная проверка.** Полноценный браузерный прогон в worktree не
   выполнялся (браузер к сессии не подключён); проверка сделана по
   собранному CSS и разметке. Стоит пройти каталог, 404 и чекаут глазами
   после сборки интеграционной ветки.
3. **T91.** Из селектора списков исключены `.teach-card` и
   `.teach-link-btn`; если T91 добавит текстовые ссылки внутри `.teach-card
   p`, общее правило `.page p a` может на них повлиять — при слиянии
   проверить карточки конструктора.
4. **`font-weight: 600`** у текстовых ссылок может незначительно изменить
   переносы в длинных абзацах; это осознанная плата за отличимость ссылки
   без постоянного подчёркивания.
