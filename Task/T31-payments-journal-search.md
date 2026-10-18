# T31 — Журнал платежей: поиск по студенту и период (backend)

**Волна:** P4 · **Ветка:** `task/T31-payments-search` · **Worktree:** `wt/t31`
**Контракт:** `Task/00-api-contract.md`, раздел «Эндпоинты волны P4» → «Поиск и период в журнале платежей (T31)». Контракт заморожен, менять его нельзя.

## Зачем

Админ ищет платёж по человеку, а в журнале нет ни поиска по студенту, ни периода — только
`status` и `courseId`. При живом стенде журнал фильтруется «на глаз».

## Что сделать

1. `repo.PaymentFilter` (`backend/internal/repo/store_iface.go`) — добавить поля:
   - `Q string` — подстрока по email **или** displayName студента;
   - `From time.Time` (нулевое = нет границы), `To time.Time` (нулевое = нет границы).
2. `handler.AdminPaymentsJournal` (`backend/internal/handler/admin_payments.go`) — разобрать
   `q`, `from`, `to`:
   - `from`/`to` парсятся `time.Parse("2006-01-02", v)` (UTC);
   - неверный формат → `400` «Дата в формате ГГГГ-ММ-ДД»;
   - `from` позже `to` → `400` «Начало периода позже конца»;
   - `q` тримится, пустое значение = фильтра нет.
3. Обе реализации `ListPaymentsJournal` — с одинаковой семантикой:
   - память (`backend/internal/repo/store.go`): регистронезависимый `strings.Contains` по
     email и displayName; даты: `!CreatedAt.Before(From)` и `CreatedAt.Before(To.AddDate(0,0,1))`
     (границы включительно, `To` — «до конца суток»);
   - Postgres (`backend/internal/repo/postgres_store.go`, **только функция** `ListPaymentsJournal`):
     `AND (lower(u.email) LIKE $n OR lower(coalesce(u.display_name,'')) LIKE $n)` и
     `AND p.created_at >= $m` / `AND p.created_at < $k` (следующие сутки после `To`).
     Новые условия — через существующий механизм плейсхолдеров (`args`/`fmt.Sprintf`),
     нумерация параметров не должна сбиваться.
4. `totals` (суммы) обязаны считаться **по тому же** отфильтрованному набору, что и `total` —
   проверь, что не сломал: если сейчас суммы считаются отдельным запросом, добавь туда те же условия.
5. Пагинация — после фильтрации (как сейчас).

## Тесты

Новый файл `backend/tests/contract/admin_payments_search_test.go` (пакет `contract_test`,
использовать существующие помощники из `helper_test.go`, примеры — `admin_payments_test.go`):
создать в in-memory store 4–5 пользователей/курсов/платежей с разными email, displayName и
датами (даты строить относительно `time.Now().UTC()`, чтобы не зависеть от часового пояса).

Покрыть: поиск по части email; поиск по части displayName; `q` в другом регистре;
`from`/`to` на сегодня (граница включительно); `from` = завтра → пусто; `to` = вчера → пусто;
`q`+`status` вместе; `totals` уменьшаются по фильтру; `400` на `2026-13-45` и на `01-02-2026`;
`400` при `from > to`; пагинация `page=2` после фильтра.

## Ограничения

- Править только: `backend/internal/handler/admin_payments.go`,
  `backend/internal/repo/store_iface.go`, `backend/internal/repo/store.go` (только `ListPaymentsJournal`),
  `backend/internal/repo/postgres_store.go` (только `ListPaymentsJournal`),
  `backend/tests/contract/admin_payments_search_test.go` (новый), отчёт `backend/tests/T31-payments-search.md`.
- `Task/00-api-contract.md`, `Task/STATUS.md`, `frontend/**`, `specs/**` — не трогать.
- Новых зависимостей нет. Код, комментарии и сообщения об ошибках — по-русски.

## Проверка (Docker; Go на хост не ставить)

```powershell
docker run --rm -v "C:\Users\96352\all_project\eduPlatform\edu-platform\backend:/src" -v gomodcache:/go/pkg/mod -w /src -e GOFLAGS=-mod=mod golang:1.24-alpine sh -c "go build ./... && go vet ./... && go test ./... -count=1"
```

Ждём `exit 0` и `ok` во всех пакетах (включая `tests/contract`).

## Отчёт

`backend/tests/T31-payments-search.md`: что изменено (файл → функция), список проверок теста,
вывод прогона `go test`, честный список непроверенного (например: Postgres-путь покрыт только
чтением кода — интеграционные тесты делает T34).
