// Приглашения — отдельная страница /admin/invites.
// Логика и разметка перенесены из секции #invites страницы Admin.tsx:
// создание приглашения, показ и копирование токена, список с пагинацией,
// отзыв приглашения и вычисляемый статус «Истекло».
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { ApiError, createInvite, formatDate } from '../api/client';
import type { Invite } from '../api/client';
import { listInvites, revokeInvite } from '../api/adminActions';
import { t } from '../i18n/t';
import { useSeo } from '../app/useSeo';
import {
  INVITES_LIMIT,
  INVITE_STATUS_LABEL,
  copyToClipboard,
  errorText,
  inviteDisplayStatus,
} from './adminShared';
// Стили страницы изолированы в styles/admin.css — общий styles.css не трогаем.
import '../styles/admin.css';

// Сообщения zod-валидации: ключей ru.json под них нет (та же ситуация,
// что в Auth.tsx/Constructor.tsx/Reviews.tsx) — см. отчёт T08.
const inviteSchema = z.object({
  email: z.string().min(1, 'Укажите email').email('Некорректный email'),
});

type InviteForm = z.infer<typeof inviteSchema>;

export default function AdminInvites(): JSX.Element {
  useSeo(`${t('admin.invites')} — ${t('cabinet.adminTitle')} — Edu Platform`);
  const qc = useQueryClient();

  // ---- Приглашения: список с пагинацией (T51, контракт T47) ----
  const [invitesPage, setInvitesPage] = useState(1);
  const invites = useQuery({
    queryKey: ['admin-invites', { page: invitesPage, limit: INVITES_LIMIT }],
    queryFn: () => listInvites({ page: invitesPage, limit: INVITES_LIMIT }),
    // 4xx — не временный сбой, ретраим только сетевые ошибки (как в журнале).
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  const [invite, setInvite] = useState<Invite | null>(null);
  const [inviteError, setInviteError] = useState('');
  const [tokenCopied, setTokenCopied] = useState(false);
  const [tokenCopyError, setTokenCopyError] = useState('');

  const { register, handleSubmit, reset, formState, clearErrors } = useForm<InviteForm>({
    resolver: zodResolver(inviteSchema),
  });

  const inviteMutation = useMutation({
    mutationFn: (v: InviteForm) => createInvite(v.email.trim()),
    onSuccess: (data) => {
      // Токен виден один раз — сохраняем его в состоянии сразу.
      setInvite(data);
      setInviteError('');
      setTokenCopied(false);
      setTokenCopyError('');
      reset();
      // Список приглашений должен сразу показать только что созданное
      // (сортировка на сервере — новые первыми, поэтому поднимаемся на стр. 1).
      if (invitesPage !== 1) setInvitesPage(1);
      void qc.invalidateQueries({ queryKey: ['admin-invites'] });
    },
    onError: (e: unknown) => {
      setInvite(null);
      setInviteError(errorText(e));
    },
  });

  async function onCopyToken(): Promise<void> {
    if (!invite) return;
    const ok = await copyToClipboard(invite.token);
    setTokenCopied(ok);
    setTokenCopyError(ok ? '' : t('common.error'));
  }

  // ---- Список приглашений: отзыв (T51) ----
  const [inviteNotice, setInviteNotice] = useState('');
  const [revokeErrors, setRevokeErrors] = useState<Record<string, string>>({});

  const revokeMutation = useMutation({
    mutationFn: (id: string) => revokeInvite(id),
    onSuccess: (_data, id) => {
      // Строка отозвана: ошибок по ней больше нет, показываем «тост» и refetch.
      setRevokeErrors((prev) => {
        if (!prev[id]) return prev;
        const next = { ...prev };
        delete next[id];
        return next;
      });
      setInviteNotice(t('admin.inviteRevokedDone'));
      // Отозвали последнюю строку страницы — возвращаемся на предыдущую.
      const data = invites.data;
      if (data && data.items.length === 1 && invitesPage > 1) setInvitesPage(invitesPage - 1);
      void qc.invalidateQueries({ queryKey: ['admin-invites'] });
    },
    onError: (e: unknown, id) => {
      setInviteNotice('');
      // 404/409 backend отдаёт по-русски — показываем его текст (схема adminUsers).
      setRevokeErrors((prev) => ({ ...prev, [id]: errorText(e) }));
    },
  });

  function onRevokeInvite(id: string): void {
    if (!window.confirm(t('admin.confirmInviteRevoke'))) return;
    setInviteNotice('');
    revokeMutation.mutate(id);
  }

  // ---- Производные значения списка приглашений ----
  const invitesData = invites.data;
  const invitesTotal = invitesData?.total ?? 0;
  const invitesRangeFrom = invitesTotal === 0 ? 0 : (invitesPage - 1) * INVITES_LIMIT + 1;
  const invitesRangeTo = Math.min(invitesPage * INVITES_LIMIT, invitesTotal);
  const invitesLastPage = invitesTotal > 0 && invitesRangeTo >= invitesTotal;
  // Пагинатор прячем только на пустой первой странице (как в журнале).
  const showInvitesPager = !!invitesData && (invitesTotal > 0 || invitesPage > 1);
  const revokePendingId = revokeMutation.isPending ? revokeMutation.variables : undefined;

  return (
    <div className="wrap">
      <div className="page">
        <main className="admin">
          <p className="muted">
            <Link to="/admin">{t('p10.nav.backToAdmin')}</Link>
          </p>
          <h1>{t('admin.invites')}</h1>

          <section className="admin-section panel" aria-label={t('admin.invites')}>
            <form className="form-grid" onSubmit={handleSubmit((v) => inviteMutation.mutate(v))}>
              <label htmlFor="admin-invite-email">{t('admin.inviteEmail')}</label>
              <input
                id="admin-invite-email"
                className="input"
                type="email"
                aria-invalid={formState.errors.email || inviteError ? true : undefined}
                aria-describedby={
                  [
                    formState.errors.email ? 'admin-invite-email-error' : '',
                    inviteError ? 'admin-invite-error' : '',
                  ]
                    .filter(Boolean)
                    .join(' ') || undefined
                }
                {...register('email', {
                  // Ошибка поля (и серверная ошибка прошлой попытки) снимается,
                  // как только пользователь начал исправлять email.
                  onChange: () => {
                    if (formState.errors.email) clearErrors('email');
                    if (inviteError) setInviteError('');
                  },
                })}
              />
              {formState.errors.email && (
                <p id="admin-invite-email-error" role="alert" className="error">
                  {formState.errors.email.message}
                </p>
              )}
              <button type="submit" className="btn btn-primary" disabled={inviteMutation.isPending}>
                {t('admin.inviteSend')}
              </button>
            </form>

            {inviteError && (
              <p id="admin-invite-error" role="alert" className="error">
                {inviteError}
              </p>
            )}

            {invite && (
              <div className="admin-token" role="status">
                <p>
                  <strong>{t('admin.inviteCreated')}</strong>
                </p>
                <p className="muted">
                  {invite.emailSent ? t('admin.inviteEmailSent') : t('admin.inviteEmailManual')}
                </p>
                <p>
                  <span className="admin-token__label">{t('admin.inviteEmail')}:</span>{' '}
                  {invite.email}
                </p>
                {invite.inviteLink && (
                  <>
                    <p>
                      <span className="admin-token__label">{t('admin.inviteLink')}:</span>
                    </p>
                    <code className="admin-token__value">{invite.inviteLink}</code>
                  </>
                )}
                <p>
                  <span className="admin-token__label">{t('admin.inviteToken')}:</span>
                </p>
                <code className="admin-token__value">{invite.token}</code>
                <p className="muted">{formatDate(invite.expiresAt)}</p>
                <button type="button" className="btn" onClick={() => void onCopyToken()}>
                  {tokenCopied ? t('admin.tokenCopied') : t('admin.copyToken')}
                </button>
                {tokenCopyError && (
                  <p role="alert" className="error">
                    {tokenCopyError}
                  </p>
                )}
              </div>
            )}

            {/* ---------- Список выданных приглашений (T51) ---------- */}
            {invites.isLoading && <p>{t('admin.loading')}</p>}
            {invites.isError && (
              <div className="admin-error" role="alert">
                <p className="error">{errorText(invites.error)}</p>
                <button type="button" className="btn" onClick={() => void invites.refetch()}>
                  {t('common.retry')}
                </button>
              </div>
            )}
            {invitesData && (
              <p className="muted admin-invites-total">
                {t('admin.invitesTotal')}: {invitesTotal}
              </p>
            )}
            {inviteNotice && (
              <p role="status" className="admin-notice">
                {inviteNotice}
              </p>
            )}
            {invitesData && invitesData.items.length === 0 && (
              <p className="muted">{t('admin.invitesEmpty')}</p>
            )}

            {invitesData && invitesData.items.length > 0 && (
              <div
                className="admin-table-wrap"
                role="region"
                aria-label={t('admin.invites')}
                tabIndex={0}
              >
                <table className="admin-table admin-table--invites">
                  <thead>
                    <tr>
                      <th scope="col">{t('admin.invitesColEmail')}</th>
                      <th scope="col">{t('admin.invitesColStatus')}</th>
                      <th scope="col">{t('admin.invitesColCreated')}</th>
                      <th scope="col">{t('admin.invitesColExpires')}</th>
                      <th scope="col">{t('admin.invitesColActions')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {invitesData.items.map((item) => {
                      const displayStatus = inviteDisplayStatus(item);
                      return (
                        <tr key={item.id}>
                          <td
                            data-label={t('admin.invitesColEmail')}
                            className="admin-td-email"
                          >
                            {item.email}
                          </td>
                          <td data-label={t('admin.invitesColStatus')}>
                            <span className={`admin-badge admin-badge--${displayStatus}`}>
                              {t(INVITE_STATUS_LABEL[displayStatus])}
                            </span>
                          </td>
                          <td data-label={t('admin.invitesColCreated')}>
                            {formatDate(item.createdAt)}
                          </td>
                          <td data-label={t('admin.invitesColExpires')}>
                            {formatDate(item.expiresAt)}
                          </td>
                          <td data-label={t('admin.invitesColActions')}>
                            {/* Отзыв доступен только для сырого sent — как и на backend. */}
                            {item.status === 'sent' ? (
                              <button
                                type="button"
                                className="btn btn-sm"
                                disabled={revokePendingId === item.id}
                                onClick={() => onRevokeInvite(item.id)}
                              >
                                {t('admin.inviteRevoke')}
                              </button>
                            ) : (
                              <span className="muted">—</span>
                            )}
                            {revokeErrors[item.id] && (
                              <p role="alert" className="error admin-row-error">
                                {revokeErrors[item.id]}
                              </p>
                            )}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}

            {showInvitesPager && invitesData && (
              <div className="admin-pager">
                <button
                  type="button"
                  className="btn"
                  disabled={invitesPage <= 1 || invites.isFetching}
                  onClick={() => setInvitesPage(invitesPage - 1)}
                >
                  {t('admin.prev')}
                </button>
                <span className="muted">
                  {t('admin.showing')} {invitesRangeFrom}–{invitesRangeTo} {t('admin.of')}{' '}
                  {invitesTotal}
                </span>
                <button
                  type="button"
                  className="btn"
                  disabled={invitesLastPage || invites.isFetching}
                  onClick={() => setInvitesPage(invitesPage + 1)}
                >
                  {t('admin.next')}
                </button>
              </div>
            )}
          </section>
        </main>
      </div>
    </div>
  );
}
