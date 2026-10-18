// T27: раздел «Курсы» супер-админки (/admin/courses).
// Список курсов всех статусов (GET /admin/courses) с поиском, фильтром статуса
// и пагинацией; действия «В архив» / «Снять с публикации»; раскрывающаяся панель
// отзывов курса (GET /admin/courses/{id}/reviews, DELETE /admin/reviews/{id}).
// Контракт — Task/T25-admin-console.md.
import { Fragment, useEffect, useState } from 'react';
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { ApiError, formatDate, formatPrice } from '../api/client';
import type { CourseStatus } from '../api/client';
import {
  archiveAdminCourse,
  deleteReview,
  listAdminCourses,
  listCourseReviews,
  unpublishAdminCourse,
} from '../api/adminConsole';
import type { AdminCourseRow, AdminReviewRow } from '../api/adminConsole';
import { t } from '../i18n/t';
import type { RuKey } from '../i18n/t';
import CsvExportButton, { csvDateStamp } from '../components/CsvExportButton';
import '../styles/adminConsole.css';

/** Размер страницы: 20, как у остальных списков консоли (контракт T25). */
const PAGE_LIMIT = 20;

/** Пауза дебаунса поиска: пока пользователь печатает, запросы не уходят. */
const SEARCH_DEBOUNCE_MS = 300;

/** Размер страницы выгрузки CSV (T40): 100 строк за запрос. */
const EXPORT_PAGE_LIMIT = 100;

/** Потолок выгрузки CSV: не более 50 страниц (5000 строк) за один клик. */
const EXPORT_MAX_PAGES = 50;

/** Статусы курса в порядке контракта. */
const COURSE_STATUSES: CourseStatus[] = ['draft', 'review', 'published', 'archived'];

/** Русские подписи статусов — ключи конструктора (те же четыре статуса курса). */
const STATUS_LABEL: Record<CourseStatus, RuKey> = {
  draft: 'teach.statusDraft',
  review: 'teach.statusReview',
  published: 'teach.statusPublished',
  archived: 'teach.statusArchived',
};

/** Действие над курсом: в архив или снять с публикации. */
type CourseAction = 'archive' | 'unpublish';

function isCourseStatus(value: string): value is CourseStatus {
  return (COURSE_STATUSES as readonly string[]).includes(value);
}

/** Текст ошибки без краха страницы: 403/401/400/сеть/прочее (как в Admin.tsx). */
function errorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 403) return t('cabinet.needRole');
    if (error.status === 401) return t('error.needLogin');
    return error.message || t('common.error');
  }
  if (error instanceof TypeError) return t('error.network');
  return t('common.error');
}

/**
 * Раскрытая панель под строкой курса: отзывы курса и удаление отзыва.
 * Монтируется только при раскрытии — запрос уходит по факту показа.
 */
function ReviewsPanel({ courseId }: { courseId: string }): JSX.Element {
  const qc = useQueryClient();
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);

  const reviews = useQuery({
    queryKey: ['admin-course-reviews', courseId],
    queryFn: () => listCourseReviews(courseId),
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  const remove = useMutation({
    mutationFn: (id: string) => deleteReview(id),
    onSuccess: () => {
      setMsg({ ok: true, text: t('adminCourses.reviewRemoved') });
      // После удаления backend пересчитывает рейтинг курса — обновляем
      // и список отзывов, и сводку (средний рейтинг, количество отзывов).
      void qc.invalidateQueries({ queryKey: ['admin-course-reviews', courseId] });
      void qc.invalidateQueries({ queryKey: ['admin-stats'] });
    },
    onError: (e: unknown) => setMsg({ ok: false, text: errorText(e) }),
  });

  function onRemove(review: AdminReviewRow): void {
    if (!window.confirm(t('adminCourses.confirmRemoveReview'))) return;
    setMsg(null);
    remove.mutate(review.id);
  }

  const items = reviews.data?.items ?? [];

  return (
    <div className="ac-reviews">
      {msg && (
        <p
          role={msg.ok ? 'status' : 'alert'}
          className={msg.ok ? 'ac-msg ac-msg--ok' : 'ac-msg ac-msg--error'}
        >
          {msg.text}
        </p>
      )}

      {reviews.isLoading && (
        <p className="muted" role="status">
          {t('common.loading')}
        </p>
      )}

      {reviews.isError && (
        <div className="ac-error" role="alert">
          <p className="error">{errorText(reviews.error)}</p>
          <button
            type="button"
            className="btn btn-sm"
            onClick={() => void reviews.refetch()}
          >
            {t('common.retry')}
          </button>
        </div>
      )}

      {reviews.data && items.length === 0 && (
        <p className="muted">{t('adminCourses.reviewsEmpty')}</p>
      )}

      {items.length > 0 && (
        <ul className="ac-reviews__list">
          {items.map((review) => (
            <li key={review.id} className="ac-reviews__item">
              <div className="ac-reviews__head">
                <span
                  className="ac-stars"
                  role="img"
                  aria-label={`${t('reviews.ratingLabel')}: ${review.rating}`}
                >
                  {'★'.repeat(review.rating)}
                  {'☆'.repeat(Math.max(0, 5 - review.rating))}
                </span>
                <span className="muted">{review.studentEmail || review.studentId}</span>
                <span className="muted">{formatDate(review.createdAt)}</span>
                <button
                  type="button"
                  className="btn btn-sm ac-btn--danger"
                  disabled={remove.isPending}
                  onClick={() => onRemove(review)}
                >
                  {t('adminCourses.removeReview')}
                </button>
              </div>
              {review.body && <p className="ac-reviews__body">{review.body}</p>}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

export default function AdminCourses(): JSX.Element {
  const qc = useQueryClient();

  // ---- Фильтры и пагинация (состояние экрана, в URL не пишем) ----
  // Ввод в поиск держим отдельно: запрос уходит с дебаунсом 300 мс.
  const [searchInput, setSearchInput] = useState('');
  const [q, setQ] = useState('');
  const [status, setStatus] = useState<CourseStatus | ''>('');
  const [page, setPage] = useState(1);
  // Курс, для которого раскрыта панель отзывов (одна за раз).
  const [reviewsCourseId, setReviewsCourseId] = useState<string | null>(null);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setQ(searchInput.trim());
    }, SEARCH_DEBOUNCE_MS);
    return () => window.clearTimeout(timer);
  }, [searchInput]);

  // Смена фильтров всегда возвращает на первую страницу.
  useEffect(() => {
    setPage(1);
  }, [q, status]);

  const courses = useQuery({
    queryKey: ['admin-courses', { q, status, page }],
    queryFn: () =>
      listAdminCourses({
        q: q || undefined,
        status: status || undefined,
        page,
        limit: PAGE_LIMIT,
      }),
    // При смене страницы/фильтра старые строки остаются до прихода новых.
    placeholderData: keepPreviousData,
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  // ---- Действия над курсом ----
  const [actionMsg, setActionMsg] = useState<{ ok: boolean; text: string } | null>(null);

  const courseAction = useMutation({
    mutationFn: (args: { id: string; action: CourseAction }) =>
      args.action === 'archive' ? archiveAdminCourse(args.id) : unpublishAdminCourse(args.id),
    onSuccess: (_res, args) => {
      setActionMsg({
        ok: true,
        text:
          args.action === 'archive'
            ? t('adminCourses.archived')
            : t('adminCourses.unpublished'),
      });
      // Смена статуса курса видна и в списке, и в сводке платформы.
      void qc.invalidateQueries({ queryKey: ['admin-courses'] });
      void qc.invalidateQueries({ queryKey: ['admin-stats'] });
    },
    // 409 (уже в архиве / не published) приходит текстом ApiError.message.
    onError: (e: unknown) => setActionMsg({ ok: false, text: errorText(e) }),
  });

  function onCourseAction(course: AdminCourseRow, action: CourseAction): void {
    // Название курса подставляем сами: в словаре нет интерполяции (t.ts).
    const confirmText =
      action === 'archive'
        ? t('adminCourses.confirmArchive')
        : t('adminCourses.confirmUnpublish');
    const question = `${confirmText} «${course.title}»?`;
    if (!window.confirm(question)) return;
    setActionMsg(null);
    courseAction.mutate({ id: course.id, action });
  }

  // ---- Выгрузка CSV: вся выборка по текущим фильтрам ----
  // Идём постранично (limit=100) до total, но не более 50 страниц: один клик
  // выгружает не больше 5000 строк, чтобы не подвесить браузер и стенд.
  async function loadExportRows(): Promise<Array<Array<string | number | null>>> {
    const rows: Array<Array<string | number | null>> = [];
    for (let pageNumber = 1; pageNumber <= EXPORT_MAX_PAGES; pageNumber += 1) {
      const res = await listAdminCourses({
        q: q || undefined,
        status: status || undefined,
        page: pageNumber,
        limit: EXPORT_PAGE_LIMIT,
      });
      for (const course of res.items) {
        rows.push([
          course.slug,
          course.title,
          course.teacherEmail,
          t(STATUS_LABEL[course.status]),
          formatPrice(course.priceCents),
          course.modulesCount,
          course.lessonsCount,
          course.studentsCount,
          formatDate(course.createdAt),
          formatDate(course.updatedAt),
        ]);
      }
      // Неполная страница — данных больше нет.
      if (res.items.length < EXPORT_PAGE_LIMIT || rows.length >= res.total) break;
    }
    return rows;
  }

  // Заголовки CSV: где есть подходящий ключ — берём его, иначе короткая русская
  // подпись прямо в CSV («Slug», «Создан» — это данные, а не интерфейс).
  const exportHeaders = [
    'Slug',
    t('adminCourses.colTitle'),
    t('adminCourses.colTeacher'),
    t('adminCourses.colStatus'),
    t('adminCourses.colPrice'),
    t('course.modulesCount'),
    t('adminCourses.colLessons'),
    t('adminCourses.colStudents'),
    'Создан',
    t('adminCourses.colUpdated'),
  ];

  // ---- Производные значения пагинации ----
  const data = courses.data;
  const total = data?.total ?? 0;
  const rangeFrom = total === 0 ? 0 : Math.min((page - 1) * PAGE_LIMIT + 1, total);
  const rangeTo = Math.min(page * PAGE_LIMIT, total);
  const isLastPage = total > 0 && rangeTo >= total;

  return (
    <div className="wrap">
      <div className="page">
        <main className="ac-page">
          <div className="ac-head">
            <h1>{t('adminCourses.title')}</h1>
            <span className="muted">
              {t('adminCourses.total')}: {total}
            </span>
          </div>

          <div className="ac-toolbar">
            <label className="ac-field" htmlFor="ac-courses-search">
              <span>{t('adminCourses.search')}</span>
              <input
                id="ac-courses-search"
                className="input ac-search"
                type="search"
                value={searchInput}
                onChange={(e) => setSearchInput(e.target.value)}
                placeholder={t('adminCourses.search')}
              />
            </label>
            <label className="ac-field" htmlFor="ac-courses-status">
              <span>{t('adminCourses.filterStatus')}</span>
              <select
                id="ac-courses-status"
                className="input"
                value={status}
                onChange={(e) =>
                  setStatus(isCourseStatus(e.target.value) ? e.target.value : '')
                }
              >
                <option value="">{t('admin.allStatuses')}</option>
                {COURSE_STATUSES.map((s) => (
                  <option key={s} value={s}>
                    {t(STATUS_LABEL[s])}
                  </option>
                ))}
              </select>
            </label>

            {/* T40: выгрузка всей выборки по текущим фильтрам (q, status). */}
            <CsvExportButton
              filename={`courses-${csvDateStamp()}.csv`}
              headers={exportHeaders}
              loadRows={loadExportRows}
            />
          </div>

          {actionMsg && (
            <p
              role={actionMsg.ok ? 'status' : 'alert'}
              className={actionMsg.ok ? 'ac-msg ac-msg--ok' : 'ac-msg ac-msg--error'}
            >
              {actionMsg.text}
            </p>
          )}

          {courses.isLoading && (
            <p className="muted" role="status">
              {t('common.loading')}
            </p>
          )}

          {courses.isError && (
            <div className="ac-error" role="alert">
              <p className="error">{errorText(courses.error)}</p>
              <button
                type="button"
                className="btn btn-sm"
                onClick={() => void courses.refetch()}
              >
                {t('common.retry')}
              </button>
            </div>
          )}

          {data && data.items.length === 0 && (
            <p className="muted">{t('adminCourses.empty')}</p>
          )}

          {data && data.items.length > 0 && (
            <div className="ac-table-wrap">
              <table className="ac-table">
                <thead>
                  <tr>
                    <th scope="col">{t('adminCourses.colTitle')}</th>
                    <th scope="col">{t('adminCourses.colTeacher')}</th>
                    <th scope="col">{t('adminCourses.colStatus')}</th>
                    <th scope="col">{t('adminCourses.colPrice')}</th>
                    <th scope="col">{t('adminCourses.colStudents')}</th>
                    <th scope="col">{t('adminCourses.colLessons')}</th>
                    <th scope="col">{t('adminCourses.colUpdated')}</th>
                    {/* Ключа «Действия» у раздела курсов нет: adminUsers.colActions. */}
                    <th scope="col">{t('adminUsers.colActions')}</th>
                  </tr>
                </thead>
                <tbody>
                  {data.items.map((course) => {
                    const reviewsOpen = reviewsCourseId === course.id;
                    return (
                      <Fragment key={course.id}>
                        <tr>
                          <td
                            data-label={t('adminCourses.colTitle')}
                            className="ac-td-title"
                          >
                            {/* Название — ссылка на публичную страницу курса. */}
                            <Link
                              to={`/courses/${encodeURIComponent(course.slug)}`}
                              target="_blank"
                              rel="noreferrer"
                              title={t('adminCourses.open')}
                            >
                              {course.title}
                            </Link>
                          </td>
                          <td
                            data-label={t('adminCourses.colTeacher')}
                            title={course.teacherId}
                          >
                            {course.teacherEmail}
                          </td>
                          <td data-label={t('adminCourses.colStatus')}>
                            <span className={`ac-badge ac-badge--${course.status}`}>
                              {t(STATUS_LABEL[course.status])}
                            </span>
                          </td>
                          <td data-label={t('adminCourses.colPrice')}>
                            {/* Цена курса: 0 — «Бесплатно» (как в очереди модерации). */}
                            {formatPrice(course.priceCents)}
                          </td>
                          <td data-label={t('adminCourses.colStudents')}>
                            {course.studentsCount}
                          </td>
                          <td data-label={t('adminCourses.colLessons')}>
                            {course.lessonsCount}
                          </td>
                          <td data-label={t('adminCourses.colUpdated')}>
                            {formatDate(course.updatedAt)}
                          </td>
                          <td
                            data-label={t('adminUsers.colActions')}
                            className="ac-td-actions"
                          >
                            <div className="ac-actions">
                              <button
                                type="button"
                                className="btn btn-sm"
                                aria-expanded={reviewsOpen}
                                aria-controls={`ac-reviews-${course.id}`}
                                onClick={() =>
                                  setReviewsCourseId(reviewsOpen ? null : course.id)
                                }
                              >
                                {t('adminCourses.reviews')}
                              </button>
                              {course.status !== 'archived' && (
                                <button
                                  type="button"
                                  className="btn btn-sm"
                                  disabled={courseAction.isPending}
                                  onClick={() => onCourseAction(course, 'archive')}
                                >
                                  {t('adminCourses.archive')}
                                </button>
                              )}
                              {course.status === 'published' && (
                                <button
                                  type="button"
                                  className="btn btn-sm"
                                  disabled={courseAction.isPending}
                                  onClick={() => onCourseAction(course, 'unpublish')}
                                >
                                  {t('adminCourses.unpublish')}
                                </button>
                              )}
                            </div>
                          </td>
                        </tr>
                        {reviewsOpen && (
                          <tr className="ac-tr-reviews">
                            <td
                              colSpan={8}
                              id={`ac-reviews-${course.id}`}
                              className="ac-td-reviews"
                            >
                              <ReviewsPanel courseId={course.id} />
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

          {data && total > 0 && (
            <div className="ac-pager">
              <button
                type="button"
                className="btn btn-sm"
                disabled={page <= 1 || courses.isFetching}
                onClick={() => setPage(page - 1)}
              >
                {t('admin.prev')}
              </button>
              <span className="muted">
                {t('admin.showing')} {rangeFrom}–{rangeTo} {t('admin.of')} {total}
              </span>
              <button
                type="button"
                className="btn btn-sm"
                disabled={isLastPage || courses.isFetching}
                onClick={() => setPage(page + 1)}
              >
                {t('admin.next')}
              </button>
            </div>
          )}
        </main>
      </div>
    </div>
  );
}
