import { useMemo } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { adminListLeads, adminSetLeadStatus, type Lead } from '../../api/leads';
import { overviewQueryKey } from '../../api/adminHub';
import {
  AdminPage,
  Avatar,
  Badge,
  Card,
  EmptyState,
  ErrorNote,
  Icon,
  Pager,
  RowMenu,
  SearchInput,
  Segmented,
  Spinner,
  Toolbar,
  downloadCsv,
  errorText,
  fmtAgo,
  fmtDateTime,
  useAdminUi,
  useDebounced,
  type Tone,
} from '../../components/admin/ui';
import '../../styles/admin-system.css';

type Filter = 'new' | 'done' | 'all';

const CONTACT: Record<string, { label: string; tone: Tone }> = {
  phone: { label: 'Звонок', tone: 'info' },
  telegram: { label: 'Telegram', tone: 'brand' },
  vk: { label: 'VK', tone: 'neutral' },
  max: { label: 'MAX', tone: 'neutral' },
};

const PAGE_SIZE = 30;
const leadsKey = ['admin-leads'] as const;

/** +79210001122 → +7 921 000-11-22 (остальные форматы — как есть). */
function fmtPhone(phone: string): string {
  let d = phone.replace(/\D/g, '');
  if (d.length === 11 && d.startsWith('8')) d = `7${d.slice(1)}`;
  if (d.length !== 11 || !d.startsWith('7')) return phone;
  return `+7 ${d.slice(1, 4)} ${d.slice(4, 7)}-${d.slice(7, 9)}-${d.slice(9)}`;
}

const telHref = (phone: string): string => `tel:${phone.replace(/[^\d+]/g, '')}`;

/** Ссылка «написать в Telegram» по номеру телефона (t.me/+79…). */
function telegramHref(phone: string): string | null {
  let digits = phone.replace(/\D/g, '');
  if (digits.length === 11 && digits.startsWith('8')) digits = `7${digits.slice(1)}`;
  if (digits.length === 10) digits = `7${digits}`;
  return digits.length >= 11 ? `https://t.me/+${digits}` : null;
}

function PhoneIcon(): JSX.Element {
  return (
    <svg className="ax-icon" width={15} height={15} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M5 4h4l2 5-2.5 1.5a11 11 0 0 0 5 5L15 13l5 2v4a2 2 0 0 1-2 2A16 16 0 0 1 3 6a2 2 0 0 1 2-2" />
    </svg>
  );
}

export default function Leads(): JSX.Element {
  const qc = useQueryClient();
  const { toast } = useAdminUi();
  const [params, setParams] = useSearchParams();
  const filter: Filter = params.get('status') === 'done' ? 'done' : params.get('status') === 'all' ? 'all' : 'new';
  const q = params.get('q') ?? '';
  const page = Math.max(1, Number(params.get('page')) || 1);
  const dq = useDebounced(q.trim().toLowerCase(), 200);

  const setParam = (key: string, value: string, keepPage = false): void => {
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (value) next.set(key, value);
        else next.delete(key);
        if (!keepPage) next.delete('page');
        return next;
      },
      { replace: key === 'q' },
    );
  };

  const leads = useQuery({ queryKey: leadsKey, queryFn: adminListLeads });

  const status = useMutation({
    mutationFn: (v: { id: string; status: Lead['status'] }) => adminSetLeadStatus(v.id, v.status),
    onSuccess: (_d, v) => {
      toast(v.status === 'done' ? 'Заявка отмечена обработанной' : 'Заявка возвращена в новые');
      void qc.invalidateQueries({ queryKey: leadsKey });
      void qc.invalidateQueries({ queryKey: overviewQueryKey });
    },
    onError: (e) => toast(errorText(e), 'error'),
  });

  const all = leads.data?.items ?? [];
  const newCount = all.filter((l) => l.status === 'new').length;
  const doneCount = all.length - newCount;

  const matched = useMemo(() => {
    const digits = dq.replace(/\D/g, '');
    return all.filter((l) => {
      if (filter !== 'all' && l.status !== filter) return false;
      if (!dq) return true;
      if (l.name.toLowerCase().includes(dq) || l.email.toLowerCase().includes(dq)) return true;
      return digits.length > 0 && l.phone.replace(/\D/g, '').includes(digits);
    });
  }, [all, filter, dq]);

  const lastPage = Math.max(1, Math.ceil(matched.length / PAGE_SIZE));
  const curPage = Math.min(page, lastPage);
  const rows = matched.slice((curPage - 1) * PAGE_SIZE, curPage * PAGE_SIZE);

  function exportCsv(): void {
    downloadCsv(
      `zayavki-${new Date().toISOString().slice(0, 10)}.csv`,
      ['Имя', 'Телефон', 'Email', 'Способ связи', 'Статус', 'Получена'],
      matched.map((l) => [
        l.name,
        l.phone,
        l.email,
        CONTACT[l.contactMethod]?.label ?? l.contactMethod,
        l.status === 'new' ? 'Новая' : 'Обработана',
        fmtDateTime(l.createdAt),
      ]),
    );
  }

  const emptyText =
    all.length === 0
      ? 'Когда посетитель оставит заявку в форме «Поможем подобрать курс» на главной, она появится здесь.'
      : dq
        ? 'По этому запросу заявок нет — попробуйте имя, часть телефона или email.'
        : filter === 'new'
          ? 'Все заявки обработаны. Отличная работа!'
          : 'В этом списке пока пусто.';

  return (
    <AdminPage
      title="Заявки"
      description="Посетители, которые попросили помочь с выбором курса. Свяжитесь удобным для них способом и отметьте заявку обработанной."
      actions={
        <button type="button" className="btn btn-ghost" onClick={exportCsv} disabled={matched.length === 0}>
          <Icon name="download" size={17} /> Выгрузить CSV
        </button>
      }
    >
      <Card flush>
        <Toolbar
          end={
            <Segmented<Filter>
              label="Какие заявки показать"
              value={filter}
              onChange={(v) => setParam('status', v === 'new' ? '' : v)}
              options={[
                { value: 'new', label: 'Новые', count: newCount },
                { value: 'done', label: 'Обработанные', count: doneCount },
                { value: 'all', label: 'Все', count: all.length },
              ]}
            />
          }
        >
          <SearchInput value={q} onChange={(v) => setParam('q', v)} placeholder="Имя, телефон или email" />
        </Toolbar>

        {leads.isLoading && <Spinner />}
        {leads.isError && <ErrorNote error={leads.error} onRetry={() => void leads.refetch()} />}
        {leads.data && rows.length === 0 && (
          <EmptyState
            icon="inbox"
            title={all.length === 0 ? 'Заявок пока нет' : dq ? 'Ничего не найдено' : filter === 'new' ? 'Новых заявок нет' : 'Пусто'}
            text={emptyText}
            action={
              dq ? (
                <button type="button" className="btn btn-ghost btn-sm" onClick={() => setParam('q', '')}>
                  Сбросить поиск
                </button>
              ) : filter === 'new' && doneCount > 0 ? (
                <button type="button" className="btn btn-ghost btn-sm" onClick={() => setParam('status', 'done')}>
                  Показать обработанные
                </button>
              ) : undefined
            }
          />
        )}

        {rows.length > 0 && (
          <ul className="asy-leads">
            {rows.map((l) => {
              const contact = CONTACT[l.contactMethod] ?? { label: l.contactMethod, tone: 'neutral' as Tone };
              const tg = l.contactMethod === 'telegram' ? telegramHref(l.phone) : null;
              const busy = status.isPending && status.variables?.id === l.id;
              const isNew = l.status === 'new';
              return (
                <li key={l.id} className={`asy-lead${isNew ? ' is-new' : ' is-done'}`}>
                  <div className="ax-person asy-lead-who">
                    <Avatar name={l.name} size={40} />
                    <div className="ax-person-text">
                      <strong>{l.name}</strong>
                      <small title={fmtDateTime(l.createdAt)}>
                        {fmtAgo(l.createdAt)}
                        {filter === 'all' && (isNew ? ' · новая' : ' · обработана')}
                      </small>
                    </div>
                  </div>
                  <div className="asy-lead-contacts">
                    <a className="asy-lead-link" href={telHref(l.phone)}>
                      <PhoneIcon />
                      <span className="ax-num">{fmtPhone(l.phone)}</span>
                    </a>
                    {l.email ? (
                      <a className="asy-lead-link" href={`mailto:${l.email}`}>
                        <Icon name="mail" size={15} />
                        <span>{l.email}</span>
                      </a>
                    ) : (
                      <span className="ax-muted asy-lead-noemail">email не указан</span>
                    )}
                  </div>
                  <div className="asy-lead-method">
                    <span className="ax-muted asy-lead-method-label">Связаться:</span>
                    <Badge tone={contact.tone}>{contact.label}</Badge>
                  </div>
                  <div className="asy-lead-actions">
                    <button
                      type="button"
                      className={isNew ? 'btn btn-sm asy-done-btn' : 'btn btn-ghost btn-sm'}
                      disabled={busy}
                      onClick={() => status.mutate({ id: l.id, status: isNew ? 'done' : 'new' })}
                    >
                      {isNew ? (
                        <>
                          <Icon name="check" size={16} /> Обработано
                        </>
                      ) : (
                        'Вернуть в новые'
                      )}
                    </button>
                    <RowMenu
                      label={`Действия с заявкой ${l.name}`}
                      items={[
                        ...(tg ? [{ label: 'Написать в Telegram', icon: 'external' as const, href: tg }, 'divider' as const] : []),
                        {
                          label: 'Скопировать телефон',
                          icon: 'copy',
                          onClick: () =>
                            void navigator.clipboard
                              ?.writeText(l.phone)
                              .then(() => toast('Телефон скопирован'), () => toast('Не удалось скопировать', 'error')),
                        },
                        ...(l.email
                          ? [
                              {
                                label: 'Скопировать email',
                                icon: 'copy' as const,
                                onClick: () =>
                                  void navigator.clipboard
                                    ?.writeText(l.email)
                                    .then(() => toast('Email скопирован'), () => toast('Не удалось скопировать', 'error')),
                              },
                            ]
                          : []),
                      ]}
                    />
                  </div>
                </li>
              );
            })}
          </ul>
        )}
        <Pager page={curPage} total={matched.length} limit={PAGE_SIZE} onPage={(p) => setParam('page', String(p), true)} />
      </Card>
    </AdminPage>
  );
}
