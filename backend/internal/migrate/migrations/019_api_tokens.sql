-- 019_api_tokens: токены доступа для MCP-сервера (нейросеть управляет курсами).
-- Хранится только SHA-256 токена: сам токен показывается администратору один
-- раз при создании. Токен действует, пока его владелец — активный администратор
-- и токен не отозван. Файл идемпотентен (IF NOT EXISTS).

CREATE TABLE IF NOT EXISTS api_tokens (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  prefix TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_used_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_api_tokens_user ON api_tokens (user_id, created_at DESC);
