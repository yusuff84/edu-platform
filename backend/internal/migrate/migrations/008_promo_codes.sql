-- 008_promo_codes: промокоды курсов (T63 — создание, проверка, покупка
-- со скидкой). Runner (internal/migrate) выполняет файл ЦЕЛИКОМ и при
-- каждом запуске заново, поэтому действия идемпотентны по стилю соседних
-- миграций: CREATE TABLE/INDEX IF NOT EXISTS, ADD COLUMN IF NOT EXISTS.
--
-- Контракт — Task/00-api-contract.md, раздел «T63 — промокоды» (заморожен):
-- code хранится в верхнем регистре, max_uses = 0 — без ограничения,
-- used_count увеличивается ровно один раз при успешной оплате.
-- Payments получают аддитивные promo_code (пустая строка — покупка без кода)
-- и discount_cents (разница с полной ценой курса).

CREATE TABLE IF NOT EXISTS promo_codes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  course_id UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
  code TEXT NOT NULL,
  discount_pct INT NOT NULL CHECK (discount_pct >= 1 AND discount_pct <= 90),
  max_uses INT NOT NULL DEFAULT 0 CHECK (max_uses >= 0),
  used_count INT NOT NULL DEFAULT 0 CHECK (used_count >= 0),
  expires_at TIMESTAMPTZ,
  active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (course_id, code)
);

-- Список промокодов курса в GET /teacher/courses/{id}/promo-codes:
-- новые первыми (createdAt DESC).
CREATE INDEX IF NOT EXISTS idx_promo_codes_course_created
  ON promo_codes(course_id, created_at DESC);

-- T63: платёж хранит применённый код и размер скидки (аддитивные поля,
-- старые строки получают пустую строку и 0 — обратная совместимость).
ALTER TABLE payments ADD COLUMN IF NOT EXISTS promo_code TEXT NOT NULL DEFAULT '';
ALTER TABLE payments ADD COLUMN IF NOT EXISTS discount_cents INT NOT NULL DEFAULT 0;
