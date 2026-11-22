import { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { paymentJournal, type PaymentStatus } from '../../api/client';
import { refundPayment } from '../../api/adminActions';
import { overviewQueryKey } from '../../api/adminHub';
import {
  isoDay,
  listAllAdminCourses,
  salesCoursesKey,
  syncPayment,
  useUrlFilters,
  useUrlSearch,
  type SalesPayment,
} from '../../api/adminSales';
import {
  AdminPage,
  Badge,
  Card,
  CopyButton,
  DataTable,
  Drawer,
  EmptyState,
  ErrorNote,
  FilterSelect,
  Icon,
  InfoList,
  Note,
  Pager,
  SearchInput,
  Segmented,
  Spinner,
  StatCard,
  Toolbar,
  downloadCsv,
  errorText,
  fmtDateTime,
  fmtMoney,
  plural,
  useAdminUi,
  type Column,
  type Tone,
} from '../../components/admin/ui';
import { CourseCell, StudentCell } from './Enrollments';
import '../../styles/admin-sales.css';

const LIMIT = 20;
const LIST_KEY = ['admin-payments'] as const;

const STATUS: Record<PaymentStatus, { label: string; tone: Tone }> = {
  succeeded: { label: 'Оплачен', tone: 'ok' },
  pending: { label: 'Ожидает', tone: 'warn' },
  failed: { label: 'Ошибка', tone: 'danger' },
  refunded: { label: 'Возврат', tone: 'neutral' },
};

type StatusFilter = '' | PaymentStatus;
const STATUS_OPTIONS: { value: StatusFilter; label: string }[] = [
  { value: '', label: 'Все' },
  { value: 'succeeded', label: 'Оплачен' },
  { value: 'pending', label: 'Ожидает' },
  { value: 'failed', label: 'Ошибка' },
  { value: 'refunded', label: 'Возврат' },
];

const PROVIDER_LABEL: Record<string, string> = {
  mock: 'Тестовая касса',
  yookassa: 'ЮKassa',
  stripe: 'Stripe',
};

function isStatus(v: string): v is PaymentStatus {
  return v === 'pending' || v === 'succeeded' || v === 'failed' || v === 'refunded';
}

function StatusBadge({ status }: { status: PaymentStatus }): JSX.Element {
  return <Badge tone={STATUS[status]?.tone ?? 'neutral'}>{STATUS[status]?.label ?? status}</Badge>;
}

function providerName(p: string): string {
  return PROVIDER_LABEL[p] ?? p;
}

/** Дата и время с годом — для истории платежа. */
function fmtFull(v: string | null): string {
  if (!v) return '—';
  const d = new Date(v);
  return Number.isNaN(d.getTime())
    ? '—'
    : d.toLocaleString('ru-RU', { day: 'numeric', month: 'long', year: 'numeric', hour: '2-digit', minute: '2-digit' });
}

/** Цена до скидки, если был промокод. */
function fullPrice(p: SalesPayment): number {
  return p.amountCents + (p.discountCents ?? 0);
}

export default function Payments(): JSX.Element {
  const qc = useQueryClient();
  const { confirm, toast } = useAdminUi();
  const { get, set } = useUrlFilters();

  const q = get('q');
  const courseId = get('courseId');
  const statusParam = get('status');
  const status: StatusFilter = isStatus(statusParam) ? statusParam : '';
  const from = get('from');
  const to = get('to');
  const page = Math.max(1, Number(get('page')) || 1);
  const periodInvalid = from !== '' && to !== '' && from > to;
  const [search, setSearch] = useUrlSearch('q', q, set);
  const [selected, setSelected] = useState<SalesPayment | null>(null);
  const [actionError, setActionError] = useState<unknown>(null);
  const [exporting, setExporting] = useState(false);

  const courses = useQuery({ queryKey: salesCoursesKey, queryFn: listAllAdminCourses, staleTime: 60_000 });

  const filters = { status: status || undefined, courseId: courseId || undefined, q, from, to };
  const journal = useQuery({
    queryKey: [...LIST_KEY, { page, ...filters }],
    queryFn: () => paymentJournal({ page, limit: LIMIT, ...filters }),
    enabled: !periodInvalid,
    placeholderData: keepPreviousData,
  });

  const courseOptions = useMemo(() => {
    const opts = [{ value: '', label: 'Все курсы' }, ...(courses.data ?? []).map((c) => ({ value: c.id, label: c.title }))];
    if (courseId && !opts.some((o) => o.value === courseId)) opts.push({ value: courseId, label: 'Выбранный курс' });
    return opts;
  }, [courses.data, courseId]);

  const afterChange = (p: SalesPayment): void => {
    setSelected(p);
    void qc.invalidateQueries({ queryKey: LIST_KEY });
    void qc.invalidateQueries({ queryKey: overviewQueryKey });
    void qc.invalidateQueries({ queryKey: ['admin', 'stats'] });
    void qc.invalidateQueries({ queryKey: ['admin-enrollments'] });
  };

  const sync = useMutation({
    mutationFn: (id: string) => syncPayment(id),
    onSuccess: (p, id) => {
      const before = selected?.id === id ? selected.status : undefined;
      afterChange(p);
      toast(
        before && before !== p.status
          ? `Статус обновлён: ${STATUS[p.status]?.label ?? p.status}`
          : `Статус подтверждён: ${STATUS[p.status]?.label ?? p.status}`,
      );
    },
    onError: (e) => {
      setActionError(e);
      toast(errorText(e), 'error');
    },
  });

  const refund = useMutation({
    mutationFn: (id: string) => refundPayment(id) as Promise<SalesPayment>,
    onSuccess: (p) => {
      afterChange(p);
      toast('Возврат оформлен, доступ к курсу закрыт');
    },
    onError: (e) => {
      setActionError(e);
      toast(errorText(e), 'error');
    },
  });

  function open(p: SalesPayment): void {
    setActionError(null);
    setSelected(p);
  }

  async function onRefund(p: SalesPayment): Promise<void> {
    const ok = await confirm({
      title: `Вернуть ${fmtMoney(p.amountCents)}?`,
      text: (
        <>
          <p>
            Ученик: <b>{p.studentName || p.studentEmail}</b>
            <br />
            Курс: «{p.courseTitle}»
          </p>
          <p>
            Деньги ({fmtMoney(p.amountCents)}) вернутся ученику через платёжный сервис ({providerName(p.provider)}), доступ к курсу закроется. Отменить возврат
            нельзя.
          </p>
        </>
      ),
      confirmLabel: `Вернуть ${fmtMoney(p.amountCents)}`,
      danger: true,
    });
    if (!ok) return;
    setActionError(null);
    refund.mutate(p.id);
  }

  async function onExport(): Promise<void> {
    if (periodInvalid) return;
    setExporting(true);
    try {
      const rows: SalesPayment[] = [];
      let total = 0;
      for (let p = 1; p <= 50; p += 1) {
        const res = await paymentJournal({ page: p, limit: 100, ...filters });
        if (p === 1) total = res.total;
        rows.push(...(res.items as SalesPayment[]));
        if (res.items.length === 0 || rows.length >= res.total) break;
      }
      if (rows.length === 0) {
        toast('Нечего выгружать — по фильтру платежей нет', 'info');
        return;
      }
      downloadCsv(
        `payments-${isoDay()}.csv`,
        ['Дата', 'Ученик', 'Email', 'Курс', 'Сумма', 'Промокод', 'Скидка', 'Комиссия платформы', 'Преподавателю', 'Провайдер', 'Статус', 'ID у провайдера', 'ID платежа'],
        rows.map((r) => [
          fmtDateTime(r.createdAt),
          r.studentName,
          r.studentEmail,
          r.courseTitle,
          fmtMoney(r.amountCents),
          r.promoCode ?? '',
          r.discountCents ? fmtMoney(r.discountCents) : '',
          fmtMoney(r.platformFeeCents),
          fmtMoney(r.teacherAmountCents),
          providerName(r.provider),
          STATUS[r.status]?.label ?? r.status,
          r.providerPaymentId,
          r.id,
        ]),
      );
      if (total > rows.length) toast(`Выгружены первые ${rows.length} из ${total} платежей`, 'info');
      else toast(`Выгружено ${rows.length} ${plural(rows.length, 'платёж', 'платежа', 'платежей')}`);
    } catch (e) {
      toast(errorText(e), 'error');
    } finally {
      setExporting(false);
    }
  }

  const quick = (days: number): void => set({ from: isoDay(-(days - 1)), to: isoDay() });
  const quickActive = (days: number): boolean => from === isoDay(-(days - 1)) && to === isoDay();

  const columns: Column<SalesPayment>[] = [
    {
      key: 'student',
      header: 'Ученик',
      mobile: 'title',
      render: (p) => <StudentCell id={p.studentId} name={p.studentName} email={p.studentEmail} />,
    },
    { key: 'course', header: 'Курс', render: (p) => <CourseCell id={p.courseId} title={p.courseTitle} /> },
    {
      key: 'amount',
      header: 'Сумма / промокод',
      align: 'right',
      render: (p) => (
        <span className="asl-amount">
          <span className="ax-num asl-money">{fmtMoney(p.amountCents)}</span>
          {p.promoCode && (
            <>
              <span className="asl-code asl-code--sm">{p.promoCode}</span>
              {p.discountCents ? <small className="ax-muted asl-nowrap">скидка {fmtMoney(p.discountCents)}</small> : null}
            </>
          )}
        </span>
      ),
    },
    {
      key: 'status',
      header: 'Статус',
      render: (p) => (
        <span className="asl-stack">
          <StatusBadge status={p.status} />
          <small className="ax-muted asl-nowrap asl-desktop-only">{providerName(p.provider)}</small>
        </span>
      ),
    },
    { key: 'date', header: 'Дата', render: (p) => <span className="asl-nowrap">{fmtDateTime(p.createdAt)}</span> },
  ];

  const data = journal.data;
  const items = (data?.items ?? []) as SalesPayment[];
  const filtersActive = Boolean(q || courseId || status || from || to);
  const totalsHint = status === 'succeeded' ? 'выручка, только оплаченные' : status ? 'по выбранному статусу' : 'по всем статусам';

  return (
    <AdminPage
      title="Платежи"
      description="Журнал всех оплат курсов: поиск, итоги по фильтру, сверка статуса с платёжным сервисом и возвраты."
      actions={
        <button type="button" className="btn btn-ghost" onClick={() => void onExport()} disabled={exporting || periodInvalid}>
          <Icon name="download" size={17} /> {exporting ? 'Готовим файл…' : 'Выгрузить CSV'}
        </button>
      }
    >
      <div className="ax-grid ax-grid--4">
        <StatCard label="Сумма платежей" icon="card" tone="brand" value={data ? fmtMoney(data.totals.amountCents) : '—'} hint={totalsHint} />
        <StatCard label="Комиссия" icon="chart" tone="info" value={data ? fmtMoney(data.totals.platformFeeCents) : '—'} hint="доход платформы" />
        <StatCard label="Преподавателям" icon="users" tone="ok" value={data ? fmtMoney(data.totals.teacherAmountCents) : '—'} hint="к выплате авторам" />
        <StatCard
          label="Платежей"
          icon="history"
          tone="neutral"
          value={data ? data.total : '—'}
          hint={filtersActive ? 'по текущему фильтру' : 'за всё время'}
        />
      </div>

      <Card flush className="asl-card">
        <div className="asl-toolbar-stack">
          <Segmented<StatusFilter> label="Статус платежа" value={status} onChange={(v) => set({ status: v || null })} options={STATUS_OPTIONS} />
          <Toolbar
            end={
              filtersActive ? (
                <button
                  type="button"
                  className="btn btn-ghost"
                  onClick={() => {
                    setSearch('');
                    set({ q: null, courseId: null, status: null, from: null, to: null });
                  }}
                >
                  Сбросить фильтры
                </button>
              ) : undefined
            }
          >
            <SearchInput value={search} onChange={setSearch} placeholder="Email или имя ученика" />
            <FilterSelect label="Курс" value={courseId} onChange={(v) => set({ courseId: v || null })} options={courseOptions} />
            <div className="asl-period" role="group" aria-label="Период">
              <div className="asl-period-fields">
                <input type="date" aria-label="С даты" value={from} max={to || undefined} onChange={(e) => set({ from: e.target.value || null })} />
                <span className="asl-period-dash">–</span>
                <input type="date" aria-label="По дату" value={to} min={from || undefined} onChange={(e) => set({ to: e.target.value || null })} />
              </div>
              <div className="asl-quick">
                <button type="button" className={quickActive(7) ? 'is-active' : ''} onClick={() => quick(7)}>
                  7 дней
                </button>
                <button type="button" className={quickActive(30) ? 'is-active' : ''} onClick={() => quick(30)}>
                  30 дней
                </button>
              </div>
            </div>
          </Toolbar>
          {periodInvalid && <Note tone="warn">Начало периода позже конца — поправьте даты.</Note>}
        </div>

        {periodInvalid ? null : journal.isLoading ? (
          <Spinner />
        ) : journal.isError ? (
          <ErrorNote error={journal.error} onRetry={() => void journal.refetch()} />
        ) : (
          <>
            <DataTable
              columns={columns}
              rows={items}
              rowKey={(p) => p.id}
              onRowClick={open}
              actions={(p) => (
                <button type="button" className="ax-icon-btn" aria-label="Подробнее о платеже" onClick={() => open(p)}>
                  <Icon name="chevronRight" size={18} />
                </button>
              )}
              caption="Журнал платежей"
              empty={
                filtersActive ? (
                  <EmptyState icon="search" title="Платежей не найдено" text="Измените поиск, статус, курс или период." />
                ) : (
                  <EmptyState icon="card" title="Платежей пока нет" text="Когда ученики начнут покупать курсы, оплаты появятся здесь." />
                )
              }
            />
            {data && <Pager page={page} total={data.total} limit={LIMIT} onPage={(p) => set({ page: String(p) })} />}
          </>
        )}
      </Card>

      <Drawer
        open={selected !== null}
        onClose={() => setSelected(null)}
        title={selected ? `Платёж на ${fmtMoney(selected.amountCents)}` : ''}
        subtitle={
          selected && (
            <span className="asl-drawer-head-meta">
              <StatusBadge status={selected.status} />
              <span>{fmtDateTime(selected.createdAt)}</span>
            </span>
          )
        }
        footer={
          selected && selected.amountCents > 0 && (
            <div className="asl-foot">
              <button type="button" className="btn btn-ghost" disabled={sync.isPending || refund.isPending} onClick={() => { setActionError(null); sync.mutate(selected.id); }}>
                <Icon name="refresh" size={16} /> {sync.isPending ? 'Сверяем…' : 'Сверить статус'}
              </button>
              {selected.status === 'succeeded' && (
                <button type="button" className="btn ax-btn-danger" disabled={refund.isPending || sync.isPending} onClick={() => void onRefund(selected)}>
                  {refund.isPending ? 'Оформляем…' : 'Оформить возврат'}
                </button>
              )}
            </div>
          )
        }
      >
        {selected && (
          <>
            {actionError !== null && (
              <div style={{ marginBottom: 14 }}>
                <ErrorNote error={actionError} />
              </div>
            )}
            {selected.amountCents === 0 && (
              <div style={{ marginBottom: 14 }}>
                <Note>Запись на бесплатный курс — денег не было, сверять и возвращать нечего.</Note>
              </div>
            )}
            {selected.status === 'pending' && selected.amountCents > 0 && (
              <div style={{ marginBottom: 14 }}>
                <Note tone="warn">Платёж ещё не подтверждён. Если ученик говорит, что оплатил, нажмите «Сверить статус» — мы спросим у платёжного сервиса.</Note>
              </div>
            )}
            <section className="asl-drawer-section">
              <InfoList
                items={[
                  { label: 'Ученик', value: <StudentCell id={selected.studentId} name={selected.studentName} email={selected.studentEmail} /> },
                  { label: 'Курс', value: <CourseCell id={selected.courseId} title={selected.courseTitle} /> },
                ]}
              />
            </section>
            <section className="asl-drawer-section">
              <h3 className="asl-section-title">Деньги</h3>
              <InfoList
                items={[
                  { label: 'Оплачено', value: <span className="ax-num asl-money">{fmtMoney(selected.amountCents)}</span> },
                  {
                    label: 'Промокод',
                    value: selected.promoCode ? (
                      <span className="asl-promo">
                        <span className="asl-code">{selected.promoCode}</span>
                        <small className="ax-muted">
                          скидка {fmtMoney(selected.discountCents ?? 0)} от {fmtMoney(fullPrice(selected))}
                        </small>
                      </span>
                    ) : (
                      'не применялся'
                    ),
                  },
                  { label: 'Комиссия платформы', value: <span className="ax-num">{fmtMoney(selected.platformFeeCents)}</span> },
                  { label: 'Преподавателю', value: <span className="ax-num">{fmtMoney(selected.teacherAmountCents)}</span> },
                  { label: 'Валюта', value: selected.currency || '—' },
                  { label: 'Платёжный сервис', value: providerName(selected.provider) },
                ]}
              />
            </section>
            <section className="asl-drawer-section">
              <h3 className="asl-section-title">История</h3>
              <ul className="asl-timeline">
                <li>
                  <span className="asl-timeline-dot is-brand" />
                  <span>
                    <strong>Платёж создан</strong>
                    <small>{fmtFull(selected.createdAt)}</small>
                  </span>
                </li>
                <li>
                  <span
                    className={`asl-timeline-dot ${
                      selected.status === 'succeeded' ? 'is-ok' : selected.status === 'failed' ? 'is-danger' : selected.status === 'pending' ? 'is-warn' : ''
                    }`}
                  />
                  <span>
                    <strong>
                      {selected.status === 'pending'
                        ? 'Ожидает оплаты'
                        : selected.status === 'succeeded'
                          ? 'Оплата прошла'
                          : selected.status === 'failed'
                            ? 'Оплата не прошла'
                            : 'Оформлен возврат'}
                    </strong>
                    <small>{selected.processedAt ? fmtFull(selected.processedAt) : 'платёжный сервис ещё не ответил'}</small>
                  </span>
                </li>
              </ul>
            </section>
            <section className="asl-drawer-section">
              <h3 className="asl-section-title">Идентификаторы</h3>
              <div className="asl-info-1col">
              <InfoList
                items={[
                  {
                    label: 'ID у платёжного сервиса',
                    value: selected.providerPaymentId ? (
                      <span className="asl-id">
                        <code className="ax-mono">{selected.providerPaymentId}</code>
                        <CopyButton text={selected.providerPaymentId} />
                      </span>
                    ) : (
                      'ещё не присвоен'
                    ),
                  },
                  {
                    label: 'ID платежа на платформе',
                    value: (
                      <span className="asl-id">
                        <code className="ax-mono">{selected.id}</code>
                        <CopyButton text={selected.id} />
                      </span>
                    ),
                  },
                ]}
              />
              </div>
            </section>
            <section className="asl-drawer-section">
              <div className="asl-actions-row">
                <Link className="btn btn-ghost btn-sm" to={`/admin/enrollments?studentId=${selected.studentId}`}>
                  <Icon name="key" size={16} /> Доступы ученика
                </Link>
                {selected.studentEmail && (
                  <Link
                    className="btn btn-ghost btn-sm"
                    to={`/admin/payments?q=${encodeURIComponent(selected.studentEmail)}`}
                    onClick={() => {
                      setSearch(selected.studentEmail);
                      setSelected(null);
                    }}
                  >
                    <Icon name="card" size={16} /> Все платежи ученика
                  </Link>
                )}
              </div>
            </section>
          </>
        )}
      </Drawer>
    </AdminPage>
  );
}
