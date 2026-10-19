-- 011_review_reply: публичный ответ преподавателя на отзыв (T69 —
-- PUT/DELETE /teacher/courses/{id}/reviews/{reviewId}/reply,
-- поля teacherReply/teacherReplyAt во всех видах Review). Runner
-- (internal/migrate) выполняет файл ЦЕЛИКОМ и при каждом запуске заново,
-- поэтому действие идемпотентно: ADD COLUMN IF NOT EXISTS / SET DEFAULT.
-- teacher_reply_at NULL — ответа нет; текст ответа хранится пустой строкой
-- (контракт: teacherReply — всегда строка, пустая, если ответа нет).

ALTER TABLE reviews ADD COLUMN IF NOT EXISTS teacher_reply TEXT NOT NULL DEFAULT '';
ALTER TABLE reviews ADD COLUMN IF NOT EXISTS teacher_reply_at TIMESTAMPTZ;
