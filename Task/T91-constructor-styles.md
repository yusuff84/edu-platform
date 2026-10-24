# T91 — Стили: разделы квиза и кода в конструкторе

**Волна:** P10  **Ветка:** `task/T91-constructor-styles`  **Worktree:** `wt/t91`

## Суть

В конструкторе курса блоки «вопросы» и «тесты кода» выглядят чужеродно:
огромные кнопки перестановки/удаления, разъезжающиеся строки вариантов,
несогласованные input/select/textarea и заголовки. Причина — слишком широкое
правило `.teach-section button:not(.btn)` в `constructor.css`, которое
применяется ко всем вложенным кнопкам, включая компактные действия.

Работать только в `wt/t91` (ветка `task/T91-constructor-styles`), база —
`integration/p10`.

## Что сделать

1. Проведите аудит `frontend/src/pages/Constructor.tsx` (блоки `lesson.kind ===
   'quiz'` и `'code'`), `frontend/src/styles/constructor.css`,
   `constructorExtra.css`, `styles.css`. Найдите все правила, которые
   протекают на вложенные элементы.
2. Не ломая общий стиль кнопок T62, сузьте `.teach-section button:not(.btn)`:
   компактные действия внутри `.ce-q-box`, `.teach-option-row`, `.ce-reorder`,
   `.teach-test-box` должны быть 32–34px, с меньшим padding, без «гигантских»
   кнопок. Кнопки добавления вопроса/варианта/теста — обычные вторичные.
3. Добавьте в разметку `Constructor.tsx` минимальные классы-маркеры
   (`ce-quiz`, `ce-code`, `ce-add`, `ce-btn-sm`, `ce-btn-danger` и т.п.) и
   приведите:
   - input/select/textarea в quiz/code к единой высоте/радиусу/границе,
     `width: 100%`, `box-sizing: border-box`;
   - fieldset/legend и подписи — к типографике остальных секций;
   - строку варианта — сетка `radio | label | input | действия` без переносов
     на десктопе и аккуратный перенос на узком экране;
   - тесты кода — карточки с заголовком, полями и компактным рядом действий;
   - `focus-visible` для всех кнопок/полей, `disabled` — приглушённый.
4. Уберите дубли/конфликты в `constructor.css` и `constructorExtra.css`;
   используйте существующие CSS-переменные, не вводите новый стиль.
5. Проверьте, что остальные кнопки конструктора (обложка, промокоды,
   структура, lifecycle) не изменились визуально.
6. Новых i18n-ключей не добавлять. `ru.json` не править.

## Разрешённые файлы

- `frontend/src/pages/Constructor.tsx`
- `frontend/src/styles/constructor.css`
- `frontend/src/styles/constructorExtra.css`
- `frontend/tests/T91-constructor-styles.md` (отчёт)

## Запрещено

- `Task/**`, `specs/**`, `backend/**`, `ru.json`, другие страницы,
  `styles.css` (кроме случая, если без этого нельзя — обоснуйте),
  `docker-compose*`, `qa-*.ps1`, `.github/**`, `deploy/**`.
- Менять бизнес-логику/валидацию квиза и кода.
- `git push`, `git merge`, смена ветки, работа вне `wt/t91`.

## Проверка

```
cd frontend
npm run build
```

Дополнительно проверьте DOM-структуру (без скриншотов): в блоках quiz/code
кнопки перестановки/удаления имеют компактный класс, поля не выходят за
контейнер. Опишите проверку в отчёте.

## Отчёт

`frontend/tests/T91-constructor-styles.md`: корневые причины, что изменено,
проверка, риски. Коммит явными путями.
