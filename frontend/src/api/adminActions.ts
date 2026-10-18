// API админ-действий T51: список приглашений, отзыв приглашения, возврат платежа.
// Контракт — раздел «Эндпоинты волны P6 (T47–T52)» в Task/00-api-contract.md.
// Транспорт берётся из client.ts (apiFetch/buildQuery), не дублируется.
// У revoke/refund тело не отправляется: ни контракт, ни backend его не ждут
// (DisallowUnknownFields превратил бы лишнее поле в 400).

import { apiFetch, buildQuery } from './client';
import type { PaymentJournalItem } from './types';

/** Сырой статус приглашения в хранилище; бэкенд его не пересчитывает. */
export type AdminInviteStatus = 'sent' | 'accepted' | 'expired' | 'revoked';

/** Строка списка приглашений GET /admin/invites (контракт T47). */
export type AdminInviteRow = {
  id: string;
  email: string;
  status: AdminInviteStatus;
  /** Префикс токена — сам токен повторно не отдаётся. */
  tokenPrefix: string;
  expiresAt: string;
  createdAt: string;
  createdBy: string;
};

/** Страница списка приглашений (конверт пагинации, как в остальных API). */
export type AdminInvitePage = {
  items: AdminInviteRow[];
  total: number;
  page: number;
  limit: number;
};

/** GET /admin/invites — сортировка createdAt по убыванию (задаёт backend). */
export function listInvites(params?: {
  page?: number;
  limit?: number;
}): Promise<AdminInvitePage> {
  return apiFetch<AdminInvitePage>(
    `/admin/invites${buildQuery({ page: params?.page, limit: params?.limit })}`,
  );
}

/** POST /admin/invites/{id}/revoke → {"invite": {...}} — конверт распаковываем. */
export function revokeInvite(id: string): Promise<AdminInviteRow> {
  return apiFetch<{ invite: AdminInviteRow }>(
    `/admin/invites/${encodeURIComponent(id)}/revoke`,
    { method: 'POST' },
  ).then((res) => res.invite);
}

/** POST /admin/payments/{id}/refund → {"payment": {...}} — элемент журнала. */
export function refundPayment(id: string): Promise<PaymentJournalItem> {
  return apiFetch<{ payment: PaymentJournalItem }>(
    `/admin/payments/${encodeURIComponent(id)}/refund`,
    { method: 'POST' },
  ).then((res) => res.payment);
}
