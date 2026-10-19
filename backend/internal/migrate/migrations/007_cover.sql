-- 007_cover: обложка курса (T54 — POST/DELETE /teacher/courses/{id}/cover,
-- поле coverPath string|null в видах). Runner (internal/migrate) выполняет
-- файл ЦЕЛИКОМ и при каждом запуске заново, поэтому действие идемпотентно
-- по стилю соседних миграций: ADD COLUMN IF NOT EXISTS. Схема 001_init.sql
-- уже содержит courses.cover_path TEXT — миграция нужна для баз, поднятых
-- раньше первого поколения (и гарантирует колонку при частичном накате).

ALTER TABLE courses ADD COLUMN IF NOT EXISTS cover_path TEXT;
