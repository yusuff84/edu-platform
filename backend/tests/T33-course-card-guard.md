# T33 — отчёт: правка карточки курса заморожена в архиве

**Ветка:** `task/T33-course-card-guard` · **Контракт:** `Task/00-api-contract.md` → «Правка карточки курса (T33)»
**Примечание:** сессия агента T33 умерла на инфраструктурной ошибке провайдера (HTTP 500) после написания кода и тестов. Прогон и фиксацию сделал координатор; код — авторства агента.

## Что изменено

| Файл | Что |
|---|---|
| `internal/handler/teacher_edit.go` | в `TeacherUpdateCourse` сразу после `ownCourse` (до валидации тела): `archived` → `409 «Курс в архиве: карточка заморожена»`; в комментарии функции зафиксировано осознанное исключение: в `draft/review/published` карточка правится без повторной модерации |
| `internal/handler/teacher_edit_test.go` | обновлены проверки PATCH под новый контракт + случай `archived` |
| `tests/contract/teacher_test.go` | обновлена проверка, фиксировавшая прежнее «всеядное» поведение |
| `tests/contract/teacher_card_status_test.go` | новый файл: матрица статусов |

## Что проверяется

- `draft` → `200`, поля обновились; `review` → `200`; `published` → `200`.
- `archived` → `409`, курс в сторе не изменился (`title`/`priceCents`).
- `archived` + пустое тело → `409` (guard раньше валидации).
- Регрессии: пустое название → `400`, отрицательная цена → `400`, занятый slug → `400`.

## Прогон (Docker, `golang:1.24-alpine`)

```
ok  edu-platform/backend/cmd/api            13.359s
ok  edu-platform/backend/internal/auth       0.384s
ok  edu-platform/backend/internal/handler    9.760s
ok  edu-platform/backend/internal/payments   0.689s
ok  edu-platform/backend/internal/repo       0.139s
ok  edu-platform/backend/tests/contract     38.946s
T33_TESTS_EXIT=0
```

## Не проверено

- Живой стенд: проверка `409` через UI конструктора — после сведения ветки (в интерфейсе кнопка правки у архивного курса скрывается задачей T36).
- Админский путь (`PATCH` чужого курса): тест использует владельца; поведение админа идентично по коду (`ownCourse` пропускает админа), отдельной проверки нет.
