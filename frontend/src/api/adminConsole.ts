// T27: API-клиент супер-админских разделов «Сводка» и «Курсы».
// Контракт — docs/api-contract.md (раздел T25) (поля и коды берутся оттуда буквально,
// бэкенд T25 пишется параллельно). Транспорт — общий apiFetch/buildQuery из
// client.ts: он экспортирован специально для модулей-компаньонов волны P3,
// дублировать fetch и обработку envelope {"error"} здесь нельзя.
import { apiFetch, buildQuery } from './client';
import type { CourseStatus } from './client';

/** Агрегаты сводки (GET /admin/stats). Денежные суммы — в копейках. */
type AdminStats = {
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
type AdminCoursePage = {
  items: AdminCourseRow[];
  total: number;
  page: number;
  limit: number;
};

/** Поле сортировки списка курсов (контракт P6, T48): иное backend отвергает 400. */
export type AdminCourseSort = 'createdAt' | 'updatedAt' | 'title' | 'price' | 'students';

/** Направление сортировки (контракт P6, T48): по умолчанию — desc. */
export type AdminCourseDir = 'asc' | 'desc';

/** Параметры админского списка курсов (все необязательны, кроме новых — аддитивных). */
type AdminCourseListParams = {
  status?: string;
  q?: string;
  page?: number;
  limit?: number;
  /** Поле сортировки; по умолчанию backend считает createdAt. */
  sort?: AdminCourseSort;
  /** Направление сортировки; по умолчанию backend считает desc. */
  dir?: AdminCourseDir;
  /** Подстрока по email преподавателя без учёта регистра (T48). */
  teacherEmail?: string;
  /** Нижняя граница периода по createdAt, ГГГГ-ММ-ДД (включительно, T48). */
  from?: string;
  /** Верхняя граница периода по createdAt, ГГГГ-ММ-ДД (включительно, T48). */
  to?: string;
};

/** GET /admin/stats — все агрегаты сводки одним запросом. */
export function getAdminStats(): Promise<AdminStats> {
  return apiFetch<AdminStats>('/admin/stats');
}

/**
 * GET /admin/courses — курсы всех статусов.
 * Пустые параметры buildQuery не отправляет; невалидный status backend
 * отвергает 400 (в UI такому значению появиться неоткуда — фильтр закрытый).
 * Параметры P6 (T48): sort/dir/teacherEmail/from/to — необязательные и
 * аддитивные: старый backend их просто игнорирует.
 */
export function listAdminCourses(params: AdminCourseListParams): Promise<AdminCoursePage> {
  return apiFetch<AdminCoursePage>(
    `/admin/courses${buildQuery({
      status: params.status,
      q: params.q,
      page: params.page,
      limit: params.limit,
      sort: params.sort,
      dir: params.dir,
      teacherEmail: params.teacherEmail,
      from: params.from,
      to: params.to,
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

/**
 * POST /admin/courses/{id}/unarchive — вернуть курс из архива: archived → draft
 * (комментарий модерации сохраняется, контракт P6 / T48). Иначе 409; 404.
 */
export function unarchiveAdminCourse(id: string): Promise<{ course: AdminCourseRow }> {
  return apiFetch<{ course: AdminCourseRow }>(
    `/admin/courses/${encodeURIComponent(id)}/unarchive`,
    { method: 'POST' },
  );
}

/** DELETE /admin/reviews/{id} — 204 без тела; 404, если отзыва нет. */
export function deleteReview(id: string): Promise<void> {
  return apiFetch<void>(`/admin/reviews/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  });
}
