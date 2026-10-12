-- 004_seed_fix: приводит демо-уроки 002_seed.sql к актуальным контрактам.
-- Старый quiz использовал ключи q/answer/hint (декодировались в 0 вопросов),
-- старые code-тесты использовали check-выражения (runner ждёт input/expected).
-- Идемпотентно: обычные UPDATE по фиксированным id.

UPDATE lessons SET quiz = '{"questions": [{"id": "q1", "text": "Что делает `x = 5`?", "options": ["Присваивает 5", "Сравнивает с 5", "Ошибка"], "correct": 0, "explanation": "Одиночное `=` — присваивание."}, {"id": "q2", "text": "Какой тип у `3.14`?", "options": ["int", "float", "str"], "correct": 1, "explanation": "Дробные числа — float."}]}'::jsonb
WHERE id = '66666666-6666-6666-6666-666666666666';

UPDATE lessons SET
  body_md = 'Прочитай два числа и выведи их сумму.',
  code_task = '{"language": "python", "template": "a, b = map(int, input().split())\nprint(a + b)\n", "tests": [{"name": "пример", "input": "2 3", "expected": "5"}, {"name": "скрытый", "input": "-1 1", "expected": "0", "hidden": true}]}'::jsonb
WHERE id = '77777777-7777-7777-7777-777777777777';
