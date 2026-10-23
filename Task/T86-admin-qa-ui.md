# T86 — Фронтенд: страница модерации Q&A

**Волна:** P10  **Ветка:** `task/T86-admin-qa-ui`  **Worktree:** `wt/t86`

## Суть

Страница `/admin/qa`: глобальные вопросы и ответы по всем курсам, фильтры по
курсу/уроку, поиск, раскрытие ветки ответов, удаление вопроса или отдельного
ответа. Бэкенд — T85 (`GET /admin/questions`), удаление — существующие
`deleteQuestion`/`deleteAnswer` из `frontend/src/api/client.ts`.
Контракт заморожен в `Task/00-api-contract.md`.

Работать только в `wt/t86` (ветка `task/T86-admin-qa-ui`), база —
`integration/p10`.

## Что сделать

1. Новый API-модуль `frontend/src/api/adminQuestions.ts`: типы
   `AdminQuestion`, `AdminAnswer`, `AdminQuestionsResponse`,
   `AdminQuestionsQuery`; `listAdminQuestions(query)` через `apiFetch` и
   `buildQuery`. Не правьте `client.ts`.
2. Новая страница `frontend/src/pages/AdminQa.tsx`:
   - заголовок/подзаголовок, ссылка «Назад в админку»;
   - фильтры: поиск, курс (используйте `listAdminCourses` из
     `api/adminConsole.ts`), урок (подгружается по выбранному курсу — можно
     через `getTeacherCourse`/`getCourse`; не добавляйте новых бэкендов);
   - список карточек вопросов: курс/урок (ссылки), автор, дата, текст,
     количество ответов, кнопка «Развернуть»;
   - раскрытые ответы: автор, роль (бейдж преподавателя), дата, текст,
     «Удалить ответ»;
   - «Удалить вопрос» с подтверждением, состояния pending, success/error,
     empty, loading, пагинация, URL-состояние;
   - после удаления обновлять список (`invalidateQueries`).
3. Новый CSS `frontend/src/styles/adminQa.css` в стиле существующих
   admin-страниц; адаптивность, доступные кнопки/aria-live.
4. Роут `/admin/qa` в `frontend/src/app/router.tsx` с
   `RequireRole roles={['admin']}` и lazy import, SEO.
5. Только замороженные ключи `p10.qa.*`, `p9.qa.teacherBadge`,
   `p9.qa.answers`, `common.*`, `admin.*` из `ru.json`. Не правьте `ru.json`;
   при нехватке ключа — отчёт.
6. Тест `frontend/tests/api-contract/admin-qa.test.mjs`: query string,
   форма ответа, вызовы удаления с courseId/questionId/answerId.

## Разрешённые файлы

- `frontend/src/api/adminQuestions.ts` (новый)
- `frontend/src/pages/AdminQa.tsx` (новый)
- `frontend/src/styles/adminQa.css` (новый)
- `frontend/src/app/router.tsx`
- `frontend/tests/api-contract/admin-qa.test.mjs` (новый)
- `frontend/tests/T86-admin-qa-ui.md` (отчёт)

## Запрещено

- `Task/**`, `specs/**`, `backend/**`, `ru.json`, `frontend/src/api/client.ts`,
  `frontend/src/api/types.ts`, `frontend/src/pages/Admin.tsx`,
  `docker-compose*`, `qa-*.ps1`, `.github/**`, `deploy/**`.
- `git push`, `git merge`, смена ветки, работа вне `wt/t86`.

## Проверка

```
cd frontend
npm run build
npm test
```

Если `npm test` на Windows не находит каталог, используйте
`node --test tests/api-contract/*.test.mjs` и отразите это в отчёте.

## Отчёт

`frontend/tests/T86-admin-qa-ui.md`: что сделано, проверка, отклонения,
открытые вопросы. Коммит явными путями.
