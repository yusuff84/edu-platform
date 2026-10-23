# T88 — Фронтенд: действия супер-админа над курсом и навигация

**Волна:** P10  **Ветка:** `task/T88-admin-course-actions`  **Worktree:** `wt/t88`

## Суть

Админ-страница курса показывает данные, но не даёт перейти к редактированию,
принудительно опубликовать курс или быстро открыть публичную страницу. Кроме
того, навигация админки ведёт только на якоря одной страницы. Нужно довести
экран курса до полноценного рабочего места супер-админа и связать разделы.
Бэкенд T87 даёт `POST /admin/courses/{id}/publish` и admin-bypass в
`/teacher/*`; контракт заморожен.

Работать только в `wt/t88` (ветка `task/T88-admin-course-actions`), база —
`integration/p10`.

## Что сделать

1. `frontend/src/api/adminConsole.ts`: добавьте
   `publishAdminCourse(id)` (`POST /admin/courses/{id}/publish`) с типом
   ответа `{ course: AdminCourse }`.
2. `frontend/src/pages/AdminCourseDetail.tsx`:
   - блок «Действия»: «Редактировать курс» → `/teach/:id`, «Открыть публичную
     страницу» → `/courses/:slug`, «Ученики» → `/admin/enrollments?courseId=...`,
     «Модерация»: опубликовать/отклонить с комментарием (существующий
     `moderateCourse`), «Опубликовать принудительно» (T87) с confirm,
     «Снять с публикации»/«В архив»/«Восстановить» — где применимо;
   - показывать `moderationComment` из detail (после T87), если есть;
   - disabled-состояния и подсказки для архива; success/error через
     существующий `msg`;
   - не дублировать уже существующие кнопки удаления отзыва/записи.
3. `frontend/src/pages/Constructor.tsx`: режим супер-админа:
   - получите текущего пользователя (`useQuery({queryKey:['me'], queryFn: me})`
     или существующий хелпер), определите `isAdmin`;
   - `editable = course?.status === 'draft' || (isAdmin && course?.status !== 'archived')`;
   - покажите заметный, но спокойный баннер `p10.courseAdmin.adminMode` +
     `adminModeHint` и подсказку `contentLockedArchived` для архива;
   - не меняйте бизнес-логику teacher: для teacher всё остаётся как было.
4. `frontend/src/pages/Admin.tsx`: добавьте в nav ссылки на
   `/admin/users`, `/admin/courses`, `/admin/enrollments`, `/admin/stats`,
   `/admin/reviews`, `/admin/qa`, `/admin/promo-codes` (последние три появятся
   в T84/T86/T90; ссылки можно добавить сразу). Используйте
   `p10.nav.*` + существующие ключи; активный раздел не обязателен.
5. Добавьте `useSeo` на `AdminCourseDetail`, `AdminCourses`, `AdminUsers`,
   `AdminEnrollments`, `AdminStats` (если ещё нет) и ссылку «Назад в админку»
   на страницах, где её нет. Не ломайте существующую логику.
6. CSS — в `adminCourseDetail.css`/`admin.css`; используйте существующие
   классы и переменные, без нового «дизайн-языка».
7. Только ключи `p10.courseAdmin.*`, `p10.nav.*` из `ru.json`; `ru.json` не
   править. При нехватке — отчёт.

## Разрешённые файлы

- `frontend/src/api/adminConsole.ts`
- `frontend/src/pages/AdminCourseDetail.tsx`
- `frontend/src/pages/Constructor.tsx`
- `frontend/src/pages/Admin.tsx`
- `frontend/src/pages/AdminCourses.tsx`
- `frontend/src/pages/AdminUsers.tsx`
- `frontend/src/pages/AdminEnrollments.tsx`
- `frontend/src/pages/AdminStats.tsx`
- `frontend/src/styles/adminCourseDetail.css`, `admin.css`
- `frontend/tests/T88-admin-course-actions.md` (отчёт)

## Запрещено

- `Task/**`, `specs/**`, `backend/**`, `ru.json`, `app/router.tsx` (роуты уже
  есть), `frontend/src/api/client.ts`, `docker-compose*`, `qa-*.ps1`,
  `.github/**`, `deploy/**`.
- Менять quiz/code-разметку конструктора — это T91.
- `git push`, `git merge`, смена ветки, работа вне `wt/t88`.

## Проверка

```
cd frontend
npm run build
npm test
```

## Отчёт

`frontend/tests/T88-admin-course-actions.md`: что сделано, проверка,
отклонения, открытые вопросы. Коммит явными путями.
