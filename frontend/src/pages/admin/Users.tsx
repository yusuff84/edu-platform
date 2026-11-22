import { useEffect, useMemo, useState, type FormEvent, type ReactNode } from 'react';
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError } from '../../api/client';
import { deleteUser, listUsers, setUserRole, setUserStatus, type AdminUserRow } from '../../api/adminUsers';
import {
  adminCreateUser,
  adminLogoutUser,
  adminSetUserPassword,
  adminUpdateUserProfile,
  type AdminRole,
} from '../../api/adminOps';
import { grantEnrollment } from '../../api/adminEnrollments';
import { getAdminStats, listAdminCourses } from '../../api/adminConsole';
import { overviewQueryKey } from '../../api/adminHub';
import { getAdminUserCard, userKey, usersListKey } from '../../api/adminUsersHub';
import type { Role } from '../../api/types';
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
  Modal,
  Note,
  Pager,
  ROLE_LABEL,
  RowMenu,
  SearchInput,
  Segmented,
  Spinner,
  Switch,
  Toolbar,
  downloadCsv,
  errorText,
  fmtDate,
  fmtMoney,
  plural,
  useAdminUi,
  useDebounced,
  type Column,
  type MenuItem,
  type Tone,
} from '../../components/admin/ui';
import '../../styles/admin-users.css';

/* ------------------------------------------------------------------ */
/* Общее для списка и карточки пользователя                              */
/* ------------------------------------------------------------------ */

const PAGE_LIMIT = 20;
const EXPORT_PAGE_LIMIT = 100;
const EXPORT_MAX_PAGES = 50;

const ROLE_TONE: Record<string, Tone> = { student: 'neutral', teacher: 'info', admin: 'brand' };

export function RoleBadge({ role }: { role: string }): JSX.Element {
  return <Badge tone={ROLE_TONE[role] ?? 'neutral'}>{ROLE_LABEL[role] ?? role}</Badge>;
}

export function StatusBadge({ status }: { status: string }): JSX.Element {
  return status === 'blocked' ? <Badge tone="danger">Заблокирован</Badge> : <Badge tone="ok">Активен</Badge>;
}

const ROLE_HINT: Record<AdminRole, string> = {
  student: 'Проходит курсы. Видит только свой кабинет ученика.',
  teacher: 'Создаёт свои курсы, видит своих учеников и отвечает на вопросы.',
  admin: 'Полный доступ к админке: пользователи, деньги, настройки.',
};

const ROLES: AdminRole[] = ['student', 'teacher', 'admin'];

const isEmail = (v: string): boolean => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v.trim());

/** Сбросить кэш всего, что зависит от пользователей (список, карточка, счётчики меню и обзора). */
export function useInvalidateUsers(): (id?: string) => void {
  const qc = useQueryClient();
  return (id?: string) => {
    void qc.invalidateQueries({ queryKey: usersListKey });
    if (id) void qc.invalidateQueries({ queryKey: userKey(id) });
    void qc.invalidateQueries({ queryKey: overviewQueryKey });
    void qc.invalidateQueries({ queryKey: ['admin', 'stats'] });
  };
}

type Dialog =
  | { kind: 'role'; user: AdminUserRow }
  | { kind: 'password'; user: AdminUserRow }
  | { kind: 'grant'; user: AdminUserRow }
  | { kind: 'profile'; user: AdminUserRow };

/**
 * Все действия над пользователем (меню строки списка и кнопки карточки):
 * роль, пароль, профиль, выдача курса, выход везде, блокировка, удаление.
 * Возвращает функции, пункты меню и разметку диалогов (её нужно отрендерить).
 */
export function useUserActions(opts: { onDeleted?: (u: AdminUserRow) => void } = {}) {
  const { confirm, toast } = useAdminUi();
  const invalidate = useInvalidateUsers();
  const [dialog, setDialog] = useState<Dialog | null>(null);
  const close = (): void => setDialog(null);

  async function applyStatus(user: AdminUserRow, next: 'active' | 'blocked'): Promise<void> {
    try {
      await setUserStatus(user.id, next);
      toast(next === 'blocked' ? `${user.email} заблокирован` : `${user.email} снова может входить`);
      invalidate(user.id);
    } catch (e) {
      toast(errorText(e), 'error');
    }
  }

  async function toggleBlock(user: AdminUserRow): Promise<void> {
    if (user.status === 'blocked') {
      const ok = await confirm({
        title: 'Разблокировать пользователя?',
        text: `${user.email} снова сможет входить на сайт и проходить свои курсы.`,
        confirmLabel: 'Разблокировать',
      });
      if (ok) await applyStatus(user, 'active');
      return;
    }
    const ok = await confirm({
      title: 'Заблокировать пользователя?',
      text: (
        <>
          <b>{user.email}</b> не сможет войти на сайт. Курсы, прогресс и платежи сохранятся —
          разблокировать можно в любой момент.
        </>
      ),
      confirmLabel: 'Заблокировать',
      danger: true,
    });
    if (ok) await applyStatus(user, 'blocked');
  }

  async function logoutAll(user: AdminUserRow): Promise<void> {
    const ok = await confirm({
      title: 'Выйти на всех устройствах?',
      text: `Все сеансы ${user.email} будут завершены — ему придётся войти заново. Пароль не меняется.`,
      confirmLabel: 'Завершить сеансы',
    });
    if (!ok) return;
    try {
      await adminLogoutUser(user.id);
      toast('Все сеансы пользователя завершены');
    } catch (e) {
      toast(errorText(e), 'error');
    }
  }

  /** Удалить нельзя — объясняем почему и предлагаем заблокировать. */
  async function offerBlock(user: AdminUserRow, reason: ReactNode): Promise<void> {
    if (user.status === 'blocked') {
      await confirm({
        title: 'Этого пользователя нельзя удалить',
        text: (
          <>
            {reason}
            <p className="aus-confirm-p">Он уже заблокирован и не может войти — этого достаточно, чтобы закрыть ему доступ.</p>
          </>
        ),
        confirmLabel: 'Понятно',
      });
      return;
    }
    const ok = await confirm({
      title: 'Этого пользователя нельзя удалить',
      text: (
        <>
          {reason}
          <p className="aus-confirm-p">
            Вместо удаления можно <b>заблокировать</b> его: он не сможет войти, а история сохранится.
          </p>
        </>
      ),
      confirmLabel: 'Заблокировать',
      danger: true,
    });
    if (ok) await applyStatus(user, 'blocked');
  }

  async function remove(user: AdminUserRow): Promise<void> {
    if (user.coursesCount > 0) {
      await offerBlock(
        user,
        <p className="aus-confirm-p">
          Пользователь — автор {user.coursesCount} {plural(user.coursesCount, 'курса', 'курсов', 'курсов')}. Сначала
          передайте курсы другому преподавателю или удалите их.
        </p>,
      );
      return;
    }
    const ok = await confirm({
      title: 'Удалить пользователя навсегда?',
      text: (
        <>
          Учётная запись <b>{user.email}</b> будет удалена вместе с записями на курсы и прогрессом.
          Отменить это нельзя.
        </>
      ),
      confirmLabel: 'Удалить навсегда',
      danger: true,
      typeToConfirm: user.email,
    });
    if (!ok) return;
    try {
      await deleteUser(user.id);
      toast(`Пользователь ${user.email} удалён`);
      invalidate(user.id);
      opts.onDeleted?.(user);
    } catch (e) {
      if (e instanceof ApiError && e.status === 409 && /платеж/i.test(e.message)) {
        await offerBlock(
          user,
          <p className="aus-confirm-p">У пользователя есть платежи — история оплат должна сохраниться для бухгалтерии.</p>,
        );
      } else if (e instanceof ApiError && e.status === 409 && /курс/i.test(e.message)) {
        await offerBlock(user, <p className="aus-confirm-p">У пользователя есть свои курсы.</p>);
      } else {
        toast(errorText(e), 'error');
      }
    }
  }

  const open = {
    role: (user: AdminUserRow) => setDialog({ kind: 'role', user }),
    password: (user: AdminUserRow) => setDialog({ kind: 'password', user }),
    grant: (user: AdminUserRow) => setDialog({ kind: 'grant', user }),
    profile: (user: AdminUserRow) => setDialog({ kind: 'profile', user }),
  };

  function menuItems(user: AdminUserRow, withOpen = true): MenuItem[] {
    const items: MenuItem[] = [];
    if (withOpen) items.push({ label: 'Открыть карточку', icon: 'user', to: `/admin/users/${user.id}` });
    items.push(
      { label: 'Выдать курс', icon: 'book', onClick: () => open.grant(user) },
      { label: 'Изменить профиль', icon: 'edit', onClick: () => open.profile(user) },
      { label: 'Сменить роль', icon: 'shield', onClick: () => open.role(user) },
      { label: 'Задать новый пароль', icon: 'key', onClick: () => open.password(user) },
      { label: 'Выйти на всех устройствах', icon: 'logout', onClick: () => void logoutAll(user) },
      'divider',
      user.status === 'blocked'
        ? { label: 'Разблокировать', icon: 'unlock', onClick: () => void toggleBlock(user) }
        : { label: 'Заблокировать', icon: 'lock', onClick: () => void toggleBlock(user) },
      { label: 'Удалить', icon: 'trash', danger: true, onClick: () => void remove(user) },
    );
    return items;
  }

  const element = (
    <>
      {dialog?.kind === 'role' && <RoleModal user={dialog.user} onClose={close} />}
      {dialog?.kind === 'password' && <PasswordModal user={dialog.user} onClose={close} />}
      {dialog?.kind === 'grant' && <GrantCourseModal user={dialog.user} onClose={close} />}
      {dialog?.kind === 'profile' && <ProfileModal user={dialog.user} onClose={close} />}
    </>
  );

  return { menuItems, toggleBlock, logoutAll, remove, open, element };
}

/* ---------- Смена роли ---------- */

function RoleModal({ user, onClose }: { user: AdminUserRow; onClose: () => void }): JSX.Element {
  const { toast } = useAdminUi();
  const invalidate = useInvalidateUsers();
  const [role, setRole] = useState<AdminRole>(user.role as AdminRole);
  const save = useMutation({
    mutationFn: () => setUserRole(user.id, role as Role),
    onSuccess: () => {
      toast(`Роль изменена: ${ROLE_LABEL[role]}`);
      invalidate(user.id);
      onClose();
    },
  });
  return (
    <Modal
      open
      onClose={onClose}
      title="Сменить роль"
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            Отмена
          </button>
          <button
            type="button"
            className="btn btn-primary"
            disabled={role === user.role || save.isPending}
            onClick={() => save.mutate()}
          >
            {save.isPending ? 'Сохраняем…' : 'Сохранить'}
          </button>
        </>
      }
    >
      <p className="aus-modal-lead">
        <b>{user.displayName || user.email}</b>
        {user.displayName && <span className="ax-muted"> · {user.email}</span>}
      </p>
      <div className="aus-choices" role="radiogroup" aria-label="Роль">
        {ROLES.map((r) => (
          <label key={r} className={`aus-choice${role === r ? ' is-active' : ''}`}>
            <input type="radio" name="aus-role" value={r} checked={role === r} onChange={() => setRole(r)} />
            <span className="aus-choice-text">
              <span className="aus-choice-title">
                {ROLE_LABEL[r]}
                {user.role === r && <span className="ax-muted"> — сейчас</span>}
              </span>
              <small>{ROLE_HINT[r]}</small>
            </span>
          </label>
        ))}
      </div>
      {role === 'admin' && user.role !== 'admin' && (
        <Note tone="warn">Администратор видит и меняет всё на платформе. Выдавайте эту роль только доверенным людям.</Note>
      )}
      {save.isError && <ErrorNote error={save.error} />}
    </Modal>
  );
}

/* ---------- Новый пароль ---------- */

function PasswordModal({ user, onClose }: { user: AdminUserRow; onClose: () => void }): JSX.Element {
  const { toast } = useAdminUi();
  const [mode, setMode] = useState<'generate' | 'manual'>('generate');
  const [password, setPassword] = useState('');
  const [touched, setTouched] = useState(false);
  const [shown, setShown] = useState('');
  const tooShort = mode === 'manual' && password.length < 8;
  const save = useMutation({
    mutationFn: () => adminSetUserPassword(user.id, mode === 'manual' ? password : undefined),
    onSuccess: (res) => {
      if (res.password) {
        setShown(res.password);
      } else {
        toast('Пароль изменён, все сеансы пользователя завершены');
        onClose();
      }
    },
  });
  function onSubmit(e: FormEvent): void {
    e.preventDefault();
    setTouched(true);
    if (tooShort) return;
    save.mutate();
  }
  if (shown) {
    return (
      <Modal
        open
        onClose={onClose}
        title="Новый пароль готов"
        footer={
          <button type="button" className="btn btn-primary" onClick={onClose}>
            Готово
          </button>
        }
      >
        <p className="aus-modal-lead">
          Пароль для <b>{user.email}</b>. Он показывается <b>один раз</b> — скопируйте и передайте пользователю.
        </p>
        <div className="aus-secret">
          <code className="ax-mono">{shown}</code>
          <CopyButton text={shown} />
        </div>
        <Note tone="ok">Все сеансы пользователя завершены — войти можно только с новым паролем.</Note>
      </Modal>
    );
  }
  return (
    <Modal
      open
      onClose={onClose}
      title="Задать новый пароль"
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            Отмена
          </button>
          <button type="submit" form="aus-password-form" className="btn btn-primary" disabled={save.isPending}>
            {save.isPending ? 'Меняем…' : mode === 'generate' ? 'Сгенерировать' : 'Сохранить пароль'}
          </button>
        </>
      }
    >
      <form id="aus-password-form" className="ax-form" onSubmit={onSubmit} noValidate>
        <p className="aus-modal-lead">
          Пользователь <b>{user.email}</b>. Старый пароль перестанет работать, а все его сеансы будут завершены.
        </p>
        <Segmented
          label="Способ"
          value={mode}
          onChange={setMode}
          options={[
            { value: 'generate', label: 'Сгенерировать' },
            { value: 'manual', label: 'Ввести вручную' },
          ]}
        />
        {mode === 'manual' ? (
          <Field
            label="Новый пароль"
            hint="Не короче 8 символов."
            error={touched && tooShort ? 'Пароль должен быть не короче 8 символов' : undefined}
          >
            <input
              className="ax-input"
              type="text"
              autoComplete="off"
              value={password}
              maxLength={72}
              onChange={(e) => setPassword(e.target.value)}
              autoFocus
            />
          </Field>
        ) : (
          <p className="ax-muted">Мы создадим надёжный пароль и покажем его один раз — вы сможете его скопировать.</p>
        )}
        {save.isError && <ErrorNote error={save.error} />}
      </form>
    </Modal>
  );
}

/* ---------- Профиль (email и имя) ---------- */

function ProfileModal({ user, onClose }: { user: AdminUserRow; onClose: () => void }): JSX.Element {
  const { toast } = useAdminUi();
  const invalidate = useInvalidateUsers();
  const [email, setEmail] = useState(user.email);
  const [name, setName] = useState(user.displayName ?? '');
  const [touched, setTouched] = useState(false);
  const emailError = !isEmail(email) ? 'Введите корректный email' : undefined;
  const changed = email.trim().toLowerCase() !== user.email || name.trim() !== (user.displayName ?? '');
  const save = useMutation({
    mutationFn: () => adminUpdateUserProfile(user.id, { email: email.trim(), displayName: name.trim() }),
    onSuccess: () => {
      toast('Профиль сохранён');
      invalidate(user.id);
      onClose();
    },
  });
  function onSubmit(e: FormEvent): void {
    e.preventDefault();
    setTouched(true);
    if (emailError || !changed) return;
    save.mutate();
  }
  return (
    <Modal
      open
      onClose={onClose}
      title="Изменить профиль"
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            Отмена
          </button>
          <button type="submit" form="aus-profile-form" className="btn btn-primary" disabled={save.isPending || !changed}>
            {save.isPending ? 'Сохраняем…' : 'Сохранить'}
          </button>
        </>
      }
    >
      <form id="aus-profile-form" className="ax-form" onSubmit={onSubmit} noValidate>
        <Field
          label="Email"
          hint="На этот адрес пользователь входит и получает письма."
          error={touched ? emailError : undefined}
        >
          <input className="ax-input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
        </Field>
        <Field label="Имя" hint={`Так имя будет написано в сертификате. ${name.length}/60`}>
          <input className="ax-input" value={name} maxLength={60} onChange={(e) => setName(e.target.value)} placeholder="Имя и фамилия" />
        </Field>
        {save.isError && <ErrorNote error={save.error} />}
      </form>
    </Modal>
  );
}

/* ---------- Выдать курс ---------- */

const COURSE_STATUS: Record<string, { label: string; tone: Tone }> = {
  draft: { label: 'Черновик', tone: 'neutral' },
  review: { label: 'На проверке', tone: 'warn' },
  published: { label: 'Опубликован', tone: 'ok' },
  archived: { label: 'В архиве', tone: 'neutral' },
};

function GrantCourseModal({ user, onClose }: { user: AdminUserRow; onClose: () => void }): JSX.Element {
  const { toast } = useAdminUi();
  const invalidate = useInvalidateUsers();
  const qc = useQueryClient();
  const [search, setSearch] = useState('');
  const [onlyPublished, setOnlyPublished] = useState(true);
  const [picked, setPicked] = useState('');
  const q = useDebounced(search.trim(), 300);
  const courses = useQuery({
    queryKey: ['admin', 'users', 'grant-courses', q, onlyPublished],
    queryFn: () =>
      listAdminCourses({ q: q || undefined, status: onlyPublished ? 'published' : undefined, limit: 50, sort: 'title', dir: 'asc' }),
  });
  const card = useQuery({ queryKey: userKey(user.id), queryFn: () => getAdminUserCard(user.id) });
  const owned = useMemo(() => new Map((card.data?.items ?? []).map((c) => [c.courseId, c.status])), [card.data]);
  const pickedCourse = courses.data?.items.find((c) => c.id === picked);

  const grant = useMutation({
    mutationFn: () => grantEnrollment(user.email, picked),
    onSuccess: (r) => {
      toast(
        r.wasActive
          ? `У пользователя уже был доступ к «${r.courseTitle}»`
          : r.created
            ? `Доступ к курсу «${r.courseTitle}» открыт`
            : `Доступ к курсу «${r.courseTitle}» возвращён`,
      );
      invalidate(user.id);
      void qc.invalidateQueries({ queryKey: ['admin-enrollments'] });
      onClose();
    },
  });

  return (
    <Modal
      open
      onClose={onClose}
      title="Выдать курс"
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            Отмена
          </button>
          <button type="button" className="btn btn-primary" disabled={!picked || grant.isPending} onClick={() => grant.mutate()}>
            {grant.isPending ? 'Открываем…' : 'Открыть доступ'}
          </button>
        </>
      }
    >
      <div className="ax-form">
        <p className="aus-modal-lead">
          Доступ для <b>{user.email}</b> — без оплаты, даже к платному курсу. Платёж при этом не создаётся.
        </p>
        <SearchInput value={search} onChange={setSearch} placeholder="Название курса" label="Поиск курса" />
        <Switch
          checked={!onlyPublished}
          onChange={(v) => {
            setOnlyPublished(!v);
            setPicked('');
          }}
          label="Показать все курсы"
          hint="Включая курсы на проверке и в архиве. Черновик выдать нельзя."
        />
        {courses.isLoading && <Spinner />}
        {courses.isError && <ErrorNote error={courses.error} onRetry={() => void courses.refetch()} />}
        {courses.data && courses.data.items.length === 0 && (
          <EmptyState icon="book" title="Курсы не найдены" text="Попробуйте другое название или покажите все курсы." />
        )}
        {courses.data && courses.data.items.length > 0 && (
          <div className="aus-pick" role="radiogroup" aria-label="Курс">
            {courses.data.items.map((c) => {
              const ownStatus = owned.get(c.id);
              const isDraft = c.status === 'draft';
              const hasAccess = ownStatus === 'active';
              const disabled = isDraft || hasAccess;
              const st = COURSE_STATUS[c.status];
              return (
                <label key={c.id} className={`aus-pick-item${picked === c.id ? ' is-active' : ''}${disabled ? ' is-disabled' : ''}`}>
                  <input
                    type="radio"
                    name="aus-grant-course"
                    value={c.id}
                    checked={picked === c.id}
                    disabled={disabled}
                    onChange={() => setPicked(c.id)}
                  />
                  <span className="aus-pick-text">
                    <span className="aus-pick-title">{c.title}</span>
                    <small>
                      {c.priceCents > 0 ? fmtMoney(c.priceCents) : 'Бесплатный'} · {c.teacherEmail}
                    </small>
                  </span>
                  <span className="aus-pick-badges">
                    {hasAccess && <Badge tone="ok">Уже есть</Badge>}
                    {ownStatus && ownStatus !== 'active' && <Badge tone="warn">Доступ закрыт</Badge>}
                    {c.status !== 'published' && st && <Badge tone={st.tone}>{st.label}</Badge>}
                  </span>
                </label>
              );
            })}
          </div>
        )}
        {courses.data && courses.data.total > courses.data.items.length && (
          <p className="ax-muted">Показаны первые {courses.data.items.length} из {courses.data.total} — уточните поиск.</p>
        )}
        {pickedCourse && owned.get(pickedCourse.id) && owned.get(pickedCourse.id) !== 'active' && (
          <Note tone="info">Раньше доступ к этому курсу был закрыт — он снова станет активным, прогресс сохранится.</Note>
        )}
        {grant.isError && <ErrorNote error={grant.error} />}
      </div>
    </Modal>
  );
}

/* ---------- Создание пользователя ---------- */

function CreateUserModal({ onClose }: { onClose: () => void }): JSX.Element {
  const invalidate = useInvalidateUsers();
  const navigate = useNavigate();
  const [email, setEmail] = useState('');
  const [name, setName] = useState('');
  const [role, setRole] = useState<AdminRole>('student');
  const [generate, setGenerate] = useState(true);
  const [password, setPassword] = useState('');
  const [touched, setTouched] = useState(false);
  const emailError = !email.trim() ? 'Укажите email' : !isEmail(email) ? 'Введите корректный email' : undefined;
  const passwordError = !generate && password.length < 8 ? 'Пароль должен быть не короче 8 символов' : undefined;
  const create = useMutation({
    mutationFn: () =>
      adminCreateUser({
        email: email.trim(),
        role,
        displayName: name.trim() || undefined,
        password: generate ? undefined : password,
      }),
    onSuccess: () => invalidate(),
  });
  function onSubmit(e: FormEvent): void {
    e.preventDefault();
    setTouched(true);
    if (emailError || passwordError) return;
    create.mutate();
  }
  function again(): void {
    create.reset();
    setEmail('');
    setName('');
    setPassword('');
    setTouched(false);
  }

  if (create.data) {
    const res = create.data;
    return (
      <Modal
        open
        onClose={onClose}
        title="Пользователь создан"
        footer={
          <>
            <button type="button" className="btn btn-ghost" onClick={again}>
              Создать ещё
            </button>
            <button
              type="button"
              className="btn btn-primary"
              onClick={() => {
                onClose();
                navigate(`/admin/users/${res.user.id}`);
              }}
            >
              Открыть карточку
            </button>
          </>
        }
      >
        <p className="aus-modal-lead">
          <b>{res.user.email}</b> · {ROLE_LABEL[res.user.role]}
        </p>
        {res.password ? (
          <>
            <p className="ax-muted">Пароль показывается один раз — скопируйте и передайте пользователю.</p>
            <div className="aus-secret">
              <code className="ax-mono">{res.password}</code>
              <CopyButton text={res.password} />
            </div>
          </>
        ) : (
          <Note tone="ok">Пользователь войдёт с паролем, который вы задали. Письмо не отправляется.</Note>
        )}
      </Modal>
    );
  }

  return (
    <Modal
      open
      onClose={onClose}
      title="Создать пользователя"
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            Отмена
          </button>
          <button type="submit" form="aus-create-form" className="btn btn-primary" disabled={create.isPending}>
            {create.isPending ? 'Создаём…' : 'Создать'}
          </button>
        </>
      }
    >
      <form id="aus-create-form" className="ax-form" onSubmit={onSubmit} noValidate>
        <Field label="Email" error={touched ? emailError : undefined} hint="Пользователь будет входить по этому адресу. Письмо не отправляется.">
          <input className="ax-input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="off" />
        </Field>
        <Field label="Имя" hint="Необязательно. Так имя будет написано в сертификате.">
          <input className="ax-input" value={name} maxLength={60} onChange={(e) => setName(e.target.value)} placeholder="Имя и фамилия" />
        </Field>
        <Field label="Роль" hint={ROLE_HINT[role]}>
          <select className="ax-input ax-select" value={role} onChange={(e) => setRole(e.target.value as AdminRole)}>
            {ROLES.map((r) => (
              <option key={r} value={r}>
                {ROLE_LABEL[r]}
              </option>
            ))}
          </select>
        </Field>
        <Switch
          checked={generate}
          onChange={setGenerate}
          label="Сгенерировать пароль"
          hint="Покажем его один раз после создания."
        />
        {!generate && (
          <Field label="Пароль" hint="Не короче 8 символов." error={touched ? passwordError : undefined}>
            <input className="ax-input" type="text" autoComplete="off" maxLength={72} value={password} onChange={(e) => setPassword(e.target.value)} />
          </Field>
        )}
        {create.isError && <ErrorNote error={create.error} />}
      </form>
    </Modal>
  );
}

/* ------------------------------------------------------------------ */
/* Страница списка                                                      */
/* ------------------------------------------------------------------ */

type RoleFilter = 'all' | 'student' | 'teacher' | 'admin';
type StatusFilter = 'all' | 'active' | 'blocked';

const ROLE_FILTERS: RoleFilter[] = ['all', 'student', 'teacher', 'admin'];
const STATUS_FILTERS: StatusFilter[] = ['all', 'active', 'blocked'];

export default function Users(): JSX.Element {
  const [params, setParams] = useSearchParams();
  const navigate = useNavigate();
  const location = useLocation();
  const { toast } = useAdminUi();

  const q = params.get('q') ?? '';
  const roleParam = params.get('role') as RoleFilter | null;
  const role: RoleFilter = roleParam && ROLE_FILTERS.includes(roleParam) ? roleParam : 'all';
  const statusParam = params.get('status') as StatusFilter | null;
  const status: StatusFilter = statusParam && STATUS_FILTERS.includes(statusParam) ? statusParam : 'all';
  const page = Math.max(1, Number(params.get('page')) || 1);
  const creating = params.get('new') === '1';

  const update = (patch: Record<string, string | null>, replace = false): void => {
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        for (const [k, v] of Object.entries(patch)) {
          if (v === null || v === '' || v === 'all') next.delete(k);
          else next.set(k, v);
        }
        return next;
      },
      { replace },
    );
  };

  // Поиск: поле ввода локальное, в URL уходит значение после паузы в наборе.
  const [search, setSearch] = useState(q);
  const debounced = useDebounced(search.trim(), 300);
  useEffect(() => {
    if (debounced !== q) update({ q: debounced, page: null }, true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debounced]);
  useEffect(() => {
    setSearch((prev) => (prev.trim() === q ? prev : q));
  }, [q]);

  const filters = {
    q: q || undefined,
    role: role === 'all' ? undefined : role,
    status: status === 'all' ? undefined : status,
  };
  const users = useQuery({
    queryKey: [...usersListKey, { ...filters, page }],
    queryFn: () => listUsers({ ...filters, page, limit: PAGE_LIMIT }),
    placeholderData: (prev) => prev,
  });
  const stats = useQuery({ queryKey: ['admin', 'stats'], queryFn: getAdminStats, staleTime: 60_000 });
  const s = stats.data?.users;

  // Счётчики показываем только там, где они совпадают с выборкой (без поиска и второго фильтра).
  const roleCounts = s && !q && status === 'all';
  const statusCounts = s && !q && role === 'all';

  const actions = useUserActions({
    onDeleted: () => {
      if (users.data && users.data.items.length === 1 && page > 1) update({ page: String(page - 1) });
    },
  });

  const [exporting, setExporting] = useState(false);
  async function exportCsv(): Promise<void> {
    setExporting(true);
    try {
      const rows: (string | number)[][] = [];
      let total = 0;
      for (let p = 1; p <= EXPORT_MAX_PAGES; p += 1) {
        const res = await listUsers({ ...filters, page: p, limit: EXPORT_PAGE_LIMIT });
        if (p === 1) total = res.total;
        for (const u of res.items) {
          rows.push([
            u.email,
            u.displayName,
            ROLE_LABEL[u.role] ?? u.role,
            u.status === 'blocked' ? 'Заблокирован' : 'Активен',
            fmtDate(u.createdAt),
            u.enrollmentsCount,
            u.coursesCount,
          ]);
        }
        if (res.items.length < EXPORT_PAGE_LIMIT || rows.length >= res.total) break;
      }
      if (rows.length === 0) {
        toast('Нечего выгружать — список пуст', 'info');
        return;
      }
      const stamp = new Date().toISOString().slice(0, 10);
      downloadCsv(
        `users-${stamp}.csv`,
        ['Email', 'Имя', 'Роль', 'Статус', 'Дата регистрации', 'Записей на курсы', 'Своих курсов'],
        rows,
      );
      toast(
        rows.length < total
          ? `Выгружено ${rows.length} из ${total} — сузьте фильтры, чтобы выгрузить остальное`
          : `Выгружено ${rows.length} ${plural(rows.length, 'пользователь', 'пользователя', 'пользователей')}`,
        rows.length < total ? 'info' : 'ok',
      );
    } catch (e) {
      toast(errorText(e), 'error');
    } finally {
      setExporting(false);
    }
  }

  const columns: Column<AdminUserRow>[] = [
    {
      key: 'user',
      header: 'Пользователь',
      mobile: 'title',
      render: (u) => (
        <Link className="ax-person aus-person" to={`/admin/users/${u.id}`} state={{ from: location.search }}>
          <Avatar name={u.displayName || u.email} />
          <span className="ax-person-text">
            <strong>{u.displayName || u.email.replace(/@.*/, '')}</strong>
            <small>{u.email}</small>
            <span className="aus-mobile-badges">
              <RoleBadge role={u.role} />
              {u.status === 'blocked' && <StatusBadge status={u.status} />}
            </span>
          </span>
        </Link>
      ),
    },
    { key: 'role', header: 'Роль', width: '150px', mobile: 'hidden', render: (u) => <RoleBadge role={u.role} /> },
    { key: 'status', header: 'Статус', width: '140px', mobile: 'hidden', render: (u) => <StatusBadge status={u.status} /> },
    { key: 'created', header: 'Регистрация', width: '140px', render: (u) => <span className="ax-num">{fmtDate(u.createdAt)}</span> },
    {
      key: 'courses',
      header: 'Курсы',
      width: '150px',
      render: (u) => (
        <span className="aus-counts">
          <span className="ax-num" title="Записей на курсы">
            {u.enrollmentsCount} {plural(u.enrollmentsCount, 'запись', 'записи', 'записей')}
          </span>
          {u.coursesCount > 0 && (
            <small className="ax-muted ax-num">
              автор {u.coursesCount} {plural(u.coursesCount, 'курса', 'курсов', 'курсов')}
            </small>
          )}
        </span>
      ),
    },
  ];

  const data = users.data;
  const filtered = Boolean(q || role !== 'all' || status !== 'all');

  return (
    <AdminPage
      title="Пользователи"
      description="Ученики, преподаватели и администраторы: доступы, роли, пароли и блокировки."
      actions={
        <>
          <button type="button" className="btn btn-ghost" onClick={() => void exportCsv()} disabled={exporting}>
            <Icon name="download" size={17} /> {exporting ? 'Готовим файл…' : 'Выгрузить CSV'}
          </button>
          <button type="button" className="btn btn-primary" onClick={() => update({ new: '1' })}>
            <Icon name="plus" size={17} /> Создать пользователя
          </button>
        </>
      }
    >
      <Card flush className={`aus-list${users.isFetching && users.isPlaceholderData ? ' is-stale' : ''}`}>
        <Toolbar>
          <SearchInput value={search} onChange={setSearch} placeholder="Email или имя" label="Поиск пользователя" />
          <Segmented<RoleFilter>
            label="Роль"
            value={role}
            onChange={(v) => update({ role: v, page: null })}
            options={[
              { value: 'all', label: 'Все', count: roleCounts ? s.total : undefined },
              { value: 'student', label: 'Ученики', count: roleCounts ? s.students : undefined },
              { value: 'teacher', label: 'Преподаватели', count: roleCounts ? s.teachers : undefined },
              { value: 'admin', label: 'Админы', count: roleCounts ? s.admins : undefined },
            ]}
          />
          <Segmented<StatusFilter>
            label="Статус"
            value={status}
            onChange={(v) => update({ status: v, page: null })}
            options={[
              { value: 'all', label: 'Все' },
              { value: 'active', label: 'Активные', count: statusCounts ? s.total - s.blocked : undefined },
              { value: 'blocked', label: 'Заблокированные', count: statusCounts ? s.blocked : undefined },
            ]}
          />
        </Toolbar>

        {users.isError && !data && (
          <div className="aus-pad">
            <ErrorNote error={users.error} onRetry={() => void users.refetch()} />
          </div>
        )}
        {users.isLoading && <Spinner />}
        {data && (
          <>
            <DataTable
              columns={columns}
              rows={data.items}
              rowKey={(u) => u.id}
              caption="Пользователи"
              onRowClick={(u) => navigate(`/admin/users/${u.id}`, { state: { from: location.search } })}
              actions={(u) => <RowMenu items={actions.menuItems(u)} label={`Действия: ${u.email}`} />}
              empty={
                filtered ? (
                  <EmptyState
                    icon="search"
                    title="Никого не нашли"
                    text="Проверьте написание или сбросьте фильтры."
                    action={
                      <button
                        type="button"
                        className="btn btn-ghost btn-sm"
                        onClick={() => {
                          setSearch('');
                          update({ q: null, role: null, status: null, page: null });
                        }}
                      >
                        Сбросить фильтры
                      </button>
                    }
                  />
                ) : (
                  <EmptyState
                    icon="users"
                    title="Пользователей пока нет"
                    action={
                      <button type="button" className="btn btn-primary btn-sm" onClick={() => update({ new: '1' })}>
                        Создать пользователя
                      </button>
                    }
                  />
                )
              }
            />
            <Pager page={data.page || page} total={data.total} limit={PAGE_LIMIT} onPage={(p) => update({ page: p === 1 ? null : String(p) })} />
          </>
        )}
      </Card>

      {creating && <CreateUserModal onClose={() => update({ new: null }, true)} />}
      {actions.element}
    </AdminPage>
  );
}
