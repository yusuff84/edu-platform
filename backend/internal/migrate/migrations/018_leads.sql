-- 018_leads: заявки на консультацию с главной страницы («Поможем подобрать курс»).
-- До этой миграции форма только показывала «Заявка отправлена» и ничего не
-- сохраняла — контакты посетителей терялись. Runner выполняет файл целиком
-- при каждом запуске, поэтому всё идемпотентно (IF NOT EXISTS).

CREATE TABLE IF NOT EXISTS leads (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  phone TEXT NOT NULL DEFAULT '',
  email TEXT NOT NULL DEFAULT '',
  contact_method TEXT NOT NULL DEFAULT 'phone'
    CHECK (contact_method IN ('phone', 'telegram', 'vk', 'max')),
  status TEXT NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'done')),
  ip_address TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_leads_created ON leads (created_at DESC);
