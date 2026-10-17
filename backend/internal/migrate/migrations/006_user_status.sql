-- 006_user_status: статус учётной записи (T24 — блокировка в супер-админке).
-- Runner (internal/migrate) выполняет файл ЦЕЛИКОМ и при каждом запуске заново,
-- поэтому все действия идемпотентны: ADD COLUMN IF NOT EXISTS + проверка
-- существования CHECK-констрейнта отдельным DO-блоком (ALTER TABLE ...
-- ADD CONSTRAINT не поддерживает IF NOT EXISTS и упал бы при повторе).

ALTER TABLE users ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_status_check') THEN
    ALTER TABLE users ADD CONSTRAINT users_status_check CHECK (status IN ('active','blocked'));
  END IF;
END $$;
