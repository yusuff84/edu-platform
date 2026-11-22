import { apiFetch } from './client';

interface AccountSecurity {
  email: string;
  emailVerified: boolean;
  emailVerifiedAt: string | null;
  emailDeliveryAvailable: boolean;
}

export interface AccountSession {
  id: string;
  current: boolean;
  userAgent: string;
  ipAddress: string;
  createdAt: string;
  lastSeenAt: string;
  expiresAt: string;
}

export const getAccountSecurity = () => apiFetch<AccountSecurity>('/me/security');
export const requestEmailVerification = () => apiFetch<{ message: string }>('/me/email/verification', { method: 'POST' });
export const verifyEmail = (token: string) => apiFetch<{ message: string }>('/auth/email/verify', { method: 'POST', body: JSON.stringify({ token }) });
export const forgotPassword = (email: string) => apiFetch<{ message: string }>('/auth/password/forgot', { method: 'POST', body: JSON.stringify({ email }) });
export const resetPassword = (token: string, password: string) => apiFetch<{ message: string }>('/auth/password/reset', { method: 'POST', body: JSON.stringify({ token, password }) });
export const getAccountSessions = async () => (await apiFetch<{ items: AccountSession[] }>('/me/sessions')).items;
export const revokeAccountSession = (id: string) => apiFetch<void>(`/me/sessions/${encodeURIComponent(id)}`, { method: 'DELETE' });
export const revokeOtherSessions = () => apiFetch<void>('/me/sessions/revoke-others', { method: 'POST' });
