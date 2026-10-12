-- 003_app_compat: приводит схему 001 к контракту repo.Store.
-- Нужна для PostgresStore: сессии, префикс инвайтов, комментарий модерации,
-- nullable completed_at (BumpAttempt создаёт прогресс без завершения).

ALTER TABLE teacher_invites ADD COLUMN IF NOT EXISTS token_prefix TEXT NOT NULL DEFAULT '';
ALTER TABLE courses ADD COLUMN IF NOT EXISTS moderation_comment TEXT NOT NULL DEFAULT '';
ALTER TABLE lesson_progress ALTER COLUMN completed_at DROP NOT NULL;

CREATE TABLE IF NOT EXISTS sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);
