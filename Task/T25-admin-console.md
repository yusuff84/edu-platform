# T25 — Бэкенд: супер-админ — сводка, курсы, отзывы, записи

Волна: P3. Зависимости: нет. Потребитель: T27 (UI «Сводка» и «Курсы»).

## Зачем

Админка — это четыре якорные секции на одной странице: очередь модерации, журнал
платежей, комиссия, приглашения. **Сводки состояния платформы нет вообще**: ни
сколько пользователей, ни сколько курсов по статусам, ни выручки. Модерация «слепая»:
в очереди видно карточку, но нельзя посмотреть курс целиком. Отзывы не модерируются.
Все записи на курсы (`enrollments`) админ не видит — есть только `revoke`/`unrevoke`
по конкретной паре, но найти эту пару негде.

## Владеет файлами (менять можно только их)

- `backend/internal/handler/admin_console.go` (новый)
- `backend/internal/repo/admin_console.go` (новый — методы на `*Store`)
- `backend/internal/repo/postgres_admin_console.go` (новый — методы на `*PostgresStore`)
- `backend/internal/repo/store_iface.go` (+5 методов в `StoreAPI`)
- `backend/internal/handler/middleware.go` (+1 строка монтирования в `r.Route("/admin", ...)`)
- `backend/tests/contract/admin_console_test.go` (новый)

## Хранилище

```go
// AdminOverview — агрегаты для сводки супер-админа одним вызовом (T25).
AdminOverview() AdminTotals

// ListAllCourses — курсы всех статусов с фильтрами для админки (T25),
// новые первыми; total — до пагинации.
ListAllCourses(f CourseFilter) (items []*Course, total int)

// DeleteReview удаляет отзыв и пересчитывает рейтинг курса (T25).
// ErrNotFound — отзыва нет.
DeleteReview(id string) error

// ListEnrollmentRows — все записи на курсы с данными ученика и курса (T25),
// новые первыми; total — до пагинации.
ListEnrollmentRows(f EnrollmentFilter) (items []EnrollmentRow, total int)

// CourseContentCounts — курсы → (модули, уроки, ученики) одним вызовом (T25,
// без N+1 в списке курсов).
CourseContentCounts() map[string]ContentCounts
```

Типы (в том же файле `admin_console.go`, чтобы не трогать `models.go`):

```go
type CourseFilter struct {
	Page, Limit int
	Status      CourseStatus // "" — любой
	Q           string       // подстрока по title, регистронезависимо
}

type EnrollmentFilter struct {
	Page, Limit int
	CourseID    string // "" — любой
	StudentID   string // "" — любые
}

type EnrollmentRow struct {
	Enrollment
	StudentEmail string
	StudentName  string
	CourseTitle  string
	Completed    int
	TotalLessons int
}

// AdminTotals — все агрегаты сводки (T25).
type AdminTotals struct {
	UsersTotal, UsersStudents, UsersTeachers, UsersAdmins, UsersBlocked int
	UsersNew7                                                          int
	CoursesTotal, CoursesDraft, CoursesReview, CoursesPublished        int
	CoursesArchived                                                    int
	PaymentsCount, PaidCents, PlatformFeeCents, TeacherCents           int
	PaidLast30Cents                                                    int
	EnrollmentsTotal, EnrollmentsActive                                int
	ReviewsTotal                                                       int
	AvgRating                                                          float64
}
```

- `ContentCounts` **переиспользовать** существующий (модули/уроки). Учеников в нём
  нет — либо расширить структуру новым полем `Students` (тогда это правка
  `store_iface.go`, он в твоём владении), либо отдавать `studentsCount` отдельной
  картой. Реши и зафиксируй комментарием.
- Postgres: агрегаты считать SQL-ом, а не выгрузкой таблиц в Go.

## API (внутри `/api/v1/admin`, requireRole(admin) уже навешен)

### GET `/stats`

200:

```json
{
  "users": {"total": 10, "students": 7, "teachers": 2, "admins": 1, "blocked": 0, "newLast7": 3},
  "courses": {"total": 5, "draft": 2, "review": 1, "published": 1, "archived": 1},
  "payments": {"count": 4, "succeededCents": 1960000, "platformFeeCents": 196000, "teacherCents": 1764000, "last30Cents": 980000},
  "enrollments": {"total": 9, "active": 8},
  "reviews": {"total": 6, "avgRating": 4.5}
}
```

- Денежные суммы — только по платежам в статусе `succeeded`; `last30Cents` — за
  последние 30 дней по `createdAt`.
- `avgRating` — среднее по всем отзывам, округление до 1 знака, 0 если отзывов нет.
- Платформенная комиссия берётся из полей платежа (`platformFeeCents`,
  `teacherAmountCents`), а не пересчитывается от текущей настройки.

### GET `/courses?status=&q=&page=&limit=`

200:

```json
{
  "items": [
    {
      "id": "c-1", "slug": "osnovy-go", "title": "Основы Go",
      "teacherId": "u-2", "teacherEmail": "teacher@local", "status": "published",
      "priceCents": 490000, "createdAt": "...", "updatedAt": "...",
      "modulesCount": 3, "lessonsCount": 18, "studentsCount": 12
    }
  ],
  "total": 1, "page": 1, "limit": 20
}
```

- `status` вне `draft|review|published|archived` → 400; пустой — любой.
- `q` — подстрока по названию, регистронезависимо.
- `items` — всегда массив.

### POST `/courses/{id}/archive`

200 `{"course":{...}}`, статус `archived`. 409 если уже `archived`. 404 — нет курса.

### POST `/courses/{id}/unpublish`

200 `{"course":{...}}`, `published` → `draft`. 409 для любого другого статуса. 404.

### GET `/courses/{id}/reviews`

200 `{"items":[{"id","studentId","studentEmail","rating","body","createdAt"}]}`
(массив, не `null`). 404 — нет курса.

### DELETE `/reviews/{id}`

204 без тела. 404 — нет отзыва. После удаления рейтинг курса пересчитан
(`RecalcCourseRating`).

### GET `/enrollments?courseId=&studentId=&page=&limit=`

200:

```json
{
  "items": [
    {
      "id": "e-1", "studentId": "u-1", "studentEmail": "ivan@example.com",
      "studentName": "Иван", "courseId": "c-1", "courseTitle": "Основы Go",
      "status": "active", "createdAt": "...",
      "completedLessons": 3, "totalLessons": 10, "percent": 30
    }
  ],
  "total": 1, "page": 1, "limit": 20
}
```

- Новые первыми; `items` всегда массив.
- Фильтры комбинируются через AND.

Правило кодов: 400 — валидация query, 409 — конфликт состояния, 404 — нет сущности.

## Тесты (обязательно)

`backend/tests/contract/admin_console_test.go` сквозь `NewRouter`, не менее:

1. `/stats` — форма ответа целиком, все ключи на месте; на пустой базе нули
   (и `items`-подобных `null` нет).
2. `/stats` после успешной оплаты: `succeededCents` вырос, `count` +1.
3. `/courses` — все статусы видны админу; фильтр `status=published`; `status=мусор` → 400;
   `q` фильтрует.
4. `archive` published → archived; повторно → 409.
5. `unpublish` published → draft; для draft → 409.
6. `GET /courses/{id}/reviews` → массив; `DELETE /reviews/{id}` → 204, отзыв исчез,
   `avgRating` курса пересчитан; повторное удаление → 404.
7. `/enrollments` — фильтр по `courseId` и по `studentId`, пагинация, `percent` верный.
8. Не-админ на любом из эндпоинтов → 403.

## Критерии приёмки

- `go build ./... && go vet ./... && go test ./... -count=1` — зелёные (Docker).
- `go test -race` — зелёный в `golang:1.24-bookworm`.
- Существующие эндпоинты админки (`/admin/payments`, `/admin/settings`,
  `/admin/courses/review`, `/admin/courses/{id}/moderate`, `/admin/enrollments/...`)
  не сломаны — их тесты проходят без правок.
- JSON-поля ровно как в контракте (T27 пишется параллельно).

## Проверка

```
docker run --rm -v "<путь>\backend:/src" -v gomodcache:/go/pkg/mod -w /src ^
  -e GOFLAGS=-mod=mod golang:1.24-alpine sh -c "go build ./... && go vet ./... && go test ./... -count=1"
```
