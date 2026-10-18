# T50 — Админка: раздел «Курсы» — максимум удобства (frontend)

**Волна:** P6 · **Ветка:** `task/T50-admin-courses-ux` · **Worktree:** `wt/t50`

## Зачем

Раздел `/admin/courses` — центральный экран модерации. Сейчас: поиск только по
названию, фильтр статуса, сортировки нет, состояния фильтров не живут в URL,
модерация — только в соседней вкладке `#queue`. Доводим до рабочего инструмента:
URL-состояние, чипы статусов со счётчиками, сортировка/фильтры, модерация прямо
в строке, ссылки на учеников/публичную страницу, аккуратная пагинация и вёрстка.

## Что сделать

1. `frontend/src/api/adminConsole.ts`:
   - `listAdminCourses` — новые необязательные параметры `sort`, `dir`,
     `teacherEmail`, `from`, `to` (тип сортировок — `AdminCourseSort`);
   - новая `unarchiveAdminCourse(id)` → `POST /admin/courses/{id}/unarchive`
     (эндпоинт из T48, контракт P6);
   - `archiveAdminCourse`/`unpublishAdminCourse` уже есть — не ломать.
2. `frontend/src/pages/AdminCourses.tsx` — переработать список:
   - состояние фильтров в URL (`useSearchParams`): `q`, `status`, `teacherEmail`,
     `sort`, `dir`, `page`; при загрузке читается из URL, все изменения пишутся
     в URL (ссылка на выборку должна работать); кнопка `adminCourses.reset`;
   - чипы статусов со счётчиками из `getAdminStats()`: Все (`chipAll`), Черновики
     (`statusDraft`), На модерации (`statusReview`), Опубликованные
     (`statusPublished`), В архиве (`statusArchived`); активный чип — текущий статус;
   - сортировка: select (`adminCourses.sort`, варианты `sortCreated/Updated/Title/
     Price/Students`) + переключатель направления (`dirAsc`/`dirDesc`); поле
     `teacherEmail` (debounce ~300 мс) и даты `from`/`to` (ГГГГ-ММ-ДД; при ошибке
     формата/диапазона — понятное сообщение, запрос не уходит);
   - модерация в строке (не уходя во вкладку «Очередь модерации»):
     * `review` → «Опубликовать» (`adminCourses.approve`, confirm
       `confirmApprove` → `moderateCourse(id, {decision:'approve'})` из `client.ts`)
       и «Отклонить» (`adminCourses.reject`, модалка с обязательным комментарием
       `rejectComment`; пустой — кнопка недоступна; ошибки API — текстом);
     * `published` → «Снять с публикации» (существующая кнопка);
     * `archived` → «Вернуть из архива» (`adminCourses.unarchive` →
       `unarchiveAdminCourse`);
     * не `archived` → «В архив» (существующая кнопка);
     после успеха — тост (`approvedDone` / `rejectedDone` / `unarchivedDone` и
     существующие), обновление списка и чипов; на время запроса строка disabled;
   - ссылки в строке: «Ученики» (`adminCourses.students`) → `/teach/{id}/students`,
     публичная страница (slug) — как сейчас (новая вкладка);
   - пагинация: Назад/Вперёд + «Показано N–M из total» (`adminCourses.showing`),
     выбор `limit` 20/50 (`adminCourses.perPage`), `keepPreviousData`;
   - вёрстка: таблица не вылезает за карточку; длинные названия/email переносить
     или обрезать с ellipsis (`title` с полным значением); действия в строке
     не обрезаются; контейнер `overflow-x: auto` для узких экранов; колонка
     действий — компактная (кнопки могут быть в столбик).
3. `frontend/src/styles/adminConsole.css` — дополнить под новые элементы
   (чипы, панель фильтров, модалка отклонения, строковые действия).
4. Прогнать проверки (ниже) и написать отчёт.

## Правила волны

- Работать только в своём worktree (`C:\Users\96352\all_project\eduPlatform\wt\t50`);
  коммит — в ветку `task/T50-admin-courses-ux`; **push не делать**.
- Править только: `frontend/src/pages/AdminCourses.tsx`,
  `frontend/src/api/adminConsole.ts`, `frontend/src/styles/adminConsole.css`
  и отчёт. `ru.json`, `client.ts`, `router.tsx`, другие страницы — не трогать.
- Ключи `ru.json` заморожены в prep-коммите волны — используй только
  существующие (`adminCourses.*`, `admin.*`, `common.*`); если чего-то не
  хватает — пиши в отчёте, ключи добавит координатор.
- Новые внешние зависимости не добавлять; backend не трогать; комментарии —
  по-русски.

## Проверка

```powershell
cd C:\Users\96352\all_project\eduPlatform\wt\t50\frontend
npm ci --prefer-offline   # первый раз в этом worktree
npm run build             # tsc --noEmit + vite build
npm test                  # контрактные тесты клиента — должны остаться зелёными
```

Живую приёмку (в т.ч. вёрстку) сделает координатор на стенде.

## Отчёт

`frontend/tests/T50-admin-courses-ux.md`: что сделано, как устроено URL-состояние,
список ключей i18n, которых не хватило (если есть), вывод команд.
