-- 010_certificates: сертификаты о завершении курса (T67).
-- Выдаются автоматически при первом достижении 100% прогресса по курсу;
-- номер EDU-<ГГГГ>-<8 hex ВЕРХНИМ регистром> уникален, пара
-- (user_id, course_id) — тоже (второй сертификат за тот же курс не выдаётся).
-- Runner (internal/migrate) выполняет файл ЦЕЛИКОМ при каждом запуске,
-- поэтому действие идемпотентно по стилю соседних миграций: IF NOT EXISTS.

CREATE TABLE IF NOT EXISTS certificates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  course_id UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
  number TEXT NOT NULL UNIQUE,
  issued_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (user_id, course_id)
);

-- Список «Мои сертификаты»: выборка по пользователю, новые первыми.
CREATE INDEX IF NOT EXISTS idx_certificates_user_issued
  ON certificates (user_id, issued_at DESC);
