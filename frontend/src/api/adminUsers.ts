// API супер-админки: управление пользователями (T26, контракт — T24).
// Транспорт берётся из client.ts (apiFetch/buildQuery), не дублируется.
// Тела PATCH собираются строго по контракту ({role} / {status}): backend
// включает DisallowUnknownFields, любое лишнее поле = 400.

import { apiFetch, buildQuery } from './client';
import type { Role } from './types';

/** Статус учётной записи (T24: активен / заблокирован). */
export type UserStatus = 'active' | 'blocked';

/** Строка таблицы пользователей GET /admin/users. */
export type AdminUserRow = {
  id: string;
  email: string;
  role: Role;
  /** Может быть пустой строкой — UI показывает «—». */
  displayName: string;
  status: UserStatus;
  createdAt: string;
  /** Курсы, где пользователь преподаватель. */
  coursesCount: number;
  /** Записи пользователя на курсы. */
  enrollmentsCount: number;
};

/** Страница списка пользователей (конверт пагинации, как в остальных API). */
export type AdminUserPage = {
  items: AdminUserRow[];
  total: number;
  page: number;
  limit: number;
};

/** GET /admin/users — фильтры q/role/status, пагинация page/limit.
 *  Пустые фильтры buildQuery пропускает (backend трактует как «любой»). */
export function listUsers(params: {
  q?: string;
  role?: string;
  status?: string;
  page?: number;
  limit?: number;
}): Promise<AdminUserPage> {
  return apiFetch<AdminUserPage>(
    `/admin/users${buildQuery({
      q: params.q,
      role: params.role,
      status: params.status,
      page: params.page,
      limit: params.limit,
    })}`,
  );
}

/** PATCH /admin/users/{id}/role → {"user": {...}} — конверт распаковываем. */
export function setUserRole(id: string, role: Role): Promise<AdminUserRow> {
  return apiFetch<{ user: AdminUserRow }>(
    `/admin/users/${encodeURIComponent(id)}/role`,
    {
      method: 'PATCH',
      body: JSON.stringify({ role }),
    },
  ).then((res) => res.user);
}

/** PATCH /admin/users/{id}/status → {"user": {...}}. */
export function setUserStatus(id: string, status: UserStatus): Promise<AdminUserRow> {
  return apiFetch<{ user: AdminUserRow }>(
    `/admin/users/${encodeURIComponent(id)}/status`,
    {
      method: 'PATCH',
      body: JSON.stringify({ status }),
    },
  ).then((res) => res.user);
}

/** DELETE /admin/users/{id} → 204 (apiFetch вернёт undefined). */
export function deleteUser(id: string): Promise<void> {
  return apiFetch<void>(`/admin/users/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  });
}
