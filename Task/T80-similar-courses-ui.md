# T80 — Фронтенд: блок «Похожие курсы» на странице курса

**Волна:** P9  **Ветка:** `task/T80-similar-courses-ui`  **Worktree:** `wt/t80`

## Суть

Нужно удержать ученика: показать курсы той же категории под программой курса.
API — по разделу «Волна P9» в `Task/00-api-contract.md` (**заморожен**);
бэкенд T79 идёт параллельно.

Работать только в своём worktree `wt/t80` (ветка `task/T80-similar-courses-ui`).

## Что сделать

1. `frontend/src/api/client.ts`: `getSimilarCourses(courseId, limit?)` →
   `CourseItem[]` (конверт `{items,total}`); обратная совместимость.
2. Новый компонент `frontend/src/components/SimilarCourses.tsx`:
   - заголовок «Похожие курсы», сетка карточек (переиспользуйте `CourseCard`,
     не меняя его API);
   - запрос через react-query по `courseId`, limit 4;
   - если `items` пуст — компонент не рендерится (никакого пустого блока);
   - состояния загрузки — скелетон/плейсхолдер в стиле соседних блоков, ошибки
     не показываем (блок просто скрыт);
   - ссылка «Все курсы категории» на `/?category=<category>` (category
     передайте пропом со страницы курса).
3. `frontend/src/pages/Course.tsx`: встроить блок после секции отзывов (или
   после программы — выберите место по вёрстке, не ломая существующие блоки),
   передать `course.id` и `course.category`.
4. Стили — новый `frontend/src/styles/similar.css` (импорт в компоненте), на
   переменных дизайн-системы. Чужие css не править.
5. i18n — только готовые ключи `p9.similar.*`; словарь не править.
6. Тесты: новый `frontend/tests/api-contract/similar-courses.test.mjs`:
   запрос с limit, URL-кодирование id, парсинг пустого и непустого ответа.

## Разрешённые файлы

- `frontend/src/components/SimilarCourses.tsx` (новый)
- `frontend/src/pages/Course.tsx`
- `frontend/src/api/client.ts`, `frontend/src/api/types.ts`
- `frontend/src/styles/similar.css` (новый)
- `frontend/tests/api-contract/similar-courses.test.mjs` (новый)
- `frontend/tests/T80-similar-courses-ui.md` (отчёт)

## Запрещено

- `backend/**`, `Task/**`, `specs/**`, `frontend/src/i18n/ru.json`, чужие
  страницы/компоненты/стили, `qa-*.ps1`, `docker-compose*`.
- Ломать сигнатуры существующих клиентских функций и менять `CourseCard`.
- `git push`, `git merge`, смена ветки, работа вне `wt/t80`.

## Проверка

Из папки `frontend/`:

```
npm run build
npm test
```

Сборка и все тесты (старые + новые) зелёные. При таймаутах порта 8799 —
подождите 30–60 секунд и повторите `npm test`.

## Отчёт

`frontend/tests/T80-similar-courses-ui.md`: «Что сделано», «Проверка»,
«Отклонения», «Открытые вопросы». Закоммитьте отчёт и код в ветку
`task/T80-similar-courses-ui`.
