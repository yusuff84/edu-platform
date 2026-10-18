# T27 — отчёт: UI супер-админки, разделы «Сводка» и «Курсы»

Задача: `Task/T27-admin-console-ui.md`. Ветка: `task/T27-admin-console-ui`.
Контракт бэкенда: `Task/T25-admin-console.md` (бэкенд пишется параллельно, живьём
не проверялся — см. «Не проверено»).

## Что сделано

Заменены обе заглушки и добавлены два новых файла (только владеемые задачей):

| Файл | Что внутри |
| --- | --- |
| `frontend/src/api/adminConsole.ts` (новый) | Типы и функции шести эндпоинтов T25 через общий `apiFetch`/`buildQuery` из `client.ts`. |
| `frontend/src/pages/AdminStats.tsx` | Экран «Сводка»: пять карточек, «Обновить» → `refetch()`, скелетон, ошибка + повтор. |
| `frontend/src/pages/AdminCourses.tsx` | Экран «Курсы»: таблица, поиск (debounce 300 мс), фильтр статуса, пагинация 20/стр., «В архив»/«Снять с публикации» с подтверждением, панель отзывов с удалением, ссылка на публичную страницу. |
| `frontend/src/styles/adminConsole.css` (новый) | Изолированные стили с префиксом `.ac-`; базовые `.btn/.input/.muted/.error` и токены — из `styles.css`. |

`ru.json`, `client.ts`, `router.tsx`, `Admin.tsx`, существующие CSS и
`package.json` не изменялись. Новых зависимостей нет.

## Поля API, которые читает клиент

Ровно по контракту T25 (координатору для сверки с фактическим бэкендом):

- `GET /admin/stats`: `users.{total, students, teachers, admins, blocked, newLast7}`,
  `courses.{total, draft, review, published, archived}`,
  `payments.{count, succeededCents, platformFeeCents, teacherCents, last30Cents}`,
  `enrollments.{total, active}`, `reviews.{total, avgRating}`.
- `GET /admin/courses?status=&q=&page=&limit=`: `items[].{id, slug, title, teacherId,
  teacherEmail, status, priceCents, createdAt, updatedAt, modulesCount, lessonsCount,
  studentsCount}`, `total`, `page`, `limit`.
- `POST /admin/courses/{id}/archive`, `POST /admin/courses/{id}/unpublish`:
  ответ `{course}` (тело строки курса клиент не читает — только факт успеха; при
  ошибке берётся `ApiError.message`).
- `GET /admin/courses/{id}/reviews`: `items[].{id, studentId, studentEmail, rating,
  body, createdAt}`.
- `DELETE /admin/reviews/{id}`: 204 без тела.

Параметры запроса, которые клиент отправляет: `status` — только из четырёх
статусов (закрытый селект), `q` — подстрока названия, `page` ≥ 1, `limit` = 20.
Пустые `status`/`q` не отправляются вовсе (`buildQuery` их пропускает).

Ключи react-query и инвалидации: `['admin-stats']`, `['admin-courses', {q, status,
page}]`, `['admin-course-reviews', courseId]`. После archive/unpublish
инвалидируются `admin-courses` + `admin-stats`; после удаления отзыва —
`admin-course-reviews` + `admin-stats` (рейтинг пересчитан в T25).

## Использованные ключи i18n

- `adminStats.*`: `title`, `users`, `courses`, `payments`, `enrollments`, `total`,
  `students`, `teachers`, `admins`, `blocked`, `new7`, `statusDraft`, `statusReview`,
  `statusPublished`, `statusArchived`, `paidTotal`, `platformFee`, `teacherPayout`,
  `last30`, `active`, `avgRating`, `reviewsTotal`.
- `adminCourses.*`: `title`, `search`, `filterStatus`, `total`, `empty`, `colTitle`,
  `colTeacher`, `colStatus`, `colPrice`, `colStudents`, `colLessons`, `colUpdated`,
  `open`, `archive`, `unpublish`, `archived`, `unpublished`, `reviews`,
  `removeReview`, `confirmRemoveReview`, `reviewRemoved`.
- Соседние (уже существующие) ключи — осознанные переиспользования, см. ниже:
  `teach.statusDraft/statusReview/statusPublished/statusArchived` (бейджи статусов
  строки), `adminUsers.colActions` («Действия» — своего ключа у раздела курсов нет),
  `reviews.title` («Отзывы» — заголовок карточки сводки и `reviews.ratingLabel`
  для aria-подписи звёзд), `admin.allStatuses`, `admin.showing`, `admin.of`,
  `admin.prev`, `admin.next` (фильтр и пагинация — тот же формат, что в Admin.tsx),
  `cabinet.needRole`, `error.needLogin`, `error.network`, `common.error`,
  `common.retry`, `common.loading`.

## Литералы без ключей в ru.json (править ru.json нельзя)

1. Кнопка «Обновить» на сводке (`AdminStats.tsx`) — ключа `common.refresh`/аналога
   в словаре нет.
2. Подтверждения действий в таблице курсов: «Отправить курс «…» в архив?» и
   «Снять курс «…» с публикации и вернуть в черновики?» — ключей подтверждения
   нет (есть только успех `archived`/`unpublished`).
3. Подпись пустого списка отзывов в панели: «Отзывов у курса нет» (ключ
   `reviews.empty` = «Отзывов пока нет — станьте первым!» — ученическая формулировка,
   для админки не подходит).

Все три места помечены комментариями в коде. Остальные видимые тексты — через `t()`.

## Решения, которые стоит знать координатору

- Деньги: все агрегаты сводки — строго `formatMoney` (копейки). В колонке «Цена»
  таблицы курсов — `formatPrice` (0 → «Бесплатно»), как уже сделано для цены
  курса в очереди модерации `Admin.tsx`; в неё `formatPrice` делегирует для
  положительных сумм.
- Подтверждения действий и удаления отзыва — `window.confirm` (тот же приём, что
  у подтверждения комиссии в `Admin.tsx`); ошибки и успех показываются текстом в
  разметке (`role="alert"/"status"`), `alert()` не используется.
- Фильтры и страница — локальное состояние (в URL не пишутся): экран админский,
  шарингом ссылок на выборку никто не пользуется, а URL-состояние усложнило бы
  дебаунс.
- Пагинация при смене фильтра/поиска сбрасывается на 1 двумя эффектами:
  debounce-эффект обновляет `q`, второй эффект реагирует на `[q, status]`.
- `keepPreviousData` из @tanstack/react-query 5: при смене страницы/фильтра старые
  строки не «моргают», кнопки пагинации блокируются на время выборки.
- Ретраи: как в `Admin.tsx` — 4xx не повторяются, только сетевые ошибки (< 2).
- Ссылка на публичную страницу (`/courses/{slug}`) и открывается в новой вкладке
  (`target="_blank" rel="noreferrer"`), чтобы не терять состояние админского списка.
- 409/400 показываются текстом `ApiError.message` (например, повторный архив);
  у 403/401 — словарные подписи `cabinet.needRole`/`error.needLogin`.
- В панели отзывов — все поля строки T25: звёзды (rating), `studentEmail` (фallback —
  `studentId`), дата, текст `body`, кнопка «Удалить отзыв» с подтверждением
  `adminCourses.confirmRemoveReview` и успехом `adminCourses.reviewRemoved`.

## Проверки

```
cd frontend
npx tsc --noEmit   # exit 0, без вывода
npx vite build     # exit 0, ✓ 123 modules transformed, ✓ built in 44.71s
```

`vite build` собрал отдельные чанки `AdminStats-*.js`, `AdminCourses-*.js` и
`adminConsole-*.css` (+ JS-чанк API-модуля) — код-сплиттинг роутера не сломан.

## Не проверено (честно)

- Сквозной UI-прогон против живого бэкенда T25: в этой ветке бэкенд-эндпоинтов
  `/admin/stats`, `/admin/courses` ещё нет, реальные ответы/коды (200/400/404/409)
  не воспроизводились. Сверка велась по тексту контракта T25.
- Рендер в браузере (визуальная проверка карточек, таблицы, мобильной раскладки
  ≤720px, скелетона, панели отзывов) не выполнялся — только `tsc` + сборка.
- Дебаунс 300 мс и сброс страницы проверены логически, не в браузере.
- Поведение при 409 (курс уже в архиве / не published) и при удалении уже
  удалённого отзыва (404) не воспроизводилось — опирается на контракт.
- `formatPrice` в колонке цены (см. решение выше): если координатор хочет в этой
  колонке строго `formatMoney` («0 ₽» для бесплатных), это правка одной строки.
- Двойной вызов эффектов в React StrictMode (dev) не проверялся вживую; запросы
  идемпотентны, дублей быть не должно.
