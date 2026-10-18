# T42 — CORS-префлайт: маршрутные Allow-Methods (backend)

**Волна:** P5 · **Ветка:** `task/T42-cors-methods` · **Worktree:** `wt/t42`
**Контракт:** `Task/00-api-contract.md`, раздел «Эндпоинты волны P5» → «Префлайт CORS: маршрутные методы (T42)». Контракт заморожен.

## Зачем

Открытая находка T20: `middleware.go` жёстко проставляет
`Access-Control-Allow-Methods: GET, POST, PUT, PATCH, DELETE, OPTIONS` для всех
маршрутов — даже там, где этих методов нет. Роутер — chi v5; реальные методы
пути вычисляются через `Match`, без ручного дублирования таблицы маршрутов.

## Что сделать

1. `backend/internal/handler/middleware.go`:
   - `cors` получает роутер: в `NewRouter` — `r.Use(app.cors(r))`; сигнатура
     `func (a *App) cors(mux *chi.Mux) func(http.Handler) http.Handler`
     (`Match` есть только у `*chi.Mux`, не у интерфейса `chi.Router`);
   - OPTIONS + разрешённый Origin: перебрать `GET, POST, PUT, PATCH, DELETE`,
     для каждого — `mux.Match(chi.NewRouteContext(), m, r.URL.Path)`;
     `Access-Control-Allow-Methods` = найденные (в порядке перебора) + `OPTIONS`;
     если не найден ни один — заголовок не ставить;
   - `Allow-Origin`/`Allow-Credentials`/`Allow-Headers`/`Max-Age`/`Vary: Origin`,
     статус `204` и поведение не-OPTIONS запросов — без изменений.
2. Новый `backend/internal/handler/middleware_cors_test.go` (приложение собирать
   как в соседних тестах handler — найди хелпер):
   - `OPTIONS /api/v1/courses` с разрешённого Origin → `204`,
     `Allow-Methods` = `GET, OPTIONS`;
   - `OPTIONS /api/v1/teacher/courses` → содержит `GET`, `POST`, `OPTIONS`;
   - `OPTIONS /api/v1/auth/login` → есть `POST` и **нет** `GET`;
   - `OPTIONS /api/v1/нет-такого-пути` → заголовка `Allow-Methods` нет;
   - чужой Origin → нет `Allow-Origin`.
3. `backend/tests/contract/payments_hardening_test.go` (T35, проверка preflight):
   убедиться, что путь теста — реальный POST-маршрут, прогон зелёный; при
   необходимости минимально уточнить именно preflight-проверку (владелец в P5 — T42).

## Правила волны

- Работать только в своём worktree; коммит — в свою ветку `task/T42-cors-methods`.
- Править только: `backend/internal/handler/middleware.go`,
  `backend/internal/handler/middleware_cors_test.go` (новый),
  `backend/tests/contract/payments_hardening_test.go` (только preflight-проверка),
  отчёт `backend/tests/T42-cors-methods.md`.
- `frontend/**`, `Task/**`, `specs/**` — не трогать. Новых зависимостей нет.

## Проверка (Docker, из корня worktree)

```powershell
docker run --rm -v "${PWD}\backend:/src" -v p5-gomod:/go/pkg/mod -w /src -e GOFLAGS=-mod=mod golang:1.24-alpine sh -c "go build ./... && go vet ./... && go test ./... -count=1"
```

`exit 0` обязателен. На живом стенде (после пересборки api):

```powershell
curl.exe -i -X OPTIONS -H "Origin: http://localhost:3001" -H "Access-Control-Request-Method: POST" http://localhost:8082/api/v1/teacher/courses
```

→ `204`, в `Allow-Methods` есть `POST`; вывод приложить в отчёт.

## Отчёт

`backend/tests/T42-cors-methods.md`: примеры заголовков до/после, что изменено,
честный список непроверенного.
