// T29: API-клиент экрана «Ученики курса» (/teach/:id/students).
// Контракт — Task/T22-teacher-students.md (бэкенд пишется параллельно);
// имена полей берутся буквально из контракта, ни одного «своего» поля.
// Транспорт (apiFetch/buildQuery) — общий из api/client.ts, чужой файл не правим.

import { apiFetch, buildQuery } from './client';
import type { Course, Module } from './types';

/**
 * Статус записи на курс (контракт T22).
 * EnrollmentStatus из types.ts не экспортируется (на момент T29 такого типа
 * там нет) — локальный союз, прав на types.ts у задачи нет.
 */
export type EnrollmentStatus = 'active' | 'refunded' | 'revoked';

/** Строка таблицы учеников (GET .../students, поле в поле по контракту T22). */
export type CourseStudentRow = {
  studentId: string;
  email: string;
  displayName: string;
  enrolledAt: string;
  status: EnrollmentStatus;
  completedLessons: number;
  totalLessons: number;
  percent: number;
  lastActivityAt: string | null;
};

/** Страница списка учеников (paginate на бэкенде, limit ≤ 100, по умолчанию 20). */
export type CourseStudentPage = {
  items: CourseStudentRow[];
  total: number;
  page: number;
  limit: number;
};

/** Плитки статистики курса (GET .../stats, поле в поле по контракту T22). */
export type CourseStats = {
  studentsTotal: number;
  studentsActive: number;
  studentsCompleted: number;
  avgPercent: number;
  revenueCents: number;
  modulesTotal: number;
  lessonsTotal: number;
  reviewsCount: number;
  avgRating: number;
};

/** GET /teacher/courses/{id}/students — ученики курса с прогрессом. */
export function listCourseStudents(
  courseId: string,
  params: { q?: string; page?: number; limit?: number },
): Promise<CourseStudentPage> {
  return apiFetch<CourseStudentPage>(
    `/teacher/courses/${encodeURIComponent(courseId)}/students${buildQuery({
      q: params.q,
      page: params.page,
      limit: params.limit,
    })}`,
  );
}

/** GET /teacher/courses/{id}/stats — статистика курса для плиток. */
export function courseStats(courseId: string): Promise<CourseStats> {
  return apiFetch<CourseStats>(
    `/teacher/courses/${encodeURIComponent(courseId)}/stats`,
  );
}

/**
 * Конверт GET /teacher/courses/{id} (T13) — курс и его модули как есть.
 * T36: под этим конвертом данные лежат в общем кэше конструктора и страницы
 * «Ученики курса» (ключ ['teach-course', id]) — форма ответа у обеих страниц
 * должна совпадать, поэтому функция ничего не распаковывает.
 */
export type TeacherCourseDetail = {
  course: Course;
  modules: Module[];
};

/** GET /teacher/courses/{id} — конверт { course, modules } без распаковки (T36). */
export function getTeacherCourseDetail(courseId: string): Promise<TeacherCourseDetail> {
  return apiFetch<TeacherCourseDetail>(
    `/teacher/courses/${encodeURIComponent(courseId)}`,
  );
}

/**
 * GET /teacher/courses/{id} (эндпоинт T13) — только название и статус курса
 * для шапки. Обёртка над getTeacherCourseDetail: странице «Ученики курса»
 * нужен лишь course. Для данных общего кэша используйте getTeacherCourseDetail.
 */
export function getTeacherCourse(courseId: string): Promise<Course> {
  return getTeacherCourseDetail(courseId).then((res) => res.course);
}
