# Implementation Plan: IT-курсы MVP

**Branch**: `001-it-courses-mvp` | **Date**: 2026-10-06 | **Spec**: specs/001-it-courses-mvp/spec.md

**Input**: Feature specification from `/specs/001-it-courses-mvp/spec.md`

## Summary

Платформа IT-курсов для новичков: каталог + страница курса, конструктор учителя (модули/уроки video_text, live-ссылка, quiz, code), модерация админом по приглашениям, гибкая онлайн-оплата через `PaymentProvider`, кабинеты и прогресс. Подход: монорепо React SPA + Go API + Postgres, все в Docker Compose, изолированный code-runner, сессии в cookie, русский UI.

## Technical Context

**Language/Version**: Backend Go 1.22+, Frontend TypeScript 5 + React 18

**Primary Dependencies**: Backend: chi, pgx/v5, golang-migrate, argon2; Frontend: Vite, React Router, TanStack Query, Hook Form + zod; Инфра: Postgres 16, Nginx (статика + /uploads)

**Storage**: PostgreSQL 16 (все доменные данные + сессии), локальный volume `/uploads` за `ObjectStore`-интерфейсом (путь к S3 позже)

**Testing**: Go `testing` + httptest (контракты API), Vitest + Playwright smoke (критические пути: auth, оплата, урок), `docker compose` e2e по quickstart.md

**Target Platform**: Linux Docker, `docker compose up` — api:8080, web:3000, db:5432, runner:8081

**Project Type**: Web application (frontend + backend + runner)

**Performance Goals**: Каталог 100 курсов <2с; 500 конкурентных читателей; автопроверка <10с (таймаут задачи 5с); покупка → доступ <5 мин e2e

**Constraints**: Русский UI через i18n-словарь; email+пароль в v1, Яндекс позже адаптером; live только ссылкой; цена — учитель, комиссия — админ (0–90%, default 15); идемпотентные webhook; архив вместо удаления купленных курсов

**Scale/Scope**: MVP: ~100 курсов, ~1000 пользователей, ~10 экранов (каталог, курс, auth, кабинеты ученик/учитель/админ, конструктор, модерация, оплата, прогресс)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- [x] I. Понятность новичку — каталог→покупка→продолжить в 3 клика, quickstart.md проверяет SC-001/SC-002.
- [x] II. Приглашения+модерация — TeacherInvite одноразовый, статус draft→review→published, без самопубликации.
- [x] III. API-First/Test-First — contracts/openapi.yaml версионирован, тесты до merge для auth/оплаты/прогресса.
- [x] IV. Гибкая оплата — `PaymentProvider` интерфейс (mock|stripe|yookassa), split в домене, webhook unique.
- [x] V. Docker-First/простота — одна команда up, YAGNI: без Next SSR, без S3/Judge0 в MVP.
- [x] VI. Русский + доступность — i18n-словарь, сессии httpOnly, Яндекс как будущий адаптер.

Post-design re-check: нарушений нет, data-model запрещает удаление published с enrollments, runner изолирован без сети.

## Project Structure

### Documentation (this feature)

```text
specs/001-it-courses-mvp/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
│   └── openapi.yaml
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
backend/
├── cmd/api/main.go
├── internal/
│   ├── handler/      # auth, courses, teacher, admin, enroll, progress, webhook
│   ├── service/      # course, payment, progress, invite
│   ├── repo/         # pgx репозитории
│   ├── payments/     # provider.go + mock.go + stripe.go + yookassa.go + factory
│   ├── auth/         # sessions, password, invite
│   └── migrate/migrations/
├── Dockerfile
└── openapi.yaml (генерируется из contracts/)

frontend/
├── src/
│   ├── pages/      # Catalog, Course, Auth, StudentCab, TeacherCab, AdminCab, Constructor
│   ├── components/ # CourseCard, LessonView, QuizForm, CodeEditor
│   ├── api/client.ts
│   └── i18n/ru.json
├── Dockerfile + nginx.conf
└── tests/

runner/
├── main.go           # очередь + воркеры python/node/htmlcss, timeout 5s, no-net
└── Dockerfile

docker-compose.yml
.env.example
```

**Structure Decision**: Web application — выбрана структура backend/frontend/runner + Postgres, т.к. обнаружены обе части. Монорепо с тремя Dockerfile и одним compose для `docker compose up`.

## Complexity Tracking

> Fill ONLY if Constitution Check has violations that must be justified

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |
