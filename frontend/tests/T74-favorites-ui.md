# T74 — Фронтенд: избранное (сердечко на карточке и страница)

**Ветка:** `task/T74-favorites-ui`  
**Worktree:** `wt/t74`  
**Контракт:** `Task/00-api-contract.md`, раздел «Волна P9 (T71–T82)» → T73 (заморожен, не менялся).

## Что сделано

### `frontend/src/api/types.ts`

- Добавлен `FavoritesPage` — конверт `GET /me/favorites`: `{ items: CourseItem[],
  total: number }`. Формы существующих типов не менялись.

### `frontend/src/api/client.ts`

- `addFavorite(courseId)` — `PUT /me/favorites/{courseId}`, `encodeURIComponent`
  для id, тело не отправляется, `204` → `undefined`.
- `removeFavorite(courseId)` — `DELETE /me/favorites/{courseId}`, без тела,
  `204` → `undefined` (идемпотентно).
- `myFavorites()` — `GET /me/favorites`, конверт `{ items, total }`
  распаковывается в `CourseItem[]`; порядок сервера (добавленные позже первыми)
  сохраняется.
- `FavoritesPage` добавлен в реэкспорт типов; существующие функции и их
  сигнатуры не изменялись.

### `frontend/src/components/CourseCard.tsx`

- В правом верхнем углу карточки — кнопка-сердечко (сосед ссылки `.tile`,
  а не вложенный в неё интерактив; клик дополнительно гасится
  `preventDefault`/`stopPropagation` и курс не открывает).
- `aria-label` — `p9.fav.add` / `p9.fav.remove`, состояние передаётся через
  `aria-pressed`; у гостя в `title` — `p9.fav.loginHint`.
- Вошедшему: `useMutation` с оптимистичным обновлением кэша `['favorites']`
  (сердечко и список откликаются сразу), откат при ошибке и инвалидация
  `['favorites']` после ответа. При `401` — переход на `/login`.
- Гостю: переход на `/login`; запросы избранного для гостя отключены
  (`enabled: Boolean(user)`), сессия берётся из общего `['me']`
  (`meQueryOptions`), поэтому лишних запросов нет.
- Признак избранного определяется по общему списку `['favorites']` (в
  `CourseItem` поля `isFavorite` нет — так требует контракт T73); React Query
  дедуплицирует запрос по ключу, все карточки страницы используют один ответ.
- Обложка, бейдж категории, звёзды, цена и существующая вёрстка не менялись.

### `frontend/src/pages/Favorites.tsx` (новый)

- Страница `/me/favorites`: заголовок `p9.fav.title`, ссылка в каталог,
  сетка карточек `CourseCard` на общем ключе `['favorites']`.
- Состояния: загрузка (текст `common.loading` + 6 скелетонов), ошибка сети
  (`error.network` + `common.retry`), пустое состояние (`p9.fav.empty`,
  `p9.fav.emptyHint`, CTA `p9.fav.goCatalog`).
- Снятие с избранного на этой странице сразу убирает карточку из кэша
  (общий ключ с сердечком).

### `frontend/src/app/router.tsx`

- Ленивый чанк `Favorites` рядом с `MyCourses`.
- Маршрут `{ path: '/me/favorites', element: <RequireAuth><Favorites /></RequireAuth> }`
  сразу после `/me`.
- Пункт «Избранное» (`p9.fav.title`) в навигации для вошедших — между
  «Мои курсы» и «Профиль». Гостевая навигация P7 не менялась.
- Правки только аддитивные, существующие маршруты не тронуты.

### `frontend/src/pages/MyCourses.tsx`

- Рядом с заголовком — ссылка-кнопка «Избранное» (`/me/favorites`) в основном
  и в пустом состоянии списка курсов.

### `frontend/src/styles/favorites.css` (новый)

- `.fav-btn` — круглая кнопка поверх обложки (не сдвигает контент), hover,
  active, focus (общее кольцо дизайн-системы), `aria-pressed`-состояние
  (заливка сердца `var(--brand)`), disabled на время запроса.
- `.favorites-head`, `.cab-head` — заголовок со ссылкой; `.fav-skeletons` /
  `.fav-skeleton` — сетка скелетонов 3/2/1 колонки.
- Только переменные и классы дизайн-системы (`--brand`, `--muted`, `--surface`,
  `--line`, `--shadow-sm`, `--shadow-pop`, `--press-shift` и т. д.); чужие css
  не правились. Файл импортируется из `CourseCard.tsx`, `Favorites.tsx`,
  `MyCourses.tsx`.

### `frontend/tests/api-contract/favorites.test.mjs` (новый)

13 контрактных тестов на мини-API `helper.mjs`:

- `addFavorite`: `PUT`, путь, `204` → `undefined`, отсутствие тела,
  URL-кодирование id, идемпотентный повтор, `401` и `404 «Курс не найден»`.
- `removeFavorite`: `DELETE`, путь, `204` → `undefined`, отсутствие тела,
  URL-кодирование id, идемпотентный повтор, `401` (с текстом сервера и с
  запасным текстом клиента).
- `myFavorites`: `GET /me/favorites`, конверт `{items,total}` → массив,
  порядок элементов, пустой список, `401`.

### i18n

- Использованы только существующие ключи `p9.fav.*` (`title`, `add`, `remove`,
  `empty`, `emptyHint`, `goCatalog`, `loginHint`) и общие (`common.loading`,
  `common.retry`, `error.network`, `nav.myCourses`). `ru.json` не менялся,
  недостающих ключей нет.

## Проверка

Из папки `frontend/`:

```
npm run build
```

Результат: успешно. `tsc --noEmit` без ошибок, Vite собрал прод-бандл —
149 модулей за 34.09 с, отдельный чанк `Favorites-*.js`.

```
npm test
```

Результат: успешно. `test:build` собрал `.tmp-api-test/client.js`;
`node --test tests/api-contract/*.test.mjs`:

```
# tests 134
# pass 134
# fail 0
# cancelled 0
# skipped 0
# todo 0
```

13 новых тестов `favorites.test.mjs` — все зелёные; старые файлы
(`auth-catalog`, `transport`, `catalog-filters`, `certificates`, `promo`,
`review-replies`, `teacher-cover`, `teaching-admin`) не сломаны. Порт 8799
не конфликтовал, повтор не потребовался.

## Отклонения от постановки

- Использованы **и** оптимистичное обновление кэша, **и** инвалидация
  `['favorites']` (постановка допускает любой из вариантов): оптимистичное —
  для мгновенного отклика сердечка, инвалидация — чтобы состояние и список
  сошлись с сервером, в том числе если запрос упал.
- Заголовок «Мои курсы» обёрнут в `.cab-head` со ссылкой «Избранное»; стиль
  `.cab-head` добавлен в новый `favorites.css`, чтобы не править `profile.css`
  (импорт нового файла добавлен в `MyCourses.tsx`). Ссылка показывается и в
  пустом состоянии «Моих курсов» — избранное может быть непустым.
- Отдельного визуального тоста об успехе нет (системы уведомлений в проекте
  нет): ключи `p9.fav.added` / `p9.fav.removed` не используются. Обратная
  связь — изменение `aria-pressed` и подписи кнопки.

## Открытые вопросы

- Бэкенд T73 идёт параллельно: UI рассчитан на замороженный контракт
  (`PUT/DELETE /me/favorites/{id}` → `204`, `GET /me/favorites` →
  `{items,total}`). Если формы ответа изменятся, потребуется правка
  `myFavorites()` и тестов.
- До первого ответа `GET /me/favorites` сердечки отображаются незаполненными
  (признака `isFavorite` в курсе нет). При недоступном бэкенде избранного
  сердечко после клика откатывается, отдельного текста ошибки на карточке нет;
  при `401` пользователь уходит на `/login`.
- В словаре нет ключа для текста ошибки загрузки именно избранного —
  используется общий `error.network`; отдельный ключ можно добавить
  координатору, если нужен специфичный текст. Словарь не правился.
