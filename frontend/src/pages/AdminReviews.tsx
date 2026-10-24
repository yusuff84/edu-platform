// T84: страница «Отзывы» супер-админки (/admin/reviews).
// Глобальный список отзывов (GET /admin/reviews, T83) с фильтрами
// courseId/rating/q/from/to/sort/dir, пагинацией и удалением отзыва
// (DELETE /admin/reviews/{id}). Состояние выборки живёт в URL — ссылкой можно
// поделиться, кнопки «назад/вперёд» браузера работают как в AdminCourses.
// Стили изолированы в styles/adminReviews.css; styles.css и admin.css не
// изменяются.
import { type FormEvent, useEffect, useMemo, useState } from 'react';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link, useSearchParams } from 'react-router-dom';
import { ApiError, formatDate } from '../api/client';
import {
  deleteAdminReview,
  listAdminReviews,
} from '../api/adminReviews';
import type {
  AdminReview,
  AdminReviewsDir,
  AdminReviewsSort,
} from '../api/adminReviews';
import { listAdminCourses } from '../api/adminConsole';
import { t } from '../i18n/t';
import type { RuKey } from '../i18n/t';
import { useSeo } from '../app/useSeo';
import '../styles/adminReviews.css';

// ---------- Константы ----------

/** Размер страницы: контракт T83 — 20 по умолчанию, максимум 100. */
const PAGE_LIMIT = 20;

/** Справочник курсов для фильтра: первые 100 (потолок limit контракта). */
const COURSE_OPTIONS_LIMIT = 100;

/** Префикс ключа кэша списка: по нему инвалидируются все страницы с фильтрами. */
const LIST_KEY = ['admin-reviews'] as const;

/** Ключ кэша справочника курсов для select (не пересекается со списком). */
const COURSE_OPTIONS_KEY = ['admin-reviews-course-options'] as const;

/** Допустимые поля сортировки (контракт T83). */
const SORTS: AdminReviewsSort[] = ['createdAt', 'rating'];

/** Допустимые направления сортировки (контракт T83). */
const DIRS: AdminReviewsDir[] = ['asc', 'desc'];

/** Значения по умолчанию совпадают с backend: createdAt desc. */
const DEFAULT_SORT: AdminReviewsSort = 'createdAt';
const DEFAULT_DIR: AdminReviewsDir = 'desc';

/** Оценки в фильтре — от старшей к младшей. */
const RATINGS = [5, 4, 3, 2, 1] as const;

/** Подписи полей сортировки: «по оценке» своего ключа нет — берём оценку. */
const SORT_LABEL: Record<AdminReviewsSort, RuKey> = {
  createdAt: 'adminCourses.sortCreated',
  rating: 'p10.reviews.filterRating',
};

const DIR_LABEL: Record<AdminReviewsDir, RuKey> = {
  asc: 'adminCourses.dirAsc',
  desc: 'adminCourses.dirDesc',
};

// ---------- Утилиты ----------

function isReviewSort(value: string): value is AdminReviewsSort {
  return (SORTS as readonly string[]).includes(value);
}

function isReviewDir(value: string): value is AdminReviewsDir {
  return (DIRS as readonly string[]).includes(value);
}

/**
 * Строгая проверка даты ГГГГ-ММ-ДД: и формат, и реальный календарный день
 * (2026-02-30 отбрасывается — backend тоже отвечает на неё 400).
 */
function isIsoDate(value: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const parsed = new Date(`${value}T00:00:00Z`);
  return !Number.isNaN(parsed.getTime()) && parsed.toISOString().slice(0, 10) === value;
}

/** Текст ошибки без краха страницы: 403/401/400/сеть/прочее (как в админке). */
function errorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 403) return t('cabinet.needRole');
    if (error.status === 401) return t('error.needLogin');
    return error.message || t('common.error');
  }
  if (error instanceof TypeError) return t('error.network');
  return t('common.error');
}

/** Строка ученика: имя (email), либо только email. */
function studentLabel(row: AdminReview): string {
  const name = row.studentName.trim();
  return name ? `${name} (${row.studentEmail})` : row.studentEmail;
}

// ---------- Страница ----------

export default function AdminReviews(): JSX.Element {
  useSeo(`${t('p10.reviews.title')} — Edu Platform`);
  const qc = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();

  // ---- Применённые фильтры и страница живут в URL ----
  const q = searchParams.get('q') ?? '';
  const courseId = searchParams.get('courseId') ?? '';
  const ratingParam = Number(searchParams.get('rating'));
  const rating =
    Number.isInteger(ratingParam) && ratingParam >= 1 && ratingParam <= 5
      ? ratingParam
      : 0;
  const from = searchParams.get('from') ?? '';
  const to = searchParams.get('to') ?? '';
  const sortParam = searchParams.get('sort') ?? '';
  const sort: AdminReviewsSort = isReviewSort(sortParam) ? sortParam : DEFAULT_SORT;
  const dirParam = searchParams.get('dir') ?? '';
  const dir: AdminReviewsDir = isReviewDir(dirParam) ? dirParam : DEFAULT_DIR;
  const pageParam = Number(searchParams.get('page'));
  const page = Number.isInteger(pageParam) && pageParam >= 1 ? pageParam : 1;

  // Неверный период, попавший в URL вручную: запрос не уходит (backend — 400).
  const periodInvalid =
    (from !== '' && !isIsoDate(from)) ||
    (to !== '' && !isIsoDate(to)) ||
    (from !== '' && to !== '' && from > to);

  // ---- Черновик формы: в URL уходит только по кнопке «Применить» ----
  const [draftQ, setDraftQ] = useState(q);
  const [draftCourseId, setDraftCourseId] = useState(courseId);
  const [draftRating, setDraftRating] = useState(rating);
  const [draftFrom, setDraftFrom] = useState(from);
  const [draftTo, setDraftTo] = useState(to);
  const [draftSort, setDraftSort] = useState<AdminReviewsSort>(sort);
  const [draftDir, setDraftDir] = useState<AdminReviewsDir>(dir);

  // URL — источник истины: переходы по истории и сброс обновляют черновик.
  useEffect(() => {
    setDraftQ(q);
    setDraftCourseId(courseId);
    setDraftRating(rating);
    setDraftFrom(from);
    setDraftTo(to);
    setDraftSort(sort);
    setDraftDir(dir);
  }, [q, courseId, rating, from, to, sort, dir]);

  // ---- Справочник курсов для select ----
  // Отдельный запрос (не тянем весь список курсов пачками): первые 100
  // по контракту limit=100; иначе курс просто не попадёт в фильтр.
  const courses = useQuery({
    queryKey: COURSE_OPTIONS_KEY,
    queryFn: () => listAdminCourses({ limit: COURSE_OPTIONS_LIMIT }),
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  // Опции по названию; выбранный курс виден в select, даже если его нет
  // среди первых 100 — иначе фильтр «невидим» и его нельзя снять.
  const courseOptions = useMemo<Array<{ id: string; title: string }>>(() => {
    const items = (courses.data?.items ?? []).map((c) => ({ id: c.id, title: c.title }));
    items.sort((a, b) => a.title.localeCompare(b.title, 'ru'));
    if (courseId !== '' && !items.some((c) => c.id === courseId)) {
      items.unshift({ id: courseId, title: courseId });
    }
    return items;
  }, [courses.data, courseId]);

  // ---- Список отзывов ----
  const reviews = useQuery({
    queryKey: [...LIST_KEY, { q, courseId, rating, from, to, sort, dir, page }],
    queryFn: () =>
      listAdminReviews({
        q: q || undefined,
        courseId: courseId || undefined,
        rating: rating || undefined,
        from: from || undefined,
        to: to || undefined,
        sort,
        dir,
        page,
        limit: PAGE_LIMIT,
      }),
    // Неверный период из URL блокирует запрос: backend ответил бы 400.
    enabled: !periodInvalid,
    // При смене страницы/фильтра старые строки остаются до прихода новых.
    placeholderData: keepPreviousData,
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  // ---- Уведомления об операциях ----
  const [notice, setNotice] = useState('');
  const [actionError, setActionError] = useState('');
  const [formError, setFormError] = useState('');

  const removeMutation = useMutation({
    mutationFn: (review: AdminReview) => deleteAdminReview(review.id),
    onSuccess: (_result, review) => {
      setActionError('');
      setNotice(t('p10.reviews.deleted'));
      // Удалили последнюю строку не первой страницы — возвращаемся на
      // предыдущую, чтобы не оставлять пользователя на пустой странице.
      if (page > 1 && reviews.data && reviews.data.items.length === 1) {
        goToPage(page - 1);
      } else {
        void qc.invalidateQueries({ queryKey: LIST_KEY });
      }
      // Backend пересчитывает рейтинг курса — обновляем и сводку админки.
      void qc.invalidateQueries({ queryKey: ['admin-stats'] });
    },
    onError: (e: unknown) => {
      setNotice('');
      setActionError(errorText(e));
    },
  });

  // ---- Обработчики ----

  /** Точечно обновить страницу в URL (page=1 — параметр не нужен). */
  function goToPage(nextPage: number): void {
    // Функциональная форма: удаление может завершиться уже после смены URL.
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (nextPage <= 1) next.delete('page');
      else next.set('page', String(nextPage));
      return next;
    });
  }

  function onApply(event: FormEvent<HTMLFormElement>): void {
    event.preventDefault();
    const nextQ = draftQ.trim();
    const invalidDates =
      (draftFrom !== '' && !isIsoDate(draftFrom)) ||
      (draftTo !== '' && !isIsoDate(draftTo)) ||
      (draftFrom !== '' && draftTo !== '' && draftFrom > draftTo);
    if (invalidDates) {
      setFormError(t('admin.periodInvalid'));
      return;
    }
    setFormError('');
    setNotice('');
    setActionError('');

    const next = new URLSearchParams();
    if (nextQ !== '') next.set('q', nextQ);
    if (draftCourseId !== '') next.set('courseId', draftCourseId);
    if (draftRating !== 0) next.set('rating', String(draftRating));
    if (draftFrom !== '') next.set('from', draftFrom);
    if (draftTo !== '') next.set('to', draftTo);
    if (draftSort !== DEFAULT_SORT) next.set('sort', draftSort);
    if (draftDir !== DEFAULT_DIR) next.set('dir', draftDir);
    // Применение фильтров всегда возвращает на первую страницу.
    setSearchParams(next);
  }

  function onReset(): void {
    setDraftQ('');
    setDraftCourseId('');
    setDraftRating(0);
    setDraftFrom('');
    setDraftTo('');
    setDraftSort(DEFAULT_SORT);
    setDraftDir(DEFAULT_DIR);
    setFormError('');
    setNotice('');
    setActionError('');
    setSearchParams({});
  }

  function onDelete(review: AdminReview): void {
    // Подтверждение: вопрос из словаря + кого и какой курс касается действие,
    // второй строкой (ключи словаря без интерполяции).
    if (!window.confirm(`${t('p10.reviews.confirmDelete')}\n${studentLabel(review)} — «${review.courseTitle}»`)) {
      return;
    }
    setNotice('');
    setActionError('');
    removeMutation.mutate(review);
  }

  // ---- Производные значения ----
  const data = reviews.data;
  const total = data?.total ?? 0;
  const rangeFrom = total === 0 ? 0 : Math.min((page - 1) * PAGE_LIMIT + 1, total);
  const rangeTo = Math.min(page * PAGE_LIMIT, total);
  const isLastPage = total > 0 && rangeTo >= total;
  // Пагинатор прячем только на пустой первой странице: если данных не
  // осталось, а страница не первая, кнопка «Назад» вернёт список.
  const showPager = !!data && (total > 0 || page > 1);
  const pendingId = removeMutation.isPending ? removeMutation.variables?.id : undefined;

  const filtersActive =
    searchParams.toString() !== '' ||
    draftQ !== '' ||
    draftCourseId !== '' ||
    draftRating !== 0 ||
    draftFrom !== '' ||
    draftTo !== '' ||
    draftSort !== DEFAULT_SORT ||
    draftDir !== DEFAULT_DIR;

  return (
    <div className="wrap">
      <div className="page">
        <main className="ar-admin">
          <p className="ar-back">
            <Link to="/admin">{t('p10.nav.backToAdmin')}</Link>
          </p>
          <h1>{t('p10.reviews.title')}</h1>
          <p className="ar-subtitle muted">{t('p10.reviews.subtitle')}</p>

          <section className="panel" aria-label={t('p10.reviews.title')}>
            {/* ---------- Фильтры ---------- */}
            <form className="ar-filters" onSubmit={onApply} noValidate>
              <label className="ar-field ar-field--search" htmlFor="ar-search">
                <span>{t('p10.reviews.search')}</span>
                <input
                  id="ar-search"
                  className="input"
                  type="search"
                  value={draftQ}
                  placeholder={t('p10.reviews.search')}
                  onChange={(e) => setDraftQ(e.target.value)}
                />
              </label>

              <label className="ar-field" htmlFor="ar-filter-course">
                <span>{t('p10.reviews.filterCourse')}</span>
                <select
                  id="ar-filter-course"
                  className="input ar-select"
                  value={draftCourseId}
                  onChange={(e) => setDraftCourseId(e.target.value)}
                >
                  <option value="">{t('admin.allCourses')}</option>
                  {courseOptions.map((course) => (
                    <option key={course.id} value={course.id}>
                      {course.title}
                    </option>
                  ))}
                </select>
              </label>

              <label className="ar-field" htmlFor="ar-filter-rating">
                <span>{t('p10.reviews.filterRating')}</span>
                <select
                  id="ar-filter-rating"
                  className="input ar-select ar-select--rating"
                  value={draftRating}
                  onChange={(e) => setDraftRating(Number(e.target.value))}
                >
                  <option value={0}>{t('p10.reviews.allRatings')}</option>
                  {RATINGS.map((value) => (
                    <option key={value} value={value}>
                      {value} ★
                    </option>
                  ))}
                </select>
              </label>

              <label className="ar-field" htmlFor="ar-filter-from">
                <span>{t('p10.reviews.periodFrom')}</span>
                <input
                  id="ar-filter-from"
                  className="input ar-date"
                  type="date"
                  value={draftFrom}
                  aria-invalid={
                    draftFrom !== '' && !isIsoDate(draftFrom) ? true : undefined
                  }
                  onChange={(e) => setDraftFrom(e.target.value)}
                />
              </label>

              <label className="ar-field" htmlFor="ar-filter-to">
                <span>{t('p10.reviews.periodTo')}</span>
                <input
                  id="ar-filter-to"
                  className="input ar-date"
                  type="date"
                  value={draftTo}
                  aria-invalid={draftTo !== '' && !isIsoDate(draftTo) ? true : undefined}
                  onChange={(e) => setDraftTo(e.target.value)}
                />
              </label>

              {/* Поле и направление сортировки — одна группа «Сортировка»;
                  отдельного ключа «направление» в словаре нет. */}
              <fieldset className="ar-field ar-field--sort">
                <legend>{t('adminCourses.sort')}</legend>
                <div className="ar-sort-row">
                  <select
                    className="input ar-select"
                    value={draftSort}
                    aria-label={t('adminCourses.sort')}
                    onChange={(e) => {
                      if (isReviewSort(e.target.value)) setDraftSort(e.target.value);
                    }}
                  >
                    {SORTS.map((value) => (
                      <option key={value} value={value}>
                        {t(SORT_LABEL[value])}
                      </option>
                    ))}
                  </select>
                  <select
                    className="input ar-select"
                    value={draftDir}
                    aria-label={t(DIR_LABEL[draftDir])}
                    onChange={(e) => {
                      if (isReviewDir(e.target.value)) setDraftDir(e.target.value);
                    }}
                  >
                    {DIRS.map((value) => (
                      <option key={value} value={value}>
                        {t(DIR_LABEL[value])}
                      </option>
                    ))}
                  </select>
                </div>
              </fieldset>

              <div className="ar-filter-actions">
                <button type="submit" className="btn btn-primary">
                  {t('p8.checkout.apply')}
                </button>
                <button
                  type="button"
                  className="btn"
                  disabled={!filtersActive}
                  onClick={onReset}
                >
                  {t('p10.reviews.reset')}
                </button>
              </div>
            </form>

            {formError && (
              <p className="error" role="alert">
                {formError}
              </p>
            )}
            {periodInvalid && (
              <p className="error" role="alert">
                {t('admin.periodInvalid')}
              </p>
            )}

            {/* ---------- Итог и уведомления ---------- */}
            {data && (
              <p className="ar-total muted">
                {t('admin.total')}: {total}
              </p>
            )}
            <div aria-live="polite">
              {notice && (
                <p className="ar-notice" role="status">
                  {notice}
                </p>
              )}
              {actionError && (
                <p className="error ar-action-error" role="alert">
                  {actionError}
                </p>
              )}
            </div>

            {/* ---------- Состояния запроса ---------- */}
            {reviews.isLoading && (
              <p className="muted" role="status" aria-live="polite">
                {t('admin.loading')}
              </p>
            )}

            {reviews.isError && (
              <div className="ar-error" role="alert">
                <p className="error">{errorText(reviews.error)}</p>
                <button
                  type="button"
                  className="btn"
                  onClick={() => void reviews.refetch()}
                >
                  {t('common.retry')}
                </button>
              </div>
            )}

            {data && data.items.length === 0 && (
              <p className="muted">{t('p10.reviews.empty')}</p>
            )}

            {/* ---------- Таблица ---------- */}
            {data && data.items.length > 0 && (
              <div
                className="ar-table-wrap"
                role="region"
                aria-label={t('p10.reviews.title')}
                tabIndex={0}
              >
                <table className="ar-table">
                  <thead>
                    <tr>
                      <th scope="col">{t('p10.reviews.course')}</th>
                      <th scope="col">{t('p10.reviews.student')}</th>
                      <th scope="col">{t('p10.reviews.filterRating')}</th>
                      <th scope="col">{t('p10.reviews.body')}</th>
                      <th scope="col">{t('admin.colDate')}</th>
                      <th scope="col">{t('adminUsers.colActions')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.items.map((row) => {
                      const busy = pendingId === row.id;
                      const name = row.studentName.trim();
                      return (
                        <tr
                          key={row.id}
                          className={busy ? 'ar-row--busy' : undefined}
                          aria-busy={busy || undefined}
                        >
                          <td
                            className="ar-td-course"
                            data-label={t('p10.reviews.course')}
                            title={row.courseTitle}
                          >
                            <Link
                              to={`/courses/${encodeURIComponent(row.courseSlug)}`}
                              target="_blank"
                              rel="noreferrer"
                              title={t('p10.reviews.openCourse')}
                            >
                              {row.courseTitle}
                            </Link>
                          </td>
                          <td
                            className="ar-td-student"
                            data-label={t('p10.reviews.student')}
                          >
                            <span className="ar-student">
                              <b className="ar-student__name" title={studentLabel(row)}>
                                {name || row.studentEmail}
                              </b>
                              {name !== '' && (
                                <span
                                  className="ar-student__email"
                                  title={row.studentEmail}
                                >
                                  {row.studentEmail}
                                </span>
                              )}
                            </span>
                          </td>
                          <td data-label={t('p10.reviews.filterRating')}>
                            <span
                              className="ar-rating"
                              role="img"
                              aria-label={`${t('p10.reviews.filterRating')}: ${row.rating} / 5`}
                            >
                              <span className="ar-stars" aria-hidden="true">
                                {'★'.repeat(Math.max(0, Math.min(5, row.rating)))}
                                {'☆'.repeat(Math.max(0, 5 - row.rating))}
                              </span>
                              <span className="ar-rating-num" aria-hidden="true">
                                {row.rating}
                              </span>
                            </span>
                          </td>
                          <td
                            className="ar-td-body"
                            data-label={t('p10.reviews.body')}
                          >
                            {row.body.trim() !== '' ? (
                              <span title={row.body}>{row.body}</span>
                            ) : (
                              <span className="muted">—</span>
                            )}
                          </td>
                          <td data-label={t('admin.colDate')}>
                            {formatDate(row.createdAt)}
                          </td>
                          <td
                            className="ar-td-actions"
                            data-label={t('adminUsers.colActions')}
                          >
                            <button
                              type="button"
                              className="btn btn-sm ar-btn--danger"
                              disabled={busy}
                              onClick={() => onDelete(row)}
                            >
                              {busy ? t('common.loading') : t('p10.reviews.delete')}
                            </button>
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
              <div className="ar-pager">
                <button
                  type="button"
                  className="btn"
                  disabled={page <= 1 || reviews.isFetching}
                  onClick={() => goToPage(page - 1)}
                >
                  {t('admin.prev')}
                </button>
                <span className="muted">
                  {t('admin.showing')} {rangeFrom}–{rangeTo} {t('admin.of')} {total}
                </span>
                <button
                  type="button"
                  className="btn"
                  disabled={isLastPage || reviews.isFetching}
                  onClick={() => goToPage(page + 1)}
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
