# T42 — CORS-префлайт: маршрутные Allow-Methods (backend)

**Ветка:** `task/T42-cors-methods` · **Волна:** P5 · **Worktree:** `wt/t42`
**Контракт:** `Task/00-api-contract.md` → «Эндпоинты волны P5» → «Префлайт CORS: маршрутные методы (T42)».

## Что изменено

1. `backend/internal/handler/middleware.go`:
   - `NewRouter` теперь вызывает `r.Use(app.cors(r))`; сигнатура —
     `func (a *App) cors(mux *chi.Mux) func(http.Handler) http.Handler`
     (`Match` есть только у `*chi.Mux`, не у интерфейса `chi.Router`);
   - для `OPTIONS` с разрешённого `Origin` перебираются
     `GET, POST, PUT, PATCH, DELETE`; каждый проверяется
     `mux.Match(chi.NewRouteContext(), m, r.URL.Path)` — chi v5 сам обходит
     вложенные подроутеры (`Route("/api/v1")`, `/teacher`, `/admin`);
     `Access-Control-Allow-Methods` = найденные в порядке перебора + `OPTIONS`;
     если не найден ни один — заголовок не ставится;
   - `Allow-Origin` / `Allow-Credentials` / `Allow-Headers` / `Max-Age` /
     `Vary: Origin`, статус `204` и поведение не-OPTIONS запросов — без изменений.
     Сохранён и прежний нюанс: у неизвестного пути с разрешённого Origin
     `Allow-Headers`/`Max-Age` остаются, пропадает только `Allow-Methods`.
2. Новый `backend/internal/handler/middleware_cors_test.go` (package handler,
   роутер через `newTestRouter(devConfig())` из `security_test.go`):
   - `OPTIONS /api/v1/courses` → `204`, `Allow-Methods` ровно `GET, OPTIONS`;
   - `OPTIONS /api/v1/teacher/courses` → есть `GET`, `POST`, `OPTIONS`;
   - `OPTIONS /api/v1/auth/login` → есть `POST` и нет `GET`;
   - `OPTIONS /api/v1/нет-такого-пути` → заголовка `Allow-Methods` нет;
   - чужой `Origin` → нет `Allow-Origin`/`Allow-Methods`/`Allow-Credentials`;
   - не-OPTIONS `GET /api/v1/courses` → `200` без CORS-заголовков (регресс-страховка).
3. `backend/tests/contract/payments_hardening_test.go` (T35, preflight-проверка —
   владелец в P5 — T42): путь проверки переведён с GET-маршрута
   `/api/v1/auth/me` на реальный POST-маршрут `/api/v1/auth/login` (иначе после
   T42 assert «есть POST в Allow-Methods» противоречил бы контракту), добавлена
   проверка «`GET` у `/auth/login` в списке быть не должно». Остальные
   preflight-проверки T35 не тронуты.

## Заголовки: до / после

### До (текущий живой стенд :8082, старый код — curl перед правкой)

```text
> curl.exe -s -i -X OPTIONS -H "Origin: http://localhost:3001" -H "Access-Control-Request-Method: POST" http://localhost:8082/api/v1/teacher/courses

HTTP/1.1 204 No Content
Access-Control-Allow-Credentials: true
Access-Control-Allow-Headers: Content-Type, Authorization
Access-Control-Allow-Methods: GET, POST, PUT, PATCH, DELETE, OPTIONS
Access-Control-Allow-Origin: http://localhost:3001
Access-Control-Max-Age: 86400
Vary: Origin
```

То же самое отдавали `/api/v1/courses`, `/api/v1/auth/login` и даже
`/api/v1/no-such-path` — жёсткая константа для всех путей.

### После (новый код, эфемерный контейнер на :18082 — отдельный от стенда)

```text
> curl.exe -s -i -X OPTIONS -H "Origin: http://localhost:3001" -H "Access-Control-Request-Method: POST" http://localhost:18082/api/v1/teacher/courses

HTTP/1.1 204 No Content
Access-Control-Allow-Credentials: true
Access-Control-Allow-Headers: Content-Type, Authorization
Access-Control-Allow-Methods: GET, POST, OPTIONS
Access-Control-Allow-Origin: http://localhost:3001
Access-Control-Max-Age: 86400
Vary: Origin

> ... http://localhost:18082/api/v1/courses
Access-Control-Allow-Methods: GET, OPTIONS

> ... http://localhost:18082/api/v1/auth/login
Access-Control-Allow-Methods: POST, OPTIONS

> ... http://localhost:18082/api/v1/no-such-path
(204; Allow-Origin/Credentials/Headers/Max-Age/Vary есть, Allow-Methods отсутствует)

> curl.exe -s -i -X OPTIONS -H "Origin: https://evil.example" http://localhost:18082/api/v1/courses
HTTP/1.1 204 No Content
Date: ...
(никаких Access-Control-*)
```

**Важно (честно):** общий живой стенд `:8082` не пересобирался и не
останавливался — на нём до сих пор старый код и старые заголовки (блок «До»).
Новые заголовки появятся на стенде только после пересборки/перезапуска `api`
координатором. «После» подтверждено на отдельном эфемерном контейнере
(`docker run`, имя `t42-verify`, порт `18082` → внутри 8080, in-memory store,
`ALLOWED_ORIGINS=http://localhost:3001`), поднятом из этого же worktree;
контейнер удалён, стенд не затронут.

## Выполненные проверки (фактические итоги)

1. Команда из постановки (Docker, из корня worktree) — **exit 0**:

   ```text
   go build ./... && go vet ./... && go test ./... -count=1
   ok  edu-platform/backend/cmd/api
   ok  edu-platform/backend/internal/auth
   ok  edu-platform/backend/internal/handler
   ok  edu-platform/backend/internal/payments
   ok  edu-platform/backend/internal/repo
   ok  edu-platform/backend/tests/contract
   ok  edu-platform/backend/tests/postgres
   ```

2. Целевые тесты `-v`: `TestCORSPreflightAllowMethodsByRoute`,
   `TestCORSPreflightForeignOrigin`, `TestCORSNonOptionsUnchanged` и
   `TestPaymentsHardeningPreflightCORS` — все PASS.

3. Мутационная проверка чувствительности: с временно возвращённой старой
   константой (`if true` вместо `mux.Match`) тест падает именно на старом
   значении:
   `Allow-Methods="GET, POST, PUT, PATCH, DELETE, OPTIONS", ждали "GET, OPTIONS"`.
   Мутация откатана, финальный прогон — зелёный.

4. `curl` до/после — см. раздел выше; на живом стенде `GET /healthz` → `200`
   (стенд не тронут).

## Непроверенное и риски (честно)

- **Живой стенд не пересобран**: поведение «после» проверено на отдельном
  эфемерном запуске того же кода, а не на общем стенде `:8082`. После
  пересборки координатором стоит повторить curl из постановки.
- **Браузерные префлайты фронтенда (порт 3001)** не гонялись: проверены только
  OPTIONS через httptest и curl. Заголовок `Access-Control-Request-Method` не
  анализируется — как и раньше, ответ строится по методам **пути** (контракт
  T42 это и требует).
- **Завершающий слэш**: `mux.Match` различает `/api/v1/courses` и
  `/api/v1/courses/` — для пути со слэшем на конце `Allow-Methods` не будет.
  Это осознанное следствие контракта («неизвестный путь — заголовка нет»);
  все реальные API-пути зарегистрированы без конечного слэша.
- **Postgres-контур**: правка не зависит от хранилища; env-gated тесты
  `tests/postgres` в этом прогоне прошли без БД (как и раньше, скипом).
- `gofmt -l` в Linux-контейнере перечисляет почти все файлы репозитория
  из-за CRLF рабочего каталога Windows (`core.autocrlf=true`) — это не
  регрессия T42; новый файл gofmt-совместим (в списке его нет).
