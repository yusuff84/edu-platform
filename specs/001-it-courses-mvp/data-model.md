# Data Model: IT-курсы MVP

**Источник**: specs/001-it-courses-mvp/spec.md
**Дата**: 2026-10-06

## Сущности

### User
- `id UUID PK`, `email CITEXT UNIQUE NOT NULL`, `password_hash TEXT NOT NULL`, `role ENUM(student, teacher, admin) NOT NULL DEFAULT student`, `display_name TEXT`, `created_at TIMESTAMPTZ`
- Валидация: email RFC, пароль ≥8 символов; argon2id.
- Переходы: student → teacher (по принятому invite), любая → admin (только сид/админ).

### TeacherInvite
- `id UUID PK`, `email CITEXT NOT NULL`, `token_hash TEXT UNIQUE NOT NULL`, `expires_at TIMESTAMPTZ NOT NULL`, `status ENUM(sent, accepted, expired, revoked)`, `created_by UUID FK users`, `created_at`
- Правила: токен одноразовый, TTL 7 дней, повторная отправка отзывает старый.

### Course
- `id UUID PK`, `teacher_id UUID FK users NOT NULL`, `slug CITEXT UNIQUE NOT NULL`, `title TEXT NOT NULL`, `description TEXT NOT NULL`, `cover_path TEXT`, `price_cents INT NOT NULL CHECK >=0`, `currency CHAR(3) DEFAULT RUB`, `status ENUM(draft, review, published, archived) DEFAULT draft`, `avg_rating NUMERIC(2,1) DEFAULT 0`, `ratings_count INT DEFAULT 0`, `created_at`, `updated_at`
- Правила: в каталоге только `published`; удаление published с enrollments запрещено (только archived); цена меняется только для новых покупок.

### Module
- `id UUID PK`, `course_id UUID FK courses ON DELETE CASCADE`, `title TEXT NOT NULL`, `position INT NOT NULL`
- Уникальность: `(course_id, position)`; переупорядочивание — перенумерация в транзакции.

### Lesson
- `id UUID PK`, `module_id UUID FK modules ON DELETE CASCADE`, `title TEXT NOT NULL`, `kind ENUM(video_text, live, quiz, code) NOT NULL`, `position INT NOT NULL`, `body_md TEXT` (текст/описание), `video_url TEXT`, `live_url TEXT`, `starts_at TIMESTAMPTZ`, `quiz JSONB` (вопросы, варианты, пояснения), `code_task JSONB` (язык python|js|htmlcss, шаблон, скрытые тесты), `created_at`
- Уникальность: `(module_id, position)`.
- Валидация по kind: video_text требует body_md или video_url; live — live_url+starts_at; quiz — ≥1 вопрос; code — язык+тесты.

### Enrollment
- `id UUID PK`, `student_id UUID FK`, `course_id UUID FK`, `status ENUM(active, refunded, revoked) DEFAULT active`, `created_at`
- Уникальность: `(student_id, course_id)` — защита от двойной покупки.

### LessonProgress
- `student_id UUID FK`, `lesson_id UUID FK`, `completed_at TIMESTAMPTZ NOT NULL DEFAULT now()`, `attempts INT DEFAULT 1`
- PK `(student_id, lesson_id)`. Прогресс курса = completed / total. "Продолжить" = первый incomplete по (module.position, lesson.position).

### Payment
- `id UUID PK`, `student_id UUID FK`, `course_id UUID FK`, `amount_cents INT NOT NULL`, `platform_fee_cents INT NOT NULL`, `teacher_amount_cents INT NOT NULL`, `currency CHAR(3)`, `provider TEXT NOT NULL` (mock|stripe|yookassa), `provider_payment_id TEXT UNIQUE`, `status ENUM(pending, succeeded, failed, refunded)`, `created_at`, `processed_at`
- Правила: `amount = platform_fee + teacher_amount`; `platform_fee` считается из действующего `commission_pct` на момент создания; webhook идемпотентен по `provider_payment_id`.

### Review
- `id UUID PK`, `student_id UUID FK`, `course_id UUID FK`, `rating SMALLINT CHECK 1..5`, `body TEXT`, `created_at`
- Уникальность: `(student_id, course_id)` — один отзыв. Пересчет `avg_rating/ratings_count` в транзакции.

### AppSettings (комиссия)
- `key TEXT PK` (`commission_pct`), `value TEXT`, `updated_at`
- Значение 0..90, default 15. Применяется только к новым платежам.

## Связи
- User 1—N Course (как teacher), User 1—N Enrollment (как student)
- Course 1—N Module 1—N Lesson
- Course 1—N Enrollment, Enrollment + Lesson → LessonProgress
- Course 1—N Payment, 1—N Review

## Индексы
- `courses(status, updated_at)`, `courses(slug)`, `modules(course_id, position)`, `lessons(module_id, position)`, `enrollments(student_id)`, `payments(provider_payment_id) UNIQUE`, `reviews(course_id, created_at)`.
