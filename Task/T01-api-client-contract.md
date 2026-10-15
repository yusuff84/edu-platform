# T01 — Контракт API-клиента фронтенда

**Ключ сессии:** `T01-api-client-contract`
**Волна:** 1 · **Слой:** frontend
**Приоритет:** P0 — без этой задачи 5 страниц падают с `TypeError`

## Проблема

`frontend/src/api/client.ts` описывает ответы backend неправильно:

| Что ожидает клиент | Что реально отдаёт backend | Где ломается |
|---|---|---|
| `getCourse` → `Course` | `{ course, modules }` | `Course.tsx:73` `.some is not a function`, `Learn.tsx:13` `.find is not a function` |
| `myCourses` → `MyCourse[]` | `{ items, total }` | `MyCourses.tsx:27` |
| `myTeaching` → `Course[]` | `{ items, total }` | `Cabinets.tsx:60` |
| `reviewQueue` → `Course[]` | `{ items, total }` | `Admin.tsx:72` |
| `listReviews` → `Review[]` | `{ items, total, avgRating, ratingsCount }` | `Reviews.tsx:72` |
| `completeLesson` → `{ accepted }` | `{ passed, completed }` | `Learn.tsx:48` — вердикт всегда «не зачтено» |
| `createReview` шлёт `{ text }` | ждёт `{ body }` | 400 на каждый отзыв |
| `createLesson` шлёт `textContent`/`scheduledAt`/`quiz:[]`/`codeTask:string` | ждёт `bodyMd`/`startsAt`/`quiz:{questions}`/`codeTask:{}` | 400, урок не создаётся |

Плюс расхождения в типах: `reviewsCount` vs `ratingsCount`, `coverUrl` vs `coverPath`,
`question` vs `text`, статус `rejected`, которого в backend не существует.

## Владение файлами (жёсткая граница)

```
frontend/src/api/client.ts      — переписать
frontend/src/api/types.ts       — создать (вынести типы)
```
Больше ничего не трогать. Страницы чинит T03–T06, T08 — они будут работать
по этому клиенту.

## Что сделать

1. Вынести все доменные типы в `frontend/src/api/types.ts`:
   `Role`, `LessonKind`, `CourseStatus` (`draft|review|published|archived`),
   `User`, `Course`, `CourseItem`, `Module`, `Lesson`, `QuizQuestion`,
   `QuizData`, `CodeTest`, `CodeTask`, `Review`, `MyCourse`, `CompleteResult`,
   `PaymentJournalItem`, `AdminSettings`, `EnrollmentResult`.
   Имена полей — строго по `Task/00-api-contract.md`.
2. Оставить в `client.ts` только транспорт (`apiFetch`, `ApiError`) и функции.
3. **Нормализовать конверты на границе клиента**: `getCourse`, `myCourses`,
   `myTeaching`, `reviewQueue`, `listReviews` возвращают плоские значения,
   с которыми удобно работать страницам. Внутри функции — распаковка
   `{ items: [...] }`.
   - `getCourse(slug)` → `{ ...Course, modules: Module[] }`
   - `myCourses()` → `MyCourse[]`, где `MyCourse = { course, percent, nextLesson: {lessonId,title,kind} | null }`
   - `myTeaching()` → `Course[]`
   - `reviewQueue()` → `Course[]`
   - `listReviews(courseId)` → `Review[]`
4. Исправить `createReview`: поле `body`, не `text`.
5. Исправить `createLesson`: `bodyMd`, `startsAt`, `quiz: { questions }`,
   `codeTask: { language, template, tests }`. **Собирай тело динамически** —
   отправляй только поля, релевантные `kind` (см. раздел «Отступления»
   контракта: лишнее поле = 400).
6. Исправить `completeLesson` → возвращать `{ passed, completed, percent, nextLesson, message? }`.
7. `completeLesson` должен уметь отправить **пустое тело** для `video_text`/`live`
   (`POST` без body), иначе backend ответит `400`.
8. `logout()` — обработать `204` (уже сделано в `apiFetch`, проверить).
9. Добавить функции для новых эндпоинтов US6 (их реализует T07, типы — здесь):
   `paymentJournal(params)`, `getAdminSettings()`, `updateAdminSettings(pct)`.
10. `formatPrice` — оставить, добавить `formatDate` (RFC3339 → `ru-RU`) для
    дат, которые понадобятся T04 и T08.
11. Добавить `unwrap`/нормализацию `ApiError`: при `401` прокидывать понятный текст.

## Критерии приёмки

- [ ] `npx tsc --noEmit` в `frontend/` проходит без ошибок
- [ ] `client.ts` не содержит `reviewsCount`, `coverUrl`, `textContent`,
      `scheduledAt`, `accepted`, `'rejected'`, `authorEmail`
- [ ] Каждая экспортируемая функция соответствует разделу контракта,
      соответствующему её эндпоинту
- [ ] В `client.ts` нет ни одного `fetch`/дублированной логики транспорта
- [ ] Типы переехали в `types.ts`, `client.ts` их **реэкспортирует**
      (`export type { ... } from './types'`), чтобы T03–T06 могли
      импортировать из `../api/client` без правок

## Проверка

```bash
cd frontend
npm ci
npx tsc --noEmit
```

## Отчёт

В конце работы перечисли: какие функции изменились, какие типы добавлены,
какие запросы теперь формируются иначе. Отметь любое место, где контракт
оказался неоднозначным.
