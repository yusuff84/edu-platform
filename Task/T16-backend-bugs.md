# T16 — Бэкенд-баги: уроки, журнал, возвраты, демо-учётки

**Ключ сессии:** `T16-backend-bugs` · **Волна:** A · **Слой:** backend · **Приоритет:** P0

Исправляет семь багов, найденных задачами T03, T06, T07, T12 и обнаруженных
координатором при сквозном прогоне. Ничего не ломает: все изменения должны
пройти существующие контрактные тесты без правки самих тестов (кроме
тестов-фиксаторов, которые специально падают — см. п.1).

## Владение файлами (жёсткая граница)

```
backend/internal/handler/teacher.go
backend/internal/handler/progress.go
backend/internal/handler/admin_payments.go
backend/internal/repo/store_iface.go
backend/internal/repo/store.go
backend/internal/repo/postgres_store.go
backend/internal/repo/models.go
backend/cmd/api/main.go
```

**Не трогать:** `frontend/**` (T17, T18), `backend/tests/contract/**` (T20),
`middleware.go`, `docker-compose.yml`, `specs/**`.

---

## 1. `startsAt` принимается, но не сохраняется (P0)

**Симптом:** `POST /teacher/courses/{id}/lessons` принимает поле `startsAt`
(не отвечает 400), но `handler/teacher.go:171-180` **не копирует** `req.StartsAt`
в `repo.Lesson`. Планирование живых эфиров не работает вообще: преподаватель
заполняет дату, а фронт всегда показывает «Эфир ещё не назначен».

**Важно:** в `postgres_store.go:525-526` поле **уже пишется** — теряется
ровно на шаге копирования в хендлере, поэтому баг проявляется в обоих сторах.

**Что сделать:** скопировать `req.StartsAt` в `repo.Lesson`. Поле `repo.Lesson`
уже существует (`models.go`) — **`models.go` не менять**.

**Тест-фиксатор уже ждёт:** `tests/contract/teacher_test.go` →
`TestStartsAtNotPersistedDiscrepancy4`. Сейчас он проходит, ожидая `startsAt=null`.
**После твоей правки он ЗАПАДЕТ** — это задумано: он подтверждает, что баг
закрыт. **Обнови ожидание теста** на `startsAt != null` и переименуй в
`TestStartsAtPersisted`, убрав формулировку про расхождение. Иначе сборка упадёт.
О том, что фиксатор падает при починке, сказано в `Task/00-api-contract.md`.

---

## 2. При провале квиза/кода нет `percent` и `nextLesson` (P1)

**Симптом:** `POST .../lessons/{id}/complete` при **неуспешной** попытке
возвращает только `{passed:false, correct, total, message}`. Фронт (T03) не
может обновить прогресс и вынужден полагаться на процент из `me-courses`
до рефетча — ложная лента прогресса.

**Что сделать:** в `handler/progress.go` при провале тоже возвращать текущие
`percent` и доступный `nextLesson` — по той же логике, что и при успехе.

**Контракт:** форма ответа описана в `Task/00-api-contract.md`. Добавление
полей обратно совместимо, фронт их читает (`CompleteResult`). Обнови описание
в контракте, если формулировка отличается.

---

## 3. `GET /admin/payments` не отдаёт email студента (P1)

**Симптом:** в `admin_payments.go` DTO есть только `studentId`. Админка (T08)
показывает обрезанный UUID в колонке «Студент» — непригодно для работы.
Расхождение №6 контракта.

**Что сделать:**
- расширить `PaymentJournalRow` (`store_iface.go`) полем `StudentEmail`
  (и `StudentDisplayName`, если пуст — тогда показывать email);
- в `postgres_store.go` — уже есть `JOIN courses c` для названия курса,
  добавить `JOIN users u ON u.id = p.student_id` и выбрать email;
- в `store.go` (in-memory) — достать email из `store.users`;
- в DTO `admin_payments.go` добавить `studentEmail`;
- **`items` по-прежнему никогда не `null`**;
- **`tests/contract/admin_payments_test.go`** содержит тест, который падает,
  если появится `studentEmail` — обнови ожидание (см. п.1).

---

## 4. `SetCommissionPct` в Postgres глотает ошибку записи (P1)

**Симптом:** `postgres_store.go` делает `_, _ = ...` при записи комиссии.
Админ меняет комиссию, API отвечает `200`, но в базу ничего не записалось —
настройка «теряется» после перезапуска, и никто не знает, почему.

**Что сделать:** пробрасывать `err`. `SetCommissionPct` вызывается из
`PUT /admin/settings` — там ошибка уже обрабатывается (`admin_payments.go`).
Проверь, что сигнатура совпадает с `store_iface.go` и что **in-memory кэш
обновляется только после успешной записи** (иначе рассинхрон кэша и БД).

---

## 5. `EnsureEnrollment` не сбрасывает `refunded` → `active` (P1)

**Симптом:** студент вернул деньги (`EnrollmentRefunded`), потом купил курс
заново. `EnsureEnrollment` при конфликте возвращает **существующую запись**
со статусом `refunded` — оплата прошла, а доступ не открылся.

**Что сделать:** при успешном новом платеже переводить запись в
`EnrollmentActive`. Используй уже существующий метод
`SetEnrollmentStatus(studentID, courseID, status)` — его добавил T07.
**Не дублируй** — сначала посмотри `store_iface.go`.

**Место вызова:** `handler/enroll.go` в ветке успешного платежа, либо внутри
`EnsureEnrollment`, если семантика метода это допускает. Выбери вариант и
обоснуй в отчёте. Файл `enroll.go` свободен.

**Обработай и возврат:** в `handler/enroll.go` при `refunded` уже вызывается
`SetEnrollmentStatus(..., EnrollmentRefunded)` — проверь, что после твоей
правки это не конфликтует (повторная покупка → active, возврат → refunded).

---

## 6. Нет `POST /admin/enrollments/revoke`, `EnrollmentRevoked` не выставляется (P2)

**Симптом:** статус `EnrollmentRevoked` объявлен в `models.go`, но **никем
не выставляется** — админ не может отозвать доступ вручную.

**Что сделать:**
- новый файл `backend/internal/handler/admin_enrollments.go` (уникальное имя);
- `RegisterAdminEnrollmentRoutes(r chi.Router)` — **по образцу** T07 в
  `admin_payments.go:20`, регистрирующий **абсолютные** пути
  `/admin/enrollments/{studentId}/courses/{courseId}/revoke` и `unrevoke`;
- `.With(requireAuth, requireRole(admin))` **внутри метода**;
- вызов регистрации — в `middleware.go` на уровне `/api/v1`
  (в том же месте, где `app.RegisterAdminPaymentsRoutes(r)`, строка ~145).
  **`middleware.go` — единственная твоя правка в нём, одна строка.**

Это единственное исключение из запрета трогать `middleware.go`.

---

## 7. Демо-учётки из миграции не могут войти (P0 — блокирует сквозной прогон)

**Обнаружено координатором сквозным прогоном `qa-abc.ps1`.**

В `backend/internal/migrate/migrations/002_seed.sql:5-6` хеши **поддельные**:

```
admin@local   => $2a$10$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAadmin1234
teacher@local => $2a$10$BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBteacher1
```

Это 50 символов-заглушка плюс **открытый текст пароля**. Настоящий bcrypt —
60 символов псевдослучайных (сравни у студентов, зарегистрированных API:
`$2a$10$GReDsivc/VyEoFHdyQymlecFO7cOTHqbgxpamVPcHa3LsTUoiu8w.`).

**Почему `seedAdmin` не помогает:** `main.go:177` делает
`if _, ok := store.GetUserByEmail(cfg.AdminEmail); ok { return }` — пользователь
**уже существует** (его создала миграция), и хеш не исправляется никогда.

**Миграции НЕ трогать** — это артефакт SDD-процесса, и уже применённые
миграции не перезапустятся.

**Что сделать в `cmd/api/main.go`, функция `seedAdmin`:**
1. Если пользователя нет — как сейчас (создать с настоящим bcrypt).
2. Если пользователь **есть**, но `ADMIN_PASSWORD` задан — проверить хеш:
   `bcrypt.CompareHashAndPassword([]byte(stored), []byte(cfg.AdminPassword))`.
   - Ошибка парсинга хеша **или** несовпадение пароля → **перезаписать хеш**
     настоящим `bcrypt.GenerateFromPassword`.
   - Совпадает → ничего не делать.
3. Логировать каждое действие по-русски: создан / исправлен / уже корректен.

Это безопасно: срабатывает только при явно заданных `ADMIN_EMAIL`+
`ADMIN_PASSWORD`, секретов в коде нет.

**То же для `teacher@local`?** Учитель из миграции тоже с поддельным хешем,
но переменных под него нет. Решение за тобой — предлагаю вариант:
ввести `TEACHER_EMAIL`/`TEACHER_PASSWORD` и общий хелпер
`ensureAccount(store, email, password, role)` вместо дублирования логики.
Обоснуй выбор в отчёте.

**Связано:** `demo-teacher@local` из `seedDemo` (in-memory) — рабочий,
его не трогай.

---

## Проверка (выполни обязательно)

```
docker run --rm -v "C:\Users\96352\all_project\eduPlatform\wt-T16-backend-bugs\backend:/src" -v gomodcache:/go/pkg/mod -w /src -e GOFLAGS=-mod=mod golang:1.24-alpine sh -c "go build ./... && go vet ./... && go test ./... -count=1 && echo ALL_OK"
```

Для `-race` (нужен после правок `store.go`): в alpine
`apk add gcc musl-dev` и `CGO_ENABLED=1`.

**Сквозная проверка — самая ценная:**
```
cd C:\Users\96352\all_project\eduPlatform\wt-T16-backend-bugs
docker compose up -d --build db api runner
docker compose logs api | findstr /i "админ"     # должна быть строка "исправлен"/"создан"
powershell -File qa-abc.ps1 -AdminEmail admin@local -AdminPassword admin1234
docker compose down
```
**Цель: 39/39 PASS, exit 0.** Сейчас на интеграционной ветке — **8 PASS / 1 FAIL / 12 SKIP**
ровно из-за п.7. Это твой главный критерий приёмки.

Если `docker compose build` упадёт с `exec format error` на `node:20-alpine`
или `nginx:1.27-alpine` — это **повреждённые слои в локальном dockerd**, не код.
Не поднимай сервис `web`, он для QA не нужен.

## Коммит
```
git -C C:\Users\96352\all_project\eduPlatform\wt-T16-backend-bugs add -A
git -C C:\Users\96352\all_project\eduPlatform\wt-T16-backend-bugs commit -m "T16: <кратко>"
```

## Отчёт (обязателен)
По каждому из 7 пунктов: что было, что сделал, где в коде (file:line).
Отдельно: результат `qa-abc.ps1` (числа PASS/FAIL/SKIP), что именно
потребовало обновления тестов-фиксаторов, и решение по `teacher@local`.
