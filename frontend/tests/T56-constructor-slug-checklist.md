# T56 — Конструктор: slug, чеклист готовности, кнопки статусов (frontend). Отчёт

Дата: 2026-10-19. Ветка: `task/T56-constructor-slug-checklist`. Worktree: `wt/t56`.
Волна: P7. Контракт: `Task/00-api-contract.md`, раздел «Эндпоинты волны P7
(T53–T55)» — `POST /teacher/courses/{id}/unpublish` (T53), замороженные ключи
`p7.constructor.*`. Живой стенд и браузер не использовались — визуальную
приёмку делает координатор.

## Что сделано

### Файлы

| Файл | Что |
|---|---|
| `frontend/src/api/teacherExtra.ts` | +1 функция `unpublishTeacherCourse(courseId): Promise<Course>` (соседние не тронуты) |
| `frontend/src/pages/Constructor.tsx` | slug в форме создания и правке карточки; чеклист готовности; unpublish; унификация удаления курса |
| `frontend/src/styles/constructorExtra.css` | стили СВОИХ блоков, все классы с префиксом `.ce-` (секция T56 в конце файла) |

Другие файлы не тронуты: `ru.json`, `client.ts`, `router.tsx`, `types.ts`,
`package.json`, backend, задачи `Task/**` — без изменений (проверено
`git status --short`: только три файла выше).

### API (`teacherExtra.ts`)

- `unpublishTeacherCourse(courseId)` → `POST /teacher/courses/{id}/unpublish`
  (T53, тело не требуется). Транспорт — `apiFetch` из `client.ts`, как у соседних
  `archiveCourse`/`unarchiveCourse`/`withdrawCourse`; чужой `client.ts` не менялся.
 Примечание: в контракте T53 конверт ответа `{"course": <Course>}` (как у
 архивных действий T21), поэтому в функции обёртка распаковывается и наружу
 отдаётся `Course` (сигнатура `Promise<Course>` по ТЗ). `crudFetch` в
 `teacherExtra.ts` нет — в этом файле транспорт называется `apiFetch`.
- Ошибки по контракту: 404 «Курс не найден», 403 — чужой курс, 409 — статус не
  published. Обрабатываются общим механизмом страницы (`errText` → notice).

### Slug (форма `/teach/new` и правка карточки)

- Форма создания: поле `#course-slug` с label `p7.constructor.slugLabel` и
 подсказкой `p7.constructor.slugHint` (строка под полем, `.ce-field-hint`).
- Валидация: замороженный regex `^[a-z0-9](?:[a-z0-9-]{1,118}[a-z0-9])?$`,
 пустое (или из пробелов) значение считается корректным — в теле POST slug не
 отправляется вовсе, backend сгенерирует из названия (T41). Ошибка —
 `p7.constructor.slugInvalid` (показывается «на лету», `aria-invalid`).
 Схема zod (`courseSchema`) расширена полем `slug` с `.refine(... по тому же
 regex)` — повторная защита на уровне `handleSubmit`.
- Кнопка «Создать курс»: `disabled = isPending || !slugReady` — выключена,
 пока slug некорректен (пусто — можно). В `onInvalidCourse` фокус цепляется
 за `course-slug`. Порядок полей формы: название → описание → цена → slug.
- Правка карточки (T13 PATCH): то же поле (`#card-slug`) в форме карточки
 (label + hint + live-ошибка), префилл — текущий `course.slug`. Валидация в
 `onCardSubmit`: при некорректном непустом slug — notice `slugInvalid` и фокус
 на поле, мутация не запускается. В PATCH-теле `slug` уходит только непустым
 (пусто = «оставить как было»); участие в правке совпадает с остальной
 карточкой — draft|review|published, в архиве форма карточки не показывается
 (`cardFrozen`, там PATCH отвечает 409 и/UI заранее скрыт).
 Тело PATCH по-прежнему собирается только из переданных полей — лишние поля не
 отправляются (backend DisallowUnknownFields), при этом PATCH-контракт slug
 поддерживает (проверено по `backend/internal/handler/teacher_edit.go:64`,
 `Slug *string`) — только чтение бэкенда, изменения не вносились.

### Чеклист готовности к отправке на модерацию

- Рендерится в блоке «Отправить на модерацию» (только для draft). Заголовок —
 `p7.constructor.checklistTitle`; три пункта с ✓/✗ (`.ce-check-done` /
 `.ce-check-todo`):
  1. `p7.constructor.checklistModule` — `modules.length > 0`;
  2. `p7.constructor.checklistLesson` — суммарно ≥1 урок;
  3. `p7.constructor.checklistLessonFields` — каждый урок заполнен по
     правилам своего kind (функция `lessonReady`):
     - `video_text`: есть bodyMd ИЛИ videoUrl (после trim, достаточно одного);
     - `live`: есть startsAt И liveUrl;
     - `quiz`: ≥1 вопрос, у каждого непустой текст, ≥2 непустых варианта и
       `correct` в границах списка (аналог `validateLesson`);
     - `code`: язык выбран и тестов ≥1.
- Пока чеклист не закрыт: кнопка «Отправить на модерацию» disabled с
 `title=p7.constructor.checklistTitle`; когда всё готово — строка
 `p7.constructor.checklistReady` (`role="status"`, класс `.teach-success`)
 и кнопка активна.
- Список курсов (`/teach/new`): кнопка «Отправить» в карточке не менялась
 (чеклист по ТЗ — рядом с Submit в редакторе).

### Снятие с публикации (T53)

- У курса в статусе `published` в блоке жизненного цикла появилась кнопка
 `p7.constructor.unpublish` (класс `.teach-danger`).
- Подтверждение — inline-модалка (`.ce-modal`/`.ce-modal__box`,
 `role="dialog"`/`aria-modal`/`aria-labelledby`; стиль по образцу T50/T52):
 текст `p7.constructor.unpublishConfirm`, кнопки «Отмена» (`common.cancel`)
 и «Подтвердить» (`common.confirm`). Нативный `window.confirm` НЕ используется.
 Закрытие: Отмена, клик мимо окна, Esc; во время запроса окно остаётся
 открытым (кнопки disabled), при ошибке окно закрывается, чтобы notice был
 виден — оверлей перекрывает уведомление внизу страницы.
- После успеха: тост `p7.constructor.unpublished` (`role="status"`) +
 инвалидация `['teach-course', id]` и `['teaching']` (refetch курса и списка).

### Кнопка «Удалить курс»

- Видна всегда: и в списке `/teach/new`, и в редакторе (в правке карточки).
- Неактивна (`disabled`) для любого статуса кроме `draft`, с подписью
 `p7.constructor.deleteHint` (отдельная строка под кнопкой, `.ce-delete-hint`)
 и tooltip `title=deleteHint`.
- Попытка удаления не-draft блокируется на клиенте: в `onDeleteCourse(courseId,
 status?)` при статусе ≠ draft выводится тост `p7.constructor.deleteBlocked`
 (kind: error), запрос не отправляется. Кнопка disabled — это первая линия
 защиты, гвард в хендлере — вторая (страховка статуса, не доехавшего до рендера).
- Удаление черновика работает как раньше: `window.confirm(teach.confirmDelete)`
 → `DELETE /teacher/courses/{id}` (локальный CRUD-транспорт страницы) → тост
 «Курс удалён» + инвалидация `['teaching']`/`['teach-course']`. Логика
 `deleteCourseMutation` не менялась.

## Ключи i18n (все — из замороженного `ru.json`, файл не менялся)

`p7.constructor.*` — использованы ВСЕ 13 ключей раздела: slugLabel, slugHint,
slugInvalid, checklistTitle, checklistReady, checklistModule, checklistLesson,
checklistLessonFields, deleteHint, deleteBlocked, unpublish, unpublishConfirm,
unpublished. Новых ключей не добавлялось.

Прочие существующие ключи: `teach.save`, `teach.saving`, `teach.submitReview`,
`teach.confirmSubmit`, `teach.confirmDelete`, `teach.cardArchivedHint` (рендер
остался), `common.delete`, `common.cancel`, `common.confirm`, `common.error`.

Полнота ключей проверена компилятором: `t()` принимает только ключи из
`ru.json` (строгий `RuKey`), `tsc --noEmit` зелёный.

### Литералы вне `t()`

1. ✓/✗ — маркеры чеклиста (`aria-hidden`, смысл несут классы
   `.ce-check-done`/`.ce-check-todo` и текст пункта).
2. Русские строки неизменённых мест (сообщения `MSG_*`, «Редактировать карточку»,
   « курс» в составе «Удалить курс» и т.п.) — унаследованы, не новые.

## Проверка

```
cd C:\Users\96352\all_project\eduPlatform\edu-platform\wt\t56\frontend
npm install --no-audit --no-fund   # node_modules отсутствовал; added 76 packages
npm run build                      # tsc --noEmit: 0 ошибок; vite build: ✓ 138 modules,
                                   #   чанк Constructor-*.js (~41.7 КБ) собран
npm test                           # tests 61 / pass 61 / fail 0
```

- Тесты контрактного клиента (`tests/api-contract`) не затронуты:
  `client.ts` и его тесты не менялись, прогон остался 61/61 PASS.
- Маршруты/роутер, `Task/**` и прочие страницы не правились; расхождений с
  замороженным контрактом не обнаружено. Эндпоинт T53 на backend этого wt
  к моменту задачи ещё не подмержен — фронт написан строго по замороженному
  контракту; PATCH-карточки на backend уже принимает `slug`
  (только чтение backend-кода, изменений не вносилось).

## Отступления / замечания для координатора

1. Транспорт в `teacherExtra.ts` называется `apiFetch` (не `crudFetch` — такой
   функции в файле/`client.ts` нет); стиль функций-соседей сохранён.
2. Полная сигнатура T56 из ТЗ `unpublishTeacherCourse(): Promise<Course>`
   требует распаковки конверта `{"course": ...}` — сделано внутри функции,
   контракт ответа backend не нарушен.
3. Замороженный regex клиента допускает slug длиной 1–2 символа, а текст
   `slugHint`/`slugInvalid` и backend (`slugRe: {3,120}`) требуют 3–120:
   расхождение словаря/regex — по заморозке не исправляем, короткие значения
   отсекутся на backend с его 400-ответом («Некорректный адрес страницы…»).
   Если нужно — ключи/regex править отдельно.
