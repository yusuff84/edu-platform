# T56 — Фронтенд: конструктор — slug, чеклист, кнопки статусов (frontend)

**Волна:** P7  **Ветка:** `task/T56-constructor-slug-checklist`  **Worktree:** `wt/t56`

## Суть

В создании курса («конструктор», `/teach/new` и `/teach/:id`) не хватает
базового UX:
- бэкенд поддерживает кастомный `slug` (T41: транслитерация), но в форме его нет;
- непонятно, что мешает отправить курс на модерацию (нет чеклиста готовности);
- кнопка «Удалить курс» видна в любом статусе, хотя бэкенд позволяет удалить
  только черновик → пользователи бьются об `409`;
- нельзя снять курс с публикации (новый эндпоинт T53: учительский
  `POST /teacher/courses/{id}/unpublish`).

## Что сделать (только `frontend/**`)

1. `frontend/src/api/teacherExtra.ts` — добавить (по замороженному контракту):
   ```ts
   // T53: учитель снимает курс с публикации (published → draft).
   export function unpublishTeacherCourse(courseId: string): Promise<Course> {
     return crudFetch<Course>(`/teacher/courses/${courseId}/unpublish`, { method: 'POST' });
   }
   ```
   (стиль — как соседние `archive/unarchive/withdraw`; тип `Course` уже есть).
2. `frontend/src/pages/Constructor.tsx`:
   - форма создания (`/teach/new`): поле Slug с подписью
     `t('p7.constructor.slugLabel')`, подсказка `slugHint`, клиентская
     проверка `^[a-z0-9]([а-дез-])…` — РЕАЛЬНО только
     `^[a-z0-9](?:[a-z0-9-]{1,118}[a-z0-9])?$` (или пусто → оставить пустым,
     бэкенд сгенерирует); ошибка — `slugInvalid`, кнопка «Создать курс»
     disabled пока slug некорректен;
     дополнительно поле присутствует в правке карточки (PATCH) только для
     статусов `draft|review|published` (как остальная карточка).
   - чеклист готовности (блок рядом с «Отправить на модерацию»): три пункта
     (`checklistModule`, `checklistLesson`, `checklistLessonFields`) с
     галочками исходя из `GET /teacher/courses/{id}` («прогресс» уже есть);
     пока всё не готово: кнопка Submit disabled с подписью `checklistTitle`
     и легендой; когда готово — строка `checklistReady`.
   - статусы: у `published` появляется кнопка
     `t('p7.constructor.unpublish')`: confirm-модалка (текст
     `unpublishConfirm`, нативный confirm недопустим — стиль T49/T52 с
     inline-подтверждением) → `unpublishTeacherCourse` → тост `unpublished`
     (уведомления в стиле существующих toasts) + refetch курса;
   - «удалить»: кнопка delete видна ВСЕГДА (в т.ч. списке курсов и в
     редакторе), но НЕАКТИВНА для не-draft статусов с tooltip/подписью
     `deleteHint`; при попытке удаления не-draft (блокировка на клиенте)
     — сообщение `deleteBlocked`; дружелюбный тост уже существовать должен.
3. Все тексты — через `t()` со СТРОГО замороженными ключами `p7.constructor.*`
   (см. контракт волны P7). Новых ключей не добавлять.

## Разрешённые файлы

- `frontend/src/api/teacherExtra.ts` (только добавление функции),
  `frontend/src/pages/Constructor.tsx`,
  `frontend/src/styles/constructor.css` (тили-поддержка СВОИХ блоков,
  аккуратно, без глобальных правок), `frontend/src/styles/constructorExtra.css`;
- НЕЛЬЗЯ: `client.ts` (используем crudFetch из teacherExtra), router.tsx
  (маршрутов новых нет), ru.json, чужие страницы.

Проверка UI: `npm run build` → 0 ошибок; `npm test` → все зелёные; тесты
API-вызовов (react-query моки) обновить, если затронуты `crudFetch`.

## Отчёт

`frontend/tests/T56-constructor-slug-checklist.md` — как в T49: что сделано,
какие ключи использованы, придумцна `npm test`, счёт PASS, скрины DOM-мер не
нужны. Коммит `T56: ...`, push `task/T56-constructor-slug-checklist`.
