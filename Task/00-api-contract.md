# 00 — Зафиксированный контракт API (Task/00-api-contract.md)

> **Это источник правды.** Все фронтенд-задачи (T01–T06, T08) пишутся строго
> под эти формы. Бэкенд соответствует этому документу; если фронтенду кажется,
> что «так удобнее» — меняется фронтенд, а не контракт.
> Исключения оговорены явно в разделе «Отступления».

Проверено по исходникам `backend/internal/handler/*.go` и `backend/internal/repo/models.go`.

---

## 0. Общие правила

| Правило | Значение |
|---|---|
| Базовый URL | `/api/v1` (nginx/vite проксирует на `api:8080`) |
| Аутентификация | cookie `session_id`, httpOnly, `SameSite=Lax`; клиент шлёт `credentials: 'include'` |
| Формат тела | `application/json` |
| Ограничение тела | 1 МБ |
| **Строгость JSON** | backend вызывает `DisallowUnknownFields()` → **любое лишнее поле = `400 «Некорректное тело запроса»»** |
| Ошибка | `{"error": "<текст по-русски>"}`, HTTP-код в поле `status` фронтового `ApiError` |
| Пагинация | `page` (с 1), `limit` (по умолч. 20, максимум 100) |

**Золотое правило фронтенда:** отправлять ровно те поля, что перечислены ниже.
Никаких `...(rest)`, никаких `undefined`-полей через `JSON.stringify`
(убедись, что неиспользуемые поля не попадают в объект — используй условное
распространение или собирай объект явно).

---

## 1. `POST /auth/register`

Тело:
```json
{ "email": "string", "password": "string", "inviteToken": "string" }
```
`inviteToken` опционален. Если передан — регистрация в роли `teacher`.

Ответ `201`:
```json
{ "id": "uuid", "email": "string", "role": "student|teacher|admin", "displayName": "string" }
```

Ошибки: `400` (валидация, невалидный invite-токен, **занятый email** —
расхождение №8: документ ранее обещал `409`, факт закреплён тестом
`auth_test.go` и проверкой A6 в `qa-abc.ps1`).

---

## 2. `POST /auth/login`

Тело: `{ "email": "string", "password": "string" }`
Ответ `200`: та же форма `User`, что в п.1. Устанавливает cookie `session_id`.
Ошибки: `400`, `401`.

---

## 3. `POST /auth/logout`

Тело пустое. Фактически ответ `200` с JSON `{"status":"ok"}`, а не `204`
(расхождение №2: документ обещал `204 No Content`; код и контрактный тест
`tests/contract/auth_test.go` закрепляют `200`, фронтенд принимает оба
варианта — см. отчёт T10).

---

## 4. `GET /auth/me`

Ответ `200`: `User`. Если сессии нет — `401`.

---

## 5. `GET /courses` — каталог

Query: `q` (поиск), `page`, `limit`.

Ответ `200`:
```json
{
  "items": [CourseItem],
  "total": 42,
  "page": 1,
  "limit": 20
}
```

`CourseItem`:
```json
{
  "id": "uuid",
  "slug": "python-s-nulya",
  "title": "string",
  "description": "string",
  "priceCents": 4900,
  "currency": "RUB",
  "avgRating": 4.5,
  "ratingsCount": 12
}
```

> ⚠️ Именно `ratingsCount`, **не** `reviewsCount`.
> ⚠️ Поля `status`, `teacherName`, `coverUrl` в `CourseItem` **отсутствуют**
> (omitempty + другая структура) — не рассчитывай на них в каталоге.

---

## 6. `GET /courses/{slug}` — страница курса

`slug` — человекоприятный слаг вида `python-s-nulya`, **не UUID**.

Ответ `200`:
```json
{
  "course": {
    "id": "uuid",
    "teacherId": "uuid",
    "slug": "string",
    "title": "string",
    "description": "string",
    "coverPath": "string",
    "priceCents": 4900,
    "currency": "RUB",
    "status": "draft|review|published|archived",
    "avgRating": 4.5,
    "ratingsCount": 12,
    "moderationComment": "string",
    "createdAt": "RFC3339",
    "updatedAt": "RFC3339"
  },
  "modules": [ModuleView]
}
```

> ⚠️ Это **объект-конверт**, а не «плоский курс». Отдельного поля `modules`
> на верхнем уровне `course` нет.

`ModuleView`:
```json
{ "id": "uuid", "title": "string", "position": 1, "lessons": [LessonView] }
```

`LessonView`:
```json
{
  "id": "uuid",
  "title": "string",
  "kind": "video_text|live|quiz|code",
  "position": 1,
  "bodyMd": "string",
  "videoUrl": "string",
  "liveUrl": "string",
  "startsAt": "RFC3339",
  "quiz":    { "questions": [QuizQuestionPublic] },
  "codeTask": { "language": "string", "template": "string", "tests": [CodeTest] }
}
```

`QuizQuestionPublic`:
```json
{ "id": "uuid", "text": "string", "options": ["a","b"], "explanation": "string" }
```

> ⚠️ Именно `text`, **не** `question`. Именно `quiz: { questions: [...] }`,
> **не** `quiz: [...]`.
> ⚠️ Поле `correct` в `QuizQuestionPublic` **отсутствует** — ответы не утекают.
> ⚠️ В `codeTask.tests` приходят только тесты с `hidden != true`.

Видимость: черновик (`draft`) виден только автору-преподавателю и админу,
остальным `404`. Публичному пользователю в `lessonView` приходит `quiz`
без `correct`.

---

## 7. `POST /teacher/courses` — создать черновик

Тело: `{ "title": "string", "description": "string", "priceCents": 0, "slug": "string", "currency": "RUB" }`
Ответ `201`: объект `Course` (полная форма из п.6).

---

## 8. `GET /teacher/courses` — мои курсы

Ответ `200`: **`{ "items": [Course], "total": 5 }`**
Это конверт, а не массив.

---

## 9. `POST /teacher/courses/{id}/modules`

Тело: `{ "title": "string" }`
Ответ `201`: `{ "id", "courseId", "title", "position" }`

---

## 10. `POST /teacher/courses/{id}/lessons`

Тело (**точно эти имена**):
```json
{
  "moduleId": "uuid",
  "title": "string",
  "kind": "video_text|live|quiz|code",
  "bodyMd": "string",
  "videoUrl": "string",
  "liveUrl": "string",
  "startsAt": "RFC3339-строка",
  "quiz": { "questions": [
      { "id": "uuid", "text": "string", "options": ["a","b"], "correct": 0, "explanation": "string" }
  ]},
  "codeTask": { "language": "python|js|htmlcss", "template": "string",
                "tests": [ { "name": "string", "input": "string", "expected": "string", "hidden": true } ] }
}
```

> ⚠️ `textContent` → **`bodyMd`**; `scheduledAt` → **`startsAt`**;
> `quiz` — объект с `questions`, а не массив; `codeTask` — объект, а не строка.
> `correct` обязателен и должен быть в диапазоне `[0, len(options))`.
> Поля, не относящиеся к `kind`, **не отправлять** — иначе 400
> (например, `quiz` для `kind: "video_text"` лучше не слать вовсе).

Ответ `201`: объект `Lesson`.

---

## 11. `POST /teacher/courses/{id}/submit`

Тело пустое. Ответ `200`: объект `Course` со `status: "review"`.

---

## 12. `POST /admin/invites`

Тело: `{ "email": "string" }`
Ответ `201`: `{ "id", "email", "token", "tokenPrefix", "expiresAt" }`
**Токен показывается один раз** — сохранять его в UI нужно сразу.

---

## 13. `GET /admin/courses/review` — очередь модерации

Ответ `200`: **`{ "items": [Course], "total": 3 }`** — конверт, не массив.

---

## 14. `POST /admin/courses/{id}/moderate`

Тело: `{ "decision": "approve|reject", "comment": "string" }`
`comment` **обязателен при `reject`**, иначе `400`.
Ответ `200`: объект `Course`.
При `reject` курс возвращается в `draft` с заполненным `moderationComment`.

---

## 15. `POST /enrollments` — покупка

Тело: `{ "courseId": "uuid", "provider": "mock|stripe|yookassa" }`
Ответ `201`:
```json
{
  "paymentId": "uuid",
  "providerPaymentId": "string",
  "redirectUrl": "string",
  "amountCents": 4900,
  "status": "pending|succeeded|failed|refunded",
  "enrollmentActive": false
}
```
При `priceCents === 0` → `enrollmentActive: true`, статус `succeeded`.
Ошибки: `404`, `409` (уже есть активная запись).

---

## 16. `POST /payments/webhook/{provider}`

Принимает провайдер-специфичное тело, проверяет подпись.
Ответ `200`: `{ "status": "processed|duplicate|ignored" }`

---

## 17. `GET /me/courses` — мои курсы с прогрессом

Ответ `200`:
```json
{
  "items": [
    {
      "course": { /* CourseItem из п.5 */ },
      "percent": 40,
      "nextLesson": { "lessonId": "uuid", "title": "string", "kind": "video_text" } | null
    }
  ],
  "total": 3
}
```

> ⚠️ `nextLesson` — это **объект `{lessonId,title,kind}`**, а не `nextLessonId`.
> Плоского `courseId`/`slug`/`percent` на верхнем уровне нет.

---

## 18. `POST /me/courses/{id}/lessons/{lessonId}/complete`

Тело (для video/live можно **вообще не слать**):
```json
{ "quizAnswers": { "<questionId>": 0 }, "code": "string", "language": "python" }
```

Ответ `200` — **два разных варианта**:

Успех:
```json
{ "passed": true, "completed": true, "percent": 40,
  "nextLesson": { "lessonId": "uuid", "title": "string", "kind": "video_text" } | null }
```

Провал (T16: `percent`/`nextLesson` добавлены и при провале — по той же
логике, что и при успехе, чтобы фронт обновлял ленту без рефетча):
```json
{ "passed": false, "completed": false, "message": "текст ошибки",
  "percent": 40,
  "nextLesson": { "lessonId": "uuid", "title": "string", "kind": "video_text" } | null }
```
(для quiz дополнительно `correct: 1, total: 5`)

> ⚠️ Ключ называется **`passed`**, а не `accepted`.

Ошибки: `400` (нет ответов на quiz / нет кода / урок не принадлежит курсу),
`403` (нет активной записи), `404`, `502` (runner недоступен).

---

## 19. `GET /courses/{id}/reviews`

`{id}` — UUID **или** slug, backend принимает оба.

Ответ `200`:
```json
{
  "items": [ { "id": "uuid", "studentId": "uuid", "courseId": "uuid",
               "rating": 5, "body": "string", "createdAt": "RFC3339" } ],
  "total": 3,
  "avgRating": 4.5,
  "ratingsCount": 3
}
```

> ⚠️ Конверт. Поля `authorEmail` **нет** (backend его не отдаёт),
> текст отзыва называется **`body`**, а не `text`.
> Определить «свой» отзыв можно только по `studentId` vs `User.id` из `/auth/me`.

---

## 20. `POST /courses/{id}/reviews`

Тело: **`{ "rating": 1..5, "body": "string" }`** — ключ `body`, **не** `text`.
Ответ `201`: объект `Review`.
Ошибки: `400`, `403` (нет активной записи на курс), `409` (отзыв уже есть).

---

## 21. `GET /healthz` и `GET /api/v1/healthz`

Ответ `200`, JSON `{"status":"ok"}` (расхождение №5: документ ранее обещал
плоский текст `ok`; факт — см. `handler/middleware.go:healthz` и проверку A1
в `qa-abc.ps1`).

---

## Новые эндпоинты US6 (реализуются в T07, потребляются в T08)

### `GET /admin/payments?page=&limit=&status=&courseId=`
Только роль `admin`.
```json
{
  "items": [
    { "id": "uuid", "studentId": "uuid", "studentEmail": "student@example.com",
      "studentName": "Имя студента (может быть пустым — тогда показывать studentEmail)",
      "courseId": "uuid", "courseTitle": "string",
      "amountCents": 4900, "platformFeeCents": 735, "teacherAmountCents": 4165,
      "currency": "RUB", "provider": "mock", "providerPaymentId": "string",
      "status": "pending|succeeded|failed|refunded",
      "createdAt": "RFC3339", "processedAt": "RFC3339" | null }
  ],
  "total": 42, "page": 1, "limit": 20,
  "totals": { "amountCents": 123400, "platformFeeCents": 18510, "teacherAmountCents": 104890 }
}
```
> `studentEmail`/`studentName` добавлены T16 (расхождение №6 закрыто):
> колонка «Студент» в админке больше не показывает обрезанный UUID.

### `GET /admin/settings`
```json
{ "commissionPct": 15 }
```

### `PUT /admin/settings`
Тело: `{ "commissionPct": 15 }` (`0..90`)
Ответ `200`: `{ "commissionPct": 15 }`
Ошибка записи в БД → `500 {"error": "..."}` (T16: раньше Postgres глотал
ошибку, API отвечал `200`, но значение не сохранялось).

### `POST /admin/enrollments/{studentId}/courses/{courseId}/revoke` и `.../unrevoke` (T16, п.6)
Только роль `admin`. Ручной отзыв/восстановление доступа к курсу.

Ответ `200`:
```json
{ "studentId": "uuid", "courseId": "uuid", "status": "revoked|active" }
```
`revoke` → `status: "revoked"` (доступ закрыт: `/me/courses` больше не отдаёт
курс, `.../complete` отвечает `403`), `unrevoke` → `status: "active"`.
Ошибки: `400` (не UUID), `404` (записи нет).

---

## Эндпоинты волны P3 (T21–T25)

Все пути ниже — от `/api/v1`. Конверты пагинации те же: `{items,total,page,limit}`.
Владение курсом проверяет общий `ownCourse`: `403` — чужой курс, `404` — курса нет;
админу разрешены все курсы.

### Конструктор курса (T21)

| Метод | Путь | Ответ | Коды |
|---|---|---|---|
| POST | `/teacher/courses/{id}/duplicate` | `201 {"course":{…}}` — копия-черновик вызывающего | `403`, `404` |
| POST | `/teacher/courses/{id}/archive` | `200 {"course":{…}}`, статус `archived` | `409` уже в архиве |
| POST | `/teacher/courses/{id}/unarchive` | `200 {"course":{…}}`, статус `draft` | `409` не в архиве |
| POST | `/teacher/courses/{id}/withdraw` | `200 {"course":{…}}`, `review → draft`, комментарий модерации очищается | `409` не на модерации |
| POST | `/teacher/courses/{id}/lessons/{lessonId}/move` | `200 {"lesson":{…}}` — урок в конце другого модуля, тело `{"moduleId":"uuid"}` | `400` пустой или чужой `moduleId`, `404` урока нет в курсе, `409` курс не черновик |

Копия курса копирует программу глубоко (модули и уроки с новыми `id`), но не
записи, платежи, отзывы и прогресс. Повторный перенос в тот же модуль —
идемпотентный `200`.

### Ученики курса (T22)

`GET /teacher/courses/{id}/students?q=&page=&limit=` → `200 {items,total,page,limit}`,
элемент: `studentId,email,displayName,enrolledAt,status,completedLessons,totalLessons,percent,lastActivityAt`.
`q` ищет по email и имени; `lastActivityAt` = `null`, пока ученик ничего не проходил.

`GET /teacher/courses/{id}/stats` → `200`
`{studentsTotal,studentsActive,studentsCompleted,avgPercent,revenueCents,modulesTotal,lessonsTotal,reviewsCount,avgRating}`.

### Кабинет ученика (T23)

| Метод | Путь | Ответ | Коды |
|---|---|---|---|
| GET | `/me/courses/{id}/progress` | `200 {courseId,completedLessonIds,completedLessons,totalLessons,percent,nextLessonId,completed}`; `completedLessonIds` — всегда массив, `nextLessonId` = `null` при полном прохождении | `403` нет активной записи, `404` курса нет |
| GET | `/me/profile` | `200 {id,email,role,displayName,createdAt}` | `401` |
| PATCH | `/me/profile` | `200 {id,email,role,displayName,createdAt}`, тело `{"displayName":"…"}` | `400` имя короче 2 или длиннее 60 символов |
| GET | `/me/payments?page=&limit=` | `200 {items,total,page,limit}`, элемент: `id,courseId,courseTitle,amountCents,status,createdAt,processedAt` | `401` |

### Супер-админ (T24/T25) — только роль `admin`

| Метод | Путь | Ответ | Коды |
|---|---|---|---|
| GET | `/admin/users?q=&role=&status=&page=&limit=` | `200 {items,total,page,limit}`, элемент: `id,email,role,displayName,status,createdAt,coursesCount,enrollmentsCount` | `400` мусорные `role`/`status` |
| PATCH | `/admin/users/{id}/role` | `200 {"user":{…}}`, тело `{"role":"student\|teacher\|admin"}` | `400` мусорная роль, `409` себе, `409` последний админ, `404` |
| PATCH | `/admin/users/{id}/status` | `200 {"user":{…}}`, тело `{"status":"active\|blocked"}` | `409` себе, `409` последний админ |
| DELETE | `/admin/users/{id}` | `204` | `409` себе / последний админ / есть курсы / есть платежи |
| GET | `/admin/stats` | `200 {users,courses,payments,enrollments,reviews}`: `users{total,students,teachers,admins,blocked,newLast7}`, `courses{total,draft,review,published,archived}`, `payments{count,succeededCents,platformFeeCents,teacherCents,last30Cents}`, `enrollments{total,active}`, `reviews{total,avgRating}` | `403` |
| GET | `/admin/courses?q=&status=&page=&limit=` | `200 {items,total,page,limit}`, элемент: `id,slug,title,teacherId,teacherEmail,status,priceCents,createdAt,updatedAt,modulesCount,lessonsCount,studentsCount` | `400` мусорный `status` |
| POST | `/admin/courses/{id}/unpublish` | `200 {"course":{…}}`, `published → draft` | `409` не опубликован |
| POST | `/admin/courses/{id}/archive` | `200 {"course":{…}}`, статус `archived` | `409` уже в архиве |
| GET | `/admin/courses/{id}/reviews` | `200 {items,total}`: `id,studentId,studentEmail,rating,body,createdAt` | |
| DELETE | `/admin/reviews/{id}` | `204` | `404` |
| GET | `/admin/enrollments?courseId=&page=&limit=` | `200 {items,total,page,limit}`: `id,studentId,studentEmail,studentName,courseId,courseTitle,status,createdAt,completedLessons,totalLessons,percent` | |

Заблокированный пользователь (`status: "blocked"`) получает `403` и на входе,
и по уже выданной cookie-сессии; разблокировка возвращает доступ без повторного
входа. Маршрут `GET /admin/courses/review` (T07) сохраняется: он зарегистрирован
раньше `GET /admin/courses` и не перехватывается.

Сквозная проверка этих эндпоинтов — `qa-p3.ps1` (80 проверок).

---

## Эндпоинты волны P4 (T31–T35)

Все пути ниже — от `/api/v1`. Коды те же: `400` — валидация, `403` — нет прав /
заблокирован, `404` — нет сущности, `409` — конфликт состояния.

### Поиск и период в журнале платежей (T31)

`GET /admin/payments?q=&status=&courseId=&from=&to=&page=&limit=` — только роль `admin`.

| Параметр | Формат | Смысл |
|---|---|---|
| `q` | строка | подстрока (без учёта регистра) по email **или** displayName студента; пусто — без фильтра |
| `from` | `ГГГГ-ММ-ДД` | платежи этой даты и позже (с начала суток) |
| `to` | `ГГГГ-ММ-ДД` | платежи этой даты и раньше (до конца суток) |

- `status` и `courseId` — как было (см. раздел US6 выше).
- Фильтры складываются по «И»; форма ответа прежняя: `{items,total,page,limit,totals}`,
  причём `totals` считаются **по отфильтрованному набору**.
- Пустые `from`/`to` — фильтра нет. Неверный формат даты или `from > to` → `400`
  «Даты в формате ГГГГ-ММ-ДД».

### Блокировка и вход через Яндекс (T32)

`GET /auth/yandex/callback`: если найденный или только что созданный пользователь
заблокирован (`status = blocked`), cookie-сессия **не** ставится; ответ тот же `403`
с тем же текстом, что при парольном входе заблокированного пользователя.
Новый пользователь через Яндекс создаётся в статусе `active`.

### Правка карточки курса (T33)

`PATCH /teacher/courses/{id}` (владелец или админ):

- `draft | review | published` — правка разрешена (осознанное исключение из правила
  «менять можно только черновик»: карточка не проходит модерацию заново);
- `archived` — `409` «Курс в архиве: карточка заморожена»; сначала вернуть курс
  из архива (`POST /teacher/courses/{id}/unarchive`).

Валидация полей (пустые строки, отрицательная цена, занятый slug, пустое тело) —
как была, `400`.

### Закрываемые расхождения (без изменения API)

| Что | Кто | Смысл |
|---|---|---|
| `AdminOverview` на Postgres считает `users.blocked` | T34 | в SQL-запросе нет условия `status='blocked'` — счётчик всегда 0 |
| Интеграционные тесты Postgres (env-gated) | T34 | новые методы P3 проверяются только сквозным QA на живом стенде |
| Негативные и конкурентные тесты платёжного контура | T35 | подделка подписи вебхука, повторная доставка, гонка на одном `providerPaymentId`, preflight `OPTIONS` |

---

## Отступления (осознанные и разрешённые)

| Отступление | Кто | Почему |
|---|---|---|
| Фронтенд нормализует `{items:[Course],total}` в массив на уровне клиента | T01 | Менять 8 хендлеров ради удобства дороже; контракт снаружи остаётся |
| `getCourse` принимает `slug`, но конструктору нужен курс по `id` | T01/T06 | Использовать `myTeaching()` и найти по `id` там, где `slug` неизвестен |
| Фронтенд добавляет `import type { Role }` из клиента | T01–T06 | Переиспользование типов вместо дублей |

## Расхождения, найденные агентами (действуй по ним)

Обновляется по мере работы агентов. **Обязательно прочитай перед началом работы.**

| # | Расхождение | Где | Статус | Что делать |
|---|---|---|---|---|
| 1 | `User.displayName` обещан контрактом, но `handler/auth.go` не отдаёт его | backend | ✅ закрыто T10 | Принимать `displayName?: string` — фронт не должен падать без него |
| 2 | `POST /auth/logout` возвращает `200 {"status":"ok"}`, а не `204` | backend | ✅ закрыто T10; **документ исправлен T20** (п.3) | Клиент обрабатывает оба варианта; не полагаться на `204` |
| 3 | `listReviews` теряет `avgRating`/`ratingsCount` из конверта (клиент возвращает плоский массив) | T01 → T04 | ⚠️ осознанно | **Рейтинг курса бери из `getCourse()`** (`course.avgRating`, `course.ratingsCount`), а не из `listReviews()` |
| 4 | `POST /teacher/courses/{id}/lessons` принимает `startsAt`, но **не сохраняет** его в урок | backend | ✅ закрыто T16 | `req.StartsAt` парсится и копируется в `repo.Lesson`; некорректная дата — `400`. Тест-фиксатор обновлён: `teacher_test.go:TestStartsAtPersisted` |
| 5 | `GET /healthz` возвращает JSON `{"status":"ok"}`, а не текст `ok` | backend | ⚠️ код не менялся; **документ исправлен T20** (п.21) | Сверяйся с `docs/API.md`, он описывает фактическое поведение; проверка A1 в `qa-abc.ps1` |
| 6 | `GET /admin/payments` отдаёт только `studentId`, без email/имени | backend | ✅ закрыто T16 | В строке журнала теперь есть `studentEmail` и `studentName` (пустое имя → показывать email). Тест-фиксатор в `admin_payments_test.go` обновлён на проверку наличия полей |
| 7 | `formatPrice(0)` возвращает «Бесплатно» — неверно для итогов журнала | frontend | ⚠️ обойдено | T08 использовал локальный форматтер `money()`; `client.ts` не менялся |
| 8 | `POST /auth/register` при занятом email отвечает `400`, а контракт обещает `409` | backend | ✅ подтверждено | Закреплено тестом `auth_test.go:20-26` и проверкой A6 в `qa-abc.ps1`. **Контракт поправлен на 400** — чинить код бессмысленно, тест уже зафиксировал поведение |

Все расхождения подтверждены контрактными тестами T12. Тест-фиксатор
`teacher_test.go:TestStartsAtNotPersistedDiscrepancy4` **специально упал**,
когда расхождение №4 починили (T16) — по нему обновлено ожидание:
тест переименован в `TestStartsAtPersisted` и теперь требует `startsAt != null`.
Аналогично обновлён фиксатор в `admin_payments_test.go` (расхождение №6).

Расхождение 4 было реальным багом бэкенда (`handler/teacher.go`: поле не
копировалось в `repo.Lesson`) — закрыто T16.
Если заметишь похожее — пиши в отчёте, не правь чужие файлы.

## Правило кодов 400/409 (закреплено T20)

Фактическая семантика API, зафиксирована тестом
`internal/handler/teacher_edit_test.go:TestSlugConflictIs400Consistent`
и сквозной проверкой B7d в `qa-abc.ps1`:

- **`400 Bad Request`** — ошибка валидации тела/параметров запроса, в том
  числе **уникальность ссылаемых значений**: занятый `email` при
  `POST /auth/register`, занятый `slug` при `POST/PATCH /teacher/courses`
  (одинаково в `teacher.go:67` и `teacher_edit.go:112`).
- **`409 Conflict`** — конфликт с **целевым состоянием ресурса**: активная
  запись на курс при повторной покупке, существующий отзыв, удаление/правка
  вне статуса `draft`, удаление непустого модуля.

Альтернатива по RFC 9110 (slug → 409) потребовала бы правки прод-хендлеров,
что выходит за границы владения T20; код и документ согласованы на правиле
выше, ветка-исключение отсутствует.

## Что НЕ делать

- ❌ Не добавлять поля в запросы, которых нет в этом документе (`DisallowUnknownFields` → 400).
- ❌ Не переименовывать ключи ответов «для красоты».
- ❌ Не отправлять `JSON.stringify({...x, optional: undefined})` — поле уйдёт как `null`; собирай объект явно.
- ❌ Не полагаться на порядок полей — используй точные имена.
