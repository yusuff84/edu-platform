# T57 — Фронтенд: обложка курса в конструкторе, каталоге и на странице (frontend)

**Волна:** P7  **Ветка:** `task/T57-course-cover-ui`  **Worktree:** `wt/t57`

## Суть

T54 (бэкенд) вводит `POST/DELETE /teacher/courses/{id}/cover` и поле `coverPath`
в видах. Требуется UI: загрузка/замена/удаление обложки в конструкторе,
отображение на публичной странице курса и в карточках каталога. Контракт
заморожен (раздел T54), ключи — `p7.cover.*` (см. заморозку ru.json).

## Что сделать (только `frontend/**`)

1. `frontend/src/api/teacherCover.ts` (НОВЫЙ файл):
   ```ts
   // T54: загрузка обложки курса (multipart/form-data, поле file).
   export function uploadCover(courseId: string, file: File): Promise<{ coverPath: string }> { ... }
   export function removeCover(courseId: string): Promise<void> { ... }
   ```
   multipart через `fetch` c `credentials:'include'`, **без** ручного
   `Content-Type` (браузер сам границы ставит); ошибки через `ApiError`.
   CSRF-заголовок нужен тем же механизмом, что и `apiFetch` (посмотри, как
   `api/teacherStudents.ts`/`crudFetch` тянут `X-CSRF-Token`; если он
   прикрепляется в apiFetch — повтори путь доступа к заголовку снаружи).
2. `frontend/src/pages/Constructor.tsx` — блок «Обложка» в карточке курса
   (только при `course.id`):
   - превью `<img>` (если `course.coverPath`; alt `t('p7.cover.alt')`),
     классы `tc-cover*` в `constructor.css`;
   - кнопки: `Загрузить обложку` (`upload`), `Заменить обложку` (`replace`),
     `Убрать обложку` (`remove`, с inline-подтверждением — стиль T49);
   - input[type=file] hidden, accept `image/jpeg,image/png,image/webp`;
   - клиентская проверка размера ≤5 МБ и типа → сообщение `typeError` без
     похода на сервер; на сервере — через POST c хранением ошибки;
   - тосты `saved`/`removed`, refetch курса; статус `archived` — кнопки
     disabled (409 придет с бэка, но и кнопки глушим).
3. `frontend/src/pages/Course.tsx` (публичная страница `/courses/:slug`):
   обложка в шапке курса — хиро с `<img>` (или при отсутствии — прежний стиль;
   страница круто смотрится без обложки, не ломаем) + `<main>`:
   обернуть страницу курса в `<main className="page">` — семантика (у
   остальных публичных страниц main сейчас тоже нет, но только эта страница
   закреплена за T57; остальные main проставит T61).
4. `frontend/src/pages/Catalog.tsx` — в карточках (`CourseCard.tsx`):
   если у курса есть `coverPath` — мини-превью на плитке (класс поверх
   существующих `.tile`); иначе прежний вид (никто не должен видеть «битую
   пикчю»). Внимание:CourseCard общий — не_me шаrd с T61 (она только стили:
   каталог правки в catalog.css допустимыeeee; jsx CourseCard — за T61-стайла
   не трогается).
5. Типы: `Course`/`CourseItem`/`CourseDetail` в `frontend/src/api/types.ts` —
   аддитивно поле `coverPath?: string | null`.

## Разрешённые файлы

- НОВЫЕ: `frontend/src/api/teacherCover.ts`,
  `frontend/tests/T57-course-cover-ui.md`;
- ПРАВКИ: `frontend/src/pages/Constructor.tsx`, `Course.tsx`, `Catalog.tsx`,
  `frontend/src/components/CourseCard.tsx`, `frontend/src/api/types.ts`
  (аддитивно), `frontend/src/styles/constructor.css`, `catalog.css`;
- НЕЛЬЗЯ: `client.ts`, `ru.json`, `router.tsx`, чужие страницы, глобальный
  `styles.css` (он в другом воркtame).

## Проверка

`npm run build && npm test` — зелёные. Для UI-поведения тест писать не
обязательно (приёмка живьём за координатором), но юнит на `uploadCover`
(multipart shape — поле `file`, нет JSON content-type) стоит добавить в
существующий набор коллюqe.dumps api-тестов T37.

## Отчёт

`frontend/tests/T57-course-cover-ui.md`. Коммит `T57: ...`, push
`task/T57-course-cover-ui`.
