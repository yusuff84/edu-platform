-- 005_price_fix: цена демо-курса была 4900 копеек (49 ₽), а задумывалась 4 900 ₽.
-- Идемпотентно: правим только старое значение.
UPDATE courses SET price_cents = 490000 WHERE slug = 'python-s-nulya' AND price_cents = 4900;
