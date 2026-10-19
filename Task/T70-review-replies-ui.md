# T70 — Фронтенд: отзывы — гистограмма, фильтр по оценке, ответы учителя

**Волна:** P8  **Ветка:** `task/T70-review-replies-ui`  **Worktree:** `wt/t70`

## Суть

Блок отзывов показывает плоский список без разбивки оценок, преподаватель не
может ответить. Нужны: гистограмма 5→1 с фильтром по клику, отображение ответа
учителя и форма ответа для владельца курса/админа. API — по разделу
«Эндпоинты волны P8» в `Task/00-api-contract.md` (**заморожен**); бэкенд T69
идёт параллельно.

Работать только в своём worktree `wt/t70` (ветка `task/T70-review-replies-ui`).

## Что сделать

1. `frontend/src/api/types.ts`: в `Review` аддитивные `teacherReply?: string`,
   `teacherReplyAt?: string | null`; типы `ReviewBreakdown`
   (`Record<'1'|'2'|'3'|'4'|'5', number>`) и `CourseReviewsPage`
   (`items: Review[], total, page, limit, avgRating, ratingsCount, breakdown`).
2. `frontend/src/api/client.ts`: новая `getCourseReviews(courseId, params?)` —
   возвращает конверт целиком (`rating` необязателен); `replyToReview(courseId,
   reviewId, body)` → `Review`; `deleteReviewReply(courseId, reviewId)`.
   Существующую `listReviews` не ломать.
3. `frontend/src/components/Reviews.tsx`:
   - перейти на `getCourseReviews`; блок статистики: средний балл, всего
     отзывов, гистограмма 5→1 (число и полоса-процент от максимума);
   - клик по строке гистограммы — фильтр по этой оценке (запрос с `rating`),
     повторный клик или «Все оценки» — сброс; счётчики гистограммы всегда по
     всем отзывам; при активном фильтре список показывает только выбранную
     оценку; пусто — `p8.reviews.emptyRating`;
   - под отзывом с `teacherReply` — плашка «Преподаватель» + текст + дата;
   - если пользователь — владелец курса или admin: кнопка «Ответить»/
     «Изменить ответ» → textarea (3..2000, счётчик символов), «Сохранить»,
     «Отмена», «Удалить ответ» (с подтверждением); после успеха — инвалидация
     запросов отзывов/курса и сообщение `p8.reviews.replySaved`;
   - ошибки сервера показывать текстом `ApiError.message`;
   - не ломать форму создания отзыва (rating/body, дубликат 409) и текущие
     состояния «нужна запись на курс».
4. Признак владельца: `Reviews` получает новый необязательный проп
   `canReply?: boolean`; `Course.tsx` передаёт `isAuthor || user?.role ===
   'admin'` (в Course.tsx уже есть `isAuthor`). Не ломать существующие вызовы.
5. Стили — новый `frontend/src/styles/reviews-extra.css` (импорт в
   `Reviews.tsx`); `catalog.css` и чужие стили не править. Использовать
   переменные/классы дизайн-системы.
6. i18n — только готовые ключи `p8.reviews.*` (+ существующие `reviews.*`);
   словарь не править, не хватает — в отчёт.
7. Тесты: новый `frontend/tests/api-contract/review-replies.test.mjs` по образцу
   существующих (мок-сервер 127.0.0.1:8799): `getCourseReviews` с `rating` и
   без, парсинг `breakdown`, PUT/DELETE ответа, ошибки 400/403/404.

## Разрешённые файлы

- `frontend/src/components/Reviews.tsx`
- `frontend/src/pages/Course.tsx` (только передача пропа `canReply`)
- `frontend/src/api/client.ts`, `frontend/src/api/types.ts`
- `frontend/src/styles/reviews-extra.css` (новый)
- `frontend/tests/api-contract/review-replies.test.mjs` (новый)
- `frontend/tests/T70-review-replies-ui.md` (отчёт)

## Запрещено

- `backend/**`, `Task/**`, `specs/**`, `frontend/src/i18n/ru.json`,
  `catalog.css`, чужие страницы/компоненты/стили, `qa-*.ps1`,
  `docker-compose*`.
- Менять сигнатуры существующих клиентских функций, ломая обратную
  совместимость.
- `git push`, `git merge`, смена ветки, работа вне `wt/t70`.

## Проверка

Из папки `frontend/`:

```
npm run build
npm test
```

Сборка и все тесты (старые + новые) зелёные; вывод приложите к отчёту.

## Отчёт

`frontend/tests/T70-review-replies-ui.md`: «Что сделано», «Проверка» (команды и
результаты), «Отклонения от постановки», «Открытые вопросы». Закоммитьте отчёт
и код в ветку `task/T70-review-replies-ui`.
