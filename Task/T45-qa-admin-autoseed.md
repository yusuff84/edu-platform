# T45 — Сквозные скрипты: автосид qa-админа (scripts)

**Волна:** P5 · **Ветка:** `task/T45-qa-admin-autoseed` · **Worktree:** `wt/t45`

## Зачем

Ограничение P4: `qa-abc.ps1` ждёт учётку `qa-admin@example.test`, которую на
стенде заводили руками (регистрация + `PATCH /admin/users/{id}/role`).
Автоматизируем — сквозной прогон не должен зависеть от ручных шагов.

## Что сделать

1. Новый `qa-seed-admin.ps1` (PowerShell 5.1, **UTF-8 с BOM**; стиль и утилиты —
   по образцу `qa-p4.ps1`):
   - параметры: `-BaseUrl` = `http://localhost:8082`, `-AdminEmail` = `qa-admin@example.test`,
     `-AdminPassword` = `QaAdmin12345`, `-SeedAdminEmail` = `admin@local`,
     `-SeedAdminPassword` = `admin1234`;
   - алгоритм: `POST /auth/login` под `AdminEmail` → `200` = готово (exit 0);
     иначе вход сид-админом (неудача → понятная ошибка + exit 1);
     `GET /admin/users?q=<AdminEmail>` — найти учётку; если нет — `POST /auth/register`;
     затем `PATCH /admin/users/{id}/role` `{"role":"admin"}`; повторный
     `POST /auth/login` под `AdminEmail` → `200` (иначе exit 1);
   - не-GET запросы — с заголовком `Origin: http://localhost:3000` (CSRF, как в qa-скриптах);
   - учёт `429`: ожидание ~20 с и повтор, до 4 попыток;
   - краткий вывод шагов и итога; код возврата 0/1.
2. `qa-abc.ps1`: если вход под `AdminEmail` не удался и админ-блок включён —
   один раз вызвать `.\qa-seed-admin.ps1` (через `Join-Path $PSScriptRoot ...`;
   те же BaseUrl/AdminEmail/AdminPassword), затем повторить вход; новые
   параметры `-SeedAdminEmail`/`-SeedAdminPassword` (дефолты как выше) и switch
   `-NoAutoSeed`. Поведение без автосида — прежнее. Провалившаяся первая
   попытка входа не должна попадать в счёт проверок (B1 переформулировать аккуратно).
3. Докблоки обоих скриптов; в `README.md` (раздел «Тесты и QA», абзац про qa-abc)
   добавить 2–3 строки про автосид.

## Правила волны

- Работать только в своём worktree; коммит — в свою ветку `task/T45-qa-admin-autoseed`.
- Править только: `qa-seed-admin.ps1` (новый), `qa-abc.ps1`, `README.md`
  (только абзац про автосид), отчёт `backend/tests/T45-qa-admin-autoseed.md`.
- `qa-p3.ps1`, `qa-p4.ps1` работают под `admin@local` и не трогаются.
  Новых зависимостей нет.

## Проверка (живой стенд, api `localhost:8082`; между прогонами пауза ~60 с — лимит `/auth/*`)

1. `qa-abc.ps1` с существующим qa-admin → **49/49** (регрессия).
2. `qa-abc.ps1 -AdminEmail "qa-seed-<штамп>@example.test"` → автосид нового
   админа → **49/49**; повторный прогон с той же учёткой → **49/49**.
3. Хвосты выводов обоих прогонов — в отчёт.

## Отчёт

`backend/tests/T45-qa-admin-autoseed.md`: как работает автосид, вывод прогонов,
ограничения (сид-админ должен совпадать с `ADMIN_EMAIL`/`ADMIN_PASSWORD` стенда
или передаваться параметрами).
