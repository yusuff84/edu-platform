-- 020_admin_audit: журнал действий администраторов (кто, что и когда сделал
-- в админке или через MCP). Пишется middleware adminAudit после успешных
-- меняющих запросов к /admin/* и вызовов инструментов MCP. Файл идемпотентен.

CREATE TABLE IF NOT EXISTS admin_audit (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
  actor_email TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL,
  ip TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_admin_audit_created ON admin_audit (created_at DESC);
