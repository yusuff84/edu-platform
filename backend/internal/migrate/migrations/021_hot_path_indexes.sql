-- 021_hot_path_indexes: индексы под частые запросы публичных страниц и входа.
--
-- * users: GetUserByEmail ищет по lower(email) (вход, регистрация, приглашения,
--   OAuth) — UNIQUE-индекс по email для такого выражения не используется.
-- * courses: GetCourseBySlug ищет по lower(slug) (страница курса, превью,
--   отзывы по slug); teacher_id — курсы преподавателя (кабинет, публичный
--   профиль, аналитика, проверка ON DELETE RESTRICT при удалении пользователя).
-- * payments: платежи ученика (/me/payments, повторная покупка —
--   FindPendingPayment по student_id + course_id, сводка зависимостей).
-- * lesson_progress: первичный ключ начинается со student_id; выборки и
--   каскадное удаление по lesson_id без индекса читали всю таблицу.
--
-- Файл идемпотентен: IF NOT EXISTS.

CREATE INDEX IF NOT EXISTS idx_users_email_lower ON users (lower(email));
CREATE INDEX IF NOT EXISTS idx_courses_slug_lower ON courses (lower(slug));
CREATE INDEX IF NOT EXISTS idx_courses_teacher_created ON courses (teacher_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_payments_student_course ON payments (student_id, course_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_lesson_progress_lesson ON lesson_progress (lesson_id);
