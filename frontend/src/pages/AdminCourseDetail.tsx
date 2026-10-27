// T58: админ-страница «Детали курса» (/admin/courses/:id) — волна P7.
// Данные: GET /admin/courses/{id} (контракт T55 из Task/00-api-contract.md,
// ЗАМОРОЖЕН) + существующие API T25/T49: отзывы курса — listCourseReviews/
// deleteReview (adminConsole.ts), записи на курс — listAdminEnrollments
// с courseId-фильтром (adminEnrollments.ts). Все i18n-подписи — ключи
// p7.adminCourse.* и уже существующие ключи словаря; новых ключей нет.
// Стили изолированы в styles/adminCourseDetail.css (префикс .acd-).
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link, useParams } from 'react-router-dom';
import {
  ApiError,
  formatDate,
  formatMoney,
  formatPrice,
  moderateCourse,
} from '../api/client';
import {
  archiveAdminCourse,
  deleteReview,
  listCourseReviews,
  publishAdminCourse,
  unarchiveAdminCourse,
  unpublishAdminCourse,
} from '../api/adminConsole';
import type { AdminReviewRow } from '../api/adminConsole';
import { listAdminCourseDetail } from '../api/adminCourseDetail';
import { listAdminEnrollments } from '../api/adminEnrollments';
import { coverUrl } from '../api/teacherCover';
import type {
  AdminEnrollmentRow,
  AdminEnrollmentStatus,
} from '../api/adminEnrollments';
import { t } from '../i18n/t';
import type { RuKey } from '../i18n/t';
import { useSeo } from '../app/useSeo';
import '../styles/adminCourseDetail.css';

/** Размер страницы списка записей (потолок контрактного limit — 100). */
const ENROLLMENTS_LIMIT = 100;

/**
 * Ссылка на «Записи на курсы» с сохранённым фильтром courseId.
 * Один helper для ссылки в действиях и в блоке учеников — фильтр не теряется.
 */
export function courseEnrollmentsPath(courseId: string): string {
  return `/admin/enrollments?courseId=${encodeURIComponent(courseId)}`;
}

/** Русские подписи статуса курса — те же ключи, что в строке AdminCourses (T48). */
const STATUS_LABEL: Record<string, RuKey> = {
  draft: 'teach.statusDraft',
  review: 'teach.statusReview',
  published: 'teach.statusPublished',
  archived: 'teach.statusArchived',
};

/** Русские подписи статуса записи на курс (как в AdminEnrollments, T49). */
const ENROLLMENT_STATUS_LABEL: Record<AdminEnrollmentStatus, RuKey> = {
  active: 'adminEnrollments.statusActive',
  refunded: 'adminEnrollments.statusRefunded',
  revoked: 'adminEnrollments.statusRevoked',
};

/** Действие супер-админа над курсом (T88). */
type CourseAction =
  | 'approve'
  | 'reject'
  | 'forcePublish'
  | 'unpublish'
  | 'archive'
  | 'unarchive';

/** Тост после успешного действия — только существующие ключи словаря. */
const ACTION_DONE: Record<CourseAction, RuKey> = {
  approve: 'adminCourses.approvedDone',
  reject: 'adminCourses.rejectedDone',
  forcePublish: 'p10.courseAdmin.forcePublishDone',
  unpublish: 'adminCourses.unpublished',
  archive: 'adminCourses.archived',
  unarchive: 'adminCourses.unarchivedDone',
};

/** Текст ошибки без краха страницы: 403/401/400/сеть/прочее (как в AdminCourses). */
function errorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 403) return t('cabinet.needRole');
    if (error.status === 401) return t('error.needLogin');
    return error.message || t('common.error');
  }
  if (error instanceof TypeError) return t('error.network');
  return t('common.error');
}

/** Средний рейтинг: одна цифра после запятой в русской записи (как в AdminStats). */
function formatRating(value: number): string {
  return value.toLocaleString('ru-RU', {
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
  });
}

/** Процент прогресса в границах 0..100 (как в AdminEnrollments). */
function clampPercent(value: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.min(100, Math.max(0, Math.round(value)));
}

/** Имя ученика для ячеек: имя (email), либо только email (как в AdminEnrollments). */
function studentLabel(row: AdminEnrollmentRow): string {
  const name = row.studentName.trim();
  if (!name) return row.studentEmail;
  return `${name} (${row.studentEmail})`;
}

/** Карточка-число блока «Показатели». */
function StatCard({ label, value }: { label: string; value: string | number }): JSX.Element {
  return (
    <div className="acd-stat">
      <span className="acd-stat__label">{label}</span>
      <span className="acd-stat__value">{value}</span>
    </div>
  );
}

export default function AdminCourseDetail(): JSX.Element {
  const qc = useQueryClient();
  const params = useParams();
  // Пустой id невозможен под маршрутом /admin/courses/:id, но guard-условие
  // держит запросы выключенными до подтверждения параметра.
  const id = params.id ?? '';
  const ready = id !== '';

  // ---- Уведомления об операциях (действия над курсом и удаление отзыва) ----
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);
  // Отзыв, у которого открыто inline-подтверждение удаления (НЕ window.confirm).
  const [confirmId, setConfirmId] = useState<string | null>(null);
  // T88: причина отклонения при модерации прямо со страницы курса.
  const [moderationText, setModerationText] = useState('');

  // ---- Детали курса (T55) ----
  const detail = useQuery({
    queryKey: ['admin-course', id],
    queryFn: () => listAdminCourseDetail(id),
    enabled: ready,
    // 403/404 — не временный сбой: ретраим только сетевые ошибки.
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  // T88: заголовок вкладки — название курса, пока он загружается/не найден.
  useSeo(
    detail.data
      ? `${detail.data.course.title} — ${t('adminCourses.title')} — Edu Platform`
      : `${t('p7.adminCourse.info')} — Edu Platform`,
  );

  // ---- Записи на этот курс (T49 API, компактная таблица) ----
  const enrollments = useQuery({
    queryKey: ['admin-course-enrollments', id],
    queryFn: () => listAdminEnrollments({ courseId: id, limit: ENROLLMENTS_LIMIT }),
    enabled: ready,
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  // ---- Отзывы курса (T25 API) ----
  const reviews = useQuery({
    queryKey: ['admin-course-reviews', id],
    queryFn: () => listCourseReviews(id),
    enabled: ready,
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  // ---- Удаление отзыва с inline-подтверждением и refetch ----
  const remove = useMutation({
    mutationFn: (reviewId: string) => deleteReview(reviewId),
    onSuccess: () => {
      setConfirmId(null);
      setMsg({ ok: true, text: t('adminCourses.reviewRemoved') });
      // Рейтинг и количество отзывов пересчитываются на backend — обновляем
      // и список отзывов, и детали (stats), и сводку платформы.
      void qc.invalidateQueries({ queryKey: ['admin-course-reviews', id] });
      void qc.invalidateQueries({ queryKey: ['admin-course', id] });
      void qc.invalidateQueries({ queryKey: ['admin-stats'] });
    },
    onError: (e: unknown) => setMsg({ ok: false, text: errorText(e) }),
  });

  function askRemove(review: AdminReviewRow): void {
    setMsg(null);
    setConfirmId(review.id);
  }

  function cancelRemove(): void {
    setConfirmId(null);
  }

  function confirmRemove(reviewId: string): void {
    setMsg(null);
    remove.mutate(reviewId);
  }

  // ---- T88: действия супер-админа над курсом ----
  const courseAction = useMutation({
    // Формы ответов у действий разные (Course и {course}); результат не нужен —
    // важны завершение запроса, обновление detail и текст ошибки.
    mutationFn: (args: { action: CourseAction; comment?: string }): Promise<unknown> => {
      switch (args.action) {
        case 'approve':
          return moderateCourse(id, { decision: 'approve' });
        case 'reject':
          return moderateCourse(id, {
            decision: 'reject',
            comment: args.comment ?? '',
          });
        case 'forcePublish':
          return publishAdminCourse(id);
        case 'unpublish':
          return unpublishAdminCourse(id);
        case 'archive':
          return archiveAdminCourse(id);
        case 'unarchive':
          return unarchiveAdminCourse(id);
      }
    },
    onSuccess: (_res, args) => {
      if (args.action === 'approve' || args.action === 'reject') {
        setModerationText('');
      }
      setMsg({ ok: true, text: t(ACTION_DONE[args.action]) });
      // Статус и комментарий модерации видны в деталях, списке, сводке и
      // конструкторе (админский режим) — обновляем все связанные кэши.
      void qc.invalidateQueries({ queryKey: ['admin-course', id] });
      void qc.invalidateQueries({ queryKey: ['admin-courses'] });
      void qc.invalidateQueries({ queryKey: ['admin-stats'] });
      void qc.invalidateQueries({ queryKey: ['review-queue'] });
      void qc.invalidateQueries({ queryKey: ['teach-course', id] });
      void qc.invalidateQueries({ queryKey: ['teaching'] });
    },
    // 409 (курс не в том статусе) и 404 приходят текстом ApiError.message.
    onError: (e: unknown) => setMsg({ ok: false, text: errorText(e) }),
  });

  /** Подтверждение действия: нативный confirm — как в списке курсов T50. */
  function confirmAction(action: CourseAction, title: string): boolean {
    if (action === 'approve') return window.confirm(t('adminCourses.confirmApprove'));
    if (action === 'forcePublish') {
      return window.confirm(t('p10.courseAdmin.forcePublishConfirm'));
    }
    if (action === 'unarchive') return window.confirm(t('teach.confirmUnarchive'));
    if (action === 'archive') {
      return window.confirm(`${t('adminCourses.confirmArchive')}? «${title}»`);
    }
    if (action === 'unpublish') {
      return window.confirm(`${t('adminCourses.confirmUnpublish')}? «${title}»`);
    }
    // reject не подтверждаем: причина уже введена в поле рядом.
    return true;
  }

  function onCourseAction(action: CourseAction, title: string): void {
    const comment = moderationText.trim();
    // Без причины backend ответит 400 — ловим на клиенте, кнопка тоже disabled.
    if (action === 'reject' && !comment) return;
    if (!confirmAction(action, title)) return;
    setMsg(null);
    courseAction.mutate(
      action === 'reject' ? { action, comment } : { action },
    );
  }

  // 404 — фактическое отсутствие курса: отдельный экран (p7.adminCourse.notFound).
  const notFound = detail.isError && detail.error instanceof ApiError && detail.error.status === 404;

  const data = detail.data;
  const stats = data?.stats;
  // Уроков в курсе = сумма уроков модулей (тот же смысл, что в списке курсов).
  const lessonsTotal = data ? data.modules.reduce((n, m) => n + m.lessons.length, 0) : 0;
  const reviewItems = reviews.data?.items ?? [];
  const enrollmentRows = enrollments.data?.items ?? [];
  const enrollmentsTotal = enrollments.data?.total ?? 0;
  // coverPath — путь хранения; coverUrl учитывает абсолютный BASE API (T57).
  const cover = data ? coverUrl(data.course.coverPath) : undefined;

  return (
    <div className="wrap">
      <div className="page">
        <main className="acd-page">
          <p className="acd-back">
            <Link to="/admin/courses">{t('p7.adminCourse.back')}</Link>
            {' · '}
            <Link to="/admin">{t('p10.courseAdmin.backToAdmin')}</Link>
          </p>
          <h1 className="sr-only">{t('p7.adminCourse.info')}</h1>

          {detail.isLoading && (
            <p className="muted" role="status" aria-live="polite">
              {t('common.loading')}
            </p>
          )}

          {!detail.isLoading && notFound && (
            <div className="acd-notfound" role="alert">
              <h2>{t('p7.adminCourse.notFound')}</h2>
              <Link className="btn" to="/admin/courses">
                {t('p7.adminCourse.back')}
              </Link>
            </div>
          )}

          {!notFound && detail.isError && (
            <div className="acd-error" role="alert">
              <p className="error">{errorText(detail.error)}</p>
              <button
                type="button"
                className="btn"
                onClick={() => void detail.refetch()}
              >
                {t('common.retry')}
              </button>
            </div>
          )}

          {data && stats && (
            <>
              {/* ---------- Шапка: статус/цена/автор + быстрые ссылки ---------- */}
              <header className="acd-head">
                <div className="acd-head__main">
                  {cover && (
                    <img className="acd-cover" src={cover} alt={t('p7.cover.alt')} />
                  )}
                  <div>
                    <h2 className="acd-head__title">{data.course.title}</h2>
                    <div className="acd-head__meta">
                      <span className={`acd-badge acd-badge--${data.course.status}`}>
                        {t(STATUS_LABEL[data.course.status] ?? 'teach.statusDraft')}
                      </span>
                      <span>
                        {t('course.price')}: <b>{formatPrice(data.course.priceCents)}</b>
                      </span>
                      <span>
                        {t('course.teacher')}:{' '}
                        <b title={data.course.teacherEmail}>
                          {data.course.teacherEmail || '—'}
                        </b>
                      </span>
                    </div>
                  </div>
                </div>
              </header>

              {msg && (
                <p
                  role={msg.ok ? 'status' : 'alert'}
                  className={msg.ok ? 'acd-msg acd-msg--ok' : 'acd-msg acd-msg--error'}
                >
                  {msg.text}
                </p>
              )}

              {/* T87: комментарий модерации из GET /admin/courses/{id}. */}
              {data.course.moderationComment && (
                <p className="acd-moderation-comment" role="note">
                  <strong>{t('teach.moderationComment')}:</strong>{' '}
                  {data.course.moderationComment}
                </p>
              )}

              {/* ---------- T88: действия супер-админа ---------- */}
              <section className="acd-block acd-actions" aria-labelledby="acd-actions-title">
                <h3 id="acd-actions-title" className="acd-block__title">
                  {t('p10.courseAdmin.actions')}
                </h3>

                <div className="acd-actions__links">
                  {data.course.status === 'archived' ? (
                    // Архивный курс нельзя редактировать даже админу (T87):
                    // ссылка неактивна, причина — в подсказке ниже.
                    <span
                      className="btn btn-sm acd-btn--disabled"
                      aria-disabled="true"
                      title={t('p10.courseAdmin.contentLockedArchived')}
                    >
                      {t('p10.courseAdmin.edit')}
                    </span>
                  ) : (
                    <Link
                      className="btn btn-sm btn-primary"
                      to={`/teach/${encodeURIComponent(data.course.id)}`}
                      title={t('p10.courseAdmin.editHint')}
                    >
                      {t('p10.courseAdmin.edit')}
                    </Link>
                  )}
                  <Link
                    className="btn btn-sm"
                    to={`/courses/${encodeURIComponent(data.course.slug)}`}
                    target="_blank"
                    rel="noreferrer"
                  >
                    {t('p10.courseAdmin.openPublic')}
                  </Link>
                  <Link
                    className="btn btn-sm"
                    to={courseEnrollmentsPath(data.course.id)}
                  >
                    {t('p10.courseAdmin.students')}
                  </Link>
                </div>

                {data.course.status === 'archived' && (
                  <p className="muted acd-actions__hint">
                    {t('p10.courseAdmin.contentLockedArchived')}
                  </p>
                )}

                {data.course.status === 'review' && (
                  <div className="acd-actions__group">
                    <h4>{t('p10.courseAdmin.moderate')}</h4>
                    <label className="acd-actions__comment" htmlFor="acd-moderation-comment">
                      <span>{t('p10.courseAdmin.rejectComment')}</span>
                      <textarea
                        id="acd-moderation-comment"
                        className="input"
                        rows={3}
                        value={moderationText}
                        aria-describedby="acd-moderation-comment-hint"
                        onChange={(e) => setModerationText(e.target.value)}
                      />
                    </label>
                    <p id="acd-moderation-comment-hint" className="muted acd-actions__hint">
                      {t('admin.rejectCommentRequired')}
                    </p>
                    <div className="acd-actions__row">
                      <button
                        type="button"
                        className="btn btn-sm btn-primary"
                        disabled={courseAction.isPending}
                        onClick={() => onCourseAction('approve', data.course.title)}
                      >
                        {t('p10.courseAdmin.approve')}
                      </button>
                      <button
                        type="button"
                        className="btn btn-sm acd-btn--danger"
                        disabled={courseAction.isPending || moderationText.trim() === ''}
                        title={
                          moderationText.trim() === ''
                            ? t('admin.rejectCommentRequired')
                            : undefined
                        }
                        onClick={() => onCourseAction('reject', data.course.title)}
                      >
                        {t('p10.courseAdmin.reject')}
                      </button>
                    </div>
                  </div>
                )}

                <div className="acd-actions__group">
                  <div className="acd-actions__row">
                    {(data.course.status === 'draft' || data.course.status === 'review') && (
                      <button
                        type="button"
                        className="btn btn-sm"
                        disabled={courseAction.isPending}
                        onClick={() => onCourseAction('forcePublish', data.course.title)}
                      >
                        {t('p10.courseAdmin.forcePublish')}
                      </button>
                    )}
                    {data.course.status === 'published' && (
                      <button
                        type="button"
                        className="btn btn-sm"
                        disabled={courseAction.isPending}
                        onClick={() => onCourseAction('unpublish', data.course.title)}
                      >
                        {t('adminCourses.unpublish')}
                      </button>
                    )}
                    {data.course.status !== 'archived' && (
                      <button
                        type="button"
                        className="btn btn-sm acd-btn--danger"
                        disabled={courseAction.isPending}
                        onClick={() => onCourseAction('archive', data.course.title)}
                      >
                        {t('adminCourses.archive')}
                      </button>
                    )}
                    {data.course.status === 'archived' && (
                      <button
                        type="button"
                        className="btn btn-sm"
                        disabled={courseAction.isPending}
                        onClick={() => onCourseAction('unarchive', data.course.title)}
                      >
                        {t('adminCourses.unarchive')}
                      </button>
                    )}
                  </div>
                </div>
              </section>

              {/* ---------- Показатели ---------- */}
              <section className="acd-block" aria-labelledby="acd-stats-title">
                <h3 id="acd-stats-title" className="acd-block__title">
                  {t('p7.adminCourse.stats')}
                </h3>
                <div className="acd-stats">
                  <StatCard label={t('p7.adminCourse.studentsCount')} value={stats.studentsCount} />
                  {/* Ключа p7.adminCourse.* под activeCount нет — существующий adminStats.active. */}
                  <StatCard label={t('adminStats.active')} value={stats.activeCount} />
                  <StatCard label={t('p7.adminCourse.lessonsCount')} value={lessonsTotal} />
                  <StatCard label={t('p7.adminCourse.reviewsCount')} value={stats.reviewsCount} />
                  <StatCard label={t('adminStats.avgRating')} value={formatRating(stats.avgRating)} />
                  {/* Ключа под paymentsCount нет — существующий adminStats.payments. */}
                  <StatCard label={t('adminStats.payments')} value={stats.paymentsCount} />
                  <StatCard
                    label={t('p7.adminCourse.paidTotal')}
                    value={formatMoney(stats.paidTotalCents)}
                  />
                </div>
              </section>

              {/* ---------- Программа ---------- */}
              <section className="acd-block" aria-labelledby="acd-program-title">
                <h3 id="acd-program-title" className="acd-block__title">
                  {t('p7.adminCourse.program')}
                </h3>
                {data.modules.length === 0 ? (
                  <p className="muted">{t('admin.reviewEmpty')}</p>
                ) : (
                  <ul className="acd-modules">
                    {data.modules.map((module) => (
                      <li key={module.id} className="acd-module">
                        <div className="acd-module__head">
                          <span className="acd-module__pos">#{module.position}</span>
                          <span className="acd-module__title">{module.title}</span>
                        </div>
                        {module.lessons.length > 0 && (
                          <ul className="acd-lessons">
                            {module.lessons.map((lesson) => (
                              <li key={lesson.id} className="acd-lesson">
                                <span className="acd-lesson__pos">#{lesson.position}</span>
                                <span className="acd-kind">{t(`kind.${lesson.kind}`)}</span>
                                <span className="acd-lesson__title">{lesson.title}</span>
                              </li>
                            ))}
                          </ul>
                        )}
                      </li>
                    ))}
                  </ul>
                )}
              </section>

              {/* ---------- Ученики: компактная таблица записей курса ---------- */}
              <section className="acd-block" aria-labelledby="acd-students-title">
                <h3 id="acd-students-title" className="acd-block__title">
                  {t('p7.adminCourse.students')}
                </h3>
                <p className="acd-block__link muted">
                  <Link to={courseEnrollmentsPath(id)}>{t('adminEnrollments.title')}</Link>
                </p>

                {enrollments.isLoading && (
                  <p className="muted" role="status">
                    {t('common.loading')}
                  </p>
                )}

                {enrollments.isError && (
                  <div className="acd-error" role="alert">
                    <p className="error">{errorText(enrollments.error)}</p>
                    <button
                      type="button"
                      className="btn btn-sm"
                      onClick={() => void enrollments.refetch()}
                    >
                      {t('common.retry')}
                    </button>
                  </div>
                )}

                {enrollments.data && enrollmentRows.length === 0 && (
                  <p className="muted">{t('adminEnrollments.empty')}</p>
                )}

                {enrollmentRows.length > 0 && (
                  <table className="acd-table">
                    <thead>
                      <tr>
                        <th scope="col">{t('adminEnrollments.colStudent')}</th>
                        <th scope="col">{t('adminEnrollments.colStatus')}</th>
                        <th scope="col">{t('adminEnrollments.colProgress')}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {enrollmentRows.map((row) => {
                        const percent = clampPercent(row.percent);
                        const name = row.studentName.trim();
                        return (
                          <tr key={row.id}>
                            <td>
                              <span className="acd-student" title={studentLabel(row)}>
                                <span className="acd-student__name">
                                  {name || row.studentEmail}
                                </span>
                                <span className="acd-student__email">{row.studentEmail}</span>
                              </span>
                            </td>
                            <td>
                              <span className={`acd-ebadge acd-ebadge--${row.status}`}>
                                {t(ENROLLMENT_STATUS_LABEL[row.status])}
                              </span>
                            </td>
                            <td>
                              <span
                                className="acd-progress"
                                aria-label={t('adminEnrollments.colProgress')}
                              >
                                <span
                                  className="progress"
                                  role="progressbar"
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
                                {/* Прогресс: percent по контракту записи (T49). */}
                                <span className="acd-progress__num">{percent}%</span>
                              </span>
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                )}

                {/* Лимит контракта — 100 строк: честно говорим, что показано
                    не всё, и уводим на полный список с фильтром courseId. */}
                {enrollments.data && enrollmentsTotal > ENROLLMENTS_LIMIT && (
                  <p className="acd-block__link muted">
                    Первые {ENROLLMENTS_LIMIT} из {enrollmentsTotal}.{' '}
                    <Link to={courseEnrollmentsPath(id)}>Открыть все записи</Link>
                  </p>
                )}
              </section>

              {/* ---------- Отзывы: список + удаление с inline-подтверждением ---------- */}
              <section className="acd-block" aria-labelledby="acd-reviews-title">
                <h3 id="acd-reviews-title" className="acd-block__title">
                  {t('p7.adminCourse.reviews')}
                </h3>

                {reviews.isLoading && (
                  <p className="muted" role="status">
                    {t('common.loading')}
                  </p>
                )}

                {reviews.isError && (
                  <div className="acd-error" role="alert">
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

                {reviews.data && reviewItems.length === 0 && (
                  <p className="muted">{t('adminCourses.reviewsEmpty')}</p>
                )}

                {reviewItems.length > 0 && (
                  <ul className="acd-reviews">
                    {reviewItems.map((review) => {
                      // Inline-подтверждение открытого удаления: вопрос и две кнопки
                      // прямо в строке отзыва (нативный confirm() не используется).
                      const confirming = confirmId === review.id;
                      return (
                        <li key={review.id} className="acd-review">
                          <div className="acd-review__head">
                            <span
                              className="acd-stars"
                              role="img"
                              aria-label={`${t('reviews.ratingLabel')}: ${review.rating}`}
                            >
                              {'★'.repeat(review.rating)}
                              {'☆'.repeat(Math.max(0, 5 - review.rating))}
                            </span>
                            <span className="muted acd-review__author">
                              {review.studentEmail || review.studentId}
                            </span>
                            <span className="muted">{formatDate(review.createdAt)}</span>
                            {confirming ? (
                              <span className="acd-confirm">
                                <span className="acd-confirm__q">
                                  {t('adminCourses.confirmRemoveReview')}
                                </span>
                                <button
                                  type="button"
                                  className="btn btn-sm acd-btn--danger"
                                  disabled={remove.isPending}
                                  onClick={() => confirmRemove(review.id)}
                                >
                                  {remove.isPending ? t('common.loading') : t('common.confirm')}
                                </button>
                                <button
                                  type="button"
                                  className="btn btn-sm"
                                  disabled={remove.isPending}
                                  onClick={cancelRemove}
                                >
                                  {t('common.cancel')}
                                </button>
                              </span>
                            ) : (
                              <button
                                type="button"
                                className="btn btn-sm acd-btn--danger"
                                disabled={remove.isPending}
                                onClick={() => askRemove(review)}
                              >
                                {t('adminCourses.removeReview')}
                              </button>
                            )}
                          </div>
                          {review.body && <p className="acd-review__body">{review.body}</p>}
                        </li>
                      );
                    })}
                  </ul>
                )}
              </section>
            </>
          )}
        </main>
      </div>
    </div>
  );
}
