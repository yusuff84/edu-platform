# Tasks: IT-курсы MVP

**Input**: Design documents from `/specs/001-it-courses-mvp/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: Контрактные тесты API включены (требование конституции III для auth/оплаты/прогресса). Писать до реализации, должны падать.

**Organization**: По user story для независимой реализации и проверки.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Можно параллельно (разные файлы, нет зависимостей)
- **[Story]**: К какой истории относится (US1..US6)
- Пути точные из plan.md

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Инициализация монорепо и Docker

- [x] T001 Create monorepo skeleton backend/frontend/runner per plan.md
- [x] T002 [P] Create docker-compose.yml (api:8080, web:3000, db:5432, runner:8081) in docker-compose.yml
- [x] T003 [P] Create .env.example (DATABASE_URL, SESSION_SECRET, PAYMENTS_PROVIDER=mock, COMMISSION_PCT=15) in .env.example
- [x] T004 [P] Init Go module + chi/pgx/migrate deps in backend/go.mod
- [x] T005 [P] Init React+Vite+TS app with Router/Query in frontend/package.json
- [x] T006 [P] Create Nginx config for SPA + /uploads in frontend/nginx.conf

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: БД, API-каркас, auth-ядро, i18n — блокирует все истории

**CRITICAL**: Никакая история не начинается до завершения фазы

- [x] T007 Setup Postgres schema + golang-migrate with users/teacher_invites/courses/modules/lessons/enrollments/lesson_progress/payments/reviews/app_settings in backend/internal/migrate/migrations/001_init.sql
- [x] T008 [P] Implement healthcheck + OpenAPI mount in backend/cmd/api/main.go
- [x] T009 Implement session auth API (register/login/logout, argon2, httpOnly cookie, CSRF) with role ENUM student/teacher/admin in backend/internal/handler/auth.go
- [x] T010 [P] Implement roles middleware + rus error envelope in backend/internal/handler/middleware.go
- [x] T011 [P] Implement pgx repositories base (users, settings) in backend/internal/repo/users.go
- [x] T012 Create frontend shell + router + ru.json i18n + api client in frontend/src/app/router.tsx
- [x] T013 Configure structured logging + env config in backend/internal/config/config.go

**Checkpoint**: `docker compose up`, `/healthz` OK, регистрация/вход через API работает

---

## Phase 3: User Story 4 - Регистрация и кабинеты (Priority: P2, базовая — идет первой технически)

**Goal**: Вход по email+пароль и кабинеты по ролям

**Independent Test**: Регистрация, вход/выход, неверный пароль показывает русскую ошибку, кабинеты различаются по ролям

- [x] T014 [P] [US4] Contract test auth register/login/me in backend/tests/contract/auth_test.go
- [x] T015 [P] [US4] Create Auth pages (Register/Login) with zod validation in frontend/src/pages/Auth.tsx
- [x] T016 [US4] Implement me + StudentCab/TeacherCab/AdminCab shells with role guard in frontend/src/pages/Cabinets.tsx
- [x] T017 [US4] Wire session logout + invite-token register path in backend/internal/auth/invite.go

**Checkpoint**: Новый пользователь регистрируется и видит свой кабинет

---

## Phase 4: User Story 1 - Поиск и покупка курса (Priority: P1) — MVP

**Goal**: Каталог, страница курса, онлайн-покупка с гибким провайдером, доступ в "Мои курсы"

**Independent Test**: Сид-курс виден в каталоге, покупка mock-провайдером открывает доступ, повторная покупка блокируется 409, неуспешная оплата не дает доступ

- [x] T018 [P] [US1] Contract test GET /courses + GET /courses/{slug} in backend/tests/contract/courses_test.go
- [x] T019 [P] [US1] Contract test POST /enrollments + webhook idempotency in backend/tests/contract/payments_test.go
- [x] T020 [P] [US1] Create courses + enrollments tables access with price_cents>=0, status published-only catalog in backend/internal/repo/courses.go
- [x] T021 [US1] Implement PaymentProvider interface + factory + MockProvider with provider_payment_id UNIQUE in backend/internal/payments/provider.go
- [x] T022 [US1] Implement Stripe adapter stub behind PAYMENTS_PROVIDER in backend/internal/payments/stripe.go
- [x] T023 [US1] Implement YooKassa adapter stub behind PAYMENTS_PROVIDER in backend/internal/payments/yookassa.go
- [x] T024 [US1] Implement enroll handler (commission split, 409 if exists, redirectUrl) + webhook handler in backend/internal/handler/enroll.go
- [x] T025 [P] [US1] Create Catalog page with search/filter in frontend/src/pages/Catalog.tsx
- [x] T026 [P] [US1] Create Course page (program, teacher, price, rating) + Buy button in frontend/src/pages/Course.tsx
- [x] T027 [US1] Create Student MyCourses list wired to /me/courses in frontend/src/pages/MyCourses.tsx

**Checkpoint**: US1 e2e по quickstart.md сценарий A проходит

---

## Phase 5: User Story 2 - Создание курса учителем по приглашению (Priority: P1)

**Goal**: Приглашение → конструктор → модерация → публикация

**Independent Test**: Инвайт → регистрация учителя → черновик 1 модуль/3 урока → на проверку → одобрение админом → курс в каталоге; пустой курс не уходит

- [x] T028 [P] [US2] Contract test invites + submit + moderate in backend/tests/contract/teacher_test.go
- [x] T029 [P] [US2] Implement TeacherInvite (token hash, TTL 7d, sent/accepted/expired/revoked) in backend/internal/repo/invites.go
- [x] T030 [US2] Implement teacher course CRUD draft + modules/lessons reorder in transaction in backend/internal/handler/teacher.go
- [x] T031 [US2] Implement submit guard (≥1 модуль, ≥1 урок) + review queue + approve/reject with comment in backend/internal/handler/admin.go
- [x] T032 [P] [US2] Create Constructor UI (course/modules/lessons, kinds video_text/live/quiz/code) in frontend/src/pages/Constructor.tsx
- [x] T033 [P] [US2] Create Admin pages (Invites, ReviewQueue, Payments journal, commission) in frontend/src/pages/Admin.tsx

**Checkpoint**: Сценарий B из quickstart.md проходит

---

## Phase 6: User Story 3 - Прохождение и прогресс (Priority: P2)

**Goal**: Уроки всех типов, квизы и код с автопроверкой, прогресс и "Продолжить"

**Independent Test**: Video зачет, квиз с пояснением, python/js/htmlcss accepted/failed, прогресс %, live-ссылка доступна

- [x] T034 [P] [US3] Contract test complete lesson + quiz + code submit in backend/tests/contract/progress_test.go
- [x] T035 [P] [US3] Implement LessonProgress (PK student+lesson, percent, next lesson query) in backend/internal/repo/progress.go
- [x] T036 [US3] Implement quiz checker + code proxy to runner (timeout 5s) in backend/internal/service/checker.go
- [x] T037 [US3] Implement code-runner service (queue+workers python/node/htmlcss, no-net, limits) in runner/main.go
- [x] T038 [P] [US3] Create LessonView (video/text, live block, QuizForm, CodeEditor) in frontend/src/components/LessonView.tsx
- [x] T039 [US3] Wire progress bar + Continue button in frontend/src/pages/Learn.tsx

**Checkpoint**: Сценарий C из quickstart.md проходит, автопроверка <10с

---

## Phase 7: User Story 5 - Отзывы и рейтинг (Priority: P3)

**Goal**: Один отзыв на ученика, средний рейтинг в каталоге

**Independent Test**: Записанный оставляет отзыв, гость видит рейтинг, повторный отзыв блокируется

- [x] T040 [P] [US5] Contract test reviews create/list in backend/tests/contract/reviews_test.go
- [x] T041 [US5] Implement reviews repo with UNIQUE(student+course) + avg_rating recount in tx in backend/internal/repo/reviews.go
- [x] T042 [US5] Implement reviews endpoints (POST only enrolled, GET public) in backend/internal/handler/reviews.go
- [x] T043 [P] [US5] Create Reviews block + form on Course page in frontend/src/components/Reviews.tsx

**Checkpoint**: Рейтинг пересчитывается, в каталоге виден

---

## Phase 8: User Story 6 - Комиссия и журнал (Priority: P3)

**Goal**: Настраиваемая комиссия, журнал платежей

**Independent Test**: Смена commission_pct влияет только на новые платежи, журнал показывает split

- [x] T044 [P] [US6] Contract test commission change + payments journal in backend/tests/contract/admin_payments_test.go
- [x] T045 [US6] Implement AppSettings commission_pct 0..90 default 15 applied at payment creation in backend/internal/service/billing.go
- [x] T046 [US6] Implement admin payments journal query in backend/internal/repo/payments.go

**Checkpoint**: Старые платежи не пересчитываются

---

## Phase 9: Polish & Cross-Cutting

**Purpose**: Готовность к демо

- [x] T047 [P] Seed demo data (admin, teacher, python course, quiz/code samples) in backend/internal/migrate/migrations/002_seed.sql
- [x] T048 [P] Add ObjectStore interface for covers (/uploads volume) in backend/internal/storage/store.go
- [x] T049 Yandex OAuth adapter stub as AuthProvider without core change in backend/internal/auth/yandex.go
- [x] T050 [P] Frontend a11y + ru strings audit in frontend/src/i18n/ru.json
- [x] T051 Security hardening (rate-limit auth, webhook signature check, CORS) in backend/internal/handler/middleware.go
- [x] T052 Run quickstart.md validation scenarios A/B/C in specs/001-it-courses-mvp/quickstart.md

---

## Dependencies & Execution Order

- Phase 1 → Phase 2 → US4 → US1 → US2 → US3 → US5 → US6 → Polish
- US4 вынесена раньше US1 технически (auth UI), хотя приоритет P2 — фундамент AUTH API уже в Phase 2
- US1/US2 обе P1, идут последовательно: сначала покупка (деньги), затем контент-поток
- Параллельно можно: T002–T006, T008/T010/T011, тесты [P] внутри каждой истории, frontend [P] компоненты

## Parallel Example: US1

```bash
# Тесты вместе:
Task: "Contract test GET /courses в backend/tests/contract/courses_test.go"
Task: "Contract test POST /enrollments в backend/tests/contract/payments_test.go"
# Затем провайдеры:
Task: "Stripe adapter в backend/internal/payments/stripe.go"
Task: "YooKassa adapter в backend/internal/payments/yookassa.go"
# Затем UI:
Task: "Catalog в frontend/src/pages/Catalog.tsx"
Task: "Course в frontend/src/pages/Course.tsx"
```

## Implementation Strategy

- MVP: Phase 1+2 + US4 + US1 → стоп, валидация покупки, демо
- Инкремент: + US2 (контент) → + US3 (обучение) → + US5/US6 → Polish
- Каждый чекпоинт — независимо тестируемый инкремент без поломки предыдущего
