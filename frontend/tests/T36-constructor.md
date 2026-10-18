# T36 — отчёт: конструктор грузит курс по id, единый кэш, заморозка карточки в архиве

Волна P4. Ветка `task/T36-constructor-course-by-id`, worktree `wt/t36`.
Контракты: `Task/00-api-contract.md` (§8/§9/§10, «Правка карточки курса (T33)»),
`Task/T28-constructor-ui.md`, `Task/T29-course-students-ui.md`.

## Что изменено

1. `frontend/src/api/teacherStudents.ts`
   - Добавлен тип `TeacherCourseDetail` = `{ course: Course; modules: Module[] }`
     и функция `getTeacherCourseDetail(courseId)` — `GET /teacher/courses/{id}`
     возвращает конверт **как есть**, без распаковки. Конверт — общая форма
     данных для кэша `['teach-course', id]`, из которой и конструктор, и
     «Ученики курса» читают `.course`/`.modules`.
   - `getTeacherCourse` (только `course`) сохранён по ТЗ и теперь выражен через
     `getTeacherCourseDetail` (тонкая обёртка). На страницах после перехода
     на общий ключ он не вызывается — оставлен как публичная функция модуля
     (в письме T36 требуется «добавить рядом»).
   - `api/client.ts` не менялся (принадлежит T38).

2. `frontend/src/pages/Constructor.tsx`
   - Источник курса: `useQuery({ queryKey: ['teach-course', id], queryFn: () =>
     getTeacherCourseDetail(id ?? ''), enabled: !isNew, retry: false })`.
     Поиск курса в `myTeaching()` и публичный `getCourse(slug)` для карточки
     убраны; импорт `getCourse` удалён (`tsc` это подтвердил).
   - `teachingQuery` (`['teaching']`) оставлен только для списка на `/teach/new`
     и инвалидаций; определение текущего курса от него больше не зависит.
   - Состояния страницы `/teach/:id`: загрузка — `teach.courseLoading`;
     ошибка — `courseErrorText()`: `403`/`404` → `courseStudents.noAccess`,
     прочее → `teach.courseLoadFailed`; кнопка «Повторить» (`common.retry`)
     и ссылка назад (`nav.teach`) сохранены. Блоки, привязанные к
     `teachingQuery`/`ownCourse`, удалены.
   - Архив: `cardFrozen = course?.status === 'archived'`. В этом состоянии
     кнопка и форма «Редактировать карточку» не показываются, вместо них —
     подсказка `teach.cardArchivedHint` (`teach-note`). Кнопка «Удалить курс»
     и жизненный цикл («Вернуть из архива», «Копировать курс») не тронуты —
     удаление в архиве остаётся доступным.
   - Обоснование «почему видно у чужих»: `GET /teacher/courses/{id}` проверяет
     владение владелец/админ (контракт, §P3), поэтому админ теперь видит
     чужой курс, а не «Курс не найден».
   - `startCardEdit` дополнительно защищён `cardFrozen` (форма не откроется,
     если статус стал `archived` после загрузки).
   - Копирование курса: `await qc.refetchQueries(['teaching'])` больше не нужен
     (копия грузится напрямую по своему id); вместо него — неблокирующая
     инвалидация `['teaching']` + `['teach-course']` и переход на копию.

3. `frontend/src/pages/CourseStudents.tsx`
   - Ключ `['teacher-course', id]` заменён на общий `['teach-course', courseId]`.
   - Источник шапки — `getTeacherCourseDetail(courseId)`; `const course =
     courseQuery.data?.course`. Форма кэша/URL/статусы/инвалидации не менялись,
     локальных дублей запроса в файле нет. Конструктор после сохранения карточки
     инвалидирует `['teach-course']` по префиксу — шапка «Учеников» обновится
     сразу, без перезагрузки.
   - Остальные запросы (`['course-students-stats']`, `['course-students']`)
     не тронуты.

4. `frontend/src/styles/constructor*.css`, `courseStudents.css` — не менялись:
   новых классов не понадобилось (`teach-note`/`teach-card-actions` уже есть).

### Ключи словаря, использованные в T36

`teach.courseLoading`, `teach.courseLoadFailed`, `teach.cardArchivedHint`,
`courseStudents.noAccess`, а также существовавшие ранее `common.retry`,
`nav.teach`. `ru.json` не менялся; новых литералов интерфейса не добавлено
(надпись «Редактировать карточку» — прежний литерал без ключа, оставлен как был).

## Проверка

```
> npx tsc --noEmit
tsc exit: 0

> npx vite build
vite v5.4.21 building for production...
✓ 132 modules transformed.
dist/assets/Constructor-CkmONPJh.css      6.15 kB │ gzip: 1.65 kB
dist/assets/CourseStudents-DqNgCt2q.css   2.92 kB │ gzip: 0.99 kB
dist/assets/Constructor-B3GdECR5.js      37.05 kB │ gzip: 9.28 kB
dist/assets/CourseStudents-Byl8Mtay.js    6.44 kB │ gzip: 2.08 kB
✓ built in 57.89s
vite build exit: 0
```

Вывод команд — с машины разработчика (PowerShell, `node_modules` подключён
junction-ом), 2026-10-18.

## Что НЕ проверено (честно)

- **Живой стенд не проверялся.** Стенд (`localhost:3001`, `olymp-*`/
  `edu-platform-*`) собран из ветки main; кода T36 в нём нет, свой
  dev-сервер не поднимался (порты заняты стендом). Поэтому не проверены
  вручную: открытие `/teach/{id}` чужого курса под админом, отсутствие
  кнопки правки карточки у архивного курса, синхронное обновление
  `['teach-course']` между конструктором и «Учениками курса», тексты
  состояний в браузере. Запись/скриншотов не выдумываю.
- **Postgres-путь** (прод-БД) не проверялся — только типы и сборка; стенд
  и локальные прогоны не запускались.
- Не проверены чужие состояния: 403/404/401 в браузере, поведение при
  смене статуса курса в соседней вкладке (формы не открываются/скрываются
  по `cardFrozen`, но реакция на refetch вживую не смотрелась).
- Совместимость с бэкендом T33 (409 на PATCH в архиве) проверена только
  по контракту, не вживую.
- Смежное: если в волне P4 кто-то ещё положит данные под ключ
  `['teach-course', id]` в другой форме (не конверт), ожидания страниц
  разойдутся — инвариант зафиксирован комментарием в `teacherStudents.ts`.

## Ограничения задачи соблюдены

`api/client.ts`, `i18n/ru.json`, `api/types.ts`, `Task/**`, `specs/**`,
стили и чужие страницы — не менялись. Новых зависимостей нет. Правки локальны:
`api/teacherStudents.ts`, `pages/Constructor.tsx`, `pages/CourseStudents.tsx`,
этот отчёт.
