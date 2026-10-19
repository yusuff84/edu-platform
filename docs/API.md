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
| GET | `/api/v1/courses?q=&page=&limit=` | публичный | → `200` конверт `{items:[CourseItem],total,page,limit}` |
| GET | `/api/v1/courses/{slug}` | публичный* | → `200` `{course, modules:[ModuleView]}`; *`draft` виден автору и админу, остальным 404 |
| GET | `/api/v1/courses/{id}/reviews` | публичный | `{id}` — UUID или slug → `200` `{items:[Review],total,avgRating,ratingsCount}` |
| POST | `/api/v1/courses/{id}/reviews` | студент с активной записью | `{rating:1..5, body}` → `201` `Review` |

### Прогресс обучения (`me`)

| Метод | Путь | Доступ | Тело / ответ |
|---|---|---|---|
| GET | `/api/v1/me/courses` | cookie | → `200` `{items:[{course,percent,nextLesson}],total}` |
| POST | `/api/v1/me/courses/{id}/lessons/{lessonId}/complete` | активная запись | `{quizAnswers?, code?, language?}` → `200 {passed,completed,percent,nextLesson}` или `{passed:false,message}`; для code-задач `502` при недоступном runner |

### Преподаватель (`teacher` — роли teacher/admin)

| Метод | Путь | Тело / ответ |
|---|---|---|
| POST | `/api/v1/teacher/courses` | `{title, description, priceCents, slug?, currency?}` → `201` Course |
| GET | `/api/v1/teacher/courses` | → `200` `{items:[Course],total}` |
| POST | `/api/v1/teacher/courses/{id}/modules` | `{title}` → `201` Module |
| POST | `/api/v1/teacher/courses/{id}/lessons` | по `kind`: `{moduleId,title,kind,bodyMd?,videoUrl?,liveUrl?,startsAt?,quiz?,codeTask?}` → `201` Lesson |
| POST | `/api/v1/teacher/courses/{id}/submit` | пустое → `200` Course (`status:review`) |

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
| GET | `/api/v1/admin/settings` | → `200 {commissionPct}` (US6, T07) |
| PUT | `/api/v1/admin/settings` | `{commissionPct:0..90}` → `200 {commissionPct}` (US6, T07) |

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
