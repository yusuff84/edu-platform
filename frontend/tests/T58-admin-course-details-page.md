# T58 — Админ-страница «Детали курса» (/admin/courses/:id). Отчёт

Дата: 2026-10-19. Ветка: `task/T58-admin-course-details-page`. Волна: P7.
Контракт: `Task/00-api-contract.md` — раздел «T55 — `GET /admin/courses/{id}`»
(ЗАМОРОЖЕН, форма ответа соблюдена дословно). Живой стенд и браузер не
использовались — визуальную приёмку делает координатор (как в задачах волны P6).

## Что сделано

### Файлы

| Файл | Что |
|---|---|
| `frontend/src/api/adminCourseDetail.ts` | НОВЫЙ: типы и `listAdminCourseDetail(courseId)` → `GET /admin/courses/{id}` через `apiFetch` из `client.ts` |
| `frontend/src/pages/AdminCourseDetail.tsx` | НОВЫЙ: экран деталей курса одним экраном |
| `frontend/src/styles/adminCourseDetail.css` | НОВЫЙ: стили, все селекторы с префиксом `.acd-` |
| `frontend/src/app/router.tsx` | +3 строки: `lazy`-импорт и маршрут `/admin/courses/:id` под `RequireRole roles={['admin']}`, строго ПОСЛЕ `/admin/courses` |
| `frontend/src/pages/AdminCourses.tsx` | +9 строк: кнопка-ссылка на детали первой в колонке действий (стиль существующих `.btn .btn-sm`, колонка не разжимается) |

`ru.json`, `client.ts`, `Admin.tsx`/`AdminStats.tsx`/`AdminEnrollments.tsx`,
`styles.css`, backend и specs — не тронуты (проверено `git status --short`).
CSS подключён по принятому в проекте образцу: импорт в самом tsx-файле
(как `adminConsole.css` в AdminCourses/AdminStats и `adminEnrollments.css`
в AdminEnrollments).

### API (`adminCourseDetail.ts`) — строго по замороженному T55

- `AdminCourseDetail` = `{ course, modules, stats }`;
  - `course` — `AdminCourseDetailCourse` = элемент `GET /admin/courses`
    (переиспользован тип `AdminCourseRow` из `adminConsole.ts` — он не
    дублируется) + `coverPath?: string | null` (T54);
  - `modules` — `{ id, title, position, lessons: [{ id, title, kind,
    position }] }`: у урока ТОЛЬКО метаданные (id/title/kind/position) —
    тел lesson (`bodyMd`/`quiz`/`codeTask`) в типе нет по контракту;
  - `stats` — `{ studentsCount, activeCount, reviewsCount, avgRating,
    paymentsCount, paidTotalCents }`.
- `listAdminCourseDetail(courseId)` — GET, id кодируется
  (`encodeURIComponent`); ошибки 401/403/404 приходят как `ApiError.status`;
  404 «Курс не найден» обрабатывается отдельным экраном.
- Отзывы НЕ дублируются: `listCourseReviews`/`deleteReview`
  импортированы из `adminConsole.ts` (T25) — как в задании.

### Экран (`AdminCourseDetail.tsx`)

- Шапка: back-ссылка `p7.adminCourse.back` → `/admin/courses` (и в
  404-экране); название курса; статус-бейдж в палитре T48
  (`teach.statusDraft/statusReview/statusPublished/statusArchived` — тот же
  набор, что в строке AdminCourses); цена `formatPrice` (0 → «Бесплатно»);
  автор `teacherEmail` (пусто → «—»); обложка `coverPath` (если есть,
  T54, alt = `p7.cover.alt`); кнопки: `p7.adminCourse.openPublic` →
  `/courses/{slug}` (новая вкладка, как тайтл-ссылка в списке),
  `p7.adminCourse.openStudents` → `/teach/{id}/students`.
- Блок «Показатели» (`p7.adminCourse.stats`) — 7 карточек-чисел:
  `studentsCount` (`p7.adminCourse.studentsCount`), `activeCount`
  (`adminStats.active`), `lessonsCount` — сумма уроков модулей
  (`p7.adminCourse.lessonsCount`), `reviewsCount`
  (`p7.adminCourse.reviewsCount`), `avgRating` (формат «4,5» — одна цифра
  после запятой, `adminStats.avgRating`), `paymentsCount`
  (`adminStats.payments`), `paidTotalCents` → `formatMoney`
  (`p7.adminCourse.paidTotal`).
- Блок «Программа» (`p7.adminCourse.program`): модули списком с
  `#position` и названием; уроки — строки `#position` + kind-метка
  (`kind.video_text|live|quiz|code` — существующие ключи, как в
  Constructor/Course) + title. Пусто — прочерк.
- Блок «Ученики» (`p7.adminCourse.students`): компактная таблица записей
  через `listAdminEnrollments({ courseId: id, limit: 100 })` (существующий
  API T49): ученик (имя + email, без имени — email), статус-бейдж
  `adminEnrollments.statusActive|Refunded|Revoked`), прогресс — полоса
  `.progress/.progress-bar` + percent (`adminEnrollments.colProgress`
  aria, значение «N из M» — `admin.of` в aria-valuetext). Ссылка «Все
  записи» → `/admin/enrollments` (текст — `adminEnrollments.title`),
  как в ТЗ T58. Пусто — `adminEnrollments.empty`.
- Блок «Отзывы» (`p7.adminCourse.reviews`): список `listCourseReviews`
  (звёзды, `studentEmail`, дата `formatDate`, текст `body`) и удаление
  `deleteReview` с INLINE-подтверждением — `window.confirm` НЕ
  используется: по клику «Удалить отзыв» (`adminCourses.removeReview`)
  в строке отзыва раскрывается вопрос `adminCourses.confirmRemoveReview` и
  две кнопки: `common.confirm` (красная) / `common.cancel`. После
  успешного удаления — тост `adminCourses.reviewRemoved`, refetch списка
  отзывов и деталей (инвалидация `['admin-course-reviews', id]`,
  `['admin-course', id]` — stats.avgRating/reviewsCount пересчитываются) и
  `['admin-stats']` (как в AdminCourses).
- Состояния: загрузка — `common.loading` (`role="status"`); ошибка —
  текст `errorText` (403 → `cabinet.needRole`, 401 → `error.needLogin`,
  сеть → `error.network`, прочее — `ApiError.message`/`common.error`) +
  кнопка `common.retry`; **404 → экран `p7.adminCourse.notFound`** с
  кнопкой возврата к списку. Каждый блок (детали/записи/отзывы) имеет своё
  состояние ошибки и повтора — ошибка отзывов не убивает остальное.
- Доступность: `sr-only`-h1 (`p7.adminCourse.info`), `aria-labelledby` у
  блоков, `role="status"/"alert"` у сообщений, `aria` у прогрессбара,
  `scope="col"` у заголовков таблицы.
- Данные блоков независимы: ошибка или пустота одного блока не влияют на
  остальные.

### Стили (`adminCourseDetail.css`)

Все селекторы с префиксом `.acd-`; базовые классы (.btn, .panel-подобные
карточки, .muted, .progress) — из styles.css. По требованию ТЗ таблица
(acd-table) БЕЗ min-width и без горизонтального скролла: ширина 100%,
`max-width: 100%`, `white-space: normal`, длинные email/имена переносятся
(`overflow-wrap: anywhere`); бейджи всех видов (курса, записи) и
progress-in-card вписываются в карточку. На ≤720px — компактные паддинги.

## i18n

Использованы ТОЛЬКО существующие ключи: `p7.adminCourse.*` (back, info,
program, students, reviews, stats, studentsCount, lessonsCount,
reviewsCount, paidTotal, openPublic, openStudents, notFound — все 13
присутствуют в ru.json), плюс существующие: `adminStats.*` (active,
avgRating, payments), `adminEnrollments.*` (title, empty, colStudent,
colStatus, colProgress, status*), `adminCourses.*` (open, removeReview,
confirmRemoveReview, reviewRemoved, reviewsEmpty), `teach.status*`,
`kind.*`, `course.price`, `course.teacher`, `admin.of`,
`admin.reviewEmpty`, `cabinet.needRole`, `error.*`, `common.*`,
`p7.cover.alt`. Новых ключей НЕ добавлено.

**Не хватает ключей (для координатора, в словарь по заморозке не лезем):**

| Не хватает | Чем заменить | Где |
|---|---|---|
| «Детали» для кнопки в списке курсов | `adminCourses.open` («Открыть») | AdminCourses.tsx, действия строки |
| подпись `activeCount` | `adminStats.active` («Активные») | страница, «Показатели» |
| подпись `paymentsCount` | `adminStats.payments` («Платежи») | страница, «Показатели» |
| подпись `avgRating` при счётчике | `adminStats.avgRating` («Средний рейтинг») | страница, «Показатели» |

Если координатор согласует ключи (например `p7.adminCourse.activeCount`,
`p7.adminCourse.paymentsCount`, `p7.adminCourse.avgRating`,
`p7.adminCourse.details`) — замена подписей точечная, схема не меняется.

## Проверка

- `node_modules` отсутствовали → `npm install` (чистая установка npm, 76 пакетов).
- `npm run build` (tsc --noEmit && vite build) — 0 ошибок; типы по
  замороженному контракту компилируются (в т.ч. `LessonKind` для
  kind-меток, конверт T55). Чанк `AdminCourseDetail` выделяется отдельно
  (code splitting по стилю соседей).
- `npm test` — 61/61 зелёные (контракты api-клиента, вне изменения).
- Отчёт T58 `frontend/tests/T58-admin-course-details-page.md` создан; коммит
  `T58: страница деталей курса в админке (/admin/courses/:id)`, ветка
  `task/T58-admin-course-details-page` — push.

## Примечания

- Маршрут добавлен строго после `/admin/courses`; конфликт с статичным
  `/admin/courses/review` — вопрос только backend (маршрута `/admin/courses/review`
  во фронте нет, там очередь не рендерится отдельно).
- В таблице учеников колонки «дата записи» НЕТ — по ТЗ таблица компактная
  (email/имя, статус, прогресс percent); данные createdAt в строке есть,
  при желании колонку добавить легко.
- `coverPath` типизирован `string | null` (аддитивное поле T54; NULL — обложки нет).
