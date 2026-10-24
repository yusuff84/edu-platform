// T26: раздел «Пользователи» супер-админки.
// Таблица с поиском (debounce), фильтрами роль/статус, пагинацией и мутациями
// смены роли / блокировки / удаления. Контракт backend — T24.
// T52: карточка «Профиль пользователя» (детали, записи, платежи) и
// инлайн-подтверждения смены роли / блокировки; правки вёрстки таблицы.
// T94: активные сессии пользователя и принудительный выход.
// Стили изолированы в styles/adminUsers.css и styles/adminSessions.css;
// styles.css и admin.css не изменяются.
import { useEffect, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { QueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { ApiError, formatDate, formatMoney } from '../api/client';
import {
  deleteUser,
  listStudentEnrollments,
  listStudentPayments,
  listUsers,
  setUserRole,
  setUserStatus,
} from '../api/adminUsers';
import type {
  AdminUserPage,
  AdminUserRow,
  EnrollmentStatus,
  UserStatus,
} from '../api/adminUsers';
import {
  listAdminUserSessions,
  revokeAdminUserSessions,
} from '../api/adminConsole';
import type { AdminSessionsResponse } from '../api/adminConsole';
import type { PaymentStatus, Role } from '../api/types';
import { t } from '../i18n/t';
import type { RuKey } from '../i18n/t';
import CsvExportButton, { csvDateStamp, exportTruncated } from '../components/CsvExportButton';
import '../styles/adminUsers.css';
import '../styles/adminSessions.css';

// ---------- Константы ----------

/** Задержка поиска: запрос уходит после паузы в наборе (ТЗ T26 — ~300 мс). */
const SEARCH_DEBOUNCE_MS = 300;

/** Размер страницы (контракт T24: limit=20). */
const PAGE_LIMIT = 20;

/** Размер страницы выгрузки CSV (T40): 100 строк за запрос. */
const EXPORT_PAGE_LIMIT = 100;

/** Потолок выгрузки CSV: не более 50 страниц (5000 строк) за один клик. */
const EXPORT_MAX_PAGES = 50;

/** Размер мини-списков в карточке «Профиль» (T52): записи и платежи. */
const DETAILS_LIMIT = 10;

/** Префикс ключа кэша списка: по нему инвалидируются все страницы с фильтрами. */
const LIST_KEY = ['admin-users'] as const;

/** Порядок ролей в select — от младшей к старшей. */
const ROLES: Role[] = ['student', 'teacher', 'admin'];

/** Порядок статусов в фильтре. */
const STATUSES: UserStatus[] = ['active', 'blocked'];

/** Русские подписи ролей — ключи из словаря. */
const ROLE_LABEL: Record<Role, RuKey> = {
  student: 'adminUsers.roleStudent',
  teacher: 'adminUsers.roleTeacher',
  admin: 'adminUsers.roleAdmin',
};

/** Русские подписи статусов (бейдж и фильтр). */
const STATUS_LABEL: Record<UserStatus, RuKey> = {
  active: 'adminUsers.statusActive',
  blocked: 'adminUsers.statusBlocked',
};

/** Подписи статусов записей: профильных ключей нет — переиспользуем courseStudents.* (как T30). */
const ENROLLMENT_STATUS_LABEL: Record<EnrollmentStatus, RuKey> = {
  active: 'courseStudents.statusActive',
  refunded: 'courseStudents.statusRefunded',
  revoked: 'courseStudents.statusRevoked',
};

/** Подписи статусов платежей: переиспользуем admin.status* (прецедент — Profile.tsx, T30). */
const PAYMENT_STATUS_LABEL: Record<PaymentStatus, RuKey> = {
  pending: 'admin.statusPending',
  succeeded: 'admin.statusSucceeded',
  failed: 'admin.statusFailed',
  refunded: 'admin.statusRefunded',
};

/** Класс бейджа по статусу записи на курс (цвета — только из au-badge--*). */
function enrollmentBadgeClass(status: EnrollmentStatus): string {
  if (status === 'active') return 'au-badge--active';
  if (status === 'revoked') return 'au-badge--blocked';
  return 'au-badge--neutral';
}

/** Класс бейджа по статусу платежа. */
function paymentBadgeClass(status: PaymentStatus): string {
  if (status === 'succeeded') return 'au-badge--active';
  if (status === 'failed') return 'au-badge--blocked';
  return 'au-badge--neutral';
}

/** Опасное действие над строкой, ожидающее инлайн-подтверждения (T52). */
type PendingConfirm =
  | { kind: 'remove'; id: string }
  | { kind: 'role'; id: string; next: Role }
  | { kind: 'status'; id: string; next: UserStatus };

// ---------- Утилиты ----------

/** Текст ошибки без краха страницы (схема из Admin.tsx). */
function errorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 403) return t('cabinet.needRole');
    if (error.status === 401) return t('error.needLogin');
    // 400/404/409 и прочие — текст бэкенда («Нельзя заблокировать последнего
    // администратора» и т.п.); если текста нет — общий.
    return error.message || t('common.error');
  }
  if (error instanceof TypeError) return t('error.network');
  return t('common.error');
}

/**
 * Текст ошибки секции сессий (T94). 404 — пользователя уже нет; 401/403
 * обрабатываются общим errorText (как и остальные запросы карточки).
 */
function sessionsErrorText(error: unknown): string {
  if (error instanceof ApiError && error.status === 404) {
    return error.message || t('error.notFound');
  }
  return errorText(error);
}

/** Точечно обновить строку пользователя во всех кэшированных страницах списка. */
function patchCachedUser(qc: QueryClient, id: string, patch: Partial<AdminUserRow>): void {
  qc.setQueriesData<AdminUserPage>({ queryKey: LIST_KEY }, (prev) => {
    if (!prev) return prev;
    return {
      ...prev,
      items: prev.items.map((u) => (u.id === id ? { ...u, ...patch } : u)),
    };
  });
}

/** Убрать строку из кэша сразу после успешного DELETE — без ожидания refetch. */
function removeCachedUser(qc: QueryClient, id: string): void {
  qc.setQueriesData<AdminUserPage>({ queryKey: LIST_KEY }, (prev) => {
    if (!prev) return prev;
    const items = prev.items.filter((u) => u.id !== id);
    if (items.length === prev.items.length) return prev;
    return { ...prev, items, total: Math.max(0, prev.total - 1) };
  });
}

// ---------- Карточка «Профиль пользователя» (T52) ----------

/**
 * Модальное окно с деталями пользователя: карточка, записи на курсы
 * (GET /admin/enrollments?studentId=…&limit=10), последние платежи
 * (GET /admin/payments?q=<email>&limit=10) и активные сессии
 * (GET /admin/users/{id}/sessions, T94). Монтируется только при открытии —
 * запросы уходят по факту показа. Закрытие: Esc, кнопка «Закрыть», клик по фону.
 */
function UserDetails({
  user,
  onClose,
  focusSessions = false,
}: {
  user: AdminUserRow;
  onClose: () => void;
  focusSessions?: boolean;
}): JSX.Element {
  const qc = useQueryClient();
  const sessionsSectionRef = useRef<HTMLElement | null>(null);

  // Esc закрывает окно, пока оно смонтировано.
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent): void {
      if (e.key === 'Escape') onClose();
    }
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [onClose]);

  // T94: действие «Принудительный выход» в строке открывает карточку сразу
  // на секции сессий, не заставляя прокручивать профиль вручную.
  useEffect(() => {
    if (!focusSessions) return;
    const node = sessionsSectionRef.current;
    if (!node) return;
    node.scrollIntoView({ block: 'start' });
    node.focus({ preventScroll: true });
  }, [focusSessions]);

  const enrollments = useQuery({
    queryKey: ['admin-user-enrollments', user.id],
    queryFn: () => listStudentEnrollments(user.id, DETAILS_LIMIT),
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  const payments = useQuery({
    queryKey: ['admin-user-payments', user.email],
    queryFn: () => listStudentPayments(user.email, DETAILS_LIMIT),
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  const sessions = useQuery({
    queryKey: ['admin-user-sessions', user.id],
    queryFn: () => listAdminUserSessions(user.id),
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  // T94: подтверждение отзыва всех сессий и сообщение об успехе — локальные,
  // живут только пока открыта карточка.
  const [confirmRevoke, setConfirmRevoke] = useState(false);
  const [sessionsNotice, setSessionsNotice] = useState('');

  const revokeSessions = useMutation({
    mutationFn: () => revokeAdminUserSessions(user.id),
    onSuccess: () => {
      setConfirmRevoke(false);
      setSessionsNotice(t('p10.sessions.revoked'));
      // Список пустеет сразу, затем подтверждаем состояние запросом.
      qc.setQueryData<AdminSessionsResponse>(['admin-user-sessions', user.id], { items: [] });
      void qc.invalidateQueries({ queryKey: ['admin-user-sessions', user.id] });
    },
    onError: () => {
      setConfirmRevoke(false);
      setSessionsNotice('');
    },
  });

  const enrollmentsItems = enrollments.data?.items ?? [];
  const paymentsItems = payments.data?.items ?? [];
  const sessionsItems = sessions.data?.items ?? [];

  return (
    <div className="au-modal-overlay" onClick={onClose}>
      <div
        className="au-modal"
        role="dialog"
        aria-modal="true"
        aria-label={t('adminUsers.detailsTitle')}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="au-modal-head">
          <h2>{t('adminUsers.detailsTitle')}</h2>
          <button type="button" className="btn btn-sm" onClick={onClose}>
            {t('common.close')}
          </button>
        </div>

        {/* ----- Основные поля пользователя ----- */}
        <dl className="au-details">
          <dt>{t('adminUsers.detailsId')}</dt>
          <dd className="au-details-mono">{user.id}</dd>
          <dt>{t('profile.email')}</dt>
          <dd className="au-details-mono">{user.email}</dd>
          <dt>{t('profile.displayName')}</dt>
          <dd>{user.displayName || '—'}</dd>
          <dt>{t('profile.role')}</dt>
          <dd>{t(ROLE_LABEL[user.role])}</dd>
          <dt>{t('adminUsers.colStatus')}</dt>
          <dd>
            <span className={`au-badge au-badge--${user.status}`}>
              {t(STATUS_LABEL[user.status])}
            </span>
          </dd>
          <dt>{t('profile.registered')}</dt>
          <dd>{formatDate(user.createdAt)}</dd>
        </dl>

        {/* ----- Записи на курсы ----- */}
        <section className="au-details-section" aria-label={t('adminUsers.detailsEnrollments')}>
          <h3>{t('adminUsers.detailsEnrollments')}</h3>
          {enrollments.isLoading && (
            <p className="muted" role="status">
              {t('common.loading')}
            </p>
          )}
          {enrollments.isError && (
            <p className="error" role="alert">
              {errorText(enrollments.error)}
            </p>
          )}
          {enrollments.data && enrollmentsItems.length === 0 && (
            <p className="muted">{t('adminUsers.detailsEmpty')}</p>
          )}
          {enrollmentsItems.length > 0 && (
            <ul className="au-details-list">
              {enrollmentsItems.map((en) => (
                <li key={en.id}>
                  <span className="au-details-item-name">{en.courseTitle || '—'}</span>
                  <span className={`au-badge ${enrollmentBadgeClass(en.status)}`}>
                    {t(ENROLLMENT_STATUS_LABEL[en.status])}
                  </span>
                  <span className="muted au-details-item-meta">
                    {t('profile.lessonsDone')}: {en.completedLessons}/{en.totalLessons}
                  </span>
                  <span className="muted au-details-item-meta">{formatDate(en.createdAt)}</span>
                </li>
              ))}
            </ul>
          )}
        </section>

        {/* ----- Последние платежи ----- */}
        <section className="au-details-section" aria-label={t('adminUsers.detailsPayments')}>
          <h3>{t('adminUsers.detailsPayments')}</h3>
          {payments.isLoading && (
            <p className="muted" role="status">
              {t('common.loading')}
            </p>
          )}
          {payments.isError && (
            <p className="error" role="alert">
              {errorText(payments.error)}
            </p>
          )}
          {payments.data && paymentsItems.length === 0 && (
            <p className="muted">{t('adminUsers.detailsEmpty')}</p>
          )}
          {paymentsItems.length > 0 && (
            <ul className="au-details-list">
              {paymentsItems.map((p) => (
                <li key={p.id}>
                  <span className="au-details-item-name">{p.courseTitle || '—'}</span>
                  <span className={`au-badge ${paymentBadgeClass(p.status)}`}>
                    {t(PAYMENT_STATUS_LABEL[p.status])}
                  </span>
                  <span className="au-details-item-meta">{formatMoney(p.amountCents)}</span>
                  <span className="muted au-details-item-meta">{formatDate(p.createdAt)}</span>
                </li>
              ))}
            </ul>
          )}
        </section>

        {/* ----- Активные сессии (T94) ----- */}
        <section
          ref={sessionsSectionRef}
          className="au-details-section au-sessions"
          aria-label={t('p10.sessions.title')}
          tabIndex={-1}
        >
          <h3>{t('p10.sessions.title')}</h3>
          <p className="muted au-sessions-hint">{t('p10.sessions.hint')}</p>

          {sessions.isLoading && (
            <p className="muted" role="status">
              {t('common.loading')}
            </p>
          )}
          {sessions.isFetching && !sessions.isLoading && (
            <p className="muted au-sessions-updating" role="status">
              {t('common.loading')}
            </p>
          )}
          {sessions.isError && (
            <div className="au-sessions-error" role="alert">
              <p className="error">{sessionsErrorText(sessions.error)}</p>
              <button
                type="button"
                className="btn btn-sm"
                onClick={() => void sessions.refetch()}
              >
                {t('common.retry')}
              </button>
            </div>
          )}
          {sessions.data && sessionsItems.length === 0 && !sessions.isFetching && (
            <p className="muted">{t('p10.sessions.empty')}</p>
          )}
          {sessionsItems.length > 0 && (
            <>
              <p className="muted au-sessions-count">
                {t('p10.sessions.count')}: {sessionsItems.length}
              </p>
              <ul className="au-details-list au-sessions-list" aria-live="polite">
                {sessionsItems.map((session) => (
                  <li key={session.id}>
                    <span className="au-details-item-name au-sessions-id">{session.id}</span>
                    <span className="muted au-details-item-meta">
                      {t('p10.sessions.createdAt')}: {formatDate(session.createdAt)}
                    </span>
                    <span className="muted au-details-item-meta">
                      {t('p10.sessions.expiresAt')}: {formatDate(session.expiresAt)}
                    </span>
                  </li>
                ))}
              </ul>
            </>
          )}

          {sessionsItems.length > 0 && (
            <div className="au-sessions-actions">
              {confirmRevoke ? (
                <>
                  <span className="au-confirm">{t('p10.sessions.confirmRevokeAll')}</span>
                  <button
                    type="button"
                    className="btn btn-sm au-btn--danger"
                    disabled={revokeSessions.isPending}
                    onClick={() => revokeSessions.mutate()}
                  >
                    {revokeSessions.isPending ? t('common.loading') : t('common.confirm')}
                  </button>
                  <button
                    type="button"
                    className="btn btn-sm"
                    disabled={revokeSessions.isPending}
                    onClick={() => setConfirmRevoke(false)}
                  >
                    {t('common.cancel')}
                  </button>
                </>
              ) : (
                <button
                  type="button"
                  className="btn btn-sm au-btn--danger"
                  onClick={() => {
                    setSessionsNotice('');
                    revokeSessions.reset();
                    setConfirmRevoke(true);
                  }}
                >
                  {t('p10.sessions.revokeAll')}
                </button>
              )}
            </div>
          )}

          {sessionsNotice && (
            <p className="au-notice au-sessions-notice" role="status" aria-live="polite">
              {sessionsNotice}
            </p>
          )}
          {revokeSessions.isError && (
            <p className="error au-sessions-error" role="alert">
              {sessionsErrorText(revokeSessions.error)}
            </p>
          )}
        </section>
      </div>
    </div>
  );
}

// ---------- Страница ----------

export default function AdminUsers(): JSX.Element {
  const qc = useQueryClient();

  // ---- Состояние фильтров и пагинации (локальное, без URL-параметров) ----
  const [searchInput, setSearchInput] = useState('');
  const [q, setQ] = useState('');
  const [role, setRole] = useState('');
  const [status, setStatus] = useState('');
  const [page, setPage] = useState(1);

  // Поиск с debounce: в запрос уходит «успокоившееся» значение, страница — на 1.
  useEffect(() => {
    const timer = window.setTimeout(() => {
      setQ(searchInput.trim());
      setPage(1);
    }, SEARCH_DEBOUNCE_MS);
    return () => window.clearTimeout(timer);
  }, [searchInput]);

  // ---- Список пользователей ----
  const users = useQuery({
    queryKey: [...LIST_KEY, { q, role, status, page }],
    queryFn: () =>
      listUsers({
        q: q || undefined,
        role: role || undefined,
        status: status || undefined,
        page,
        limit: PAGE_LIMIT,
      }),
    // 4xx — не временный сбой, ретраим только сетевые ошибки (как в Admin.tsx).
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  // ---- Локальные сообщения об операциях ----
  const [notice, setNotice] = useState(''); // успех последней мутации
  const [rowErrors, setRowErrors] = useState<Record<string, string>>({}); // ошибки по строкам
  // T52: строка, ждущая инлайн-подтверждения (удаление / смена роли / блокировка).
  const [confirm, setConfirm] = useState<PendingConfirm | null>(null);
  // T52: пользователь, открытый в карточке «Профиль» (null — окно закрыто).
  const [detailsUser, setDetailsUser] = useState<AdminUserRow | null>(null);
  // T94: карточку открыли действием «Принудительный выход» — сразу к сессиям.
  const [detailsFocusSessions, setDetailsFocusSessions] = useState(false);

  function openDetails(user: AdminUserRow, focusSessions: boolean): void {
    setDetailsFocusSessions(focusSessions);
    setDetailsUser(user);
  }

  function setRowError(id: string, text: string): void {
    setRowErrors((prev) => ({ ...prev, [id]: text }));
  }

  function clearRowError(id: string): void {
    setRowErrors((prev) => {
      if (!prev[id]) return prev;
      const next = { ...prev };
      delete next[id];
      return next;
    });
  }

  // ---- Мутации. Инвалидация — по префиксу ключа списка ----
  const roleMutation = useMutation({
    mutationFn: (args: { id: string; role: Role }) => setUserRole(args.id, args.role),
    onSuccess: (user, args) => {
      clearRowError(args.id);
      setConfirm(null);
      setNotice(t('adminUsers.roleChanged'));
      // Ответ применяем к кэшу сразу, чтобы select не «откатывался» до refetch.
      patchCachedUser(qc, args.id, { role: user?.role ?? args.role });
      void qc.invalidateQueries({ queryKey: LIST_KEY });
    },
    onError: (e: unknown, args) => {
      setConfirm(null);
      setNotice('');
      setRowError(args.id, errorText(e));
    },
  });

  const statusMutation = useMutation({
    mutationFn: (args: { id: string; status: UserStatus }) => setUserStatus(args.id, args.status),
    onSuccess: (user, args) => {
      clearRowError(args.id);
      setConfirm(null);
      setNotice(t('adminUsers.statusChanged'));
      patchCachedUser(qc, args.id, { status: user?.status ?? args.status });
      void qc.invalidateQueries({ queryKey: LIST_KEY });
    },
    onError: (e: unknown, args) => {
      setConfirm(null);
      setNotice('');
      setRowError(args.id, errorText(e));
    },
  });

  const removeMutation = useMutation({
    mutationFn: (id: string) => deleteUser(id),
    onSuccess: (_data, id) => {
      clearRowError(id);
      setConfirm(null);
      setNotice(t('adminUsers.removed'));
      removeCachedUser(qc, id);
      // Удалили последнюю строку страницы — возвращаемся на предыдущую.
      const data = users.data;
      if (data && data.items.length === 1 && page > 1) setPage(page - 1);
      void qc.invalidateQueries({ queryKey: LIST_KEY });
    },
    onError: (e: unknown, id) => {
      setConfirm(null);
      setNotice('');
      // 409 (есть курсы/платежи) и прочие ошибки — серверный текст (errorText).
      setRowError(id, errorText(e));
    },
  });

  // ---- Обработчики действий ----
  // T52: смена роли и блокировка больше не уходят на сервер сразу — сначала
  // инлайн-подтверждение в колонке действий (как у удаления в T26).
  function onRoleChange(user: AdminUserRow, next: Role): void {
    if (next === user.role) return;
    clearRowError(user.id);
    setNotice('');
    setConfirm({ kind: 'role', id: user.id, next });
  }

  function onToggleStatus(user: AdminUserRow): void {
    const next: UserStatus = user.status === 'blocked' ? 'active' : 'blocked';
    clearRowError(user.id);
    setNotice('');
    setConfirm({ kind: 'status', id: user.id, next });
  }

  function onRemove(user: AdminUserRow): void {
    clearRowError(user.id);
    setNotice('');
    setConfirm({ kind: 'remove', id: user.id });
  }

  /** Подтверждённое действие строки — запуск соответствующей мутации. */
  function onConfirmAction(user: AdminUserRow): void {
    if (!confirm || confirm.id !== user.id) return;
    if (confirm.kind === 'role') roleMutation.mutate({ id: user.id, role: confirm.next });
    else if (confirm.kind === 'status') statusMutation.mutate({ id: user.id, status: confirm.next });
    else removeMutation.mutate(user.id);
  }

  /** Текст подтверждения: старая → новая роль, блокировка или удаление. */
  function confirmText(pending: PendingConfirm, user: AdminUserRow): string {
    if (pending.kind === 'remove') return t('adminUsers.confirmRemove');
    if (pending.kind === 'role') {
      return `${t('adminUsers.confirmRole')} ${t(ROLE_LABEL[user.role])} → ${t(ROLE_LABEL[pending.next])}`;
    }
    return pending.next === 'blocked' ? t('adminUsers.confirmBlock') : t('adminUsers.confirmUnblock');
  }

  // ---- Состояния ожидания по строкам (для блокировки кнопок) ----
  const rolePendingId = roleMutation.isPending ? roleMutation.variables?.id : undefined;
  const statusPendingId = statusMutation.isPending ? statusMutation.variables?.id : undefined;
  const removePendingId = removeMutation.isPending ? removeMutation.variables : undefined;

  // ---- Выгрузка CSV: вся выборка по текущим фильтрам ----
  // Идём постранично (limit=100) до total, но не более 50 страниц: один клик
  // выгружает не больше 5000 строк, чтобы не подвесить браузер и стенд.
  // Если total первой страницы больше загруженного — выборка усечена (T43).
  async function loadExportRows(): Promise<{
    rows: Array<Array<string | number | null>>;
    truncated: boolean;
  }> {
    const rows: Array<Array<string | number | null>> = [];
    let total = 0;
    for (let pageNumber = 1; pageNumber <= EXPORT_MAX_PAGES; pageNumber += 1) {
      const res = await listUsers({
        q: q || undefined,
        role: role || undefined,
        status: status || undefined,
        page: pageNumber,
        limit: EXPORT_PAGE_LIMIT,
      });
      if (pageNumber === 1) total = res.total;
      for (const u of res.items) {
        rows.push([
          u.email,
          u.displayName,
          t(ROLE_LABEL[u.role]),
          t(STATUS_LABEL[u.status]),
          formatDate(u.createdAt),
          u.coursesCount,
          u.enrollmentsCount,
        ]);
      }
      // Неполная страница — данных больше нет.
      if (res.items.length < EXPORT_PAGE_LIMIT || rows.length >= res.total) break;
    }
    return { rows, truncated: exportTruncated(total, rows.length) };
  }

  // Заголовки CSV — существующие ключи таблицы; «Дата регистрации» — profile.registered.
  const exportHeaders = [
    t('adminUsers.colEmail'),
    t('adminUsers.colName'),
    t('adminUsers.colRole'),
    t('adminUsers.colStatus'),
    t('profile.registered'),
    t('adminUsers.colCourses'),
    t('adminUsers.colEnrollments'),
  ];

  // ---- Производные значения списка ----
  const data = users.data;
  const total = data?.total ?? 0;
  const rangeFrom = total === 0 ? 0 : (page - 1) * PAGE_LIMIT + 1;
  const rangeTo = Math.min(page * PAGE_LIMIT, total);
  const isLastPage = total > 0 && rangeTo >= total;
  // Пагинатор прячем только на пустой первой странице (как в Admin.tsx):
  // если данных не осталось, а страница не первая, кнопка «Назад» вернёт список.
  const showPager = !!data && (total > 0 || page > 1);

  return (
    <div className="wrap">
      <div className="page">
        <main className="au-admin">
          <p className="au-back">
            <Link to="/admin">{t('cabinet.adminTitle')}</Link>
          </p>
          <h1>{t('adminUsers.title')}</h1>

          <section className="panel">
            {/* ---------- Фильтры ---------- */}
            <div className="au-filters">
              <label className="au-field au-field--search" htmlFor="au-search">
                <span>{t('adminUsers.search')}</span>
                <input
                  id="au-search"
                  className="input"
                  type="search"
                  value={searchInput}
                  placeholder={t('adminUsers.search')}
                  onChange={(e) => setSearchInput(e.target.value)}
                />
              </label>
              <label className="au-field" htmlFor="au-filter-role">
                <span>{t('adminUsers.filterRole')}</span>
                <select
                  id="au-filter-role"
                  className="input au-select"
                  value={role}
                  onChange={(e) => {
                    setRole(e.target.value);
                    setPage(1);
                  }}
                >
                  <option value="">{t('adminUsers.all')}</option>
                  {ROLES.map((r) => (
                    <option key={r} value={r}>
                      {t(ROLE_LABEL[r])}
                    </option>
                  ))}
                </select>
              </label>
              <label className="au-field" htmlFor="au-filter-status">
                <span>{t('adminUsers.filterStatus')}</span>
                <select
                  id="au-filter-status"
                  className="input au-select"
                  value={status}
                  onChange={(e) => {
                    setStatus(e.target.value);
                    setPage(1);
                  }}
                >
                  <option value="">{t('adminUsers.all')}</option>
                  {STATUSES.map((s) => (
                    <option key={s} value={s}>
                      {t(STATUS_LABEL[s])}
                    </option>
                  ))}
                </select>
              </label>

              {/* T40: выгрузка всей выборки по текущим фильтрам (q, role, status). */}
              <CsvExportButton
                filename={`users-${csvDateStamp()}.csv`}
                headers={exportHeaders}
                loadRows={loadExportRows}
              />
            </div>

            {/* ---------- Итог по фильтру и уведомление об успехе ---------- */}
            {data && (
              <p className="au-total muted">
                {t('adminUsers.total')}: {total}
              </p>
            )}
            {notice && (
              <p className="au-notice" role="status">
                {notice}
              </p>
            )}

            {/* ---------- Состояния запроса ---------- */}
            {users.isLoading && (
              <div className="au-skeleton" role="status" aria-live="polite">
                <span className="au-skeleton-line" />
                <span className="au-skeleton-line" />
                <span className="au-skeleton-line" />
                <span className="sr-only">{t('common.loading')}</span>
              </div>
            )}

            {users.isError && (
              <div className="au-error" role="alert">
                <p className="error">{errorText(users.error)}</p>
                <button type="button" className="btn" onClick={() => void users.refetch()}>
                  {t('common.retry')}
                </button>
              </div>
            )}

            {data && data.items.length === 0 && <p className="muted">{t('adminUsers.empty')}</p>}

            {/* ---------- Таблица ---------- */}
            {data && data.items.length > 0 && (
              <div
                className="au-table-wrap"
                role="region"
                aria-label={t('adminUsers.title')}
                tabIndex={0}
              >
                <table className="au-table">
                  <thead>
                    <tr>
                      <th scope="col">{t('adminUsers.colEmail')}</th>
                      <th scope="col">{t('adminUsers.colName')}</th>
                      <th scope="col">{t('adminUsers.colRole')}</th>
                      <th scope="col">{t('adminUsers.colStatus')}</th>
                      <th scope="col">{t('adminUsers.colCreated')}</th>
                      <th scope="col">{t('adminUsers.colCourses')}</th>
                      <th scope="col">{t('adminUsers.colEnrollments')}</th>
                      <th scope="col" className="au-col-actions">
                        {t('adminUsers.colActions')}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.items.map((u) => {
                      const rowBusy =
                        rolePendingId === u.id ||
                        statusPendingId === u.id ||
                        removePendingId === u.id;
                      // Подтверждение, относящееся именно к этой строке (T52).
                      const rowConfirm = confirm && confirm.id === u.id ? confirm : null;
                      return (
                        <tr key={u.id}>
                          <td className="au-col-email" data-label={t('adminUsers.colEmail')}>
                            {u.email}
                          </td>
                          <td
                            className="au-col-name"
                            data-label={t('adminUsers.colName')}
                            title={u.displayName || undefined}
                          >
                            {/* T52: длинное имя усекается многоточием, полное — в title. */}
                            <span className="au-name-text">{u.displayName || '—'}</span>
                          </td>
                          <td data-label={t('adminUsers.colRole')}>
                            {/* Значение — роль с сервера; после подтверждения кэш обновляется. */}
                            <select
                              className="input au-role-select"
                              value={u.role}
                              disabled={rolePendingId === u.id}
                              aria-label={t('adminUsers.changeRole')}
                              onChange={(e) => onRoleChange(u, e.target.value as Role)}
                            >
                              {ROLES.map((r) => (
                                <option key={r} value={r}>
                                  {t(ROLE_LABEL[r])}
                                </option>
                              ))}
                            </select>
                          </td>
                          <td data-label={t('adminUsers.colStatus')}>
                            <span className={`au-badge au-badge--${u.status}`}>
                              {t(STATUS_LABEL[u.status])}
                            </span>
                          </td>
                          <td data-label={t('adminUsers.colCreated')}>{formatDate(u.createdAt)}</td>
                          <td data-label={t('adminUsers.colCourses')}>{u.coursesCount}</td>
                          <td data-label={t('adminUsers.colEnrollments')}>{u.enrollmentsCount}</td>
                          <td className="au-col-actions" data-label={t('adminUsers.colActions')}>
                            <div className="au-actions">
                              {rowConfirm ? (
                                <>
                                  <span className="au-confirm">{confirmText(rowConfirm, u)}</span>
                                  <button
                                    type="button"
                                    className={`btn btn-sm${
                                      rowConfirm.kind === 'remove' ? ' au-btn--danger' : ''
                                    }`}
                                    disabled={rowBusy}
                                    onClick={() => onConfirmAction(u)}
                                  >
                                    {t('common.confirm')}
                                  </button>
                                  <button
                                    type="button"
                                    className="btn btn-sm"
                                    disabled={rowBusy}
                                    onClick={() => setConfirm(null)}
                                  >
                                    {t('common.cancel')}
                                  </button>
                                </>
                              ) : (
                                <>
                                  <button
                                    type="button"
                                    className="btn btn-sm"
                                    onClick={() => openDetails(u, false)}
                                  >
                                    {t('adminUsers.open')}
                                  </button>
                                  <button
                                    type="button"
                                    className="btn btn-sm"
                                    onClick={() => openDetails(u, true)}
                                  >
                                    {t('p10.sessions.forceLogout')}
                                  </button>
                                  <button
                                    type="button"
                                    className="btn btn-sm"
                                    disabled={rowBusy}
                                    onClick={() => onToggleStatus(u)}
                                  >
                                    {u.status === 'blocked'
                                      ? t('adminUsers.unblock')
                                      : t('adminUsers.block')}
                                  </button>
                                  <button
                                    type="button"
                                    className="btn btn-sm au-btn--danger"
                                    disabled={rowBusy}
                                    onClick={() => onRemove(u)}
                                  >
                                    {t('adminUsers.remove')}
                                  </button>
                                </>
                              )}
                            </div>
                            {rowErrors[u.id] && (
                              <p className="error au-row-error" role="alert">
                                {rowErrors[u.id]}
                              </p>
                            )}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}

            {/* ---------- Пагинация ---------- */}
            {showPager && data && (
              <div className="au-pager">
                <button
                  type="button"
                  className="btn"
                  disabled={page <= 1 || users.isFetching}
                  onClick={() => setPage(page - 1)}
                >
                  {t('admin.prev')}
                </button>
                <span className="muted">
                  {t('adminUsers.showing')} {rangeFrom}–{rangeTo} {t('admin.of')} {total}
                </span>
                <button
                  type="button"
                  className="btn"
                  disabled={isLastPage || users.isFetching}
                  onClick={() => setPage(page + 1)}
                >
                  {t('admin.next')}
                </button>
              </div>
            )}
          </section>

          {/* ---------- Карточка «Профиль пользователя» (T52) ---------- */}
          {detailsUser && (
            <UserDetails
              user={detailsUser}
              focusSessions={detailsFocusSessions}
              onClose={() => {
                setDetailsUser(null);
                setDetailsFocusSessions(false);
              }}
            />
          )}
        </main>
      </div>
    </div>
  );
}
