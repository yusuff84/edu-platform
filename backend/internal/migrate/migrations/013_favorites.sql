-- 013_favorites: избранные курсы ученика (T73).
-- Пара (user_id, course_id) уникальна: повторное добавление идемпотентно.
-- Обе ссылки удаляются каскадом: удаление пользователя или курса убирает
-- соответствующие строки избранного. Runner (internal/migrate) выполняет
-- файл ЦЕЛИКОМ при каждом запуске, поэтому действие идемпотентно по стилю
-- соседних миграций: IF NOT EXISTS.

CREATE TABLE IF NOT EXISTS favorites (
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  course_id UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, course_id)
);

-- Список «Избранное»: выборка по пользователю, добавленные позже — первыми.
CREATE INDEX IF NOT EXISTS idx_favorites_user_created
  ON favorites (user_id, created_at DESC);
