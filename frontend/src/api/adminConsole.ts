// T27: API-клиент супер-админских разделов «Сводка» и «Курсы».
// Контракт — Task/T25-admin-console.md (поля и коды берутся оттуда буквально,
// бэкенд T25 пишется параллельно). Транспорт — общий apiFetch/buildQuery из
// client.ts: он экспортирован специально для модулей-компаньонов волны P3,
// дублировать fetch и обработку envelope {"error"} здесь нельзя.
import { apiFetch, buildQuery } from './client';
import type { CourseStatus } from './client';

/** Агрегаты сводки (GET /admin/stats). Денежные суммы — в копейках. */
export type AdminStats = {
  users: {
    total: number;
    students: number;
    teachers: number;
    admins: number;
    blocked: number;
    newLast7: number;
  };
  courses: {
    total: number;
    draft: number;
    review: number;
    published: number;
    archived: number;
  };
  payments: {
    count: number;
    succeededCents: number;
    platformFeeCents: number;
    teacherCents: number;
    last30Cents: number;
  };
  enrollments: {
    total: number;
    active: number;
  };
  reviews: {
    total: number;
    avgRating: number;
  };
};

/** Строка курса в админском списке (GET /admin/courses). */
export type AdminCourseRow = {
  id: string;
  slug: string;
  title: string;
  teacherId: string;
  teacherEmail: string;
  status: CourseStatus;
  priceCents: number;
  createdAt: string;
  updatedAt: string;
  modulesCount: number;
  lessonsCount: number;
  studentsCount: number;
};

/** Страница админского списка курсов: total — до пагинации. */
export type AdminCoursePage = {
  items: AdminCourseRow[];
  total: number;
  page: number;
  limit: number;
};

/** Строка отзыва курса (GET /admin/courses/{id}/reviews). */
export type AdminReviewRow = {
  id: string;
  studentId: string;
  studentEmail: string;
  rating: number;
  body: string;
  createdAt: string;
};

/** GET /admin/stats — все агрегаты сводки одним запросом. */
export function getAdminStats(): Promise<AdminStats> {
  return apiFetch<AdminStats>('/admin/stats');
}

/**
 * GET /admin/courses — курсы всех статусов, новые первыми.
 * Пустые параметры buildQuery не отправляет; невалидный status backend
 * отвергает 400 (в UI такому значению появиться неоткуда — фильтр закрытый).
 */
export function listAdminCourses(params: {
  status?: string;
  q?: string;
  page?: number;
  limit?: number;
}): Promise<AdminCoursePage> {
  return apiFetch<AdminCoursePage>(
    `/admin/courses${buildQuery({
      status: params.status,
      q: params.q,
      page: params.page,
      limit: params.limit,
    })}`,
  );
}

/** POST /admin/courses/{id}/archive — 200 {course}; 409, если уже в архиве; 404. */
export function archiveAdminCourse(id: string): Promise<{ course: AdminCourseRow }> {
  return apiFetch<{ course: AdminCourseRow }>(
    `/admin/courses/${encodeURIComponent(id)}/archive`,
    { method: 'POST' },
  );
}

/** POST /admin/courses/{id}/unpublish — published → draft; иначе 409; 404. */
export function unpublishAdminCourse(id: string): Promise<{ course: AdminCourseRow }> {
  return apiFetch<{ course: AdminCourseRow }>(
    `/admin/courses/${encodeURIComponent(id)}/unpublish`,
    { method: 'POST' },
  );
}

/** GET /admin/courses/{id}/reviews — items всегда массив; 404, если курса нет. */
export function listCourseReviews(courseId: string): Promise<{ items: AdminReviewRow[] }> {
  return apiFetch<{ items: AdminReviewRow[] }>(
    `/admin/courses/${encodeURIComponent(courseId)}/reviews`,
  );
}

/** DELETE /admin/reviews/{id} — 204 без тела; 404, если отзыва нет. */
export function deleteReview(id: string): Promise<void> {
  return apiFetch<void>(`/admin/reviews/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  });
}
