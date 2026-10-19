# T55 — детали курса для админа (GET /admin/courses/{id})

**Волна:** P7  **Ветка:** `task/T55-admin-course-detail`  **Worktree:** `wt/t55`
**Статус:** ✅ готово (ветка запушена, не смержена — координатор)

## Что сделано

- **Новый handler** `backend/internal/handler/admin_course_detail.go`:
  - `AdminCourseDetail` — `GET /admin/courses/{id}`, только admin (права
    проверяет middleware группы `/admin`: `requireAuth + requireRole(admin)`,
    маршрут зарегистрирован в `middleware.go` рядом с
    `RegisterAdminConsoleRoutes`);
  - ответ строго по замороженному разделу T55 `Task/00-api-contract.md`:
    `course` (форма элемента `GET /admin/courses` T25 — встраивание
    `adminCourseItem` — плюс `coverPath: string|null`), `modules`
    (по `position`, уроки внутри модуля по `position`, урок — только
    `{id,title,kind,position}`, `lessons`/`modules` — всегда массивы) и
    `stats` (`studentsCount` — ВСЕ записи, `activeCount` — только `active`,
    `reviewsCount`/`avgRating` — как в публичной карточке курса,
    `paymentsCount`/`paidTotalCents` — только платежи `succeeded`);
  - ошибки: `404 «Курс не найден»` (в том числе не-UUID id), `403` — не админ,
    `401` — нет сессии.
- **Хранилище** — новый метод `AdminCourseDetail(courseID) (*AdminCourseDetailData, bool)`:
  - декларация в `backend/internal/repo/store_iface.go` (+ типы
    `AdminCourseDetailData`, `AdminCourseDetailStats`);
  - in-memory: `backend/internal/repo/store.go` — один проход под одним
    `RLock` (тот же приём, что `ListEnrollmentRows` T25: вложенные вызовы
    взяли бы RWMutex повторно); наружу — снимки, не живые указатели;
  - Postgres: **новый файл** `backend/internal/repo/postgres_course_detail.go`
    — два запроса + склейка: (1) курс `courseColsJoined` + email автора
    (LEFT JOIN users) + счётчики и агрегаты платежей скалярными подзапросами —
    приёмы из `postgres_admin_console.go` (`AdminOverview`,
    `CourseContentCounts`); (2) программа `modules LEFT JOIN lessons`
    `ORDER BY m.position, l.position` (LEFT JOIN — модуль без уроков не
    теряется); ошибка скана не-UUID id → «курса нет», как в `GetCourseByID`.
- **docs/API.md** — строка эндпоинта в разделе «Админ».
- Отзывы по курсу не тронуты: `GET /admin/courses/{id}/reviews` и
  `DELETE /admin/reviews/{id}` (T25) работают как раньше (закреплено тестом
  `TestAdminCourseDetailDoesNotShadowReviews`).

## Решения по спорным местам

1. **`coverPath` при невлитой T54.** Колонка `cover_path` уже есть
   (`001_init.sql`), модель `Course.CoverPath` тоже, но T54 (эндпоинты
   загрузки) в ветке отсутствует → реальные пути пустые. Поле отдаётся
   всегда (`*string`, `json:"coverPath"` без omitempty): пустое значение →
   явный JSON `null`. После влития T54 polling координатора не требуется:
   эндпоинт читает `cover_path` напрямую, форма не меняется.
2. **`avgRating`/`reviewsCount`** берутся из полей курса
   (`avg_rating`/`ratings_count`), а не пересчётом сырых отзывов: это тот же
   источник, что у публичной карточки («как в публичной карточке» из
   контракта); `RecalcCourseRating` поддерживает согласованность на каждом
   изменении отзывов.
3. **`stats.studentsCount` ≠ `course.studentsCount`:** stats — ВСЕ записи
   (контракт T55), course — элемент списка T25 (только `active`); различие
   осознанное и закреплено тестом `TestAdminCourseDetailStatsWithActivity`.

## Контрактные тесты

`backend/tests/contract/admin_course_detail_p7_test.go` (пакет
`contract_test`, соседние helpers: `newTestApp`, `doRequest`, `wantKeys`,
`wantNum`, `seedPublishedCourse`, `createDraftCourse`):

| Тест | Проверяет |
|---|---|
| `TestAdminCourseDetailHappyPath` | курс с 2 модулями (video_text + quiz / live): порядок программы по `position`; урок — ровно `{id,title,kind,position}` (любое лишнее поле — `wantKeys` упадёт); форма `course` = 13 полей элемента T25 + `coverPath`; `teacherEmail` непустой; общие поля совпадают с элементом `GET /admin/courses`; `stats` нули (учеников/платежей нет) |
| `TestAdminCourseDetailNotFound` | fake id (нулевой UUID и не-UUID) → `404 «Курс не найден»` |
| `TestAdminCourseDetailRoleGuards` | teacher-сессия → `403`; без сессии → `401` |
| `TestAdminCourseDetailEmptyStats` | пустой курс: все 6 счётчиков `stats` = 0, `modules` = `[]` (не null), `coverPath` присутствует и `null`, счётчики элемента 0 |
| `TestAdminCourseDetailStatsWithActivity` | бесплатный курс: 1 запись (active), 1 succeeded-платёж на 0 копеек, отзыв 5 → `avgRating` 5; согласованность `studentsCount` course/stats |
| `TestAdminCourseDetailDoesNotShadowReviews` | налог шляпы маршрута: `/admin/courses/{id}` не перехватил `/admin/courses/{id}/reviews` (T25) |

## Проверка (Docker)

```
docker run --rm -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src \
  golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Результат см. ниже (обновляется в конце задачи).

## Изменённые файлы

- НОВЫЕ: `backend/internal/handler/admin_course_detail.go`,
  `backend/internal/repo/postgres_course_detail.go`,
  `backend/tests/contract/admin_course_detail_p7_test.go`;
- ПРАВКИ: `backend/internal/handler/middleware.go` (только регистрация
  маршрута рядом с остальными `/admin`), `backend/internal/repo/store_iface.go`
  (метод + типы), `backend/internal/repo/store.go` (in-memory реализация),
  `docs/API.md` (строка в разделе «Админ»).

## Ограничения / notes для координатора

- Верстающий frontend (T59) `coverPath` до влития T54 всегда получает `null` —
  формы не ломает (строка|null).
- SQL-версия `AdminCourseDetail` проверяется только сквозным QA на живом
  стенде (env-gated, как принято с T34) — in-memory путь покрыт контрактными
  тестами.
