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
| Время | RFC3339 в UTC (`Z`) независимо от часового пояса сервера; `datetime-local` без зоны трактуется как UTC |
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
Ответ `201`: `{ "id", "email", "token", "tokenPrefix", "expiresAt", "inviteLink", "emailSent" }`
**Токен показывается один раз** — сохранять его в UI нужно сразу.

`inviteLink` — готовая ссылка `/register?inviteToken=...` на базе
`PUBLIC_APP_URL`. Если настроен SMTP (`SMTP_ADDR`/`SMTP_FROM`), сервер
отправляет на email письмо со ссылкой и ставит `emailSent: true`. Ошибка SMTP
не отменяет приглашение: ответ остаётся `201`, `emailSent: false`, а админ
передаёт ссылку вручную.

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

## Эндпоинты волны P5 (T41–T42)

Все пути ниже — от `/api/v1`. Коды те же: `400` — валидация, `403` — нет прав /
заблокирован, `404` — нет сущности, `409` — конфликт состояния.

### Slug: транслитерация кириллицы (T41)

`POST /teacher/courses` и `PATCH /teacher/courses/{id}` — поле `slug`:

- кириллица транслитерируется в латиницу (сначала нижний регистр) по таблице:
  `а→a б→b в→v г→g д→d е→e ё→e ж→zh з→z и→i й→y к→k л→l м→m н→n о→o п→p р→r
  с→s т→t у→u ф→f х→h ц→c ч→ch ш→sh щ→sch ъ→— ы→y ь→— э→e ю→yu я→ya`;
- всё не из `[a-z0-9]` становится `-`; повторы `-` схлопываются, `-` по краям
  обрезаются, длина — до 120 символов;
- пустой `slug` строится из названия курса по тем же правилам; занятость
  разрешается суффиксами `-2`…`-9` (с учётом лимита длины), затем случайным
  `course-<hex8>` (как раньше); если от названия осталось меньше 3 символов —
  сразу `course-<hex8>`;
- после транслитерации slug обязан быть вида `^[a-z0-9-]{3,120}$`, иначе `400`
  («Некорректный или занятый slug (латиница, цифры, дефис, 3–120 символов)» /
  «Некорректный slug (латиница, цифры, дефис, 3–120 символов)» — тексты те же);
- уже сохранённые slug не пересчитываются (миграции нет).

### Префлайт CORS: маршрутные методы (T42)

`OPTIONS` с разрешённого `Origin` отвечает `204`; `Access-Control-Allow-Methods`
перечисляет реально зарегистрированные методы запрошенного пути + `OPTIONS`
(пример: `/api/v1/courses` → `GET, OPTIONS`; `/api/v1/teacher/courses` →
`GET, POST, OPTIONS`; неизвестный путь — заголовка нет). `Allow-Origin`,
`Allow-Credentials`, `Allow-Headers`, `Max-Age`, `Vary: Origin` — как были;
поведение не-OPTIONS запросов не меняется.

---

## Эндпоинты волны P6 (T47–T52)

Все пути ниже — от `/api/v1`. Раздел заморожен до запуска агентов: бэкенд
приводится к этим формам, фронтенд пишется под них. Коды — те же: `400` —
валидация, `403` — нет прав, `404` — нет сущности, `409` — конфликт состояния.

### `GET /admin/invites` — список приглашений (T47)

Только admin. Query: `page` (с 1, по умолчанию 1), `limit` (по умолчанию 20,
максимум 100). Сортировка — `createdAt` по убыванию.

Ответ `200`:

```json
{
  "items": [
    { "id": "uuid", "email": "user@example.test", "status": "sent",
      "tokenPrefix": "ab12cd34", "expiresAt": "2026-11-18T12:00:00Z",
      "createdAt": "2026-10-19T12:00:00Z", "createdBy": "uuid" }
  ],
  "total": 1, "page": 1, "limit": 20
}
```

`status` — сырой статус хранилища: `sent | accepted | expired | revoked`.
«Истекло» — вычисляемое состояние (`sent` и `expiresAt` в прошлом): бэкенд
статус не пересчитывает, фронт отображает «Истекло». Нечисловые `page/limit`
→ `400 «Параметры page и limit должны быть целыми числами»` (как в журнале).

### `POST /admin/invites/{id}/revoke` — отзыв приглашения (T47)

Только admin. Тело не требуется (не отправлять). Ответ `200`:
`{"invite": {<элемент из списка, "status": "revoked">}}`.

Ошибки: `404` — «Приглашение не найдено»; `409` — статус не `sent`
(«Отозвать можно только активное приглашение»).

### `POST /admin/payments/{id}/refund` — возврат средств (T47)

Только admin. `id` — внутренний UUID платежа (не `providerPaymentId`). Тело
не требуется. Провайдер берётся из поля `provider` самого платежа, вызывается
`PaymentProvider.Refund(providerPaymentId)` (задел T13).

Ответ `200`: `{"payment": <paymentJournalItem со "status": "refunded">}` —
форма элемента та же, что в строке журнала `GET /admin/payments`.

После успеха: платёж → `refunded` (processedAt = момент возврата), запись
ученика на этот курс → `refunded` — та же семантика, что у вебхука `refunded`
(повторная покупка после возврата работает; вебхук-дубль идемпотентен).

Ошибки: `404` — «Платёж не найден»; `409` — статус не `succeeded`
(«Возврат возможен только для успешного платежа»); `502` — «Не удалось
выполнить возврат у провайдера» (ошибка вызова провайдера). Повторный возврат
→ `409`. Применение возврата сериализуется тем же мьютексом, что и вебхук
(`App.webhookMu`, прецедент гонки P5): два параллельных возврата одного
платежа → ровно один `200`, второй `409`.

Mock-платёж (`providerPaymentId` вида `mock_...`) возвращается локально по
данным Store без вызова провайдера: денег вне платформы нет, а реестр mock
живёт в памяти процесса. Поэтому возврат переживает перезапуск api. Для
бесплатного платежа (`free-...`) и реальных провайдеров (stripe/yookassa)
вызов `Refund` обязателен; его ошибка по-прежнему даёт `502`.

### `POST /admin/courses/{id}/unarchive` — вернуть курс из архива (T48)

Только admin. Тело не требуется. `archived → draft`, комментарий модерации
сохраняется. Ответ `200`: `{"course": <Course>}` — как у `archive`/`unpublish`
(T25). Ошибки: `404` — «Курс не найден»; `409` — «Вернуть из архива можно
только архивный курс».

### `GET /admin/courses` — новые параметры (T48)

Дополнительно к `status/q/page/limit` (форма ответа не меняется; старые
запросы работают как раньше — все новые параметры необязательны):

- `sort`: `createdAt | updatedAt | title | price | students`, по умолчанию
  `createdAt`; иное → `400 «Параметр sort должен быть одним из: createdAt,
  updatedAt, title, price, students»`. `price` — по `priceCents`, `title` —
  без учёта регистра, `students` — по числу учеников курса;
- `dir`: `asc | desc`, по умолчанию `desc`; иное → `400 «Параметр dir должен
  быть asc или desc»`;
- при равенстве значений вторичный ключ — `createdAt` по убыванию;
- `teacherEmail`: подстрока по email преподавателя без учёта регистра; пусто —
  без фильтра;
- `from`/`to`: даты `ГГГГ-ММ-ДД` по `createdAt` включительно; тексты ошибок —
  как в журнале платежей («Дата в формате ГГГГ-ММ-ДД», «Начало периода позже
  конца»).

### `GET /admin/courses/review` — дополнение полей (T48)

Существующие поля элементов не меняются; аддитивно добавляются:
`"teacherEmail": "string"` (пустая строка, если автор не найден),
`"modulesCount": 0`, `"lessonsCount": 0` — как в `GET /admin/courses` (T25).

### `GET /admin/enrollments` — новые параметры (T48)

Дополнительно к `courseId/studentId/page/limit` (форма ответа не меняется):

- `q`: подстрока без учёта регистра по email **или** displayName ученика;
  пусто — без фильтра;
- `status`: `active | refunded | revoked`; пусто — любой; иное →
  `400 «Некорректный статус записи: допустимы active, refunded, revoked»`.

---

## Эндпоинты волны P7 (T53–T55) — ЗАМОРОЖЕНО до запуска агентов

Все пути ниже — от `/api/v1`. Коды те же: `400` — валидация, `403` — нет прав,
`404` — нет сущности, `409` — конфликт состояния. Бэкенд приводится к этим
формам (T53–T55), фронтенд (T56–T59) пишется строго под них.

### T53 — жизненный цикл учителя и жёсткая валидация

#### `POST /teacher/courses/{id}/unpublish` — снять с публикации (учитель)

Владелец курса или админ (права — как у `archive`/`withdraw` из T21; админ
может пользоваться и существующим админским
`POST /admin/courses/{id}/unpublish`, T25). Тело не требуется. Семантика:
`published → draft`.
Ответ `200`: `{"course": <Course>}` — как у архивных действий T21.

Ошибки: `404` — «Курс не найден»; `403` — чужой курс; `409` — «Снять с
публикации можно только опубликованный курс» (статус не `published`).

#### `PATCH /teacher/courses/{id}/lessons/{lessonId}` — `startsAt` в двух форматах

Дополнительно к текущим правилам PATCH: поле `startsAt` у `live`-урока
принимается и в RFC3339 (как POST), и в datetime-local
(`ГГГГ-ММ-ДДТЧЧ:ММ`, без зоны); вторая форма трактуется как UTC. Пустое
`startsAt` → `400`, как было. Права/коды — без изменений.

#### Валидация quiz на бэкенде

`POST /teacher/courses/{id}/lessons` (kind `quiz`) и `PATCH` того же урока
проверяют `quiz.questions`:
- текст вопроса непустой после TrimSpace;
- `options`: не менее двух, каждый непустой;
- ровно один правильный вариант (`correctIndex` в границах списка).
Иначе `400 «Некорректный тест: <причина>»` (тексты в свободной форме).
Существующие уроки, сохранённые раньше, не рестриктурируются.

#### Чистка мёртвого кода

В `handler/teacher.go` удаляется мёртвый построчный дубль регистрации
`GET /teacher/courses` (реальная регистрация остаётся только через
`RegisterTeacherCRUDRoutes`); внешний контракт не меняется — это лишь
очистка кода.

### T54 — обложка курса (cover)

#### `POST /teacher/courses/{id}/cover` (multipart/form-data)

Владелец курса или admin (монтаж внутри `r.Route("/teacher")` — среди teacher-
маршрутов). Поле формы: `file` (обязательное). Допустимые типы по байта тела
(`http.DetectContentType`): `image/jpeg | image/png | image/webp`; размер
≤ `5 * 1024 * 1024` байт; иначе `400 «Обложка: допустимы JPEG, PNG или WebP
не больше 5 МБ»`.

Успех `201`: файл сохраняется через локальное хранилище (`storage.SaveLocal`)
в `/uploads/covers/…` с путём, привязанным к курсу (повторная загрузка —
перезаписывает файл), поле `cover_path` обновляется; ответ:
`{"coverPath":"/uploads/covers/<имя>"}`.

Ошибки: `400` — нет файла / плохой тип / слишком большой; `401/403`; `404` —
«Курс не найден»; `409` — «Курс в архиве: обложка заморожена» (статус
`archived`).

#### `DELETE /teacher/courses/{id}/cover`

Владелец/admin. Успех `204` (обложки не было — тем же `204`): `cover_path`
становится `NULL` (физический файл удалять не обязательно). `409` — archived;
`404` — неизвестный курс.

#### `coverPath` в публичных и служебных ответах (аддитивно)

Поле `coverPath` (JSON-имя `coverPath`, значение `string | null`):

- `GET /courses` (каталог) — в каждом элементе;
- `GET /courses/{slug}` (публичная страница) — в объекте `course`;
- `GET /teacher/courses` и `GET /teacher/courses/{id}`;
- `GET /admin/courses` (список T48) и новый `GET /admin/courses/{id}` (T55).

Хранение: колонка `cover_path TEXT` в `courses` (миграция `007_cover.sql` —
продолжение нумерации goose). Раздача `/uploads/*` уже существует.

### T55 — `GET /admin/courses/{id}` — детали курса для админа

Только admin. Ответ `200`:

```json
{
  "course":  { "<элемент GET /admin/courses плюс coverPath>": null },
  "modules": [
    { "id": "uuid", "title": "Модуль", "position": 1,
      "lessons": [ { "id": "uuid", "title": "Урок", "kind": "video_text", "position": 1 } ] }
  ],
  "stats": { "studentsCount": 0, "activeCount": 0, "reviewsCount": 0,
    "avgRating": 0, "paymentsCount": 0, "paidTotalCents": 0 }
}
```

- `course.teacherEmail` — как в списке T25; `course` содержит `coverPath`;
- `modules` — сортировка по `position`, уроки внутри — по `position`; уроки
  отдаются ТОЛЬКО метаданные (id/title/kind/position), тела уроков нет;
- `stats.studentsCount` — все записи (любой статус); `activeCount` — только
  `active`; `paymentsCount`/`paidTotalCents` — только платежи `succeeded`;
- `avgRating`/`reviewsCount` — как в публичной карточке курса.
Ошибки: `401` — не вошёл; `403` — не админ; `404` — «Курс не найден».

Отзывы по курсу остаются прежними: `GET /admin/courses/{id}/reviews` и
`DELETE /admin/reviews/{id}` (T25) — без изменений.

### Новые ключи i18n (ru.json) — заморозка

Словарь дополняется ТОЛЬКО следующими ключами (фронтенд-агенты не добавляют
своих; не хватает — «писать в отчёт», не править словарь):

- `p7.constructor.*`: slugLabel, slugHint, slugInvalid, checklistTitle,
  checklistReady, checklistModule, checklistLesson, checklistLessonFields,
  deleteHint, deleteBlocked, unpublish, unpublishConfirm, unpublished;
- `p7.cover.*`: label, hint, upload, replace, remove, saved, removed,
  typeError, alt;
- `p7.adminCourse.*`: back, info, program, students, reviews, stats,
  studentsCount, lessonsCount, reviewsCount, paidTotal, openPublic,
  openStudents, notFound;
- `p7.stats.*`: quickLinks, linksUsers, linksCourses, linksEnrollments,
  linksJournal, linksQueue;
- `p7.enroll.*`: exportCsv, exportDone.

---

## Эндпоинты волны P8 (T63–T70) — ЗАМОРОЖЕНО до запуска агентов

Все пути ниже — от `/api/v1`. Коды те же: `400` — валидация (включая занятость
уникальных значений — правило T20), `403` — нет прав, `404` — нет сущности,
`409` — конфликт с целевым состоянием ресурса. Волна: промокоды (T63/T64),
категории и фильтры каталога (T65/T66), сертификаты (T67/T68), ответы
преподавателя на отзывы (T69/T70). Все поля в существующих ответах — только
аддитивные.

### T63 — промокоды: создание и применение

#### Модель PromoCode

```json
{ "id": "uuid", "courseId": "uuid", "code": "SALE20", "discountPct": 20,
  "maxUses": 100, "usedCount": 0, "expiresAt": "2026-12-31T23:59:59Z" | null,
  "active": true, "createdAt": "2026-10-19T12:00:00Z" }
```

`code` хранится и сравнивается в верхнем регистре (ввод нормализуется),
`maxUses = 0` — без ограничения. Миграция `008_promo_codes.sql`: таблица
`promo_codes` (id uuid PK, course_id FK → courses ON DELETE CASCADE,
code text, discount_pct int, max_uses int, used_count int, expires_at
timestamptz null, active bool, created_at timestamptz; UNIQUE (course_id, code)).

#### `POST /teacher/courses/{id}/promo-codes` (владелец/admin)

Тело: `{"code":"sale20","discountPct":20,"maxUses":100,"expiresAt":"2026-12-31T23:59:59Z"}`
— `maxUses` и `expiresAt` необязательны (по умолчанию 0 и null). Ответ `201`:
`{"promoCode": <PromoCode>}` (код в ответе — в верхнем регистре).

Валидация: `code` после trim+upper — `^[A-Z0-9_-]{3,32}$`, иначе
`400 «Код: 3–32 символа — латиница, цифры, дефис или подчёркивание»`;
`discountPct` — целое 1..90, иначе `400 «Скидка должна быть целым числом от 1
до 90»`; `maxUses` ≥ 0, иначе `400 «Лимит использований не может быть
отрицательным»`; `expiresAt` (если передан) — корректный RFC3339 в будущем,
иначе `400 «Срок действия должен быть в будущем»`.

Ошибки: `401`; `403` — чужой курс; `404` — «Курс не найден»;
`400 «Такой промокод уже существует на этом курсе»` (дубликат — правило
уникальности значений); `409 «Курс в архиве: промокоды недоступны»`.

#### `GET /teacher/courses/{id}/promo-codes` (владелец/admin)

`200`: `{"items":[<PromoCode>]}` — по `createdAt` убыванию. Права — как выше.

#### `DELETE /teacher/courses/{id}/promo-codes/{promoId}` (владелец/admin)

Деактивация: `active=false`, запись сохраняется (история). Ответ `204`;
повторный вызов — тоже `204` (идемпотентно). Ошибки: `403`;
`404 «Промокод не найден»`.

#### `POST /courses/{id}/promo/validate` (любой вошедший)

Тело: `{"code":"SALE20"}`. Ответ `200`:

```json
{ "code": "SALE20", "discountPct": 20, "priceCents": 10000, "finalPriceCents": 8000 }
```

`finalPriceCents = priceCents − ⌊priceCents·discountPct/100⌋` (к целым копейкам
вниз, в пользу студента). Ошибки: пустой `code` → `400 «Укажите промокод»`;
`404` — «Курс не найден»; `400 «Промокод не найден»` (нет такого на курсе);
`400 «Промокод отключён»`; `400 «Промокод истёк»`;
`400 «Лимит использований промокода исчерпан»`;
`400 «Курс бесплатный — промокод не нужен»`.

#### Расширение `POST /enrollments` (T63)

Необязательное поле `"promoCode": "SALE20"`. Если передано непустым — те же
проверки и тексты ошибок (`400`); `amountCents` = `finalPriceCents`; платёж
получает аддитивные поля `promoCode` (string) и `discountCents` (int, разница
с полной ценой). Сплит комиссии — от фактической суммы, как сейчас. Бесплатный
курс + `promoCode` → `400 «Курс бесплатный — промокод не нужен»`.

`usedCount` увеличивается ровно один раз — при первом переходе платежа в
`succeeded` (идемпотентность — существующая, по `provider_payment_id`).
При возврате `usedCount` не уменьшается.

#### Журнал платежей админа — аддитивные поля

`paymentJournalItem` получает `"promoCode": "string"` (пустая строка, если не
было) и `"discountCents": 0`. Форма строки не меняется.

### T65 — категории и фильтры каталога

#### Категории — фиксированный список

Слаги: `programming`, `design`, `data`, `marketing`, `ai`, `other`. Миграция
`009_course_category.sql`: `ALTER TABLE courses ADD COLUMN category TEXT NOT NULL
DEFAULT 'other';` (идемпотентно, как 007).

Поле `category` (string) аддитивно во ВСЕХ видах курса: `GET /courses`,
`GET /courses/{slug}`, teacher-списки, admin-списки.
`POST /teacher/courses` и `PATCH /teacher/courses/{id}` принимают необязательный
`category`; неизвестный слага → `400 «Неизвестная категория курса»`; без поля
при создании — `other`.

#### `GET /categories` (публичный)

`200`: `{"items":[{"slug":"programming","coursesCount":3}, …]}` — все шесть
слагов в фиксированном порядке (programming, design, data, marketing, ai,
other), `coursesCount` — число `published` курсов (может быть 0).

#### `GET /courses` — новые параметры

Дополнительно к `q/page/limit` (форма ответа не меняется):

- `category` — один из слагов выше; иное → `400 «Неизвестная категория»`;
- `price` — `free | paid`; иное → `400 «Недопустимый фильтр цены»`;
- `minRating` — `3.5 | 4 | 4.5` (принимаются `3.5`, `4`, `4.5`); иное →
  `400 «Недопустимый фильтр рейтинга»`; курсы с `ratingsCount = 0` в выдачу
  не попадают;
- `sort` — `popular | new | rating | price_asc | price_desc`, по умолчанию
  `popular`; иное → `400 «Недопустимая сортировка»`.

Семантика: `popular` — число `active`-записей по убыванию; `new` — `createdAt`
по убыванию; `rating` — `avgRating` по убыванию; `price_asc/price_desc` —
`priceCents`. Вторичный ключ всех сортировок — `createdAt` по убыванию. Выдача —
только `published` (как сейчас).

### T67 — сертификаты

Миграция `010_certificates.sql`: таблица `certificates` (id uuid PK,
user_id FK → users ON DELETE CASCADE, course_id FK → courses ON DELETE CASCADE,
number text UNIQUE, issued_at timestamptz; UNIQUE (user_id, course_id)).

`number` — `EDU-<ГГГГ>-<8 hex ВЕРХНИМ регистром>` (пример `EDU-2026-1A2B3C4D`).

Выдача: автоматически, когда прогресс ученика впервые достигает 100% (все
уроки курса завершены; у курса ≥ 1 урока). Идемпотентно — повторный вызов
и повторное достижение 100% второй сертификат не создают.

#### `GET /me/certificates` (вошедший)

`200`: `{"items":[{"number":"EDU-2026-1A2B3C4D","issuedAt":"…","courseId":"uuid","courseTitle":"…","courseSlug":"…"}]}`
— новые первыми.

#### Расширение `GET /me/courses` (T67)

В каждом элементе — аддитивное `"certificateNumber": "string"` (пустая строка,
если сертификата нет).

#### `GET /certificates/{number}` — публичный (без авторизации)

`200`:

```json
{ "number": "EDU-2026-1A2B3C4D", "issuedAt": "…", "studentName": "…",
  "courseTitle": "…", "courseSlug": "…" }
```

`studentName` — displayName студента (пустая строка, если не задан; e-mail
не отдаётся). Ошибки: `404 «Сертификат не найден»`.

### T69 — ответы преподавателя на отзывы + разбивка оценок

Миграция `011_review_reply.sql`: `reviews.teacher_reply TEXT NOT NULL DEFAULT ''`,
`reviews.teacher_reply_at TIMESTAMPTZ NULL` (идемпотентно).

Review во всех видах ответов получает аддитивные `"teacherReply": "string"`
(пустая строка, если нет) и `"teacherReplyAt": "string" | null`.

#### `PUT /teacher/courses/{id}/reviews/{reviewId}/reply` (владелец/admin)

Тело: `{"body":"Спасибо за отзыв!"}`. Ответ `200`: `{"review": <Review>}`.
Повторный PUT заменяет текст и обновляет `teacherReplyAt`. Валидация: после
trim 3..2000 символов, иначе `400 «Ответ: от 3 до 2000 символов»`.
Ошибки: `403` — чужой курс; `404 «Отзыв не найден»` (в том числе отзыв,
принадлежащий другому курсу).

#### `DELETE /teacher/courses/{id}/reviews/{reviewId}/reply` (владелец/admin)

`204` (ответа не было — тоже `204`). Ошибки: `403`, `404`.

#### `GET /courses/{id}/reviews` — расширение

Новый параметр `rating` (1..5); иное → `400 «Оценка должна быть числом
от 1 до 5»`. `items`/`total` — по фильтру; `avgRating`/`ratingsCount` — по
всем отзывам курса (не зависят от фильтра); аддитивно:

```json
"breakdown": { "1": 0, "2": 1, "3": 2, "4": 5, "5": 10 }
```

Ключи — строки `"1"`..`"5"`, значения — число отзывов с этой оценкой.

### Новые ключи i18n (ru.json) — заморозка P8

Словарь дополняется ТОЛЬКО этими ключами (тексты уже добавлены координатором;
фронтенд-агенты словарь не правят — не хватает ключа, «писать в отчёт»):

- `p8.promo.*`: title, hint, code, discount, maxUses, maxUsesHint, expiresAt,
  expiresHint, create, empty, active, inactive, expired, used, noLimit,
  turnOff, turnOffConfirm, turnedOff, created, codeInvalid, discountInvalid,
  maxUsesInvalid, expiresInvalid;
- `p8.checkout.*`: promoToggle, promoLabel, apply, applied, remove, buyFor;
- `p8.catalog.*`: filters, category, allCategories, price, priceAny, priceFree,
  pricePaid, rating, ratingAny, rating35, rating40, rating45, sort,
  sortPopular, sortNew, sortRating, sortPriceAsc, sortPriceDesc, reset, found,
  nothingFound, nothingFoundHint, filtersShow, filtersHide;
- `p8.cat.*`: programming, design, data, marketing, ai, other;
- `p8.cert.*`: pageTitle, awarded, student, course, issuedAt, number, copyLink,
  copied, notFound, notFoundHint, open, badge, done, toCourse;
- `p8.reviews.*`: ratings, all, reply, editReply, replyFor, replySaved,
  replyDelete, replyPlaceholder, replySave, replyCancel, teacher, emptyRating.

---

## Волна P9 (T71–T82) — ЗАМОРОЖЕНО до запуска агентов

Шесть фич: вопросы и ответы по урокам, избранное, превью-уроки, аналитика
учителя, похожие курсы, публичный профиль преподавателя. Все изменения
аддитивные: формы существующих ответов не меняются, кроме аддитивных
`isPreview` в уроке и `isFavorite` в видах курса (расширения T73/T75).
Миграции волны: `012_lesson_questions.sql`,
`013_favorites.sql`, `014_lesson_preview.sql`, `015_analytics_indexes.sql`.

### T71 — Вопросы и ответы по урокам

Модели (JSON-теги):

```
LessonQuestion {
  id, courseId, lessonId, lessonTitle,
  studentId, studentName, body, createdAt,
  answers: [LessonAnswer]        // старые первыми, может быть []
}
LessonAnswer {
  id, questionId, authorId, authorName,
  authorRole,                    // "student" | "teacher" | "admin"
  isTeacher,                     // true — автор курса или admin
  body, createdAt
}
```

`GET /api/v1/courses/{id}/questions?lessonId=&page=&limit=` — публичный.
`id` — UUID курса. Без параметров — все вопросы курса. `lessonId` не из этого
курса → `404 «Урок не найден в этом курсе»`. Неопубликованный курс для
не-владельца/не-админа → `404 «Курс не найден»`. Ответ:
`200 {"items":[LessonQuestion],"total":N,"page":1,"limit":20}`; вопросы — новые
первыми, ответы внутри — старые первыми. `page` по умолчанию 1, `limit` — 20
(максимум 100; нечисловые/вне диапазона → дефолт).

`POST /api/v1/courses/{id}/questions` — только вошедший. Тело
`{"lessonId":"…","body":"…"}`; `body` после trim 10..2000, иначе
`400 «Вопрос: от 10 до 2000 символов»`. Писать могут записанные на курс
(активная запись), владелец курса и admin; иначе
`403 «Задавать вопросы могут только записанные на курс»`. Урок не из курса →
`404 «Урок не найден в этом курсе»`. Ответ: `201 {"question": LessonQuestion}`
(с пустым `answers`).

`POST /api/v1/courses/{id}/questions/{questionId}/answers` — только вошедший.
Тело `{"body":"…"}`; trim 3..2000, иначе `400 «Ответ: от 3 до 2000 символов»`.
Отвечать могут записанные на курс, владелец курса и admin; иначе
`403 «Отвечать могут только записанные на курс»`. Вопрос не найден (в т.ч.
вопрос другого курса) → `404 «Вопрос не найден»`. Ответ:
`201 {"answer": LessonAnswer}`.

`PATCH /api/v1/courses/{id}/questions/{questionId}` — только автор вопроса.
Тело `{"body":"…"}`; trim 10..2000, иначе
`400 «Вопрос: от 10 до 2000 символов»`. Чужой текст (включая владельца курса и
admin) → `403 «Нет прав на редактирование»`; нет вопроса →
`404 «Вопрос не найден»`. Ответ: `200 {"question": LessonQuestion}`.

`PATCH /api/v1/courses/{id}/questions/{questionId}/answers/{answerId}` — только
автор ответа. Тело `{"body":"…"}`; trim 3..2000, иначе
`400 «Ответ: от 3 до 2000 символов»`; чужой ответ →
`403 «Нет прав на редактирование»`; нет ответа → `404 «Ответ не найден»`.
Ответ: `200 {"answer": LessonAnswer}`.

`DELETE /api/v1/courses/{id}/questions/{questionId}` — автор вопроса, владелец
курса или admin; иначе `403 «Нет прав на удаление»`; нет вопроса →
`404 «Вопрос не найден»`; успех — `204` (ответы каскадом).

`DELETE /api/v1/courses/{id}/questions/{questionId}/answers/{answerId}` — автор
ответа, владелец курса или admin; иначе `403 «Нет прав на удаление»`; нет
ответа → `404 «Ответ не найден»`; успех — `204`.

### T73 — Избранное

Таблица `favorites (user_id, course_id, created_at)`, PK `(user_id, course_id)`,
FK каскадом на users/courses.

- `PUT /api/v1/me/favorites/{courseId}` — только вошедший. Только published
  курс, иначе `404 «Курс не найден»`. Идемпотентно: `204`.
- `DELETE /api/v1/me/favorites/{courseId}` — только вошедший. Идемпотентно:
  `204` (в т.ч. если курса уже нет).
- `GET /api/v1/me/favorites` — только вошедший:
  `200 {"items":[CourseItem],"total":N}`; порядок — добавленные позже первыми;
  архивные/удалённые курсы в список не попадают.
- Расширение T73: публичные виды курса (`GET /courses`, `GET /courses/{slug}`,
  `GET /courses/{id}/similar`, `GET /teachers/{id}`, а также `GET /me/favorites`)
  отдают аддитивное `isFavorite` (bool). Для гостя — `false`; для вошедшего —
  по его избранному. `GET /me/favorites` всегда отдаёт `true` в элементах.
  Прежний способ через список `/me/favorites` остаётся рабочим.

### T75 — Превью-уроки

- `Lesson.isPreview` (`json:"isPreview"`) — аддитивное поле во всех видах
  урока, включая публичную страницу курса.
- `POST /teacher/courses/{id}/lessons` и
  `PATCH /teacher/courses/{id}/lessons/{lessonID}` принимают необязательное
  `isPreview` (bool); создание без поля — `false`, PATCH без поля не меняет.
- `GET /api/v1/courses/{slug}/preview/{lessonId}` — публичный. Курс должен
  существовать и быть published, урок — принадлежать курсу и иметь
  `isPreview=true`; иначе `404 «Курс не найден»` / `404 «Урок не найден»`.
  Ответ: `200 {"course":{"id","slug","title"},"lesson":<lessonView>}`, где
  `lessonView` — публичный вид урока (тест без правильных ответов, код без
  скрытых тестов), как на странице курса.

### T77 — Аналитика учителя

`GET /api/v1/teacher/stats` — teacher/admin. Teacher видит только свои курсы,
admin — все курсы платформы.

```
200 {
  "courses":    {"total","published","draft","review","archived"},
  "students":   {"total","unique"},
  "revenue":    {"paidTotalCents","platformFeeCents","teacherCents"},
  "completion": {"completed","avgPercent"},
  "ratings":    {"avgRating","ratingsCount"},
  "last30d":    {"enrollments","revenueCents"}
}
```

- `students.total` — активные записи; `unique` — уникальные ученики с активной
  записью; `completion.completed` — активные записи с прогрессом 100 %,
  `avgPercent` — средний прогресс активных записей (0..100, дробное);
- `revenue.*` — только `succeeded`-платежи: сумма `amountCents`,
  `platformFeeCents`, `teacherAmountCents`;
- `ratings` — по опубликованным курсам преподавателя (средневзвешенный
  `avgRating` и суммарное число оценок);
- `last30d` — активные записи, созданные за 30 дней, и succeeded-платежи за
  30 дней.

`GET /api/v1/teacher/stats/courses?limit=50` — teacher/admin:
`200 {"items":[…],"total":N}`, где элемент:
`{courseId,title,status,students,revenueCents,avgRating,ratingsCount,avgPercent}`.
Порядок — `revenueCents` desc, затем `students` desc, затем `title`;
`limit` по умолчанию 50 (максимум 200). Входят все курсы (любой статус).

### T79 — Похожие курсы

`GET /api/v1/courses/{id}/similar?limit=4` — публичный. Курс (UUID) должен
существовать и быть published, иначе `404 «Курс не найден»`. Возвращает
опубликованные курсы той же `category`, кроме самого курса, сортировка —
активные записи desc, затем `avgRating` desc, затем `createdAt` desc.
`limit` по умолчанию 4 (максимум 12). Расширение T79: если в категории не
осталось ни одного published-курса, подборка добирается курсами других
категорий (сначала всегда та же категория). Ответ:
`200 {"items":[CourseItem],"total":N}`; `items` пуст только если на платформе
нет других published-курсов.

### T81 — Публичный профиль преподавателя

`GET /api/v1/teachers/{id}` — публичный:

```
200 {
  "teacher": {"id","displayName","coursesCount","studentsCount",
              "avgRating","ratingsCount"},
  "courses": [CourseItem]
}
```

- 404 «Преподаватель не найден» — пользователя нет или роль не teacher/admin;
- `coursesCount` — число published-курсов; `studentsCount` — активные записи
  на published-курсы; `avgRating`/`ratingsCount` — по published-курсам
  (средневзвешенно);
- `courses` — все published-курсы преподавателя, порядок: активные записи desc,
  затем `createdAt` desc; без пагинации (максимум 100).

### Новые ключи i18n (ru.json) — заморожены для P9

- `p9.qa.*`: title, ask, askTitle, placeholder, send, answer, answerPlaceholder,
  answers, empty, emptyHint, loginToAsk, enrollToAsk, edit, edited,
  answerEdited, delete, deleteConfirm, deleted, sent, answerSent, notFound,
  teacherBadge, lessonLabel, allLessons, cancel, save, count;
- `p9.fav.*`: add, remove, added, removed, title, empty, emptyHint, goCatalog,
  loginHint;
- `p9.preview.*`: badge, watch, title, close, hint, toggle, toggleHint;
- `p9.stats.*`: title, courses, students, uniqueStudents, revenue, platformFee,
  net, completion, avgPercent, ratings, last30d, enrollments, perCourse, course,
  status, studentsCol, revenueCol, avgPercentCol, empty, open, completed,
  totalPaid, noCourses, coursesCount;
- `p9.similar.*`: title, all;
- `p9.teacher.*`: title, courses, students, rating, coursesCount, notFound,
  notFoundHint, allCourses, openProfile.

---

## Волна P10 (T83-T94) — супер-админ: модерация и управление

Все маршруты ниже требуют `requireAuth` + `requireRole(admin)` и дополнительно
проверяют `adminOnly` в обработчике. Ошибки авторизации: `401` — нет сессии,
`403` — не admin. Денежные суммы — в копейках; даты — RFC3339 UTC, если не
сказано иное. `items` — всегда массив, не `null`.

### T83 — `GET /admin/reviews` — глобальная модерация отзывов

Query:

| Параметр | Правило |
|---|---|
| `courseId` | UUID курса, иначе `400` |
| `rating` | целое 1..5, иначе `400` |
| `q` | поиск без учёта регистра по названию курса, email/имени ученика и тексту отзыва |
| `from`, `to` | даты `ГГГГ-ММ-ДД` по `createdAt`, включительно; неверный формат — `400`, `from > to` — `400` |
| `sort` | `createdAt` (по умолчанию) или `rating` |
| `dir` | `asc` или `desc` (по умолчанию `desc`) |
| `page`, `limit` | как в журнале платежей: `page` 1-based, `limit` 20 по умолчанию, максимум 100 |

Ответ `200`:

```json
{
  "items": [{
    "id": "uuid",
    "courseId": "uuid",
    "courseTitle": "Название",
    "courseSlug": "slug",
    "studentId": "uuid",
    "studentEmail": "student@example.test",
    "studentName": "Имя",
    "rating": 5,
    "body": "Текст отзыва",
    "createdAt": "2026-10-24T10:00:00Z",
    "teacherReply": "",
    "teacherReplyAt": null
  }],
  "total": 1,
  "page": 1,
  "limit": 20
}
```

Удаление — существующий `DELETE /admin/reviews/{id}` (`204`, `404`). После
удаления рейтинг курса пересчитывается. Отзывы без текста отдаются с `body: ""`.

### T85 — `GET /admin/questions` — глобальная модерация Q&A

Query: `courseId` (UUID), `lessonId` (UUID урока), `q` (поиск без учёта регистра
по вопросам, ответам, email/именам авторов, названиям курса и урока), `page`,
`limit` (20..100). Неверные UUID/пагинация — `400`.

Ответ `200`:

```json
{
  "items": [{
    "id": "uuid",
    "courseId": "uuid",
    "courseTitle": "Название",
    "courseSlug": "slug",
    "lessonId": "uuid",
    "lessonTitle": "Урок",
    "studentId": "uuid",
    "studentEmail": "student@example.test",
    "studentName": "Имя",
    "body": "Текст вопроса",
    "createdAt": "2026-10-24T10:00:00Z",
    "answers": [{
      "id": "uuid",
      "authorId": "uuid",
      "authorEmail": "author@example.test",
      "authorName": "Автор",
      "authorRole": "teacher",
      "body": "Текст ответа",
      "createdAt": "2026-10-24T10:05:00Z"
    }]
  }],
  "total": 1,
  "page": 1,
  "limit": 20
}
```

Сортировка: новые вопросы первыми (`createdAt desc, id desc`), ответы внутри —
старые первыми. Для удаления UI использует существующие
`DELETE /courses/{courseId}/questions/{questionId}` и
`DELETE /courses/{courseId}/questions/{questionId}/answers/{answerId}`:
ответы удаляются каскадом вместе с вопросом, `admin` имеет право удалять чужие
вопросы/ответы.

### T87 — редактирование курса супер-админом

Администратор может читать и изменять содержимое **любого неархивного** курса
через существующие `/teacher/courses/{id}` и вложенные CRUD-маршруты: карточка,
модули, уроки, квизы, codeTask, порядок, перенос урока. Для `teacher` правила
`draft`-guard не меняются; для `admin` проверка статуса `draft` заменяется на
`status != archived`. Архивный курс — `409` до `unarchive`.

Новый маршрут:

`POST /admin/courses/{id}/publish` → `200 {"course": {...}}`.
Публикует курс из `draft` или `review` без очереди модерации; `published` —
идемпотентный `200`; `archived` — `409`; нет курса — `404`. Комментарий
модерации очищается. Ответ — та же форма курса, что у `moderate`.

`GET /admin/courses/{id}` дополняется аддитивным полем
`course.moderationComment: string` (пустая строка, если комментария нет).

### T89 — глобальное управление промокодами

`GET /admin/promo-codes?courseId=&q=&active=&page=&limit=`.
`courseId` — UUID; `q` — код или название курса без учёта регистра; `active` —
`true|false`; пагинация 20..100. Ответ `200`:

```json
{
  "items": [{
    "id": "uuid",
    "courseId": "uuid",
    "courseTitle": "Название",
    "courseSlug": "slug",
    "code": "SALE20",
    "discountPct": 20,
    "maxUses": 100,
    "usedCount": 3,
    "expiresAt": null,
    "active": true,
    "createdAt": "2026-10-24T10:00:00Z"
  }],
  "total": 1,
  "page": 1,
  "limit": 20
}
```

`PATCH /admin/promo-codes/{id}` — частичное обновление; тело содержит хотя бы
одно поле:

```json
{"discountPct": 25, "maxUses": 200, "expiresAt": "2026-12-31T20:59:59Z", "active": false}
```

Правила: `discountPct` 1..90; `maxUses >= 0` и `>= usedCount`; `expiresAt` —
RFC3339 в будущем или `null` (без срока); `active` — bool. Неизвестное поле —
`400`; нет промокода — `404`; успех — `200 {"promoCode": <элемент списка>}`.
Изменение `active=false` не удаляет код; `active=true` реактивирует, если срок
не истёк. Архивный курс менять нельзя — `409`.

### T93 — принудительный выход пользователя

`GET /admin/users/{id}/sessions` → `200`:

```json
{"items": [{"id": "session-id", "createdAt": "...", "expiresAt": "..."}]}
```

Сессии сортируются новыми первыми; истёкшие не возвращаются. Нет пользователя —
`404`.

`DELETE /admin/users/{id}/sessions` → `204` без тела: удаляет все сессии
пользователя (включая текущую, если админ завершает свои сессии). Нет
пользователя — `404`. После удаления старые cookie перестают проходить
`requireAuth`.

### Новые ключи i18n (ru.json) — заморожены для P10

- `p10.reviews.*`: title, subtitle, search, filterCourse, filterRating,
  allRatings, course, student, body, delete, confirmDelete, deleted, empty,
  openCourse, periodFrom, periodTo, reset;
- `p10.qa.*`: title, subtitle, search, filterCourse, filterLesson, allCourses,
  allLessons, lesson, question, answers, deleteQuestion, deleteAnswer,
  confirmQuestion, confirmAnswer, deleted, empty, openCourse, author, student,
  created, expand, collapse;
- `p10.promo.*`: title, subtitle, search, filterCourse, filterStatus, all,
  active, inactive, colCode, colCourse, colDiscount, colUses, colExpires,
  colStatus, colActions, edit, save, cancel, deactivate, activate,
  confirmDeactivate, confirmActivate, updated, empty, never, usedOf, editTitle,
  openCourse;
- `p10.courseAdmin.*`: edit, editHint, adminMode, adminModeHint, actions,
  moderate, approve, reject, rejectComment, forcePublish, forcePublishConfirm,
  forcePublishDone, openPublic, students, backToAdmin, contentEditable,
  contentLockedArchived;
- `p10.sessions.*`: title, hint, empty, createdAt, expiresAt, revokeAll,
  confirmRevokeAll, revoked, count, forceLogout;
- `p10.nav.*`: reviews, qa, promos, aria, backToAdmin.

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
