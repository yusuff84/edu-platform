// T26: раздел «Пользователи» супер-админки.
// Таблица с поиском (debounce), фильтрами роль/статус, пагинацией и мутациями
// смены роли / блокировки / удаления. Контракт backend — T24.
// Стили изолированы в styles/adminUsers.css; styles.css и admin.css не изменяются.
import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { QueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { ApiError, formatDate } from '../api/client';
import { deleteUser, listUsers, setUserRole, setUserStatus } from '../api/adminUsers';
import type { AdminUserPage, AdminUserRow, UserStatus } from '../api/adminUsers';
import type { Role } from '../api/types';
import { t } from '../i18n/t';
import type { RuKey } from '../i18n/t';
import CsvExportButton, { csvDateStamp, exportTruncated } from '../components/CsvExportButton';
import '../styles/adminUsers.css';

// ---------- Константы ----------

/** Задержка поиска: запрос уходит после паузы в наборе (ТЗ T26 — ~300 мс). */
const SEARCH_DEBOUNCE_MS = 300;

/** Размер страницы (контракт T24: limit=20). */
const PAGE_LIMIT = 20;

/** Размер страницы выгрузки CSV (T40): 100 строк за запрос. */
const EXPORT_PAGE_LIMIT = 100;

/** Потолок выгрузки CSV: не более 50 страниц (5000 строк) за один клик. */
const EXPORT_MAX_PAGES = 50;

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
  const [confirmId, setConfirmId] = useState(''); // строка, ждущая подтверждения удаления

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
      setNotice(t('adminUsers.roleChanged'));
      // Ответ применяем к кэшу сразу, чтобы select не «откатывался» до refetch.
      patchCachedUser(qc, args.id, { role: user?.role ?? args.role });
      void qc.invalidateQueries({ queryKey: LIST_KEY });
    },
    onError: (e: unknown, args) => {
      setNotice('');
      setRowError(args.id, errorText(e));
    },
  });

  const statusMutation = useMutation({
    mutationFn: (args: { id: string; status: UserStatus }) => setUserStatus(args.id, args.status),
    onSuccess: (user, args) => {
      clearRowError(args.id);
      setNotice(t('adminUsers.statusChanged'));
      patchCachedUser(qc, args.id, { status: user?.status ?? args.status });
      void qc.invalidateQueries({ queryKey: LIST_KEY });
    },
    onError: (e: unknown, args) => {
      setNotice('');
      setRowError(args.id, errorText(e));
    },
  });

  const removeMutation = useMutation({
    mutationFn: (id: string) => deleteUser(id),
    onSuccess: (_data, id) => {
      clearRowError(id);
      setConfirmId('');
      setNotice(t('adminUsers.removed'));
      removeCachedUser(qc, id);
      // Удалили последнюю строку страницы — возвращаемся на предыдущую.
      const data = users.data;
      if (data && data.items.length === 1 && page > 1) setPage(page - 1);
      void qc.invalidateQueries({ queryKey: LIST_KEY });
    },
    onError: (e: unknown, id) => {
      setConfirmId('');
      setNotice('');
      setRowError(id, errorText(e));
    },
  });

  // ---- Обработчики действий ----
  function onRoleChange(user: AdminUserRow, next: Role): void {
    if (next === user.role) return;
    clearRowError(user.id);
    setNotice('');
    roleMutation.mutate({ id: user.id, role: next });
  }

  function onToggleStatus(user: AdminUserRow): void {
    const next: UserStatus = user.status === 'blocked' ? 'active' : 'blocked';
    clearRowError(user.id);
    setNotice('');
    statusMutation.mutate({ id: user.id, status: next });
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
                      <th scope="col">{t('adminUsers.colActions')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.items.map((u) => {
                      const rowBusy =
                        rolePendingId === u.id ||
                        statusPendingId === u.id ||
                        removePendingId === u.id;
                      return (
                        <tr key={u.id}>
                          <td data-label={t('adminUsers.colEmail')}>{u.email}</td>
                          <td data-label={t('adminUsers.colName')}>{u.displayName || '—'}</td>
                          <td data-label={t('adminUsers.colRole')}>
                            {/* Значение — роль с сервера; после PATCH кэш обновляется. */}
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
                          <td data-label={t('adminUsers.colActions')}>
                            <div className="au-actions">
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
                              {confirmId === u.id ? (
                                <>
                                  <span className="au-confirm">{t('adminUsers.confirmRemove')}</span>
                                  <button
                                    type="button"
                                    className="btn btn-sm au-btn--danger"
                                    disabled={removePendingId === u.id}
                                    onClick={() => removeMutation.mutate(u.id)}
                                  >
                                    {t('common.confirm')}
                                  </button>
                                  <button
                                    type="button"
                                    className="btn btn-sm"
                                    disabled={removePendingId === u.id}
                                    onClick={() => setConfirmId('')}
                                  >
                                    {t('common.cancel')}
                                  </button>
                                </>
                              ) : (
                                <button
                                  type="button"
                                  className="btn btn-sm au-btn--danger"
                                  disabled={rowBusy}
                                  onClick={() => {
                                    setNotice('');
                                    setConfirmId(u.id);
                                  }}
                                >
                                  {t('adminUsers.remove')}
                                </button>
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
        </main>
      </div>
    </div>
  );
}
