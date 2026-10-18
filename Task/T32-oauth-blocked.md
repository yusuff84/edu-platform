# T32 — Блокировка учитывается при входе через Яндекс (backend)

**Волна:** P4 · **Ветка:** `task/T32-oauth-blocked` · **Worktree:** `wt/t32`
**Контракт:** `Task/00-api-contract.md`, раздел «Эндпоинты волны P4» → «Блокировка и вход через Яндекс (T32)». Контракт заморожен.

## Зачем

`requireAuth` (`backend/internal/handler/middleware.go:310`) и парольный вход (`auth.go:113,139`)
блокировку учитывают, а OAuth-callback — нет. Заблокированный пользователь входит через Яндекс
и спокойно работает до истечения/сброса cookie; это дыра в T24.

## Что сделать

1. В `YandexAuthCallback` (`backend/internal/handler/auth_yandex.go`) после того, как пользователь
   найден или создан — **до** постановки cookie-сессии — проверить статус:
   `if u.Status == repo.UserBlocked { … 403 … }`.
   Текст берётся **буквально тот же**, что на парольном входе заблокированного пользователя
   (`auth.go`): не придумывать свой. Если эти строки дублируются — вынести общий помощник
   в `auth.go` и переиспользовать (текст и код ответа не менять).
2. Cookie не ставится: `Set-Cookie` в ответе быть не должно, `auth/me` с этим клиентом — `401`.
3. Новый пользователь через Яндекс создаётся в статусе `active` (проверить `createOAuthUser`,
   при необходимости — выставить явно).
4. Проверить, что ветка «пользователь существует и active» и «новый пользователь» не сломаны.

## Тесты

Файл `backend/internal/handler/auth_yandex_test.go` (в зоне T32 — только он):
добавить `TestYandexAuthCallbackRejectsBlockedUser` по образцу
`TestYandexAuthCallbackFullFlow` (там уже есть подставной Яндекс через httptest):

1. пользователь с email из подставного профиля существует и **заблокирован**
   (`app.Store.UpdateUserStatus(...)` или что доступно в сторе) → callback возвращает `403`,
   в ответе нет `Set-Cookie`, `GET /auth/me` → `401`;
2. тот же пользователь разблокирован → callback возвращает `302` и ставит cookie,
   `GET /auth/me` → `200` (негативный и положительный случай в одном тесте или двумя — как удобнее);
3. новый (несуществующий) пользователь → `302` + cookie, в сторе он `active`.

Если для проверки нужен доступ к сессии — использовать существующие помощники теста,
новых seam-ов в продакшн-коде не добавлять (кроме общего помощника из п.1).

## Ограничения

- Править только: `backend/internal/handler/auth_yandex.go`,
  `backend/internal/handler/auth.go` (только вынесение общего помощника/текста, если понадобится),
  `backend/internal/handler/auth_yandex_test.go`, отчёт `backend/tests/T32-oauth-blocked.md`.
- `Task/**`, `frontend/**`, `specs/**` — не трогать. Новых зависимостей нет. Тексты по-русски.

## Проверка (Docker)

```powershell
docker run --rm -v "C:\Users\96352\all_project\eduPlatform\edu-platform\backend:/src" -v gomodcache:/go/pkg/mod -w /src -e GOFLAGS=-mod=mod golang:1.24-alpine sh -c "go build ./... && go vet ./... && go test ./... -count=1"
```

## Отчёт

`backend/tests/T32-oauth-blocked.md`: что изменено, точный текст ответа `403`, вывод теста,
честный список непроверенного.
