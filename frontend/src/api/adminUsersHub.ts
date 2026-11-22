// Новая админка, раздел «Пользователи»: обёртки, которых нет в adminUsers/adminOps.
// Отдельного GET /admin/users/{id} у бэкенда нет — карточку пользователя отдаёт
// GET /admin/users/{id}/courses (в ответе есть и сам пользователь, и его курсы).
import { apiFetch, paymentJournal } from './client';
import type { PaymentJournalItem } from './types';
import type { AdminUserRow } from './adminUsers';
import type { AdminUserCourse } from './adminOps';
import { adminListCertificates, type AdminCertificate } from './adminOps';
import { listAdminCourses, type AdminCourseRow } from './adminConsole';

type AdminUserCard = { user: AdminUserRow; items: AdminUserCourse[] };

/** Ключи кэша раздела: всё про одного пользователя — под ['admin','user',id]. */
export const usersListKey = ['admin', 'users'] as const;
export const userKey = (id: string) => ['admin', 'user', id] as const;

/** Пользователь + его записи на курсы (прогресс, статус доступа, сертификат). */
export function getAdminUserCard(id: string): Promise<AdminUserCard> {
  return apiFetch<AdminUserCard>(`/admin/users/${encodeURIComponent(id)}/courses`);
}

/**
 * Платежи пользователя. Журнал ищет по подстроке email/имени, поэтому
 * результат дополнительно фильтруется по studentId.
 */
export async function listUserPayments(user: { id: string; email: string }): Promise<PaymentJournalItem[]> {
  const res = await paymentJournal({ q: user.email, limit: 100 });
  return res.items.filter((p) => p.studentId === user.id);
}

/** Сертификаты пользователя (поиск по email, затем точный фильтр по userId). */
export async function listUserCertificates(user: { id: string; email: string }): Promise<AdminCertificate[]> {
  const res = await adminListCertificates(user.email);
  return res.items.filter((c) => c.userId === user.id);
}

/** Курсы, где пользователь — преподаватель (поиск по email, затем точный фильтр по teacherId). */
export async function listUserTeaching(user: { id: string; email: string }): Promise<AdminCourseRow[]> {
  const res = await listAdminCourses({ teacherEmail: user.email, limit: 100 });
  return res.items.filter((c) => c.teacherId === user.id);
}
