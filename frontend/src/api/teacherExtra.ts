// API жизненного цикла курса в конструкторе (T28; контракт — T21).
// Транспорт — apiFetch из api/client.ts (чужой файл не трогаем, транспорт
// не дублируем). Пути, коды и тела ответов — буквально из
// Task/T21-course-lifecycle.md: без самодеятельных полей, иначе
// DisallowUnknownFields на backend ответит 400.
//
// Все пять операций живут на префиксе /teacher/courses (requireAuth + роль
// teacher|admin уже навешены роутером backend).

import { apiFetch } from './client';
import type { Course, Lesson } from './types';

/** POST /teacher/courses/{id}/duplicate → 201 {"course":{...}}. */
export function duplicateCourse(id: string): Promise<{ course: Course }> {
  return apiFetch<{ course: Course }>(`/teacher/courses/${id}/duplicate`, {
    method: 'POST',
  });
}

/** POST /teacher/courses/{id}/archive → 200 {"course":{...}}; 409 — уже archived. */
export function archiveCourse(id: string): Promise<{ course: Course }> {
  return apiFetch<{ course: Course }>(`/teacher/courses/${id}/archive`, {
    method: 'POST',
  });
}

/** POST /teacher/courses/{id}/unarchive → 200 {"course":{...}}; 409 — не archived. */
export function unarchiveCourse(id: string): Promise<{ course: Course }> {
  return apiFetch<{ course: Course }>(`/teacher/courses/${id}/unarchive`, {
    method: 'POST',
  });
}

/** POST /teacher/courses/{id}/withdraw → 200 {"course":{...}}; 409 — не review. */
export function withdrawCourse(id: string): Promise<{ course: Course }> {
  return apiFetch<{ course: Course }>(`/teacher/courses/${id}/withdraw`, {
    method: 'POST',
  });
}

/**
 * POST /teacher/courses/{courseId}/lessons/{lessonId}/move → 200 {"lesson":{...}}.
 * Тело строго {"moduleId"} (контракт T21). Ошибки: 400 — чужой модуль,
 * 409 — курс не в черновике, 404 — нет урока/модуля, 403 — чужой курс.
 */
export function moveLesson(
  courseId: string,
  lessonId: string,
  moduleId: string,
): Promise<{ lesson: Lesson }> {
  return apiFetch<{ lesson: Lesson }>(
    `/teacher/courses/${courseId}/lessons/${lessonId}/move`,
    {
      method: 'POST',
      body: JSON.stringify({ moduleId }),
    },
  );
}
