// T49: API-клиент экрана «Записи на курсы» супер-админки (/admin/enrollments).
// Транспорт берётся из client.ts (apiFetch/buildQuery), не дублируется.
// Контракт — docs/api-contract.md: GET /admin/enrollments (T25) с
// фильтрами q/status из волны P6 (T48) и ручной отзыв/возврат доступа (T16).
// Тела запросов не шлём: POST revoke/unrevoke без body — backend так и ждёт.

import { apiFetch, buildQuery } from './client';

/** Статус записи на курс (repo.EnrollmentStatus: active|refunded|revoked). */
export type AdminEnrollmentStatus = 'active' | 'refunded' | 'revoked';

/** Строка списка записей GET /admin/enrollments. */
export type AdminEnrollmentRow = {
  id: string;
  studentId: string;
  studentEmail: string;
  /** Может быть пустой строкой — UI показывает email. */
  studentName: string;
  courseId: string;
  courseTitle: string;
  status: AdminEnrollmentStatus;
  createdAt: string;
  /** Пройденные уроки курса и всего уроков; percent считает backend. */
  completedLessons: number;
  totalLessons: number;
  percent: number;
};

/** Страница списка записей: конверт пагинации, как в остальных API. */
export type AdminEnrollmentPage = {
  items: AdminEnrollmentRow[];
  total: number;
  page: number;
  limit: number;
};

/** Ответ POST revoke/unrevoke (T16): новая пара ученик↔курс со статусом. */
type AdminEnrollmentStatusResponse = {
  studentId: string;
  courseId: string;
  status: AdminEnrollmentStatus;
};

/**
 * GET /admin/enrollments — фильтры q (email/имя ученика), courseId, status
 * (active|refunded|revoked), пагинация page/limit. Пустые фильтры buildQuery
 * пропускает (для backend это «любой»).
 */
export function listAdminEnrollments(params: {
  q?: string;
  courseId?: string;
  status?: string;
  page?: number;
  limit?: number;
}): Promise<AdminEnrollmentPage> {
  return apiFetch<AdminEnrollmentPage>(
    `/admin/enrollments${buildQuery({
      q: params.q,
      courseId: params.courseId,
      status: params.status,
      page: params.page,
      limit: params.limit,
    })}`,
  );
}

/** POST .../revoke — ручной отзыв доступа; 200 {studentId,courseId,status:"revoked"}; 404. */
export function revokeEnrollment(
  studentId: string,
  courseId: string,
): Promise<AdminEnrollmentStatusResponse> {
  return apiFetch<AdminEnrollmentStatusResponse>(
    `/admin/enrollments/${encodeURIComponent(studentId)}/courses/${encodeURIComponent(courseId)}/revoke`,
    { method: 'POST' },
  );
}

/** POST .../unrevoke — возврат доступа; 200 {studentId,courseId,status:"active"}; 404. */
export function unrevokeEnrollment(
  studentId: string,
  courseId: string,
): Promise<AdminEnrollmentStatusResponse> {
  return apiFetch<AdminEnrollmentStatusResponse>(
    `/admin/enrollments/${encodeURIComponent(studentId)}/courses/${encodeURIComponent(courseId)}/unrevoke`,
    { method: 'POST' },
  );
}

/** Ответ ручной выдачи доступа POST /admin/enrollments/grant. */
export type GrantEnrollmentResponse = {
  studentId: string;
  email: string;
  courseId: string;
  courseTitle: string;
  status: 'active';
  /** Запись создана сейчас (раньше доступа не было). */
  created: boolean;
  /** Доступ уже был активен — ничего не изменилось. */
  wasActive: boolean;
};

/**
 * POST /admin/enrollments/grant — открыть пользователю доступ к курсу без
 * оплаты (в т.ч. к платному; отозванный доступ восстанавливается).
 * 404 — пользователь не зарегистрирован или курса нет; 400 — черновик.
 */
export function grantEnrollment(email: string, courseId: string): Promise<GrantEnrollmentResponse> {
  return apiFetch<GrantEnrollmentResponse>('/admin/enrollments/grant', {
    method: 'POST',
    body: JSON.stringify({ email: email.trim(), courseId }),
  });
}
