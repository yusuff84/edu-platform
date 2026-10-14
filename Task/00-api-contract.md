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

Ошибки: `400` (валидация, невалидный invite-токен), `409` (email занят).

---

## 2. `POST /auth/login`

Тело: `{ "email": "string", "password": "string" }`
Ответ `200`: та же форма `User`, что в п.1. Устанавливает cookie `session_id`.
Ошибки: `400`, `401`.

---

## 3. `POST /auth/logout`

Тело пустое. Ответ `204 No Content` (не `200`, не JSON).

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

Провал:
```json
{ "passed": false, "completed": false, "message": "текст ошибки" }
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

Ответ `200`, текст `ok`.

---

## Новые эндпоинты US6 (реализуются в T07, потребляются в T08)

### `GET /admin/payments?page=&limit=&status=&courseId=`
Только роль `admin`.
```json
{
  "items": [
    { "id": "uuid", "studentId": "uuid", "courseId": "uuid", "courseTitle": "string",
      "amountCents": 4900, "platformFeeCents": 735, "teacherAmountCents": 4165,
      "currency": "RUB", "provider": "mock", "providerPaymentId": "string",
      "status": "pending|succeeded|failed|refunded",
      "createdAt": "RFC3339", "processedAt": "RFC3339" | null }
  ],
  "total": 42, "page": 1, "limit": 20,
  "totals": { "amountCents": 123400, "platformFeeCents": 18510, "teacherAmountCents": 104890 }
}
```

### `GET /admin/settings`
```json
{ "commissionPct": 15 }
```

### `PUT /admin/settings`
Тело: `{ "commissionPct": 15 }` (`0..90`)
Ответ `200`: `{ "commissionPct": 15 }`

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
| 2 | `POST /auth/logout` возвращает `200 {"status":"ok"}`, а не `204` | backend | ✅ закрыто T10 | Клиент обрабатывает оба варианта; не полагаться на `204` |
| 3 | `listReviews` теряет `avgRating`/`ratingsCount` из конверта (клиент возвращает плоский массив) | T01 → T04 | ⚠️ осознанно | **Рейтинг курса бери из `getCourse()`** (`course.avgRating`, `course.ratingsCount`), а не из `listReviews()` |
| 4 | `POST /teacher/courses/{id}/lessons` принимает `startsAt`, но **не сохраняет** его в урок | backend | ❌ не закрыто | Преподаватель не может запланировать эфир. Ничего не подстраивай во фронтенде — шли по контракту и опиши в отчёте |
| 5 | `GET /healthz` возвращает JSON `{"status":"ok"}`, а не текст `ok` | backend | ⚠️ так и осталось | Сверяйся с `docs/API.md`, он описывает фактическое поведение |
| 6 | `GET /admin/payments` отдаёт только `studentId`, без email/имени | backend | ❌ не закрыто | В админке колонка «Студент» показывает обрезанный UUID. Требует расширения контракта, T08 не подстраивал |
| 7 | `formatPrice(0)` возвращает «Бесплатно» — неверно для итогов журнала | frontend | ⚠️ обойдено | T08 использовал локальный форматтер `money()`; `client.ts` не менялся |

Расхождение 4 — реальный баг бэкенда (`handler/teacher.go`: поле не копируется в `repo.Lesson`).
Если заметишь похожее — пиши в отчёте, не правь чужие файлы.

## Что НЕ делать

- ❌ Не добавлять поля в запросы, которых нет в этом документе (`DisallowUnknownFields` → 400).
- ❌ Не переименовывать ключи ответов «для красоты».
- ❌ Не отправлять `JSON.stringify({...x, optional: undefined})` — поле уйдёт как `null`; собирай объект явно.
- ❌ Не полагаться на порядок полей — используй точные имена.
