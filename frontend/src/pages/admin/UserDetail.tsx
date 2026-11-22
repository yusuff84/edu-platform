import { useMemo } from 'react';
import { Link, useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError } from '../../api/client';
import {
  adminCompleteCourse,
  adminIssueCertificate,
  adminResetProgress,
  adminRevokeCertificate,
  type AdminCertificate,
  type AdminUserCourse,
} from '../../api/adminOps';
import { revokeEnrollment, unrevokeEnrollment } from '../../api/adminEnrollments';
import type { AdminCourseRow } from '../../api/adminConsole';
import type { PaymentJournalItem } from '../../api/types';
import {
  getAdminUserCard,
  listUserCertificates,
  listUserPayments,
  listUserTeaching,
  userKey,
} from '../../api/adminUsersHub';
import {
  AdminPage,
  Avatar,
  Badge,
  Card,
  CopyButton,
  DataTable,
  EmptyState,
  ErrorNote,
  Icon,
  Note,
  RowMenu,
  Spinner,
  StatCard,
  Tabs,
  errorText,
  fmtDate,
  fmtDateTime,
  fmtMoney,
  plural,
  useAdminUi,
  type Column,
  type MenuItem,
  type Tone,
} from '../../components/admin/ui';
import { RoleBadge, StatusBadge, useInvalidateUsers, useUserActions } from './Users';
import '../../styles/admin-users.css';

type TabId = 'courses' | 'payments' | 'certificates' | 'teaching';

const ACCESS: Record<string, { label: string; tone: Tone }> = {
  active: { label: 'Доступ открыт', tone: 'ok' },
  revoked: { label: 'Доступ закрыт', tone: 'danger' },
  refunded: { label: 'Возврат', tone: 'neutral' },
};

const PAYMENT_STATUS: Record<string, { label: string; tone: Tone }> = {
  succeeded: { label: 'Оплачен', tone: 'ok' },
  pending: { label: 'Ожидает', tone: 'warn' },
  failed: { label: 'Ошибка', tone: 'danger' },
  refunded: { label: 'Возврат', tone: 'neutral' },
};

const COURSE_STATUS: Record<string, { label: string; tone: Tone }> = {
  draft: { label: 'Черновик', tone: 'neutral' },
  review: { label: 'На проверке', tone: 'warn' },
  published: { label: 'Опубликован', tone: 'ok' },
  archived: { label: 'В архиве', tone: 'neutral' },
};

const pct = (c: AdminUserCourse): number => (c.totalLessons ? Math.round((c.completedLessons / c.totalLessons) * 100) : 0);

function Progress({ course }: { course: AdminUserCourse }): JSX.Element {
  const p = pct(course);
  return (
    <div className="aus-progress-cell">
      <div
        className={`aus-progress${p >= 100 ? ' is-done' : ''}`}
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={p}
        aria-label={`Прогресс ${p}%`}
      >
        <i style={{ width: `${p}%` }} />
      </div>
      <small className="ax-muted ax-num">
        {course.completedLessons} из {course.totalLessons} · {p}%
      </small>
    </div>
  );
}

export default function UserDetail(): JSX.Element {
  const { id = '' } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { confirm, toast } = useAdminUi();
  const invalidateUsers = useInvalidateUsers();
  const [params, setParams] = useSearchParams();

  const from = (location.state as { from?: string } | null)?.from ?? '';
  const backTo = `/admin/users${from}`;

  const card = useQuery({
    queryKey: userKey(id),
    queryFn: () => getAdminUserCard(id),
    retry: (n, e) => !(e instanceof ApiError) && n < 2,
  });
  const user = card.data?.user;
  const courses = card.data?.items ?? [];
  const showTeaching = Boolean(user && (user.role !== 'student' || user.coursesCount > 0));

  const payments = useQuery({
    queryKey: [...userKey(id), 'payments', user?.email],
    queryFn: () => listUserPayments(user!),
    enabled: Boolean(user),
  });
  const certs = useQuery({
    queryKey: [...userKey(id), 'certificates', user?.email],
    queryFn: () => listUserCertificates(user!),
    enabled: Boolean(user),
  });
  const teaching = useQuery({
    queryKey: [...userKey(id), 'teaching', user?.email],
    queryFn: () => listUserTeaching(user!),
    enabled: showTeaching,
  });

  const actions = useUserActions({ onDeleted: () => navigate(backTo, { replace: true }) });

  const rawTab = params.get('tab') as TabId | null;
  const tab: TabId =
    rawTab === 'payments' || rawTab === 'certificates' || (rawTab === 'teaching' && showTeaching) ? rawTab : 'courses';
  const setTab = (t: TabId): void => {
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (t === 'courses') next.delete('tab');
        else next.set('tab', t);
        return next;
      },
      { replace: true, state: location.state },
    );
  };

  const summary = useMemo(() => {
    const done = courses.filter((c) => (c.totalLessons > 0 && c.completedLessons >= c.totalLessons) || c.certificateNumber).length;
    const active = courses.filter((c) => c.status === 'active').length;
    const avg = courses.length ? Math.round(courses.reduce((s, c) => s + pct(c), 0) / courses.length) : 0;
    const paid = (payments.data ?? []).filter((p) => p.status === 'succeeded');
    return { done, active, avg, paidCents: paid.reduce((s, p) => s + p.amountCents, 0), paidCount: paid.length };
  }, [courses, payments.data]);

  /* ---------- Действия по курсу ---------- */

  function refreshCourses(): void {
    invalidateUsers(id);
    void qc.invalidateQueries({ queryKey: ['admin-enrollments'] });
  }

  async function run(action: () => Promise<string>): Promise<void> {
    try {
      toast(await action());
    } catch (e) {
      toast(errorText(e), 'error');
    } finally {
      refreshCourses();
    }
  }

  function courseMenu(c: AdminUserCourse): MenuItem[] {
    const items: MenuItem[] = [
      { label: 'Открыть курс в админке', icon: 'book', to: `/admin/courses/${c.courseId}` },
      'divider',
      {
        label: 'Засчитать курс',
        icon: 'check',
        onClick: async () => {
          const ok = await confirm({
            title: `Засчитать курс «${c.courseTitle}»?`,
            text: 'Все уроки будут отмечены пройденными, а пользователь получит сертификат.',
            confirmLabel: 'Засчитать',
          });
          if (ok) {
            await run(async () => {
              const r = await adminCompleteCourse(user!.id, c.courseId);
              return `Курс засчитан, сертификат ${r.certificateNumber}`;
            });
          }
        },
      },
    ];
    if (!c.certificateNumber) {
      items.push({
        label: 'Выдать сертификат',
        icon: 'award',
        onClick: async () => {
          const ok = await confirm({
            title: 'Выдать сертификат без прохождения?',
            text: `Сертификат по курсу «${c.courseTitle}» появится сразу, прогресс останется как есть. Подходит, если курс пройден вне платформы.`,
            confirmLabel: 'Выдать',
          });
          if (ok) {
            await run(async () => {
              const r = await adminIssueCertificate({ studentId: user!.id, courseId: c.courseId });
              return `Сертификат выдан: ${r.certificateNumber}`;
            });
          }
        },
      });
    }
    items.push({
      label: 'Сбросить прогресс',
      icon: 'refresh',
      onClick: async () => {
        const ok = await confirm({
          title: `Сбросить прогресс по курсу «${c.courseTitle}»?`,
          text: 'Отметки о пройденных уроках будут удалены — пользователь начнёт курс заново. Сертификат при этом останется (его можно отозвать отдельно).',
          confirmLabel: 'Сбросить',
          danger: true,
        });
        if (ok) {
          await run(async () => {
            const r = await adminResetProgress(user!.id, c.courseId);
            return `Прогресс сброшен (${r.deleted} ${plural(r.deleted, 'урок', 'урока', 'уроков')})`;
          });
        }
      },
    });
    items.push('divider');
    if (c.status === 'active') {
      items.push({
        label: 'Закрыть доступ',
        icon: 'lock',
        danger: true,
        onClick: async () => {
          const ok = await confirm({
            title: `Закрыть доступ к курсу «${c.courseTitle}»?`,
            text: 'Пользователь перестанет видеть уроки. Прогресс сохранится — доступ можно вернуть в любой момент. Деньги автоматически не возвращаются.',
            confirmLabel: 'Закрыть доступ',
            danger: true,
          });
          if (ok) {
            await run(async () => {
              await revokeEnrollment(user!.id, c.courseId);
              return 'Доступ к курсу закрыт';
            });
          }
        },
      });
    } else {
      items.push({
        label: 'Вернуть доступ',
        icon: 'unlock',
        onClick: () =>
          void run(async () => {
            await unrevokeEnrollment(user!.id, c.courseId);
            return 'Доступ к курсу возвращён';
          }),
      });
    }
    if (c.certificateNumber) {
      items.push({
        label: 'Отозвать сертификат',
        icon: 'trash',
        danger: true,
        onClick: () => void revokeCert(c.certificateNumber, c.courseTitle),
      });
    }
    return items;
  }

  async function revokeCert(number: string, courseTitle: string): Promise<void> {
    const ok = await confirm({
      title: `Отозвать сертификат ${number}?`,
      text: `Сертификат по курсу «${courseTitle}» станет недействительным, ссылка на него перестанет открываться.`,
      confirmLabel: 'Отозвать',
      danger: true,
    });
    if (!ok) return;
    await run(async () => {
      await adminRevokeCertificate(number);
      return 'Сертификат отозван';
    });
  }

  /* ---------- Загрузка / не найден ---------- */

  if (card.isLoading) {
    return (
      <AdminPage title="Пользователь" back={{ to: backTo, label: 'Пользователи' }}>
        <Spinner />
      </AdminPage>
    );
  }
  if (!user) {
    const notFound = card.error instanceof ApiError && (card.error.status === 404 || card.error.status === 400);
    return (
      <AdminPage title="Пользователь" back={{ to: backTo, label: 'Пользователи' }}>
        {notFound || !card.error ? (
          <Card>
            <EmptyState
              icon="user"
              title="Пользователь не найден"
              text="Возможно, его удалили или ссылка неверная."
              action={
                <Link className="btn btn-ghost btn-sm" to={backTo}>
                  К списку пользователей
                </Link>
              }
            />
          </Card>
        ) : (
          <ErrorNote error={card.error} onRetry={() => void card.refetch()} />
        )}
      </AdminPage>
    );
  }

  const name = user.displayName || user.email.replace(/@.*/, '');

  /* ---------- Колонки вкладок ---------- */

  const courseCols: Column<AdminUserCourse>[] = [
    {
      key: 'course',
      header: 'Курс',
      mobile: 'title',
      render: (c) => (
        <span className="aus-cell-title">
          <Link className="aus-strong-link" to={`/admin/courses/${c.courseId}`}>
            {c.courseTitle}
          </Link>
          <small className="ax-muted">Записан {fmtDate(c.enrolledAt)}</small>
        </span>
      ),
    },
    { key: 'progress', header: 'Прогресс', width: '200px', render: (c) => <Progress course={c} /> },
    {
      key: 'access',
      header: 'Доступ',
      width: '150px',
      render: (c) => {
        const a = ACCESS[c.status] ?? { label: c.status, tone: 'neutral' as Tone };
        return <Badge tone={a.tone}>{a.label}</Badge>;
      },
    },
    {
      key: 'cert',
      header: 'Сертификат',
      width: '170px',
      render: (c) =>
        c.certificateNumber ? (
          <a className="ax-link ax-mono" href={`/certificates/${encodeURIComponent(c.certificateNumber)}`} target="_blank" rel="noopener noreferrer">
            {c.certificateNumber}
          </a>
        ) : (
          <span className="ax-muted">—</span>
        ),
    },
  ];

  const paymentCols: Column<PaymentJournalItem>[] = [
    {
      key: 'course',
      header: 'Курс',
      mobile: 'title',
      render: (p) => (
        <span className="aus-cell-title">
          <Link className="aus-strong-link" to={`/admin/courses/${p.courseId}`}>
            {p.courseTitle || 'Курс удалён'}
          </Link>
          {p.provider && <small className="ax-muted">{p.provider}</small>}
        </span>
      ),
    },
    { key: 'amount', header: 'Сумма', width: '130px', align: 'right', render: (p) => <span className="ax-num">{fmtMoney(p.amountCents)}</span> },
    {
      key: 'status',
      header: 'Статус',
      width: '130px',
      render: (p) => {
        const s = PAYMENT_STATUS[p.status] ?? { label: p.status, tone: 'neutral' as Tone };
        return <Badge tone={s.tone}>{s.label}</Badge>;
      },
    },
    { key: 'date', header: 'Дата', width: '150px', render: (p) => <span className="ax-num">{fmtDateTime(p.createdAt)}</span> },
  ];

  const certCols: Column<AdminCertificate>[] = [
    {
      key: 'number',
      header: 'Сертификат',
      mobile: 'title',
      render: (c) => (
        <span className="aus-cell-title">
          <a className="aus-strong-link ax-mono" href={`/certificates/${encodeURIComponent(c.number)}`} target="_blank" rel="noopener noreferrer">
            {c.number} <Icon name="external" size={14} />
          </a>
          <small className="ax-muted">{c.studentName || c.email}</small>
        </span>
      ),
    },
    {
      key: 'course',
      header: 'Курс',
      render: (c) => (
        <Link className="ax-link" to={`/admin/courses/${c.courseId}`}>
          {c.courseTitle}
        </Link>
      ),
    },
    { key: 'issued', header: 'Выдан', width: '150px', render: (c) => <span className="ax-num">{fmtDate(c.issuedAt)}</span> },
  ];

  const teachCols: Column<AdminCourseRow>[] = [
    {
      key: 'course',
      header: 'Курс',
      mobile: 'title',
      render: (c) => (
        <span className="aus-cell-title">
          <Link className="aus-strong-link" to={`/admin/courses/${c.id}`}>
            {c.title}
          </Link>
          <small className="ax-muted">
            {c.modulesCount} {plural(c.modulesCount, 'модуль', 'модуля', 'модулей')} · {c.lessonsCount}{' '}
            {plural(c.lessonsCount, 'урок', 'урока', 'уроков')}
          </small>
        </span>
      ),
    },
    {
      key: 'status',
      header: 'Статус',
      width: '140px',
      render: (c) => {
        const s = COURSE_STATUS[c.status] ?? { label: c.status, tone: 'neutral' as Tone };
        return <Badge tone={s.tone}>{s.label}</Badge>;
      },
    },
    { key: 'price', header: 'Цена', width: '120px', align: 'right', render: (c) => <span className="ax-num">{c.priceCents > 0 ? fmtMoney(c.priceCents) : 'Бесплатно'}</span> },
    { key: 'students', header: 'Учеников', width: '110px', align: 'right', render: (c) => <span className="ax-num">{c.studentsCount}</span> },
  ];

  const tabs: { id: TabId; label: string; count?: number }[] = [
    { id: 'courses', label: 'Курсы', count: courses.length },
    { id: 'payments', label: 'Платежи', count: payments.data?.length },
    { id: 'certificates', label: 'Сертификаты', count: certs.data?.length },
  ];
  if (showTeaching) tabs.push({ id: 'teaching', label: 'Преподавание', count: teaching.data?.length });

  return (
    <AdminPage
      back={{ to: backTo, label: 'Пользователи' }}
      title={
        <span className="aus-title">
          <Avatar name={user.displayName || user.email} size={52} />
          <span className="aus-title-text">{name}</span>
        </span>
      }
      description={
        <span className="aus-sub">
          <span className="aus-sub-email">{user.email}</span>
          <RoleBadge role={user.role} />
          <StatusBadge status={user.status} />
          {user.emailVerified !== undefined && (
            <Badge tone={user.emailVerified ? 'ok' : 'neutral'}>
              {user.emailVerified ? 'Email подтверждён' : 'Email не подтверждён'}
            </Badge>
          )}
          <span className="aus-sub-date">с {fmtDate(user.createdAt)}</span>
        </span>
      }
      actions={
        <div className="aus-hero-actions">
          <button type="button" className="btn btn-ghost" onClick={() => actions.open.profile(user)}>
            <Icon name="edit" size={17} /> Профиль
          </button>
          <button type="button" className="btn btn-ghost" onClick={() => actions.open.password(user)}>
            <Icon name="key" size={17} /> Пароль
          </button>
          <button type="button" className="btn btn-ghost" onClick={() => actions.open.role(user)}>
            <Icon name="shield" size={17} /> Роль
          </button>
          <span className="aus-more">
            <RowMenu
              label="Другие действия"
              items={[
                { label: 'Выдать курс', icon: 'book', onClick: () => actions.open.grant(user) },
                { label: 'Выйти на всех устройствах', icon: 'logout', onClick: () => void actions.logoutAll(user) },
                'divider',
                user.status === 'blocked'
                  ? { label: 'Разблокировать', icon: 'unlock', onClick: () => void actions.toggleBlock(user) }
                  : { label: 'Заблокировать', icon: 'lock', onClick: () => void actions.toggleBlock(user) },
                { label: 'Удалить пользователя', icon: 'trash', danger: true, onClick: () => void actions.remove(user) },
              ]}
            />
          </span>
        </div>
      }
    >
      {user.status === 'blocked' && (
        <Note tone="warn">
          <span className="aus-note-row">
            <span>Пользователь заблокирован и не может войти на сайт. Его курсы и история сохранены.</span>
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => void actions.toggleBlock(user)}>
              <Icon name="unlock" size={16} /> Разблокировать
            </button>
          </span>
        </Note>
      )}

      <div className="ax-grid ax-grid--4">
        <StatCard
          label="Курсы"
          icon="book"
          tone="brand"
          value={courses.length}
          hint={courses.length ? `${summary.active} с открытым доступом` : 'Пока не записан'}
        />
        <StatCard
          label="Пройдено"
          icon="check"
          tone="ok"
          value={summary.done}
          hint={courses.length ? `Средний прогресс ${summary.avg}%` : '—'}
        />
        <StatCard label="Сертификаты" icon="award" tone="info" value={certs.data?.length ?? '…'} />
        <StatCard
          label="Оплачено"
          icon="card"
          tone="neutral"
          value={payments.data ? fmtMoney(summary.paidCents) : '…'}
          hint={payments.data ? `${summary.paidCount} ${plural(summary.paidCount, 'платёж', 'платежа', 'платежей')}` : undefined}
        />
      </div>

      <Card flush className="aus-tabs-card">
        <div className="aus-tabs">
          <Tabs label="Разделы карточки" tabs={tabs} value={tab} onChange={setTab} />
        </div>

        {tab === 'courses' && (
          <>
            <div className="aus-tab-head">
              <p className="ax-muted">Прогресс, доступ и сертификаты по каждому курсу.</p>
              <button type="button" className="btn btn-primary btn-sm" onClick={() => actions.open.grant(user)}>
                <Icon name="plus" size={16} /> Выдать курс
              </button>
            </div>
            <DataTable
              columns={courseCols}
              rows={courses}
              rowKey={(c) => c.courseId}
              caption="Курсы пользователя"
              actions={(c) => <RowMenu items={courseMenu(c)} label={`Действия: ${c.courseTitle}`} />}
              empty={
                <EmptyState
                  icon="book"
                  title="Пользователь не записан ни на один курс"
                  text="Можно открыть доступ вручную — даже к платному курсу."
                  action={
                    <button type="button" className="btn btn-primary btn-sm" onClick={() => actions.open.grant(user)}>
                      Выдать курс
                    </button>
                  }
                />
              }
            />
          </>
        )}

        {tab === 'payments' && (
          <>
            <div className="aus-tab-head">
              <p className="ax-muted">Курсы, выданные вручную, сюда не попадают — платёж для них не создаётся.</p>
              <Link className="btn btn-ghost btn-sm" to={`/admin/payments?q=${encodeURIComponent(user.email)}`}>
                В журнал платежей
              </Link>
            </div>
            {payments.isLoading && <Spinner />}
            {payments.isError && (
              <div className="aus-pad">
                <ErrorNote error={payments.error} onRetry={() => void payments.refetch()} />
              </div>
            )}
            {payments.data && (
              <DataTable
                columns={paymentCols}
                rows={payments.data}
                rowKey={(p) => p.id}
                caption="Платежи пользователя"
                empty={<EmptyState icon="card" title="Платежей нет" text="Пользователь ещё ничего не покупал." />}
              />
            )}
          </>
        )}

        {tab === 'certificates' && (
          <>
            {certs.isLoading && <Spinner />}
            {certs.isError && (
              <div className="aus-pad">
                <ErrorNote error={certs.error} onRetry={() => void certs.refetch()} />
              </div>
            )}
            {certs.data && (
              <DataTable
                columns={certCols}
                rows={certs.data}
                rowKey={(c) => c.number}
                caption="Сертификаты пользователя"
                actions={(c) => (
                  <RowMenu
                    label={`Действия: ${c.number}`}
                    items={[
                      { label: 'Открыть страницу сертификата', icon: 'external', href: `/certificates/${encodeURIComponent(c.number)}` },
                      {
                        label: 'Скопировать ссылку',
                        icon: 'copy',
                        onClick: () => {
                          const url = `${window.location.origin}/certificates/${encodeURIComponent(c.number)}`;
                          void navigator.clipboard?.writeText(url).then(
                            () => toast('Ссылка скопирована'),
                            () => toast(url, 'info'),
                          );
                        },
                      },
                      { label: 'Открыть курс в админке', icon: 'book', to: `/admin/courses/${c.courseId}` },
                      'divider',
                      { label: 'Отозвать сертификат', icon: 'trash', danger: true, onClick: () => void revokeCert(c.number, c.courseTitle) },
                    ]}
                  />
                )}
                empty={
                  <EmptyState
                    icon="award"
                    title="Сертификатов пока нет"
                    text="Сертификат выдаётся автоматически после прохождения курса. Его можно выдать и вручную — в меню курса на вкладке «Курсы»."
                  />
                }
              />
            )}
          </>
        )}

        {tab === 'teaching' && showTeaching && (
          <>
            {teaching.isLoading && <Spinner />}
            {teaching.isError && (
              <div className="aus-pad">
                <ErrorNote error={teaching.error} onRetry={() => void teaching.refetch()} />
              </div>
            )}
            {teaching.data && (
              <DataTable
                columns={teachCols}
                rows={teaching.data}
                rowKey={(c) => c.id}
                caption="Курсы преподавателя"
                onRowClick={(c) => navigate(`/admin/courses/${c.id}`)}
                empty={<EmptyState icon="book" title="Своих курсов нет" text="Пользователь пока не создал ни одного курса." />}
              />
            )}
          </>
        )}
      </Card>

      <p className="aus-id">
        <span className="ax-muted">ID пользователя:</span> <code className="ax-mono">{user.id}</code>
        <CopyButton text={user.id} label="Копировать" />
      </p>

      {actions.element}
    </AdminPage>
  );
}
