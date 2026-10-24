// T90: глобальный журнал промокодов супер-админки (/admin/promo-codes).
// Данные — GET /admin/promo-codes и PATCH /admin/promo-codes/{id} (контракт T89,
// Task/00-api-contract.md, заморожен). Фильтры и страница живут в URL, поиск
// уходит с debounce; скидка/лимит/срок/активность редактируются прямо в строке
// таблицы, включение и отключение — через подтверждение. Все подписи — только
// существующие ключи p10.promo.*, p8.promo.*, common.* и admin.*; ru.json не
// меняется. Стили изолированы в styles/adminPromos.css (префикс .ap-).
import { Fragment, useEffect, useMemo, useRef, useState } from 'react';
import type { FormEvent } from 'react';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { QueryClient } from '@tanstack/react-query';
import { Link, useSearchParams } from 'react-router-dom';
import { ApiError, formatDate } from '../api/client';
import {
  listAdminCourses,
  listAdminPromoCodes,
  updateAdminPromoCode,
} from '../api/adminConsole';
import type {
  AdminPromoCode,
  AdminPromoCodePatch,
  AdminPromoCodesResponse,
} from '../api/adminConsole';
import { useSeo } from '../app/useSeo';
import { t } from '../i18n/t';
import '../styles/adminPromos.css';

// ---------- Константы ----------

/** Задержка поиска: запрос уходит после паузы в наборе (как в AdminUsers). */
const SEARCH_DEBOUNCE_MS = 300;

/** Размер страницы (контракт T89: limit 20..100, по умолчанию 20). */
const PAGE_LIMIT = 20;

/** Справочник курсов для фильтра: первые 100 (потолок limit) — как в T49. */
const COURSE_OPTIONS_LIMIT = 100;

/** Префикс ключа кэша списка: по нему инвалидируются все страницы с фильтрами. */
const LIST_KEY = ['admin-promo-codes'] as const;

/** Ключ кэша справочника курсов для select (не пересекается со списком). */
const COURSE_OPTIONS_KEY = ['admin-promo-course-options'] as const;

// ---------- Утилиты ----------

/** Текст ошибки без краха страницы (схема из AdminUsers/AdminEnrollments). */
function errorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 403) return t('cabinet.needRole');
    if (error.status === 401) return t('error.needLogin');
    // 400/404/409 и прочие — текст бэкенда («Курс в архиве…» и т.п.);
    // если текста нет — общий.
    return error.message || t('common.error');
  }
  if (error instanceof TypeError) return t('error.network');
  return t('common.error');
}

/** URL-параметр active: только true/false, иное значение игнорируем. */
function parseActive(value: string | null): boolean | undefined {
  if (value === 'true') return true;
  if (value === 'false') return false;
  return undefined;
}

/** Двузначное число для datetime-local (локальное время). */
function pad2(value: number): string {
  return String(value).padStart(2, '0');
}

/** RFC3339 → значение input[type=datetime-local] в локальной зоне. */
function toDateTimeLocal(iso: string | null): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}T${pad2(
    d.getHours(),
  )}:${pad2(d.getMinutes())}`;
}

/** datetime-local (локальное время) → RFC3339 UTC; некорректное — null. */
function dateTimeLocalToIso(value: string): string | null {
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return null;
  return d.toISOString();
}

/** Срок уже прошёл (для пометки «Истёк» рядом с датой). */
function isExpired(row: AdminPromoCode): boolean {
  if (!row.expiresAt) return false;
  const time = new Date(row.expiresAt).getTime();
  return !Number.isNaN(time) && time <= Date.now();
}

/** Поля строки, которые редактируются в форме. */
type EditForm = {
  discountPct: string;
  maxUses: string;
  expiresAt: string;
  active: boolean;
};

/** Начальные значения формы из строки списка. */
function formFromRow(row: AdminPromoCode): EditForm {
  return {
    discountPct: String(row.discountPct),
    maxUses: String(row.maxUses),
    expiresAt: toDateTimeLocal(row.expiresAt),
    active: row.active,
  };
}

/** Точечно обновить строку во всех кэшированных страницах списка. */
function patchCachedPromo(qc: QueryClient, id: string, promo: AdminPromoCode): void {
  qc.setQueriesData<AdminPromoCodesResponse>({ queryKey: LIST_KEY }, (prev) => {
    if (!prev) return prev;
    return {
      ...prev,
      items: prev.items.map((row) => (row.id === id ? promo : row)),
    };
  });
}

// ---------- Страница ----------

export default function AdminPromoCodes(): JSX.Element {
  useSeo('Промокоды — Edu Platform');

  const qc = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();

  // ---- Состояние выборки живёт в URL: ссылкой можно поделиться ----
  const q = searchParams.get('q') ?? '';
  const courseId = searchParams.get('courseId') ?? '';
  const activeFilter = parseActive(searchParams.get('active'));
  const pageParam = Number(searchParams.get('page'));
  const page = Number.isInteger(pageParam) && pageParam >= 1 ? pageParam : 1;

  // Свежие параметры URL для отложенных обработчиков (debounce): ссылка
  // обновляется эффектом, поэтому таймер пишет поверх актуального URL.
  const paramsRef = useRef(searchParams);
  useEffect(() => {
    paramsRef.current = searchParams;
  }, [searchParams]);

  /** Точечное обновление query-параметров: null/'' — удалить параметр. */
  function setParams(patch: Record<string, string | null>): void {
    const next = new URLSearchParams(paramsRef.current);
    for (const [key, value] of Object.entries(patch)) {
      if (value === null || value === '') next.delete(key);
      else next.set(key, value);
    }
    setSearchParams(next);
  }

  // ---- Текстовый поиск: ввод локально, в URL — с дебаунсом ----
  const [qInput, setQInput] = useState(q);

  // URL — источник истины: сброс фильтров и переходы по истории обновляют поле.
  useEffect(() => {
    setQInput(q);
  }, [q]);

  useEffect(() => {
    if (qInput.trim() === q) return;
    const timer = window.setTimeout(() => {
      setParams({ q: qInput.trim() || null, page: null });
    }, SEARCH_DEBOUNCE_MS);
    return () => window.clearTimeout(timer);
    // setParams читает актуальный URL через paramsRef — в зависимостях не нужен.
  }, [qInput, q]);

  // ---- Справочник курсов для фильтра ----
  const courses = useQuery({
    queryKey: COURSE_OPTIONS_KEY,
    queryFn: () => listAdminCourses({ limit: COURSE_OPTIONS_LIMIT }),
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  // Выбранный курс должен быть виден в select, даже если его нет среди первых
  // 100 — иначе фильтр «невидим» и его нельзя снять (как в AdminEnrollments).
  const courseOptions = useMemo<Array<{ id: string; title: string }>>(() => {
    const items = (courses.data?.items ?? []).map((c) => ({ id: c.id, title: c.title }));
    items.sort((a, b) => a.title.localeCompare(b.title, 'ru'));
    if (courseId !== '' && !items.some((c) => c.id === courseId)) {
      items.unshift({ id: courseId, title: courseId });
    }
    return items;
  }, [courses.data, courseId]);

  // ---- Список промокодов ----
  const promos = useQuery({
    queryKey: [...LIST_KEY, { q, courseId, active: activeFilter, page }],
    queryFn: () =>
      listAdminPromoCodes({
        q: q || undefined,
        courseId: courseId || undefined,
        active: activeFilter,
        page,
        limit: PAGE_LIMIT,
      }),
    // При смене страницы/фильтра старые строки остаются до прихода новых.
    placeholderData: keepPreviousData,
    // 4xx — не временный сбой, ретраим только сетевые ошибки (как в Admin.tsx).
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  // ---- Локальные сообщения и состояние редактирования ----
  const [notice, setNotice] = useState('');
  const [rowErrors, setRowErrors] = useState<Record<string, string>>({});
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editForm, setEditForm] = useState<EditForm | null>(null);
  const [editError, setEditError] = useState('');

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

  // Смена фильтра/страницы убирает строку из выборки — закрываем редактор,
  // чтобы форма не «повисала» над чужими данными.
  useEffect(() => {
    setEditingId(null);
    setEditForm(null);
    setEditError('');
  }, [q, courseId, activeFilter, page]);

  // ---- Мутации: сохранение формы и быстрое включение/отключение ----
  const saveMutation = useMutation({
    mutationFn: (args: { id: string; patch: AdminPromoCodePatch }) =>
      updateAdminPromoCode(args.id, args.patch),
    onSuccess: (promo, args) => {
      setNotice(t('p10.promo.updated'));
      setEditError('');
      setEditingId(null);
      setEditForm(null);
      // Ответ применяем к кэшу сразу, чтобы строка не «откатывалась» до refetch.
      patchCachedPromo(qc, args.id, promo);
      void qc.invalidateQueries({ queryKey: LIST_KEY });
    },
    onError: (e: unknown) => {
      setNotice('');
      setEditError(errorText(e));
    },
  });

  const toggleMutation = useMutation({
    mutationFn: (args: { id: string; active: boolean }) =>
      updateAdminPromoCode(args.id, { active: args.active }),
    onSuccess: (promo, args) => {
      clearRowError(args.id);
      setNotice(t('p10.promo.updated'));
      patchCachedPromo(qc, args.id, promo);
      void qc.invalidateQueries({ queryKey: LIST_KEY });
    },
    onError: (e: unknown, args) => {
      setNotice('');
      setRowError(args.id, errorText(e));
    },
  });

  // ---- Обработчики ----
  function startEdit(row: AdminPromoCode): void {
    setNotice('');
    setEditError('');
    clearRowError(row.id);
    setEditingId(row.id);
    setEditForm(formFromRow(row));
  }

  function cancelEdit(): void {
    setEditingId(null);
    setEditForm(null);
    setEditError('');
  }

  function onToggle(row: AdminPromoCode): void {
    const next = !row.active;
    const question = next
      ? t('p10.promo.confirmActivate')
      : t('p10.promo.confirmDeactivate');
    // Ключи словаря без интерполяции — код подставляем сами второй строкой.
    if (!window.confirm(`${question}\n${row.code}`)) return;
    clearRowError(row.id);
    setNotice('');
    toggleMutation.mutate({ id: row.id, active: next });
  }

  function submitEdit(e: FormEvent<HTMLFormElement>): void {
    e.preventDefault();
    if (!editingId || !editForm) return;

    const discountPct = Number(editForm.discountPct);
    if (
      editForm.discountPct.trim() === '' ||
      !Number.isInteger(discountPct) ||
      discountPct < 1 ||
      discountPct > 90
    ) {
      setEditError(t('p8.promo.discountInvalid'));
      return;
    }

    const row = promos.data?.items.find((item) => item.id === editingId);
    const usedCount = row?.usedCount ?? 0;
    const maxUses = Number(editForm.maxUses);
    if (editForm.maxUses.trim() === '' || !Number.isInteger(maxUses) || maxUses < 0) {
      setEditError(t('p8.promo.maxUsesInvalid'));
      return;
    }
    if (maxUses < usedCount) {
      // Словарь не умеет интерполяцию: показываем уже использованное значение
      // рядом с общим текстом ошибки лимита.
      setEditError(
        `${t('p8.promo.maxUsesInvalid')} (${t('p10.promo.colUses')}: ${usedCount})`,
      );
      return;
    }

    // Срок проверяем только если его действительно меняли: у истёкшего кода
    // дата в прошлом, но это не мешает править скидку или лимит — неизменённое
    // поле в PATCH не попадает (контракт требует срок в будущем).
    const expiresRaw = editForm.expiresAt.trim();
    const originalExpires = row ? toDateTimeLocal(row.expiresAt) : '';
    const expiresChanged = expiresRaw !== originalExpires;
    const expiresAt = expiresRaw === '' ? null : dateTimeLocalToIso(expiresRaw);
    if (expiresChanged && expiresRaw !== '' && expiresAt === null) {
      setEditError(t('p8.promo.expiresInvalid'));
      return;
    }
    if (
      expiresChanged &&
      expiresAt !== null &&
      new Date(expiresAt).getTime() <= Date.now()
    ) {
      setEditError(t('p8.promo.expiresInvalid'));
      return;
    }

    // В тело уходят только изменённые поля: частичный PATCH не трогает
    // протухший срок и не провоцирует лишний 400 на неизменённых данных.
    const patch: AdminPromoCodePatch = {};
    if (row) {
      if (discountPct !== row.discountPct) patch.discountPct = discountPct;
      if (maxUses !== row.maxUses) patch.maxUses = maxUses;
      if (expiresChanged) patch.expiresAt = expiresAt;
      if (editForm.active !== row.active) patch.active = editForm.active;
    } else {
      patch.discountPct = discountPct;
      patch.maxUses = maxUses;
      patch.expiresAt = expiresAt;
      patch.active = editForm.active;
    }

    setEditError('');
    if (Object.keys(patch).length === 0) {
      cancelEdit();
      return;
    }
    saveMutation.mutate({ id: editingId, patch });
  }

  function resetFilters(): void {
    setQInput('');
    setParams({ q: null, courseId: null, active: null, page: null });
  }

  // ---- Производные значения ----
  const data = promos.data;
  const total = data?.total ?? 0;
  const rangeFrom = total === 0 ? 0 : (page - 1) * PAGE_LIMIT + 1;
  const rangeTo = Math.min(page * PAGE_LIMIT, total);
  const isLastPage = total > 0 && rangeTo >= total;
  const showPager = !!data && (total > 0 || page > 1);
  const togglePendingId = toggleMutation.isPending
    ? toggleMutation.variables?.id
    : undefined;

  return (
    <div className="wrap">
      <div className="page">
        <main className="ap-admin">
          <p className="ap-back">
            <Link to="/admin">{t('p10.nav.backToAdmin')}</Link>
          </p>
          <h1>{t('p10.promo.title')}</h1>
          <p className="muted ap-subtitle">{t('p10.promo.subtitle')}</p>

          <section className="panel">
            {/* ---------- Фильтры ---------- */}
            <div className="ap-filters">
              <label className="ap-field ap-field--search" htmlFor="ap-search">
                <span>{t('p10.promo.search')}</span>
                <input
                  id="ap-search"
                  className="input"
                  type="search"
                  value={qInput}
                  placeholder={t('p10.promo.search')}
                  onChange={(e) => setQInput(e.target.value)}
                />
              </label>

              <label className="ap-field" htmlFor="ap-filter-course">
                <span>{t('p10.promo.filterCourse')}</span>
                <select
                  id="ap-filter-course"
                  className="input ap-select"
                  value={courseId}
                  onChange={(e) =>
                    setParams({ courseId: e.target.value || null, page: null })
                  }
                >
                  <option value="">{t('p10.promo.all')}</option>
                  {courseOptions.map((course) => (
                    <option key={course.id} value={course.id}>
                      {course.title}
                    </option>
                  ))}
                </select>
              </label>

              <label className="ap-field" htmlFor="ap-filter-status">
                <span>{t('p10.promo.filterStatus')}</span>
                <select
                  id="ap-filter-status"
                  className="input ap-select"
                  value={activeFilter === undefined ? '' : String(activeFilter)}
                  onChange={(e) =>
                    setParams({ active: e.target.value || null, page: null })
                  }
                >
                  <option value="">{t('p10.promo.all')}</option>
                  <option value="true">{t('p10.promo.active')}</option>
                  <option value="false">{t('p10.promo.inactive')}</option>
                </select>
              </label>

              <button type="button" className="btn ap-reset" onClick={resetFilters}>
                {t('admin.resetFilters')}
              </button>
            </div>

            {/* ---------- Итог, уведомление, состояния запроса ---------- */}
            {data && (
              <p className="ap-total muted">
                {t('admin.showing')} {rangeFrom}–{rangeTo} {t('admin.of')} {total}
                {promos.isFetching && (
                  <span className="ap-fetching"> · {t('common.loading')}</span>
                )}
              </p>
            )}
            {notice && (
              <p className="ap-notice" role="status" aria-live="polite">
                {notice}
              </p>
            )}

            {promos.isLoading && (
              <div className="ap-skeleton" role="status" aria-live="polite">
                <span className="ap-skeleton-line" />
                <span className="ap-skeleton-line" />
                <span className="ap-skeleton-line" />
                <span className="sr-only">{t('common.loading')}</span>
              </div>
            )}

            {promos.isError && (
              <div className="ap-error" role="alert">
                <p className="error">{errorText(promos.error)}</p>
                <button type="button" className="btn" onClick={() => void promos.refetch()}>
                  {t('common.retry')}
                </button>
              </div>
            )}

            {data && data.items.length === 0 && (
              <p className="muted ap-empty">{t('p10.promo.empty')}</p>
            )}

            {/* ---------- Таблица ---------- */}
            {data && data.items.length > 0 && (
              <div
                className="ap-table-wrap"
                role="region"
                aria-label={t('p10.promo.title')}
                aria-busy={promos.isFetching}
                tabIndex={0}
              >
                <table className="ap-table">
                  <thead>
                    <tr>
                      <th scope="col">{t('p10.promo.colCode')}</th>
                      <th scope="col">{t('p10.promo.colCourse')}</th>
                      <th scope="col">{t('p10.promo.colDiscount')}</th>
                      <th scope="col">{t('p10.promo.colUses')}</th>
                      <th scope="col">{t('p10.promo.colExpires')}</th>
                      <th scope="col">{t('p10.promo.colStatus')}</th>
                      <th scope="col" className="ap-col-actions">
                        {t('p10.promo.colActions')}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.items.map((row) => {
                      const editing = editingId === row.id && editForm !== null;
                      const rowBusy = togglePendingId === row.id;
                      const expired = isExpired(row);
                      return (
                        <Fragment key={row.id}>
                          <tr>
                            <td data-label={t('p10.promo.colCode')}>
                              <span className="ap-code">{row.code}</span>
                            </td>
                            <td data-label={t('p10.promo.colCourse')}>
                              {row.courseSlug ? (
                                <Link
                                  className="ap-course-link"
                                  to={`/courses/${encodeURIComponent(row.courseSlug)}`}
                                  target="_blank"
                                  rel="noreferrer"
                                  title={t('p10.promo.openCourse')}
                                >
                                  {row.courseTitle || row.courseSlug}
                                </Link>
                              ) : (
                                <span>{row.courseTitle || '—'}</span>
                              )}
                            </td>
                            <td data-label={t('p10.promo.colDiscount')}>
                              {row.discountPct}%
                            </td>
                            <td data-label={t('p10.promo.colUses')}>
                              {row.maxUses > 0
                                ? `${row.usedCount} ${t('p10.promo.usedOf')} ${row.maxUses}`
                                : `${row.usedCount} / ${t('p8.promo.noLimit')}`}
                            </td>
                            <td data-label={t('p10.promo.colExpires')}>
                              <span className="ap-expires">
                                {row.expiresAt
                                  ? formatDate(row.expiresAt)
                                  : t('p10.promo.never')}
                                {expired && (
                                  <span className="ap-badge ap-badge--expired">
                                    {t('p8.promo.expired')}
                                  </span>
                                )}
                              </span>
                            </td>
                            <td data-label={t('p10.promo.colStatus')}>
                              <span
                                className={`ap-badge ap-badge--${
                                  row.active ? 'active' : 'inactive'
                                }`}
                              >
                                {row.active
                                  ? t('p10.promo.active')
                                  : t('p10.promo.inactive')}
                              </span>
                            </td>
                            <td
                              className="ap-col-actions"
                              data-label={t('p10.promo.colActions')}
                            >
                              <div className="ap-actions">
                                <button
                                  type="button"
                                  className="btn btn-sm"
                                  disabled={editing || rowBusy}
                                  aria-label={`${t('p10.promo.edit')}: ${row.code}`}
                                  onClick={() => startEdit(row)}
                                >
                                  {t('p10.promo.edit')}
                                </button>
                                <button
                                  type="button"
                                  className={`btn btn-sm${
                                    row.active ? ' ap-btn--danger' : ''
                                  }`}
                                  disabled={rowBusy || saveMutation.isPending}
                                  aria-label={`${
                                    row.active
                                      ? t('p10.promo.deactivate')
                                      : t('p10.promo.activate')
                                  }: ${row.code}`}
                                  onClick={() => onToggle(row)}
                                >
                                  {rowBusy
                                    ? t('common.loading')
                                    : row.active
                                      ? t('p10.promo.deactivate')
                                      : t('p10.promo.activate')}
                                </button>
                              </div>
                              {rowErrors[row.id] && (
                                <p className="error ap-row-error" role="alert">
                                  {rowErrors[row.id]}
                                </p>
                              )}
                            </td>
                          </tr>

                          {/* ---------- Инлайн-редактор строки ---------- */}
                          {editing && (
                            <tr className="ap-edit-row">
                              <td colSpan={7} className="ap-edit-cell">
                                <form
                                  className="ap-edit-form"
                                  aria-label={t('p10.promo.editTitle')}
                                  onSubmit={submitEdit}
                                >
                                  <h2 className="ap-edit-title">
                                    {t('p10.promo.editTitle')}:{' '}
                                    <span className="ap-code">{row.code}</span>
                                  </h2>

                                  <div className="ap-edit-fields">
                                    <label className="ap-edit-field" htmlFor="ap-edit-discount">
                                      <span>{t('p8.promo.discount')}</span>
                                      <input
                                        id="ap-edit-discount"
                                        className="input"
                                        type="number"
                                        min={1}
                                        max={90}
                                        step={1}
                                        required
                                        value={editForm.discountPct}
                                        disabled={saveMutation.isPending}
                                        onChange={(e) =>
                                          setEditForm({
                                            ...editForm,
                                            discountPct: e.target.value,
                                          })
                                        }
                                      />
                                    </label>

                                    <label className="ap-edit-field" htmlFor="ap-edit-max-uses">
                                      <span>{t('p8.promo.maxUses')}</span>
                                      <input
                                        id="ap-edit-max-uses"
                                        className="input"
                                        type="number"
                                        min={0}
                                        step={1}
                                        value={editForm.maxUses}
                                        disabled={saveMutation.isPending}
                                        onChange={(e) =>
                                          setEditForm({
                                            ...editForm,
                                            maxUses: e.target.value,
                                          })
                                        }
                                      />
                                      <span className="ap-edit-hint muted">
                                        {t('p8.promo.maxUsesHint')}
                                      </span>
                                    </label>

                                    <label className="ap-edit-field" htmlFor="ap-edit-expires">
                                      <span>{t('p8.promo.expiresAt')}</span>
                                      <input
                                        id="ap-edit-expires"
                                        className="input"
                                        type="datetime-local"
                                        value={editForm.expiresAt}
                                        disabled={saveMutation.isPending}
                                        onChange={(e) =>
                                          setEditForm({
                                            ...editForm,
                                            expiresAt: e.target.value,
                                          })
                                        }
                                      />
                                      <span className="ap-edit-hint muted">
                                        {t('p8.promo.expiresHint')}
                                      </span>
                                    </label>

                                    <label className="ap-edit-check" htmlFor="ap-edit-active">
                                      <input
                                        id="ap-edit-active"
                                        type="checkbox"
                                        checked={editForm.active}
                                        disabled={saveMutation.isPending}
                                        onChange={(e) =>
                                          setEditForm({
                                            ...editForm,
                                            active: e.target.checked,
                                          })
                                        }
                                      />
                                      <span>{t('p8.promo.active')}</span>
                                    </label>
                                  </div>

                                  {editError && (
                                    <p className="error ap-edit-error" role="alert">
                                      {editError}
                                    </p>
                                  )}

                                  <div className="ap-edit-actions">
                                    <button
                                      type="submit"
                                      className="btn btn-primary"
                                      disabled={saveMutation.isPending}
                                    >
                                      {saveMutation.isPending
                                        ? t('common.loading')
                                        : t('p10.promo.save')}
                                    </button>
                                    <button
                                      type="button"
                                      className="btn"
                                      disabled={saveMutation.isPending}
                                      onClick={cancelEdit}
                                    >
                                      {t('p10.promo.cancel')}
                                    </button>
                                  </div>
                                </form>
                              </td>
                            </tr>
                          )}
                        </Fragment>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}

            {/* ---------- Пагинация ---------- */}
            {showPager && data && (
              <div className="ap-pager">
                <button
                  type="button"
                  className="btn"
                  disabled={page <= 1 || promos.isFetching}
                  onClick={() => setParams({ page: page <= 2 ? null : String(page - 1) })}
                >
                  {t('admin.prev')}
                </button>
                <span className="muted">
                  {t('admin.showing')} {rangeFrom}–{rangeTo} {t('admin.of')} {total}
                </span>
                <button
                  type="button"
                  className="btn"
                  disabled={isLastPage || promos.isFetching}
                  onClick={() => setParams({ page: String(page + 1) })}
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
