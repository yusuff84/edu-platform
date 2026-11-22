# API edu-platform — сводка для разработчиков

Подробный контракт по разделам — [`api-contract.md`](api-contract.md); здесь — сводка
эндпоинтов, форм запросов/ответов и кодов ошибок. При расхождении источник истины — код и
контрактные тесты.

## Общие правила

### Безопасность аккаунта (T83)

| Метод | Путь | Доступ и результат |
|---|---|---|
| POST | `/auth/password/forgot` | `{email}` → 202 `{message}` с одинаковым текстом для существующего/неизвестного/заблокированного аккаунта и cooldown. Невалидный email — 400, почта отключена — 503 |
| POST | `/auth/password/reset` | `{token,password}` → 200 `{message}`, все сессии и токены аккаунта отозваны. Неверный/истёкший/использованный токен — 400 |
| POST | `/auth/email/verify` | `{token}` → 200 `{message}`; неверная ссылка — 400. GET ничего не подтверждает |
| GET | `/me/security` | cookie → `{email,emailVerified,emailVerifiedAt,emailDeliveryAvailable}` |
| POST | `/me/email/verification` | cookie → 202 `{message}`; уже подтверждён — 200; cooldown — 429 с `Retry-After:60`; почта недоступна — 503 |
| GET | `/me/sessions` | cookie → `{items:[{id,current,userAgent,ipAddress,createdAt,lastSeenAt,expiresAt}]}` |
| DELETE | `/me/sessions/{sessionID}` | cookie → 204, только своя сессия. Чужой/неизвестный id — 404. Для текущей удаляется cookie |
| POST | `/me/sessions/revoke-others` | cookie → 204, текущая сессия сохраняется |

В auth-response (`register`, `login`, `me`) добавлен `emailVerified:boolean`.
Подтверждение адреса не блокирует существующий доступ к курсам. Сброс действует
30 минут, подтверждение — 24 часа; новая ссылка заменяет предыдущую того же
назначения. Только SHA256 токена сохраняется в БД. Все меняющие методы
используют существующую Origin/CSRF-защиту, публичные recovery-методы — auth
rate-limit. При сбросе пароль и отзыв сессий меняются одной транзакцией.
Пароль — минимум 8 символов, максимум 72 UTF-8 байта (bcrypt).

`id` списка сессий — отдельный management ID, а не `session_id` cookie.
Metadata сессии — подсказка из запроса, а не доказательство устройства.
Секретные ссылки передаются в email как `/reset-password#token=...` и
`/verify-email#token=...`; токен не попадает в URL HTTP-запроса и access log.
Без SMTP ссылки не выдаются через API. Примеры настройки и локальный SMTP —
[api-contract.md (раздел T83)](api-contract.md).

Изменения после ревью 17.11.2026:

- `GET /courses/{slug}` выдаёт обычные платные уроки только активному покупателю,
  владельцу и админу. Без покупки остаются метаданные программы и разрешённое
  превью; материалы бесплатных курсов доступны публично. Покупатель может
  продолжать обучение после archive/unpublish.
- Повтор `POST /enrollments` использует сохранённую попытку, исходные параметры,
  стабильный ключ провайдера и сохранённый redirectUrl. У старого pending без
  ссылки возможен явный `409`; новый платёж молча не создаётся.
- Язык `/complete` определяется заданием; несовпадающий language даёт `400`.
  Ошибка сохранения прогресса даёт `500`, а не `completed:true`.
- `DELETE /teacher/courses/{id}` требует draft без платежей и записей.
  Снятие с публикации не разрешает удалить финансовую историю: в этом случае
  возвращается `409`, вместо удаления используется archive.
- Duplicate succeeded не снимает ручной revoke; старый refund не отзывает новую
  успешную покупку. Завершённый refund даёт `409` при повторе; после частичного
  локального сбоя повтор может восстановить отзыв доступа и вернуть `200`.
- Платёжные операции используют общий PostgreSQL lock и транзакционное применение
  статуса/доступа/usedCount. Pending/неопределённые попытки резервируют лимит
  промокода; безопасное освобождение требует подтверждённой отмены провайдера.
- `POST /admin/payments/{id}/sync` (admin) сверяет платёж с провайдером и
  восстанавливает незавершённое присоединение checkout по сохранённому ключу.
  Ответ: `{payment: PaymentJournalItem}`; неизвестный id — `404`, ошибка
  провайдера — `502`, ошибка локальной записи — `500`.
- HTTP-успех создания возврата сам по себе не ставит локальный `refunded`:
  требуется `status=succeeded`. YooKassa refund webhook использует payment_id
  и статус из подтверждающего ответа API, а не из присланного тела события.

| Правило | Значение |
|---|---|
| Базовый URL | `/api/v1` (nginx и vite-прокси ведут его на backend; в compose backend слушает `:8080`, наружу — `localhost:8082`) |
| Аутентификация | cookie `session_id`, httpOnly, подпись HMAC-SHA256 (`SESSION_SECRET`), `SameSite` настраивается (`COOKIE_SAMESITE`, по умолчанию `lax`). Клиент шлёт `credentials: 'include'` |
| Формат тела | `application/json`; ошибка — `{"error": "<текст по-русски>"}` |
| Строгость JSON | `DisallowUnknownFields`: **любое лишнее поле = 400**. Отправляйте ровно перечисленные поля |
| Ограничение тела | 1 МБ для JSON; обложка курса — до 5 МБ (multipart) |
| Пагинация | `page` (с 1), `limit` (20 по умолчанию, максимум 100; исключение — `/teacher/stats/courses`, до 200) |
| Rate-limit | 10/мин на IP для `/auth/login|register`, 300/мин глобально, 120/мин для вебхуков (настраиваются env); при превышении `429` + `Retry-After` |
| CSRF | не-GET запросы требуют `Origin`/`Referer` из `ALLOWED_ORIGINS`; вебхуки провайдеров проверяются подписью и исключены из проверки |

Формы моделей (`User`, `Course`, `CourseItem`, `LessonView`, `ModuleView`,
конверты `{items,total}`) — в `api-contract.md`, пп. 5–20.

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
| GET | `/api/v1/auth/yandex/start` | публичный | `302` на Яндекс OAuth + одноразовый `oauth_state`; без `YANDEX_CLIENT_ID` — `503 «Вход через Яндекс не настроен»` |
| GET | `/api/v1/auth/yandex/callback` | публичный | сверяет `state`, обменивает `code`, находит/создаёт пользователя и открывает сессию; ошибки `400`/`403` (блокировка)/`502` |

### Курсы и отзывы

| Метод | Путь | Доступ | Тело / ответ |
|---|---|---|---|
| GET | `/api/v1/courses?q=&category=&price=&minRating=&sort=&page=&limit=` | публичный | → `200` конверт `{items:[CourseItem],total,page,limit}` |
| GET | `/api/v1/categories` | публичный | → `200` `{items:[{slug,coursesCount}]}` — шесть категорий в фиксированном порядке: `programming, design, data, marketing, ai, other`; `coursesCount` — только `published` (T65) |
| GET | `/api/v1/courses/{slug}` | публичный* | → `200` `{course, modules:[ModuleView]}`; *`draft` виден автору и админу, остальным 404 |
| GET | `/api/v1/courses/{id}/similar?limit=4` | публичный | → `200` `{items:[CourseItem],total}` — published-курсы той же `category`, кроме самого курса; порядок: активные записи desc, `avgRating` desc, `createdAt` desc; `limit` по умолчанию 4 (максимум 12, нечисловой/`<=0` — дефолт), `total` — число отданных элементов; `404 «Курс не найден»` для неопубликованного/несуществующего `id` (T79) |
| GET | `/api/v1/courses/{id}/reviews?rating=1..5` | публичный | `{id}` — UUID или slug; `rating` необязателен и фильтрует `items`/`total` → `200` `{items:[Review],total,avgRating,ratingsCount,breakdown:{"1".."5"}}`; `avgRating`/`ratingsCount`/`breakdown` — по всем отзывам курса; иной `rating` → `400 «Оценка должна быть числом от 1 до 5»` |
| POST | `/api/v1/courses/{id}/reviews` | студент с активной записью | `{rating:1..5, body}` → `201` `Review` |
| GET | `/api/v1/courses/{id}/questions?lessonId=&page=&limit=` | публичный* | → `200` `{items:[LessonQuestion],total,page,limit}`; вопросы новые первыми, ответы внутри — старые первыми; *неопубликованный курс виден только владельцу/админу, остальным `404 «Курс не найден»`; `lessonId` не из курса → `404 «Урок не найден в этом курсе»` |
| GET | `/api/v1/certificates/{number}` | публичный | → `200` `{number,issuedAt,studentName,courseTitle,courseSlug}`; `studentName` — displayName студента (e-mail не отдаётся); `404 «Сертификат не найден»` |
| GET | `/api/v1/teachers/{id}` | публичный | → `200 {"teacher":{id,displayName,coursesCount,studentsCount,avgRating,ratingsCount},"courses":[CourseItem]}`; только `published`-курсы, порядок — активные записи desc, затем `createdAt` desc; агрегаты — по published-курсам; `404 «Преподаватель не найден»` — пользователя нет или роль не teacher/admin (T81) |

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
| GET | `/api/v1/me/favorites` | cookie | → `200` `{items:[CourseItem],total}` — избранные курсы, добавленные позже первыми; архивные/неопубликованные и удалённые курсы не попадают (T73) |
| PUT | `/api/v1/me/favorites/{courseId}` | cookie | без тела → `204`; идемпотентно; только published-курс, иначе `404 «Курс не найден»` (T73) |
| DELETE | `/api/v1/me/favorites/{courseId}` | cookie | → `204`; идемпотентно, в т.ч. если курса или пары уже нет (T73) |
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

> Волна P9 (T75): превью-уроки. В урок добавлено аддитивное поле `isPreview`
> (bool, всегда присутствует — не omitempty) во всех видах: страница курса,
> полный вид владельца/админа и ответы teacher-хендлеров. `POST
> /api/v1/teacher/courses/{id}/lessons` и `PATCH …/lessons/{lessonID}`
> принимают необязательное `isPreview`: создание без поля — `false`, PATCH без
> поля значение не меняет. PATCH, содержащий только `isPreview`, разрешён и
> для опубликованного курса (преподаватель управляет превью без снятия курса с
> публикации); любые другие поля по-прежнему требуют черновика (`409`).
> Публичный `GET /api/v1/courses/{slug}/preview/{lessonId}`
> без входа отдаёт `200 {"course":{"id","slug","title"},"lesson":<lessonView>}`
> только для published-курса и урока с `isPreview=true`; урок отдаётся
> публичным видом — без правильных ответов теста и скрытых тестов code-задания.
> Иначе `404 «Курс не найден»` / `404 «Урок не найден»`.

> Волна P9 (T77): аналитика учителя. `GET /api/v1/teacher/stats` →
> `200 {courses:{total,published,draft,review,archived},
> students:{total,unique}, revenue:{paidTotalCents,platformFeeCents,teacherCents},
> completion:{completed,avgPercent}, ratings:{avgRating,ratingsCount},
> last30d:{enrollments,revenueCents}}` — teacher видит только свои курсы,
> admin — все. `students.total` — активные записи, `unique` — уникальные
> ученики с активной записью; `revenue.*` — только `succeeded`-платежи;
> `completion.avgPercent` — средний прогресс активных записей (0..100,
> дробное, 1 знак); `ratings` — средневзвешенно по опубликованным курсам;
> `last30d` — активные записи и `succeeded`-платежи за 30 дней.
> `GET /api/v1/teacher/stats/courses?limit=50` →
> `200 {"items":[{courseId,title,status,students,revenueCents,avgRating,
> ratingsCount,avgPercent}],"total":N}` — курсы любого статуса, порядок
> `revenueCents` desc → `students` desc → `title`; `limit` по умолчанию 50,
> максимум 200 (`total` — все курсы в области видимости до применения limit;
> limit только укорачивает `items`).
> Ученику оба эндпоинта — `403` (группа `/teacher` — только teacher/admin).

> В бэклоге T13: `GET/PATCH/DELETE /teacher/courses[...]`, PATCH модулей/уроков,
> reorder — в api-contract.md (раздел T13).

### Админ (`admin` — только роль admin)

| Метод | Путь | Тело / ответ |
|---|---|---|
| POST | `/api/v1/admin/invites` | `{email}` → `201 {id,email,token,tokenPrefix,expiresAt,inviteLink,emailSent}`; **токен показывается один раз**, `inviteLink` — готовая ссылка, `emailSent:false` при недоступном SMTP |
| GET | `/api/v1/admin/invites?page=&limit=` | → `200 {items:[{id,email,status,tokenPrefix,expiresAt,createdAt,createdBy}],total,page,limit}`; сырой `token` после создания не возвращается |
| POST | `/api/v1/admin/invites/{id}/revoke` | без тела → `200 {"invite":{...status:"revoked"}}`; `404`/`409` |
| GET | `/api/v1/admin/courses/review` | → `200` `{items:[Course],total}` |
| POST | `/api/v1/admin/courses/{id}/moderate` | `{decision:"approve\|reject", comment?}` → `200` Course; `comment` обязателен при `reject` |
| GET | `/api/v1/admin/courses/{id}` | → `200` `{course:<элемент GET /admin/courses + coverPath>, modules:[{id,title,position,lessons:[{id,title,kind,position}]}], stats:{studentsCount,activeCount,reviewsCount,avgRating,paymentsCount,paidTotalCents}}` (T55; `404` «Курс не найден») |
| GET | `/api/v1/admin/payments?page=&limit=&status=&courseId=&q=&from=&to=` | → `200` `{items:[PaymentJournalItem],total,page,limit,totals}`; `q` — подстрока email/имени студента, `from`/`to` — `ГГГГ-ММ-ДД` включительно (US6, T07/T31) |
| GET | `/api/v1/admin/settings` | → `200 {commissionPct,promoBarEnabled}` (US6, T07; промо — пост-P7) |
| PUT | `/api/v1/admin/settings` | `{commissionPct?0..90, promoBarEnabled?}` — хотя бы одно поле → `200 {commissionPct,promoBarEnabled}` (US6, T07; частичное обновление — пост-P7) |

### Оплата

| Метод | Путь | Доступ | Тело / ответ |
|---|---|---|---|
| POST | `/api/v1/enrollments` | cookie | `{courseId, promoCode?}` → `201 {paymentId,providerPaymentId,redirectUrl,amountCents,status,enrollmentActive}`; провайдера выбирает сервер из `PAYMENTS_PROVIDER`, поле `provider` в запросе больше не используется; бесплатный курс сразу `succeeded` + `enrollmentActive:true` |
| POST | `/api/v1/payments/webhook/{provider}` | подпись провайдера (без cookie) | провайдер-специфичное тело → `200 {status:"processed\|duplicate\|ignored"}`; Stripe — `Stripe-Signature`, YooKassa — IP из `YOOKASSA_TRUSTED_IPS`, mock — `X-Mock-Signature` (если задан `MOCK_WEBHOOK_SECRET`). В production `PAYMENTS_PROVIDER=mock` запрещён на старте; defense-in-depth — mock-вебхук `503` |
| GET | `/pay/mock/{id}` | публичный | HTML-страница подтверждения mock-оплаты (не под `/api/v1`); доступна только когда сервер в dev-режиме |

## Частые коды ошибок

| Код | Причина | Что делать |
|---|---|---|
| `400` | Невалидное тело: неизвестное поле (`DisallowUnknownFields`), неверный тип, ошибка валидации (`rating` вне 1..5, `commissionPct` вне 0..90, `quiz` без ответов, отсутствует `comment` при `reject` и т.п.) | Отправить ровно поля из контракта; не добавлять «лишних» через спред; проверить обязательные поля по `kind` урока |
| `401` | Нет сессии, истёкшая/подделанная подпись cookie, неверный пароль, неверная подпись вебхука | Перелогиниться; для вебхука — проверить секрет провайдера |
| `403` | Недостаточно прав (роль), нет активной записи на курс, запрос с недопустимого `Origin` (CSRF) | Проверить роль и запись; для локальных скриптов — `CSRF_ALLOW_MISSING_ORIGIN=true` |
| `404` | Сущности нет, либо она скрыта (черновик чужого курса) | Проверить идентификатор (slug в каталоге, UUID в остальном); не считать 404 «курс удалён» — он может быть draft |
| `409` | Конфликт состояния: курс уже куплен, отзыв уже оставлен, платёж уже создан, архивный курс нельзя менять | Показать «уже есть»; при повторной покупке pending-платежа клиент получит `200` с `redirectUrl` для продолжения. Занятый email при регистрации — `400` (историческое расхождение, закреплено тестом) |
| `503` | Отключённая возможность: Яндекс OAuth без ключей; mock-оплата, если production-конфигурацию обошли (штатно старт с mock запрещён) | Для mock — переключиться на stripe/yookassa; для OAuth — задать `YANDEX_CLIENT_ID`/`YANDEX_CLIENT_SECRET` |
| `429` | Rate-limit по IP (auth 10/мин, глобально 300/мин, вебхуки 120/мин) | Подождать `Retry-After`; не ретраить в цикле |
| `500` | Внутренняя ошибка сервера (БД и т.п.) | Логи backend; текст наружу не раскрывает детали |
| `502` | Внешний сервис недоступен: `PAYMENTS_PROVIDER` не настроен (`ErrNotConfigured`), API оплаты недоступен, runner не ответил | Для оплаты — проверить env-ключи провайдера; для runner — `docker compose ps`, `RUNNER_URL` |
