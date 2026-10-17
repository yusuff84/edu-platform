# T19 — CRUD конструктора: редактирование, удаление, порядок

**Ключ сессии:** `T19-constructor-crud` · **Волна:** B (после T16) · **Слой:** full

Закрывает бэклог T06: сейчас конструктор умеет только **создавать** курсы,
модули и уроки. Переименование, удаление, перестановка и правка урока после
создания отсутствуют — и T06 честно не рисовал под это кнопки.

Запускается **после смержения T16**, потому что оба трогают `repo` и
`backend/internal/handler`.

## Владение файлами (жёсткая граница)

```
backend/internal/handler/teacher_edit.go          — создан T13, расширяется
backend/internal/handler/teacher_crud.go          — создать (новые методы)
backend/internal/repo/store_iface.go
backend/internal/repo/store.go
backend/internal/repo/postgres_store.go
backend/internal/repo/models.go                   — только если реально нужно новое поле
frontend/src/pages/Constructor.tsx
frontend/src/styles/constructor.css
```

**Не трогать:** `frontend/**` кроме `Constructor.tsx`/`constructor.css`
(T17/T18), `backend/tests/contract/**` (T20), `middleware.go` — регистрацию
веди **выносным методом** `RegisterTeacherCRUDRoutes(r chi.Router)` по образцу
`admin_payments.go:20` (абсолютные пути!), подключение — одной строкой на уровне
`/api/v1/teacher` в `middleware.go`.

> Путь монтирования критичен: T07 ошибся бы, смонтировав абсолютные пути внутри
> `r.Route("/admin", ...)`. Повтори проверку сам: посмотри, куда уже смонтирован
> `teacher_edit.go`, и поступи аналогично.

---

## Эндпоинты (отсортировано по приоритету)

Реализуй **минимум первые четыре** — они разблокируют основные сценарии.
Остальное — по остаточному принципу, но **не рисуй кнопки без API**.

| Приоритет | Эндпоинт | Тело | Ответ |
|---|---|---|---|
| высокий | `PATCH /teacher/courses/{id}` | `{title?, description?, priceCents?, slug?}` | Course |
| высокий | `PATCH /teacher/courses/{id}/modules/{moduleId}` | `{title}` | Module |
| высокий | `PATCH /teacher/courses/{id}/lessons/{lessonId}` | тело как в POST (по `kind`) | Lesson |
| высокий | `DELETE /teacher/courses/{id}/modules/{moduleId}` | — | 204 / 409 при непустых уроках |
| средний | `DELETE /teacher/courses/{id}/lessons/{lessonId}` | — | 204 |
| средний | `POST /teacher/courses/{id}/modules/reorder` | `{items:[{id,position}]}` | 204 |
| средний | `POST /teacher/courses/{id}/lessons/reorder` | `{items:[{id,position}]}` | 204 |
| низкий | `DELETE /teacher/courses/{id}` | — | 204 |
| средний | обогатить `GET /teacher/courses` | — | + `modulesCount`, `lessonsCount` (сейчас N+1 на фронте) |

## Обязательные правила

1. **Права:** только владелец курса **или** админ. Роль проверяй как в
   существующих teacher-хендлерах.
2. **Статус:** редактирование контента — только в `draft`. Правка карточки
   (`PATCH /courses/{id}`) допустима и для `review`/`published`, если это
   не влияет на модерацию — реши и обоснуй.
3. **`DisallowUnknownFields`** включён: неизвестное поле = 400. Собирай тело
   явно, как делает `createLesson` (T01).
4. **Оба стора** (`store.go` и `postgres_store.go`) + `store_iface.go`
   с `var _ StoreAPI = ...` — иначе не соберётся.
5. **Ошибки БД пробрасывай**, не глотай (`postgres_store.go` — историческая
   привычка `return nil, false`; для нового не применяй).
6. **Валидация:** пустой title/description → 400, `position` вне диапазона → 400,
   дубликат slug → 409.
7. **Идемпотентность порядка:** reorder принимает полный список позиций,
   применять одной транзакцией.

## Фронтенд (`Constructor.tsx`)

- Подключи кнопки **только** для реально реализованных эндпоинтов.
- Остальное — честная пометка `teach.apiLimitNote` (ключ уже есть).
- Обнови бэклог в отчёте: что осталось.
- Состояния: успех/ошибка/403/404/409, подтверждения на удаление
  (`teach.confirmDelete`, `teach.confirmDeleteModule` — ключи есть).
- Сохрани query-ключи `['teaching']`, `['teach-course', id]` и инвалидацию.

## Проверка

```
docker run --rm -v "C:\Users\96352\all_project\eduPlatform\wt-T19-constructor-crud\backend:/src" -v gomodcache:/go/pkg/mod -w /src -e GOFLAGS=-mod=mod golang:1.24-alpine sh -c "go build ./... && go vet ./... && go test ./... -count=1 && echo ALL_OK"
```
```
cd ...wt-T19-constructor-crud\frontend && npm ci && npx tsc --noEmit && npm run build
```

**Напиши тесты** на каждый новый эндпоинт (права, валидация, 409, порядок) —
в **новом** файле `backend/internal/handler/teacher_crud_test.go`, существующие
тесты не меняй (их правит T20).

## Отчёт (обязателен)
1. Таблица реализованных эндпоинтов: метод, путь, права, ошибки.
2. Сигнатуры новых методов стора.
3. Точная точка подключения в `middleware.go` (file:line) для T20/T13-шаблона.
4. Что осталось из бэклога T06.
5. Какие ключи `ru.json` потребовались, но отсутствуют.
