# T66 — Фронтенд: каталог — категории, фильтры, серверная сортировка

**Волна:** P8  **Ветка:** `task/T66-catalog-filters-ui`  **Worktree:** `wt/t66`

## Суть

Сейчас в каталоге захардкоженные «пилюли» (подменяют `q`) и клиентская
сортировка текущей страницы. Нужна панель фильтров: категории, цена, рейтинг,
сортировка — всё серверное и синхронизированное с URL (ссылки шарятся,
назад/вперёд работают). API — по разделу «Эндпоинты волны P8» в
`Task/00-api-contract.md` (**заморожен**); бэкенд T65 идёт параллельно.

Работать только в своём worktree `wt/t66` (ветка `task/T66-catalog-filters-ui`).

## Что сделать

1. `frontend/src/api/types.ts`: `CourseItem.category?: string`; тип `CategoryStat`
   (`slug, coursesCount`).
2. `frontend/src/api/client.ts`: `listCourses` принимает необязательные
   `category`, `price`, `minRating`, `sort` (пустые/undefined не отправляются);
   новая `getCategories()` → `CategoryStat[]`. Обратная совместимость.
3. `frontend/src/pages/Catalog.tsx`:
   - удалить `PILLS` (хардкод направлений) и клиентскую сортировку (`SEGS`,
     `useMemo`-сортировку); заменить на серверные параметры;
   - панель фильтров: категории (чипы/пилюли, включая «Все курсы»),
     цена (`Любая/Бесплатные/Платные`), рейтинг (`Любой/3,5+/4,0+/4,5+`),
     сортировка (селект `Сначала популярные/новые/с высоким рейтингом/дешевые/
     дорогие`), кнопка «Сбросить фильтры». На узких экранах панель
     сворачивается тогглом «Фильтры»;
   - URL: `q, category, price, minRating, sort, page` — читать при загрузке,
     писать при изменении; любое изменение фильтра/поиска сбрасывает `page`
     на 1; поиск остаётся с debounce 300 мс;
   - показывать «Найдено: N» (из `total`), empty state с подсказкой и кнопкой
     сброса (ключи `p8.catalog.nothingFound/nothingFoundHint`);
   - активные фильтры не дублировать в состоянии и URL — единственный источник
     истины URL;
   - маркетинговые блоки (`DiplomaBlock`, `DreamsBlock`, `FinalCta`,
     `LeadBlock`, `PerksBlock`, `SupportBlock`) и общую вёрстку не ломать.
4. Категория на карточке: в `frontend/src/components/CourseCard.tsx` покажите
   бейдж категории (если `category` есть), тексты — `p8.cat.*`.
5. Стили — новый `frontend/src/styles/catalog-filters.css` (импорт в
   `Catalog.tsx`), на переменных/классах дизайн-системы. `catalog.css` не править.
6. i18n — только готовые ключи `p8.catalog.*` и `p8.cat.*`; словарь не править,
   не хватает — в отчёт.
7. Тесты: новый `frontend/tests/api-contract/catalog-filters.test.mjs` по
   образцу существующих (мок-сервер 127.0.0.1:8799): передача каждого параметра,
   отсутствие пустых, парсинг `getCategories`, `total`/`items`.

## Разрешённые файлы

- `frontend/src/pages/Catalog.tsx`
- `frontend/src/components/CourseCard.tsx`
- `frontend/src/api/client.ts`, `frontend/src/api/types.ts`
- `frontend/src/styles/catalog-filters.css` (новый)
- `frontend/tests/api-contract/catalog-filters.test.mjs` (новый)
- `frontend/tests/T66-catalog-filters-ui.md` (отчёт)

## Запрещено

- `backend/**`, `Task/**`, `specs/**`, `frontend/src/i18n/ru.json`,
  `catalog.css`, чужие страницы/компоненты/стили, `qa-*.ps1`,
  `docker-compose*`.
- Ломать сигнатуры существующих клиентских функций.
- `git push`, `git merge`, смена ветки, работа вне `wt/t66`.

## Проверка

Из папки `frontend/`:

```
npm run build
npm test
```

Сборка и все тесты (старые + новые) зелёные; вывод приложите к отчёту.

## Отчёт

`frontend/tests/T66-catalog-filters-ui.md`: «Что сделано», «Проверка» (команды
и результаты), «Отклонения от постановки», «Открытые вопросы». Закоммитьте
отчёт и код в ветку `task/T66-catalog-filters-ui`.
