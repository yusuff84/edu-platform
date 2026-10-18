# T32 — отчёт: блокировка учитывается при входе через Яндекс

**Ветка:** `task/T32-oauth-blocked` · **Контракт:** `Task/00-api-contract.md` → «Блокировка и вход через Яндекс (T32)»
**Примечание:** сессия агента T32 умерла на инфраструктурной ошибке провайдера (HTTP 500) после написания кода и тестов. Правки и прогон зафиксировал координатор; код — авторства агента.

## Что изменено

| Файл | Что |
|---|---|
| `internal/handler/auth_yandex.go` | в `YandexAuthCallback` после поиска/создания пользователя и **до** выдачи cookie: `if u.Status == repo.UserBlocked` → `403` с тем же текстом, что на парольном входе; сессия не создаётся |
| `internal/handler/auth.go` | общий помощник/текст ответа для заблокированного пользователя (используется и парольным входом, и OAuth-callback), поведение прежнее |
| `internal/handler/auth_yandex_test.go` | новые проверки поверх существующего подставного Яндекса (httptest) |

## Что проверяет тест

1. Пользователь из подставного профиля существует и **заблокирован** → callback отвечает `403`, `Set-Cookie` в ответе нет, `GET /auth/me` → `401`.
2. Тот же пользователь разблокирован → `302` и cookie, `GET /auth/me` → `200`.
3. Новый пользователь → `302` + cookie, в сторе — `active`.

## Прогон (Docker, `golang:1.24-alpine`)

```
ok  edu-platform/backend/cmd/api            12.111s
ok  edu-platform/backend/internal/auth       0.335s
ok  edu-platform/backend/internal/handler    8.247s
ok  edu-platform/backend/internal/payments   0.335s
ok  edu-platform/backend/internal/repo       0.108s
ok  edu-platform/backend/tests/contract     40.420s
T32_TESTS_EXIT=0
```

## Не проверено

- **Живой Яндекс**: на стенде нет ключей OAuth (эндпоинт `start` отдаёт `503`), реальный редирект и настоящий профиль не проходились — проверка только на подставном провайдере.
- Postgres-путь чтения статуса (`GetUserByEmail` возвращает `status` — строка SQL проверена чтением, автотест живого Postgres — в T34).
- Поведение при одновременном входе и блокировке (гонка) не воспроизводилось.
