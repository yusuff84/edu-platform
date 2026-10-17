# T30 — UI кабинета ученика: профиль, честный прогресс, история платежей

Дата: 2026-10-18. Ветка: `task/T30-student-cabinet-ui` (worktree `wt/t30`).
Контракт — `Task/T23-me-extra.md` (бэкенд пишется параллельно; поля и коды взяты буквально).

## Что сделано

### Новые файлы

- **`frontend/src/api/meExtra.ts`** — клиент контракта T23 без дублирования транспорта:
  - `courseProgress(courseId)` → `GET /me/courses/{id}/progress`;
  - `meProfile()` → `GET /me/profile`;
  - `updateProfile(displayName)` → `PATCH /me/profile`, тело ровно `{ displayName }`;
  - `myPayments({ page, limit })` → `GET /me/payments` (через `buildQuery`);
  - типы `CourseProgress`, `PaymentRow`, `PaymentPage`, `MeProfile`.
  - **Отклонение от буквы сигнатур ТЗ**: `meProfile`/`updateProfile` возвращают
    `MeProfile = User & { createdAt: string }`, а не голый `User` — в контракте T23
    у профиля есть `createdAt` (нужен для «Даты регистрации»), но в `api/types.ts`
    поля нет, а `types.ts` задаче T30 не принадлежит. Подтип совместим с `User`,
    вызовы в остальном коде не затронуты.

- **`frontend/src/styles/profile.css`** — все стили под префиксом `.pf-`:
  плейсхолдер незагруженного прогресса `.pf-progress-pending` (анимация по
  существующей палитре трека), `.pf-course-done`, `.pf-enroll-status`,
  карточка профиля `.pf-facts`, форма `.pf-name-form`, таблица `.pf-table`
  (+ адаптив в карточки с `data-label`), бейджи статусов `.pf-badge--*`,
  пагинация `.pf-pager`. Существующие CSS не менялись.

### `pages/Profile.tsx` — заглушка заменена страницей

- Карточка: email (только чтение), роль по-русски, дата регистрации (`formatDate`).
- Поле «Отображаемое имя»: валидация после `trim` 2..60 (кнопка заблокирована
  + подсказка `profile.nameHint`, `aria-invalid`); сохранение через `PATCH`.
  Черновик поля хранится отдельно (`nameDraft`), чтобы фоновый рефетч не затирал
  печатаемый текст. Успех — `profile.saved`, ошибка — `ApiError.message`.
- После сохранения: `setQueryData(['me-profile'], ответ)` и
  `invalidateQueries(['me'])` — шапка (`meQueryOptions`) перечитывает пользователя.
- «История платежей»: таблица (дата, курс, сумма `formatMoney`, статус),
  пагинация page/limit=20, кнопки `common.prev`/`common.next`, диапазон
  «Показано X–Y из N»; пусто — `profile.paymentsEmpty`. `processedAt === null`
  выводится подписью «ожидает подтверждения». `courseTitle === ''` (курс удалён)
  показывается как «—».

### `pages/MyCourses.tsx` — прогресс только с сервера

- Один `useQueries` по всем записям: `courseProgress(course.id)` на каждую,
  ключ `['course-progress', id]` (общий с Learn.tsx), `retry: false`.
- Поля `percent`/`nextLesson` из `GET /me/courses` в интерфейсе больше не
  используются вовсе: полоса, подписи и кнопка — только из ответа progress.
- Пока прогресс грузится — нейтральная полоса `.pf-progress-pending` и текст
  `common.loading`, без «0 %». При ошибке запроса — статус записи вместо
  процентов: 403 → «Запись на курс отозвана» (кнопка «Продолжить» скрывается),
  прочее → «Не удалось загрузить прогресс» + кнопка `common.retry`.
- «Продолжить» ведёт на `/learn/{courseId}/{nextLessonId}`, если он есть; иначе —
  на `/learn/{courseId}` (страница курса сама откроет нужный/первый урок).
  При `completed` — подпись `profile.courseDone`, ссылка ведёт туда же и
  открывает первый урок (Learn при `nextLessonId = null` берёт первый).
- Сводка вверху пересчитана: «Средний прогресс» — по загруженным прогрессам,
  пока их нет — «—»; «Завершено» — по полю `completed`.

### `pages/Learn.tsx` — честные отметки уроков

- Новый запрос `['course-progress', courseId]` (включён после того, как курс
  найден в «моих»); отметка уроков — ровно `new Set(completedLessonIds)`;
  «следующий урок» — `nextLessonId`; percent — `percent` из этого же ответа.
  Локальная эвристика («всё до nextLesson считается пройденным» + множество
  `completedNow`) удалена полностью.
- После успешного `complete` — `invalidateQueries(['course-progress', courseId])`
  (список и полоса обновляются из сервера) плюс прежняя инвалидация
  `['my-courses']`/`['me-courses']`.
- Пока прогресс не загружен — «—» и нейтральная полоса (`pf-progress-pending`),
  при ошибке — «—» без анимации.

### Не тронуто

`ru.json`, `api/client.ts`, `api/types.ts`, `components/LessonView.tsx`,
`app/router.tsx` — без изменений (проверено `git status`). Новых зависимостей нет:
использованы уже установленные `@tanstack/react-query` (`useQueries`,
`keepPreviousData`). Push не делался.

## До/после по строкам

Номера «до» — по исходным файлам на момент начала задачи, «после» — по итоговым.

### `MyCourses.tsx` (до: 131 строка → после: ~201)

| До (что было) | После (что стало) |
|---|---|
| стр. 9–11: доккомментарий «прогресс с сервера» | стр. 1–10: комментарий о T30 (прогресс только из `/progress`), стр. 12 — импорт `useQueries`, стр. 15 — `courseProgress`, стр. 17 — `profile.css` |
| — | стр. 23–36: `useQueries` — по одному запросу `courseProgress` на запись, `retry: false` |
| стр. 68–71: `avg` через `items.reduce(... m.percent ?? 0)` и `done` через `m.percent >= 100` (поля из `/me/courses`) | стр. 85–92: `loaded` (успешно загруженные прогрессы), `done` по `completed`, `avg` по `percent` из progress; пока ничего не загружено — `null` → «—» (стр. 104) |
| стр. 92–98: `items.map((item) => { const nextLessonId = item.nextLesson?.lessonId; ... })` | стр. 111–131: `progress = progressQueries[i]`, разбор 403/ошибки (`revoked`, `statusText`), `nextLessonId = data?.nextLessonId ?? null` |
| стр. 110–112: полоса ширины `item.percent`, `aria-label` | стр. 146–167: при наличии данных — полоса и meta (`my.progress` %, `profile.lessonsDone` или `profile.courseDone`) |
| стр. 113–118: meta с `item.percent` и `item.nextLesson?.title` | стр. 169–186: загрузка/ошибка — полоса `pf-progress-pending` и статус записи, без «0 %» |
| стр. 119–123: кнопка «Продолжить» на `href` из `item.nextLesson` | стр. 189–200: «Продолжить» на `nextLessonId` из progress (при 403 скрыта), при прочих ошибках добавлена «Повторить» |

### `Learn.tsx` (до: 406 строк → после: 421)

| До (что было) | После (что стало) |
|---|---|
| стр. 50–51: состояние `completedNow` («уроки, завершённые в этой сессии») | состояние удалено; стр. 72–81: запрос `['course-progress', courseId]` с `enabled` и `retry: false` |
| стр. 84–85: `percent` из `verdictState`/`meCourse.percent` | стр. 99–101: `percent` только из `progress` |
| стр. 87–90: `serverNextLessonId` из `verdictState`/`meCourse.nextLesson` | стр. 103–104: `serverNextLessonId = progress?.nextLessonId ?? null` |
| стр. 92–107: **эвристика** — все уроки до `nextLesson` по порядку + `completedNow` считались пройденными | стр. 106–110: `completedIds = new Set(progress?.completedLessonIds ?? [])` — ровно серверный список |
| стр. 135–137: `if (res.completed) setCompletedNow(...)` | удалено (список обновляет сервер) |
| стр. 138–141: инвалидация `my-courses`/`me-courses` | стр. 138–144: добавлена инвалидация `['course-progress', courseId]`, прежние две сохранены |
| стр. 277–291: полоса от `percent` (при отсутствии данных рисовался «0 %») | стр. 280–306: при данных — полоса/`aria-valuenow`; иначе «—» и `pf-progress-pending` (при ошибке — просто трек), `aria-valuetext` = `common.loading` |
| стр. 8–12: верхний комментарий без T30 | стр. 8–10: отметка о честном прогрессе; стр. 19, 23 — импорты `courseProgress` и `profile.css` |

## Литералы, оставшиеся в коде (ключей в `ru.json` нет, менять его нельзя)

1. `MyCourses.tsx`: **«Запись на курс отозвана»** (403 от `/progress` — «показать
   статус записи, а не выдуманные проценты», спека T30).
2. `MyCourses.tsx`: **«Не удалось загрузить прогресс»** (прочие ошибки запроса).
3. `Profile.tsx`: **«ожидает подтверждения»** — требуется критериями приёмки T30
   для `processedAt === null`.
4. Типографский «—» в `MyCourses.tsx` (средний прогресс до загрузки),
   `Learn.tsx` (процент до загрузки) и `Profile.tsx` (`courseTitle === ''` —
   курс удалён); сам `formatDate` уже возвращает «—» по контракту T30-клиента.

Предсуществующие литералы в `MyCourses.tsx`, не относящиеся к T30 и не
тронутые: «Курсов», «Средний прогресс», «Завершено», тег «Курс».

## Переиспользованные ключи из других неймспейсов `ru.json`

Профильных ключей под эти подписи в словаре нет, поэтому взяты существующие
(переводы точные, `t()` типизирован):

- роль: `adminUsers.roleStudent` / `adminUsers.roleTeacher` / `adminUsers.roleAdmin`;
- статусы платежей: `admin.statusPending` / `admin.statusSucceeded` /
  `admin.statusFailed` / `admin.statusRefunded`;
- пагинация: `admin.showing` («Показано») и `admin.of` («из»).

Ключи `profile.progress` / `profile.progressEmpty` не использованы: ТЗ T30
не описывает секцию «Мой прогресс» на странице профиля (прогресс живёт
в «Моих курсах» и на экране обучения).

## Что НЕ проверено (и сомнения) — честно

1. **Сквозной прогон с бэкендом T23 невозможен**: в этой ветке endpoint'ов
   `/me/courses/{id}/progress`, `/me/profile`, `/me/payments` ещё нет (бэкенд
   `me_extra.go` пишется параллельно), поэтому проверены только `tsc` и `vite
   build`; реальные ответы сервера, коды 403/404 и пагинация вживую не гонялись.
2. **«Смена» прогресса после `complete`** проверена только логически: инвалидация
   `['course-progress', courseId]` + перезапрос. Гонки визуально не проверялись.
3. **403 при отзыве записи**: показать «статус записи» точнее нельзя — в контракте
   `GET /me/courses` поля статуса записи нет, а 403 не различает refunded/revoked.
   Показываем одну формулировку на оба случая.
4. **«Иначе — на последний урок»** (п.2 ТЗ): в `MyCourses.tsx` нет списка уроков,
   чтобы вычислить последний, поэтому fallback — `/learn/{courseId}`, и первый/
   последний урок выбирает уже Learn (при `completed` это первый урок, при
   единственном вырожденном случае «0 уроков» — пустой курс). Расхождение
   осознанное.
5. **Отражение имени в шапке**: инвалидация `['me']` сделана, `GET /auth/me`
   отдаёт `displayName`, но текущая шапка (`app/router.tsx`) рендерит
   `user.email` — файл не принадлежит T30. Имя станет видимым в шапке, когда
   владелец шапки начнёт показывать `displayName`.
6. **Платёжный `currency`**: суммы выводятся через `formatMoney` (рубли), поле
   `currency` не отображается — так задано в ТЗ T30; для не-RUB платежей
   отображение будет без указания валюты.
7. **Стили/адаптив** `profile.css` не проверялись визуально (нет живого бэкенда
   для входа на страницы); проверены только сборкой.
8. **Мобильная раскладка** таблицы платежей (`.pf-table` → карточки) — по CSS,
   без реального узкого вьюпорта.

## Проверка

```
cd frontend
npx tsc --noEmit   → exit 0
npx vite build     → exit 0 (123 modules transformed; чанки Profile/MyCourses/Learn собраны)
```

`ru.json` не изменялся; новых зависимостей нет; push не выполнялся.
