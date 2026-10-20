# T76 — Фронтенд: превью-уроки

**Волна:** P9 · **Ветка:** `task/T76-lesson-preview-ui` · **Worktree:** `wt/t76`

## Что сделано

- `frontend/src/api/types.ts`
  - `Lesson.isPreview?: boolean` — аддитивное поле публичного и полного вида урока.
  - Тип `LessonPreview` — ответ `GET /courses/{slug}/preview/{lessonId}`
    (`course{id,slug,title}`, `lesson: Lesson`).
  - `CreateLessonInput.isPreview?: boolean` для создания урока.
- `frontend/src/api/client.ts`
  - `getLessonPreview(slug, lessonId)` — публичный `GET` с
    `encodeURIComponent` для обоих параметров.
  - `createLesson` отправляет `isPreview` только если он задан явно
    (отсутствие поля при создании backend трактует как `false`; старые тела
    запросов не изменились).
  - `updateLesson(courseId, lessonId, body)` — PATCH урока, вынесен из
    `Constructor.tsx` в клиент, чтобы контракт `isPreview` проверялся
    контрактными тестами (сигнатура существующих функций не менялась).
- `frontend/src/pages/Course.tsx` (точечные аддитивные правки)
  - В программе курса у уроков с `isPreview` — бейдж «Превью».
  - Для гостя/незаписанного пользователя — кнопка «Смотреть превью».
  - Модалка: `role="dialog"`, `aria-modal`, заголовок из `p9.preview.title`,
    закрытие по Esc, клику по фону и крестику; минимальный фокус-трап
    (Tab по кругу), автофокус на крестике и возврат фокуса на кнопку.
  - Содержимое: `<video controls>` при `videoUrl`, Markdown-текст `bodyMd`
    через общий `Markdown` из `LessonView`, для `live` — дата и ссылка на эфир.
  - Состояния загрузки (`role="status"`) и ошибки (`role="alert"`) с повтором;
    текст ошибки берётся из `ApiError.message`.
  - Существующая навигация записанных учеников не менялась: у них строка
    остаётся ссылкой на урок, добавлен только бейдж.
- `frontend/src/pages/Constructor.tsx`
  - Чекбокс «Доступен без записи (превью)» (`p9.preview.toggle`) и подсказка
    (`p9.preview.toggleHint`) — общий для всех типов урока.
  - В режиме правки чекбокс показывает текущее значение `lesson.isPreview`,
    в программе курса у таких уроков выводится бейдж.
  - `isPreview` уходит в `POST` (создание) и `PATCH` (правка) урока.
- `frontend/src/styles/preview.css` (новый)
  - Бейдж, кнопка, модалка, состояния, чекбокс; только переменные дизайн-системы
    (`--surface`, `--fg`, `--muted`, `--line`, `--sun-soft`, `--cta-blue`,
    `--r-*`, `--shadow` и т. д.). Импортируется из обеих страниц.
- `frontend/tests/api-contract/preview.test.mjs` (новый)
  - `getLessonPreview`: 200 и форма конверта, кодирование slug/lessonId
    (кириллица, пробел, слэш), 404 «Урок не найден» и «Курс не найден».
  - `createLesson`: `isPreview=true`, явный `false`, отсутствие поля
    (обратная совместимость).
  - `updateLesson` (PATCH): `isPreview=false`, `isPreview=true` для `live`,
    404 урока.

`ru.json`, backend, `Task/**`, чужие страницы, компоненты и стили не менялись.

## Проверка

Из `frontend/`:

```
npm run build
```

- `tsc --noEmit` — без ошибок;
- `vite build` — успешно, 148 модулей, `✓ built`.

```
npm test
```

- `npm run test:build` — успешно;
- `node --test tests/api-contract/*.test.mjs` — **131 тест, 131 pass, 0 fail**
  (включая 10 новых тестов T76).

## Отклонения

1. В `api/client.ts` добавлена функция `updateLesson` (PATCH урока). Без неё
   требование «передача `isPreview` при PATCH» нельзя проверить контрактным
   тестом: прежний PATCH жил локально в `Constructor.tsx` и не экспортировался.
   Локальная обёртка `patchLesson` сохранена, её сигнатура не менялась.
2. Кнопка «Смотреть превью» показывается только гостю/незаписанному
   пользователю: у записанного строка программы — существующая ссылка на урок,
   и вложенная кнопка сломала бы навигацию. Бейдж «Превью» виден всем.
3. Текст урока в модалке рендерится общим `Markdown` из `LessonView`, а не
   отдельным рендерером — чтобы оформление совпадало со страницей обучения.
4. В программе конструктора дополнительно показан бейдж «Превью» рядом с
   названием урока — наглядное «текущее значение» помимо чекбокса.

## Открытые вопросы

- Нет.
