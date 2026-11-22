import { useState, type ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  archiveAdminCourse,
  getAdminStats,
  listAdminCourses,
  unarchiveAdminCourse,
  unpublishAdminCourse,
  type AdminCourseDir,
  type AdminCourseSort,
} from '../../api/adminConsole';
import { adminPublishCourse, adminSetCourseOwner } from '../../api/adminOps';
import { duplicateCourse } from '../../api/teacherExtra';
import { formatPrice, moderateCourse } from '../../api/client';
import type { CourseStatus } from '../../api/types';
import {
  COURSE_STATUS,
  contentKeys,
  pageOf,
  useDebouncedInput,
  useFilterState,
} from '../../api/adminContent';
import {
  AdminPage,
  Badge,
  Card,
  DataTable,
  EmptyState,
  ErrorNote,
  Field,
  FilterSelect,
  Icon,
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
  useAdminUi,
  type Column,
  type MenuItem,
} from '../../components/admin/ui';
import '../../styles/admin-content.css';

/* ------------------------------------------------------------------ */
/* Общие элементы раздела «Контент» (используются и на других страницах) */
/* ------------------------------------------------------------------ */

/** Обложка курса; без обложки — цветная плашка с первой буквой названия. */
export function CourseThumb({
  title,
  coverPath,
  size = 44,
  wide,
}: {
  title: string;
  coverPath?: string | null;
  size?: number;
  /** Широкая обложка 16:9 во всю ширину контейнера (карточки модерации). */
  wide?: boolean;
}): JSX.Element {
  const [broken, setBroken] = useState(false);
  let h = 0;
  for (const ch of title) h = (h * 31 + ch.charCodeAt(0)) % 360;
  const letter = (title.trim()[0] ?? '?').toUpperCase();
  const style = wide ? undefined : { width: size, height: size, fontSize: size * 0.42 };
  const cls = `acn-thumb${wide ? ' acn-thumb--wide' : ''}`;
  if (coverPath && !broken) {
    return (
      <span className={cls} style={style}>
        <img src={coverPath} alt="" loading="lazy" onError={() => setBroken(true)} />
      </span>
    );
  }
  return (
    <span
      className={`${cls} acn-thumb--letter`}
      style={{ ...style, background: `linear-gradient(135deg, hsl(${h} 75% 90%), hsl(${(h + 40) % 360} 70% 84%))`, color: `hsl(${h} 45% 30%)` }}
      aria-hidden="true"
    >
      {letter}
    </span>
  );
}

export function CourseStatusBadge({ status }: { status: CourseStatus }): JSX.Element {
  const s = COURSE_STATUS[status] ?? { label: status, tone: 'neutral' as const };
  return <Badge tone={s.tone}>{s.label}</Badge>;
}

/** Минимум данных о курсе для действий администратора. */
type OpsCourse = {
  id: string;
  slug: string;
  title: string;
  status: CourseStatus;
  teacherEmail?: string;
};

type Busy = { id: string; action: string } | null;

/**
 * Все действия администратора над курсом: модерация, публикация, архив,
 * передача владельца, копия. Возвращает пункты меню (⋮) и окна, которые
 * нужно отрисовать один раз на странице.
 */
export function useCourseOps(opts?: { onDuplicated?: (id: string) => void }): {
  menu: (c: OpsCourse, o?: { withOpen?: boolean; withEdit?: boolean; withSite?: boolean }) => MenuItem[];
  approve: (c: OpsCourse) => void;
  reject: (c: OpsCourse) => void;
  busy: Busy;
  dialogs: ReactNode;
} {
  const qc = useQueryClient();
  const { confirm, toast } = useAdminUi();
  const [busy, setBusy] = useState<Busy>(null);
  const [rejecting, setRejecting] = useState<OpsCourse | null>(null);
  const [approving, setApproving] = useState<OpsCourse | null>(null);
  const [transfer, setTransfer] = useState<OpsCourse | null>(null);

  const refresh = (): void => {
    // Все запросы админки: списки, детали, очередь, счётчики меню и сводку.
    void qc.invalidateQueries({ queryKey: ['admin'] });
  };

  async function run(c: OpsCourse, action: string, fn: () => Promise<unknown>, done: string): Promise<boolean> {
    setBusy({ id: c.id, action });
    try {
      await fn();
      toast(done);
      refresh();
      return true;
    } catch (e) {
      toast(errorText(e), 'error');
      return false;
    } finally {
      setBusy(null);
    }
  }

  async function publish(c: OpsCourse): Promise<void> {
    const ok = await confirm({
      title: 'Опубликовать курс сразу?',
      text: (
        <>
          «{c.title}» появится в каталоге без проверки модератором. Курс без уроков опубликовать нельзя.
        </>
      ),
      confirmLabel: 'Опубликовать',
    });
    if (ok) await run(c, 'publish', () => adminPublishCourse(c.id), 'Курс опубликован');
  }

  async function unpublish(c: OpsCourse): Promise<void> {
    const ok = await confirm({
      title: 'Снять курс с публикации?',
      text: <>«{c.title}» пропадёт из каталога и вернётся в черновики. Уже записанные ученики сохранят доступ.</>,
      confirmLabel: 'Снять с публикации',
      danger: true,
    });
    if (ok) await run(c, 'unpublish', () => unpublishAdminCourse(c.id), 'Курс снят с публикации');
  }

  async function archive(c: OpsCourse): Promise<void> {
    const ok = await confirm({
      title: 'Отправить курс в архив?',
      text: <>«{c.title}» скроется из каталога, промокоды курса перестанут работать. Вернуть из архива можно в любой момент.</>,
      confirmLabel: 'В архив',
      danger: true,
    });
    if (ok) await run(c, 'archive', () => archiveAdminCourse(c.id), 'Курс перемещён в архив');
  }

  async function unarchive(c: OpsCourse): Promise<void> {
    const ok = await confirm({
      title: 'Вернуть курс из архива?',
      text: <>«{c.title}» станет черновиком. Чтобы он снова появился в каталоге, его нужно опубликовать.</>,
      confirmLabel: 'Вернуть',
    });
    if (ok) await run(c, 'unarchive', () => unarchiveAdminCourse(c.id), 'Курс возвращён из архива');
  }

  async function duplicate(c: OpsCourse): Promise<void> {
    const ok = await confirm({
      title: 'Сделать копию курса?',
      text: <>Будет создан черновик с той же программой (модули и уроки). Ученики, отзывы и платежи не копируются. Владельцем копии станете вы.</>,
      confirmLabel: 'Сделать копию',
    });
    if (!ok) return;
    setBusy({ id: c.id, action: 'duplicate' });
    try {
      const res = await duplicateCourse(c.id);
      toast('Копия создана');
      refresh();
      opts?.onDuplicated?.(res.course.id);
    } catch (e) {
      toast(errorText(e), 'error');
    } finally {
      setBusy(null);
    }
  }

  const menu = (c: OpsCourse, o: { withOpen?: boolean; withEdit?: boolean; withSite?: boolean } = {}): MenuItem[] => {
    const { withOpen = true, withEdit = true, withSite = true } = o;
    const disabled = busy !== null;
    const items: MenuItem[] = [];
    if (withOpen) items.push({ label: 'Открыть карточку', icon: 'book', to: `/admin/courses/${c.id}` });
    if (withEdit) items.push({ label: 'Редактировать в конструкторе', icon: 'edit', to: `/teach/${c.id}` });
    if (withSite) items.push({ label: 'Открыть на сайте', icon: 'external', href: `/courses/${encodeURIComponent(c.slug)}` });
    items.push({ label: 'Ученики (кабинет автора)', icon: 'users', to: `/teach/${c.id}/students` });
    items.push('divider');
    if (c.status === 'review') {
      items.push({ label: 'Одобрить и опубликовать', icon: 'check', onClick: () => setApproving(c), disabled });
      items.push({ label: 'Отклонить…', icon: 'x', onClick: () => setRejecting(c), disabled });
    }
    if (c.status === 'draft' || c.status === 'archived') {
      items.push({ label: 'Опубликовать сразу', icon: 'globe', onClick: () => void publish(c), disabled });
    }
    if (c.status === 'published') {
      items.push({ label: 'Снять с публикации', icon: 'eye', onClick: () => void unpublish(c), disabled });
    }
    if (c.status === 'archived') {
      items.push({ label: 'Вернуть из архива', icon: 'refresh', onClick: () => void unarchive(c), disabled });
    }
    items.push({ label: 'Передать преподавателю…', icon: 'user', onClick: () => setTransfer(c), disabled });
    items.push({ label: 'Сделать копию', icon: 'copy', onClick: () => void duplicate(c), disabled });
    if (c.status !== 'archived') {
      items.push('divider');
      items.push({ label: 'В архив', icon: 'inbox', onClick: () => void archive(c), danger: true, disabled });
    }
    return items;
  };

  const dialogs = (
    <>
      <ModerateModal
        course={approving}
        decision="approve"
        onClose={() => setApproving(null)}
        onDone={() => {
          setApproving(null);
          toast('Курс одобрен и опубликован');
          refresh();
        }}
      />
      <ModerateModal
        course={rejecting}
        decision="reject"
        onClose={() => setRejecting(null)}
        onDone={() => {
          setRejecting(null);
          toast('Курс отклонён и возвращён автору');
          refresh();
        }}
      />
      <TransferModal
        course={transfer}
        onClose={() => setTransfer(null)}
        onDone={(email) => {
          setTransfer(null);
          toast(`Курс передан: ${email}`);
          refresh();
        }}
      />
    </>
  );

  return { menu, approve: setApproving, reject: setRejecting, busy, dialogs };
}

/** Решение модерации: одобрить (комментарий по желанию) или отклонить (комментарий обязателен). */
function ModerateModal({
  course,
  decision,
  onClose,
  onDone,
}: {
  course: OpsCourse | null;
  decision: 'approve' | 'reject';
  onClose: () => void;
  onDone: () => void;
}): JSX.Element | null {
  const [comment, setComment] = useState('');
  const [touched, setTouched] = useState(false);
  const isReject = decision === 'reject';
  const m = useMutation({
    mutationFn: () => moderateCourse(course!.id, { decision, comment: comment.trim() || undefined }),
    onSuccess: () => {
      setComment('');
      setTouched(false);
      onDone();
    },
  });
  const missing = isReject && comment.trim() === '';
  const close = (): void => {
    if (m.isPending) return;
    setComment('');
    setTouched(false);
    m.reset();
    onClose();
  };
  return (
    <Modal
      open={course !== null}
      onClose={close}
      title={isReject ? 'Отклонить курс' : 'Одобрить и опубликовать'}
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={close} disabled={m.isPending}>
            Отмена
          </button>
          <button
            type="button"
            className={isReject ? 'btn ax-btn-danger' : 'btn btn-primary'}
            disabled={m.isPending}
            onClick={() => {
              setTouched(true);
              if (!missing) m.mutate();
            }}
          >
            {m.isPending ? 'Сохраняем…' : isReject ? 'Отклонить' : 'Одобрить и опубликовать'}
          </button>
        </>
      }
    >
      <form
        className="ax-form"
        onSubmit={(e) => {
          e.preventDefault();
          setTouched(true);
          if (!missing) m.mutate();
        }}
      >
        <p className="acn-modal-course">
          <strong>«{course?.title}»</strong>
          {course?.teacherEmail && <span className="ax-muted"> · {course.teacherEmail}</span>}
        </p>
        <p className="ax-muted acn-modal-text">
          {isReject
            ? 'Курс вернётся автору в черновики. Напишите, что нужно исправить, — автор увидит этот комментарий в конструкторе.'
            : 'Курс сразу появится в каталоге. Можно оставить автору короткий комментарий.'}
        </p>
        <Field
          label={isReject ? 'Комментарий для автора *' : 'Комментарий для автора (необязательно)'}
          hint={isReject ? 'Например: «Добавьте описание к модулю 2 и проверьте ответы в тесте».' : undefined}
          error={touched && missing ? 'Без комментария отклонить нельзя — автору нужно понять, что исправить.' : undefined}
        >
          <textarea
            className="ax-textarea"
            rows={4}
            value={comment}
            maxLength={2000}
            onChange={(e) => setComment(e.target.value)}
            aria-invalid={touched && missing ? true : undefined}
          />
        </Field>
        {m.isError && <ErrorNote error={m.error} />}
      </form>
    </Modal>
  );
}

const EMAIL_RE = /^[^\s@]+@[^\s@]+$/;

/** Передача курса другому преподавателю по email. */
function TransferModal({
  course,
  onClose,
  onDone,
}: {
  course: OpsCourse | null;
  onClose: () => void;
  onDone: (email: string) => void;
}): JSX.Element | null {
  const [email, setEmail] = useState('');
  const [touched, setTouched] = useState(false);
  const m = useMutation({
    mutationFn: () => adminSetCourseOwner(course!.id, email.trim()),
    onSuccess: (r) => {
      setEmail('');
      setTouched(false);
      onDone(r.teacherEmail);
    },
  });
  const invalid = !EMAIL_RE.test(email.trim());
  const same = course?.teacherEmail && email.trim().toLowerCase() === course.teacherEmail.toLowerCase();
  const close = (): void => {
    if (m.isPending) return;
    setEmail('');
    setTouched(false);
    m.reset();
    onClose();
  };
  const submit = (): void => {
    setTouched(true);
    if (!invalid && !same) m.mutate();
  };
  return (
    <Modal
      open={course !== null}
      onClose={close}
      title="Передать курс"
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={close} disabled={m.isPending}>
            Отмена
          </button>
          <button type="button" className="btn btn-primary" onClick={submit} disabled={m.isPending}>
            {m.isPending ? 'Передаём…' : 'Передать'}
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
        <p className="acn-modal-course">
          <strong>«{course?.title}»</strong>
          {course?.teacherEmail && <span className="ax-muted"> · сейчас: {course.teacherEmail}</span>}
        </p>
        <Field
          label="Email нового владельца"
          hint="Владельцем может быть только преподаватель или администратор. Ученики, отзывы и выплаты останутся у курса."
          error={
            touched && invalid ? 'Введите корректный email' : touched && same ? 'Курс уже принадлежит этому пользователю' : undefined
          }
        >
          <input
            className="ax-input"
            type="email"
            inputMode="email"
            autoComplete="off"
            placeholder="teacher@example.ru"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </Field>
        {m.isError && <ErrorNote error={m.error} />}
      </form>
    </Modal>
  );
}

/* ------------------------------------------------------------------ */
/* Страница «Курсы»                                                     */
/* ------------------------------------------------------------------ */

type StatusFilter = '' | CourseStatus;

const SORTS: { value: string; label: string; sort: AdminCourseSort; dir: AdminCourseDir }[] = [
  { value: 'createdAt-desc', label: 'Сначала новые', sort: 'createdAt', dir: 'desc' },
  { value: 'createdAt-asc', label: 'Сначала старые', sort: 'createdAt', dir: 'asc' },
  { value: 'updatedAt-desc', label: 'Недавно изменённые', sort: 'updatedAt', dir: 'desc' },
  { value: 'title-asc', label: 'По названию, А–Я', sort: 'title', dir: 'asc' },
  { value: 'title-desc', label: 'По названию, Я–А', sort: 'title', dir: 'desc' },
  { value: 'students-desc', label: 'Больше учеников', sort: 'students', dir: 'desc' },
  { value: 'students-asc', label: 'Меньше учеников', sort: 'students', dir: 'asc' },
  { value: 'price-desc', label: 'Сначала дороже', sort: 'price', dir: 'desc' },
  { value: 'price-asc', label: 'Сначала дешевле', sort: 'price', dir: 'asc' },
];

const STATUSES: CourseStatus[] = ['draft', 'review', 'published', 'archived'];
const STATUS_TABS: Record<CourseStatus, string> = {
  review: 'На проверке',
  published: 'Опубликованы',
  draft: 'Черновики',
  archived: 'Архив',
};

function isIsoDate(v: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(v)) return false;
  const d = new Date(`${v}T00:00:00Z`);
  return !Number.isNaN(d.getTime()) && d.toISOString().slice(0, 10) === v;
}

export default function Courses(): JSX.Element {
  const navigate = useNavigate();
  const f = useFilterState('url');
  const { toast } = useAdminUi();
  const ops = useCourseOps({ onDuplicated: (id) => navigate(`/admin/courses/${id}`) });

  const statusRaw = f.get('status');
  const status: StatusFilter = (STATUSES as string[]).includes(statusRaw) ? (statusRaw as CourseStatus) : '';
  const sortKey = SORTS.some((s) => s.value === `${f.get('sort')}-${f.get('dir') || 'desc'}`)
    ? `${f.get('sort')}-${f.get('dir') || 'desc'}`
    : 'createdAt-desc';
  const sortDef = SORTS.find((s) => s.value === sortKey) ?? SORTS[0];
  const q = f.get('q');
  const teacherEmail = f.get('teacherEmail');
  const from = f.get('from');
  const to = f.get('to');
  const page = pageOf(f.get('page'));
  const limit = f.get('limit') === '50' ? 50 : 20;

  const [qInput, setQInput] = useDebouncedInput(q, (v) => f.set({ q: v, page: null }));
  const [teacherInput, setTeacherInput] = useDebouncedInput(teacherEmail, (v) => f.set({ teacherEmail: v, page: null }));
  const extraActive = Boolean(teacherEmail || from || to);
  const [moreOpen, setMoreOpen] = useState(extraActive);
  const periodInvalid = (from !== '' && !isIsoDate(from)) || (to !== '' && !isIsoDate(to)) || (from !== '' && to !== '' && from > to);

  const params = {
    q: q || undefined,
    status: status || undefined,
    teacherEmail: teacherEmail || undefined,
    sort: sortDef.sort,
    dir: sortDef.dir,
    from: from || undefined,
    to: to || undefined,
  };

  const courses = useQuery({
    queryKey: [...contentKeys.courses, params, page, limit],
    queryFn: () => listAdminCourses({ ...params, page, limit }),
    enabled: !periodInvalid,
    placeholderData: keepPreviousData,
  });
  const stats = useQuery({ queryKey: contentKeys.stats, queryFn: getAdminStats, staleTime: 30_000 });
  const counts = stats.data?.courses;

  const [exporting, setExporting] = useState(false);
  async function exportCsv(): Promise<void> {
    setExporting(true);
    try {
      const rows: (string | number)[][] = [];
      let total = 0;
      for (let p = 1; p <= 50; p += 1) {
        const res = await listAdminCourses({ ...params, page: p, limit: 100 });
        total = res.total;
        for (const c of res.items) {
          rows.push([
            c.title,
            c.slug,
            c.teacherEmail,
            COURSE_STATUS[c.status]?.label ?? c.status,
            formatPrice(c.priceCents),
            c.modulesCount,
            c.lessonsCount,
            c.studentsCount,
            fmtDate(c.createdAt),
            fmtDate(c.updatedAt),
          ]);
        }
        if (res.items.length < 100 || rows.length >= res.total) break;
      }
      downloadCsv(
        `courses-${new Date().toISOString().slice(0, 10)}.csv`,
        ['Название', 'Адрес', 'Преподаватель', 'Статус', 'Цена', 'Модулей', 'Уроков', 'Учеников', 'Создан', 'Обновлён'],
        rows,
      );
      toast(rows.length < total ? `Выгружено ${rows.length} из ${total} (ограничение 5000 строк)` : `Выгружено курсов: ${rows.length}`, rows.length < total ? 'info' : 'ok');
    } catch (e) {
      toast(errorText(e), 'error');
    } finally {
      setExporting(false);
    }
  }

  type Row = NonNullable<typeof courses.data>['items'][number] & { coverPath?: string | null };

  const columns: Column<Row>[] = [
    {
      key: 'course',
      header: 'Курс',
      mobile: 'title',
      render: (c) => (
        <span className="acn-course">
          <CourseThumb title={c.title} coverPath={c.coverPath} size={44} />
          <span className="acn-course-text">
            <strong title={c.title}>{c.title}</strong>
            <small>{c.teacherEmail || 'автор не указан'}</small>
            <span className="acn-only-m">
              <CourseStatusBadge status={c.status} />
              <small className="ax-num">обновлён {fmtDate(c.updatedAt)}</small>
            </span>
          </span>
        </span>
      ),
    },
    { key: 'status', header: 'Статус', width: '130px', mobile: 'hidden', render: (c) => <CourseStatusBadge status={c.status} /> },
    { key: 'price', header: 'Цена', width: '110px', align: 'right', render: (c) => <span className="ax-num">{formatPrice(c.priceCents)}</span> },
    { key: 'students', header: 'Ученики', width: '90px', align: 'right', render: (c) => <span className="ax-num">{c.studentsCount}</span> },
    {
      key: 'lessons',
      header: 'Уроки',
      width: '90px',
      align: 'right',
      render: (c) => (
        <span className="ax-num" title={`${c.modulesCount} мод. · ${c.lessonsCount} ур.`}>
          {c.lessonsCount}
        </span>
      ),
    },
    { key: 'updated', header: 'Обновлён', width: '120px', mobile: 'hidden', render: (c) => <span className="ax-num">{fmtDate(c.updatedAt)}</span> },
  ];

  const segOptions: { value: StatusFilter; label: string; count?: number }[] = [
    { value: '', label: 'Все', count: counts?.total },
    ...(['review', 'published', 'draft', 'archived'] as CourseStatus[]).map((s) => ({
      value: s as StatusFilter,
      label: STATUS_TABS[s],
      count: counts?.[s],
    })),
  ];

  const data = courses.data;
  const filtered = !f.isEmpty(['limit']);

  return (
    <AdminPage
      title="Курсы"
      description="Все курсы платформы: черновики, на проверке, опубликованные и архив."
      actions={
        <>
          <button type="button" className="btn btn-ghost" onClick={() => void exportCsv()} disabled={exporting || periodInvalid}>
            <Icon name="download" size={17} /> {exporting ? 'Готовим…' : 'Выгрузить CSV'}
          </button>
          <button type="button" className="btn btn-primary" onClick={() => navigate('/teach/new')}>
            <Icon name="plus" size={17} /> Новый курс
          </button>
        </>
      }
    >
      <Card flush>
        <div className="acn-seg-row">
          <Segmented label="Статус курса" value={status} onChange={(v) => f.set({ status: v, page: null })} options={segOptions} />
        </div>
        <Toolbar
          end={
            <>
              <button
                type="button"
                className={`btn btn-ghost${extraActive ? ' acn-btn-on' : ''}`}
                aria-expanded={moreOpen}
                onClick={() => setMoreOpen((v) => !v)}
              >
                <Icon name="filter" size={16} /> Фильтры
                {extraActive && <span className="acn-dot" aria-label="есть активные фильтры" />}
              </button>
              {filtered && (
                <button type="button" className="btn btn-ghost" onClick={() => f.reset()}>
                  Сбросить
                </button>
              )}
            </>
          }
        >
          <SearchInput value={qInput} onChange={setQInput} placeholder="Название курса" label="Поиск по названию" />
          <FilterSelect
            label="Сортировка"
            value={sortKey}
            onChange={(v) => {
              const s = SORTS.find((x) => x.value === v) ?? SORTS[0];
              f.set({ sort: s.sort === 'createdAt' ? null : s.sort, dir: s.dir === 'desc' ? null : s.dir, page: null });
            }}
            options={SORTS.map((s) => ({ value: s.value, label: s.label }))}
          />
        </Toolbar>
        {moreOpen && (
          <div className="acn-more">
            <Field label="Преподаватель">
              <SearchInput value={teacherInput} onChange={setTeacherInput} placeholder="Email преподавателя" label="Email преподавателя" />
            </Field>
            <Field label="Создан с">
              <input className="ax-input" type="date" value={from} max={to || undefined} onChange={(e) => f.set({ from: e.target.value, page: null })} />
            </Field>
            <Field label="по">
              <input className="ax-input" type="date" value={to} min={from || undefined} onChange={(e) => f.set({ to: e.target.value, page: null })} />
            </Field>
          </div>
        )}
        {periodInvalid && (
          <div className="acn-pad">
            <Note tone="warn">Проверьте период: дата «с» должна быть не позже даты «по».</Note>
          </div>
        )}

        {courses.isError ? (
          <ErrorNote error={courses.error} onRetry={() => void courses.refetch()} />
        ) : !data ? (
          !periodInvalid && <Spinner />
        ) : (
          <div className={`acn-courses${courses.isFetching && courses.isPlaceholderData ? ' acn-fetching' : ''}`}>
            <DataTable<Row>
              caption="Курсы"
              columns={columns}
              rows={data.items as Row[]}
              rowKey={(c) => c.id}
              onRowClick={(c) => navigate(`/admin/courses/${c.id}`)}
              actions={(c) => <RowMenu label={`Действия: ${c.title}`} items={ops.menu(c)} />}
              empty={
                filtered ? (
                  <EmptyState
                    icon="search"
                    title="Ничего не нашлось"
                    text="Попробуйте изменить запрос или сбросить фильтры."
                    action={
                      <button type="button" className="btn btn-ghost" onClick={() => f.reset()}>
                        Сбросить фильтры
                      </button>
                    }
                  />
                ) : (
                  <EmptyState
                    icon="book"
                    title="Курсов пока нет"
                    text="Создайте первый курс в конструкторе."
                    action={
                      <button type="button" className="btn btn-primary" onClick={() => navigate('/teach/new')}>
                        Новый курс
                      </button>
                    }
                  />
                )
              }
            />
          </div>
        )}
        {data && data.total > 0 && (
          <div className="acn-pager">
            <FilterSelect
              label="Строк на странице"
              value={String(limit)}
              onChange={(v) => f.set({ limit: v === '20' ? null : v, page: null })}
              options={[
                { value: '20', label: 'По 20' },
                { value: '50', label: 'По 50' },
              ]}
            />
            <Pager page={page} total={data.total} limit={limit} onPage={(p) => f.set({ page: p > 1 ? String(p) : null })} />
          </div>
        )}
      </Card>
      {ops.dialogs}
    </AdminPage>
  );
}
