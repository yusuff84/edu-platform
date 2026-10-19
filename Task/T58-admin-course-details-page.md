# T58 — Фронтенд: админ-страница «Детали курса» (frontend)

**Волна:** P7  **Ветка:** `task/T58-admin-course-details-page`  **Worktree:** `wt/t58`

## Суть

Админ смотрит курсы плоским списком: чтобы понять, что внутри курса
(программа, ученики, отзывы), нужно шляться по 4 разделам. Новый маршрут
`/admin/courses/:id` — «Детали курса» одним экраном. Бэкенд — T55
(`GET /admin/courses/{id}`), отзывы уже есть (T25). Контракт P7 заморожен;
ключи — `p7.adminCourse.*`.

## Что сделать (только `frontend/**`)

1. `frontend/src/api/adminCourseDetail.ts` (НОВЫЙ файл) — типы и функции
   строго по замороженному контракту:
   ```ts
   export type AdminCourseDetail = { course: AdminCourseRow; modules: …; stats: … };
   listAdminCourseDetail(courseId): Promise<AdminCourseDetail>   // GET /admin/courses/{id}
   ```
   Использовать `apiFetch`; отзывы — уже готовое `listCourseReviews`/
   `deleteReview` из `adminConsole.ts` (T25).
2. `frontend/src/pages/AdminCourseDetail.tsx` (НОВЫЙ):
   - шапка: название, статус-бейдж (переиспользовать класс бейджей T48),
     цена, автор (email), ссылка «Публичная страница» (slug),
     «Ученики у автора» → `/teach/{id}/students`, кнопка `back` → `/admin/courses`;
   - блок «Показатели» (stats): `studentsCount`, `activeCount` (подписи из
     численности — если ключа нет, используй текст без ключа только там, где
     заморозка не покрывает; лучше построить на
     `p7.adminCourse.*`-числах), `reviewsCount`, `avgRating`,
     `paymentsCount`, `paidTotalCents` (formatMoney из client.ts);
   - блок «Программа»: модули/уроки списком (урок: kind-метитка, `position`);
   - блок «Ученики»: этот список уже есть в T49 (`listAdminEnrollments` с
     courseId-фильтром) — ВЫЗЫВАЙ `listAdminEnrollments({ courseId: id })`
     и показывай компактную таблицу (email/имя, статус, прогресс);
     ссылка «Все записи» → `/admin/enrollments` (T49-страница уже умеет
    queryParams необязательно — просто линка без фильтра);
   - блок «Отзывы»: `listCourseReviews` + удаление (`deleteReview`) с
     inline-подтверждением, refetch. Тексты кнопок — существующие ключи
     (adminConsole.*) НЕ удалять; при отсутствии ключей — писать в отчёт.
   - состояния: `loading`/`error`/`notFound` (`p7.adminCourse.notFound`).
3. `frontend/src/app/router.tsx` — маршрут
   `{ path: '/admin/courses/:id', element: <RequireRole roles={['admin']}><AdminCourseDetail /></RequireRole> }`
   (добавить ПОСЛЕ `/admin/courses` — react-router сам разрулит конфликт).
4. `frontend/src/pages/AdminCourses.tsx` — в таблице добавить в действия
   кнопку «Детали» (Link на `/admin/courses/{id}`; стиль — существующие
   классы кнопок; колонку не разжимать).
5. Стили: `frontend/src/styles/adminCourseDetail.css` (НОВЫЙ файл; импорт в
   `src/index.css` или как принято — посмотри, как подключаются остальные);
   классыprefix `acd-*`; таблица/чипы в стиле T48-T52 (вписывается в
   «карточку», не наезжает: max-width 100%).

## Разрешённые файлы

- НОВЫЕ: `frontend/src/api/adminCourseDetail.ts`,
  `frontend/src/pages/AdminCourseDetail.tsx`,
  `frontend/src/styles/adminCourseDetail.css`,
  `frontend/tests/T58-admin-course-details-page.md`;
- ПРАВКИ: `App/router.tsx` (одна строка), `AdminCourses.tsx` (кнопка),
  `src/main.tsx`/`index.css` — только импорт нового css, если так принято в
  проекте;
- НЕЛЬЗЯ: `ru.json`, `client.ts` (испортируй apiFetch из client), чужие
  страницы (Admin.tsx/AdminEnrollments.tsx — у них свои волны; AdminCourses
  правим только кнопку).

## Проверка

`npm run build && npm test` — зелёные. API-функции — типы должны
кмпилироваться (бэк T55 ещё не влита — TS-типы по контракту, runtime при
live-приёмке).

## Отчёт

`frontend/tests/T58-admin-course-details-page.md`. Коммит `T58: ...`, push
`task/T58-admin-course-details-page`.
