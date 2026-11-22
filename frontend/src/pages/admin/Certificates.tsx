import { useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { adminIssueCertificate, adminListCertificates, adminRevokeCertificate, type AdminCertificate } from '../../api/adminOps';
import { overviewQueryKey } from '../../api/adminHub';
import { ApiError } from '../../api/client';
import { isoDay, listAllAdminCourses, looksLikeEmail, salesCoursesKey, useUrlFilters, useUrlSearch } from '../../api/adminSales';
import {
  AdminPage,
  Card,
  CopyButton,
  DataTable,
  EmptyState,
  ErrorNote,
  Field,
  Icon,
  Modal,
  Note,
  Pager,
  RowMenu,
  SearchInput,
  Spinner,
  Toolbar,
  downloadCsv,
  errorText,
  fmtDate,
  plural,
  useAdminUi,
  type Column,
  type MenuItem,
} from '../../components/admin/ui';
import { CourseCell, CoursePicker, StudentCell } from './Enrollments';
import '../../styles/admin-sales.css';

const LIST_KEY = ['admin-certificates'] as const;
const LIMIT = 25;
/** Столько сертификатов максимум отдаёт backend за один запрос. */
const SERVER_CAP = 500;

const publicPath = (n: string): string => `/certificates/${encodeURIComponent(n)}`;
const publicUrl = (n: string): string => `${window.location.origin}${publicPath(n)}`;

/** Модалка «Выдать сертификат»: email + курс. */
function IssueModal({ open, onClose }: { open: boolean; onClose: () => void }): JSX.Element {
  const qc = useQueryClient();
  const { toast } = useAdminUi();
  const courses = useQuery({ queryKey: salesCoursesKey, queryFn: listAllAdminCourses, staleTime: 60_000, enabled: open });
  const [email, setEmail] = useState('');
  const [courseId, setCourseId] = useState('');
  const [touched, setTouched] = useState(false);
  const [issued, setIssued] = useState<string | null>(null);

  const issue = useMutation({
    mutationFn: () => adminIssueCertificate({ email: email.trim(), courseId }),
    onSuccess: (r) => {
      setIssued(r.certificateNumber);
      toast(`Сертификат № ${r.certificateNumber} выдан`);
      void qc.invalidateQueries({ queryKey: LIST_KEY });
      void qc.invalidateQueries({ queryKey: ['admin-enrollments'] });
      void qc.invalidateQueries({ queryKey: overviewQueryKey });
    },
  });

  const notFound = issue.error instanceof ApiError && issue.error.status === 404 && /пользовател/i.test(issue.error.message);

  function reset(): void {
    setEmail('');
    setCourseId('');
    setTouched(false);
    setIssued(null);
    issue.reset();
  }
  function close(): void {
    reset();
    onClose();
  }
  function submit(e: FormEvent): void {
    e.preventDefault();
    setTouched(true);
    if (looksLikeEmail(email) && courseId) issue.mutate();
  }

  return (
    <Modal
      open={open}
      onClose={close}
      title="Выдать сертификат"
      footer={
        issued ? (
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
            <button type="submit" form="asl-cert-form" className="btn btn-primary" disabled={issue.isPending}>
              {issue.isPending ? 'Выдаём…' : 'Выдать сертификат'}
            </button>
          </>
        )
      }
    >
      {issued ? (
        <div className="asl-result">
          <Note tone="ok">
            Готово: сертификат <b className="ax-mono">{issued}</b> выдан <b>{email.trim()}</b>. Доступ к курсу открыт.
          </Note>
          <div className="asl-actions-row">
            <a className="btn btn-ghost btn-sm" href={publicPath(issued)} target="_blank" rel="noopener noreferrer">
              <Icon name="external" size={16} /> Открыть
            </a>
            <CopyButton text={publicUrl(issued)} label="Скопировать ссылку" />
          </div>
        </div>
      ) : (
        <form id="asl-cert-form" className="ax-form" onSubmit={submit} noValidate>
          <Note>
            Например, если курс пройден вне платформы. Пользователь должен быть зарегистрирован; доступ к курсу откроется автоматически.
          </Note>
          <Field
            label="Email ученика"
            hint="Пользователь должен быть зарегистрирован"
            error={touched && !looksLikeEmail(email) ? 'Введите email зарегистрированного пользователя' : undefined}
          >
            <input
              className="ax-input"
              type="email"
              inputMode="email"
              autoComplete="off"
              placeholder="user@example.com"
              value={email}
              onChange={(e) => {
                setEmail(e.target.value);
                issue.reset();
              }}
            />
          </Field>
          <CoursePicker
            courses={courses.data ?? []}
            loading={courses.isLoading}
            value={courseId}
            onChange={(id) => {
              setCourseId(id);
              issue.reset();
            }}
            error={touched && !courseId ? 'Выберите курс' : undefined}
          />
          {issue.isError &&
            (notFound ? (
              <Note tone="warn">
                Пользователь с email <b>{email.trim()}</b> не найден. Попросите его зарегистрироваться или{' '}
                <Link className="ax-link" to="/admin/users?new=1" onClick={close}>
                  создайте пользователя
                </Link>
                .
              </Note>
            ) : (
              <ErrorNote error={issue.error} />
            ))}
        </form>
      )}
    </Modal>
  );
}

export default function Certificates(): JSX.Element {
  const qc = useQueryClient();
  const { confirm, toast } = useAdminUi();
  const { get, set } = useUrlFilters();
  const q = get('q');
  const page = Math.max(1, Number(get('page')) || 1);
  const issueOpen = get('new') === '1';
  const [search, setSearch] = useUrlSearch('q', q, set);

  const list = useQuery({
    queryKey: [...LIST_KEY, q],
    queryFn: () => adminListCertificates(q),
    placeholderData: keepPreviousData,
  });
  const all = list.data?.items ?? [];
  const rows = all.slice((page - 1) * LIMIT, page * LIMIT);

  const revoke = useMutation({
    mutationFn: (n: string) => adminRevokeCertificate(n),
    onSuccess: (_r, n) => {
      toast(`Сертификат № ${n} отозван`);
      void qc.invalidateQueries({ queryKey: LIST_KEY });
      void qc.invalidateQueries({ queryKey: overviewQueryKey });
    },
    onError: (e) => toast(errorText(e), 'error'),
  });

  async function onRevoke(c: AdminCertificate): Promise<void> {
    const ok = await confirm({
      title: `Отозвать сертификат № ${c.number}?`,
      text: (
        <>
          <p>
            {c.studentName || c.email} — «{c.courseTitle}».
          </p>
          <p>Сертификат будет удалён, ссылка на него перестанет работать. Отменить нельзя — при необходимости можно выдать новый (с новым номером).</p>
        </>
      ),
      confirmLabel: 'Отозвать',
      danger: true,
      typeToConfirm: 'ОТОЗВАТЬ',
    });
    if (ok) revoke.mutate(c.number);
  }

  function copyLink(n: string): void {
    void navigator.clipboard
      ?.writeText(publicUrl(n))
      .then(() => toast('Ссылка скопирована'))
      .catch(() => toast('Не удалось скопировать', 'error'));
  }

  function onExport(): void {
    if (all.length === 0) {
      toast('Нечего выгружать — список пуст', 'info');
      return;
    }
    downloadCsv(
      `certificates-${isoDay()}.csv`,
      ['Номер', 'Ученик', 'Email', 'Курс', 'Дата выдачи', 'Ссылка'],
      all.map((c) => [c.number, c.studentName, c.email, c.courseTitle, fmtDate(c.issuedAt), publicUrl(c.number)]),
    );
    toast(`Выгружено: ${all.length}`);
  }

  const menu = (c: AdminCertificate): MenuItem[] => [
    { label: 'Открыть сертификат', icon: 'external', href: publicPath(c.number) },
    { label: 'Скопировать ссылку', icon: 'copy', onClick: () => copyLink(c.number) },
    { label: 'Открыть ученика', icon: 'user', to: `/admin/users/${c.userId}` },
    { label: 'Открыть курс', icon: 'book', to: `/admin/courses/${c.courseId}` },
    'divider',
    { label: 'Отозвать', icon: 'trash', danger: true, onClick: () => void onRevoke(c) },
  ];

  const columns: Column<AdminCertificate>[] = [
    {
      key: 'number',
      header: 'Номер',
      mobile: 'title',
     
      render: (c) => (
        <a className="asl-code" href={publicPath(c.number)} target="_blank" rel="noopener noreferrer" title="Открыть сертификат">
          {c.number}
        </a>
      ),
    },
    { key: 'student', header: 'Ученик', render: (c) => <StudentCell id={c.userId} name={c.studentName} email={c.email} /> },
    { key: 'course', header: 'Курс', render: (c) => <CourseCell id={c.courseId} title={c.courseTitle} /> },
    { key: 'date', header: 'Выдан', render: (c) => <span className="asl-nowrap">{fmtDate(c.issuedAt)}</span> },
  ];

  const openIssue = (): void => set({ new: '1' }, { replace: true });

  return (
    <AdminPage
      title="Сертификаты"
      description="Все выданные сертификаты: поиск по номеру, ученику или курсу, ручная выдача и отзыв."
      actions={
        <button type="button" className="btn btn-primary" onClick={openIssue}>
          <Icon name="award" size={17} /> Выдать сертификат
        </button>
      }
    >
      <Card flush className="asl-card">
        <Toolbar
          end={
            <span className="asl-toolbar-end">
              {all.length >= SERVER_CAP && (
                <span className="ax-muted">
                  Показаны первые {all.length} {plural(all.length, 'сертификат', 'сертификата', 'сертификатов')} — уточните поиск
                </span>
              )}
              <button type="button" className="btn btn-ghost" onClick={onExport} disabled={all.length === 0}>
                <Icon name="download" size={16} /> CSV
              </button>
            </span>
          }
        >
          <SearchInput value={search} onChange={setSearch} placeholder="Номер, email, имя или курс" />
        </Toolbar>
        {list.isLoading ? (
          <Spinner />
        ) : list.isError ? (
          <ErrorNote error={list.error} onRetry={() => void list.refetch()} />
        ) : (
          <>
            <DataTable
              columns={columns}
              rows={rows}
              rowKey={(c) => c.number}
              actions={(c) => <RowMenu items={menu(c)} />}
              caption="Сертификаты"
              empty={
                q ? (
                  <EmptyState icon="search" title="Ничего не найдено" text={`По запросу «${q}» сертификатов нет.`} />
                ) : (
                  <EmptyState
                    icon="award"
                    title="Сертификатов пока нет"
                    text="Они появляются, когда ученик проходит курс целиком. Можно выдать и вручную."
                    action={
                      <button type="button" className="btn btn-primary" onClick={openIssue}>
                        Выдать сертификат
                      </button>
                    }
                  />
                )
              }
            />
            <Pager page={page} total={all.length} limit={LIMIT} onPage={(p) => set({ page: String(p) })} />
          </>
        )}
      </Card>

      {issueOpen && <IssueModal open onClose={() => set({ new: null }, { replace: true })} />}
    </AdminPage>
  );
}
