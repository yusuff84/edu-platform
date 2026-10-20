// T78: страница «Аналитика» кабинета преподавателя (/teach/stats).
// Данные — замороженный контракт P9 (Task/00-api-contract.md, T77):
// GET /api/v1/teacher/stats (сводка) и GET /api/v1/teacher/stats/courses
// (разбивка по курсам). Все суммы приходят в копейках и показываются через
// formatMoney; проценты и рейтинг — в русской записи.
import { useQuery } from '@tanstack/react-query';
import { Link, useNavigate } from 'react-router-dom';
import type { MouseEvent, ReactNode } from 'react';
import {
  ApiError,
  formatMoney,
  teacherCourseStats,
  teacherOverview,
} from '../api/client';
import type { CourseStatus, TeacherCourseStat } from '../api/client';
import { t } from '../i18n/t';
import { useSeo } from '../app/useSeo';
import type { RuKey } from '../i18n/t';
// Стили страницы изолированы в styles/teacherStats.css (префикс .tstats-).
import '../styles/teacherStats.css';

/** Подписи и классы статусов курса — те же ключи, что в конструкторе. */
const COURSE_STATUS_META: Record<CourseStatus, { label: RuKey; className: string }> = {
  draft: { label: 'teach.statusDraft', className: 'tstats-status--draft' },
  review: { label: 'teach.statusReview', className: 'tstats-status--review' },
  published: { label: 'teach.statusPublished', className: 'tstats-status--published' },
  archived: { label: 'teach.statusArchived', className: 'tstats-status--archived' },
};

/** Текст ошибки без краха страницы: 403/401/сеть/прочее (как в AdminStats). */
function errorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 403) return t('cabinet.needRole');
    if (error.status === 401) return t('error.needLogin');
    return error.message || t('common.error');
  }
  if (error instanceof TypeError) return t('error.network');
  return t('common.error');
}

/** Число с разделителями разрядов (1 234). */
function formatCount(value: number): string {
  if (!Number.isFinite(value)) return '—';
  return new Intl.NumberFormat('ru-RU').format(value);
}

/** Средний рейтинг: одна цифра после запятой; без оценок — прочерк. */
function formatRating(value: number, ratingsCount: number): string {
  if (ratingsCount <= 0 || !Number.isFinite(value)) return '—';
  return value.toLocaleString('ru-RU', {
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
  });
}

/** Процент 0..100: не больше одной цифры после запятой. */
function formatPercent(value: number): string {
  if (!Number.isFinite(value)) return '—';
  return `${value.toLocaleString('ru-RU', { maximumFractionDigits: 1 })}%`;
}

/** Ширина полосы прогресса: не выходим за 0..100 при неожиданном ответе. */
function clampPercent(value: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.min(100, Math.max(0, value));
}

/** Подпись статуса: неизвестное значение с сервера не роняет страницу. */
function statusMeta(status: string): { label: RuKey; className: string } | null {
  return COURSE_STATUS_META[status as CourseStatus] ?? null;
}

/** Строка «подпись — значение» внутри KPI-карточки. */
function KpiRow({ label, value }: { label: string; value: ReactNode }): JSX.Element {
  return (
    <div className="tstats-row">
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

/** KPI-карточка: заголовок и список показателей. */
function KpiCard({ title, children }: { title: string; children: ReactNode }): JSX.Element {
  return (
    <section className="tstats-card">
      <h2 className="tstats-card__title">{title}</h2>
      <dl className="tstats-rows">{children}</dl>
    </section>
  );
}

/** Строка таблицы «По курсам»; клик по строке и кнопка ведут в конструктор. */
function CourseRow({ row }: { row: TeacherCourseStat }): JSX.Element {
  const navigate = useNavigate();
  const meta = statusMeta(row.status);
  const to = `/teach/${row.courseId}`;

  /** Клик по ссылке/кнопке внутри строки не должен дублировать переход. */
  function stop(event: MouseEvent): void {
    event.stopPropagation();
  }

  return (
    <tr className="tstats-tr" onClick={() => navigate(to)}>
      <td className="tstats-td-title" data-label={t('p9.stats.course')}>
        <Link to={to} onClick={stop} title={row.title}>
          {row.title}
        </Link>
      </td>
      <td data-label={t('p9.stats.status')}>
        {meta ? (
          <span className={`tstats-status ${meta.className}`}>{t(meta.label)}</span>
        ) : (
          <span className="tstats-status tstats-status--draft">{row.status}</span>
        )}
      </td>
      <td className="tstats-num" data-label={t('p9.stats.studentsCol')}>
        {formatCount(row.students)}
      </td>
      <td className="tstats-num" data-label={t('p9.stats.revenueCol')}>
        {formatMoney(row.revenueCents)}
      </td>
      <td className="tstats-td-progress" data-label={t('p9.stats.avgPercentCol')}>
        <div
          className="tstats-progress"
          role="progressbar"
          aria-label={t('p9.stats.avgPercentCol')}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={Math.round(clampPercent(row.avgPercent))}
        >
          <span
            className="tstats-progress__bar"
            style={{ width: `${clampPercent(row.avgPercent)}%` }}
          />
        </div>
        <span className="tstats-percent">{formatPercent(row.avgPercent)}</span>
      </td>
      <td className="tstats-num" data-label={t('p9.stats.ratings')}>
        {formatRating(row.avgRating, row.ratingsCount)}
        {row.ratingsCount > 0 && (
          <span className="tstats-muted"> ({formatCount(row.ratingsCount)})</span>
        )}
      </td>
      <td className="tstats-td-action" data-label={t('p9.stats.open')}>
        <Link className="btn btn-sm" to={to} onClick={stop}>
          {t('p9.stats.open')}
        </Link>
      </td>
    </tr>
  );
}

export default function TeacherStats(): JSX.Element {
  useSeo('Аналитика преподавателя — Edu Platform');
  const overview = useQuery({
    queryKey: ['teacher-stats'],
    queryFn: teacherOverview,
    // 4xx — не временный сбой: повторяем только сетевые ошибки (как в AdminStats).
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  const courses = useQuery({
    queryKey: ['teacher-stats-courses', 50],
    queryFn: () => teacherCourseStats(50),
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  const data = overview.data;
  const courseItems = courses.data?.items ?? [];

  return (
    <div className="wrap">
      <main className="page tstats-page">
        <h1>{t('p9.stats.title')}</h1>

        {overview.isLoading && (
          <div className="tstats-grid" aria-busy="true">
            <span className="sr-only" role="status">
              {t('common.loading')}
            </span>
            {/* Скелетон: шесть карточек-заготовок, пока не пришёл ответ. */}
            {[0, 1, 2, 3, 4, 5].map((slot) => (
              <div key={slot} className="tstats-card" aria-hidden="true">
                <span className="tstats-skeleton tstats-skeleton--title" />
                <span className="tstats-skeleton" />
                <span className="tstats-skeleton" />
                <span className="tstats-skeleton tstats-skeleton--short" />
              </div>
            ))}
          </div>
        )}

        {overview.isError && (
          <div className="tstats-error" role="alert">
            <p className="error">{errorText(overview.error)}</p>
            <button
              type="button"
              className="btn btn-sm"
              onClick={() => void overview.refetch()}
            >
              {t('common.retry')}
            </button>
          </div>
        )}

        {data && (
          <div className="tstats-grid">
            <KpiCard title={t('p9.stats.courses')}>
              <KpiRow label={t('p9.stats.coursesCount')} value={formatCount(data.courses.total)} />
              <KpiRow
                label={t('adminStats.statusPublished')}
                value={formatCount(data.courses.published)}
              />
            </KpiCard>

            <KpiCard title={t('p9.stats.students')}>
              <KpiRow
                label={t('p9.stats.enrollments')}
                value={formatCount(data.students.total)}
              />
              <KpiRow
                label={t('p9.stats.uniqueStudents')}
                value={formatCount(data.students.unique)}
              />
            </KpiCard>

            <KpiCard title={t('p9.stats.revenue')}>
              <KpiRow
                label={t('p9.stats.totalPaid')}
                value={formatMoney(data.revenue.paidTotalCents)}
              />
              <KpiRow
                label={t('p9.stats.net')}
                value={formatMoney(data.revenue.teacherCents)}
              />
              <KpiRow
                label={t('p9.stats.platformFee')}
                value={formatMoney(data.revenue.platformFeeCents)}
              />
            </KpiCard>

            <KpiCard title={t('p9.stats.completion')}>
              <KpiRow
                label={t('p9.stats.completed')}
                value={formatCount(data.completion.completed)}
              />
              <KpiRow
                label={t('p9.stats.avgPercent')}
                value={formatPercent(data.completion.avgPercent)}
              />
            </KpiCard>

            <KpiCard title={t('p9.stats.ratings')}>
              <KpiRow
                label={t('adminStats.avgRating')}
                value={formatRating(data.ratings.avgRating, data.ratings.ratingsCount)}
              />
              <KpiRow
                label={t('adminStats.reviewsTotal')}
                value={formatCount(data.ratings.ratingsCount)}
              />
            </KpiCard>

            <KpiCard title={t('p9.stats.last30d')}>
              <KpiRow
                label={t('p9.stats.enrollments')}
                value={formatCount(data.last30d.enrollments)}
              />
              <KpiRow
                label={t('p9.stats.revenue')}
                value={formatMoney(data.last30d.revenueCents)}
              />
            </KpiCard>
          </div>
        )}

        <section className="tstats-section" aria-labelledby="tstats-per-course">
          <h2 id="tstats-per-course" className="tstats-section__title">
            {t('p9.stats.perCourse')}
          </h2>

          {courses.isLoading && (
            <p role="status" className="tstats-muted">
              {t('common.loading')}
            </p>
          )}

          {courses.isError && (
            <div className="tstats-error" role="alert">
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

          {courses.data && courseItems.length === 0 && (
            <div className="tstats-empty">{t('p9.stats.noCourses')}</div>
          )}

          {courseItems.length > 0 && (
            <div className="tstats-table-wrap">
              <table className="tstats-table">
                <thead>
                  <tr>
                    <th scope="col">{t('p9.stats.course')}</th>
                    <th scope="col">{t('p9.stats.status')}</th>
                    <th scope="col">{t('p9.stats.studentsCol')}</th>
                    <th scope="col">{t('p9.stats.revenueCol')}</th>
                    <th scope="col">{t('p9.stats.avgPercentCol')}</th>
                    <th scope="col">{t('p9.stats.ratings')}</th>
                    <th scope="col">
                      <span className="sr-only">{t('p9.stats.open')}</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {courseItems.map((row) => (
                    <CourseRow key={row.courseId} row={row} />
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
      </main>
    </div>
  );
}
