# T64 — Фронтенд: промокоды (конструктор курса и чекаут)

**Волна:** P8  **Ветка:** `task/T64-promo-codes-ui`  **Worktree:** `wt/t64`

## Суть

Учитель управляет промокодами в конструкторе курса, студент применяет код на
странице курса и видит цену со скидкой. API — по разделу «Эндпоинты волны P8»
в `Task/00-api-contract.md` (**заморожен**); бэкенд делает T63 параллельно,
ориентируйтесь на контракт.

Работать только в своём worktree `wt/t64` (ветка `task/T64-promo-codes-ui`).

## Что сделать

1. `frontend/src/api/types.ts` — аддитивные типы: `PromoCode`
   (`id, courseId, code, discountPct, maxUses, usedCount, expiresAt: string|null,
   active, createdAt`) и `PromoValidation` (`code, discountPct, priceCents,
   finalPriceCents`).
2. `frontend/src/api/client.ts`:
   - `createPromoCode(courseId, payload)`, `listPromoCodes(courseId)`,
     `deactivatePromoCode(courseId, promoId)`, `validatePromoCode(courseId, code)`;
   - `enroll(courseId, provider, promoCode?)` — необязательный третий параметр,
     обратная совместимость обязательна.
3. Конструктор (`frontend/src/pages/Constructor.tsx`): секция «Промокоды» для
   существующего курса (не для архива):
   - форма: код, скидка % (1–90), лимит использований (0 — без лимита),
     «действует до» (`<input type="date">`, пусто — бессрочно; отправлять
     `YYYY-MM-DDT23:59:59Z`);
   - список: код, скидка, «использовано N / лимит» (или «Без лимита»), срок,
     статус (Активен / Отключён / Истёк — вычислять на клиенте), кнопка
     «Отключить» с подтверждением;
   - ошибки сервера показывать текстом `ApiError.message`; клиентская
     предвалидация — по ключам `p8.promo.*Invalid`;
   - обновление списка после создания/отключения (react-query invalidate).
4. Чекаут на странице курса (`frontend/src/pages/Course.tsx`): для вошедшего
   студента на платном курсе без активной записи:
   - тоггл «У меня есть промокод» → поле + «Применить»;
   - успех: показать полную цену зачёркнутой и итоговую, кнопка покупки —
     «Купить за <итог>»; возможность убрать код;
   - покупка передаёт `promoCode` в `enroll`; ошибки — текстом сервера;
   - бесплатный курс — прежнее поведение без промо-блока.
5. Стили — новый файл `frontend/src/styles/promo.css` (импорт в `Constructor.tsx`
   и `Course.tsx`). Используйте существующие переменные/классы дизайн-системы;
   чужие css-файлы не править.
6. i18n — только готовые ключи `p8.promo.*` и `p8.checkout.*` из
   `frontend/src/i18n/ru.json`. Словарь не править; не хватает ключа — в отчёт.
7. Тесты: новый `frontend/tests/api-contract/promo.test.mjs` по образцу
   существующих (мок-сервер 127.0.0.1:8799): запросы/ответы новых функций,
   передача `promoCode` в `enroll`, обработка ошибок.

## Разрешённые файлы

- `frontend/src/api/client.ts`, `frontend/src/api/types.ts`
- `frontend/src/pages/Constructor.tsx`, `frontend/src/pages/Course.tsx`
- `frontend/src/styles/promo.css` (новый)
- `frontend/tests/api-contract/promo.test.mjs` (новый)
- `frontend/tests/T64-promo-codes-ui.md` (отчёт)

## Запрещено

- `backend/**`, `Task/**`, `specs/**`, `frontend/src/i18n/ru.json`,
  `router.tsx`, `Layout.tsx`, чужие страницы/компоненты/стили, `qa-*.ps1`,
  `docker-compose*`.
- Менять сигнатуры существующих клиентских функций, ломая обратную
  совместимость.
- `git push`, `git merge`, смена ветки, работа вне `wt/t64`.

## Проверка

Из папки `frontend/`:

```
npm run build
npm test
```

Сборка и ВСЕ тесты (старые + новые) зелёные; вывод приложите к отчёту.

## Отчёт

`frontend/tests/T64-promo-codes-ui.md`: «Что сделано», «Проверка» (команды и
результаты), «Отклонения от постановки», «Открытые вопросы». Закоммитьте отчёт
и код в ветку `task/T64-promo-codes-ui`.
