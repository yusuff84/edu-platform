# T27 — UI: супер-админ, разделы «Сводка» и «Курсы»

Волна: P3. Зависимости: контракт T25 (бэкенд пишется параллельно — брать из
`Task/T25-admin-console.md`). Разводка уже сделана: роуты `/admin/stats` и
`/admin/courses`, ссылки в навигации админки, заглушки страниц.

## Зачем

У супер-админа нет ответа на вопрос «что вообще происходит на платформе»: сколько
людей, курсов по статусам, выручки, отзывов. И нет способа увидеть все курсы,
снять плохой курс с публикации, открыть его целиком или удалить хамский отзыв.

## Владеет файлами (менять можно только их)

- `frontend/src/pages/AdminStats.tsx` (заменить заглушку)
- `frontend/src/pages/AdminCourses.tsx` (заменить заглушку)
- `frontend/src/styles/adminConsole.css` (новый)
- `frontend/src/api/adminConsole.ts` (новый)
- `frontend/tests/T27-admin-console.md` (отчёт, создать в конце)

`ru.json`, `client.ts`, `router.tsx`, `Admin.tsx` трогать нельзя. Не хватает ключа —
русский литерал + отметка в отчёте.

## API-клиент (`api/adminConsole.ts`)

```ts
import { apiFetch, buildQuery } from './client';

export type AdminStats = {
  users: { total: number; students: number; teachers: number; admins: number; blocked: number; newLast7: number };
  courses: { total: number; draft: number; review: number; published: number; archived: number };
  payments: { count: number; succeededCents: number; platformFeeCents: number; teacherCents: number; last30Cents: number };
  enrollments: { total: number; active: number };
  reviews: { total: number; avgRating: number };
};
export type AdminCourseRow = {
  id: string; slug: string; title: string; teacherId: string; teacherEmail: string;
  status: CourseStatus; priceCents: number; createdAt: string; updatedAt: string;
  modulesCount: number; lessonsCount: number; studentsCount: number;
};
export type AdminCoursePage = { items: AdminCourseRow[]; total: number; page: number; limit: number };
export type AdminReviewRow = { id: string; studentId: string; studentEmail: string; rating: number; body: string; createdAt: string };

export function getAdminStats(): Promise<AdminStats>;
export function listAdminCourses(params: { status?: string; q?: string; page?: number; limit?: number }): Promise<AdminCoursePage>;
export function archiveAdminCourse(id: string): Promise<{ course: AdminCourseRow }>;
export function unpublishAdminCourse(id: string): Promise<{ course: AdminCourseRow }>;
export function listCourseReviews(courseId: string): Promise<{ items: AdminReviewRow[] }>;
export function deleteReview(id: string): Promise<void>;
```

## Экран «Сводка» (`/admin/stats`)

1. Пять блоков-карточек: **Пользователи**, **Курсы**, **Платежи**, **Записи**,
   **Отзывы** — каждый с ключевыми числами из контракта.
2. Деньги — через `formatMoney` (копейки), даты — `formatDate` из `api/client`.
3. Кнопка «Обновить» → `refetch()`; `staleTime` можно оставить дефолтным.
4. Загрузка — скелетон; ошибка — `common.error` + повтор.

Обязательные цифры: пользователи (всего/ученики/учителя/админы/заблокированы/новые
за 7 дней), курсы (всего + 4 статуса), платежи (кол-во, оплачено всего, комиссия
платформы, к выплате, за 30 дней), записи (всего/активные), отзывы (кол-во, средний
рейтинг).

## Экран «Курсы» (`/admin/courses`)

Таблица: название (ссылка на `/courses/{slug}` — публичная страница), преподаватель
(email), статус (бейдж), цена, учеников, уроков, обновлён + действия.

Обязательное:

1. Поиск по названию (debounce 300 мс) и фильтр по статусу — сбрасывают страницу на 1.
2. Пагинация 20/страница, «Показано N из total».
3. Действия: **«В архив»** (для не-archived), **«Снять с публикации»** (только для
   published). После действия — `invalidateQueries` и сообщение об успехе.
4. Кнопка **«Отзывы»** — раскрывает панель под строкой со списком отзывов курса и
   кнопкой «Удалить отзыв» (подтверждение, затем `deleteReview` и обновление списка).
5. Ошибки 409/400 — текстом из `ApiError.message`, не `alert`.
6. Пустой список — `adminCourses.empty`.

## Стиль

`styles/adminConsole.css`, префикс `.ac-`. Существующие CSS не трогать.

## Проверка

```
cd frontend
npx tsc --noEmit
npx vite build
```

- 0 ошибок TS, сборка зелёная, новых зависимостей нет.
- Отчёт `frontend/tests/T27-admin-console.md` по тому же шаблону, что T26.

## Критерии приёмки

- Обе заглушки заменены рабочими экранами, роуты открываются.
- Числа и подписи соответствуют контракту T25 (поле в поле).
- Баланс в копейках не показывается «как есть» — только через `formatMoney`.
