import { type FormEvent, useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link, useSearchParams } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import {
  ApiError,
  createInvite,
  formatDate,
  formatMoney,
  formatPrice,
  getAdminSettings,
  moderateCourse,
  paymentJournal,
  reviewQueue,
  updateAdminSettings,
} from '../api/client';
import type { Invite, ModerationDecision, PaymentJournalItem, PaymentStatus } from '../api/client';
import { t } from '../i18n/t';
import type { RuKey } from '../i18n/t';
// Стили страницы изолированы в styles/admin.css — общий styles.css не трогаем.
import '../styles/admin.css';
// T43: стиль предупреждения об усечении выгрузки (.ce-msg--warn) — из csvExport.css.
import '../styles/csvExport.css';

// ---------- Константы и утилиты ----------

// Сообщения zod-валидации: ключей ru.json под них нет (та же ситуация,
// что в Auth.tsx/Constructor.tsx/Reviews.tsx) — см. отчёт T08.
const inviteSchema = z.object({
  email: z.string().min(1, 'Укажите email').email('Некорректный email'),
});

type InviteForm = z.infer<typeof inviteSchema>;

/** Статусы платежей в порядке контракта (00-api-contract.md, US6). */
const PAYMENT_STATUSES: PaymentStatus[] = ['pending', 'succeeded', 'failed', 'refunded'];

/** Русские подписи статусов — ключи из словаря T02. */
const STATUS_LABEL: Record<PaymentStatus, RuKey> = {
  pending: 'admin.statusPending',
  succeeded: 'admin.statusSucceeded',
  failed: 'admin.statusFailed',
  refunded: 'admin.statusRefunded',
};

/** Значение limit по умолчанию (контракт: 20, максимум 100). */
const DEFAULT_LIMIT = 20;

/** Размер страницы выгрузки CSV: контракт разрешает limit до 100. */
const EXPORT_PAGE_SIZE = 100;

/** Предохранитель выгрузки: не больше 50 страниц = 5000 строк (см. отчёт T38). */
const EXPORT_MAX_PAGES = 50;

function isPaymentStatus(value: string | null): value is PaymentStatus {
  return value !== null && (PAYMENT_STATUSES as readonly string[]).includes(value);
}

/**
 * Короткий идентификатор для таблицы.
 */
function shortId(id: string): string {
  return id.length > 8 ? id.slice(0, 8) : id;
}

/** Текст ошибки без краха страницы: 403/401/400/сеть/прочее. */
function errorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 403) return t('cabinet.needRole');
    if (error.status === 401) return t('error.needLogin');
    if (error.status === 400) return error.message || t('common.error');
    return error.message || t('common.error');
  }
  if (error instanceof TypeError) return t('error.network');
  return t('common.error');
}

/** Копирование в буфер обмена: Clipboard API, иначе запасной способ. */
async function copyToClipboard(value: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value);
      return true;
    }
  } catch {
    // Нет прав/небезопасный контекст — пробуем запасной способ.
  }
  try {
    const area = document.createElement('textarea');
    area.value = value;
    area.style.position = 'fixed';
    area.style.opacity = '0';
    document.body.appendChild(area);
    area.select();
    const ok = document.execCommand('copy');
    area.remove();
    return ok;
  } catch {
    return false;
  }
}

// ---------- Выгрузка журнала в CSV ----------

/** Сегодняшняя локальная дата как ГГГГ-ММ-ДД — для имени файла выгрузки. */
function todayIsoDate(): string {
  const now = new Date();
  const month = String(now.getMonth() + 1).padStart(2, '0');
  const day = String(now.getDate()).padStart(2, '0');
  return `${now.getFullYear()}-${month}-${day}`;
}

/** Ячейка CSV: значение всегда в кавычках, внутренние кавычки удваиваются. */
function csvCell(value: string): string {
  return `"${value.replace(/"/g, '""')}"`;
}

/**
 * CSV выборки журнала: разделитель «;» (русский Excel), заголовки — из словаря,
 * порядок колонок совпадает с таблицей на странице.
 */
function paymentsCsv(rows: PaymentJournalItem[]): string {
  const header = [
    t('admin.colDate'),
    t('admin.colStudent'),
    t('admin.colCourse'),
    t('admin.colAmount'),
    t('admin.colCommission'),
    t('admin.colTeacher'),
    t('admin.colProvider'),
    t('admin.colStatus'),
    t('admin.colPaymentId'),
  ];
  const lines = rows.map((item) => [
    formatDate(item.createdAt),
    // Как в таблице: имя → email → короткий id.
    item.studentName || item.studentEmail || shortId(item.studentId),
    item.courseTitle,
    formatMoney(item.amountCents),
    formatMoney(item.platformFeeCents),
    formatMoney(item.teacherAmountCents),
    item.provider,
    t(STATUS_LABEL[item.status]),
    item.providerPaymentId,
  ]);
  // CRLF — привычный для Excel разделитель строк.
  return [header, ...lines].map((cells) => cells.map(csvCell).join(';')).join('\r\n');
}

/** Скачивание текста как файла; CSV собирается вручную через Blob (новых зависимостей нет). */
function downloadTextFile(fileName: string, content: string, mimeType: string): void {
  const url = URL.createObjectURL(new Blob([content], { type: mimeType }));
  const link = document.createElement('a');
  link.href = url;
  link.download = fileName;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

// ---------- Страница ----------

export default function Admin(): JSX.Element {
  const qc = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();

  // ---- URL-состояние журнала: page/limit/status/courseId (ссылка шарится) ----
  const pageParam = Number(searchParams.get('page'));
  const page = Number.isInteger(pageParam) && pageParam >= 1 ? pageParam : 1;
  const limitParam = Number(searchParams.get('limit'));
  const limit =
    Number.isInteger(limitParam) && limitParam >= 1 && limitParam <= 100
      ? limitParam
      : DEFAULT_LIMIT;
  const statusParam = searchParams.get('status');
  const status: PaymentStatus | undefined = isPaymentStatus(statusParam) ? statusParam : undefined;
  const courseId = searchParams.get('courseId') ?? undefined;
  // Поиск по студенту и период живут в URL — ссылка шарится, как status/courseId.
  const q = searchParams.get('q') ?? '';
  const from = searchParams.get('from') ?? '';
  const to = searchParams.get('to') ?? '';
  // Границы формата ГГГГ-ММ-ДД сравниваются лексикографически = хронологически.
  // Неверный период (начало позже конца): запрос не уходит, показываем admin.periodInvalid.
  const periodInvalid = from !== '' && to !== '' && from > to;

  /** Точечное обновление query-параметров: null — удалить параметр. */
  function setParams(patch: Record<string, string | null>): void {
    const next = new URLSearchParams(searchParams);
    for (const [key, value] of Object.entries(patch)) {
      if (value === null || value === '') next.delete(key);
      else next.set(key, value);
    }
    setSearchParams(next);
  }

  // Поле поиска правится локально, в URL попадает по Enter или кнопке «Найти» —
  // иначе каждый введённый символ дёргал бы запрос.
  const [searchInput, setSearchInput] = useState(q);
  useEffect(() => {
    // URL — источник истины: сброс фильтров и переходы по истории обновляют поле.
    setSearchInput(q);
  }, [q]);

  /** Применение поиска из формы (Enter или кнопка): значение — в URL, страница на первую. */
  function onSearchSubmit(e: FormEvent<HTMLFormElement>): void {
    e.preventDefault();
    setParams({ q: searchInput.trim() || null, page: null });
  }

  // ---- Запросы ----
  // Ключ ['review-queue'] сохранён — его инвалидация после решения по модерации.
  const queue = useQuery({ queryKey: ['review-queue'], queryFn: reviewQueue });

  const journal = useQuery({
    queryKey: [
      'admin-payments',
      { page, limit, status: status ?? '', courseId: courseId ?? '', q, from, to },
    ],
    queryFn: () => paymentJournal({ page, limit, status, courseId, q, from, to }),
    // Неверный период блокирует запрос: бэкенд ответил бы 400.
    enabled: !periodInvalid,
    // 4xx — не временный сбой, ретраим только сетевые ошибки.
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  const settings = useQuery({
    queryKey: ['admin-settings'],
    queryFn: getAdminSettings,
    retry: false,
  });

  // ---- Справочник курсов для фильтра: накапливаем из загруженных страниц журнала.
  // Обоснование выбора источника — в отчёте T08 (без дополнительного запроса).
  const [courseOptions, setCourseOptions] = useState<Record<string, string>>({});
  useEffect(() => {
    const items = journal.data?.items;
    if (!items || items.length === 0) return;
    setCourseOptions((prev) => {
      let changed = false;
      const next = { ...prev };
      for (const item of items) {
        if (next[item.courseId] !== item.courseTitle) {
          next[item.courseId] = item.courseTitle;
          changed = true;
        }
      }
      return changed ? next : prev;
    });
  }, [journal.data]);

  const courseSelectOptions = useMemo(() => {
    const map = { ...courseOptions };
    // Курс, выбранный через URL, должен присутствовать в списке, даже если его
    // нет в текущей выборке, — иначе фильтр «невидим» и его нельзя снять.
    if (courseId && !map[courseId]) map[courseId] = courseId;
    return Object.entries(map).sort((a, b) => a[1].localeCompare(b[1], 'ru'));
  }, [courseOptions, courseId]);

  // ---- Выгрузка всей выборки в CSV (не только открытой страницы) ----
  const [exporting, setExporting] = useState(false);
  // Тон сообщения: 'ok' — успех/пусто, 'warn' — выборка усечена (T43),
  // 'error' — ошибка сети/сервера.
  const [exportMsg, setExportMsg] = useState<{ tone: 'ok' | 'warn' | 'error'; text: string } | null>(
    null,
  );

  async function onExportCsv(): Promise<void> {
    setExportMsg(null);
    setExporting(true);
    try {
      const rows: PaymentJournalItem[] = [];
      // total выборки — из ответов пагинации: если больше выгруженного, файл
      // обрезан потолком EXPORT_MAX_PAGES (T43).
      let total = 0;
      // Постранично limit=100 до total, но не больше EXPORT_MAX_PAGES:
      // защита от неограниченной выгрузки (ограничение описано в отчёте T38).
      for (let p = 1; p <= EXPORT_MAX_PAGES; p += 1) {
        const res = await paymentJournal({
          page: p,
          limit: EXPORT_PAGE_SIZE,
          status,
          courseId,
          q,
          from,
          to,
        });
        if (p === 1) total = res.total;
        rows.push(...res.items);
        if (rows.length >= res.total || res.items.length === 0) break;
      }
      if (rows.length === 0) {
        // Пустая выборка: сообщаем, файл не создаём.
        setExportMsg({ tone: 'ok', text: t('admin.exportEmpty') });
        return;
      }
      downloadTextFile(
        `payments-${todayIsoDate()}.csv`,
        `\uFEFF${paymentsCsv(rows)}`,
        'text/csv;charset=utf-8',
      );
      if (total > rows.length) {
        // Файл создан, но выборка усечена потолком выгрузки — честно сообщаем.
        setExportMsg({ tone: 'warn', text: t('admin.exportTruncated') });
        return;
      }
      setExportMsg({ tone: 'ok', text: t('admin.exported') });
    } catch (e: unknown) {
      setExportMsg({ tone: 'error', text: errorText(e) });
    } finally {
      setExporting(false);
    }
  }

  // ---- Приглашения ----
  const [invite, setInvite] = useState<Invite | null>(null);
  const [inviteError, setInviteError] = useState('');
  const [tokenCopied, setTokenCopied] = useState(false);
  const [tokenCopyError, setTokenCopyError] = useState('');

  const { register, handleSubmit, reset, formState } = useForm<InviteForm>({
    resolver: zodResolver(inviteSchema),
  });

  const inviteMutation = useMutation({
    mutationFn: (v: InviteForm) => createInvite(v.email.trim()),
    onSuccess: (data) => {
      // Токен виден один раз — сохраняем его в состоянии сразу.
      setInvite(data);
      setInviteError('');
      setTokenCopied(false);
      setTokenCopyError('');
      reset();
    },
    onError: (e: unknown) => {
      setInvite(null);
      setInviteError(errorText(e));
    },
  });

  async function onCopyToken(): Promise<void> {
    if (!invite) return;
    const ok = await copyToClipboard(invite.token);
    setTokenCopied(ok);
    setTokenCopyError(ok ? '' : t('common.error'));
  }

  // ---- Очередь модерации ----
  // Комментарий и ошибка валидации — по каждому курсу отдельно.
  const [comments, setComments] = useState<Record<string, string>>({});
  const [commentErrors, setCommentErrors] = useState<Record<string, boolean>>({});
  const [moderateError, setModerateError] = useState('');

  const moderateMutation = useMutation({
    mutationFn: (args: { id: string; decision: ModerationDecision; comment?: string }) =>
      moderateCourse(args.id, { decision: args.decision, comment: args.comment }),
    onSuccess: (_data, args) => {
      setComments((prev) => ({ ...prev, [args.id]: '' }));
      setCommentErrors((prev) => ({ ...prev, [args.id]: false }));
      setModerateError('');
      void qc.invalidateQueries({ queryKey: ['review-queue'] });
    },
    onError: (e: unknown) => setModerateError(errorText(e)),
  });

  function onModerate(id: string, decision: ModerationDecision): void {
    const comment = (comments[id] ?? '').trim();
    if (decision === 'reject' && !comment) {
      // Без комментария backend отвечает 400 — ловим на клиенте.
      setCommentErrors((prev) => ({ ...prev, [id]: true }));
      return;
    }
    setCommentErrors((prev) => ({ ...prev, [id]: false }));
    moderateMutation.mutate({ id, decision, comment: comment || undefined });
  }

  // ---- Комиссия платформы ----
  const [commissionInput, setCommissionInput] = useState('');
  const [commissionMsg, setCommissionMsg] = useState<{ ok: boolean; text: string } | null>(null);

  useEffect(() => {
    if (typeof settings.data?.commissionPct === 'number') {
      setCommissionInput(String(settings.data.commissionPct));
    }
  }, [settings.data]);

  const commissionMutation = useMutation({
    mutationFn: (pct: number) => updateAdminSettings(pct),
    onSuccess: (data) => {
      qc.setQueryData(['admin-settings'], data);
      setCommissionInput(String(data.commissionPct));
      setCommissionMsg({ ok: true, text: t('admin.commissionSaved') });
    },
    onError: (e: unknown) => setCommissionMsg({ ok: false, text: errorText(e) }),
  });

  function onSaveCommission(e: FormEvent<HTMLFormElement>): void {
    e.preventDefault();
    const raw = commissionInput.trim().replace(',', '.');
    const pct = raw === '' ? NaN : Number(raw);
    if (!Number.isInteger(pct) || pct < 0 || pct > 90) {
      setCommissionMsg({ ok: false, text: t('admin.commissionInvalid') });
      return;
    }
    const confirmed = window.confirm(
      `${t('admin.commissionConfirm')}\n${t('admin.commissionHint')}`,
    );
    if (!confirmed) return;
    setCommissionMsg(null);
    commissionMutation.mutate(pct);
  }

  // ---- Копирование ID платежа ----
  const [copiedPaymentId, setCopiedPaymentId] = useState('');

  async function onCopyPaymentId(id: string, providerPaymentId: string): Promise<void> {
    const ok = await copyToClipboard(providerPaymentId);
    if (!ok) return; // текст всё равно виден — можно скопировать вручную
    setCopiedPaymentId(id);
    window.setTimeout(() => setCopiedPaymentId((prev) => (prev === id ? '' : prev)), 2000);
  }

  // ---- Производные значения журнала ----
  const journalData = journal.data;
  const journalTotal = journalData?.total ?? 0;
  const rangeFrom = journalTotal === 0 ? 0 : (page - 1) * limit + 1;
  const rangeTo = Math.min(page * limit, journalTotal);
  const isLastPage = journalTotal > 0 && rangeTo >= journalTotal;
  const showPager = !!journalData && (journalTotal > 0 || page > 1);

  return (
    <div className="wrap">
      <div className="page">
        <main className="admin">
      <h1>{t('cabinet.adminTitle')}</h1>

      <nav className="admin-nav" aria-label={t('cabinet.adminTitle')}>
        <a href="#queue">{t('admin.reviewQueue')}</a>
        <a href="#journal">{t('admin.journal')}</a>
        <a href="#settings">{t('admin.commissionPct')}</a>
        <a href="#invites">{t('admin.invites')}</a>
      </nav>

      {/* Подразделы супер-админки вынесены на отдельные страницы: у каждой свой
          файл-владелец, поэтому они разрабатываются независимо друг от друга. */}
      <nav className="admin-nav admin-nav--pages" aria-label={t('cabinet.adminTitle')}>
        <Link to="/admin/stats">{t('adminStats.title')}</Link>
        <Link to="/admin/users">{t('adminUsers.title')}</Link>
        <Link to="/admin/courses">{t('adminCourses.title')}</Link>
      </nav>

      {/* ---------- 1. Очередь модерации ---------- */}
      <section id="queue" className="admin-section panel">
        <h2>{t('admin.reviewQueue')}</h2>

        {queue.isLoading && <p>{t('admin.loading')}</p>}
        {queue.isError && (
          <div className="admin-error" role="alert">
            <p className="error">{errorText(queue.error)}</p>
            <button type="button" className="btn" onClick={() => void queue.refetch()}>
              {t('common.retry')}
            </button>
          </div>
        )}
        {queue.data && queue.data.length === 0 && <p className="muted">{t('admin.reviewEmpty')}</p>}
        {moderateError && (
          <p role="alert" className="error">
            {moderateError}
          </p>
        )}

        {queue.data && queue.data.length > 0 && (
          <ul className="admin-queue list teach-list">
            {queue.data.map((c) => (
              <li key={c.id} className="card admin-queue-card l-item">
                <div className="admin-queue-card__head">
                  <strong>{c.title}</strong>
                  <span className="muted">{formatDate(c.createdAt)}</span>
                </div>
                <dl className="admin-meta">
                  <div>
                    <dt>{t('course.teacher')}</dt>
                    <dd>
                      <code>{shortId(c.teacherId)}</code>
                    </dd>
                  </div>
                  <div>
                    <dt>{t('course.price')}</dt>
                    {/* Это цена курса — для неё «Бесплатно» при 0 уместно (как в каталоге). */}
                    <dd>{formatPrice(c.priceCents)}</dd>
                  </div>
                </dl>
                <div className="admin-queue-actions btn-row">
                  <label className="admin-comment">
                    <span className="sr-only">{t('admin.comment')}</span>
                    <input
                      className="input"
                      value={comments[c.id] ?? ''}
                      onChange={(e) => setComments((prev) => ({ ...prev, [c.id]: e.target.value }))}
                      placeholder={t('admin.comment')}
                      aria-invalid={commentErrors[c.id] ? true : undefined}
                    />
                  </label>
                  <button
                    type="button"
                    className="btn btn-primary"
                    onClick={() => onModerate(c.id, 'approve')}
                    disabled={moderateMutation.isPending}
                  >
                    {t('admin.approve')}
                  </button>
                  <button
                    type="button"
                    className="btn admin-btn--danger"
                    onClick={() => onModerate(c.id, 'reject')}
                    disabled={moderateMutation.isPending}
                  >
                    {t('admin.reject')}
                  </button>
                </div>
                {commentErrors[c.id] && (
                  <p role="alert" className="error">
                    {t('admin.rejectCommentRequired')}
                  </p>
                )}
              </li>
            ))}
          </ul>
        )}
      </section>

      {/* ---------- 2. Журнал платежей ---------- */}
      <section id="journal" className="admin-section panel">
        <h2>{t('admin.journal')}</h2>

        <div className="admin-filters">
          <span className="admin-filters__title">{t('admin.filters')}</span>
          {/* Поиск применяется по Enter и по кнопке «Найти»; значение живёт в URL (?q=). */}
          <form className="admin-search" onSubmit={onSearchSubmit}>
            <label htmlFor="admin-filter-q">
              {t('admin.searchStudent')}
              <input
                id="admin-filter-q"
                className="input"
                type="search"
                value={searchInput}
                placeholder={t('admin.searchStudent')}
                onChange={(e) => setSearchInput(e.target.value)}
              />
            </label>
            <button type="submit" className="btn">
              {t('catalog.searchButton')}
            </button>
          </form>
          <label htmlFor="admin-filter-status">
            {t('admin.filterStatus')}
            <select
              id="admin-filter-status"
              className="input"
              value={status ?? ''}
              onChange={(e) => setParams({ status: e.target.value || null, page: null })}
            >
              <option value="">{t('admin.allStatuses')}</option>
              {PAYMENT_STATUSES.map((s) => (
                <option key={s} value={s}>
                  {t(STATUS_LABEL[s])}
                </option>
              ))}
            </select>
          </label>
          <label htmlFor="admin-filter-course">
            {t('admin.filterCourse')}
            <select
              id="admin-filter-course"
              className="input"
              value={courseId ?? ''}
              onChange={(e) => setParams({ courseId: e.target.value || null, page: null })}
            >
              <option value="">{t('admin.allCourses')}</option>
              {courseSelectOptions.map(([id, title]) => (
                <option key={id} value={id}>
                  {title}
                </option>
              ))}
            </select>
          </label>
          {/* Период: границы включительные (контракт T31), значения — в URL. */}
          <div className="admin-filter-period" role="group" aria-label={t('admin.period')}>
            <span className="admin-filter-period__title">{t('admin.period')}</span>
            <div className="admin-filter-period__fields">
              <label htmlFor="admin-filter-from">
                {t('admin.periodFrom')}
                <input
                  id="admin-filter-from"
                  className="input"
                  type="date"
                  value={from}
                  onChange={(e) => setParams({ from: e.target.value || null, page: null })}
                />
              </label>
              <label htmlFor="admin-filter-to">
                {t('admin.periodTo')}
                <input
                  id="admin-filter-to"
                  className="input"
                  type="date"
                  value={to}
                  onChange={(e) => setParams({ to: e.target.value || null, page: null })}
                />
              </label>
            </div>
          </div>
          <button
            type="button"
            className="btn"
            title={t('admin.exportCsvHint')}
            onClick={() => void onExportCsv()}
            disabled={exporting || periodInvalid}
          >
            {exporting ? t('admin.loading') : t('admin.exportCsv')}
          </button>
          <button
            type="button"
            className="btn"
            onClick={() => setSearchParams({})}
            disabled={searchParams.toString() === ''}
          >
            {t('admin.resetFilters')}
          </button>
        </div>

        {periodInvalid && (
          <p role="alert" className="error">
            {t('admin.periodInvalid')}
          </p>
        )}
        {exportMsg && (
          <p
            role={exportMsg.tone === 'error' ? 'alert' : 'status'}
            className={
              exportMsg.tone === 'warn'
                ? 'ce-msg ce-msg--warn'
                : exportMsg.tone === 'error'
                  ? 'error'
                  : ''
            }
          >
            {exportMsg.text}
          </p>
        )}

        {journal.isLoading && <p>{t('admin.loading')}</p>}
        {journal.isError && (
          <div className="admin-error" role="alert">
            <p className="error">{errorText(journal.error)}</p>
            <button type="button" className="btn" onClick={() => void journal.refetch()}>
              {t('common.retry')}
            </button>
          </div>
        )}
        {journalData && journalData.items.length === 0 && (
          <p className="muted">{t('admin.journalEmpty')}</p>
        )}

        {journalData && journalTotal > 0 && (
          <div className="admin-totals">
            <strong>{t('admin.totalsLabel')}</strong>
            <span>
              {t('admin.total')}: {formatMoney(journalData.totals.amountCents)}
            </span>
            <span>
              {t('admin.commission')}: {formatMoney(journalData.totals.platformFeeCents)}
            </span>
            <span>
              {t('admin.toTeacher')}: {formatMoney(journalData.totals.teacherAmountCents)}
            </span>
          </div>
        )}

        {journalData && journalData.items.length > 0 && (
          <div className="admin-table-wrap" role="region" aria-label={t('admin.journal')} tabIndex={0}>
            <table className="admin-table">
              <thead>
                <tr>
                  <th scope="col">{t('admin.colDate')}</th>
                  <th scope="col">{t('admin.colStudent')}</th>
                  <th scope="col">{t('admin.colCourse')}</th>
                  <th scope="col">{t('admin.colAmount')}</th>
                  <th scope="col">{t('admin.colCommission')}</th>
                  <th scope="col">{t('admin.colTeacher')}</th>
                  <th scope="col">{t('admin.colProvider')}</th>
                  <th scope="col">{t('admin.colStatus')}</th>
                  <th scope="col">{t('admin.colPaymentId')}</th>
                </tr>
              </thead>
              <tbody>
                {journalData.items.map((item) => (
                  <tr key={item.id}>
                    <td data-label={t('admin.colDate')}>{formatDate(item.createdAt)}</td>
                    <td data-label={t('admin.colStudent')}>
                      {/* Имя → email → короткий id: студента надо узнать по человекочитаемому
                          признаку, а UUID в журнале US6 не помогает админу работать. */}
                      <span title={item.studentId}>
                        {item.studentName ||
                          item.studentEmail ||
                          shortId(item.studentId)}
                      </span>
                    </td>
                    <td data-label={t('admin.colCourse')} className="admin-td-course">
                      {item.courseTitle}
                    </td>
                    <td data-label={t('admin.colAmount')}>{formatMoney(item.amountCents)}</td>
                    <td data-label={t('admin.colCommission')}>{formatMoney(item.platformFeeCents)}</td>
                    <td data-label={t('admin.colTeacher')}>{formatMoney(item.teacherAmountCents)}</td>
                    <td data-label={t('admin.colProvider')}>{item.provider}</td>
                    <td data-label={t('admin.colStatus')}>
                      <span className={`admin-badge admin-badge--${item.status}`}>
                        {t(STATUS_LABEL[item.status])}
                      </span>
                    </td>
                    <td data-label={t('admin.colPaymentId')} className="admin-td-id">
                      <button
                        type="button"
                        className="admin-copy"
                        title={t('admin.colPaymentId')}
                        aria-label={t('admin.colPaymentId')}
                        onClick={() => void onCopyPaymentId(item.id, item.providerPaymentId)}
                      >
                        <code>{item.providerPaymentId}</code>
                      </button>
                      {copiedPaymentId === item.id && (
                        <span className="admin-copied">{t('common.done')}</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {showPager && journalData && (
          <div className="admin-pager">
            <button
              type="button"
              className="btn"
              disabled={page <= 1 || journal.isFetching}
              onClick={() => setParams({ page: String(page - 1) })}
            >
              {t('admin.prev')}
            </button>
            <span className="muted">
              {t('admin.showing')} {rangeFrom}–{rangeTo} {t('admin.of')} {journalTotal}
            </span>
            <button
              type="button"
              className="btn"
              disabled={isLastPage || journal.isFetching}
              onClick={() => setParams({ page: String(page + 1) })}
            >
              {t('admin.next')}
            </button>
          </div>
        )}
      </section>

      {/* ---------- 3. Настройки комиссии ---------- */}
      <section id="settings" className="admin-section panel">
        <h2>{t('admin.commissionPct')}</h2>

        {settings.isLoading && <p>{t('admin.loading')}</p>}
        {settings.isError && (
          <div className="admin-error" role="alert">
            <p className="error">{errorText(settings.error)}</p>
            <button type="button" className="btn" onClick={() => void settings.refetch()}>
              {t('common.retry')}
            </button>
          </div>
        )}

        <form className="form-grid" onSubmit={onSaveCommission}>
          <label htmlFor="admin-commission">{t('admin.commissionPct')}</label>
          <input
            id="admin-commission"
            className="input"
            type="number"
            min={0}
            max={90}
            step={1}
            inputMode="numeric"
            value={commissionInput}
            onChange={(e) => setCommissionInput(e.target.value)}
          />
          <p className="muted">{t('admin.commissionHint')}</p>
          {commissionMsg && (
            <p role={commissionMsg.ok ? 'status' : 'alert'} className={commissionMsg.ok ? '' : 'error'}>
              {commissionMsg.text}
            </p>
          )}
          <button type="submit" className="btn btn-primary" disabled={commissionMutation.isPending || settings.isLoading}>
            {commissionMutation.isPending ? t('admin.loading') : t('admin.save')}
          </button>
        </form>
      </section>

      {/* ---------- 4. Приглашения ---------- */}
      <section id="invites" className="admin-section panel">
        <h2>{t('admin.invites')}</h2>

        <form className="form-grid" onSubmit={handleSubmit((v) => inviteMutation.mutate(v))}>
          <label htmlFor="admin-invite-email">{t('admin.inviteEmail')}</label>
          <input id="admin-invite-email" className="input" type="email" {...register('email')} />
          {formState.errors.email && (
            <p role="alert" className="error">
              {formState.errors.email.message}
            </p>
          )}
          <button type="submit" className="btn btn-primary" disabled={inviteMutation.isPending}>
            {t('admin.inviteSend')}
          </button>
        </form>

        {inviteError && (
          <p role="alert" className="error">
            {inviteError}
          </p>
        )}

        {invite && (
          <div className="admin-token" role="status">
            <p>
              <strong>{t('admin.inviteCreated')}</strong>
            </p>
            <p>
              <span className="admin-token__label">{t('admin.inviteEmail')}:</span> {invite.email}
            </p>
            <p>
              <span className="admin-token__label">{t('admin.inviteToken')}:</span>
            </p>
            <code className="admin-token__value">{invite.token}</code>
            <p className="muted">{formatDate(invite.expiresAt)}</p>
            <button type="button" className="btn" onClick={() => void onCopyToken()}>
              {tokenCopied ? t('admin.tokenCopied') : t('admin.copyToken')}
            </button>
            {tokenCopyError && (
              <p role="alert" className="error">
                {tokenCopyError}
              </p>
            )}
          </div>
        )}
      </section>
        </main>
      </div>
    </div>
  );
}
