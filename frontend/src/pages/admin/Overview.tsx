import { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { getAdminOverview, overviewQueryKey, type OverviewDay } from '../../api/adminHub';
import { getAdminStats } from '../../api/adminConsole';
import {
  AdminPage,
  Avatar,
  Badge,
  Card,
  EmptyState,
  ErrorNote,
  Icon,
  Note,
  ROLE_LABEL,
  Segmented,
  Spinner,
  StatCard,
  fmtAgo,
  fmtMoney,
  plural,
  type IconName,
  type Tone,
} from '../../components/admin/ui';

type Metric = 'signups' | 'enrollments' | 'revenueCents';

const METRICS: { value: Metric; label: string }[] = [
  { value: 'signups', label: 'Регистрации' },
  { value: 'enrollments', label: 'Записи на курсы' },
  { value: 'revenueCents', label: 'Выручка' },
];

const PAYMENT_STATUS: Record<string, { label: string; tone: Tone }> = {
  succeeded: { label: 'Оплачен', tone: 'ok' },
  pending: { label: 'Ожидает', tone: 'warn' },
  failed: { label: 'Ошибка', tone: 'danger' },
  refunded: { label: 'Возврат', tone: 'neutral' },
};

/** Столбчатый график за период (SVG, без библиотек). */
function BarChart({ data, metric }: { data: OverviewDay[]; metric: Metric }): JSX.Element {
  const [hover, setHover] = useState<number | null>(null);
  const values = data.map((d) => d[metric]);
  const max = Math.max(1, ...values);
  const total = values.reduce((s, v) => s + v, 0);
  const fmt = (v: number): string => (metric === 'revenueCents' ? fmtMoney(v) : String(v));
  const W = 720;
  const H = 180;
  const gap = 4;
  const bw = (W - gap * (data.length - 1)) / data.length;
  const shown = hover ?? data.length - 1;
  const day = data[shown];
  const dayLabel = (iso: string): string =>
    new Date(`${iso}T12:00:00`).toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' });
  return (
    <div className="ov-chart">
      <div className="ov-chart-head">
        <div>
          <span className="ov-chart-total">{fmt(total)}</span>
          <span className="ax-muted"> за {data.length} {plural(data.length, 'день', 'дня', 'дней')}</span>
        </div>
        {day && (
          <span className="ov-chart-day">
            {dayLabel(day.date)}: <strong>{fmt(day[metric])}</strong>
          </span>
        )}
      </div>
      <svg
        className="ov-chart-svg"
        viewBox={`0 0 ${W} ${H}`}
        preserveAspectRatio="none"
        role="img"
        aria-label={`${METRICS.find((m) => m.value === metric)?.label}: всего ${fmt(total)}`}
        onMouseLeave={() => setHover(null)}
      >
        {[0.25, 0.5, 0.75].map((f) => (
          <line key={f} x1="0" x2={W} y1={H * f} y2={H * f} className="ov-chart-grid" />
        ))}
        {data.map((d, i) => {
          const v = d[metric];
          const h = v === 0 ? 2 : Math.max(4, (v / max) * (H - 8));
          return (
            <g key={d.date} onMouseEnter={() => setHover(i)}>
              <rect x={i * (bw + gap)} y={0} width={bw} height={H} fill="transparent" />
              <rect
                x={i * (bw + gap)}
                y={H - h}
                width={bw}
                height={h}
                rx={Math.min(4, bw / 3)}
                className={`ov-bar${i === shown ? ' is-active' : ''}${v === 0 ? ' is-zero' : ''}`}
              />
            </g>
          );
        })}
      </svg>
      <div className="ov-chart-axis" aria-hidden="true">
        <span>{data[0] && dayLabel(data[0].date)}</span>
        <span>{data[Math.floor(data.length / 2)] && dayLabel(data[Math.floor(data.length / 2)].date)}</span>
        <span>сегодня</span>
      </div>
    </div>
  );
}

type AttentionItem = { key: string; n: number; label: string; to: string; icon: IconName; tone: Tone };

export default function Overview(): JSX.Element {
  const [metric, setMetric] = useState<Metric>('signups');
  const overview = useQuery({ queryKey: overviewQueryKey, queryFn: () => getAdminOverview(30), staleTime: 30_000 });
  const stats = useQuery({ queryKey: ['admin', 'stats'], queryFn: getAdminStats, staleTime: 60_000 });

  const attention = useMemo<AttentionItem[]>(() => {
    const a = overview.data?.attention;
    if (!a) return [];
    const items: AttentionItem[] = [
      { key: 'leads', n: a.newLeads, label: plural(a.newLeads, 'новая заявка', 'новые заявки', 'новых заявок'), to: '/admin/leads', icon: 'inbox', tone: 'brand' },
      { key: 'queue', n: a.reviewQueue, label: plural(a.reviewQueue, 'курс ждёт проверки', 'курса ждут проверки', 'курсов ждут проверки'), to: '/admin/moderation', icon: 'shield', tone: 'warn' },
      { key: 'qa', n: a.unansweredQuestions, label: plural(a.unansweredQuestions, 'вопрос без ответа', 'вопроса без ответа', 'вопросов без ответа'), to: '/admin/questions', icon: 'message', tone: 'info' },
      { key: 'low', n: a.lowRatingNoReply, label: plural(a.lowRatingNoReply, 'низкая оценка без ответа', 'низкие оценки без ответа', 'низких оценок без ответа'), to: '/admin/reviews?filter=low', icon: 'star', tone: 'danger' },
      { key: 'failed', n: a.failedPayments7d, label: plural(a.failedPayments7d, 'неудачный платёж за неделю', 'неудачных платежа за неделю', 'неудачных платежей за неделю'), to: '/admin/payments?status=failed', icon: 'card', tone: 'danger' },
      { key: 'pending', n: a.pendingPayments, label: plural(a.pendingPayments, 'платёж ожидает оплаты', 'платежа ожидают оплаты', 'платежей ожидают оплаты'), to: '/admin/payments?status=pending', icon: 'clock', tone: 'neutral' },
    ];
    return items.filter((i) => i.n > 0);
  }, [overview.data]);

  const s = stats.data;

  return (
    <AdminPage
      title="Обзор"
      description="Что происходит на платформе и что требует вашего внимания."
      actions={
        <>
          <Link className="btn btn-ghost" to="/admin/enrollments?grant=1">
            <Icon name="key" size={17} /> Выдать доступ
          </Link>
          <Link className="btn btn-ghost" to="/admin/users?new=1">
            <Icon name="user" size={17} /> Создать пользователя
          </Link>
          <Link className="btn btn-primary" to="/teach/new">
            <Icon name="plus" size={17} /> Новый курс
          </Link>
        </>
      }
    >
      {overview.isError && <ErrorNote error={overview.error} onRetry={() => void overview.refetch()} />}

      <Card title="Требует внимания">
        {overview.isLoading ? (
          <Spinner />
        ) : attention.length === 0 ? (
          <Note tone="ok">
            Всё спокойно: новых заявок, курсов на проверке, вопросов без ответа и проблемных платежей нет.
          </Note>
        ) : (
          <div className="ov-attention">
            {attention.map((it) => (
              <Link key={it.key} to={it.to} className="ov-att">
                <span className={`ov-att-icon ax-tone--${it.tone}`}>
                  <Icon name={it.icon} size={20} />
                </span>
                <span className="ov-att-text">
                  <strong>{it.n}</strong> {it.label}
                </span>
                <Icon name="chevronRight" size={18} className="ov-att-go" />
              </Link>
            ))}
          </div>
        )}
      </Card>

      <div className="ax-grid ax-grid--4">
        <StatCard
          label="Пользователи"
          icon="users"
          tone="brand"
          to="/admin/users"
          value={s ? s.users.total : '—'}
          hint={s ? `+${s.users.newLast7} за неделю · ${s.users.teachers} ${plural(s.users.teachers, 'преподаватель', 'преподавателя', 'преподавателей')}` : undefined}
        />
        <StatCard
          label="Курсы"
          icon="book"
          tone="info"
          to="/admin/courses"
          value={s ? s.courses.published : '—'}
          hint={s ? `опубликовано из ${s.courses.total} · черновиков ${s.courses.draft}` : undefined}
        />
        <StatCard
          label="Активные записи"
          icon="key"
          tone="ok"
          to="/admin/enrollments"
          value={s ? s.enrollments.active : '—'}
          hint={s ? `всего записей ${s.enrollments.total}` : undefined}
        />
        <StatCard
          label="Выручка за 30 дней"
          icon="card"
          tone="warn"
          to="/admin/payments"
          value={s ? fmtMoney(s.payments.last30Cents) : '—'}
          hint={s ? `за всё время ${fmtMoney(s.payments.succeededCents)} · комиссия ${fmtMoney(s.payments.platformFeeCents)}` : undefined}
        />
      </div>

      <Card
        title="Динамика"
        description="По дням, московское время"
        actions={<Segmented label="Показатель" value={metric} onChange={setMetric} options={METRICS} />}
      >
        {overview.data ? <BarChart data={overview.data.series} metric={metric} /> : <Spinner />}
      </Card>

      <div className="ax-grid ax-grid--3">
        <Card title="Новые пользователи" actions={<Link className="ax-link" to="/admin/users">Все</Link>}>
          {overview.data && overview.data.recentUsers.length === 0 && <EmptyState icon="users" title="Пока никого" />}
          <ul className="ov-list">
            {overview.data?.recentUsers.map((u) => (
              <li key={u.id}>
                <Link to={`/admin/users/${u.id}`} className="ov-row">
                  <Avatar name={u.displayName || u.email} size={34} />
                  <span className="ax-person-text">
                    <strong>{u.displayName || u.email}</strong>
                    <small>
                      {ROLE_LABEL[u.role] ?? u.role} · {fmtAgo(u.createdAt)}
                    </small>
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        </Card>

        <Card title="Последние платежи" actions={<Link className="ax-link" to="/admin/payments">Все</Link>}>
          {overview.data && overview.data.recentPayments.length === 0 && (
            <EmptyState icon="card" title="Платежей за месяц нет" />
          )}
          <ul className="ov-list">
            {overview.data?.recentPayments.map((p) => (
              <li key={p.id}>
                <Link to={`/admin/users/${p.studentId}`} className="ov-row">
                  <span className="ax-person-text">
                    <strong>{p.courseTitle}</strong>
                    <small>
                      {p.studentEmail} · {fmtAgo(p.createdAt)}
                    </small>
                  </span>
                  <span className="ov-row-end">
                    <span className="ax-num">{fmtMoney(p.amountCents)}</span>
                    <Badge tone={PAYMENT_STATUS[p.status]?.tone ?? 'neutral'}>{PAYMENT_STATUS[p.status]?.label ?? p.status}</Badge>
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        </Card>

        <Card title="Действия админов" actions={<Link className="ax-link" to="/admin/audit">Журнал</Link>}>
          {overview.data && overview.data.recentAudit.length === 0 && (
            <EmptyState icon="history" title="Пока пусто" text="Здесь появятся изменения, сделанные в админке и через MCP." />
          )}
          <ul className="ov-list ov-audit">
            {overview.data?.recentAudit.map((e) => (
              <li key={e.id}>
                <span className="ov-audit-text">{e.action}</span>
                <small className="ax-muted">
                  {e.actorEmail} · {fmtAgo(e.createdAt)}
                </small>
              </li>
            ))}
          </ul>
        </Card>
      </div>
    </AdminPage>
  );
}
