# T28 — UI: конструктор курса — предпросмотр, структура, жизненный цикл

Волна: P3. Зависимости: контракт T21 (бэкенд пишется параллельно — брать из
`Task/T21-course-lifecycle.md`). Разводка уже сделана: ссылка «Ученики курса» в шапке
курса, роут `/teach/:id/students`.

Это **главная задача волны**: раздел «составление курса» должен стать полноценным
рабочим местом, а не набором форм.

## Зачем

У учителя уже есть CRUD модулей/уроков (T19) и правка карточки (T13), но:

- **нельзя увидеть урок глазами ученика** — текст, квиз и код-задачу видно только в
  виде полей формы; ошибку в разметке учитель ловит уже на живом курсе;
- нельзя **скопировать курс** (учителя делают это вручную, по одному уроку);
- нельзя **отправить курс в архив** или **вернуть из архива**;
- нельзя **отозвать курс с модерации**, если передумал;
- нельзя **перенести урок в другой модуль** — только удалить и создать заново;
- нельзя переставить **вопросы квиза** и **тесты код-задачи** — порядок = порядок ввода.

## Владеет файлами (менять можно только их)

- `frontend/src/pages/Constructor.tsx` (правки внутри существующего файла)
- `frontend/src/components/LessonPreview.tsx` (новый)
- `frontend/src/styles/constructorExtra.css` (новый)
- `frontend/src/api/teacherExtra.ts` (новый)
- `frontend/tests/T28-constructor.md` (отчёт, создать в конце)

`ru.json` менять нельзя — ключи `teach.*` (duplicate/archive/unarchive/withdraw/preview/
moveLesson/…), `courseStudents.*`, `common.*` уже добавлены. Чего не хватает — русский
литерал + отметка в отчёте.

`components/LessonView.tsx`, `pages/Learn.tsx`, `api/client.ts`, `router.tsx` — **не трогать**
(их владельцы — T30 и координатор).

## API-клиент (`api/teacherExtra.ts`)

```ts
import { apiFetch } from './client';
import type { Course, CourseDetail, Lesson } from './types';

export function duplicateCourse(id: string): Promise<{ course: Course }>;              // POST /teacher/courses/{id}/duplicate
export function archiveCourse(id: string): Promise<{ course: Course }>;                // POST .../archive
export function unarchiveCourse(id: string): Promise<{ course: Course }>;              // POST .../unarchive
export function withdrawCourse(id: string): Promise<{ course: Course }>;               // POST .../withdraw
export function moveLesson(courseId: string, lessonId: string, moduleId: string): Promise<{ lesson: Lesson }>; // POST .../lessons/{lessonId}/move
```

Тела/ответы — строго из контракта T21. `-1` самодеятельности, лишних полей нет
(`DisallowUnknownFields` → 400).

## 1. Предпросмотр урока (`components/LessonPreview.tsx`)

Новый компонент. Рендерит **черновик** урока, ещё не отправленный на сервер, —
поэтому принимает не `Lesson`, а собранный объект:

```ts
export type LessonPreviewData = {
  title: string;
  kind: LessonKind;
  bodyMd: string;
  videoUrl: string;
  liveUrl: string;
  startsAt: string;
  quiz?: { questions: Array<{ text: string; options: string[]; correct: number; explanation?: string }> };
  codeTask?: { language: string; template?: string; tests: Array<{ name: string; input: string; expected: string; hidden?: boolean }> };
};
```

Обязательное поведение:

- `bodyMd` рендерить **существующим** `Markdown` из `components/LessonView` — импорт
  `import { Markdown } from './LessonView';`. Свой markdown-рендерер не писать.
- `video_text`: markdown + `videoUrl` как ссылка (не встраивать iframe).
- `live`: markdown + дата эфира + `liveUrl`.
- `quiz`: список вопросов, **правильный вариант подсвечен** с пометкой «Верный ответ»
  (это предпросмотр учителя — он должен видеть ключ), `explanation` показывается.
- `code`: язык, шаблон (`template`) моноширинным блоком, таблица тестов; скрытые
  (`hidden`) помечены бейджем «Скрытый тест» — в предпросмотре они видны.
- Пустые поля не рисуют пустых блоков; полностью пустой черновик → подсказка
  «Заполните текст, чтобы увидеть предпросмотр» (литерал — в отчёт).

Место в `Constructor.tsx`: в редакторе урока кнопка **«Предпросмотр»**
(`teach.preview`). UI — раскрывающаяся панель/двухколоночный режим **рядом с формой**
(не модалка поверх: учитель правит и сразу смотрит). Внутри — заголовок
`teach.previewHint` («Так урок увидит ученик»).

## 2. Структура курса

- В шапке курса — сводка: модулей / уроков (`teach.structure`, `teach.lessonsCount`),
  считается по уже загруженному `CourseDetail`.
- В редакторе урока — селект **«Перенести в модуль…»** (`teach.moveLesson`) со списком
  модулей курса, кроме текущего; по выбору → `moveLesson`, затем перезагрузка курса и
  сообщение `teach.moveLessonDone`. Селект показывать только при `status === 'draft'`
  (бэкенд вернёт 409 — не даём нажать зря).
- **Перестановка вопросов квиза** и **тестов код-задачи** кнопками Вверх/Вниз
  (`teach.moveUp` / `teach.moveDown`, уже есть) — правильные ответы (`correct` — индекс)
  должны пересчитываться вслед за перестановкой вариантов и вопроса. Это частая причина
  битых квизов: переставили варианты — поехал ключ.
- То же для **вариантов ответа** внутри вопроса: перестановка с сохранением `correct`.

## 3. Жизненный цикл курса

Кнопки в шапке курса, каждая — по своему статусу:

| Статус | Кнопки |
|---|---|
| `draft` | «Копировать курс», «В архив» |
| `review` | «Отозвать с модерации» |
| `published` | «Копировать курс», «В архив», ссылка «Публичная страница» |
| `archived` | «Вернуть из архива», «Копировать курс» |

- Подтверждения — текстами `teach.confirmDuplicate` / `teach.confirmArchive` /
  `teach.confirmWithdraw`.
- Успех — `teach.duplicated` / `teach.archived` / `teach.unarchived` / `teach.withdrawn`.
- После `duplicate` — **перейти на новый курс** (`/teach/{новый id}`), чтобы учитель
  сразу видел копию.
- «Публичная страница» — ссылка на `/courses/{slug}` в новой вкладке.
- Ошибки — текстом `ApiError.message` (409 «Менять структуру можно только в черновике»
  и т.п.), не `alert`.

## Стиль

`styles/constructorExtra.css`, префикс `.ce-`. Общий `styles.css` и `teach.css`
не трогать.

## Проверка

```
cd frontend
npx tsc --noEmit
npx vite build
```

Плюс ручная проверка логики перестановок: напиши в отчёте, какие места пересчитывают
`correct` и почему это верно.

## Критерии приёмки

- Предпросмотр показывает несохранённый черновик урока и использует общий `Markdown`.
- Все кнопки жизненного цикла шлют запросы строго из контракта T21.
- Перестановка вопросов/вариантов/тестов не ломает `correct`.
- `npx tsc --noEmit` и `npx vite build` — зелёные, новых зависимостей нет.
- Отчёт `frontend/tests/T28-constructor.md`: что сделано, какие литералы остались,
  что не проверено сквозным тестом.
