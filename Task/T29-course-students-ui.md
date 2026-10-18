# T29 — UI: кабинет учителя, «Ученики курса»

Волна: P3. Зависимости: контракт T22 (бэкенд пишется параллельно — брать из
`Task/T22-teacher-students.md`). Разводка уже сделана: роут `/teach/:id/students`,
ссылка «Ученики курса» из конструктора, заглушка страницы.

## Зачем

Учитель узнаёт, что курс кто-то купил, только по кошельку. Кто записался, дошёл ли
до конца, когда заходил последний раз, сколько заработано — этих ответов нет ни в UI,
ни в API. Без них учитель не может улучшать курс: он не видит, где ученики бросают.

## Владеет файлами (менять можно только их)

- `frontend/src/pages/CourseStudents.tsx` (заменить заглушку)
- `frontend/src/styles/courseStudents.css` (новый)
- `frontend/src/api/teacherStudents.ts` (новый)
- `frontend/tests/T29-course-students.md` (отчёт, создать в конце)

`ru.json` менять нельзя — ключи `courseStudents.*`, `profile.*`, `common.*` уже есть.
Не хватает — русский литерал + отметка в отчёте.

## API-клиент (`api/teacherStudents.ts`)

```ts
import { apiFetch, buildQuery } from './client';
import type { EnrollmentStatus } from './types';

export type CourseStudentRow = {
  studentId: string; email: string; displayName: string;
  enrolledAt: string; status: EnrollmentStatus;
  completedLessons: number; totalLessons: number; percent: number;
  lastActivityAt: string | null;
};
export type CourseStudentPage = { items: CourseStudentRow[]; total: number; page: number; limit: number };
export type CourseStats = {
  studentsTotal: number; studentsActive: number; studentsCompleted: number;
  avgPercent: number; revenueCents: number;
  modulesTotal: number; lessonsTotal: number;
  reviewsCount: number; avgRating: number;
};

export function listCourseStudents(courseId: string, params: { q?: string; page?: number; limit?: number }): Promise<CourseStudentPage>;
export function courseStats(courseId: string): Promise<CourseStats>;
```

Если `EnrollmentStatus` не экспортируется из `types.ts` — опиши локальным союзом
`'active' | 'refunded' | 'revoked'`, не правя `types.ts`.

## Экран

Идентификатор курса — из `useParams` (`/teach/:id/students`).

1. **Шапка**: заголовок `courseStudents.title`, статус курса и название (получить
   существующим `getTeacherCourse` из `api/client`), ссылка `courseStudents.backToCourse`
   на `/teach/{id}`.
2. **Плитки статистики** (`courseStats`): всего учеников, учатся, завершили, средний
   прогресс (%), выручка (`formatMoney`).
3. **Поиск** по email/имени с debounce ~300 мс (сброс на страницу 1) и пагинация 20.
4. **Таблица**: ученик (email + имя), записан (`formatDate`), прогресс (полоса +
   `N из M`), статус (бейдж: учится / возврат / отчислен), последняя активность.
   Если `lastActivityAt === null` → `courseStudents.neverActive`.
5. **Пустое состояние** — `courseStudents.empty` (не пустая таблица).
6. **Ошибки**: 403/404 — понятный текст («Курс не найден или нет доступа»),
   401 — как в проекте (гварды уже стоят в роутере).
7. Доступность: `<th scope="col">`, `aria-label` у полосы прогресса
   (`aria-valuenow`), `<label>` у поиска.

Прогресс-полоса — тот же визуальный язык, что в `MyCourses.tsx` (посмотри и повтори
структуру, но **не правь этот файл**).

## Проверка

```
cd frontend
npx tsc --noEmit
npx vite build
```

Отчёт `frontend/tests/T29-course-students.md`: что сделано, какие ключи использованы,
какие литералы остались, что не проверено сквозным тестом.

## Критерии приёмки

- Заглушка заменена рабочим экраном, роут открывается.
- Поля строго из контракта T22 (поле в поле).
- Деньги — только через `formatMoney`; даты — через `formatDate`.
- Никаких `alert()`; ошибки — текстом API.
