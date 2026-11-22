import { useEffect, useRef, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { listAudit, type AuditEntry } from '../../api/adminHub';
import {
  AdminPage,
  Avatar,
  Card,
  CopyButton,
  DataTable,
  Drawer,
  EmptyState,
  ErrorNote,
  Icon,
  InfoList,
  Note,
  Pager,
  SearchInput,
  Spinner,
  Toolbar,
  downloadCsv,
  errorText,
  fmtAgo,
  fmtDateTime,
  useAdminUi,
  useDebounced,
  type Column,
} from '../../components/admin/ui';
import '../../styles/admin-system.css';

const LIMIT = 30;
const EXPORT_LIMIT = 100;
const EXPORT_MAX_PAGES = 50;

/** Быстрые фильтры — подстроки, по которым ищет сервер (действие или email). */
const QUICK: { label: string; q: string }[] = [
  { label: 'Нейросеть (MCP)', q: 'MCP' },
  { label: 'Курсы', q: 'курс' },
  { label: 'Пользователи', q: 'пользовател' },
  { label: 'Доступы', q: 'доступ' },
  { label: 'Платежи', q: 'платеж' },
  { label: 'Сертификаты', q: 'сертификат' },
  { label: 'Настройки', q: 'настройк' },
];

const isMcp = (e: AuditEntry): boolean => /MCP/.test(e.action);

export default function Audit(): JSX.Element {
  const { toast } = useAdminUi();
  const [params, setParams] = useSearchParams();
  const urlQ = params.get('q') ?? '';
  const page = Math.max(1, Number(params.get('page')) || 1);
  const [input, setInput] = useState(urlQ);
  const dq = useDebounced(input.trim(), 350);
  const [open, setOpen] = useState<AuditEntry | null>(null);
  const [exporting, setExporting] = useState(false);

  // Поле поиска ↔ URL (кнопки «назад/вперёд» меняют URL — подтягиваем в поле).
  const pushed = useRef<string | null>(null);
  useEffect(() => {
    if (pushed.current !== null && pushed.current === urlQ) return;
    setInput(urlQ);
  }, [urlQ]);
  useEffect(() => {
    if (dq === urlQ.trim()) return;
    pushed.current = dq;
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (dq) next.set('q', dq);
        else next.delete('q');
        next.delete('page');
        return next;
      },
      { replace: true },
    );
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dq]);

  const setQ = (q: string): void => {
    pushed.current = q;
    setInput(q);
    setParams((prev) => {
      const next = new URLSearchParams(prev);
      if (q) next.set('q', q);
      else next.delete('q');
      next.delete('page');
      return next;
    });
  };
  const setPage = (p: number): void =>
    setParams((prev) => {
      const next = new URLSearchParams(prev);
      if (p > 1) next.set('page', String(p));
      else next.delete('page');
      return next;
    });

  const q = urlQ.trim();
  const audit = useQuery({
    queryKey: ['admin', 'audit', { q, page, limit: LIMIT }],
    queryFn: () => listAudit({ q: q || undefined, page, limit: LIMIT }),
    placeholderData: keepPreviousData,
  });

  async function exportCsv(): Promise<void> {
    setExporting(true);
    try {
      const rows: AuditEntry[] = [];
      let total = 0;
      let truncated = false;
      for (let p = 1; p <= EXPORT_MAX_PAGES; p++) {
        const res = await listAudit({ q: q || undefined, page: p, limit: EXPORT_LIMIT });
        total = res.total;
        rows.push(...res.items);
        if (res.items.length === 0 || rows.length >= total) break;
        if (p === EXPORT_MAX_PAGES) truncated = true;
      }
      downloadCsv(
        `zhurnal-deystviy-${new Date().toISOString().slice(0, 10)}.csv`,
        ['Время', 'Кто', 'Действие', 'IP'],
        rows.map((e) => [new Date(e.createdAt).toLocaleString('ru-RU'), e.actorEmail, e.action, e.ip]),
      );
      toast(
        truncated
          ? `Выгружены первые ${rows.length} из ${total} записей — уточните поиск, чтобы выгрузить остальное`
          : `Выгружено записей: ${rows.length}`,
        truncated ? 'info' : 'ok',
      );
    } catch (e) {
      toast(`Не удалось выгрузить: ${errorText(e)}`, 'error');
    } finally {
      setExporting(false);
    }
  }

  const columns: Column<AuditEntry>[] = [
    {
      key: 'time',
      header: 'Когда',
      width: '150px',
      mobile: 'meta',
      render: (e) => (
        <span className="asy-when">
          <span>{fmtDateTime(e.createdAt)}</span>
          <small className="ax-muted">{fmtAgo(e.createdAt)}</small>
        </span>
      ),
    },
    {
      key: 'action',
      header: 'Действие',
      mobile: 'title',
      render: (e) => (
        <span className="asy-action">
          {isMcp(e) && (
            <span className="asy-action-tag" title="Через нейросеть (MCP)">
              <Icon name="bot" size={14} />
            </span>
          )}
          <span>{e.action}</span>
        </span>
      ),
    },
    {
      key: 'who',
      header: 'Кто',
      width: '240px',
      render: (e) => (
        <span className="ax-person asy-who">
          <Avatar name={e.actorEmail || '?'} size={28} />
          <span className="asy-break">{e.actorEmail || '—'}</span>
        </span>
      ),
    },
    { key: 'ip', header: 'IP', width: '150px', mobile: 'hidden', render: (e) => <span className="ax-mono ax-muted asy-nowrap">{e.ip || '—'}</span> },
  ];

  const data = audit.data;

  return (
    <AdminPage
      title="Журнал действий"
      description="Кто, что и когда изменил в админке. Сюда автоматически попадают все изменения — сделанные вручную и через нейросеть (MCP)."
      actions={
        <button type="button" className="btn btn-ghost" onClick={() => void exportCsv()} disabled={exporting || !data || data.total === 0}>
          <Icon name="download" size={17} /> {exporting ? 'Готовим файл…' : 'Выгрузить CSV'}
        </button>
      }
    >
      <Card flush>
        <Toolbar>
          <SearchInput value={input} onChange={setInput} placeholder="Действие или email" label="Поиск по действию или email администратора" />
        </Toolbar>
        <div className="asy-chips" role="group" aria-label="Быстрые фильтры">
          {QUICK.map((c) => {
            const active = q.toLowerCase() === c.q.toLowerCase();
            return (
              <button
                key={c.q}
                type="button"
                className={`asy-chip${active ? ' is-active' : ''}`}
                aria-pressed={active}
                onClick={() => setQ(active ? '' : c.q)}
              >
                {c.label}
              </button>
            );
          })}
        </div>

        {audit.isLoading && <Spinner />}
        {audit.isError && <ErrorNote error={audit.error} onRetry={() => void audit.refetch()} />}
        {data && (
          <div className={audit.isFetching && audit.isPlaceholderData ? 'asy-dim' : undefined}>
            <DataTable
              caption="Журнал действий"
              columns={columns}
              rows={data.items}
              rowKey={(e) => e.id}
              onRowClick={setOpen}
              empty={
                q ? (
                  <EmptyState
                    icon="search"
                    title="Ничего не найдено"
                    text={`По запросу «${q}» записей нет.`}
                    action={
                      <button type="button" className="btn btn-ghost btn-sm" onClick={() => setQ('')}>
                        Сбросить поиск
                      </button>
                    }
                  />
                ) : (
                  <EmptyState icon="history" title="Журнал пока пуст" text="Как только кто-то из администраторов что-то изменит, запись появится здесь." />
                )
              }
            />
          </div>
        )}
        {data && <Pager page={page} total={data.total} limit={LIMIT} onPage={setPage} />}
      </Card>

      <Drawer open={open !== null} onClose={() => setOpen(null)} title="Запись журнала" subtitle={open ? fmtDateTime(open.createdAt) : undefined}>
        {open && (
          <div className="asy-stack">
            <p className="asy-drawer-action">{open.action}</p>
            {isMcp(open) && <Note tone="info">Действие выполнено через нейросеть (MCP) по токену этого администратора.</Note>}
            <InfoList
              items={[
                {
                  label: 'Кто',
                  value: open.actorId ? (
                    <Link className="ax-link" to={`/admin/users/${encodeURIComponent(open.actorId)}`}>
                      {open.actorEmail}
                    </Link>
                  ) : (
                    open.actorEmail || '—'
                  ),
                },
                { label: 'Когда', value: `${new Date(open.createdAt).toLocaleString('ru-RU')} (${fmtAgo(open.createdAt)})` },
                { label: 'IP-адрес', value: <span className="ax-mono">{open.ip || '—'}</span> },
              ]}
            />
            <div className="ax-form-actions">
              <CopyButton text={`${new Date(open.createdAt).toLocaleString('ru-RU')} — ${open.actorEmail} — ${open.action}`} label="Скопировать запись" />
              {open.actorEmail && (
                <button
                  type="button"
                  className="btn btn-ghost btn-sm"
                  onClick={() => {
                    setQ(open.actorEmail);
                    setOpen(null);
                  }}
                >
                  <Icon name="filter" size={16} /> Все действия этого человека
                </button>
              )}
            </div>
          </div>
        )}
      </Drawer>
    </AdminPage>
  );
}
