-- 002_seed: демо-курс "Python с нуля" для ЛОКАЛЬНОЙ РАЗРАБОТКИ.
-- ВАЖНО: migrate.ApplyMigrations пропускает файлы с seed в имени; cmd/api
-- вызывает SeedIfEmpty только при cfg.IsDev(). В production этот файл не
-- применяется, поэтому демо-учёток и курса там не будет.
-- Учётные данные ниже известны и допустимы только в dev; admin@local здесь
-- намеренно отсутствует.

-- Демо-преподаватель с настоящим bcrypt-хешем пароля teach1234 (pgcrypto).
-- id отличается от старого teacher@local (2222...), чтобы сид не конфликтовал
-- по первичному ключу с БД, где старая демо-учётка уже существует.
INSERT INTO users (id, email, password_hash, role, display_name) VALUES
  ('88888888-8888-8888-8888-888888888888', 'demo-teacher@local', crypt('teach1234', gen_salt('bf', 10)), 'teacher', 'Demo Teacher')
ON CONFLICT (email) DO NOTHING;

INSERT INTO courses (id, teacher_id, slug, title, description, price_cents, currency, status) VALUES
  -- 490000 копеек = 4 900 ₽: сид применяется ПОСЛЕ 005_price_fix.sql, поэтому
  -- цена должна быть актуальной сразу (005 исправляет только старые БД).
  ('33333333-3333-3333-3333-333333333333', '88888888-8888-8888-8888-888888888888', 'python-s-nulya', 'Python с нуля', 'Базовый курс Python: синтаксис, функции, практика в песочнице.', 490000, 'RUB', 'published')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO modules (id, course_id, title, position) VALUES
  ('44444444-4444-4444-4444-444444444444', '33333333-3333-3333-3333-333333333333', 'Основы', 1)
ON CONFLICT (id) DO NOTHING;

INSERT INTO lessons (id, module_id, title, kind, position, body_md, video_url) VALUES
  ('55555555-5555-5555-5555-555555555555', '44444444-4444-4444-4444-444444444444', 'Привет, Python', 'video_text', 1,
   '# Привет, Python\n\nУстановка интерпретатора, первая программа `print("hello")`.', NULL)
ON CONFLICT (id) DO NOTHING;

INSERT INTO lessons (id, module_id, title, kind, position, quiz) VALUES
  ('66666666-6666-6666-6666-666666666666', '44444444-4444-4444-4444-444444444444', 'Проверка знаний', 'quiz', 2,
   '{"questions": [{"id": "q1", "text": "Что делает `x = 5`?", "options": ["Присваивает 5", "Сравнивает с 5", "Ошибка"], "correct": 0, "explanation": "Одиночное `=` — присваивание."}, {"id": "q2", "text": "Какой тип у `3.14`?", "options": ["int", "float", "str"], "correct": 1, "explanation": "Дробные числа — float."}]}'::jsonb)
ON CONFLICT (id) DO NOTHING;

INSERT INTO lessons (id, module_id, title, kind, position, body_md, code_task) VALUES
  ('77777777-7777-7777-7777-777777777777', '44444444-4444-4444-4444-444444444444', 'Сумма: код', 'code', 3,
   'Прочитай два числа и выведи их сумму.',
   '{"language": "python", "template": "a, b = map(int, input().split())\nprint(a + b)\n", "tests": [{"name": "пример", "input": "2 3", "expected": "5"}, {"name": "скрытый", "input": "-1 1", "expected": "0", "hidden": true}]}'::jsonb)
ON CONFLICT (id) DO NOTHING;
