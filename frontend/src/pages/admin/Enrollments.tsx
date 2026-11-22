import { useMemo, useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { keepPreviousData, useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query';
import {
  grantEnrollment,
  revokeEnrollment,
  unrevokeEnrollment,
  type AdminEnrollmentRow,
  type AdminEnrollmentStatus,
  type GrantEnrollmentResponse,
} from '../../api/adminEnrollments';
import { adminCompleteCourse, adminResetProgress } from '../../api/adminOps';
import { overviewQueryKey } from '../../api/adminHub';
import type { AdminCourseRow } from '../../api/adminConsole';
import { ApiError } from '../../api/client';
import {
  listAllAdminCourses,
  listSalesEnrollments,
  looksLikeEmail,
  salesCoursesKey,
  useUrlFilters,
  useUrlSearch,
} from '../../api/adminSales';
import {
  AdminPage,
  Avatar,
  Badge,
  Card,
  DataTable,
  Drawer,
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
  Segmented,
  Spinner,
  Toolbar,
  downloadCsv,
  errorText,
  fmtDate,
  fmtMoney,
  useAdminUi,
  type Column,
  type MenuItem,
  type Tone,
} from '../../components/admin/ui';
import '../../styles/admin-sales.css';

/* ------------------------------------------------------------------ */
/* Общие части раздела «Продажи и доступы» (используются и на других    */
/* страницах раздела)                                                    */
/* ------------------------------------------------------------------ */

const COURSE_STATUS_LABEL: Record<string, string> = {
  draft: 'черновик',
  review: 'на проверке',
  published: 'опубликован',
  archived: 'в архиве',
};

/** Ученик: аватар + имя/email, ссылка на карточку пользователя. */
export function StudentCell({ id, name, email }: { id: string; name: string; email: string }): JSX.Element {
  const title = name.trim() || email || 'Без имени';
  return (
    <Link className="asl-person" to={`/admin/users/${id}`}>
      <Avatar name={title} size={34} />
      <span className="ax-person-text">
        <strong>{title}</strong>
        {name.trim() && email && <small>{email}</small>}
      </span>
    </Link>
  );
}

/** Название курса со ссылкой на карточку курса в админке. */
export function CourseCell({ id, title }: { id: string; title: string }): JSX.Element {
  return (
    <Link className="asl-course-link" to={`/admin/courses/${id}`} title={title}>
      {title}
    </Link>
  );
}

/**
 * Выбор курса с поиском по названию (список-кнопки — удобно и на телефоне).
 * filter — какие курсы можно выбрать (например, только платные).
 */
export function CoursePicker({
  courses,
  loading,
  value,
  onChange,
  filter,
  label = 'Курс',
  hint,
  error,
}: {
  courses: AdminCourseRow[];
  loading?: boolean;
  value: string;
  onChange: (id: string) => void;
  filter?: (c: AdminCourseRow) => boolean;
  label?: string;
  hint?: string;
  error?: string;
}): JSX.Element {
  const [q, setQ] = useState('');
  const options = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return courses
      .filter((c) => (filter ? filter(c) : true))
      .filter((c) => !needle || c.title.toLowerCase().includes(needle) || c.teacherEmail.toLowerCase().includes(needle));
  }, [courses, filter, q]);
  const selected = courses.find((c) => c.id === value);
  return (
    <div className="ax-field asl-picker">
      <span className="asl-picker-label">{label}</span>
      <SearchInput value={q} onChange={setQ} placeholder="Найти курс по названию" label="Поиск курса" />
      <div className="asl-picker-list" role="listbox" aria-label={label}>
        {loading && <div className="asl-picker-empty">Загружаем курсы…</div>}
        {!loading && options.length === 0 && <div className="asl-picker-empty">Подходящих курсов нет</div>}
        {options.map((c) => {
          const isSel = c.id === value;
          return (
            <button
              key={c.id}
              type="button"
              role="option"
              aria-selected={isSel}
              className={`asl-picker-option${isSel ? ' is-selected' : ''}`}
              onClick={() => onChange(c.id)}
            >
              <span className="asl-picker-text">
                <strong>{c.title}</strong>
                <small>
                  {c.priceCents > 0 ? fmtMoney(c.priceCents) : 'бесплатно'}
                  {c.status !== 'published' ? ` · ${COURSE_STATUS_LABEL[c.status] ?? c.status}` : ''}
                  {c.teacherEmail ? ` · ${c.teacherEmail}` : ''}
                </small>
              </span>
              {isSel && <Icon name="check" size={18} className="asl-picker-check" />}
            </button>
          );
        })}
      </div>
      {error ? (
        <span className="ax-field-error">{error}</span>
      ) : selected ? (
        <span className="ax-field-hint">Выбран: «{selected.title}»</span>
      ) : (
        hint && <span className="ax-field-hint">{hint}</span>
      )}
    </div>
  );
}

/** Полоска прогресса с подписью «3 из 10 уроков · 30 %». */
function Progress({ done, total, percent }: { done: number; total: number; percent: number }): JSX.Element {
  const p = Math.min(100, Math.max(0, Math.round(Number.isFinite(percent) ? percent : 0)));
  return (
    <div className="asl-progress">
      <div
        className={`asl-progress-bar${p >= 100 ? ' is-done' : ''}`}
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={p}
        aria-label="Прогресс"
      >
        <span style={{ width: `${p}%` }} />
      </div>
      <span className="asl-progress-meta">
        {total > 0 ? `${done} из ${total} уроков · ${p} %` : 'в курсе нет уроков'}
      </span>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* Страница «Доступы к курсам»                                          */
/* ------------------------------------------------------------------ */

const LIMIT = 20;
const LIST_KEY = ['admin-enrollments'] as const;

const STATUS: Record<AdminEnrollmentStatus, { label: string; tone: Tone }> = {
  active: { label: 'Открыт', tone: 'ok' },
  refunded: { label: 'Возврат', tone: 'neutral' },
  revoked: { label: 'Закрыт', tone: 'danger' },
};

type StatusFilter = '' | AdminEnrollmentStatus;
const STATUS_OPTIONS: { value: StatusFilter; label: string }[] = [
  { value: '', label: 'Все' },
  { value: 'active', label: 'Открыт' },
  { value: 'revoked', label: 'Закрыт' },
  { value: 'refunded', label: 'Возврат' },
];

function isStatus(v: string): v is AdminEnrollmentStatus {
  return v === 'active' || v === 'refunded' || v === 'revoked';
}

function invalidateAll(qc: QueryClient): void {
  void qc.invalidateQueries({ queryKey: LIST_KEY });
  void qc.invalidateQueries({ queryKey: overviewQueryKey });
  void qc.invalidateQueries({ queryKey: ['admin', 'stats'] });
  void qc.invalidateQueries({ queryKey: ['admin-certificates'] });
}

/** Модалка «Выдать доступ»: email + курс, понятный итог. */
function GrantModal({
  open,
  onClose,
  courses,
  coursesLoading,
}: {
  open: boolean;
  onClose: () => void;
  courses: AdminCourseRow[];
  coursesLoading: boolean;
}): JSX.Element {
  const qc = useQueryClient();
  const { toast } = useAdminUi();
  const [email, setEmail] = useState('');
  const [courseId, setCourseId] = useState('');
  const [touched, setTouched] = useState(false);
  const [result, setResult] = useState<GrantEnrollmentResponse | null>(null);

  const grant = useMutation({
    mutationFn: () => grantEnrollment(email, courseId),
    onSuccess: (res) => {
      setResult(res);
      invalidateAll(qc);
      toast(res.wasActive ? 'Доступ уже был — ничего не изменилось' : 'Доступ выдан', res.wasActive ? 'info' : 'ok');
    },
  });

  const emailError = touched && !looksLikeEmail(email) ? 'Введите email зарегистрированного пользователя' : undefined;
  const courseError = touched && !courseId ? 'Выберите курс' : undefined;
  const notFound = grant.error instanceof ApiError && grant.error.status === 404 && /пользовател/i.test(grant.error.message);

  function reset(): void {
    setEmail('');
    setCourseId('');
    setTouched(false);
    setResult(null);
    grant.reset();
  }

  function close(): void {
    reset();
    onClose();
  }

  function submit(e?: FormEvent): void {
    e?.preventDefault();
    setTouched(true);
    if (!looksLikeEmail(email) || !courseId) return;
    grant.mutate();
  }

  return (
    <Modal
      open={open}
      onClose={close}
      title="Выдать доступ к курсу"
      footer={
        result ? (
          <>
            <button type="button" className="btn btn-ghost" onClick={reset}>
              Выдать ещё
            </button>
            <button type="button" className="btn btn-primary" onClick={close}>
              Готово
            </button>
          </>
        ) : (
          <>
            <button type="button" className="btn btn-ghost" onClick={close}>
              Отмена
            </button>
            <button type="submit" form="asl-grant-form" className="btn btn-primary" disabled={grant.isPending}>
              {grant.isPending ? 'Выдаём…' : 'Выдать доступ'}
            </button>
          </>
        )
      }
    >
      {result ? (
        <div className="asl-result">
          <Note tone={result.wasActive ? 'info' : 'ok'}>
            {result.wasActive ? (
              <>
                У <b>{result.email}</b> уже есть доступ к курсу «{result.courseTitle}». Ничего не изменилось.
              </>
            ) : (
              <>
                Готово: <b>{result.email}</b> {result.created ? 'получил' : 'снова получил'} доступ к курсу «{result.courseTitle}».
              </>
            )}
          </Note>
          <div className="asl-actions-row">
            <Link className="btn btn-ghost btn-sm" to={`/admin/users/${result.studentId}`} onClick={close}>
              <Icon name="user" size={16} /> Карточка ученика
            </Link>
            <Link className="btn btn-ghost btn-sm" to={`/admin/courses/${result.courseId}`} onClick={close}>
              <Icon name="book" size={16} /> Карточка курса
            </Link>
          </div>
        </div>
      ) : (
        <form id="asl-grant-form" className="ax-form" onSubmit={submit} noValidate>
          <Note>
            Откроет курс без оплаты — в том числе платный. В журнал платежей и выручку такая запись не попадает.
            Закрытый ранее доступ будет восстановлен.
          </Note>
          <Field label="Email ученика" hint="Пользователь должен быть зарегистрирован" error={emailError}>
            <input
              className="ax-input"
              type="email"
              inputMode="email"
              autoComplete="off"
              placeholder="user@example.com"
              value={email}
              onChange={(e) => {
                setEmail(e.target.value);
                grant.reset();
              }}
            />
          </Field>
          <CoursePicker
            courses={courses}
            loading={coursesLoading}
            value={courseId}
            onChange={(id) => {
              setCourseId(id);
              grant.reset();
            }}
            filter={(c) => c.status !== 'draft'}
            hint="Черновики не показываются — к ним доступ выдать нельзя"
            error={courseError}
          />
          {grant.isError &&
            (notFound ? (
              <Note tone="warn">
                Пользователь с email <b>{email.trim()}</b> не найден. Попросите его зарегистрироваться или{' '}
                <Link className="ax-link" to="/admin/users?new=1" onClick={close}>
                  создайте пользователя
                </Link>
                .
              </Note>
            ) : (
              <ErrorNote error={grant.error} />
            ))}
        </form>
      )}
    </Modal>
  );
}

export default function Enrollments(): JSX.Element {
  const qc = useQueryClient();
  const { confirm, toast } = useAdminUi();
  const { get, set } = useUrlFilters();

  const q = get('q');
  const courseId = get('courseId');
  const studentId = get('studentId');
  const statusParam = get('status');
  const status: StatusFilter = isStatus(statusParam) ? statusParam : '';
  const page = Math.max(1, Number(get('page')) || 1);
  const grantOpen = get('grant') === '1';
  const [search, setSearch] = useUrlSearch('q', q, set);
  const [details, setDetails] = useState<AdminEnrollmentRow | null>(null);
  const [exporting, setExporting] = useState(false);

  const courses = useQuery({ queryKey: salesCoursesKey, queryFn: listAllAdminCourses, staleTime: 60_000 });

  const list = useQuery({
    queryKey: [...LIST_KEY, { q, courseId, studentId, status, page }],
    queryFn: () =>
      listSalesEnrollments({ q, courseId, studentId, status, page, limit: LIMIT }),
    placeholderData: keepPreviousData,
  });

  const courseOptions = useMemo(() => {
    const opts = [{ value: '', label: 'Все курсы' }, ...(courses.data ?? []).map((c) => ({ value: c.id, label: c.title }))];
    if (courseId && !opts.some((o) => o.value === courseId)) opts.push({ value: courseId, label: 'Выбранный курс' });
    return opts;
  }, [courses.data, courseId]);

  const studentLabel = useMemo(() => {
    if (!studentId) return '';
    const row = list.data?.items.find((r) => r.studentId === studentId);
    return row ? row.studentName.trim() || row.studentEmail : 'выбранный ученик';
  }, [studentId, list.data]);

  const pair = (row: AdminEnrollmentRow): string => `${row.studentName.trim() || row.studentEmail} — «${row.courseTitle}»`;

  const syncDetails = (row: AdminEnrollmentRow, patch: Partial<AdminEnrollmentRow>): void => {
    setDetails((d) => (d && d.id === row.id ? { ...d, ...patch } : d));
  };

  const revoke = useMutation({
    mutationFn: (row: AdminEnrollmentRow) => revokeEnrollment(row.studentId, row.courseId),
    onSuccess: (_r, row) => {
      toast('Доступ закрыт');
      syncDetails(row, { status: 'revoked' });
      invalidateAll(qc);
    },
    onError: (e) => toast(errorText(e), 'error'),
  });
  const unrevoke = useMutation({
    mutationFn: (row: AdminEnrollmentRow) => unrevokeEnrollment(row.studentId, row.courseId),
    onSuccess: (_r, row) => {
      toast('Доступ возвращён');
      syncDetails(row, { status: 'active' });
      invalidateAll(qc);
    },
    onError: (e) => toast(errorText(e), 'error'),
  });
  const resetProgress = useMutation({
    mutationFn: (row: AdminEnrollmentRow) => adminResetProgress(row.studentId, row.courseId),
    onSuccess: (r, row) => {
      toast(r.deleted > 0 ? `Прогресс сброшен (отметок удалено: ${r.deleted})` : 'Прогресс уже был нулевым', 'ok');
      syncDetails(row, { completedLessons: 0, percent: 0 });
      invalidateAll(qc);
    },
    onError: (e) => toast(errorText(e), 'error'),
  });
  const complete = useMutation({
    mutationFn: (row: AdminEnrollmentRow) => adminCompleteCourse(row.studentId, row.courseId),
    onSuccess: (r, row) => {
      toast(`Курс засчитан, сертификат № ${r.certificateNumber}`);
      syncDetails(row, { completedLessons: row.totalLessons, percent: 100, status: 'active' });
      invalidateAll(qc);
    },
    onError: (e) => toast(errorText(e), 'error'),
  });

  async function onRevoke(row: AdminEnrollmentRow): Promise<void> {
    const ok = await confirm({
      title: 'Закрыть доступ к курсу?',
      text: (
        <>
          <p>{pair(row)}</p>
          <p>Ученик перестанет видеть уроки. Прогресс сохранится — доступ можно вернуть в любой момент. Деньги при этом не возвращаются.</p>
        </>
      ),
      confirmLabel: 'Закрыть доступ',
      danger: true,
    });
    if (ok) revoke.mutate(row);
  }
  async function onUnrevoke(row: AdminEnrollmentRow): Promise<void> {
    const ok = await confirm({
      title: 'Вернуть доступ к курсу?',
      text: <p>{pair(row)}</p>,
      confirmLabel: 'Вернуть доступ',
    });
    if (ok) unrevoke.mutate(row);
  }
  async function onReset(row: AdminEnrollmentRow): Promise<void> {
    const ok = await confirm({
      title: 'Сбросить прогресс?',
      text: (
        <>
          <p>{pair(row)}</p>
          <p>Все отметки о пройденных уроках будут удалены, ученик начнёт курс заново. Отменить это нельзя.</p>
        </>
      ),
      confirmLabel: 'Сбросить прогресс',
      danger: true,
    });
    if (ok) resetProgress.mutate(row);
  }
  async function onComplete(row: AdminEnrollmentRow): Promise<void> {
    const ok = await confirm({
      title: 'Засчитать курс и выдать сертификат?',
      text: (
        <>
          <p>{pair(row)}</p>
          <p>Все уроки будут отмечены пройденными, ученик получит сертификат. Если доступ был закрыт — он откроется.</p>
        </>
      ),
      confirmLabel: 'Засчитать курс',
    });
    if (ok) complete.mutate(row);
  }

  function menuItems(row: AdminEnrollmentRow): MenuItem[] {
    const items: MenuItem[] = [
      { label: 'Открыть ученика', icon: 'user', to: `/admin/users/${row.studentId}` },
      { label: 'Открыть курс', icon: 'book', to: `/admin/courses/${row.courseId}` },
      { label: 'Платежи ученика', icon: 'card', to: `/admin/payments?q=${encodeURIComponent(row.studentEmail)}` },
      'divider',
      { label: 'Засчитать курс и выдать сертификат', icon: 'award', onClick: () => void onComplete(row), disabled: row.totalLessons === 0 },
      { label: 'Сбросить прогресс', icon: 'refresh', onClick: () => void onReset(row), disabled: row.completedLessons === 0 },
    ];
    if (row.status === 'active') items.push('divider', { label: 'Закрыть доступ', icon: 'lock', danger: true, onClick: () => void onRevoke(row) });
    if (row.status === 'revoked') items.push('divider', { label: 'Вернуть доступ', icon: 'unlock', onClick: () => void onUnrevoke(row) });
    return items;
  }

  async function onExport(): Promise<void> {
    setExporting(true);
    try {
      const rows: AdminEnrollmentRow[] = [];
      let total = 0;
      for (let p = 1; p <= 50; p += 1) {
        const res = await listSalesEnrollments({ q, courseId, studentId, status, page: p, limit: 100 });
        if (p === 1) total = res.total;
        rows.push(...res.items);
        if (res.items.length < 100 || rows.length >= res.total) break;
      }
      if (rows.length === 0) {
        toast('Нечего выгружать — список пуст', 'info');
        return;
      }
      downloadCsv(
        `enrollments-${new Date().toISOString().slice(0, 10)}.csv`,
        ['ID записи', 'Email', 'Имя', 'Курс', 'ID курса', 'Доступ', 'Пройдено уроков', 'Всего уроков', 'Прогресс, %', 'Дата записи'],
        rows.map((r) => [
          r.id,
          r.studentEmail,
          r.studentName,
          r.courseTitle,
          r.courseId,
          STATUS[r.status]?.label ?? r.status,
          r.completedLessons,
          r.totalLessons,
          r.percent,
          fmtDate(r.createdAt),
        ]),
      );
      toast(total > rows.length ? `Выгружены первые ${rows.length} из ${total}` : `Выгружено строк: ${rows.length}`, total > rows.length ? 'info' : 'ok');
    } catch (e) {
      toast(errorText(e), 'error');
    } finally {
      setExporting(false);
    }
  }

  const columns: Column<AdminEnrollmentRow>[] = [
    {
      key: 'student',
      header: 'Ученик',
      mobile: 'title',
      render: (r) => <StudentCell id={r.studentId} name={r.studentName} email={r.studentEmail} />,
    },
    { key: 'course', header: 'Курс', render: (r) => <CourseCell id={r.courseId} title={r.courseTitle} /> },
    {
      key: 'progress',
      header: 'Прогресс',
     
      render: (r) => <Progress done={r.completedLessons} total={r.totalLessons} percent={r.percent} />,
    },
    {
      key: 'status',
      header: 'Доступ',
     
      render: (r) => <Badge tone={STATUS[r.status]?.tone ?? 'neutral'}>{STATUS[r.status]?.label ?? r.status}</Badge>,
    },
    { key: 'date', header: 'Записан', render: (r) => <span className="asl-nowrap">{fmtDate(r.createdAt)}</span> },
  ];

  const filtersActive = Boolean(q || courseId || status || studentId);
  const data = list.data;

  return (
    <AdminPage
      title="Доступы к курсам"
      description="Кто на какие курсы записан, прогресс учеников. Здесь можно выдать или закрыть доступ, сбросить прогресс или засчитать курс."
      actions={
        <button type="button" className="btn btn-primary" onClick={() => set({ grant: '1' }, { replace: true })}>
          <Icon name="key" size={17} /> Выдать доступ
        </button>
      }
    >
      <Card flush className="asl-card">
        <div className="asl-toolbar-stack">
          <Toolbar
            end={
              <>
                {filtersActive && (
                  <button
                    type="button"
                    className="btn btn-ghost"
                    onClick={() => {
                      setSearch('');
                      set({ q: null, courseId: null, status: null, studentId: null });
                    }}
                  >
                    Сбросить
                  </button>
                )}
                <button type="button" className="btn btn-ghost" onClick={() => void onExport()} disabled={exporting}>
                  <Icon name="download" size={16} /> {exporting ? 'Готовим…' : 'CSV'}
                </button>
              </>
            }
          >
            <SearchInput value={search} onChange={setSearch} placeholder="Email или имя ученика" />
            <FilterSelect label="Курс" value={courseId} onChange={(v) => set({ courseId: v || null })} options={courseOptions} />
          </Toolbar>
          <Segmented<StatusFilter>
            label="Статус доступа"
            value={status}
            onChange={(v) => set({ status: v || null })}
            options={STATUS_OPTIONS}
          />
          {studentId && (
            <div className="asl-chips">
              <span className="asl-chip">
                <span>Ученик: {studentLabel}</span>
                <button type="button" aria-label="Показать всех учеников" onClick={() => set({ studentId: null })}>
                  <Icon name="x" size={14} />
                </button>
              </span>
            </div>
          )}
        </div>

        {list.isLoading ? (
          <Spinner />
        ) : list.isError ? (
          <ErrorNote error={list.error} onRetry={() => void list.refetch()} />
        ) : (
          <>
            <DataTable
              columns={columns}
              rows={data?.items ?? []}
              rowKey={(r) => r.id}
              onRowClick={setDetails}
              actions={(r) => <RowMenu items={menuItems(r)} />}
              caption="Записи на курсы"
              empty={
                filtersActive ? (
                  <EmptyState icon="search" title="Ничего не найдено" text="Попробуйте изменить поиск или фильтры." />
                ) : (
                  <EmptyState
                    icon="key"
                    title="Записей пока нет"
                    text="Когда ученики запишутся на курсы, они появятся здесь. Доступ можно выдать и вручную."
                    action={
                      <button type="button" className="btn btn-primary" onClick={() => set({ grant: '1' }, { replace: true })}>
                        Выдать доступ
                      </button>
                    }
                  />
                )
              }
            />
            {data && <Pager page={page} total={data.total} limit={LIMIT} onPage={(p) => set({ page: String(p) })} />}
          </>
        )}
      </Card>

      <GrantModal
        open={grantOpen}
        onClose={() => set({ grant: null }, { replace: true })}
        courses={courses.data ?? []}
        coursesLoading={courses.isLoading}
      />

      <Drawer
        open={details !== null}
        onClose={() => setDetails(null)}
        title={details?.courseTitle ?? ''}
        subtitle={details ? details.studentName.trim() || details.studentEmail : undefined}
        footer={
          details && details.status !== 'refunded' && (
            <div className="asl-foot">
              {details.status === 'active' && (
                <button type="button" className="btn ax-btn-danger" onClick={() => void onRevoke(details)}>
                  Закрыть доступ
                </button>
              )}
              {details.status === 'revoked' && (
                <button type="button" className="btn btn-primary" onClick={() => void onUnrevoke(details)}>
                  Вернуть доступ
                </button>
              )}
            </div>
          )
        }
      >
        {details && (
          <>
            <section className="asl-drawer-section">
              <InfoList
                items={[
                  { label: 'Ученик', value: <StudentCell id={details.studentId} name={details.studentName} email={details.studentEmail} /> },
                  { label: 'Курс', value: <CourseCell id={details.courseId} title={details.courseTitle} /> },
                  { label: 'Доступ', value: <Badge tone={STATUS[details.status]?.tone ?? 'neutral'}>{STATUS[details.status]?.label ?? details.status}</Badge> },
                  { label: 'Записан', value: fmtDate(details.createdAt) },
                  { label: 'Прогресс', value: <Progress done={details.completedLessons} total={details.totalLessons} percent={details.percent} /> },
                ]}
              />
              {details.status === 'refunded' && (
                <div style={{ marginTop: 14 }}>
                  <Note tone="warn">Доступ закрыт, потому что за курс оформлен возврат денег. Чтобы открыть курс снова, выдайте доступ вручную.</Note>
                </div>
              )}
            </section>
            <section className="asl-drawer-section">
              <h3 className="asl-section-title">Прогресс</h3>
              <div className="asl-actions-row">
                <button type="button" className="btn btn-ghost" disabled={details.totalLessons === 0 || complete.isPending} onClick={() => void onComplete(details)}>
                  <Icon name="award" size={16} /> Засчитать курс и выдать сертификат
                </button>
                <button type="button" className="btn btn-ghost" disabled={details.completedLessons === 0 || resetProgress.isPending} onClick={() => void onReset(details)}>
                  <Icon name="refresh" size={16} /> Сбросить прогресс
                </button>
              </div>
            </section>
            <section className="asl-drawer-section">
              <h3 className="asl-section-title">Перейти</h3>
              <div className="asl-actions-row">
                <Link className="btn btn-ghost btn-sm" to={`/admin/users/${details.studentId}`}>
                  <Icon name="user" size={16} /> Карточка ученика
                </Link>
                <Link className="btn btn-ghost btn-sm" to={`/admin/courses/${details.courseId}`}>
                  <Icon name="book" size={16} /> Карточка курса
                </Link>
                <Link className="btn btn-ghost btn-sm" to={`/admin/payments?q=${encodeURIComponent(details.studentEmail)}`}>
                  <Icon name="card" size={16} /> Платежи ученика
                </Link>
                <Link className="btn btn-ghost btn-sm" to={`/admin/enrollments?studentId=${details.studentId}`} onClick={() => setDetails(null)}>
                  <Icon name="key" size={16} /> Все курсы ученика
                </Link>
              </div>
            </section>
          </>
        )}
      </Drawer>
    </AdminPage>
  );
}
