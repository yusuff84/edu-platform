# API edu-platform — сводка для разработчиков

Машино-читаемый контракт: [`specs/001-it-courses-mvp/contracts/openapi.yaml`](../specs/001-it-courses-mvp/contracts/openapi.yaml)
(OpenAPI 3.0). ⚠️ **Он может отставать от кода**: эндпоинты US6
(`/admin/payments`, `/admin/settings`, T07), mock-страница оплаты (`/pay/mock/{id}`, T09)
и маршруты T13 в него не внесены. Актуальные формы запросов зафиксированы также
в `Task/00-api-contract.md` — при расхождении правь клиент по нему.

## Общие правила

| Правило | Значение |
|---|---|
| Базовый URL | `/api/v1` (nginx и vite-прокси ведут его на backend; в compose backend слушает `:8080`, наружу — `localhost:8082`) |
| Аутентификация | cookie `session_id`, httpOnly, подпись HMAC-SHA256 (`SESSION_SECRET`), `SameSite` настраивается (`COOKIE_SAMESITE`, по умолчанию `lax`). Клиент шлёт `credentials: 'include'` |
| Формат тела | `application/json`; ошибка — `{"error": "<текст по-русски>"}` |
| Строгость JSON | `DisallowUnknownFields`: **любое лишнее поле = 400**. Отправляйте ровно перечисленные поля |
| Ограничение тела | 1 МБ |
| Пагинация | `page` (с 1), `limit` (20 по умолчанию, максимум 100) |
| Rate-limit | 10/мин на IP для `/auth/login|register`, 300/мин глобально, 120/мин для вебхуков (настраиваются env); при превышении `429` + `Retry-After` |
| CSRF | не-GET запросы требуют `Origin`/`Referer` из `ALLOWED_ORIGINS`; вебхуки провайдеров проверяются подписью и исключены из проверки |

Формы моделей (`User`, `Course`, `CourseItem`, `LessonView`, `ModuleView`,
конверты `{items,total}`) — в `Task/00-api-contract.md`, пп. 5–20.

## Эндпоинты по группам

### Здоровье

| Метод | Путь | Доступ | Ответ |
|---|---|---|---|
| GET | `/healthz` | публичный | `200 {"status":"ok"}` |
| GET | `/api/v1/healthz` | публичный | `200 {"status":"ok"}` |

### Auth

| Метод | Путь | Доступ | Тело / ответ |
|---|---|---|---|
| POST | `/api/v1/auth/register` | публичный | `{email, password, inviteToken?}` → `201` `User` (`inviteToken` — регистрация преподавателя) |
| POST | `/api/v1/auth/login` | публичный | `{email, password}` → `200` `User` + cookie `session_id` |
| POST | `/api/v1/auth/logout` | публичный | → `200 {"status":"ok"}` (клиент также принимает `204`) |
| GET | `/api/v1/auth/me` | cookie | → `200` `User` |
| GET | `/api/v1/auth/yandex/start` | публичный | ⏳ **не подключён** (клиент T10 готов, маршрут добавляет T13): редирект на Яндекс OAuth |
| GET | `/api/v1/auth/yandex/callback` | публичный | ⏳ **не подключён**: обмен code → сессия |

### Курсы и отзывы

| Метод | Путь | Доступ | Тело / ответ |
|---|---|---|---|
| GET | `/api/v1/courses?q=&category=&price=&minRating=&sort=&page=&limit=` | публичный | → `200` конверт `{items:[CourseItem],total,page,limit}` |
| GET | `/api/v1/categories` | публичный | → `200` `{items:[{slug,coursesCount}]}` — шесть категорий в фиксированном порядке: `programming, design, data, marketing, ai, other`; `coursesCount` — только `published` (T65) |
| GET | `/api/v1/courses/{slug}` | публичный* | → `200` `{course, modules:[ModuleView]}`; *`draft` виден автору и админу, остальным 404 |
| GET | `/api/v1/courses/{id}/reviews?rating=1..5` | публичный | `{id}` — UUID или slug; `rating` необязателен и фильтрует `items`/`total` → `200` `{items:[Review],total,avgRating,ratingsCount,breakdown:{"1".."5"}}`; `avgRating`/`ratingsCount`/`breakdown` — по всем отзывам курса; иной `rating` → `400 «Оценка должна быть числом от 1 до 5»` |
| POST | `/api/v1/courses/{id}/reviews` | студент с активной записью | `{rating:1..5, body}` → `201` `Review` |
| GET | `/api/v1/courses/{id}/questions?lessonId=&page=&limit=` | публичный* | → `200` `{items:[LessonQuestion],total,page,limit}`; вопросы новые первыми, ответы внутри — старые первыми; *неопубликованный курс виден только владельцу/админу, остальным `404 «Курс не найден»`; `lessonId` не из курса → `404 «Урок не найден в этом курсе»` |
| GET | `/api/v1/certificates/{number}` | публичный | → `200` `{number,issuedAt,studentName,courseTitle,courseSlug}`; `studentName` — displayName студента (e-mail не отдаётся); `404 «Сертификат не найден»` |

> Волна P8 (T65): `category` — слаг из `programming`, `design`, `data`,
> `marketing`, `ai`, `other`. Необязателен при создании курса (по умолчанию
> `other`), в `PATCH /api/v1/teacher/courses/{id}` без поля не меняется,
> неизвестное или пустое значение — `400 «Неизвестная категория курса»`.
> Поле `category` добавлено во все виды курса: каталог, страница курса,
> списки и деталь teacher/admin. Серверные фильтры `GET /courses`:
> `price=free|paid`, `minRating=3.5|4|4.5`, а также `sort` со значениями
> `popular`, `new`, `rating`, `price_asc`, `price_desc` (по умолчанию
> `popular`, вторичный ключ — `createdAt` desc); фильтры комбинируются с
> `q/page/limit`, `total` считается до пагинации. Ошибки: `400 «Неизвестная
> категория»`, «Недопустимый фильтр цены», «Недопустимый фильтр рейтинга»,
> «Недопустимая сортировка».

### Настройки интерфейса (публичные)

| Метод | Путь | Доступ | Тело / ответ |
|---|---|---|---|
| GET | `/api/v1/settings/public` | публичный | → `200 {promoBarEnabled}` — пост-P7: тогл промо-баннера, читается шапкой без входа; приватные настройки сюда не попадают |

### Прогресс обучения (`me`)

| Метод | Путь | Доступ | Тело / ответ |
|---|---|---|---|
| GET | `/api/v1/me/courses` | cookie | → `200` `{items:[{course,percent,nextLesson,certificateNumber}],total}`; `certificateNumber` — пустая строка, если сертификата нет |
| GET | `/api/v1/me/certificates` | cookie | → `200` `{items:[{number,issuedAt,courseId,courseTitle,courseSlug}]}` — новые первыми; сертификат выдаётся автоматически при первом достижении 100% прогресса (все уроки курса завершены) |
| POST | `/api/v1/me/courses/{id}/lessons/{lessonId}/complete` | активная запись | `{quizAnswers?, code?, language?}` → `200 {passed,completed,percent,nextLesson}` или `{passed:false,message}`; для code-задач `502` при недоступном runner |

### Преподаватель (`teacher` — роли teacher/admin)

| Метод | Путь | Тело / ответ |
|---|---|---|
| POST | `/api/v1/teacher/courses` | `{title, description, priceCents, category?, slug?, currency?}` → `201` Course |
| GET | `/api/v1/teacher/courses` | → `200` `{items:[Course],total}` |
| POST | `/api/v1/teacher/courses/{id}/modules` | `{title}` → `201` Module |
| POST | `/api/v1/teacher/courses/{id}/lessons` | по `kind`: `{moduleId,title,kind,bodyMd?,videoUrl?,liveUrl?,startsAt?,quiz?,codeTask?}` → `201` Lesson |
| POST | `/api/v1/teacher/courses/{id}/submit` | пустое → `200` Course (`status:review`) |

> Волна P7 (T53): `POST /api/v1/teacher/courses/{id}/unpublish` — тело
> не требуется, `published → draft`, ответ `200` `{course}`; `409 «Снять с
> публикации можно только опубликованный курс»`, `403` — чужой курс, `404` —
> курса нет. PATCH урока принимает `startsAt` в RFC3339 **или** datetime-local
> `ГГГГ-ММ-ДДТЧЧ:ММ` (строка без зоны трактуется как UTC; мусор — `400`).
> Тест (`kind:"quiz"`) валидируется на бэкенде: текст вопроса после TrimSpace,
> не менее двух непустых вариантов, правильный ответ (`correct` или
> `correctIndex`) указан и попадает в границы списка, иначе
> `400 «Некорректный тест: …»`.

> Волна P7 (T54): обложка курса — `POST /api/v1/teacher/courses/{id}/cover`
> (multipart, поле `file`; тип по байтам через `http.DetectContentType`,
> только JPEG/PNG/WebP, ≤5 МБ) → `201 {"coverPath":"/uploads/covers/<имя>"}`,
> повторная загрузка перезаписывает файл; `DELETE …/cover` → `204` сбрасывает
> `cover_path` (отсутствие обложки — тот же `204`). Ошибки: `404` нет курса,
> `409` «Курс в архиве: обложка заморожена», `400` «Обложка: допустимы JPEG,
> PNG или WebP не больше 5 МБ». Поле `coverPath` (string|null) добавлено во
> все виды курса: каталог `GET /courses`, страница `GET /courses/{slug}`,
> списки учителя и админа.

> Волна P8 (T69): ответ преподавателя на отзыв —
> `PUT /api/v1/teacher/courses/{id}/reviews/{reviewId}/reply`
> (`{"body":"…"}` после trim 3..2000 символов → `200 {"review":<Review>}`;
> повторный PUT заменяет текст и обновляет `teacherReplyAt`) и
> `DELETE …/reply` → `204` (ответа не было — тоже `204`). Ошибки: `403` —
> чужой курс, `404 «Отзыв не найден»` (в т.ч. отзыв другого курса),
> `400 «Ответ: от 3 до 2000 символов»`. В `Review` добавлены аддитивные
> `teacherReply` (string, пустая — ответа нет) и `teacherReplyAt`
> (string|null); они есть во всех видах отзывов (страница курса,
> админ-список отзывов).

> Волна P8 (T63): промокоды. `POST /api/v1/teacher/courses/{id}/promo-codes`
> `{code, discountPct:1..90, maxUses?, expiresAt?}` → `201 {"promoCode": PromoCode}`
> (код нормализуется к верхнему регистру; дубликат — `400`, архивный курс — `409`);
> `GET …/promo-codes` → `200 {"items":[PromoCode]}` (новые первыми);
> `DELETE …/promo-codes/{promoId}` → `204` (деактивация `active=false`,
> повторный вызов — тоже `204`). `POST /api/v1/courses/{id}/promo/validate`
> (только вошедший) `{code}` → `200 {code,discountPct,priceCents,finalPriceCents}`,
> скидка округляется вниз до копеек; ошибки применения кода — `400` с текстами
> контракта, бесплатный курс — `400`. `POST /api/v1/enrollments` принимает
> необязательный `promoCode`: `amountCents` — со скидкой, в платеже и журнале
> админа появляются аддитивные `promoCode` и `discountCents`, `usedCount`
> увеличивается один раз при успешной оплате.

> Волна P9 (T71): вопросы и ответы по урокам.
> `GET /api/v1/courses/{id}/questions` — публичная лента курса (`id` — UUID):
> `{items:[LessonQuestion],total,page,limit}`, `page` по умолчанию 1,
> `limit` — 20 (максимум 100, нечисловые/вне диапазона — дефолт);
> `LessonQuestion` = `{id,courseId,lessonId,lessonTitle,studentId,studentName,
> body,createdAt,answers:[LessonAnswer]}` (вопросы новые первыми, ответы —
> старые первыми, `answers` всегда массив).
> `POST /api/v1/courses/{id}/questions` (cookie) `{lessonId, body}` после trim
> 10..2000 → `201 {"question": LessonQuestion}`; `POST
> /api/v1/courses/{id}/questions/{questionId}/answers` (cookie) `{body}` после
> trim 3..2000 → `201 {"answer": LessonAnswer}`. Писать могут записанные на
> курс, владелец курса и admin; иначе `403 «Задавать вопросы могут только
> записанные на курс»` / `403 «Отвечать могут только записанные на курс»`.
> `LessonAnswer` = `{id,questionId,authorId,authorName,authorRole,isTeacher,
> body,createdAt}`, где `authorRole` — `student|teacher|admin`, `isTeacher` —
> автор является преподавателем курса или admin.
> `DELETE …/questions/{questionId}` и `DELETE
> …/questions/{questionId}/answers/{answerId}` — автор, владелец курса или
> admin, иначе `403 «Нет прав на удаление»`; успех — `204` (ответы вопроса
> удаляются каскадом). Ошибки: `404 «Курс не найден»`, `404 «Урок не найден в
> этом курсе»`, `404 «Вопрос не найден»` (в т.ч. вопрос другого курса),
> `404 «Ответ не найден»`, `400 «Вопрос: от 10 до 2000 символов»`,
> `400 «Ответ: от 3 до 2000 символов»`.

> В бэклоге T13: `GET/PATCH/DELETE /teacher/courses[...]`, PATCH модулей/уроков,
> reorder — в `Task/T13-integration-wiring.md`.

### Админ (`admin` — только роль admin)

| Метод | Путь | Тело / ответ |
|---|---|---|
| POST | `/api/v1/admin/invites` | `{email}` → `201 {id,email,token,tokenPrefix,expiresAt}`; **токен показывается один раз** |
| GET | `/api/v1/admin/courses/review` | → `200` `{items:[Course],total}` |
| POST | `/api/v1/admin/courses/{id}/moderate` | `{decision:"approve\|reject", comment?}` → `200` Course; `comment` обязателен при `reject` |
| GET | `/api/v1/admin/courses/{id}` | → `200` `{course:<элемент GET /admin/courses + coverPath>, modules:[{id,title,position,lessons:[{id,title,kind,position}]}], stats:{studentsCount,activeCount,reviewsCount,avgRating,paymentsCount,paidTotalCents}}` (T55; `404` «Курс не найден») |
| GET | `/api/v1/admin/payments?page=&limit=&status=&courseId=` | → `200` `{items:[PaymentJournalItem],total,page,limit,totals}` (US6, T07) |
| GET | `/api/v1/admin/settings` | → `200 {commissionPct,promoBarEnabled}` (US6, T07; промо — пост-P7) |
| PUT | `/api/v1/admin/settings` | `{commissionPct?0..90, promoBarEnabled?}` — хотя бы одно поле → `200 {commissionPct,promoBarEnabled}` (US6, T07; частичное обновление — пост-P7) |

### Оплата

| Метод | Путь | Доступ | Тело / ответ |
|---|---|---|---|
| POST | `/api/v1/enrollments` | cookie | `{courseId, provider:"mock\|stripe\|yookassa"}` → `201 {paymentId,providerPaymentId,redirectUrl,amountCents,status,enrollmentActive}`; бесплатный курс сразу `succeeded` + `enrollmentActive:true` |
| POST | `/api/v1/payments/webhook/{provider}` | подпись провайдера (без cookie) | провайдер-специфичное тело → `200 {status:"processed\|duplicate\|ignored"}`; Stripe — `Stripe-Signature`, YooKassa — IP из `YOOKASSA_TRUSTED_IPS`, mock — опционально `X-Mock-Signature` |
| GET | `/pay/mock/{id}` | публичный | HTML-страница подтверждения mock-оплаты (не под `/api/v1`; подключает T13) |

## Частые коды ошибок

| Код | Причина | Что делать |
|---|---|---|
| `400` | Невалидное тело: неизвестное поле (`DisallowUnknownFields`), неверный тип, ошибка валидации (`rating` вне 1..5, `commissionPct` вне 0..90, `quiz` без ответов, отсутствует `comment` при `reject` и т.п.) | Отправить ровно поля из контракта; не добавлять «лишних» через спред; проверить обязательные поля по `kind` урока |
| `401` | Нет сессии, истёкшая/подделанная подпись cookie, неверный пароль, неверная подпись вебхука | Перелогиниться; для вебхука — проверить секрет провайдера |
| `403` | Недостаточно прав (роль), нет активной записи на курс, запрос с недопустимого `Origin` (CSRF) | Проверить роль и запись; для локальных скриптов — `CSRF_ALLOW_MISSING_ORIGIN=true` |
| `404` | Сущности нет, либо она скрыта (черновик чужого курса) | Проверить идентификатор (slug в каталоге, UUID в остальном); не считать 404 «курс удалён» — он может быть draft |
| `409` | Конфликт состояния: email занят, курс уже куплен, отзыв уже оставлен, платёж уже создан | Показать «уже есть»; при повторной покупке pending-платежа клиент получит `200` с `redirectUrl` для продолжения |
| `429` | Rate-limit по IP (auth 10/мин, глобально 300/мин, вебхуки 120/мин) | Подождать `Retry-After`; не ретраить в цикле |
| `500` | Внутренняя ошибка сервера (БД и т.п.) | Логи backend; текст наружу не раскрывает детали |
| `502` | Внешний сервис недоступен: `PAYMENTS_PROVIDER` не настроен (`ErrNotConfigured`), API оплаты недоступен, runner не ответил | Для оплаты — проверить env-ключи провайдера; для runner — `docker compose ps`, `RUNNER_URL` |
