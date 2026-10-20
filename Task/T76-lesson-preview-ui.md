# T76 — Фронтенд: превью-уроки (страница курса и конструктор)

**Волна:** P9  **Ветка:** `task/T76-lesson-preview-ui`  **Worktree:** `wt/t76`

## Суть

Нужно показать на странице курса, какие уроки доступны без записи, открывать
превью в модалке и управлять флагом в конструкторе. API — по разделу «Волна
P9» в `Task/00-api-contract.md` (**заморожен**); бэкенд T75 идёт параллельно.

Работать только в своём worktree `wt/t76` (ветка `task/T76-lesson-preview-ui`).

## Что сделать

1. `frontend/src/api/types.ts`: `Lesson.isPreview?: boolean`; тип
   `LessonPreview` (`course{id,slug,title}`, `lesson: Lesson`).
2. `frontend/src/api/client.ts`: `getLessonPreview(slug, lessonId)`.
3. `frontend/src/pages/Course.tsx`: в программе курса у уроков с `isPreview`
   бейдж «Превью» и кнопка «Смотреть превью»; по клику — модалка
   (`role="dialog"`, закрытие по Esc/фону/крестику, фокус-трап минимальный):
   видео (`<video controls>` для videoUrl), текст (bodyMd), для live — ссылка;
   состояния загрузки/ошибки. Не ломать существующую программу и навигацию.
4. `frontend/src/pages/Constructor.tsx`: в редакторе урока чекбокс
   «Доступен без записи (превью)» + подсказка; отправлять `isPreview` в
   create/patch урока; показывать текущее значение.
5. Стили — новый `frontend/src/styles/preview.css` (импорт в обе страницы),
   на переменных дизайн-системы. Чужие css не править.
6. i18n — только готовые ключи `p9.preview.*`; словарь не править.
7. Тесты: новый `frontend/tests/api-contract/preview.test.mjs`:
   `getLessonPreview` (200, кодирование slug/lessonId, 404), передача
   `isPreview` при создании и PATCH урока.

## Разрешённые файлы

- `frontend/src/pages/Course.tsx`
- `frontend/src/pages/Constructor.tsx`
- `frontend/src/api/client.ts`, `frontend/src/api/types.ts`
- `frontend/src/styles/preview.css` (новый)
- `frontend/tests/api-contract/preview.test.mjs` (новый)
- `frontend/tests/T76-lesson-preview-ui.md` (отчёт)

## Запрещено

- `backend/**`, `Task/**`, `specs/**`, `frontend/src/i18n/ru.json`, чужие
  страницы/компоненты/стили, `qa-*.ps1`, `docker-compose*`.
- Ломать существующие сигнатуры клиентских функций.
- `git push`, `git merge`, смена ветки, работа вне `wt/t76`.

## Проверка

Из папки `frontend/`:

```
npm run build
npm test
```

Сборка и все тесты (старые + новые) зелёные. При таймаутах порта 8799 —
подождите 30–60 секунд и повторите `npm test`.

## Отчёт

`frontend/tests/T76-lesson-preview-ui.md`: «Что сделано», «Проверка»,
«Отклонения», «Открытые вопросы». Закоммитьте отчёт и код в ветку
`task/T76-lesson-preview-ui`.
