import { useState, type FormEvent } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { createInvite, type Invite } from '../../api/client';
import { listInvites, revokeInvite, type AdminInviteRow, type AdminInviteStatus } from '../../api/adminActions';
import { overviewQueryKey } from '../../api/adminHub';
import {
  AdminPage,
  Avatar,
  Badge,
  Card,
  CopyButton,
  DataTable,
  EmptyState,
  ErrorNote,
  Field,
  Icon,
  Note,
  Pager,
  RowMenu,
  Spinner,
  errorText,
  fmtAgo,
  fmtDate,
  fmtDateTime,
  plural,
  useAdminUi,
  type Column,
  type MenuItem,
  type Tone,
} from '../../components/admin/ui';
import '../../styles/admin-system.css';

const LIMIT = 10;
const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

const STATUS: Record<AdminInviteStatus, { label: string; tone: Tone }> = {
  sent: { label: 'Ожидает', tone: 'warn' },
  accepted: { label: 'Принято', tone: 'ok' },
  revoked: { label: 'Отозвано', tone: 'neutral' },
  expired: { label: 'Истекло', tone: 'danger' },
};

/** Бэкенд не пересчитывает статус: «отправлено», но срок прошёл — показываем «Истекло». */
function displayStatus(row: Pick<AdminInviteRow, 'status' | 'expiresAt'>): AdminInviteStatus {
  if (row.status === 'sent') {
    const t = new Date(row.expiresAt).getTime();
    if (!Number.isNaN(t) && t < Date.now()) return 'expired';
  }
  return row.status;
}

function daysLeft(iso: string): number {
  return Math.ceil((new Date(iso).getTime() - Date.now()) / 86_400_000);
}

export default function Invites(): JSX.Element {
  const qc = useQueryClient();
  const { confirm, toast } = useAdminUi();
  const [params, setParams] = useSearchParams();
  const page = Math.max(1, Number(params.get('page')) || 1);
  const setPage = (p: number): void =>
    setParams((prev) => {
      const next = new URLSearchParams(prev);
      if (p > 1) next.set('page', String(p));
      else next.delete('page');
      return next;
    });

  const [email, setEmail] = useState('');
  const [emailError, setEmailError] = useState('');
  const [created, setCreated] = useState<Invite | null>(null);
  // Ссылки приглашений, созданных в этой вкладке: сервер отдаёт их только один раз.
  const [links, setLinks] = useState<Record<string, string>>({});

  const invites = useQuery({
    queryKey: ['admin-invites', { page, limit: LIMIT }],
    queryFn: () => listInvites({ page, limit: LIMIT }),
  });

  const create = useMutation({
    mutationFn: (value: string) => createInvite(value),
    onSuccess: (inv) => {
      setCreated(inv);
      setEmail('');
      window.setTimeout(() => document.querySelector('.asy-created')?.scrollIntoView({ behavior: 'smooth', block: 'nearest' }), 50);
      if (inv.inviteLink) setLinks((m) => ({ ...m, [inv.id]: inv.inviteLink as string }));
      toast(inv.emailSent ? `Приглашение отправлено на ${inv.email}` : 'Приглашение создано — передайте ссылку вручную');
      if (page !== 1) setPage(1);
      void qc.invalidateQueries({ queryKey: ['admin-invites'] });
      void qc.invalidateQueries({ queryKey: overviewQueryKey });
    },
    onError: (e) => setEmailError(errorText(e)),
  });

  const revoke = useMutation({
    mutationFn: (id: string) => revokeInvite(id),
    onSuccess: () => {
      toast('Приглашение отозвано — ссылка больше не работает');
      if (invites.data && invites.data.items.length === 1 && page > 1) setPage(page - 1);
      void qc.invalidateQueries({ queryKey: ['admin-invites'] });
      void qc.invalidateQueries({ queryKey: overviewQueryKey });
    },
    onError: (e) => toast(errorText(e), 'error'),
  });

  function onSubmit(e: FormEvent): void {
    e.preventDefault();
    const v = email.trim();
    if (!v) return setEmailError('Укажите email преподавателя');
    if (!EMAIL_RE.test(v)) return setEmailError('Похоже, в email опечатка — проверьте адрес');
    setEmailError('');
    create.mutate(v);
  }

  async function onRevoke(row: AdminInviteRow): Promise<void> {
    const ok = await confirm({
      title: 'Отозвать приглашение?',
      text: (
        <>
          Ссылка для <b>{row.email}</b> перестанет работать. Если передумаете — просто отправьте новое приглашение.
        </>
      ),
      confirmLabel: 'Отозвать',
      danger: true,
    });
    if (ok) revoke.mutate(row.id);
  }

  const copy = (text: string, done: string): void =>
    void navigator.clipboard?.writeText(text).then(
      () => toast(done),
      () => toast('Не удалось скопировать', 'error'),
    );

  const columns: Column<AdminInviteRow>[] = [
    {
      key: 'email',
      header: 'Кого пригласили',
      mobile: 'title',
      render: (r) => (
        <div className="ax-person">
          <Avatar name={r.email} size={34} />
          <div className="ax-person-text">
            <strong>{r.email}</strong>
            <small className="ax-mono">код {r.tokenPrefix}…</small>
          </div>
        </div>
      ),
    },
    {
      key: 'status',
      header: 'Статус',
      width: '130px',
      render: (r) => {
        const s = STATUS[displayStatus(r)];
        return <Badge tone={s.tone}>{s.label}</Badge>;
      },
    },
    {
      key: 'created',
      header: 'Отправлено',
      width: '170px',
      render: (r) => <span title={fmtDateTime(r.createdAt)}>{fmtAgo(r.createdAt)}</span>,
    },
    {
      key: 'expires',
      header: 'Истекает',
      width: '190px',
      render: (r) => {
        const st = displayStatus(r);
        if (st !== 'sent') return <span className="ax-muted">{fmtDate(r.expiresAt)}</span>;
        const d = daysLeft(r.expiresAt);
        return (
          <span title={fmtDateTime(r.expiresAt)}>
            {fmtDate(r.expiresAt)}
            <small className="ax-muted asy-inline-hint">
              {d <= 1 ? 'меньше суток' : `через ${d} ${plural(d, 'день', 'дня', 'дней')}`}
            </small>
          </span>
        );
      },
    },
  ];

  const data = invites.data;

  return (
    <AdminPage
      title="Приглашения преподавателей"
      description="Преподаватель регистрируется только по приглашению. Отправьте его на email — человек получит письмо со ссылкой и сможет создавать курсы."
    >
      <Card title="Пригласить преподавателя" description="Письмо со ссылкой уйдёт автоматически. Ссылка действует ограниченное время и срабатывает один раз.">
        <form className="asy-invite-form" onSubmit={onSubmit} noValidate>
          <Field label="Email преподавателя" error={emailError || undefined}>
            <input
              className="ax-input"
              type="email"
              inputMode="email"
              autoComplete="off"
              placeholder="teacher@example.ru"
              value={email}
              aria-invalid={emailError ? true : undefined}
              onChange={(e) => {
                setEmail(e.target.value);
                if (emailError) setEmailError('');
              }}
            />
          </Field>
          <button type="submit" className="btn btn-primary asy-invite-submit" disabled={create.isPending}>
            <Icon name="send" size={17} /> {create.isPending ? 'Отправляем…' : 'Пригласить'}
          </button>
        </form>

        {created && (
          <div className="asy-created" role="status">
            <div className="asy-created-head">
              <span className="asy-created-icon">
                <Icon name="check" size={18} />
              </span>
              <div>
                <strong>Приглашение для {created.email} создано</strong>
                <p className="ax-muted">
                  {created.emailSent
                    ? 'Письмо со ссылкой отправлено. Если оно не дойдёт — передайте ссылку вручную.'
                    : 'Письмо отправить не удалось — скопируйте ссылку и передайте её преподавателю сами.'}{' '}
                  Действует до {fmtDateTime(created.expiresAt)}.
                </p>
              </div>
              <button type="button" className="ax-icon-btn" aria-label="Скрыть" onClick={() => setCreated(null)}>
                <Icon name="x" size={18} />
              </button>
            </div>
            {created.inviteLink && (
              <div className="asy-secret">
                <span className="asy-secret-label">Ссылка для регистрации</span>
                <div className="asy-secret-row">
                  <code className="asy-code">{created.inviteLink}</code>
                  <CopyButton text={created.inviteLink} label="Скопировать ссылку" />
                </div>
              </div>
            )}
            <div className="asy-secret">
              <span className="asy-secret-label">Код приглашения (если человек регистрируется вручную)</span>
              <div className="asy-secret-row">
                <code className="asy-code">{created.token}</code>
                <CopyButton text={created.token} label="Скопировать код" />
              </div>
            </div>
            <Note tone="warn">Ссылка и код показываются только сейчас — потом их не увидеть. Сохраните или отправьте сразу.</Note>
          </div>
        )}
      </Card>

      <Card
        flush
        title="Отправленные приглашения"
        actions={
          <button type="button" className="btn btn-ghost" onClick={() => void invites.refetch()} disabled={invites.isFetching}>
            <Icon name="refresh" size={16} /> Обновить
          </button>
        }
      >
        {invites.isLoading && <Spinner />}
        {invites.isError && <ErrorNote error={invites.error} onRetry={() => void invites.refetch()} />}
        {data && (
          <DataTable
            caption="Приглашения преподавателей"
            columns={columns}
            rows={data.items}
            rowKey={(r) => r.id}
            empty={
              page > 1 ? (
                <EmptyState title="На этой странице пусто" action={<button type="button" className="btn btn-ghost btn-sm" onClick={() => setPage(1)}>К первой странице</button>} />
              ) : (
                <EmptyState icon="mail" title="Приглашений пока нет" text="Введите email преподавателя в форме выше — приглашение появится здесь." />
              )
            }
            actions={(r) => {
              const items: MenuItem[] = [];
              if (links[r.id] && displayStatus(r) === 'sent') {
                items.push({ label: 'Скопировать ссылку', icon: 'copy', onClick: () => copy(links[r.id], 'Ссылка скопирована') });
              }
              items.push({ label: 'Скопировать email', icon: 'mail', onClick: () => copy(r.email, 'Email скопирован') });
              if (r.status === 'sent') {
                items.push('divider', {
                  label: 'Отозвать приглашение',
                  icon: 'x',
                  danger: true,
                  disabled: revoke.isPending && revoke.variables === r.id,
                  onClick: () => void onRevoke(r),
                });
              }
              return <RowMenu items={items} label={`Действия с приглашением ${r.email}`} />;
            }}
          />
        )}
        {data && <Pager page={page} total={data.total} limit={LIMIT} onPage={setPage} />}
      </Card>
      <p className="ax-muted asy-footnote">
        Ссылку можно скопировать только у приглашений, созданных сейчас: в базе хранится лишь начало кода. Если ссылка потерялась —
        отзовите приглашение и отправьте новое.
      </p>
    </AdminPage>
  );
}
