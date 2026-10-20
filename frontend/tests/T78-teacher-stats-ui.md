# T78 — фронтенд: страница аналитики учителя

**Ветка:** `task/T78-teacher-stats-ui`<br>
**Worktree:** `wt/t78`<br>
**Контракт:** раздел «Волна P9 (T71–T82)» → T77 в `Task/00-api-contract.md`
(заморожен, не менялся).

## Что сделано

### `frontend/src/api/types.ts`

Добавлены типы строго по контракту T77:

- `TeacherOverviewCourses` (`total`, `published`, `draft`, `review`, `archived`);
- `TeacherOverviewStudents` (`total`, `unique`);
- `TeacherOverviewRevenue` (`paidTotalCents`, `platformFeeCents`, `teacherCents`);
- `TeacherOverviewCompletion` (`completed`, `avgPercent`);
- `TeacherOverviewRatings` (`avgRating`, `ratingsCount`);
- `TeacherOverviewLast30d` (`enrollments`, `revenueCents`);
- `TeacherOverview` — сводка из шести блоков;
- `TeacherCourseStat` (`courseId`, `title`, `status`, `students`,
  `revenueCents`, `avgRating`, `ratingsCount`, `avgPercent`);
- `TeacherCourseStatsPage` (`items`, `total`).

Типы `TeacherCourseStat`, `TeacherCourseStatsPage`, `TeacherOverview`
реэкспортированы из `client.ts` рядом с остальными доменными типами.

### `frontend/src/api/client.ts`

Добавлены функции (сигнатуры существующих не менялись):

- `teacherOverview()` — `GET /teacher/stats`, возвращает конверт сводки целиком;
- `teacherCourseStats(limit?)` — `GET /teacher/stats/courses?limit=…`,
  возвращает `{ items, total }`. Без `limit` query-параметр не отправляется
  (действует серверное значение по умолчанию 50); `limit` не ограничивается
  на клиенте, максимум 200 контролирует backend.

### `frontend/src/pages/TeacherStats.tsx` (новый)

- KPI-карточки: курсы (всего/опубликовано), ученики (записи/уникальные),
  выручка (оплачено всего/ваш доход/комиссия), завершаемость (завершено/
  средний прогресс), рейтинг (средний/число оценок), за 30 дней
  (записи/выручка).
- Таблица «По курсам»: название, статус, ученики, выручка, прогресс
  (полоса + процент), рейтинг и кнопка «Открыть». Строка кликабельна целиком
  и ведёт в `/teach/{courseId}`; ссылка в названии и кнопка не дублируют
  переход. Порядок строк — серверный (выручка desc, ученики desc, название).
- Состояния: скелетон KPI при загрузке, отдельные блоки ошибки с повтором для
  сводки и таблицы, пустое состояние «Курсов пока нет».
- Форматирование: деньги — `formatMoney` из `client.ts` (копейки → рубли),
  проценты и рейтинг — локально через `ru-RU` (одна цифра после запятой),
  целые счётчики — с разделителями разрядов.
- Доступность: `role="progressbar"` с `aria-valuenow`, `role="alert"` у ошибок,
  `sr-only`-подпись у колонки действий, полное название курса в `title` ссылки.
- Адаптив: KPI-карточки на узких экранах в одну колонку, таблица — в
  горизонтальном скролле.

### `frontend/src/app/router.tsx`

- Добавлен ленивый чанк `TeacherStats`.
- Маршрут `/teach/stats` добавлен **до** `/teach/:id` и обёрнут в
  `RequireRole(['teacher', 'admin'])`. Существующие маршруты не менялись,
  правка только аддитивная.

### `frontend/src/pages/Cabinets.tsx`

- В кабинете преподавателя рядом с «Создать курс» добавлена ссылка-кнопка
  «Аналитика» (`/teach/stats`, ключ `p9.stats.title`).

### `frontend/src/styles/teacherStats.css` (новый)

- Стили страницы с префиксом `.tstats-`: KPI-сетка и карточки, строки
  «подпись — значение», скелетон, таблица, статусы, прогресс-полоса, пустое
  состояние и ошибки.
- Используются переменные дизайн-системы из `styles.css` (`--surface`,
  `--line`, `--line-soft`, `--muted`, `--brand`, `--ok-bg`, `--warn-bg`,
  `--r-md`, `--shadow-sm` и т. д.). Чужие CSS-файлы не менялись.
- Адаптив: при `max-width: 720px` карточки в одну колонку, таблица сохраняет
  минимальную ширину и скроллится внутри контейнера.

### `frontend/tests/api-contract/teacher-stats.test.mjs` (новый)

10 контрактных тестов по образцу существующих (мини-API на порту 8799):

- `teacherOverview`: метод/путь/отсутствие query и тела, конверт целиком,
  сохранение нулевых значений, `403` с текстом сервера, `401` без текста;
- `teacherCourseStats`: запрос без `limit`, `limit=7`, `limit=200`,
  сохранение всех полей строк и пустой выдачи, `403` с текстом сервера и
  запасным текстом.

## Проверка

Из папки `frontend/`:

```
npm run build
```

Результат: `tsc --noEmit` без ошибок, `vite build` успешно — 149 модулей,
сборка за 20.65 с. Страница вынесена в отдельные чанки:
`TeacherStats-*.js` (6.70 kB) и `TeacherStats-*.css` (3.47 kB).

```
npm test
```

Результат: `test:build` успешно, `node --test tests/api-contract/*.test.mjs` —
**131 тест, 131 pass, 0 fail** (10 новых из `teacher-stats.test.mjs`; остальные
файлы, включая `auth-catalog`, `transport`, `catalog-filters`, `certificates`,
`promo`, `review-replies`, `teacher-cover`, `teaching-admin`, зелёные).
Порт 8799 был свободен, повторный запуск не потребовался.

## Отклонения от постановки

- Для подписей «Опубликовано», «Средний рейтинг» и «Отзывов» в наборе
  `p9.stats.*` нет отдельных ключей. Использованы уже существующие общие
  ключи `adminStats.statusPublished`, `adminStats.avgRating`,
  `adminStats.reviewsTotal`; статусы курса — существующие `teach.statusDraft`,
  `teach.statusReview`, `teach.statusPublished`, `teach.statusArchived`.
  `ru.json` не менялся, новые ключи не добавлялись.
- В шапке страницы нет отдельной кнопки «Обновить»: обновление доступно через
  кнопку «Повторить» в блоках ошибок, а React Query сам обновляет данные при
  повторном входе на страницу. Постановка кнопку не требует.
- Таблица показывает все курсы любого статуса (как в контракте T77), без
  клиентской фильтрации и пагинации; запрошен `limit=50` — серверное значение
  по умолчанию.

## Открытые вопросы

- Бэкенд T77 идёт параллельно. UI рассчитан на замороженный контракт; если
  `status` придёт незнакомой строкой, страница покажет её как есть в нейтральной
  плашке, без падения.
- Если координатор захочет полностью убрать зависимость от «админских» ключей,
  в словарь стоит добавить `p9.stats.published`, `p9.stats.avgRating` и
  `p9.stats.ratingsCount`. Сейчас словарь не правился по условию задачи.
- Для admin страница по контракту показывает все курсы платформы; отдельного
  пояснения в UI нет — при необходимости можно добавить подзаголовок
  (потребуется новый ключ).
