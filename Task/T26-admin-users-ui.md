# T26 — UI: супер-админ, раздел «Пользователи»

Волна: P3. Зависимости: контракт T24 (бэкенд пишется параллельно — код брать из
`Task/T24-admin-users.md`, поля не выдумывать). Разводка уже сделана: роут
`/admin/users`, ссылка в навигации админки, заглушка `pages/AdminUsers.tsx`.

## Зачем

В админке нет управления пользователями. Это центральный экран супер-админа:
найти человека по email, выдать роль учителя, заблокировать, удалить мусорный аккаунт.

## Владеет файлами (менять можно только их)

- `frontend/src/pages/AdminUsers.tsx` (заменить заглушку)
- `frontend/src/styles/adminUsers.css` (новый)
- `frontend/src/api/adminUsers.ts` (новый)
- `frontend/tests/T26-admin-users.md` — отчёт о проверке (создать в конце)

`frontend/src/i18n/ru.json` менять **нельзя**: все нужные ключи (`adminUsers.*`,
`common.*`) уже добавлены координатором. Если ключа не хватает — используй русский
литерал и перечисли его в отчёте.

## API-клиент

Транспорт уже экспортирован из `client.ts` — импортируй, не дублируй:

```ts
import { ApiError, apiFetch, buildQuery } from './client';
```

Создай `api/adminUsers.ts`:

```ts
export type UserStatus = 'active' | 'blocked';
export type AdminUserRow = {
  id: string; email: string; role: Role; displayName: string;
  status: UserStatus; createdAt: string;
  coursesCount: number; enrollmentsCount: number;
};
export type AdminUserPage = { items: AdminUserRow[]; total: number; page: number; limit: number };

export function listUsers(params: { q?: string; role?: string; status?: string; page?: number; limit?: number })
  : Promise<AdminUserPage>;                       // GET /admin/users...
export function setUserRole(id: string, role: Role): Promise<AdminUserRow>;      // PATCH
export function setUserStatus(id: string, status: UserStatus): Promise<AdminUserRow>; // PATCH
export function deleteUser(id: string): Promise<void>;                          // DELETE, 204
```

`Role` бери из `./types`. Тела собирай явно (`{ role }`, `{ status }`) — бэкенд
включает `DisallowUnknownFields`, лишнее поле = 400.

## Экран

Таблица (не карточки — это список из сотен строк):

| Колонка | Содержимое |
|---|---|
| Email | `email` |
| Имя | `displayName`; если пусто — `—` |
| Роль | `<select>` (ученик/учитель/администратор) → сразу PATCH |
| Статус | бейдж: активен / заблокирован |
| Регистрация | `formatDate(createdAt)` из `api/client` |
| Курсы | `coursesCount` |
| Записи | `enrollmentsCount` |
| Действия | «Заблокировать»/«Разблокировать», «Удалить» |

Обязательное:

1. **Поиск** по email/имени с debounce ~300 мс, сбрасывает страницу на 1.
2. **Фильтры** роль и статус (select, «Все» = пустое значение), сбрасывают страницу на 1.
3. **Пагинация**: `page`/`limit` (20), кнопки «Назад»/«Вперёд», подпись «Показано N из total».
4. **Мутации** через `useMutation` + `invalidateQueries` по ключу списка.
5. **Ошибки**: текст из `ApiError.message` показывать рядом с элементом/в шапке
   (409 «Нельзя заблокировать последнего администратора» и т.п.). Никаких `alert()`.
6. **Удаление** — подтверждение (кнопка в два шага или `window.confirm`), после
   успеха `adminUsers.removed`.
7. Состояния: загрузка — `adminUsers` скелетон или текст, пусто —
   `adminUsers.empty`, ошибка — `common.error` + `common.retry`.
8. Доступность: `<table>` с `<th scope="col">`, у кнопок-иконок `aria-label`,
   `<label>` у фильтров.

## Стиль

`styles/adminUsers.css`, все классы с префиксом `.au-`. Глобальный `styles.css` и
`styles/admin.css` **не трогать**. Ориентируйся на существующие панели: `.panel`,
`.muted`, кнопки проекта — как в `pages/Admin.tsx`.

## Проверка

```
cd frontend
npx tsc --noEmit
npx vite build
```

- 0 ошибок TypeScript, сборка зелёная.
- Новых зависимостей в `package.json` нет.
- Отчёт `frontend/tests/T26-admin-users.md`: что сделано, какие ключи использованы,
  какие литералы остались, что не проверено (сквозной UI-тест делает координатор).

## Критерии приёмки

- Файл-заглушка полностью заменён рабочим экраном.
- Роут `/admin/users` открывается, ссылка в админке ведёт туда.
- Все строки — через `t()` (или перечислены в отчёте).
- `Constructor.tsx`, `Admin.tsx`, `client.ts`, `router.tsx`, `ru.json` не изменены.
