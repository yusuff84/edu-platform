# T82 — публичная страница преподавателя (фронтенд)

**Ветка:** `task/T82-teacher-profile-ui`  
**Worktree:** `wt/t82`  
**Контракт:** раздел «Волна P9» в `Task/00-api-contract.md` (T81, заморожен).

## Что сделано

- `frontend/src/api/types.ts` — добавлены `TeacherPublic` и `TeacherProfile`
  строго по форме ответа `GET /api/v1/teachers/{id}`
  (`teacher: {id, displayName, coursesCount, studentsCount, avgRating,
  ratingsCount}`, `courses: CourseItem[]`).
- `frontend/src/api/client.ts` — добавлены импорт/реэкспорт типов и функция
  `getTeacherProfile(id)`: `GET /teachers/{id}` с `encodeURIComponent` для id;
  конверт `{ teacher, courses }` возвращается как есть.
- `frontend/src/pages/TeacherProfile.tsx` (новый) — публичная страница
  `/teachers/:id`:
  - шапка: имя `displayName` (пустое → `p9.teacher.title` «Преподаватель»),
    декоративный аватар с первой буквой;
  - KPI: курсы (`coursesCount`), ученики (`studentsCount`), рейтинг
    (`avgRating` с запятой; без оценок — «—»);
  - сетка published-курсов через существующий `CourseCard` (ссылки на
    `/courses/<slug>`), счётчик курсов;
  - пустое состояние с кнопкой «Все курсы» в каталог;
  - состояния загрузки, 404 (`p9.teacher.notFound` / `notFoundHint`) и
    прочих ошибок (текст сервера + повтор);
  - адаптив: шапка и KPI перестраиваются на узких экранах.
- `frontend/src/app/router.tsx` — lazy-импорт `TeacherProfile` и публичный
  маршрут `/teachers/:id` внутри общего `Layout` (рядом с `/courses/:slug`);
  существующие маршруты не тронуты.
- `frontend/src/pages/Course.tsx` — в шапке курса, под описанием, добавлена
  ссылка на `/teachers/<course.teacherId>` (текст `p9.teacher.openProfile`).
  Вёрстка панели и блок покупки не менялись.
- `frontend/src/styles/teacherProfile.css` (новый) — стили страницы на
  переменных дизайн-системы (`--surface`, `--line`, `--muted`, `--brand`,
  `--r-lg`, `--r-sm`, `--shadow-sm`), брейкпоинты 880/560 px.
- `frontend/tests/api-contract/teacher-profile.test.mjs` (новый) — 4 теста:
  успешный профиль и разбор курсов, URL-кодирование id, пустой список курсов,
  404 → `ApiError(404)` с текстом сервера.
- `ru.json` и чужие файлы не изменялись; использованы только готовые ключи
  `p9.teacher.*`, `common.*`, `error.*`, `catalog.emptyCatalogTitle`.

## Проверка

Из `frontend/`:

```
npm run build
npm test
```

- `npm run build` — успешно: `tsc --noEmit` без ошибок, Vite собрал бандл,
  страница вынесена в отдельный чанк `TeacherProfile-*.js` + `TeacherProfile-*.css`.
- `npm test` — успешно: `125` тестов, `125` pass, `0` fail
  (включая 4 новых теста `getTeacherProfile`).

## Отклонения

1. **Имя преподавателя на странице курса.** В `Course`/`CourseItem` контракт
   отдаёт только `teacherId`, поля `teacherName` нет ни в типах, ни в
   бэкенде. Поэтому ссылка в шапке курса подписана готовым ключом
   `p9.teacher.openProfile` («Профиль преподавателя»), а имя показывается на
   самой странице `/teachers/:id`. Дополнительный запрос профиля ради имени
   на странице курса не делал (лишняя нагрузка и зависимость от ещё не
   смёрженного T81).
2. **Пустое состояние курсов.** Отдельного ключа `p9.teacher.*` для подсказки
   нет; заголовок взят из `catalog.emptyCatalogTitle`, поясняющая строка —
   статический русский текст в компоненте (словарь не правился).
3. **Бэкенд T81 в этом worktree отсутствует** (идёт параллельно). Фронтенд
   написан по замороженному контракту; контрактные тесты поднимают мок-API,
   поэтому не зависят от готовности бэкенда.

## Открытые вопросы

- После слияния T81 стоит проверить страницу на реальном ответе
  `GET /teachers/{id}` (в частности, пустой `displayName` и профиль admin).
- Если на странице курса понадобится именно имя автора, потребуется
  аддитивное поле `teacherName` в ответе курса (изменение контракта/бэкенда,
  вне рамок T82).
