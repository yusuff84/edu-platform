import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { adminListLeads, adminSetLeadStatus } from '../api/leads';
import type { Lead } from '../api/leads';
import { formatDate } from '../api/client';

const CONTACT_LABEL: Record<Lead['contactMethod'], string> = {
  phone: 'Звонок',
  telegram: 'Telegram',
  vk: 'VK',
  max: 'MAX',
};

/**
 * Заявки «Поможем подобрать курс» с главной: контакты, способ связи и
 * отметка «обработана». Новые — первыми; обработанные можно скрыть.
 */
export function AdminLeads(): JSX.Element {
  const qc = useQueryClient();
  const [showDone, setShowDone] = useState(false);
  const leads = useQuery({ queryKey: ['admin-leads'], queryFn: adminListLeads });
  const status = useMutation({
    mutationFn: (v: { id: string; status: Lead['status'] }) => adminSetLeadStatus(v.id, v.status),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['admin-leads'] }),
  });

  const items = (leads.data?.items ?? []).filter((l) => showDone || l.status === 'new');
  const newCount = leads.data?.newCount ?? 0;
  const total = leads.data?.items.length ?? 0;

  return (
    <section id="leads" className="admin-section panel admin-leads">
      <div className="admin-leads-head">
        <h2>
          Заявки на консультацию{' '}
          {newCount > 0 && <span className="admin-leads-badge">{newCount} новых</span>}
        </h2>
        {total > newCount && (
          <label className="admin-leads-toggle">
            <input type="checkbox" checked={showDone} onChange={(e) => setShowDone(e.target.checked)} />
            Показать обработанные
          </label>
        )}
      </div>
      {leads.isLoading && <p role="status">Загрузка…</p>}
      {leads.isError && (
        <p className="error" role="alert">
          {leads.error instanceof Error ? leads.error.message : 'Не удалось загрузить заявки'}
        </p>
      )}
      {leads.data && items.length === 0 && (
        <p className="muted">{total === 0 ? 'Заявок пока нет.' : 'Новых заявок нет.'}</p>
      )}
      {items.length > 0 && (
        <ul className="admin-leads-list">
          {items.map((l) => (
            <li key={l.id} className={`admin-lead${l.status === 'done' ? ' is-done' : ''}`}>
              <div className="admin-lead-main">
                <b>{l.name}</b>
                <a href={`tel:${l.phone}`}>{l.phone}</a>
                {l.email && <a href={`mailto:${l.email}`}>{l.email}</a>}
              </div>
              <div className="admin-lead-meta">
                <span className="pill">{CONTACT_LABEL[l.contactMethod] ?? l.contactMethod}</span>
                <span className="muted">{formatDate(l.createdAt)}</span>
              </div>
              <button
                type="button"
                className="btn btn-sm"
                disabled={status.isPending && status.variables?.id === l.id}
                onClick={() => status.mutate({ id: l.id, status: l.status === 'new' ? 'done' : 'new' })}
              >
                {l.status === 'new' ? 'Обработана' : 'Вернуть в новые'}
              </button>
            </li>
          ))}
        </ul>
      )}
      {status.isError && (
        <p className="error" role="alert">
          {status.error instanceof Error ? status.error.message : 'Не удалось обновить заявку'}
        </p>
      )}
    </section>
  );
}
