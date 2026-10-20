# T82 — Фронтенд: публичная страница преподавателя

**Волна:** P9  **Ветка:** `task/T82-teacher-profile-ui`  **Worktree:** `wt/t82`

## Суть

Нужна публичная страница преподавателя (имя, статистика, курсы) и ссылка на
неё со страницы курса. API — по разделу «Волна P9» в
`Task/00-api-contract.md` (**заморожен**); бэкенд T81 идёт параллельно.

Работать только в своём worktree `wt/t82` (ветка `task/T82-teacher-profile-ui`).

## Что сделать

1. `frontend/src/api/types.ts`: `TeacherPublic`, `TeacherProfile`.
2. `frontend/src/api/client.ts`: `getTeacherProfile(id)`.
3. Новая страница `frontend/src/pages/TeacherProfile.tsx`:
   - шапка: имя (`p9.teacher.title`, если displayName пуст — «Преподаватель»),
     KPI: курсы, ученики, рейтинг;
   - сетка published-курсов (`CourseCard`), пустое состояние;
   - состояния загрузки и 404 (`p9.teacher.notFound/notFoundHint`);
   - ссылки на курсы, адаптив.
4. `frontend/src/app/router.tsx`: публичный маршрут `/teachers/:id` (внутри
   общего Layout), lazy-импорт по стилю соседних маршрутов.
5. `frontend/src/pages/Course.tsx`: имя преподавателя в шапке курса — ссылка на
   `/teachers/<course.teacherId>` (сейчас `teacherId` уже есть в `Course`).
   Не ломать существующую вёрстку и блок покупки.
6. Стили — новый `frontend/src/styles/teacherProfile.css` (импорт в странице),
   на переменных дизайн-системы. Чужие css не править.
7. i18n — только готовые ключи `p9.teacher.*`; словарь не править.
8. Тесты: новый `frontend/tests/api-contract/teacher-profile.test.mjs`:
   профиль 200, парсинг курсов, 404.

## Разрешённые файлы

- `frontend/src/pages/TeacherProfile.tsx` (новый)
- `frontend/src/pages/Course.tsx`
- `frontend/src/app/router.tsx`
- `frontend/src/api/client.ts`, `frontend/src/api/types.ts`
- `frontend/src/styles/teacherProfile.css` (новый)
- `frontend/tests/api-contract/teacher-profile.test.mjs` (новый)
- `frontend/tests/T82-teacher-profile-ui.md` (отчёт)

## Запрещено

- `backend/**`, `Task/**`, `specs/**`, `frontend/src/i18n/ru.json`, чужие
  страницы/компоненты/стили (кроме перечисленных), `qa-*.ps1`,
  `docker-compose*`.
- Менять существующие маршруты и ломать сигнатуры клиентских функций.
- `git push`, `git merge`, смена ветки, работа вне `wt/t82`.

## Проверка

Из папки `frontend/`:

```
npm run build
npm test
```

Сборка и все тесты (старые + новые) зелёные. При таймаутах порта 8799 —
подождите 30–60 секунд и повторите `npm test`.

## Отчёт

`frontend/tests/T82-teacher-profile-ui.md`: «Что сделано», «Проверка»,
«Отклонения», «Открытые вопросы». Закоммитьте отчёт и код в ветку
`task/T82-teacher-profile-ui`.
