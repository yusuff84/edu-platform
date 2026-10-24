// T49: раздел «Записи на курсы» супер-админки (/admin/enrollments).
// Список всех записей (GET /admin/enrollments, T25) с поиском по ученику
// (debounce ~300 мс), фильтрами курса и статуса, пагинацией; ручной
// отзыв/возврат доступа к курсу (T16). T59: экспорт всей выборки в CSV
// (общий компонент CsvExportButton T40) рядом с фильтрами. Стили изолированы
// в styles/adminEnrollments.css; styles.css и admin.css не изменяются.
import { useEffect, useMemo, useState } from 'react';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { QueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { ApiError, formatDate } from '../api/client';
import {
  listAdminEnrollments,
  revokeEnrollment,
  unrevokeEnrollment,
} from '../api/adminEnrollments';
import type {
  AdminEnrollmentPage,
  AdminEnrollmentRow,
  AdminEnrollmentStatus,
} from '../api/adminEnrollments';
import { listAdminCourses } from '../api/adminConsole';
import { t } from '../i18n/t';
import type { RuKey } from '../i18n/t';
import { useSeo } from '../app/useSeo';
// T59: общий компонент выгрузки CSV (T40) — используется как есть,
// по текущему props API (filename/headers/loadRows), без правок компонента.
import CsvExportButton, { csvDateStamp, exportTruncated } from '../components/CsvExportButton';
import '../styles/adminEnrollments.css';

// ---------- Константы ----------

/** Задержка поиска: запрос уходит после паузы в наборе (ТЗ T49 — ~300 мс). */
const SEARCH_DEBOUNCE_MS = 300;

/** Размер страницы (контракт T25: limit по умолчанию 20). */
const PAGE_LIMIT = 20;

/** Размер страницы выгрузки CSV (T40): 100 строк за запрос. */
const EXPORT_PAGE_LIMIT = 100;

/** Потолок выгрузки CSV: не более 50 страниц (5000 строк) за один клик —
 *  те же константы, что в AdminUsers/AdminCourses (T40) и журнале (T43);
 *  сообщение admin.exportTruncated («первые 5000») остаётся правдой. */
const EXPORT_MAX_PAGES = 50;

/** Справочник курсов для фильтра: первые 100 (потолок limit) — см. отчёт T49. */
const COURSE_OPTIONS_LIMIT = 100;

/** Префикс ключа кэша списка: по нему инвалидируются все страницы с фильтрами. */
const LIST_KEY = ['admin-enrollments'] as const;

/** Ключ кэша справочника курсов для select (не пересекается со списками /admin). */
const COURSE_OPTIONS_KEY = ['admin-enrollment-course-options'] as const;

/** Порядок статусов в фильтре (контракт T48: active|refunded|revoked). */
const STATUSES: AdminEnrollmentStatus[] = ['active', 'refunded', 'revoked'];

/** Русские подписи статусов записи (бейдж и фильтр). */
const STATUS_LABEL: Record<AdminEnrollmentStatus, RuKey> = {
  active: 'adminEnrollments.statusActive',
  refunded: 'adminEnrollments.statusRefunded',
  revoked: 'adminEnrollments.statusRevoked',
};

/** Действие над записью: отозвать доступ или вернуть его. */
type EnrollmentAction = 'revoke' | 'unrevoke';

// ---------- Утилиты ----------

function isEnrollmentStatus(value: string): value is AdminEnrollmentStatus {
  return (STATUSES as readonly string[]).includes(value);
}

/** Текст ошибки без краха страницы (схема из AdminUsers/AdminCourses). */
function errorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 403) return t('cabinet.needRole');
    if (error.status === 401) return t('error.needLogin');
    // 400/404 и прочие — текст бэкенда («Запись ученика на курс не найдена»
    // и т.п.); если текста нет — общий.
    return error.message || t('common.error');
  }
  if (error instanceof TypeError) return t('error.network');
  return t('common.error');
}

/** Процент прогресса в границах 0..100 (как в CourseStudents.tsx). */
function clampPercent(value: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.min(100, Math.max(0, Math.round(value)));
}

/** Строка ученика для ячеек/подтверждений: имя (email), либо только email. */
function studentLabel(row: AdminEnrollmentRow): string {
  const name = row.studentName.trim();
  if (!name) return row.studentEmail;
  return `${name} (${row.studentEmail})`;
}

/** Точечно обновить статус записи во всех кэшированных страницах списка. */
function patchCachedStatus(
  qc: QueryClient,
  studentId: string,
  courseId: string,
  status: AdminEnrollmentStatus,
): void {
  qc.setQueriesData<AdminEnrollmentPage>({ queryKey: LIST_KEY }, (prev) => {
    if (!prev) return prev;
    return {
      ...prev,
      items: prev.items.map((row) =>
        row.studentId === studentId && row.courseId === courseId
          ? { ...row, status }
          : row,
      ),
    };
  });
}

// ---------- Страница ----------

export default function AdminEnrollments(): JSX.Element {
  useSeo(`${t('adminEnrollments.title')} — ${t('cabinet.adminTitle')} — Edu Platform`);
  const qc = useQueryClient();

  // ---- Состояние фильтров и пагинации (локальное, без URL-параметров) ----
  const [searchInput, setSearchInput] = useState('');
  const [q, setQ] = useState('');
  const [courseId, setCourseId] = useState('');
  const [status, setStatus] = useState<AdminEnrollmentStatus | ''>('');
  const [page, setPage] = useState(1);

  // Поиск с debounce: в запрос уходит «успокоившееся» значение, страница — на 1.
  useEffect(() => {
    const timer = window.setTimeout(() => {
      setQ(searchInput.trim());
      setPage(1);
    }, SEARCH_DEBOUNCE_MS);
    return () => window.clearTimeout(timer);
  }, [searchInput]);

  // ---- Справочник курсов для select ----
  // Отдельный запрос (не тянем весь список курсов пачками): первые 100
  // по контракту limit=100; иначе курс просто не попадёт в фильтр.
  const courses = useQuery({
    queryKey: COURSE_OPTIONS_KEY,
    queryFn: () => listAdminCourses({ limit: COURSE_OPTIONS_LIMIT }),
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  // Опции сортируем по названию (как в Admin.tsx); выбранный курс должен быть
  // виден в select, даже если его нет среди первых 100 — иначе фильтр «невидим»
  // и его нельзя снять.
  const courseOptions = useMemo<Array<{ id: string; title: string }>>(() => {
    const items = (courses.data?.items ?? []).map((c) => ({ id: c.id, title: c.title }));
    items.sort((a, b) => a.title.localeCompare(b.title, 'ru'));
    if (courseId !== '' && !items.some((c) => c.id === courseId)) {
      items.unshift({ id: courseId, title: courseId });
    }
    return items;
  }, [courses.data, courseId]);

  // ---- Список записей ----
  const enrollments = useQuery({
    queryKey: [...LIST_KEY, { q, courseId, status, page }],
    queryFn: () =>
      listAdminEnrollments({
        q: q || undefined,
        courseId: courseId || undefined,
        status: status || undefined,
        page,
        limit: PAGE_LIMIT,
      }),
    // При смене страницы/фильтра старые строки остаются до прихода новых.
    placeholderData: keepPreviousData,
    // 4xx — не временный сбой, ретраим только сетевые ошибки (как в Admin.tsx).
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  // ---- Локальные сообщения об операциях ----
  const [notice, setNotice] = useState(''); // успех последней мутации
  const [rowErrors, setRowErrors] = useState<Record<string, string>>({}); // ошибки по строкам

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
  const revokeMutation = useMutation({
    mutationFn: (row: AdminEnrollmentRow) => revokeEnrollment(row.studentId, row.courseId),
    onSuccess: (res, row) => {
      clearRowError(row.id);
      setNotice(t('adminEnrollments.revokedDone'));
      // Ответ применяем к кэшу сразу, чтобы кнопка не «откатывалась» до refetch.
      patchCachedStatus(qc, row.studentId, row.courseId, res?.status ?? 'revoked');
      void qc.invalidateQueries({ queryKey: LIST_KEY });
    },
    onError: (e: unknown, row) => {
      setNotice('');
      setRowError(row.id, errorText(e));
    },
  });

  const unrevokeMutation = useMutation({
    mutationFn: (row: AdminEnrollmentRow) => unrevokeEnrollment(row.studentId, row.courseId),
    onSuccess: (res, row) => {
      clearRowError(row.id);
      setNotice(t('adminEnrollments.unrevokedDone'));
      patchCachedStatus(qc, row.studentId, row.courseId, res?.status ?? 'active');
      void qc.invalidateQueries({ queryKey: LIST_KEY });
    },
    onError: (e: unknown, row) => {
      setNotice('');
      setRowError(row.id, errorText(e));
    },
  });

  // ---- Обработчики действий ----
  function onAction(row: AdminEnrollmentRow, action: EnrollmentAction): void {
    const question =
      action === 'revoke'
        ? t('adminEnrollments.confirmRevoke')
        : t('adminEnrollments.confirmUnrevoke');
    // Ключи словаря без интерполяции (t.ts), поэтому кого и какого курса
    // касается действие — подставляем сами, второй строкой под вопросом.
    if (!window.confirm(`${question}\n${studentLabel(row)} — «${row.courseTitle}»`)) return;
    clearRowError(row.id);
    setNotice('');
    if (action === 'revoke') revokeMutation.mutate(row);
    else unrevokeMutation.mutate(row);
  }

  function onResetFilters(): void {
    setSearchInput('');
    setQ('');
    setCourseId('');
    setStatus('');
    setPage(1);
  }

  // ---- Выгрузка CSV: вся выборка по текущим фильтрам (T59, компонент T40) ----
  // Идём постранично (limit=100) до total, но не более 50 страниц: защита от
  // бесконечной выгрузки; усечённая выборка честно считается T43-хелпером.
  // Значения — те же поля, что в таблице; createdAt выгружается как есть
  // (ISO от бэкенда), чтобы дату сортировать в Excel. Статус — словарной
  // подписью, как в выгрузках пользователей/курсов (T40).
  async function loadExportRows(): Promise<{
    rows: Array<Array<string | number | null>>;
    truncated: boolean;
  }> {
    try {
      const rows: Array<Array<string | number | null>> = [];
      let total = 0;
      for (let pageNumber = 1; pageNumber <= EXPORT_MAX_PAGES; pageNumber += 1) {
        const res = await listAdminEnrollments({
          q: q || undefined,
          courseId: courseId || undefined,
          status: status || undefined,
          page: pageNumber,
          limit: EXPORT_PAGE_LIMIT,
        });
        if (pageNumber === 1) total = res.total;
        for (const row of res.items) {
          rows.push([
            row.id,
            row.studentEmail,
            row.studentName,
            row.courseTitle,
            row.courseId,
            t(STATUS_LABEL[row.status]),
            `${row.completedLessons}/${row.totalLessons} (${clampPercent(row.percent)}%)`,
            row.createdAt,
          ]);
        }
        // Неполная страница — данных больше нет.
        if (res.items.length < EXPORT_PAGE_LIMIT || rows.length >= res.total) break;
      }
      // Экранный тост: файл создан → «Файл скачан»; пусто/ошибка — мелкое
      // сообщение рисует сам компонент T40 под кнопкой (exportEmpty/текст
      // ошибки), чтобы страница не показывала два противоречащих уведомления.
      setNotice(rows.length > 0 ? t('p7.enroll.exportDone') : '');
      return { rows, truncated: exportTruncated(total, rows.length) };
    } catch (e: unknown) {
      setNotice('');
      throw e;
    }
  }

  // Заголовки CSV: ключи — из таблицы страницы и списка пользователей; «ID»,
  // «ID курса» и «Создан» подписываем прямо в CSV — это данные, а не
  // интерфейс (как «Slug»/«Создан» в AdminCourses, T40).
  const exportHeaders = [
    'ID',
    t('adminUsers.colEmail'),
    t('adminUsers.colName'),
    t('adminEnrollments.colCourse'),
    'ID курса',
    t('adminEnrollments.colStatus'),
    t('adminEnrollments.colProgress'),
    'Создан',
  ];

  // ---- Состояния ожидания по строкам (для блокировки кнопок) ----
  const revokePendingId = revokeMutation.isPending
    ? revokeMutation.variables?.id
    : undefined;
  const unrevokePendingId = unrevokeMutation.isPending
    ? unrevokeMutation.variables?.id
    : undefined;

  // ---- Производные значения списка ----
  const data = enrollments.data;
  const total = data?.total ?? 0;
  const rangeFrom = total === 0 ? 0 : Math.min((page - 1) * PAGE_LIMIT + 1, total);
  const rangeTo = Math.min(page * PAGE_LIMIT, total);
  const isLastPage = total > 0 && rangeTo >= total;
  // Пагинатор прячем только на пустой первой странице (как в AdminUsers):
  // если данных не осталось, а страница не первая, кнопка «Назад» вернёт список.
  const showPager = !!data && (total > 0 || page > 1);
  const filtersActive =
    searchInput !== '' || q !== '' || courseId !== '' || status !== '' || page > 1;

  return (
    <div className="wrap">
      <div className="page">
        <main className="ae-admin">
          <p className="ae-back">
            <Link to="/admin">{t('cabinet.adminTitle')}</Link>
          </p>
          <h1>{t('adminEnrollments.title')}</h1>

          <section className="panel">
            {/* ---------- Фильтры ---------- */}
            <div className="ae-filters">
              <label className="ae-field ae-field--search" htmlFor="ae-search">
                <span>{t('adminEnrollments.search')}</span>
                <input
                  id="ae-search"
                  className="input"
                  type="search"
                  value={searchInput}
                  placeholder={t('adminEnrollments.search')}
                  onChange={(e) => setSearchInput(e.target.value)}
                />
              </label>
              <label className="ae-field" htmlFor="ae-filter-course">
                <span>{t('adminEnrollments.filterCourse')}</span>
                <select
                  id="ae-filter-course"
                  className="input ae-select"
                  value={courseId}
                  onChange={(e) => {
                    setCourseId(e.target.value);
                    setPage(1);
                  }}
                >
                  <option value="">{t('adminEnrollments.all')}</option>
                  {courseOptions.map((course) => (
                    <option key={course.id} value={course.id}>
                      {course.title}
                    </option>
                  ))}
                </select>
              </label>
              <label className="ae-field" htmlFor="ae-filter-status">
                <span>{t('adminEnrollments.filterStatus')}</span>
                <select
                  id="ae-filter-status"
                  className="input ae-select"
                  value={status}
                  onChange={(e) => {
                    setStatus(isEnrollmentStatus(e.target.value) ? e.target.value : '');
                    setPage(1);
                  }}
                >
                  <option value="">{t('adminEnrollments.all')}</option>
                  {STATUSES.map((s) => (
                    <option key={s} value={s}>
                      {t(STATUS_LABEL[s])}
                    </option>
                  ))}
                </select>
              </label>
              <button
                type="button"
                className="btn"
                disabled={!filtersActive}
                onClick={onResetFilters}
              >
                {t('adminEnrollments.reset')}
              </button>

              {/* ---------- T59: выгрузка всей выборки по текущим фильтрам ---------- */}
              {/* Подпись группы — замороженный ключ p7.enroll.exportCsv; сама
                  кнопка и её мелкие сообщения — неизменяемый компонент T40,
                  ярлык которого admin.exportCsv. */}
              <div className="ae-field ae-export">
                <span className="ae-export__label">
                  {t('p7.enroll.exportCsv')}
                </span>
                <CsvExportButton
                  filename={`enrollments-${csvDateStamp()}.csv`}
                  headers={exportHeaders}
                  loadRows={() => loadExportRows()}
                />
              </div>
            </div>

            {/* ---------- Итог по фильтру и уведомление об успехе ---------- */}
            {data && (
              <p className="ae-total muted">
                {t('adminEnrollments.total')}: {total}
              </p>
            )}
            {notice && (
              <p className="ae-notice" role="status">
                {notice}
              </p>
            )}

            {/* ---------- Состояния запроса ---------- */}
            {enrollments.isLoading && (
              <p className="muted" role="status" aria-live="polite">
                {t('common.loading')}
              </p>
            )}

            {enrollments.isError && (
              <div className="ae-error" role="alert">
                <p className="error">{errorText(enrollments.error)}</p>
                <button
                  type="button"
                  className="btn"
                  onClick={() => void enrollments.refetch()}
                >
                  {t('common.retry')}
                </button>
              </div>
            )}

            {data && data.items.length === 0 && (
              <p className="muted">{t('adminEnrollments.empty')}</p>
            )}

            {/* ---------- Таблица ---------- */}
            {data && data.items.length > 0 && (
              <div
                className="ae-table-wrap"
                role="region"
                aria-label={t('adminEnrollments.title')}
                tabIndex={0}
              >
                <table className="ae-table">
                  <thead>
                    <tr>
                      <th scope="col">{t('adminEnrollments.colStudent')}</th>
                      <th scope="col">{t('adminEnrollments.colCourse')}</th>
                      <th scope="col">{t('adminEnrollments.colStatus')}</th>
                      <th scope="col">{t('adminEnrollments.colProgress')}</th>
                      <th scope="col">{t('adminEnrollments.colDate')}</th>
                      <th scope="col">{t('adminEnrollments.colActions')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.items.map((row) => {
                      const rowBusy =
                        revokePendingId === row.id || unrevokePendingId === row.id;
                      const percent = clampPercent(row.percent);
                      const name = row.studentName.trim();
                      return (
                        <tr
                          key={row.id}
                          className={rowBusy ? 'ae-row--busy' : undefined}
                          aria-busy={rowBusy || undefined}
                        >
                          <td
                            className="ae-td-student"
                            data-label={t('adminEnrollments.colStudent')}
                          >
                            <span className="ae-student">
                              <b className="ae-student__name" title={studentLabel(row)}>
                                {name || row.studentEmail}
                              </b>
                              {name !== '' && (
                                <span className="ae-student__email" title={row.studentEmail}>
                                  {row.studentEmail}
                                </span>
                              )}
                            </span>
                          </td>
                          <td
                            className="ae-td-course"
                            data-label={t('adminEnrollments.colCourse')}
                            title={row.courseTitle}
                          >
                            {row.courseTitle}
                          </td>
                          <td data-label={t('adminEnrollments.colStatus')}>
                            <span className={`ae-badge ae-badge--${row.status}`}>
                              {t(STATUS_LABEL[row.status])}
                            </span>
                          </td>
                          <td data-label={t('adminEnrollments.colProgress')}>
                            <span className="ae-progress">
                              <span
                                className="progress"
                                role="progressbar"
                                aria-label={t('adminEnrollments.colProgress')}
                                aria-valuemin={0}
                                aria-valuemax={100}
                                aria-valuenow={percent}
                                aria-valuetext={`${row.completedLessons} ${t('admin.of')} ${row.totalLessons}`}
                              >
                                <span
                                  className="progress-bar"
                                  style={{ width: `${percent}%` }}
                                />
                              </span>
                              <span className="ae-progress-meta">
                                {row.completedLessons} {t('admin.of')} {row.totalLessons} ·{' '}
                                {percent}%
                              </span>
                            </span>
                          </td>
                          <td data-label={t('adminEnrollments.colDate')}>
                            {formatDate(row.createdAt)}
                          </td>
                          <td
                            className="ae-td-actions"
                            data-label={t('adminEnrollments.colActions')}
                          >
                            <div className="ae-actions">
                              {/* refunded — без действий: доступ закрыт возвратом
                                  средств, ручное управление только для active/revoked. */}
                              {row.status === 'active' && (
                                <button
                                  type="button"
                                  className="btn btn-sm ae-btn--danger"
                                  disabled={rowBusy}
                                  onClick={() => onAction(row, 'revoke')}
                                >
                                  {rowBusy
                                    ? t('common.loading')
                                    : t('adminEnrollments.revoke')}
                                </button>
                              )}
                              {row.status === 'revoked' && (
                                <button
                                  type="button"
                                  className="btn btn-sm"
                                  disabled={rowBusy}
                                  onClick={() => onAction(row, 'unrevoke')}
                                >
                                  {rowBusy
                                    ? t('common.loading')
                                    : t('adminEnrollments.unrevoke')}
                                </button>
                              )}
                              {row.status === 'refunded' && <span className="muted">—</span>}
                            </div>
                            {rowErrors[row.id] && (
                              <p className="error ae-row-error" role="alert">
                                {rowErrors[row.id]}
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
              <div className="ae-pager">
                <button
                  type="button"
                  className="btn"
                  disabled={page <= 1 || enrollments.isFetching}
                  onClick={() => setPage(page - 1)}
                >
                  {t('admin.prev')}
                </button>
                <span className="muted">
                  {t('adminEnrollments.showing')} {rangeFrom}–{rangeTo} {t('admin.of')} {total}
                </span>
                <button
                  type="button"
                  className="btn"
                  disabled={isLastPage || enrollments.isFetching}
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
