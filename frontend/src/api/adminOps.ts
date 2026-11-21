// Расширенная админка (backend: handler/admin_ops.go).
import { apiFetch, buildQuery } from './client';

export type AdminRole = 'student' | 'teacher' | 'admin';

export type AdminUserCourse = {
  courseId: string;
  courseTitle: string;
  status: 'active' | 'refunded' | 'revoked' | string;
  enrolledAt: string;
  completedLessons: number;
  totalLessons: number;
  certificateNumber: string;
};

export type AdminCertificate = {
  number: string;
  issuedAt: string;
  userId: string;
  email: string;
  studentName: string;
  courseId: string;
  courseTitle: string;
};

export type SiteSettings = Partial<Record<
  'promo_text' | 'promo_link_text' | 'promo_link_url' | 'contact_phone' | 'contact_email' | 'contact_city' | 'footer_tagline',
  string
>>;

const json = (method: string, body?: unknown): RequestInit => ({
  method,
  ...(body === undefined ? {} : { body: JSON.stringify(body) }),
});

export function adminCreateUser(input: { email: string; password?: string; role: AdminRole; displayName?: string }) {
  return apiFetch<{ user: { id: string; email: string; role: AdminRole }; password: string }>('/admin/users', json('POST', input));
}

export function adminUpdateUserProfile(id: string, input: { email?: string; displayName?: string }) {
  return apiFetch<unknown>(`/admin/users/${encodeURIComponent(id)}/profile`, json('PATCH', input));
}

/** Пустой пароль — сгенерировать; сгенерированный возвращается в ответе один раз. */
export function adminSetUserPassword(id: string, password?: string) {
  return apiFetch<{ password: string }>(`/admin/users/${encodeURIComponent(id)}/password`, json('POST', { password: password ?? '' }));
}

export function adminLogoutUser(id: string) {
  return apiFetch<void>(`/admin/users/${encodeURIComponent(id)}/logout-all`, json('POST'));
}

export function adminUserCourses(id: string) {
  return apiFetch<{ items: AdminUserCourse[] }>(`/admin/users/${encodeURIComponent(id)}/courses`);
}

const pairPath = (studentId: string, courseId: string, action: string): string =>
  `/admin/enrollments/${encodeURIComponent(studentId)}/courses/${encodeURIComponent(courseId)}/${action}`;

export function adminResetProgress(studentId: string, courseId: string) {
  return apiFetch<{ deleted: number }>(pairPath(studentId, courseId, 'reset-progress'), json('POST'));
}

export function adminCompleteCourse(studentId: string, courseId: string) {
  return apiFetch<{ certificateNumber: string }>(pairPath(studentId, courseId, 'complete'), json('POST'));
}

export function adminListCertificates(q: string) {
  return apiFetch<{ items: AdminCertificate[]; total: number }>(`/admin/certificates${buildQuery({ q: q || undefined })}`);
}

export function adminIssueCertificate(input: { email?: string; studentId?: string; courseId: string }) {
  return apiFetch<{ certificateNumber: string }>('/admin/certificates', json('POST', input));
}

export function adminRevokeCertificate(number: string) {
  return apiFetch<void>(`/admin/certificates/${encodeURIComponent(number)}`, json('DELETE'));
}

export function adminPublishCourse(id: string) {
  return apiFetch<unknown>(`/admin/courses/${encodeURIComponent(id)}/publish`, json('POST'));
}

export function adminSetCourseOwner(id: string, email: string) {
  return apiFetch<{ teacherEmail: string }>(`/admin/courses/${encodeURIComponent(id)}/owner`, json('POST', { email }));
}

export function adminGetSiteSettings() {
  return apiFetch<{ settings: SiteSettings }>('/admin/site-settings');
}

export function adminPutSiteSettings(settings: SiteSettings) {
  return apiFetch<{ settings: SiteSettings }>('/admin/site-settings', json('PUT', { settings }));
}

export type McpToken = {
  id: string;
  userEmail: string;
  name: string;
  prefix: string;
  createdAt: string;
  lastUsedAt: string | null;
  revokedAt: string | null;
};

export function adminListMcpTokens() {
  return apiFetch<{ items: McpToken[] }>('/admin/mcp-tokens');
}

/** Токен возвращается один раз — дальше в списке виден только префикс. */
export function adminCreateMcpToken(name: string) {
  return apiFetch<{ token: string; item: McpToken }>('/admin/mcp-tokens', json('POST', { name }));
}

export function adminRevokeMcpToken(id: string) {
  return apiFetch<void>(`/admin/mcp-tokens/${encodeURIComponent(id)}`, json('DELETE'));
}
