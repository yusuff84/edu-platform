# T36 — Конструктор: курс по id, единый кэш, заморозка карточки в архиве (frontend)

**Волна:** P4 · **Ветка:** `task/T36-constructor-course-by-id` · **Worktree:** `wt/t36`
**Контракты:** `Task/00-api-contract.md` (§8/§9/§10, «Правка карточки курса (T33)»), `Task/T28-constructor-ui.md`, `Task/T29-course-students-ui.md`.

## Зачем

Конструктор ищет курс по `id` в списке **своих** курсов (`myTeaching()`), а затем грузит его
публичным `getCourse(slug)`. Из-за этого: (1) админ, открывший `/teach/:id` чужого курса, видит
«Курс не найден»; (2) страница «Ученики курса» (`CourseStudents.tsx`) грузит тот же курс из
`GET /teacher/courses/{id}` под ключом `['teacher-course', id]`, а конструктор — под
`['teach-course', id]` — правки карточки не подсвечиваются на соседнем экране.

Плюс с T33 бэкенд запрещает правку карточки в архиве (`409`) — интерфейс должен это показывать
заранее, а не ошибкой после нажатия.

## Что сделать

1. `frontend/src/api/teacherStudents.ts`: функция `getTeacherCourse(courseId)` уже есть
   (возвращает только `course`). Добавить рядом `getTeacherCourseDetail(courseId)`, возвращающий
   конверт `{ course: Course; modules: Module[] }` **как есть** (`GET /teacher/courses/{id}`).
   Типы — из `./types` (не менять `types.ts`, если там уже есть `Course`/`Module`).
2. `frontend/src/pages/Constructor.tsx`:
   - источник курса — `useQuery({ queryKey: ['teach-course', id], queryFn: () => getTeacherCourseDetail(id) })`;
     убрать поиск курса по `myTeaching()` и `getCourse(slug)` для карточки (если `getCourse` больше не
     используется — убрать импорт, иначе `tsc` подскажет);
   - `teachingQuery` (`['teaching']`) можно оставить только для того, для чего он реально нужен
     (например, список/инвалидации) — но не для определения текущего курса;
   - состояния: загрузка → `teach.courseLoading`, `403`/`404` → `courseStudents.noAccess`,
     прочие ошибки → `teach.courseLoadFailed` (все ключи уже есть в словаре);
   - инвалидации `['teach-course']` (по префиксу ключа) сохранить/привести к одному виду.
3. `frontend/src/pages/CourseStudents.tsx`: перейти на ключ `['teach-course', courseId]` и на
   `getTeacherCourse`/`getTeacherCourseDetail` из `api/teacherStudents.ts` (убрать локальные дубли),
   инвалидации тоже перевести на `['teach-course']`.
4. **Архив**: в конструкторе кнопка/форма «Редактировать карточку» недоступна, когда
   `course.status === 'archived'`, вместо неё — подсказка `teach.cardArchivedHint`
   (карточка заморожена, `PATCH` вернёт `409` — см. T33). Прочие действия в архиве
   («Вернуть из архива», «Копировать курс», «Удалить курс») не трогать.
5. Ничего не менять в `frontend/src/api/client.ts` — этот файл в волне принадлежит T38.

## Проверка

```powershell
cd C:\Users\96352\all_project\eduPlatform\edu-platform\frontend
npx tsc --noEmit
npx vite build
```

Оба — `exit 0`. Затем ручная проверка на стенде (`localhost:3001`, `WEB_PORT=3001`):
войти админом `admin@local / admin1234`, открыть `/teach/{id}` **чужого** курса → конструктор
показывает курс (не «Курс не найден»); у архивного курса нет кнопки правки карточки.
Если стенд не поднимается или мешает — честно написать это в отчёте, запись в браузере не выдумывать.

## Ограничения

- Править только: `frontend/src/pages/Constructor.tsx`, `frontend/src/pages/CourseStudents.tsx`,
  `frontend/src/api/teacherStudents.ts`, `frontend/src/styles/constructor*.css`,
  `frontend/src/styles/courseStudents.css` (по необходимости), отчёт `frontend/tests/T36-constructor.md`.
- `frontend/src/i18n/ru.json`, `frontend/src/api/client.ts`, `Task/**`, `specs/**` — не трогать.
  Новые строки интерфейса — только ключами из словаря, которых **уже достаточно** (см. список выше).

## Отчёт

`frontend/tests/T36-constructor.md`: что изменено, как проверено (`tsc`, `vite build`, живой стенд),
что осталось непроверенным (обязательно: Postgres-путь, чужие состояния), скриншотов не требуется.
