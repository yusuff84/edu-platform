# T72 — Фронтенд: вопросы и ответы на странице обучения

**Волна:** P9  **Ветка:** `task/T72-lesson-qa-ui`  **Worktree:** `wt/t72`

## Суть

Нужен блок Q&A под уроком: ученик задаёт вопрос, преподаватель и другие
ученики отвечают, свои сообщения можно удалить. API — по разделу «Волна P9»
в `Task/00-api-contract.md` (**заморожен**); бэкенд T71 идёт параллельно.

Работать только в своём worktree `wt/t72` (ветка `task/T72-lesson-qa-ui`).

## Что сделать

1. `frontend/src/api/types.ts`: `LessonAnswer`, `LessonQuestion`,
   `LessonQuestionsPage` (`items,total,page,limit`).
2. `frontend/src/api/client.ts`: `listLessonQuestions(courseId, lessonId?,
   page?)`, `askQuestion(courseId, lessonId, body)`,
   `answerQuestion(courseId, questionId, body)`,
   `deleteQuestion(courseId, questionId)`,
   `deleteAnswer(courseId, questionId, answerId)`. Конверты `{question}` /
   `{answer}` распаковывать как в соседних функциях.
3. Новый компонент `frontend/src/components/Questions.tsx`:
   - заголовок «Вопросы и ответы», счётчик, список вопросов текущего урока
     (новые первыми), ответы (старые первыми), бейдж «Преподаватель» у
     `isTeacher`;
   - форма вопроса: textarea 10..2000, счётчик символов, «Отправить вопрос»;
     для гостя — текст «Войдите…», для вошедшего без записи — «Записывайтесь…»
     (признак записи возьмите из уже загруженных данных страницы обучения;
     если их нет — показывайте форму, сервер вернёт 403 с текстом);
   - форма ответа под каждым вопросом (3..2000), «Ответить», отмена;
   - удаление вопроса/ответа автором (кнопка «Удалить» с подтверждением);
     права окончательно проверяет сервер, ошибки показывайте текстом
     `ApiError.message`;
   - состояния загрузки и пустое состояние;
   - инвалидация запроса списка после создания/удаления.
4. `frontend/src/pages/Learn.tsx`: встроить `<Questions courseId={…}
   lessonId={…} />` под содержимым урока (после навигации «Отметить
   пройденным»), не ломая существующую вёрстку.
5. Стили — новый `frontend/src/styles/qa.css` (импорт в `Questions.tsx`), на
   переменных/классах дизайн-системы. Чужие css не править.
6. i18n — только готовые ключи `p9.qa.*`; словарь не править, не хватает —
   в отчёт.
7. Тесты: новый `frontend/tests/api-contract/lesson-qa.test.mjs` по образцу
   существующих (мок-сервер 127.0.0.1:8799): список с lessonId, создание
   вопроса/ответа, удаление, ошибки 400/403/404.

## Разрешённые файлы

- `frontend/src/components/Questions.tsx` (новый)
- `frontend/src/pages/Learn.tsx`
- `frontend/src/api/client.ts`, `frontend/src/api/types.ts`
- `frontend/src/styles/qa.css` (новый)
- `frontend/tests/api-contract/lesson-qa.test.mjs` (новый)
- `frontend/tests/T72-lesson-qa-ui.md` (отчёт)

## Запрещено

- `backend/**`, `Task/**`, `specs/**`, `frontend/src/i18n/ru.json`, чужие
  страницы/компоненты/стили (кроме перечисленных), `qa-*.ps1`,
  `docker-compose*`.
- Ломать сигнатуры существующих клиентских функций.
- `git push`, `git merge`, смена ветки, работа вне `wt/t72`.

## Проверка

Из папки `frontend/`:

```
npm run build
npm test
```

Сборка и все тесты (старые + новые) зелёные; вывод приложите к отчёту. Тестовый
helper слушает фиксированный порт 127.0.0.1:8799 с ретраями; при конкуренции
возможны таймауты — подождите 30–60 секунд и повторите `npm test`.

## Отчёт

`frontend/tests/T72-lesson-qa-ui.md`: «Что сделано», «Проверка», «Отклонения»,
«Открытые вопросы». Закоммитьте отчёт и код в ветку `task/T72-lesson-qa-ui`.
