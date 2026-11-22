import { useMemo, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, createPromoCode, deactivatePromoCode, formatPrice } from '../../api/client';
import {
  listAdminCourseDetail,
  type AdminCourseDetail,
  type AdminCourseDetailLesson,
  type AdminCourseDetailModule,
} from '../../api/adminCourseDetail';
import { listAdminEnrollments, type AdminEnrollmentRow, type AdminEnrollmentStatus } from '../../api/adminEnrollments';
import { listAdminPromoCodes, listAdminQuestions, type AdminPromoCode } from '../../api/adminHub';
import { moveLesson } from '../../api/teacherExtra';
import { LESSON_KIND, contentKeys } from '../../api/adminContent';
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
  FilterSelect,
  Icon,
  InfoList,
  Modal,
  Note,
  Pager,
  RowMenu,
  SearchInput,
  Spinner,
  StatCard,
  Tabs,
  Toolbar,
  downloadCsv,
  errorText,
  fmtDate,
  fmtMoney,
  plural,
  useAdminUi,
  type Column,
  type Tone,
} from '../../components/admin/ui';
import { CourseStatusBadge, CourseThumb, useCourseOps } from './Courses';
import { ReviewsBoard } from './Reviews';
import { QuestionsBoard } from './Questions';
import '../../styles/admin-content.css';

type Tab = 'overview' | 'students' | 'reviews' | 'questions' | 'promo';
const TABS: Tab[] = ['overview', 'students', 'reviews', 'questions', 'promo'];

/** Все записи на курс (по 100 за запрос) — для цифр «завершили» и «средний прогресс». */
async function listAllEnrollments(courseId: string): Promise<AdminEnrollmentRow[]> {
  const out: AdminEnrollmentRow[] = [];
  for (let page = 1; page <= 50; page += 1) {
    const res = await listAdminEnrollments({ courseId, page, limit: 100 });
    out.push(...res.items);
    if (res.items.length < 100 || out.length >= res.total) break;
  }
  return out;
}

const pct = (v: number): number => (Number.isFinite(v) ? Math.min(100, Math.max(0, Math.round(v))) : 0);

export default function CourseDetail(): JSX.Element {
  const { id = '' } = useParams();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const tabRaw = params.get('tab') ?? '';
  const tab: Tab = (TABS as string[]).includes(tabRaw) ? (tabRaw as Tab) : 'overview';
  const ops = useCourseOps({ onDuplicated: (newId) => navigate(`/admin/courses/${newId}`) });

  const detail = useQuery({
    queryKey: contentKeys.course(id),
    queryFn: () => listAdminCourseDetail(id),
    enabled: id !== '',
    retry: (n, e) => !(e instanceof ApiError) && n < 2,
  });
  const enrollments = useQuery({
    queryKey: contentKeys.enrollments(id),
    queryFn: () => listAllEnrollments(id),
    enabled: id !== '',
  });
  const unanswered = useQuery({
    queryKey: [...contentKeys.questions, 'unanswered-count', id],
    queryFn: () => listAdminQuestions({ status: 'unanswered', courseId: id, page: 1, limit: 1 }),
    enabled: id !== '',
  });
  const promo = useQuery({
    queryKey: contentKeys.promo(id),
    queryFn: () => listAdminPromoCodes(id),
    enabled: id !== '',
  });

  if (detail.isLoading) {
    return (
      <AdminPage title="Курс" back={{ to: '/admin/courses', label: 'Курсы' }}>
        <Spinner />
      </AdminPage>
    );
  }
  if (detail.isError || !detail.data) {
    const notFound = detail.error instanceof ApiError && detail.error.status === 404;
    return (
      <AdminPage title={notFound ? 'Курс не найден' : 'Курс'} back={{ to: '/admin/courses', label: 'Курсы' }}>
        {notFound ? (
          <div className="ax-card">
            <EmptyState
              icon="book"
              title="Такого курса нет"
              text="Возможно, его удалили или ссылка неверная."
              action={
                <Link className="btn btn-ghost" to="/admin/courses">
                  К списку курсов
                </Link>
              }
            />
          </div>
        ) : (
          <ErrorNote error={detail.error} onRetry={() => void detail.refetch()} />
        )}
      </AdminPage>
    );
  }

  const { course, stats } = detail.data;
  const opsCourse = { id: course.id, slug: course.slug, title: course.title, status: course.status, teacherEmail: course.teacherEmail };
  const activePromo = (promo.data?.items ?? []).filter((p) => p.active).length;

  return (
    <AdminPage
      back={{ to: '/admin/courses', label: 'Курсы' }}
      title={
        <span className="acn-title">
          <CourseThumb title={course.title} coverPath={course.coverPath} size={56} />
          <span>{course.title}</span>
        </span>
      }
      description={
        <span className="acn-meta">
          <CourseStatusBadge status={course.status} />
          <span>
            Преподаватель:{' '}
            {course.teacherId ? <Link to={`/admin/users/${course.teacherId}`}>{course.teacherEmail || 'профиль'}</Link> : '—'}
          </span>
          <span>
            Цена: <b className="ax-num">{formatPrice(course.priceCents)}</b>
          </span>
        </span>
      }
      actions={
        <>
          <Link className="btn btn-ghost" to={`/teach/${course.id}`}>
            <Icon name="edit" size={17} /> Редактировать
          </Link>
          <a className="btn btn-ghost" href={`/courses/${encodeURIComponent(course.slug)}`} target="_blank" rel="noopener noreferrer">
            <Icon name="external" size={17} /> <span className="acn-hide-m">Открыть на сайте</span>
            <span className="acn-show-m">На сайте</span>
          </a>
          <span className="acn-head-menu">
            <RowMenu label="Другие действия с курсом" items={ops.menu(opsCourse, { withOpen: false, withEdit: false, withSite: false })} />
          </span>
        </>
      }
    >
      {course.status === 'review' && (
        <div className="acn-review-banner">
          <span>
            <Icon name="shield" size={18} /> Курс ждёт проверки. Посмотрите программу и примите решение.
          </span>
          <span className="acn-review-banner-actions">
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => ops.reject(opsCourse)}>
              Отклонить
            </button>
            <button type="button" className="btn btn-primary btn-sm" onClick={() => ops.approve(opsCourse)}>
              Одобрить и опубликовать
            </button>
          </span>
        </div>
      )}

      <Tabs<Tab>
        label="Разделы курса"
        value={tab}
        onChange={(t) => {
          const next = new URLSearchParams(params);
          if (t === 'overview') next.delete('tab');
          else next.set('tab', t);
          setParams(next, { replace: true });
        }}
        tabs={[
          { id: 'overview', label: 'Обзор' },
          { id: 'students', label: 'Ученики', count: stats.studentsCount },
          { id: 'reviews', label: 'Отзывы', count: stats.reviewsCount },
          { id: 'questions', label: 'Вопросы', count: unanswered.data?.unanswered },
          { id: 'promo', label: 'Промокоды', count: activePromo },
        ]}
      />

      {tab === 'overview' && <OverviewTab data={detail.data} enrollments={enrollments.data} />}
      {tab === 'students' && (
        <StudentsTab courseId={course.id} courseTitle={course.title} query={enrollments} />
      )}
      {tab === 'reviews' && (
        <Card>
          <ReviewsBoard mode="local" courseId={course.id} />
        </Card>
      )}
      {tab === 'questions' && (
        <Card>
          <QuestionsBoard mode="local" courseId={course.id} />
        </Card>
      )}
      {tab === 'promo' && <PromoTab course={course} query={promo} />}

      {ops.dialogs}
    </AdminPage>
  );
}

/* ------------------------------------------------------------------ */
/* Обзор                                                                */
/* ------------------------------------------------------------------ */

function OverviewTab({ data, enrollments }: { data: AdminCourseDetail; enrollments?: AdminEnrollmentRow[] }): JSX.Element {
  const { course, stats, modules } = data;
  const lessonsTotal = modules.reduce((n, m) => n + m.lessons.length, 0);
  const counted = enrollments?.filter((e) => e.status === 'active') ?? [];
  const completed = counted.filter((e) => e.totalLessons > 0 && e.completedLessons >= e.totalLessons).length;
  const avg = counted.length ? Math.round(counted.reduce((s, e) => s + pct(e.percent), 0) / counted.length) : 0;
  const siteUrl = `${window.location.origin}/courses/${encodeURIComponent(course.slug)}`;

  return (
    <>
      <div className="acn-stats">
        <StatCard label="Ученики" icon="users" tone="brand" value={stats.studentsCount} hint={`активных ${stats.activeCount}`} />
        <StatCard
          label="Завершили"
          icon="award"
          tone="ok"
          value={enrollments ? completed : '—'}
          hint={enrollments && counted.length ? `${Math.round((completed / counted.length) * 100)}% активных` : 'пока никто'}
        />
        <StatCard label="Прогресс" icon="chart" tone="info" value={enrollments ? `${avg}%` : '—'} hint="в среднем у активных" />
        <StatCard
          label="Рейтинг"
          icon="star"
          tone="warn"
          value={stats.reviewsCount ? stats.avgRating.toLocaleString('ru-RU', { minimumFractionDigits: 1, maximumFractionDigits: 1 }) : '—'}
          hint={`${stats.reviewsCount} ${plural(stats.reviewsCount, 'отзыв', 'отзыва', 'отзывов')}`}
        />
        <StatCard
          label="Выручка"
          icon="card"
          tone="neutral"
          value={fmtMoney(stats.paidTotalCents)}
          hint={`${stats.paymentsCount} ${plural(stats.paymentsCount, 'оплата', 'оплаты', 'оплат')}`}
        />
      </div>

      <div className="ax-grid ax-grid--side">
        <ProgramCard data={data} />
        <Card title="О курсе">
          <InfoList
            items={[
              { label: 'Программа', value: `${modules.length} ${plural(modules.length, 'модуль', 'модуля', 'модулей')}, ${lessonsTotal} ${plural(lessonsTotal, 'урок', 'урока', 'уроков')}` },
              { label: 'Создан', value: fmtDate(course.createdAt) },
              { label: 'Изменён', value: fmtDate(course.updatedAt) },
              {
                label: 'Адрес на сайте',
                value: (
                  <span className="acn-copyline">
                    <span className="ax-mono">/courses/{course.slug}</span>
                    <CopyButton text={siteUrl} label="Ссылка" />
                  </span>
                ),
              },
              {
                label: 'ID курса',
                value: (
                  <span className="acn-copyline">
                    <span className="ax-mono acn-id">{course.id}</span>
                    <CopyButton text={course.id} label="ID" />
                  </span>
                ),
              },
            ]}
          />
        </Card>
      </div>
    </>
  );
}

function ProgramCard({ data }: { data: AdminCourseDetail }): JSX.Element {
  const { course, modules } = data;
  const qc = useQueryClient();
  const { toast } = useAdminUi();
  const [moving, setMoving] = useState<{ lesson: AdminCourseDetailLesson; from: AdminCourseDetailModule } | null>(null);
  const [target, setTarget] = useState('');
  const canMove = course.status === 'draft' && modules.length > 1;

  const move = useMutation({
    mutationFn: () => moveLesson(course.id, moving!.lesson.id, target),
    onSuccess: () => {
      toast('Урок перенесён');
      setMoving(null);
      void qc.invalidateQueries({ queryKey: contentKeys.course(course.id) });
    },
    onError: (e) => toast(errorText(e), 'error'),
  });

  return (
    <Card
      title="Программа"
      description={
        modules.length > 3
          ? `Нажмите на модуль, чтобы увидеть уроки.${course.status === 'draft' ? '' : ' Менять структуру можно только в черновике.'}`
          : course.status === 'draft'
            ? undefined
            : 'Менять структуру можно только в черновике.'
      }
      actions={
        <Link className="btn btn-ghost btn-sm" to={`/teach/${course.id}`}>
          <Icon name="edit" size={16} /> В конструкторе
        </Link>
      }
    >
      {modules.length === 0 ? (
        <EmptyState icon="book" title="Программа пустая" text="Автор ещё не добавил модули и уроки." />
      ) : (
        <ol className="acn-program">
          {modules.map((m, mi) => (
            <li key={m.id} className="acn-module">
              <details open={modules.length <= 3}>
                <summary className="acn-module-head">
                  <span className="acn-module-num">{mi + 1}</span>
                  <strong>{m.title || 'Без названия'}</strong>
                  <span className="ax-muted">
                    {m.lessons.length} {plural(m.lessons.length, 'урок', 'урока', 'уроков')}
                  </span>
                  <Icon name="chevronRight" size={16} className="acn-module-chev" />
                </summary>
                {m.lessons.length === 0 ? (
                  <p className="acn-module-empty ax-muted">В модуле пока нет уроков</p>
                ) : (
                  <ol className="acn-lessons">
                    {m.lessons.map((l) => (
                      <li key={l.id}>
                        <span className="acn-lesson-title">{l.title || 'Без названия'}</span>
                        <Badge tone={l.kind === 'quiz' ? 'warn' : l.kind === 'live' ? 'info' : l.kind === 'code' ? 'brand' : 'neutral'}>
                          {LESSON_KIND[l.kind] ?? l.kind}
                        </Badge>
                        {canMove && (
                          <RowMenu
                            label={`Действия с уроком ${l.title}`}
                            items={[
                              {
                                label: 'Перенести в другой модуль…',
                                icon: 'chevronRight',
                                onClick: () => {
                                  setMoving({ lesson: l, from: m });
                                  setTarget(modules.find((x) => x.id !== m.id)?.id ?? '');
                                },
                              },
                            ]}
                          />
                        )}
                      </li>
                    ))}
                  </ol>
                )}
              </details>
            </li>
          ))}
        </ol>
      )}
      <Modal
        open={moving !== null}
        onClose={() => !move.isPending && setMoving(null)}
        title="Перенести урок"
        footer={
          <>
            <button type="button" className="btn btn-ghost" onClick={() => setMoving(null)} disabled={move.isPending}>
              Отмена
            </button>
            <button type="button" className="btn btn-primary" onClick={() => move.mutate()} disabled={!target || move.isPending}>
              {move.isPending ? 'Переносим…' : 'Перенести'}
            </button>
          </>
        }
      >
        <div className="ax-form">
          <p className="acn-modal-course">
            <strong>«{moving?.lesson.title}»</strong>
            <span className="ax-muted"> · сейчас в модуле «{moving?.from.title}»</span>
          </p>
          <Field label="Новый модуль" hint="Урок встанет в конец выбранного модуля.">
            <select className="ax-select" value={target} onChange={(e) => setTarget(e.target.value)}>
              {modules
                .filter((x) => x.id !== moving?.from.id)
                .map((x) => (
                  <option key={x.id} value={x.id}>
                    {x.title || 'Без названия'}
                  </option>
                ))}
            </select>
          </Field>
        </div>
      </Modal>
    </Card>
  );
}

/* ------------------------------------------------------------------ */
/* Ученики                                                              */
/* ------------------------------------------------------------------ */

const ENROLL_STATUS: Record<AdminEnrollmentStatus, { label: string; tone: Tone }> = {
  active: { label: 'Учится', tone: 'ok' },
  refunded: { label: 'Возврат', tone: 'warn' },
  revoked: { label: 'Доступ отозван', tone: 'danger' },
};

function StudentsTab({
  courseId,
  courseTitle,
  query,
}: {
  courseId: string;
  courseTitle: string;
  query: { data?: AdminEnrollmentRow[]; isError: boolean; error: unknown; refetch: () => unknown };
}): JSX.Element {
  const navigate = useNavigate();
  const [q, setQ] = useState('');
  const [status, setStatus] = useState('');
  const [page, setPage] = useState(1);
  const LIMIT = 20;

  const rows = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return (query.data ?? []).filter(
      (r) =>
        (!status || r.status === status) &&
        (!needle || `${r.studentName} ${r.studentEmail}`.toLowerCase().includes(needle)),
    );
  }, [query.data, q, status]);

  const columns: Column<AdminEnrollmentRow>[] = [
    {
      key: 'student',
      header: 'Ученик',
      mobile: 'title',
      render: (r) => (
        <span className="ax-person">
          <Avatar name={r.studentName || r.studentEmail} size={34} />
          <span className="ax-person-text">
            <strong>{r.studentName || r.studentEmail}</strong>
            {r.studentName && <small>{r.studentEmail}</small>}
          </span>
        </span>
      ),
    },
    {
      key: 'status',
      header: 'Доступ',
      width: '140px',
      render: (r) => <Badge tone={ENROLL_STATUS[r.status]?.tone ?? 'neutral'}>{ENROLL_STATUS[r.status]?.label ?? r.status}</Badge>,
    },
    {
      key: 'progress',
      header: 'Прогресс',
      width: '220px',
      render: (r) => (
        <span className="acn-progress" title={`${r.completedLessons} из ${r.totalLessons} уроков`}>
          <span className="acn-progress-bar" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={pct(r.percent)}>
            <span style={{ width: `${pct(r.percent)}%` }} />
          </span>
          <span className="ax-num">{pct(r.percent)}%</span>
          <small className="ax-muted ax-num">
            {r.completedLessons}/{r.totalLessons}
          </small>
        </span>
      ),
    },
    { key: 'created', header: 'Записан', width: '120px', render: (r) => <span className="ax-num">{fmtDate(r.createdAt)}</span> },
  ];

  return (
    <Card flush>
      <Toolbar
        end={
          <>
            <button
              type="button"
              className="btn btn-ghost"
              disabled={rows.length === 0}
              onClick={() =>
                downloadCsv(
                  `students-${courseTitle.slice(0, 40).replace(/[^\p{L}\p{N}]+/gu, '-')}.csv`,
                  ['Имя', 'Email', 'Доступ', 'Прогресс, %', 'Пройдено уроков', 'Всего уроков', 'Записан'],
                  rows.map((r) => [
                    r.studentName,
                    r.studentEmail,
                    ENROLL_STATUS[r.status]?.label ?? r.status,
                    pct(r.percent),
                    r.completedLessons,
                    r.totalLessons,
                    fmtDate(r.createdAt),
                  ]),
                )
              }
            >
              <Icon name="download" size={16} /> CSV
            </button>
            <Link className="btn btn-ghost" to={`/teach/${courseId}/students`}>
              Кабинет автора
            </Link>
          </>
        }
      >
        <SearchInput
          value={q}
          onChange={(v) => {
            setQ(v);
            setPage(1);
          }}
          placeholder="Имя или email ученика"
        />
        <FilterSelect
          label="Доступ"
          value={status}
          onChange={(v) => {
            setStatus(v);
            setPage(1);
          }}
          options={[
            { value: '', label: 'Любой доступ' },
            { value: 'active', label: 'Учатся' },
            { value: 'refunded', label: 'Возврат' },
            { value: 'revoked', label: 'Доступ отозван' },
          ]}
        />
      </Toolbar>
      {query.isError ? (
        <ErrorNote error={query.error} onRetry={() => void query.refetch()} />
      ) : !query.data ? (
        <Spinner />
      ) : (
        <>
          <DataTable<AdminEnrollmentRow>
            caption="Ученики курса"
            columns={columns}
            rows={rows.slice((page - 1) * LIMIT, page * LIMIT)}
            rowKey={(r) => r.id}
            onRowClick={(r) => navigate(`/admin/users/${r.studentId}`)}
            actions={(r) => (
              <RowMenu
                label={`Действия: ${r.studentEmail}`}
                items={[
                  { label: 'Профиль ученика', icon: 'user', to: `/admin/users/${r.studentId}` },
                  { label: 'Все записи', icon: 'key', to: `/admin/enrollments?q=${encodeURIComponent(r.studentEmail)}` },
                ]}
              />
            )}
            empty={
              query.data.length === 0 ? (
                <EmptyState
                  icon="users"
                  title="На курс ещё никто не записан"
                  text="Выдать доступ вручную можно в разделе «Записи на курсы»."
                  action={
                    <Link className="btn btn-ghost" to="/admin/enrollments?grant=1">
                      Выдать доступ
                    </Link>
                  }
                />
              ) : (
                <EmptyState icon="search" title="Никого не нашли" text="Измените поиск или фильтр." />
              )
            }
          />
          <Pager page={page} total={rows.length} limit={LIMIT} onPage={setPage} />
        </>
      )}
    </Card>
  );
}

/* ------------------------------------------------------------------ */
/* Промокоды                                                            */
/* ------------------------------------------------------------------ */

function promoState(p: AdminPromoCode): { label: string; tone: Tone } {
  if (!p.active) return { label: 'Отключён', tone: 'neutral' };
  if (p.expiresAt && new Date(p.expiresAt).getTime() < Date.now()) return { label: 'Истёк', tone: 'warn' };
  if (p.maxUses > 0 && p.usedCount >= p.maxUses) return { label: 'Исчерпан', tone: 'warn' };
  return { label: 'Действует', tone: 'ok' };
}

function PromoTab({
  course,
  query,
}: {
  course: AdminCourseDetail['course'];
  query: { data?: { items: AdminPromoCode[] }; isError: boolean; error: unknown; refetch: () => unknown };
}): JSX.Element {
  const qc = useQueryClient();
  const { confirm, toast } = useAdminUi();
  const [creating, setCreating] = useState(false);
  const archived = course.status === 'archived';
  const items = query.data?.items ?? [];

  const disable = useMutation({
    mutationFn: (p: AdminPromoCode) => deactivatePromoCode(course.id, p.id),
    onSuccess: () => {
      toast('Промокод отключён');
      void qc.invalidateQueries({ queryKey: ['admin'] });
    },
    onError: (e) => toast(errorText(e), 'error'),
  });

  async function onDisable(p: AdminPromoCode): Promise<void> {
    const ok = await confirm({
      title: `Отключить промокод ${p.code}?`,
      text: 'Новые покупки по этому коду станут невозможны. Включить код обратно нельзя — при необходимости создайте новый.',
      confirmLabel: 'Отключить',
      danger: true,
    });
    if (ok) disable.mutate(p);
  }

  function copy(code: string): void {
    void navigator.clipboard?.writeText(code).then(
      () => toast(`Код ${code} скопирован`, 'info'),
      () => toast('Не удалось скопировать', 'error'),
    );
  }

  const columns: Column<AdminPromoCode>[] = [
    {
      key: 'code',
      header: 'Код',
      mobile: 'title',
      render: (p) => (
        <span className="acn-code">
          <strong className="ax-mono">{p.code}</strong>
          <small className="ax-muted">создан {fmtDate(p.createdAt)}</small>
        </span>
      ),
    },
    {
      key: 'discount',
      header: 'Скидка',
      width: '150px',
      render: (p) => (
        <span className="ax-num">
          −{p.discountPct}%
          {course.priceCents > 0 && (
            <small className="ax-muted"> · {fmtMoney(Math.round((course.priceCents * (100 - p.discountPct)) / 100))}</small>
          )}
        </span>
      ),
    },
    {
      key: 'uses',
      header: 'Использовано',
      width: '130px',
      render: (p) => (
        <span className="ax-num">
          {p.usedCount} {p.maxUses > 0 ? `из ${p.maxUses}` : <small className="ax-muted">· без лимита</small>}
        </span>
      ),
    },
    { key: 'expires', header: 'Действует до', width: '130px', render: (p) => <span className="ax-num">{p.expiresAt ? fmtDate(p.expiresAt) : 'бессрочно'}</span> },
    {
      key: 'state',
      header: 'Статус',
      width: '120px',
      render: (p) => {
        const s = promoState(p);
        return <Badge tone={s.tone}>{s.label}</Badge>;
      },
    },
  ];

  return (
    <Card
      flush
      title="Промокоды курса"
      description="Скидка по коду при покупке этого курса."
      actions={
        <button type="button" className="btn btn-primary" onClick={() => setCreating(true)} disabled={archived}>
          <Icon name="plus" size={16} /> Создать промокод
        </button>
      }
    >
      {(archived || course.priceCents <= 0) && (
        <div className="acn-pad">
          {archived ? (
            <Note tone="warn">Курс в архиве: промокоды не работают, новые создать нельзя.</Note>
          ) : (
            <Note tone="info">Курс бесплатный — промокоды для него не применяются.</Note>
          )}
        </div>
      )}
      {query.isError ? (
        <ErrorNote error={query.error} onRetry={() => void query.refetch()} />
      ) : !query.data ? (
        <Spinner />
      ) : (
        <DataTable<AdminPromoCode>
          caption="Промокоды курса"
          columns={columns}
          rows={items}
          rowKey={(p) => p.id}
          actions={(p) => (
            <RowMenu
              label={`Действия: ${p.code}`}
              items={[
                { label: 'Скопировать код', icon: 'copy', onClick: () => copy(p.code) },
                ...(p.active
                  ? (['divider', { label: 'Отключить', icon: 'lock', danger: true, onClick: () => void onDisable(p) }] as const)
                  : []),
              ]}
            />
          )}
          empty={
            <EmptyState
              icon="tag"
              title="Промокодов нет"
              text="Создайте код со скидкой — например, для рассылки или партнёров."
              action={
                !archived ? (
                  <button type="button" className="btn btn-ghost" onClick={() => setCreating(true)}>
                    Создать промокод
                  </button>
                ) : undefined
              }
            />
          }
        />
      )}
      <PromoCreateModal
        open={creating}
        courseId={course.id}
        priceCents={course.priceCents}
        existing={items.map((p) => p.code)}
        onClose={() => setCreating(false)}
        onDone={(code) => {
          setCreating(false);
          toast(`Промокод ${code} создан`);
          void qc.invalidateQueries({ queryKey: ['admin'] });
        }}
      />
    </Card>
  );
}

const CODE_RE = /^[A-Z0-9_-]{3,32}$/;

function randomCode(): string {
  const abc = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789';
  let s = '';
  for (let i = 0; i < 8; i += 1) s += abc[Math.floor(Math.random() * abc.length)];
  return s;
}

function todayIso(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

function PromoCreateModal({
  open,
  courseId,
  priceCents,
  existing,
  onClose,
  onDone,
}: {
  open: boolean;
  courseId: string;
  priceCents: number;
  existing: string[];
  onClose: () => void;
  onDone: (code: string) => void;
}): JSX.Element | null {
  const [code, setCode] = useState('');
  const [discount, setDiscount] = useState('10');
  const [maxUses, setMaxUses] = useState('');
  const [expires, setExpires] = useState('');
  const [touched, setTouched] = useState(false);

  const norm = code.trim().toUpperCase();
  const disc = Number(discount);
  const uses = maxUses.trim() === '' ? 0 : Number(maxUses);
  const errors = {
    code: !CODE_RE.test(norm)
      ? 'От 3 до 32 символов: латинские буквы, цифры, дефис или подчёркивание'
      : existing.includes(norm)
        ? 'Такой код уже есть у этого курса'
        : undefined,
    discount: !Number.isInteger(disc) || disc < 1 || disc > 90 ? 'Целое число от 1 до 90' : undefined,
    maxUses: !Number.isInteger(uses) || uses < 0 ? 'Целое число, 0 или больше' : undefined,
    expires: expires && expires < todayIso() ? 'Дата не может быть в прошлом' : undefined,
  };
  const invalid = Object.values(errors).some(Boolean);

  const reset = (): void => {
    setCode('');
    setDiscount('10');
    setMaxUses('');
    setExpires('');
    setTouched(false);
  };

  const m = useMutation({
    mutationFn: () =>
      createPromoCode(courseId, {
        code: norm,
        discountPct: disc,
        maxUses: uses,
        // До конца выбранного дня по местному времени.
        expiresAt: expires ? new Date(`${expires}T23:59:59`).toISOString() : null,
      }),
    onSuccess: (p) => {
      reset();
      onDone(p.code);
    },
  });

  const close = (): void => {
    if (m.isPending) return;
    reset();
    m.reset();
    onClose();
  };
  const submit = (): void => {
    setTouched(true);
    if (!invalid) m.mutate();
  };

  return (
    <Modal
      open={open}
      onClose={close}
      title="Новый промокод"
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={close} disabled={m.isPending}>
            Отмена
          </button>
          <button type="button" className="btn btn-primary" onClick={submit} disabled={m.isPending}>
            {m.isPending ? 'Создаём…' : 'Создать'}
          </button>
        </>
      }
    >
      <form
        className="ax-form"
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <Field label="Код" hint="Ученик вводит его при оплате. Регистр не важен." error={touched ? errors.code : undefined}>
          <span className="acn-code-input">
            <input
              className="ax-input ax-mono"
              value={code}
              maxLength={32}
              autoComplete="off"
              autoCapitalize="characters"
              placeholder="SPRING25"
              onChange={(e) => setCode(e.target.value.toUpperCase().replace(/\s+/g, ''))}
            />
            <button type="button" className="btn btn-ghost" onClick={() => setCode(randomCode())}>
              Придумать
            </button>
          </span>
        </Field>
        <div className="ax-form-row">
          <Field
            label="Скидка, %"
            hint={priceCents > 0 && !errors.discount ? `Цена со скидкой: ${fmtMoney(Math.round((priceCents * (100 - disc)) / 100))}` : 'От 1 до 90'}
            error={touched ? errors.discount : undefined}
          >
            <input className="ax-input" type="number" inputMode="numeric" min={1} max={90} value={discount} onChange={(e) => setDiscount(e.target.value)} />
          </Field>
          <Field label="Лимит использований" hint="Пусто или 0 — без ограничений" error={touched ? errors.maxUses : undefined}>
            <input className="ax-input" type="number" inputMode="numeric" min={0} value={maxUses} placeholder="без лимита" onChange={(e) => setMaxUses(e.target.value)} />
          </Field>
        </div>
        <Field label="Действует до (включительно)" hint="Пусто — бессрочно" error={touched ? errors.expires : undefined}>
          <input className="ax-input" type="date" min={todayIso()} value={expires} onChange={(e) => setExpires(e.target.value)} />
        </Field>
        {m.isError && <ErrorNote error={m.error} />}
      </form>
    </Modal>
  );
}
