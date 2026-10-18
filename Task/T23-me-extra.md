# T23 — Бэкенд: прогресс ученика, профиль, история платежей

Волна: P3. Зависимости: нет. Потребитель: T30 (UI кабинета ученика).

## Зачем

Сейчас прогресс рисуется **эвристикой на клиенте**: `MyCourses.tsx` берёт счётчики
курса и сравнивает с чем-то локальным, потому что сервер не отдаёт список пройденных
уроков. Из-за этого прогресс врёт после перезахода и не переживает смену устройства.
Профиля у ученика нет вовсе (нельзя задать имя), история платежей недоступна.

`CountCompletedLessons` в хранилище **уже реализован и не вызывается ни одним
хендлером** — этот эндпоинт вводит его в строй.

## Владеет файлами (менять можно только их)

- `backend/internal/handler/me_extra.go` (новый)
- `backend/internal/repo/me_extra.go` (новый — методы на `*Store`)
- `backend/internal/repo/postgres_me_extra.go` (новый — методы на `*PostgresStore`)
- `backend/internal/repo/store_iface.go` (+2 метода в `StoreAPI`)
- `backend/internal/handler/middleware.go` (+1 строка монтирования на уровне `/api/v1`)
- `backend/tests/contract/me_extra_test.go` (новый)

## API (requireAuth обязателен на всех)

### GET `/me/courses/{id}/progress`

200:

```json
{
  "courseId": "c-1",
  "completedLessonIds": ["l-1", "l-3"],
  "completedLessons": 2,
  "totalLessons": 10,
  "percent": 20,
  "nextLessonId": "l-2",
  "completed": false
}
```

- `completedLessonIds` — **массив, всегда не `null`** (пустой — `[]`).
- Порядок — по позиции урока в курсе.
- `nextLessonId` — первый непройденный урок по порядку (модуль за модулем, позиция
  внутри модуля); если всё пройдено — `null`.
- `completed` — `true`, когда `totalLessons > 0 && completedLessons == totalLessons`.
- `percent` — `completedLessons * 100 / totalLessons`, вниз; при `totalLessons == 0` → 0.
- 404 — курса нет. 403 — нет активной записи на курс (`GetEnrollment` + статус
  `active`; `refunded`/`revoked` доступа не дают).

### GET `/me/profile`

200: `{"id":"u-1","email":"a@b.c","role":"student","displayName":"Иван","createdAt":"..."}`
`displayName` — пустая строка, если не задан (не `null`).

### PATCH `/me/profile`

Тело: `{"displayName":"Иван"}` (лишние поля — 400, `decodeJSON` уже это делает).

- `TrimSpace`; длина 2..60 → иначе 400 с русским текстом.
- 200 — тот же объект, что `GET /me/profile`, с новым именем.
- Использовать существующий `SetUserDisplayName`.

### GET `/me/payments?page=&limit=`

200:

```json
{
  "items": [
    {
      "id": "p-1",
      "courseId": "c-1",
      "courseTitle": "Основы Go",
      "amountCents": 490000,
      "currency": "RUB",
      "status": "succeeded",
      "provider": "mock",
      "createdAt": "2026-01-01T00:00:00Z",
      "processedAt": "2026-01-01T00:05:00Z"
    }
  ],
  "total": 1, "page": 1, "limit": 20
}
```

- Только платежи текущего пользователя, новые первыми.
- `courseTitle` подтягивается из курса; если курс удалён — пустая строка.
- `processedAt` — `null`, пока платёж не обработан.
- Пагинация — существующим `paginate`.
- Включены платежи во всех статусах (ученик должен видеть и `failed`).

## Хранилище

Добавить в `StoreAPI` ровно два метода:

```go
// ListCompletedLessonIDs — id пройденных уроков курса в порядке программы (T23).
// Никогда не nil (пустой слайс — если ничего не пройдено).
ListCompletedLessonIDs(studentID, courseID string) []string

// ListPaymentsByStudent — платежи ученика, новые первыми (T23).
ListPaymentsByStudent(studentID string) []*Payment
```

- In-memory — `me_extra.go`; Postgres — `postgres_me_extra.go` (сортировка в SQL,
  порядок уроков — JOIN с modules/lessons по position).
- `totalLessons` — существующий `CountLessonsByCourse`; количество пройденных —
  `len(completedLessonIds)`.

## Тесты (обязательно)

`backend/tests/contract/me_extra_test.go` сквозь `NewRouter`, не менее:

1. Прогресс пустого прохождения: `completedLessonIds == []` (проверить отсутствие
   `null` в JSON), `percent=0`, `nextLessonId` — первый урок.
2. После `POST /me/courses/{id}/lessons/{lessonId}/complete` урок попадает в список,
   `percent` растёт, `nextLessonId` сдвигается.
3. Пройдены все уроки → `completed=true`, `nextLessonId=null`.
4. Курс без записи → 403; несуществующий курс → 404.
5. `PATCH /me/profile`: имя из 2 символов — ок; из 1 и из 61 — 400; лишнее поле — 400.
6. `GET /me/payments`: пусто → `items=[]`, `total=0`; после mock-оплаты платёж виден
   с `courseTitle` и `processedAt`.

## Критерии приёмки

- `go build ./... && go vet ./... && go test ./... -count=1` — зелёные (Docker).
- `go test -race` — зелёный в `golang:1.24-bookworm`.
- Существующие тесты не изменены.
- Поля названы ровно как в контракте (T30 пишется параллельно).

## Проверка

```
docker run --rm -v "<путь>\backend:/src" -v gomodcache:/go/pkg/mod -w /src ^
  -e GOFLAGS=-mod=mod golang:1.24-alpine sh -c "go build ./... && go vet ./... && go test ./... -count=1"
```
