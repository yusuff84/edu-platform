// Журнал платежей — отдельная страница /admin/payments.
// Логика и разметка перенесены из секции #journal страницы Admin.tsx без
// изменений поведения: фильтры в URL (page/limit/status/courseId/q/from/to),
// пагинация, выгрузка CSV, копирование ID платежа, модалка деталей и возврат.
import { type FormEvent, useEffect, useMemo, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link, useSearchParams } from 'react-router-dom';
import { ApiError, formatDate, formatMoney, paymentJournal } from '../api/client';
import type { PaymentJournalItem, PaymentStatus } from '../api/client';
import { refundPayment } from '../api/adminActions';
import { t } from '../i18n/t';
import { useSeo } from '../app/useSeo';
import {
  DEFAULT_LIMIT,
  EXPORT_MAX_PAGES,
  EXPORT_PAGE_SIZE,
  PAYMENT_STATUSES,
  STATUS_LABEL,
  copyToClipboard,
  downloadTextFile,
  errorText,
  isPaymentStatus,
  paymentsCsv,
  shortId,
  todayIsoDate,
} from './adminShared';
// Стили страницы изолированы в styles/admin.css — общий styles.css не трогаем.
import '../styles/admin.css';
// T43: стиль предупреждения об усечении выгрузки (.ce-msg--warn) — из csvExport.css.
import '../styles/csvExport.css';

export default function AdminPayments(): JSX.Element {
  useSeo(`${t('admin.journal')} — ${t('cabinet.adminTitle')} — Edu Platform`);
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

  // ---- Запрос журнала ----
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

  // ---- Копирование ID платежа ----
  const [copiedPaymentId, setCopiedPaymentId] = useState('');

  async function onCopyPaymentId(id: string, providerPaymentId: string): Promise<void> {
    const ok = await copyToClipboard(providerPaymentId);
    if (!ok) return; // текст всё равно виден — можно скопировать вручную
    setCopiedPaymentId(id);
    window.setTimeout(() => setCopiedPaymentId((prev) => (prev === id ? '' : prev)), 2000);
  }

  // ---- Детали платежа и возврат средств (T51, контракт T47) ----
  const [paymentDetails, setPaymentDetails] = useState<PaymentJournalItem | null>(null);
  const [refundError, setRefundError] = useState('');
  const [refundNotice, setRefundNotice] = useState('');
  const paymentTitleRef = useRef<HTMLHeadingElement | null>(null);
  const paymentDialogRef = useRef<HTMLDivElement | null>(null);
  // Элемент, на который вернём фокус после закрытия модалки.
  const paymentReturnFocusRef = useRef<HTMLElement | null>(null);

  function openPaymentDetails(item: PaymentJournalItem): void {
    const active = document.activeElement;
    paymentReturnFocusRef.current = active instanceof HTMLElement ? active : null;
    setPaymentDetails(item);
    setRefundError('');
  }

  function closePaymentDetails(): void {
    setPaymentDetails(null);
    setRefundError('');
  }

  // Пока модалка открыта: фокус — на заголовке, Esc — закрытие, Tab ходит
  // только по элементам диалога. При закрытии возвращаем фокус на кнопку,
  // которая открыла окно (требование доступности модалок).
  useEffect(() => {
    if (!paymentDetails) return;
    const dialog = paymentDialogRef.current;
    paymentTitleRef.current?.focus();
    function onKeyDown(e: KeyboardEvent): void {
      if (e.key === 'Escape') {
        closePaymentDetails();
        return;
      }
      if (e.key !== 'Tab' || !dialog) return;
      const focusable = Array.from(
        dialog.querySelectorAll<HTMLElement>(
          'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])',
        ),
      ).filter((node) => !node.hidden);
      if (focusable.length === 0) {
        e.preventDefault();
        return;
      }
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      const active = document.activeElement;
      // Заголовок модалки тоже получает фокус (tabIndex=-1), но в списке
      // фокусируемых его нет: Tab/Shift+Tab с него должны уходить внутрь окна,
      // а не за его пределы.
      const index = active instanceof HTMLElement ? focusable.indexOf(active) : -1;
      if (index === -1) {
        e.preventDefault();
        (e.shiftKey ? last : first).focus();
      } else if (e.shiftKey && index === 0) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && index === focusable.length - 1) {
        e.preventDefault();
        first.focus();
      }
    }
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('keydown', onKeyDown);
      const back = paymentReturnFocusRef.current;
      if (back && back.isConnected) back.focus();
      paymentReturnFocusRef.current = null;
    };
  }, [paymentDetails]);

  const refundMutation = useMutation({
    mutationFn: (id: string) => refundPayment(id),
    onSuccess: () => {
      // Успех: модалка закрывается, в секции журнала — «тост», выборка refetch.
      setPaymentDetails(null);
      setRefundError('');
      setRefundNotice(t('admin.refundDone'));
      void qc.invalidateQueries({ queryKey: ['admin-payments'] });
    },
    // Ошибки API (409 — уже возвращён, 502 — отказ провайдера) — текстом в модалке.
    onError: (e: unknown) => setRefundError(errorText(e)),
  });

  function onRefundPayment(id: string): void {
    if (!window.confirm(t('admin.confirmRefund'))) return;
    setRefundNotice('');
    refundMutation.mutate(id);
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
          <p className="muted">
            <Link to="/admin">{t('p10.nav.backToAdmin')}</Link>
          </p>
          <h1>{t('admin.journal')}</h1>

          <section className="admin-section panel" aria-label={t('admin.journal')}>
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
            {refundNotice && (
              <p role="status" className="admin-notice">
                {refundNotice}
              </p>
            )}

            {journal.isLoading && <p>{t('admin.loading')}</p>}
            {/* Фоновое обновление (refetch/инвалидация) не прячет таблицу,
                но честно сообщает, что данные ещё обновляются. */}
            {journal.isFetching && !journal.isLoading && (
              <p className="muted" role="status" aria-live="polite">
                Обновление…
              </p>
            )}
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
              <div
                className="admin-table-wrap"
                role="region"
                aria-label={t('admin.journal')}
                tabIndex={0}
                aria-busy={journal.isFetching || undefined}
              >
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
                      {/* Ключа «Действия» у журнала нет: заголовком служит admin.paymentOpen. */}
                      <th scope="col">{t('admin.paymentOpen')}</th>
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
                        <td data-label={t('admin.colCommission')}>
                          {formatMoney(item.platformFeeCents)}
                        </td>
                        <td data-label={t('admin.colTeacher')}>
                          {formatMoney(item.teacherAmountCents)}
                        </td>
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
                        <td data-label={t('admin.paymentOpen')}>
                          <button
                            type="button"
                            className="btn btn-sm"
                            onClick={() => openPaymentDetails(item)}
                          >
                            {t('admin.paymentOpen')}
                          </button>
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

          {/* ---------- Модалка «Детали платежа» (T51) ---------- */}
          {paymentDetails && (
            <div
              className="admin-modal-overlay"
              // Клик по подложке (не по содержимому) закрывает модалку.
              onClick={(e) => {
                if (e.target === e.currentTarget) closePaymentDetails();
              }}
            >
              <div
                ref={paymentDialogRef}
                className="admin-modal"
                role="dialog"
                aria-modal="true"
                aria-labelledby="admin-payment-details-title"
              >
                <div className="admin-modal__head">
                  <h3 id="admin-payment-details-title" ref={paymentTitleRef} tabIndex={-1}>
                    {t('admin.paymentDetails')}
                  </h3>
                  <button type="button" className="btn btn-sm" onClick={closePaymentDetails}>
                    {t('common.close')}
                  </button>
                </div>

                <dl className="admin-modal-meta">
                  <div>
                    <dt>{t('admin.paymentStudent')}</dt>
                    <dd>
                      {/* Как в журнале: имя → email → короткий id. */}
                      {paymentDetails.studentName ||
                        paymentDetails.studentEmail ||
                        shortId(paymentDetails.studentId)}
                    </dd>
                  </div>
                  <div>
                    <dt>{t('admin.paymentCourse')}</dt>
                    <dd>{paymentDetails.courseTitle}</dd>
                  </div>
                  <div>
                    <dt>{t('admin.paymentProvider')}</dt>
                    <dd>{paymentDetails.provider}</dd>
                  </div>
                  <div>
                    <dt>{t('admin.paymentProviderId')}</dt>
                    <dd>
                      <code>{paymentDetails.providerPaymentId}</code>
                    </dd>
                  </div>
                  <div>
                    <dt>{t('admin.paymentAmount')}</dt>
                    <dd>{formatMoney(paymentDetails.amountCents)}</dd>
                  </div>
                  <div>
                    <dt>{t('admin.paymentFee')}</dt>
                    <dd>{formatMoney(paymentDetails.platformFeeCents)}</dd>
                  </div>
                  <div>
                    <dt>{t('admin.paymentTeacherAmount')}</dt>
                    <dd>{formatMoney(paymentDetails.teacherAmountCents)}</dd>
                  </div>
                  <div>
                    <dt>{t('admin.colStatus')}</dt>
                    <dd>
                      <span className={`admin-badge admin-badge--${paymentDetails.status}`}>
                        {t(STATUS_LABEL[paymentDetails.status])}
                      </span>
                    </dd>
                  </div>
                  <div>
                    <dt>{t('admin.paymentCreated')}</dt>
                    <dd>{formatDate(paymentDetails.createdAt)}</dd>
                  </div>
                  <div>
                    <dt>{t('admin.paymentProcessed')}</dt>
                    <dd>{formatDate(paymentDetails.processedAt)}</dd>
                  </div>
                </dl>

                {/* Ошибка возврата (409/502 и пр.) — текстом в модалке. */}
                {refundError && (
                  <p role="alert" className="error">
                    {refundError}
                  </p>
                )}

                {/* Возврат доступен только для успешного платежа — как и на backend. */}
                {paymentDetails.status === 'succeeded' && (
                  <div className="admin-modal__actions">
                    <button
                      type="button"
                      className="btn admin-btn--danger"
                      disabled={refundMutation.isPending}
                      onClick={() => onRefundPayment(paymentDetails.id)}
                    >
                      {refundMutation.isPending ? t('admin.loading') : t('admin.paymentRefund')}
                    </button>
                  </div>
                )}
              </div>
            </div>
          )}
        </main>
      </div>
    </div>
  );
}
