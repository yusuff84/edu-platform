# T67 — сертификаты о завершении курса: отчёт

## Что сделано

1. **Миграция** `backend/internal/migrate/migrations/010_certificates.sql`:
   таблица `certificates` (`id UUID PK`, `user_id`/`course_id` c
   `ON DELETE CASCADE`, `number TEXT UNIQUE`, `issued_at TIMESTAMPTZ`,
   `UNIQUE (user_id, course_id)`) и индекс `(user_id, issued_at DESC)`.
   Идемпотентна (`IF NOT EXISTS`) в стиле `007_cover.sql`.
2. **Модели** (`repo/models.go`): `Certificate` (id, userId, courseId, number,
   issuedAt), `CertificateListItem` (number, issuedAt, courseId, courseTitle,
   courseSlug), `CertificateDetails` (number, issuedAt, studentName,
   courseTitle, courseSlug) — JSON-теги по контракту.
3. **StoreAPI** (блок в конце интерфейса):
   - `IssueCertificate(studentID, courseID)` — идемпотентная выдача: при
     существующей паре возвращается тот же сертификат; номер
     `EDU-<ГГГГ>-<8 hex ВЕРХНИМ регистром>` через `crypto/rand`, при коллизии
     номера — повторная генерация (до 5 попыток); нет ученика/курса —
     `ErrNotFound`.
   - `GetCertificateByNumber(number)` — публичная карточка с названием/слагом
     курса и `displayName` студента.
   - `ListCertificatesByStudent(studentID)` — новые первыми (`issued_at DESC`,
     при равенстве — номер по убыванию), никогда не `nil`.
   Реализации: in-memory `repo/certificates.go`, Postgres
   `repo/postgres_certificates.go`; каскад in-memory при удалении курса и
   пользователя зеркалит FK `ON DELETE CASCADE`.
4. **Автовыдача**: в `CompleteLesson` после фиксации прогресса вызывается
   `issueCertificateIfCourseCompleted` — сертификат создаётся, когда пройдены
   все уроки курса и уроков ≥ 1. Повторное завершение урока/повторное
   достижение 100% второй сертификат не создаёт (идемпотентность хранилища).
5. **`GET /me/certificates`** (requireAuth) — `{"items":[…]}` по контракту,
   новые первыми; зарегистрирован рядом с другими `/me`-маршрутами.
6. **`GET /me/courses`** — аддитивное поле `certificateNumber` в каждом
   элементе (пустая строка, если сертификата нет).
7. **`GET /certificates/{number}`** — публичный маршрут без авторизации,
   поля по контракту, `404 «Сертификат не найден»`; e-mail не отдаётся.
8. **Тесты**:
   - `backend/tests/contract/certificates_test.go`: выдача после последнего
     урока (и отсутствие до него), идемпотентность при повторном завершении,
     список `/me/certificates` (порядок новые→старые), `certificateNumber` в
     `/me/courses` (есть/нет), публичный GET без cookie, 404, 401 для
     `/me/certificates`.
   - `backend/tests/postgres/certificates_test.go`: parity-тесты Postgres —
     выдача/повторная выдача (одна строка), формат номера, карточка по номеру
     с `displayName`, список и порядок, пустой список `[]`, `ErrNotFound` для
     несуществующих ученика/курса, каскад при удалении курса и ученика.
9. **`docs/API.md`** — короткая приписка: публичный `GET /certificates/{number}`,
   `GET /me/certificates` и аддитивное `certificateNumber` в `/me/courses`.

## Проверка

### go vet + все тесты (Docker, без БД)

Из корня worktree:

```
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Результат:

```
ok  	edu-platform/backend/cmd/api	4.747s
ok  	edu-platform/backend/internal/auth	0.105s
?   	edu-platform/backend/internal/config	[no test files]
ok  	edu-platform/backend/internal/handler	3.443s
?   	edu-platform/backend/internal/migrate	[no test files]
ok  	edu-platform/backend/internal/payments	0.075s
ok  	edu-platform/backend/internal/repo	0.034s
?   	edu-platform/backend/internal/service	[no test files]
?   	edu-platform/backend/internal/storage	[no test files]
ok  	edu-platform/backend/tests/contract	40.164s
ok  	edu-platform/backend/tests/postgres	0.023s
```

Новые контрактные тесты отдельно: `TestCertificateIssuedAfterLastLesson`,
`TestCertificateOnlyAfterAllLessonsAndIdempotent`,
`TestMyCertificatesNewestFirst` — PASS.

### Postgres-parity

Контейнер `postgres:16-alpine` (`p8-t67-pg`, порт **55435**, БД `edu_test`,
`edu/edu`), прогон из контейнера тестов с хостом `host.docker.internal`:

```
docker run --rm --add-host=host.docker.internal:host-gateway -e HOME=/root \
  -e TEST_DATABASE_URL="postgres://edu:edu@host.docker.internal:55435/edu_test?sslmode=disable" \
  -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine \
  sh -c "go test ./tests/postgres -count=1 -v"
```

Результат: все 14 тестов пакета PASS, в том числе
`TestPostgresCertificateIssueAndLookup`, `TestPostgresCertificatesOrderAndCascade`,
`TestPostgresMigrationsIdempotent` (повторный накат 001–010) и
`TestPostgresDeleteCourseCascade`. После прогона контейнер остановлен и удалён.

## Отклонения от постановки

- **Порт Postgres 55435 вместо 55432** — требование координатора волны
  (55432 может быть занят параллельным агентом). Семантика проверки та же.
- **Проверка «100%» в хендлере** реализована не сравнением процента, а явной
  проверкой «все уроки курса завершены, уроков ≥ 1» — это точное условие
  контракта и не зависит от целочисленного деления процента.
- **Год в номере** берётся по UTC (`time.Now().UTC().Year()`), чтобы номер не
  зависел от часового пояса сервера.
- **Точное совпадение номера** при публичной проверке (без нормализации
  регистра): контракт задаёт номер в верхнем регистре, отдельного правила
  нормализации нет. При необходимости регистронезависимого поиска — открытый
  вопрос ниже.

## Открытые вопросы

- Нужно ли `GET /certificates/{number}` принимать номер в нижнем регистре
  (сейчас — только точное совпадение с выданным номером)?
- Нужно ли аннулировать сертификат при возврате средств/отзыве записи
  (revoked/refunded)? Контракт T67 этого не требует; сертификат остаётся
  выданным, пока живы пользователь и курс.
