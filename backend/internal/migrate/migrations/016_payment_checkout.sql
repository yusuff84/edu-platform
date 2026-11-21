ALTER TABLE payments ADD COLUMN IF NOT EXISTS redirect_url text NOT NULL DEFAULT '';
ALTER TABLE payments ADD COLUMN IF NOT EXISTS checkout_description text NOT NULL DEFAULT '';
ALTER TABLE payments ADD COLUMN IF NOT EXISTS checkout_return_url text NOT NULL DEFAULT '';
