# T74 — Фронтенд: избранное (сердечко на карточке и страница)

**Волна:** P9  **Ветка:** `task/T74-favorites-ui`  **Worktree:** `wt/t74`

## Суть

Нужно сохранять курсы в избранное с карточки каталога и смотреть отдельный
список. API — по разделу «Волна P9» в `Task/00-api-contract.md`
(**заморожен**); бэкенд T73 идёт параллельно.

Работать только в своём worktree `wt/t74` (ветка `task/T74-favorites-ui`).

## Что сделать

1. `frontend/src/api/client.ts`: `addFavorite(courseId)`,
   `removeFavorite(courseId)`, `myFavorites()` → `CourseItem[]` (конверт
   `{items,total}`); обратная совместимость.
2. `frontend/src/components/CourseCard.tsx`: кнопка-сердечко в углу карточки
   (aria-label «В избранное»/«Убрать из избранного», `aria-pressed`):
   - вошедшему — toggle через мутации, оптимистичное состояние или инвалидация
     `['favorites']`; клик не должен открывать курс (`stopPropagation`);
   - гостю — переход на `/login` (или подсказка `p9.fav.loginHint`), без
     запроса;
   - не ломать обложку, бейдж категории и существующую вёрстку.
3. Новая страница `frontend/src/pages/Favorites.tsx`: список избранных курсов
   (сетка карточек), пустое состояние с CTA в каталог, состояния загрузки.
4. `frontend/src/app/router.tsx`: маршрут `/me/favorites` под `RequireAuth` и
   пункт «Избранное» в навигации для вошедших (рядом с «Мои курсы»), не ломая
   гостевую навигацию из P7.
5. `frontend/src/pages/MyCourses.tsx`: кнопка/ссылка «Избранное» рядом с
   заголовком (если уместно по вёрстке).
6. Стили — новый `frontend/src/styles/favorites.css` (импорт в странице и
   карточке), на переменных дизайн-системы. Чужие css не править.
7. i18n — только готовые ключи `p9.fav.*`; словарь не править.
8. Тесты: новый `frontend/tests/api-contract/favorites.test.mjs`: PUT/DELETE,
   список, ошибки 401/404, отсутствие тела у DELETE.

## Разрешённые файлы

- `frontend/src/components/CourseCard.tsx`
- `frontend/src/pages/Favorites.tsx` (новый)
- `frontend/src/pages/MyCourses.tsx`
- `frontend/src/app/router.tsx`
- `frontend/src/api/client.ts`, `frontend/src/api/types.ts`
- `frontend/src/styles/favorites.css` (новый)
- `frontend/tests/api-contract/favorites.test.mjs` (новый)
- `frontend/tests/T74-favorites-ui.md` (отчёт)

## Запрещено

- `backend/**`, `Task/**`, `specs/**`, `frontend/src/i18n/ru.json`, чужие
  страницы/компоненты/стили, `qa-*.ps1`, `docker-compose*`.
- Менять существующие маршруты и ломать сигнатуры клиентских функций.
- `git push`, `git merge`, смена ветки, работа вне `wt/t74`.

## Проверка

Из папки `frontend/`:

```
npm run build
npm test
```

Сборка и все тесты (старые + новые) зелёные. При таймаутах порта 8799 —
подождите 30–60 секунд и повторите `npm test`.

## Отчёт

`frontend/tests/T74-favorites-ui.md`: «Что сделано», «Проверка», «Отклонения»,
«Открытые вопросы». Закоммитьте отчёт и код в ветку `task/T74-favorites-ui`.
