// T30: страница «Профиль» — карточка аккаунта, отображаемое имя и история платежей.
// Данные — контракт T23 (api/meExtra.ts): GET/PATCH /me/profile, GET /me/payments.
// В ru.json ключей profile.roleStudent/profile.status* НЕТ, поэтому роли и статусы
// платежей подписаны существующими adminUsers.role* / admin.status* (см. отчёт T30).
import { type FormEvent, useState } from 'react';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, formatDate, formatMoney } from '../api/client';
import type { PaymentStatus, Role } from '../api/client';
import { meProfile, myPayments, updateProfile } from '../api/meExtra';
import { t } from '../i18n/t';
import type { RuKey } from '../i18n/t';
import '../styles/profile.css';

/** Размер страницы истории платежей (контракт T23: page/limit). */
const PAYMENTS_LIMIT = 20;

/** Границы отображаемого имени после trim (контракт T23: 2..60). */
const NAME_MIN = 2;
const NAME_MAX = 60;

/** Русские подписи ролей: профильных ключей нет — переиспользуем adminUsers.role*. */
const ROLE_LABEL: Record<Role, RuKey> = {
  student: 'adminUsers.roleStudent',
  teacher: 'adminUsers.roleTeacher',
  admin: 'adminUsers.roleAdmin',
};

/** Русские подписи статусов платежей: переиспользуем admin.status* (T02). */
const STATUS_LABEL: Record<PaymentStatus, RuKey> = {
  pending: 'admin.statusPending',
  succeeded: 'admin.statusSucceeded',
  failed: 'admin.statusFailed',
  refunded: 'admin.statusRefunded',
};

/** Текст ошибки запроса: серверное сообщение ApiError, сеть — общий текст. */
function errorText(e: unknown): string {
  if (e instanceof ApiError) return e.message || t('common.error');
  if (e instanceof TypeError) return t('error.network');
  return t('common.error');
}

export default function Profile(): JSX.Element {
  const qc = useQueryClient();

  // ---------- Профиль ----------
  const profileQuery = useQuery({
    queryKey: ['me-profile'],
    queryFn: meProfile,
    retry: false,
  });

  // Черновик имени: null — пользователь ещё не редактировал поле, показываем
  // серверное значение (фоновое обновление профиля не затирает печатаемый текст).
  const [nameDraft, setNameDraft] = useState<string | null>(null);
  const [saveMsg, setSaveMsg] = useState<{ ok: boolean; text: string } | null>(null);

  const name = nameDraft ?? profileQuery.data?.displayName ?? '';
  const trimmedName = name.trim();
  const nameValid = trimmedName.length >= NAME_MIN && trimmedName.length <= NAME_MAX;

  const saveMutation = useMutation({
    mutationFn: (displayName: string) => updateProfile(displayName),
    onSuccess: (data) => {
      // Кеш профиля — сразу ответом сервера; ['me'] инвалидируем, чтобы шапка
      // (meQueryOptions) перечитала пользователя с новым именем.
      qc.setQueryData(['me-profile'], data);
      setNameDraft(null);
      setSaveMsg({ ok: true, text: t('profile.saved') });
      void qc.invalidateQueries({ queryKey: ['me'] });
    },
    onError: (e: unknown) => setSaveMsg({ ok: false, text: errorText(e) }),
  });

  function onSaveName(e: FormEvent<HTMLFormElement>): void {
    e.preventDefault();
    if (!nameValid || saveMutation.isPending) return;
    setSaveMsg(null);
    saveMutation.mutate(trimmedName);
  }

  // ---------- История платежей ----------
  const [page, setPage] = useState(1);
  const paymentsQuery = useQuery({
    queryKey: ['me-payments', page],
    queryFn: () => myPayments({ page, limit: PAYMENTS_LIMIT }),
    retry: false,
    // Переключение страниц без мигания: держим прошлую таблицу до прихода новой.
    placeholderData: keepPreviousData,
  });

  const payments = paymentsQuery.data;
  const totalPages = payments ? Math.max(1, Math.ceil(payments.total / payments.limit)) : 1;
  const rangeFrom = payments && payments.total > 0 ? (page - 1) * PAYMENTS_LIMIT + 1 : 0;
  const rangeTo = payments ? Math.min(page * PAYMENTS_LIMIT, payments.total) : 0;

  return (
    <div className="wrap">
      <div className="page">
        <h1>{t('profile.title')}</h1>

        {/* ---------- Карточка профиля ---------- */}
        <section className="panel pf-card" aria-label={t('profile.title')}>
          {profileQuery.isLoading && <p role="status">{t('common.loading')}</p>}

          {profileQuery.isError && (
            <div role="alert" className="pf-error">
              <p className="error">{errorText(profileQuery.error)}</p>
              <button type="button" className="btn" onClick={() => void profileQuery.refetch()}>
                {t('common.retry')}
              </button>
            </div>
          )}

          {profileQuery.data && (
            <>
              <dl className="pf-facts">
                <div>
                  <dt>{t('profile.email')}</dt>
                  <dd>{profileQuery.data.email}</dd>
                </div>
                <div>
                  <dt>{t('profile.role')}</dt>
                  <dd>{t(ROLE_LABEL[profileQuery.data.role])}</dd>
                </div>
                <div>
                  <dt>{t('profile.registered')}</dt>
                  <dd>{formatDate(profileQuery.data.createdAt)}</dd>
                </div>
              </dl>

              <form className="pf-name-form" onSubmit={onSaveName}>
                <label className="pf-label" htmlFor="pf-name">
                  {t('profile.displayName')}
                </label>
                <input
                  id="pf-name"
                  className="input"
                  value={name}
                  onChange={(e) => {
                    setNameDraft(e.target.value);
                    setSaveMsg(null);
                  }}
                  placeholder={t('profile.namePlaceholder')}
                  aria-invalid={nameValid ? undefined : true}
                  autoComplete="nickname"
                />
                <p className="pf-hint muted">{t('profile.nameHint')}</p>
                {saveMsg && (
                  <p
                    role={saveMsg.ok ? 'status' : 'alert'}
                    className={saveMsg.ok ? 'pf-saved' : 'error'}
                  >
                    {saveMsg.text}
                  </p>
                )}
                <button
                  type="submit"
                  className="btn btn-primary pf-save"
                  disabled={!nameValid || saveMutation.isPending}
                >
                  {saveMutation.isPending ? t('common.loading') : t('profile.save')}
                </button>
              </form>
            </>
          )}
        </section>

        {/* ---------- История платежей ---------- */}
        <section className="panel pf-payments" aria-label={t('profile.payments')}>
          <h2>{t('profile.payments')}</h2>

          {paymentsQuery.isLoading && <p role="status">{t('common.loading')}</p>}

          {paymentsQuery.isError && (
            <div role="alert" className="pf-error">
              <p className="error">{errorText(paymentsQuery.error)}</p>
              <button type="button" className="btn" onClick={() => void paymentsQuery.refetch()}>
                {t('common.retry')}
              </button>
            </div>
          )}

          {payments && payments.items.length === 0 && (
            <p className="muted">{t('profile.paymentsEmpty')}</p>
          )}

          {payments && payments.items.length > 0 && (
            <div
              className="pf-table-wrap"
              role="region"
              aria-label={t('profile.payments')}
              tabIndex={0}
            >
              <table className="pf-table">
                <thead>
                  <tr>
                    <th scope="col">{t('profile.colDate')}</th>
                    <th scope="col">{t('profile.colCourse')}</th>
                    <th scope="col">{t('profile.colAmount')}</th>
                    <th scope="col">{t('profile.colStatus')}</th>
                  </tr>
                </thead>
                <tbody>
                  {payments.items.map((p) => (
                    <tr key={p.id}>
                      <td data-label={t('profile.colDate')}>{formatDate(p.createdAt)}</td>
                      {/* Пустой courseTitle — курс удалён (контракт T23). */}
                      <td data-label={t('profile.colCourse')}>{p.courseTitle || '—'}</td>
                      <td data-label={t('profile.colAmount')}>{formatMoney(p.amountCents)}</td>
                      <td data-label={t('profile.colStatus')}>
                        <span className={`pf-badge pf-badge--${p.status}`}>
                          {t(STATUS_LABEL[p.status])}
                        </span>
                        {p.processedAt === null && (
                          <span className="pf-processing">{t('profile.pendingPayment')}</span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {payments && payments.total > 0 && (
            <div className="pf-pager">
              <button
                type="button"
                className="btn"
                disabled={page <= 1 || paymentsQuery.isFetching}
                onClick={() => setPage((p) => Math.max(1, p - 1))}
              >
                {t('common.prev')}
              </button>
              <span className="muted">
                {t('admin.showing')} {rangeFrom}–{rangeTo} {t('admin.of')} {payments.total}
              </span>
              <button
                type="button"
                className="btn"
                disabled={page >= totalPages || paymentsQuery.isFetching}
                onClick={() => setPage((p) => p + 1)}
              >
                {t('common.next')}
              </button>
            </div>
          )}
        </section>
      </div>
    </div>
  );
}
