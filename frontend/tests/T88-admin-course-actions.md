# T88 — Действия супер-админа над курсом и навигация админки. Отчёт

Дата: 2026-10-24. Ветка: `task/T88-admin-course-actions`. Волна: P10.
Worktree: `wt/t88`. База ветки — `80facc7` (см. «Отклонения»).
Контракт T87 (`POST /admin/courses/{id}/publish`, admin-bypass в `/teacher/*`)
заморожен и соблюдён; живой стенд и браузер не использовались — визуальную
приёмку делает координатор.

## Что сделано

### Файлы

| Файл | Что |
|---|---|
| `frontend/src/api/adminConsole.ts` | `publishAdminCourse(id)` → `POST /admin/courses/{id}/publish`; alias `AdminCourse = AdminCourseRow`; необязательное поле `moderationComment?` в `AdminCourseRow` |
| `frontend/src/pages/AdminCourseDetail.tsx` | Блок «Действия»: редактирование, публичная страница, ученики, модерация, force publish, unpublish, archive/unarchive; плашка `moderationComment`; `useSeo`; вторая back-ссылка «В админку» |
| `frontend/src/pages/Constructor.tsx` | Режим супер-админа: `me()` + `isAdmin`, новая формула `editable`, баннер `adminMode`/`adminModeHint` и подсказка для архива |
| `frontend/src/pages/Admin.tsx` | Навигация на 7 разделов: users, courses, enrollments, stats, reviews, qa, promo-codes; `aria-label` — `p10.nav.aria` |
| `frontend/src/pages/AdminCourses.tsx` | `useSeo` + ссылка «Назад в админку» |
| `frontend/src/pages/AdminStats.tsx` | `useSeo` + ссылка «Назад в админку» |
| `frontend/src/pages/AdminUsers.tsx` | `useSeo` (back-ссылка уже была — не дублировал) |
| `frontend/src/pages/AdminEnrollments.tsx` | `useSeo` (back-ссылка уже была — не дублировал) |
| `frontend/src/styles/adminCourseDetail.css` | Стили `.acd-actions*`, `.acd-btn--disabled`, `.acd-moderation-comment` на существующих переменных |
| `frontend/tests/T88-admin-course-actions.md` | Этот отчёт |

`ru.json`, `client.ts`, `app/router.tsx`, `Task/**`, `specs/**`, `backend/**`
не тронуты. Quiz/code-разметка конструктора (T91) не менялась.

### API (`adminConsole.ts`)

- `publishAdminCourse(id)` — POST, id кодируется `encodeURIComponent`,
  ответ `{ course: AdminCourse }`; 409 (архив) и 404 приходят как
  `ApiError.status` и показываются текстом ошибки.
- `AdminCourse = AdminCourseRow` — имя типа из постановки; отдельная сущность
  не вводится, форма ответа T87 совпадает со строкой списка курсов.
- `moderationComment?: string` добавлен в `AdminCourseRow`, чтобы
  `AdminCourseDetailCourse` (наследует строку) получил поле из detail T87.
  В элементах `GET /admin/courses` поля нет, поэтому оно необязательное.

### `AdminCourseDetail.tsx`

- **Блок «Действия»** (`p10.courseAdmin.actions`):
  - «Редактировать курс» → `/teach/:id` (`p10.courseAdmin.edit`,
    title — `p10.courseAdmin.editHint`). Для архива ссылка заменена на
    неактивный `span.btn.acd-btn--disabled` (`aria-disabled`, title и
    подсказка `p10.courseAdmin.contentLockedArchived`).
  - «Открыть публичную страницу» → `/courses/:slug` в новой вкладке
    (`p10.courseAdmin.openPublic`).
  - «Ученики» → `/admin/enrollments?courseId=...` (`p10.courseAdmin.students`)
    вместо прежней ссылки на `/teach/:id/students`.
  - **Модерация** — только для `review`: textarea причины
    (`p10.courseAdmin.rejectComment`) с подсказкой
    `admin.rejectCommentRequired`, кнопки «Опубликовать»
    (`p10.courseAdmin.approve`) и «Отклонить» (`p10.courseAdmin.reject`,
    красная, disabled без причины). Используется существующий
    `moderateCourse` из `client.ts`; reject не подтверждается — причина уже
    введена рядом.
  - **Force publish** для `draft`/`review` (`p10.courseAdmin.forcePublish`)
    с confirm `p10.courseAdmin.forcePublishConfirm` → T87 publish.
  - **Снять с публикации** для `published` (`adminCourses.unpublish`,
    confirm `adminCourses.confirmUnpublish`).
  - **В архив** для всех, кроме архива (`adminCourses.archive`, confirm
    `adminCourses.confirmArchive`); **Восстановить** для архива
    (`adminCourses.unarchive`, confirm `teach.confirmUnarchive`).
  - Во время запроса все кнопки disabled (`courseAction.isPending`).
- **Сообщения** `msg` перенесены наверх страницы, к блоку действий: успех и
  ошибка видны сразу после нажатия, а не только у отзывов. Тексты успеха —
  существующие ключи (`adminCourses.approvedDone/rejectedDone/unpublished/
  archived/unarchivedDone`, `p10.courseAdmin.forcePublishDone`); ошибки —
  общий `errorText` (403/401/сеть/`ApiError.message`), 409 показывается
  текстом backend.
- После действия инвалидируются `['admin-course', id]`, `['admin-courses']`,
  `['admin-stats']`, `['review-queue']`, `['teach-course', id]`,
  `['teaching']` — статус и комментарий обновляются в деталях, списке,
  сводке и конструкторе.
- **Комментарий модерации** из detail T87 показывается спокойной плашкой
  (`teach.moderationComment`), если поле есть.
- **SEO/back-links**: `useSeo` с названием курса (fallback —
  `p7.adminCourse.info`); back-ссылки «К списку курсов» и «В админку»
  (`p10.courseAdmin.backToAdmin`).
- Удаление отзывов не дублируется: прежний inline-confirm и мутация
  оставлены как были, `msg` для них теперь показывается вверху.

### `Constructor.tsx` — режим супер-админа

- Текущий пользователь: `useQuery({ queryKey: ['me'], queryFn: me })`,
  `isAdmin = role === 'admin'`.
- `editable = course ? course.status === 'draft' || (isAdmin &&
  course.status !== 'archived') : false` — teacher по-прежнему правит только
  черновик.
- Баннер на существующем классе `teach-banner` (`role="note"`):
  `p10.courseAdmin.adminMode` + `adminModeHint`, ниже —
  `contentEditable` или `contentLockedArchived` для архива.
- Отправка на модерацию осталась действием черновика: блок показывается
  только при `status === 'draft'`. Для teacher поведение не изменилось; админ
  в `review`/`published` правит содержимое, но не видит повторную отправку.
  Для архива выводится прежний `MSG_CONTENT_LOCKED`.
- Quiz/code-разметка и логика уроков не затронуты.

### `Admin.tsx` — навигация

- В `<nav className="admin-nav admin-nav--pages">` семь ссылок в порядке:
  users, courses, enrollments, stats, reviews, qa, promo-codes.
- `aria-label` — `p10.nav.aria`; тексты — существующие заголовки разделов и
  `p10.nav.reviews/qa/promos`. Активный раздел не подсвечивается (в ТЗ
  необязательно).

### SEO и back-links

- `useSeo` добавлен на `AdminCourseDetail`, `AdminCourses`, `AdminUsers`,
  `AdminEnrollments`, `AdminStats` (у `Admin` уже был).
- «Назад в админку» добавлена на `AdminCourses` и `AdminStats`; в
  `AdminUsers`/`AdminEnrollments` ссылка на `/admin` уже существовала —
  оставлена без дублирования. В `AdminCourseDetail` — вторая ссылка рядом
  с прежней «К списку курсов».

### CSS (`adminCourseDetail.css`)

- `.acd-actions__links`, `.acd-actions__row` — flex с переносом и gap;
  `.acd-actions__group` — разделитель `--line-soft`; `.acd-actions__comment`
  — grid-поле шириной до 38rem; `.acd-actions__hint` — мелкая подсказка.
- `.btn.acd-btn--disabled` — приглушённая неактивная ссылка-кнопка
  (`opacity`, `cursor: not-allowed`, `pointer-events: none`).
- `.acd-moderation-comment` — плашка на `--warn-bg` с левой полосой,
  `overflow-wrap: anywhere`.
- На ≤720px кнопки блока действий растягиваются на всю ширину — как уже
  сделано для шапки. Новых дизайн-переменных нет.

## i18n

Использованы только замороженные и существующие ключи; `ru.json` не
правился.

- `p10.courseAdmin.*`: `edit`, `editHint`, `adminMode`, `adminModeHint`,
  `actions`, `moderate`, `approve`, `reject`, `rejectComment`,
  `forcePublish`, `forcePublishConfirm`, `forcePublishDone`, `openPublic`,
  `students`, `backToAdmin`, `contentEditable`, `contentLockedArchived`.
- `p10.nav.*`: `reviews`, `qa`, `promos`, `aria`, `backToAdmin`.
- Существующие: `adminCourses.*` (подтверждения и тосты), `admin.*`
  (`rejectCommentRequired`), `teach.*` (`moderationComment`,
  `confirmUnarchive`), `p7.adminCourse.*`, `adminUsers.title`,
  `adminEnrollments.title`, `adminStats.title`, `adminCourses.title`,
  `cabinet.adminTitle`, `common.*`, `error.*`.

Недостающих ключей не обнаружено.

## Проверка

- `npm ci` в `frontend` — чистая установка.
- `npm run build` (`tsc --noEmit && vite build`) — успешно, 0 ошибок.
- `npm test` — штатный скрипт падает **до** запуска тестов: Node 22 не
  принимает каталог `tests/api-contract` как аргумент `node --test`
  (`Cannot find module ...\tests\api-contract`). Это проблема скрипта и
  версии Node, не изменений T88.
- Fallback из постановки: `node --test "tests/api-contract/*.test.mjs"` —
  **193/193 pass**.
- `git status --short` перед коммитом — только 9 разрешённых файлов и этот
  отчёт; `dist`, `node_modules`, временные каталоги не добавлены.

## Отклонения

1. **База ветки.** Worktree создан от `80facc7`, а не от актуального
   `integration/p10` (`dccd199`); merge/rebase/смена ветки запрещены
   постановкой. Изменения аддитивные, но `Constructor.tsx` в
   `integration/p10` уже содержит правки T91 (≈86+/50−), поэтому при
   слиянии T88 возможен конфликт в этом файле — разрешать координатору.
2. **`moderationComment`** добавлен в `AdminCourseRow` внутри разрешённого
   `adminConsole.ts`, а не в `adminCourseDetail.ts` (файл не входит в
   разрешённые). `AdminCourseDetailCourse` наследует поле, отдельная правка
   не нужна.
3. **`npm test`** на Node 22 не работает из-за каталога в аргументе
   `node --test`; использован fallback из постановки. Скрипт не менялся —
   `package.json` вне разрешённых файлов.
4. **Ссылки `/admin/reviews`, `/admin/qa`, `/admin/promo-codes`** добавлены
   заранее (разрешено ТЗ); маршруты появятся в T84/T86/T90, до этого переход
   по ним даст 404.
5. **Отправка на модерацию в конструкторе** показывается только для
   `draft`. Для teacher это прежнее поведение; админ в `review`/`published`
   редактирует содержимое без повторной отправки, чтобы не получать 409.
6. **Back-links в `AdminUsers`/`AdminEnrollments`** уже были — не
   дублировал, добавил только `useSeo`.

## Открытые вопросы

- Порядок слияния T88 с `integration/p10` и разрешение конфликта
  `Constructor.tsx` с T91.
- Нужна ли админу кнопка «Отправить на модерацию» для `review` (сейчас нет:
  модерация доступна на странице курса, а содержимое правится в
  конструкторе).
- Плашка `moderationComment` появится только после интеграции backend T87:
  до этого поле отсутствует и блок не рендерится — деградация безопасная.
