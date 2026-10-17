# T30 — UI: кабинет ученика — профиль, честный прогресс, история платежей

Волна: P3. Зависимости: контракт T23 (бэкенд пишется параллельно — брать из
`Task/T23-me-extra.md`). Разводка уже сделана: роут `/profile`, ссылка «Профиль» в шапке,
заглушка страницы.

## Зачем

Три конкретных дефекта раздела ученика:

1. **Прогресс врёт.** Сервер не отдаёт список пройденных уроков, поэтому клиент
   сравнивает счётчики «на глаз». После перезахода и на другом устройстве прогресс
   расходится с реальностью, а «Продолжить» ведёт не туда.
2. **Нет профиля.** Ученик не может задать имя; в отзывах и в админке он виден
   строкой email или обрезанным UUID.
3. **Нет истории платежей.** Купил курс — проверить, прошёл ли платёж, можно только
   через админа.

## Владеет файлами (менять можно только их)

- `frontend/src/pages/Profile.tsx` (заменить заглушку)
- `frontend/src/pages/MyCourses.tsx` (правки внутри существующего файла)
- `frontend/src/pages/Learn.tsx` (правки внутри существующего файла)
- `frontend/src/styles/profile.css` (новый)
- `frontend/src/api/meExtra.ts` (новый)
- `frontend/tests/T30-student-cabinet.md` (отчёт, создать в конце)

`ru.json` менять нельзя — ключи `profile.*`, `my.*`, `learn.*`, `common.*` уже есть.
Не хватает — русский литерал + отметка в отчёте.
`components/LessonView.tsx`, `api/client.ts`, `router.tsx` — не трогать.

## API-клиент (`api/meExtra.ts`)

```ts
import { apiFetch, buildQuery } from './client';
import type { User } from './types';

export type CourseProgress = {
  courseId: string;
  completedLessonIds: string[];
  completedLessons: number;
  totalLessons: number;
  percent: number;
  nextLessonId: string | null;
  completed: boolean;
};
export type PaymentRow = {
  id: string; courseId: string; courseTitle: string; amountCents: number;
  currency: string; status: PaymentStatus; provider: string;
  createdAt: string; processedAt: string | null;
};
export type PaymentPage = { items: PaymentRow[]; total: number; page: number; limit: number };

export function courseProgress(courseId: string): Promise<CourseProgress>;   // GET /me/courses/{id}/progress
export function meProfile(): Promise<User>;                                  // GET /me/profile
export function updateProfile(displayName: string): Promise<User>;           // PATCH /me/profile
export function myPayments(params?: { page?: number; limit?: number }): Promise<PaymentPage>; // GET /me/payments
```

`PaymentStatus` бери из `./types` (уже экспортируется). Тело `PATCH` — ровно
`{ displayName }` (лишнее поле → 400).

## 1. Профиль (`/profile`)

- Карточка: email (только чтение), роль (переводится на русский), дата регистрации
  (`formatDate`), поле «Отображаемое имя» + «Сохранить».
- Валидация на клиенте: 2..60 символов после trim (`profile.nameHint`), иначе кнопка
  заблокирована и подпись-подсказка.
- Успех — `profile.saved`; ошибка — текст `ApiError.message`.
- После сохранения — `invalidateQueries(['me'])`, чтобы имя обновилось в шапке
  (шапка берёт `meQueryOptions`).
- Секция **«История платежей»**: таблица (дата, курс, сумма `formatMoney`, статус),
  пагинация; пусто — `profile.paymentsEmpty`.

## 2. Честный прогресс

**`MyCourses.tsx`**: заменить эвристику на серверный источник.

- Для каждой записи — `courseProgress(course.id)`. Используй `useQueries` из
  `@tanstack/react-query` (пакет уже установлен) — один массив запросов вместо N
  отдельных хуков в цикле.
- Пока прогресс грузится — нейтральный плейсхолдер в полосе, не «0 %».
- Если запрос прогресса упал (403 — запись отозвана) — показать статус записи, а не
  выдуманные проценты.
- Кнопка «Продолжить» ведёт на `/learn/{courseId}/{nextLessonId}`, если `nextLessonId`
  есть; иначе — на последний урок; при `completed` — подпись `profile.courseDone` и
  ссылка на первый урок.

**`Learn.tsx`**: список уроков должен помечать пройденные по
`completedLessonIds` с сервера (после загрузки `courseProgress`), а не по локальному
состоянию сессии. После успешного `complete` — инвалидация запроса прогресса, чтобы
список и полоса обновились.

## 3. Стиль

`styles/profile.css`, префикс `.pf-`. Существующие CSS не трогать.

## Проверка

```
cd frontend
npx tsc --noEmit
npx vite build
```

Отчёт `frontend/tests/T30-student-cabinet.md`: что сделано, что именно заменено в
`MyCourses.tsx`/`Learn.tsx` (до/после по строкам), какие литералы остались, что не
проверено сквозным тестом.

## Критерии приёмки

- Прогресс берётся **только** с сервера; локальных эвристик не осталось.
- Профиль сохраняется и отражается в шапке.
- История платежей работает с пагинацией; `processedAt === null` отображается как
  «ожидает подтверждения» (литерал — в отчёт).
- `npx tsc --noEmit`, `npx vite build` — зелёные; новых зависимостей нет.
