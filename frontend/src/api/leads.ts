// Заявки «Поможем подобрать курс» (backend: handler/leads.go).
import { apiFetch } from './client';

export type LeadContactMethod = 'phone' | 'telegram' | 'vk' | 'max';

export type CreateLeadInput = {
  name: string;
  phone: string;
  email: string;
  contactMethod: LeadContactMethod;
  consent: boolean;
  /** Поле-ловушка для ботов: у людей всегда пустое. */
  website: string;
};

export type Lead = {
  id: string;
  name: string;
  phone: string;
  email: string;
  contactMethod: LeadContactMethod;
  status: 'new' | 'done';
  createdAt: string;
};

/** POST /leads — публично. Ошибки валидации приходят русским текстом (400). */
export function createLead(input: CreateLeadInput): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>('/leads', { method: 'POST', body: JSON.stringify(input) });
}

/** GET /admin/leads — только админ. */
export function adminListLeads(): Promise<{ items: Lead[]; newCount: number }> {
  return apiFetch<{ items: Lead[]; newCount: number }>('/admin/leads');
}

/** POST /admin/leads/{id}/status — отметить обработанной или вернуть в новые. */
export function adminSetLeadStatus(id: string, status: Lead['status']): Promise<void> {
  return apiFetch<void>(`/admin/leads/${encodeURIComponent(id)}/status`, {
    method: 'POST',
    body: JSON.stringify({ status }),
  });
}
