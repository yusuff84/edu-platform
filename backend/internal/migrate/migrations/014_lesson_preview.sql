-- 014_lesson_preview: превью-уроки курса (T75 — P9).
-- Флаг is_preview открывает урок для бесплатного публичного просмотра:
-- GET /api/v1/courses/{slug}/preview/{lessonId} отдаёт урок только при
-- isPreview=true и опубликованном курсе. Runner (internal/migrate)
-- выполняет файл ЦЕЛИКОМ при каждом запуске, поэтому действие идемпотентно:
-- ADD COLUMN IF NOT EXISTS. Старые уроки читаются как не-превью (false).

ALTER TABLE lessons ADD COLUMN IF NOT EXISTS is_preview BOOLEAN NOT NULL DEFAULT false;
