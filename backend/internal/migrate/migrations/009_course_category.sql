-- 009_course_category: категория курса (T65 — GET /categories и серверные
-- фильтры каталога GET /courses). Runner (internal/migrate) выполняет файл
-- ЦЕЛИКОМ и при каждом запуске заново, поэтому действие идемпотентно по
-- стилю соседних миграций: ADD COLUMN IF NOT EXISTS. Существующие курсы
-- получают 'other' (NOT NULL DEFAULT).

ALTER TABLE courses ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT 'other';
