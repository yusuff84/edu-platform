# T92 — Стили: ссылки без постоянного подчёркивания и «сильного» цвета

**Волна:** P10  **Ветка:** `task/T92-link-styles`  **Worktree:** `wt/t92`

## Суть

В разных местах ссылки до сих пор выглядят как стандартные браузерные: синий
цвет и постоянное подчёркивание. Это ломает спокойный визуальный язык проекта.
Нужно единое поведение: в покое ссылка наследует цвет текста и не подчёркнута;
при hover/focus — мягкий акцент и подчёркивание; кнопкообразные ссылки
(`.btn`, nav, карточки) не затрагиваются.

Работать только в `wt/t92` (ветка `task/T92-link-styles`), база —
`integration/p10`.

## Что сделать

1. Аудит всех правил ссылок и `text-decoration`:
   - `frontend/src/styles.css:134-146` (`.page p a`, `.page li a`, `.empty a`),
     `.perk p a` ~1765, `.diploma p a` ~1939;
   - `frontend/src/styles/shell.css:39` (`.shell-center p a`);
   - `frontend/src/styles/catalog.css:26` (`.catalog-page .diploma p a`);
   - `frontend/src/styles/promo.css:139,207` (`.promo-toggle`, `.promo-remove`);
   - `frontend/src/styles/reviews-extra.css:76` (`.reviews-reset`).
2. Замените глобальное правило на спокойное:
   - `color: inherit; text-decoration: none;`
   - hover/focus-visible: `color: var(--brand)` (или существующий акцент),
     `text-decoration: underline; text-underline-offset: 3px;`
   - не использовать `var(--cta-blue)`/#1f3ae0 для обычного текста.
3. Уберите постоянное подчёркивание у текстовых кнопок `.promo-toggle`,
   `.promo-remove`, `.reviews-reset` — оставьте underline на hover/focus.
   Не меняйте `.btn`, `.nav`, `.side-nav`, `.admin-nav`, `.footer`, карточки,
   `a.sr-only` и ссылки-обёртки, у которых уже есть собственный стиль.
4. Проверьте контраст и доступность: ссылка должна оставаться отличимой от
   обычного текста (курсор, hover/focus, `font-weight` при необходимости).
   `:focus-visible` — видимый outline, не убирать.
5. Не трогайте `constructor.css` — стили конструктора правит T91. Если нужен
   общий селектор для `.teach-card`/`.teach-link-btn`, делайте это в
   `styles.css` с низкой специфичностью, не переопределяя T91.
6. Новых i18n-ключей нет. `ru.json` не править.

## Разрешённые файлы

- `frontend/src/styles.css`
- `frontend/src/styles/shell.css`
- `frontend/src/styles/catalog.css`
- `frontend/src/styles/promo.css`
- `frontend/src/styles/reviews-extra.css`
- `frontend/src/styles/profile.css` (только если найдёте underline/цвет ссылки)
- `frontend/tests/T92-link-styles.md` (отчёт)

## Запрещено

- `Task/**`, `specs/**`, `backend/**`, `ru.json`, `constructor.css`,
  `constructorExtra.css`, `admin*.css` (если без этого можно обойтись),
  `docker-compose*`, `qa-*.ps1`, `.github/**`, `deploy/**`.
- `git push`, `git merge`, смена ветки, работа вне `wt/t92`.

## Проверка

```
cd frontend
npm run build
```

Проверьте DOM/CSS: у обычных ссылок в `.page p`, `.empty`, `.perk`,
`.diploma`, `.shell-center` нет `text-decoration: underline` в состоянии покоя;
на hover/focus есть. Опишите в отчёте.

## Отчёт

`frontend/tests/T92-link-styles.md`: список правил до/после, проверка, риски.
Коммит явными путями.
