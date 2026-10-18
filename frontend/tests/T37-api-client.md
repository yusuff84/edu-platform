# T37 — Контрактные тесты API-клиента (frontend)

**Волна:** P4 · **Ветка:** `task/T37-api-client-tests` · **Статус:** готово, `npm test` → exit 0 (61/61 pass)

Инструмент: контрактные тесты `frontend/src/api/client.ts` без единой новой зависимости —
встроенный `node:test` + мини-HTTP-сервер на 127.0.0.1:8799. `frontend/src/**` не менялся.

## Файлы

| Файл | Назначение |
| --- | --- |
| `frontend/.env.test` | `VITE_API_URL=http://127.0.0.1:8799/api/v1` — адрес, вшиваемый в тестовую сборку |
| `frontend/package.json` | только `scripts`: `test:build` (сборка клиента) и `test` (сборка + `node --test`) |
| `frontend/tests/api-contract/helper.mjs` | мини-API: журнал запросов, заготовленные ответы, повтор привязки к порту, ассерты |
| `frontend/tests/api-contract/transport.test.mjs` | транспорт `apiFetch`, `buildQuery`, `ApiError`, чистые утилиты (20 тестов) |
| `frontend/tests/api-contract/auth-catalog.test.mjs` | auth, каталог, отзывы, обучение (20 тестов) |
| `frontend/tests/api-contract/teaching-admin.test.mjs` | преподавание и админка (21 тест) |
| `.gitignore` (корень) | строка `.tmp-api-test/` — папка тестовой сборки в репозиторий не попадает |
| `frontend/tests/T37-api-client.md` | этот отчёт |

## Как запускать

```powershell
cd frontend
npm test          # test:build (vite --ssr, --mode test) + node --test tests/api-contract/*.test.mjs
npx tsc --noEmit
npx vite build    # обычная сборка: .env.test не участвует, BASE остаётся "/api/v1"
```

## Устройство стенда

- `vite build --ssr src/api/client.ts --outDir .tmp-api-test --emptyOutDir --mode test` собирает
  клиент в обычный ESM-модуль Node (29 экспортов), `VITE_API_URL` из `.env.test` вшивается в бандл.
- `helper.mjs` поднимает `http.createServer` на **127.0.0.1:8799**, пишет каждый запрос как
  `{method, url, path, query, headers, rawBody, json}` и отвечает заготовленным JSON (`mockReply`).
- `node:test` запускает файлы параллельно в отдельных процессах, поэтому `startMockApi` при
  `EADDRINUSE` повторяет привязку (150 мс, до 60 с) — файлы сами сериализуются на фиксированном порту.
  Если порт занят посторонним процессом, тест падает с понятным сообщением про `.env.test`.
- Ассерты строгие: метод + путь + ключевые query + **точный** набор полей тела
  (`expectBodyKeys`, моделирует `DisallowUnknownFields` на backend).
- `paymentJournal` проверяется по **наличию** ожидаемых параметров (`page`, `limit`, `status`,
  `courseId`); отсутствие остальных не требуется — это совместимость с T38 (`q`/`from`/`to`).

## Покрытие по функциям

| Функция | Что проверено |
| --- | --- |
| `apiFetch` | GET по умолчанию; POST/PATCH и `Content-Type: application/json`; разбор успешного JSON; 200 с не-JSON → `undefined`; 204 → `undefined` (без парсинга); 400 `{"error":…}` → `ApiError(status, текст)`; 500 с не-JSON → «Ошибка запроса (500)»; 400 с JSON-массивом → запасной текст; 401 без текста → «Требуется вход в аккаунт»; 401 с текстом сервера → текст сервера |
| `ApiError` | наследник `Error`; поле `status`; текст из envelope/запасной |
| `buildQuery` | без параметров и `{}` → `''`; `undefined`/`''` выбрасываются; числа приводятся, `0` не выбрасывается; URL-кодирование (`+`, `%26`, кириллица); порядок ключей сохраняется |
| `register` | `POST /auth/register`, тело строго `email+password`; `inviteToken` уходит только при передаче |
| `login` | `POST /auth/login`, тело строго `email+password` |
| `logout` | `POST /auth/logout` без тела вообще; 204 → `undefined` |
| `me` | `GET /auth/me`, ответ возвращается как есть (`displayName` не теряется) |
| `listCourses` | без параметров — без query; `q/page/limit` попадают в query; пустые выбрасываются; конверт `{items,total,page,limit}` |
| `getCourse` | `GET /courses/{slug}`; конверт `{course,modules}` распаковывается в `CourseDetail`; slug кодируется целиком (пробел, слэш, кириллица) |
| `createCourse` | `POST /teacher/courses`; без `slug/currency` тело из 3 полей; `slug`/`currency` добавляются только при передаче |
| `myTeaching` | `GET /teacher/courses`, конверт `{items,total}` → массив |
| `createModule` | `POST /teacher/courses/{id}/modules`, тело строго `{title}` |
| `createLesson` | `POST /teacher/courses/{id}/lessons`; 7 кейсов: `video_text` (+ минимальный), `live` (+ минимальный), `quiz` (`{questions}` как есть), `code` (с `template` и без) — в теле только поля своего kind |
| `submitCourse` | `POST /teacher/courses/{id}/submit` без тела |
| `createInvite` | `POST /admin/invites`, тело строго `{email}` |
| `reviewQueue` | `GET /admin/courses/review`, конверт → массив |
| `moderateCourse` | `POST /admin/courses/{id}/moderate`: `{decision}` без `comment`; reject с `comment` |
| `paymentJournal` | `GET /admin/payments` без query; наличие и значения `page/limit/status/courseId`; частичные параметры — `undefined` не попадают в query (forward-compat с T38) |
| `getAdminSettings` | `GET /admin/settings` |
| `updateAdminSettings` | `PUT /admin/settings`, тело строго `{commissionPct}` |
| `enroll` | `POST /enrollments`, `provider` по умолчанию `mock`; явный `provider` без изменений |
| `myCourses` | `GET /me/courses`, конверт → массив `MyCourse` (с `nextLesson`) |
| `completeLesson` | без тела для video/live; quiz — строго `{quizAnswers}`; пустой `quizAnswers` → без тела; code — `{code,language}`; code без language — только `{code}` |
| `listReviews` | `GET /courses/{id}/reviews`, конверт → массив `Review` |
| `createReview` | `POST /courses/{id}/reviews`, тело строго `{rating,body}` (ключ `body`) |
| `formatMoney` | `0` → «0 ₽»; копейки; разряды (NBSP) и запятая; отрицательные |
| `formatPrice` | `0` и меньше → «Бесплатно»; положительные совпадают с `formatMoney` |
| `formatDate` | RFC3339 → «дд.мм.гггг, чч:мм» локально (тест не зависит от таймзоны); пустое/битое → «—» |
| `useApiBase` | база равна адресу из `.env.test` — подтверждает, что `--mode test` подхватил окружение |

Итого: **61 тест-кейс, 184 явных `assert`-вызова** (+ внутренние проверки помощников
`lastRequest`/`jsonBody`/`expectBodyKeys`).

## Вывод `npm test` (полный лог, exit 0)

```text

> edu-platform-frontend@0.1.0 test
> npm run test:build && node --test tests/api-contract/*.test.mjs


> edu-platform-frontend@0.1.0 test:build
> vite build --ssr src/api/client.ts --outDir .tmp-api-test --emptyOutDir --mode test

vite v5.4.21 building SSR bundle for test...
transforming...
✓ 1 modules transformed.
rendering chunks...
.tmp-api-test/client.js  7.31 kB
✓ built in 345ms
TAP version 13
# Subtest: register: POST /auth/register, тело строго email+password
ok 1 - register: POST /auth/register, тело строго email+password
  ---
  duration_ms: 806.3071
  type: 'test'
  ...
# Subtest: register: inviteToken уходит только когда передан
ok 2 - register: inviteToken уходит только когда передан
  ---
  duration_ms: 30.2674
  type: 'test'
  ...
# Subtest: login: POST /auth/login, тело строго email+password
ok 3 - login: POST /auth/login, тело строго email+password
  ---
  duration_ms: 13.2693
  type: 'test'
  ...
# Subtest: logout: POST /auth/logout без тела, 204 → undefined
ok 4 - logout: POST /auth/logout без тела, 204 → undefined
  ---
  duration_ms: 24.1732
  type: 'test'
  ...
# Subtest: me: GET /auth/me возвращает пользователя как есть
ok 5 - me: GET /auth/me возвращает пользователя как есть
  ---
  duration_ms: 8.8323
  type: 'test'
  ...
# Subtest: listCourses: без параметров — GET /courses без query
ok 6 - listCourses: без параметров — GET /courses без query
  ---
  duration_ms: 8.6206
  type: 'test'
  ...
# Subtest: listCourses: q/page/limit попадают в query, пустые выбрасываются
ok 7 - listCourses: q/page/limit попадают в query, пустые выбрасываются
  ---
  duration_ms: 28.6182
  type: 'test'
  ...
# Subtest: listCourses: карточки каталога возвращаются в конверте {items,total,page,limit}
ok 8 - listCourses: карточки каталога возвращаются в конверте {items,total,page,limit}
  ---
  duration_ms: 33.8121
  type: 'test'
  ...
# Subtest: getCourse: GET /courses/{slug}, конверт {course, modules} распаковывается
ok 9 - getCourse: GET /courses/{slug}, конверт {course, modules} распаковывается
  ---
  duration_ms: 15.1961
  type: 'test'
  ...
# Subtest: getCourse: slug URL-кодируется целиком (пробелы, слэш, кириллица)
ok 10 - getCourse: slug URL-кодируется целиком (пробелы, слэш, кириллица)
  ---
  duration_ms: 30.9212
  type: 'test'
  ...
# Subtest: listReviews: GET /courses/{id}/reviews, конверт распаковывается в массив
ok 11 - listReviews: GET /courses/{id}/reviews, конверт распаковывается в массив
  ---
  duration_ms: 13.1713
  type: 'test'
  ...
# Subtest: createReview: POST /courses/{id}/reviews, тело строго rating+body (ключ body, не text)
ok 12 - createReview: POST /courses/{id}/reviews, тело строго rating+body (ключ body, не text)
  ---
  duration_ms: 18.2091
  type: 'test'
  ...
# Subtest: enroll: POST /enrollments, provider по умолчанию mock
ok 13 - enroll: POST /enrollments, provider по умолчанию mock
  ---
  duration_ms: 13.6578
  type: 'test'
  ...
# Subtest: enroll: явный provider попадает в тело без изменений
ok 14 - enroll: явный provider попадает в тело без изменений
  ---
  duration_ms: 8.0959
  type: 'test'
  ...
# Subtest: myCourses: GET /me/courses, конверт распаковывается в массив MyCourse
ok 15 - myCourses: GET /me/courses, конверт распаковывается в массив MyCourse
  ---
  duration_ms: 15.3259
  type: 'test'
  ...
# Subtest: completeLesson (video_text): POST …/complete без тела вовсе
ok 16 - completeLesson (video_text): POST …/complete без тела вовсе
  ---
  duration_ms: 11.4026
  type: 'test'
  ...
# Subtest: completeLesson (quiz): тело строго quizAnswers
ok 17 - completeLesson (quiz): тело строго quizAnswers
  ---
  duration_ms: 31.324
  type: 'test'
  ...
# Subtest: completeLesson (quiz): пустой quizAnswers → тело не отправляется
ok 18 - completeLesson (quiz): пустой quizAnswers → тело не отправляется
  ---
  duration_ms: 4.7741
  type: 'test'
  ...
# Subtest: completeLesson (code): тело code+language
ok 19 - completeLesson (code): тело code+language
  ---
  duration_ms: 10.4879
  type: 'test'
  ...
# Subtest: completeLesson (code): без language в теле только code
ok 20 - completeLesson (code): без language в теле только code
  ---
  duration_ms: 16.1624
  type: 'test'
  ...
# Subtest: myTeaching: GET /teacher/courses, конверт {items,total} → массив
ok 21 - myTeaching: GET /teacher/courses, конверт {items,total} → массив
  ---
  duration_ms: 414.5735
  type: 'test'
  ...
# Subtest: createCourse: POST /teacher/courses, тело строго из 3 полей без slug/currency
ok 22 - createCourse: POST /teacher/courses, тело строго из 3 полей без slug/currency
  ---
  duration_ms: 41.2917
  type: 'test'
  ...
# Subtest: createCourse: slug и currency добавляются, только когда переданы
ok 23 - createCourse: slug и currency добавляются, только когда переданы
  ---
  duration_ms: 12.274
  type: 'test'
  ...
# Subtest: createModule: POST /teacher/courses/{id}/modules, тело строго {title}
ok 24 - createModule: POST /teacher/courses/{id}/modules, тело строго {title}
  ---
  duration_ms: 20.243
  type: 'test'
  ...
# Subtest: createLesson (video_text): в теле только поля своего kind
ok 25 - createLesson (video_text): в теле только поля своего kind
  ---
  duration_ms: 10.983
  type: 'test'
  ...
# Subtest: createLesson (video_text без bodyMd/videoUrl): только обязательные поля
ok 26 - createLesson (video_text без bodyMd/videoUrl): только обязательные поля
  ---
  duration_ms: 8.4003
  type: 'test'
  ...
# Subtest: createLesson (live): liveUrl и startsAt, без чужих полей
ok 27 - createLesson (live): liveUrl и startsAt, без чужих полей
  ---
  duration_ms: 13.913
  type: 'test'
  ...
# Subtest: createLesson (live без liveUrl/startsAt): только обязательные поля
ok 28 - createLesson (live без liveUrl/startsAt): только обязательные поля
  ---
  duration_ms: 8.3541
  type: 'test'
  ...
# Subtest: createLesson (quiz): вложенный quiz = {questions}, вопросы как есть
ok 29 - createLesson (quiz): вложенный quiz = {questions}, вопросы как есть
  ---
  duration_ms: 12.6218
  type: 'test'
  ...
# Subtest: createLesson (code): вложенный codeTask с template
ok 30 - createLesson (code): вложенный codeTask с template
  ---
  duration_ms: 14.9871
  type: 'test'
  ...
# Subtest: createLesson (code без template): в codeTask только language+tests
ok 31 - createLesson (code без template): в codeTask только language+tests
  ---
  duration_ms: 9.783
  type: 'test'
  ...
# Subtest: submitCourse: POST /teacher/courses/{id}/submit без тела
ok 32 - submitCourse: POST /teacher/courses/{id}/submit без тела
  ---
  duration_ms: 6.9407
  type: 'test'
  ...
# Subtest: createInvite: POST /admin/invites, тело строго {email}
ok 33 - createInvite: POST /admin/invites, тело строго {email}
  ---
  duration_ms: 11.1512
  type: 'test'
  ...
# Subtest: reviewQueue: GET /admin/courses/review, конверт {items,total} → массив
ok 34 - reviewQueue: GET /admin/courses/review, конверт {items,total} → массив
  ---
  duration_ms: 18.9218
  type: 'test'
  ...
# Subtest: moderateCourse: POST /admin/courses/{id}/moderate, только {decision} без comment
ok 35 - moderateCourse: POST /admin/courses/{id}/moderate, только {decision} без comment
  ---
  duration_ms: 11.9137
  type: 'test'
  ...
# Subtest: moderateCourse: reject передаёт decision+comment
ok 36 - moderateCourse: reject передаёт decision+comment
  ---
  duration_ms: 5.5089
  type: 'test'
  ...
# Subtest: getAdminSettings: GET /admin/settings
ok 37 - getAdminSettings: GET /admin/settings
  ---
  duration_ms: 12.0435
  type: 'test'
  ...
# Subtest: updateAdminSettings: PUT /admin/settings, тело строго {commissionPct}
ok 38 - updateAdminSettings: PUT /admin/settings, тело строго {commissionPct}
  ---
  duration_ms: 5.5421
  type: 'test'
  ...
# Subtest: paymentJournal: без параметров — GET /admin/payments без query
ok 39 - paymentJournal: без параметров — GET /admin/payments без query
  ---
  duration_ms: 10.9193
  type: 'test'
  ...
# Subtest: paymentJournal: page/limit/status/courseId попадают в query
ok 40 - paymentJournal: page/limit/status/courseId попадают в query
  ---
  duration_ms: 5.6902
  type: 'test'
  ...
# Subtest: paymentJournal: частичные параметры — пустые не отправляются (совместимо с T38)
ok 41 - paymentJournal: частичные параметры — пустые не отправляются (совместимо с T38)
  ---
  duration_ms: 5.9583
  type: 'test'
  ...
# Subtest: apiFetch: GET по умолчанию, JSON разбирается, есть Content-Type: application/json
ok 42 - apiFetch: GET по умолчанию, JSON разбирается, есть Content-Type: application/json
  ---
  duration_ms: 120.9384
  type: 'test'
  ...
# Subtest: apiFetch: POST передаёт метод, тело и заголовок Content-Type: application/json
ok 43 - apiFetch: POST передаёт метод, тело и заголовок Content-Type: application/json
  ---
  duration_ms: 26.4938
  type: 'test'
  ...
# Subtest: apiFetch: PATCH — тоже Content-Type: application/json
ok 44 - apiFetch: PATCH — тоже Content-Type: application/json
  ---
  duration_ms: 12.3921
  type: 'test'
  ...
# Subtest: apiFetch: 200 с не-JSON телом → undefined (считается пустым ответом)
ok 45 - apiFetch: 200 с не-JSON телом → undefined (считается пустым ответом)
  ---
  duration_ms: 8.2252
  type: 'test'
  ...
# Subtest: apiFetch: 400 {"error": "…"} → ApiError со статусом и текстом сервера
ok 46 - apiFetch: 400 {"error": "…"} → ApiError со статусом и текстом сервера
  ---
  duration_ms: 10.8122
  type: 'test'
  ...
# Subtest: apiFetch: ошибка с не-JSON телом → запасной текст «Ошибка запроса (500)»
ok 47 - apiFetch: ошибка с не-JSON телом → запасной текст «Ошибка запроса (500)»
  ---
  duration_ms: 8.2418
  type: 'test'
  ...
# Subtest: apiFetch: ошибка с JSON-массивом вместо envelope → запасной текст
ok 48 - apiFetch: ошибка с JSON-массивом вместо envelope → запасной текст
  ---
  duration_ms: 8.7875
  type: 'test'
  ...
# Subtest: apiFetch: 401 без текста → «Требуется вход в аккаунт»
ok 49 - apiFetch: 401 без текста → «Требуется вход в аккаунт»
  ---
  duration_ms: 11.015
  type: 'test'
  ...
# Subtest: apiFetch: 401 с текстом сервера → текст сервера, не запасной
ok 50 - apiFetch: 401 с текстом сервера → текст сервера, не запасной
  ---
  duration_ms: 12.3572
  type: 'test'
  ...
# Subtest: apiFetch: 204 No Content → undefined без попытки парсинга
ok 51 - apiFetch: 204 No Content → undefined без попытки парсинга
  ---
  duration_ms: 7.7662
  type: 'test'
  ...
# Subtest: buildQuery: без параметров и с пустым объектом → пустая строка
ok 52 - buildQuery: без параметров и с пустым объектом → пустая строка
  ---
  duration_ms: 1.0684
  type: 'test'
  ...
# Subtest: buildQuery: undefined и пустые строки выбрасываются
ok 53 - buildQuery: undefined и пустые строки выбрасываются
  ---
  duration_ms: 0.7704
  type: 'test'
  ...
# Subtest: buildQuery: числа приводятся к строке, 0 не выбрасывается
ok 54 - buildQuery: числа приводятся к строке, 0 не выбрасывается
  ---
  duration_ms: 1.04
  type: 'test'
  ...
# Subtest: buildQuery: значения URL-кодируются
ok 55 - buildQuery: значения URL-кодируются
  ---
  duration_ms: 0.8761
  type: 'test'
  ...
# Subtest: buildQuery: порядок ключей сохраняется
ok 56 - buildQuery: порядок ключей сохраняется
  ---
  duration_ms: 0.8895
  type: 'test'
  ...
# Subtest: formatMoney: 0 — это «0 ₽», копейки и отрицательные суммы не округляются до нуля
ok 57 - formatMoney: 0 — это «0 ₽», копейки и отрицательные суммы не округляются до нуля
  ---
  duration_ms: 76.3559
  type: 'test'
  ...
# Subtest: formatPrice: 0 и меньше → «Бесплатно», иначе та же сумма, что у formatMoney
ok 58 - formatPrice: 0 и меньше → «Бесплатно», иначе та же сумма, что у formatMoney
  ---
  duration_ms: 1.4306
  type: 'test'
  ...
# Subtest: formatDate: RFC3339 → «дд.мм.гггг, чч:мм» в локальном времени
ok 59 - formatDate: RFC3339 → «дд.мм.гггг, чч:мм» в локальном времени
  ---
  duration_ms: 12.6257
  type: 'test'
  ...
# Subtest: formatDate: пустое и некорректное значение → «—»
ok 60 - formatDate: пустое и некорректное значение → «—»
  ---
  duration_ms: 0.5263
  type: 'test'
  ...
# Subtest: useApiBase: клиент собран с адресом из .env.test (VITE_API_URL)
ok 61 - useApiBase: клиент собран с адресом из .env.test (VITE_API_URL)
  ---
  duration_ms: 0.4341
  type: 'test'
  ...
1..61
# tests 61
# suites 0
# pass 61
# fail 0
# cancelled 0
# skipped 0
# todo 0
# duration_ms 1738.914
```

## Проверка задачи

| Команда | Результат |
| --- | --- |
| `npm test` | exit 0: build ок, 61/61 pass, 0 fail (~2 с) |
| `npx tsc --noEmit` | exit 0 |
| `npx vite build` | exit 0; в `dist/` нет `8799`, `BASE` = `"/api/v1"` — `.env.test` на проду не влияет |

## Отклонения от плана (сознательные)

1. В `package.json` скрипт `test` вызывает `node --test tests/api-contract/*.test.mjs`, а не
   `node --test tests/api-contract`: в Node v22.16.0 на Windows каталог-аргумент передаётся
   в дочерний процесс как путь к модулю и падает `MODULE_NOT_FOUND` (проверено `node --test tests/api-contract`).
   Глоб выбирает ровно те же 3 файла; в sh-окружениях оболочка раскроет глоб сама.
2. `node_modules` в основном чекауте (цель junction из worktree) на момент старта был пуст, поэтому
   для запуска проверок он восстановлен локально: `npm ci --offline` по `package-lock.json`
   (те же версии, новых пакетов нет, lock-файл не менялся). На содержимое репозитория это не повлияло.

## Не покрыто (честно)

- `getTeacherCourse` из `api/teacherStudents.ts` и другие функции модулей-компаньонов
  (`api/adminUsers.ts`, `api/adminConsole.ts`, `api/teacherExtra.ts`, `api/meExtra.ts`) — они живут
  вне `client.ts` и в этот бандл не собираются (используют покрытые `apiFetch`/`buildQuery`, но их
  собственные пути/тела не проверяются).
- Реальный backend не задействован: мок проверяет клиентскую сторону контракта, а не фактическое
  поведение Go-хендлеров (в т.ч. реальный `DisallowUnknownFields`).
- `credentials: 'include'` не проверяется: Node-fetch без cookie-jar не проявляет его в наблюдаемых
  заголовках.
- Слияние пользовательских `init.headers` с дефолтным `Content-Type` и сетевые ошибки `fetch`
  (сервер недоступен) не покрыты.
- UI-страницы, i18n и роутинг вне рамок задачи.
- Формат-тесты опираются на полный ICU Node (ru-RU, NBSP) — на средах без полного ICU строки
  могут отличаться.
