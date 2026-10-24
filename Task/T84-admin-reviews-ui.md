# T84 — Фронтенд: страница модерации отзывов

**Волна:** P10  **Ветка:** `task/T84-admin-reviews-ui`  **Worktree:** `wt/t84`

## Суть

Нужна страница `/admin/reviews`: глобальный список отзывов, фильтры по курсу,
оценке, периоду и поиску, пагинация и удаление отзыва с подтверждением.
Бэкенд — T83, контракт `GET /admin/reviews` и существующий
`DELETE /admin/reviews/{id}` в `Task/00-api-contract.md` (заморожен).

Работать только в `wt/t84` (ветка `task/T84-admin-reviews-ui`), база —
`integration/p10`.

## Что сделать

1. Новый API-модуль `frontend/src/api/adminReviews.ts`: типы
   `AdminReview`, `AdminReviewsResponse`, `AdminReviewsQuery` и функции
   `listAdminReviews(query)`, `deleteAdminReview(id)`. Используйте
   `apiFetch`/`buildQuery` из `client.ts`; не правьте `client.ts`.
2. Новая страница `frontend/src/pages/AdminReviews.tsx`:
   - заголовок/подзаголовок, ссылка «Назад в админку» (`/admin`);
   - фильтры: поиск, курс (селект из `getCategories`? нет — используйте
     `listCourses` с `limit` или endpoint админ-курсов; не добавляйте новых
     запросов, если можно переиспользовать существующий `listAdminCourses`),
     оценка 1–5, даты «с/по», кнопки «Применить»/«Сбросить»;
   - таблица: курс (ссылка на `/courses/:slug`), ученик (email/имя), оценка,
     текст, дата, действие «Удалить»;
   - подтверждение удаления, состояние pending, toast/уведомление об успехе,
     empty state, loading/error, пагинация (`admin.showing/of/prev/next`);
   - URL-состояние фильтров и страницы (как в других админ-страницах).
3. Новый CSS `frontend/src/styles/adminReviews.css` в стиле
   `adminUsers.css`/`adminEnrollments.css`; адаптивность и доступность
   (label для фильтров, `aria-live` для ошибок/успеха).
4. Роут `/admin/reviews` в `frontend/src/app/router.tsx` с
   `RequireRole roles={['admin']}` и lazy import, SEO-заголовок.
5. Используйте только замороженные ключи `p10.reviews.*` из
   `frontend/src/i18n/ru.json` (см. контракт). Если ключа не хватает —
   не правьте `ru.json`, укажите это в отчёте. Переиспользуйте
   `common.*`, `admin.loading`, `admin.total`, `admin.showing`, `admin.of`,
   `admin.prev`, `admin.next`, `admin.filters`, `admin.resetFilters`,
   `admin.periodFrom`, `admin.periodTo`.
6. Тест API-клиента: `frontend/tests/api-contract/admin-reviews.test.mjs` по
   образцу существующих тестов (проверка query string, типов ответа,
   delete-запроса).

## Разрешённые файлы

- `frontend/src/api/adminReviews.ts` (новый)
- `frontend/src/pages/AdminReviews.tsx` (новый)
- `frontend/src/styles/adminReviews.css` (новый)
- `frontend/src/app/router.tsx`
- `frontend/tests/api-contract/admin-reviews.test.mjs` (новый)
- `frontend/tests/T84-admin-reviews-ui.md` (отчёт)

## Запрещено

- `Task/**`, `specs/**`, `backend/**`, `frontend/src/i18n/ru.json`,
  `frontend/src/api/client.ts`, `frontend/src/api/types.ts` (по возможности),
  `docker-compose*`, `qa-*.ps1`, `.github/**`, `deploy/**`.
- Менять существующие страницы админки, кроме добавления ссылки в nav
  (ссылку добавит T88; в этой задаче не трогайте `Admin.tsx`).
- `git push`, `git merge`, смена ветки, работа вне `wt/t84`.

## Проверка

```
cd frontend
npm run build
npm test
```

Для `npm test` в этом репозитории используйте рабочий вариант из
`package.json`; если `node --test tests/api-contract/` падает на Windows,
запустите `node --test tests/api-contract/*.test.mjs` и укажите это в отчёте.

## Отчёт

`frontend/tests/T84-admin-reviews-ui.md`: что сделано, проверка, отклонения,
открытые вопросы. Коммит явными путями.
