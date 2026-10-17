# T22 — Бэкенд: ученики курса и статистика для учителя

Волна: P3. Зависимости: нет. Потребитель: T29 (UI «Ученики курса»).

## Зачем

Учитель не видит, кто записался на его курс: ни email, ни прогресса, ни выручки.
Модель данных всё это хранит (`Enrollment`, `LessonProgress`, `Payment`), но
наружу не отдаётся ни одного эндпоинта. Без этого раздела конструктор слепой:
непонятно, работает ли курс.

## Владеет файлами (менять можно только их)

- `backend/internal/handler/teacher_students.go` (новый)
- `backend/internal/repo/course_students.go` (новый — методы на `*Store`)
- `backend/internal/repo/postgres_course_students.go` (новый — методы на `*PostgresStore`)
- `backend/internal/repo/store_iface.go` (+3 метода в `StoreAPI`)
- `backend/internal/handler/middleware.go` (+1 строка монтирования внутри `r.Route("/teacher", ...)`)
- `backend/tests/contract/teacher_students_test.go` (новый)

## API (внутри `/api/v1/teacher`)

Владение курсом проверяет существующий `ownCourse` (владелец или админ).

### GET `/courses/{id}/students?page=&limit=&q=`

200:

```json
{
  "items": [
    {
      "studentId": "u-1",
      "email": "ivan@example.com",
      "displayName": "Иван",
      "enrolledAt": "2026-01-02T10:00:00Z",
      "status": "active",
      "completedLessons": 3,
      "totalLessons": 10,
      "percent": 30,
      "lastActivityAt": "2026-01-05T12:00:00Z"
    }
  ],
  "total": 1, "page": 1, "limit": 20
}
```

- Сортировка: `enrolledAt` по убыванию (сначала новые записи).
- Пагинация — существующим `paginate(total, page, limit)`; `limit` ≤ 100, по умолчанию 20.
- `q` — подстрока по `email` или `displayName`, регистронезависимо, обрезать `TrimSpace`.
- `percent` — целое 0..100: `completedLessons * 100 / totalLessons`, при `totalLessons == 0` → 0.
- `lastActivityAt` — максимум `CompletedAt` по урокам курса; если ученик не начинал —
  **`null`** (не нулевое время).
- Включены записи во всех статусах (`active`, `refunded`, `revoked`) — учитель должен
  видеть и отчисления; статус отдаётся полем.
- 404 если курса нет, 403 если чужой.

### GET `/courses/{id}/stats`

200:

```json
{
  "studentsTotal": 12,
  "studentsActive": 10,
  "studentsCompleted": 2,
  "avgPercent": 41,
  "revenueCents": 588000,
  "modulesTotal": 3,
  "lessonsTotal": 18,
  "reviewsCount": 4,
  "avgRating": 4.5
}
```

- `studentsTotal` — все записи, `studentsActive` — со статусом `active`.
- `studentsCompleted` — ученики с `percent == 100` (при `lessonsTotal > 0`).
- `avgPercent` — среднее по всем записям, округление вниз, при отсутствии записей 0.
- `revenueCents` — сумма `amountCents` **только успешных** платежей по курсу
  (`PaymentStatus == succeeded`), без вычетов.
- `reviewsCount` / `avgRating` — из `ListReviewsByCourse` (среднее по рейтингам,
  округление до 1 знака), при отсутствии отзывов 0.
- 404/403 как выше.

Правило кодов: 400 — невалидные query-параметры, 404 — нет курса, 403 — чужой.

## Хранилище

Добавить в `StoreAPI` ровно три метода:

```go
// ListEnrollmentsByCourse — все записи на курс, новые первыми (T22).
ListEnrollmentsByCourse(courseID string) []*Enrollment

// StudentProgress — карта studentID → прогресс по курсу одним запросом (T22).
// Ключи — только ученики, у которых есть записи на курс.
StudentProgress(courseID string) map[string]StudentProgress

// ListPaymentsByCourse — платежи по курсу; фильтрацию по статусу делает handler (T22).
ListPaymentsByCourse(courseID string) []*Payment
```

```go
// StudentProgress — агрегат по одному ученику курса (T22).
type StudentProgress struct {
	Completed    int       // пройдено уроков
	LastActivity time.Time // максимум CompletedAt; нулевое — не начинал
}
```

- In-memory — `course_students.go`.
- Postgres — `postgres_course_students.go`, без N+1: три запроса (`enrollments`,
  `lesson_progress` + `lessons` по `course_id`, `payments` по `course_id`).
- Данные пользователя (email, displayName) handler добирает через `GetUserByID`
  по уже усечённой пагинацией странице (не более 100 вызовов) — это допустимо,
  зафиксируй комментарием почему.

## Тесты (обязательно)

`backend/tests/contract/teacher_students_test.go` сквозь `NewRouter`, не менее:

1. Пустой курс → `total=0`, `items=[]` (не `null`).
2. Два ученика, часть уроков пройдена → корректные `completedLessons`, `percent`,
   `lastActivityAt`; у непроходившего `lastActivityAt == null`.
3. `q` фильтрует по email и по имени, регистронезависимо.
4. Пагинация: `limit=1` даёт 1 элемент и `total=2`.
5. Статистика: `studentsTotal`, `studentsCompleted`, `revenueCents` считаются верно
   (платёж в статусе `pending` в выручку не попадает).
6. Чужой курс → 403; несуществующий → 404.

## Критерии приёмки

- `go build ./... && go vet ./... && go test ./... -count=1` — зелёные (Docker).
- `go test -race` — зелёный в `golang:1.24-bookworm`.
- Существующие тесты не изменены.
- JSON-поля названы ровно так, как в контракте (фронтенд T29 пишется параллельно!).

## Проверка

```
docker run --rm -v "<путь>\backend:/src" -v gomodcache:/go/pkg/mod -w /src ^
  -e GOFLAGS=-mod=mod golang:1.24-alpine sh -c "go build ./... && go vet ./... && go test ./... -count=1"
```
