-- 012_lesson_questions: вопросы и ответы по урокам (T71 — публичный
-- GET /courses/{id}/questions, POST вопрос/ответ, DELETE своих сообщений).
-- Runner (internal/migrate) выполняет файл ЦЕЛИКОМ и при каждом запуске
-- заново, поэтому всё действие идемпотентно: CREATE TABLE/INDEX IF NOT EXISTS.
-- FK — каскадом: удаление курса/урока/пользователя/вопроса убирает связанные
-- сообщения (контракт: удаление вопроса удаляет ответы каскадом).

CREATE TABLE IF NOT EXISTS lesson_questions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  course_id UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
  lesson_id UUID NOT NULL REFERENCES lessons(id) ON DELETE CASCADE,
  student_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  body TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS lesson_answers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  question_id UUID NOT NULL REFERENCES lesson_questions(id) ON DELETE CASCADE,
  author_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  body TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Лента вопросов курса (с фильтром по уроку) и ответы вопроса — по времени.
CREATE INDEX IF NOT EXISTS idx_lesson_questions_course_lesson_created
  ON lesson_questions(course_id, lesson_id, created_at);
CREATE INDEX IF NOT EXISTS idx_lesson_answers_question_created
  ON lesson_answers(question_id, created_at);
