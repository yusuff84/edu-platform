-- 015_analytics_indexes: индексы под агрегаты аналитики учителя (T77).
--
-- Сводка и разбивка по курсам агрегируют записи/платежи/отзывы по курсам
-- преподавателя. Индексы с course_id первым столбцом позволяют Postgres
-- выбирать строки курса без полного скана таблиц (без N+1 на стороне Go).
-- Файл идемпотентен: повторный запуск миграций безопасен.

CREATE INDEX IF NOT EXISTS idx_enrollments_course_status
  ON enrollments (course_id, status);

CREATE INDEX IF NOT EXISTS idx_payments_course_status_created
  ON payments (course_id, status, created_at);

CREATE INDEX IF NOT EXISTS idx_reviews_course_rating
  ON reviews (course_id, rating);
