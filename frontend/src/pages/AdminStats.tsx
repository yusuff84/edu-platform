// T27: раздел «Сводка» супер-админки (/admin/stats).
// Данные — GET /admin/stats (контракт Task/T25-admin-console.md): пять карточек
// (пользователи, курсы, платежи, записи, отзывы). Все денежные суммы приходят
// в копейках и показываются ТОЛЬКО через formatMoney.
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { ApiError, formatMoney } from '../api/client';
import { getAdminStats } from '../api/adminConsole';
import { t } from '../i18n/t';
import type { RuKey } from '../i18n/t';
// Стили экранов консоли изолированы в styles/adminConsole.css (префикс .ac-).
import '../styles/adminConsole.css';

/** Текст ошибки без краха страницы: 403/401/сеть/прочее (как в Admin.tsx). */
function errorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 403) return t('cabinet.needRole');
    if (error.status === 401) return t('error.needLogin');
    return error.message || t('common.error');
  }
  if (error instanceof TypeError) return t('error.network');
  return t('common.error');
}

/** Средний рейтинг: одна цифра после запятой в русской записи (4,5). */
function formatRating(value: number): string {
  return value.toLocaleString('ru-RU', {
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
  });
}

/**
 * T59: быстрые переходы из сводки в разделы супер-админки. Пользователи,
 * курсы и записи — отдельные страницы; журнал платежей и очередь модерации —
 * секции /admin, у Admin.tsx есть якоря (section id="journal"/"queue"),
 * поэтому ведём по ним: после перезагрузки /admin#queue браузер отскроллит
 * к нужной секции (html { scroll-behavior: smooth } из styles.css).
 */
const QUICK_LINKS: Array<{ label: RuKey; to: string }> = [
  { label: 'p7.stats.linksUsers', to: '/admin/users' },
  { label: 'p7.stats.linksCourses', to: '/admin/courses' },
  { label: 'p7.stats.linksEnrollments', to: '/admin/enrollments' },
  { label: 'p7.stats.linksJournal', to: '/admin#journal' },
  { label: 'p7.stats.linksQueue', to: '/admin#queue' },
];

/** Строка «подпись — значение» внутри карточки сводки. */
function StatRow({ label, value }: { label: string; value: string | number }): JSX.Element {
  return (
    <div className="ac-row">
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

export default function AdminStats(): JSX.Element {
  const stats = useQuery({
    // Ключ совпадает с инвалидацией в AdminCourses.tsx: действия над курсами
    // и удаление отзыва обновляют и сводку.
    queryKey: ['admin-stats'],
    queryFn: getAdminStats,
    // 4xx — не временный сбой: повторяем только сетевые ошибки (как в Admin.tsx).
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  const data = stats.data;

  return (
    <div className="wrap">
      <div className="page">
        <main className="ac-page">
          <div className="ac-head">
            <h1>{t('adminStats.title')}</h1>
            <button
              type="button"
              className="btn btn-sm"
              onClick={() => void stats.refetch()}
              disabled={stats.isFetching}
            >
              {t('adminStats.refresh')}
            </button>
          </div>

          {/* T59: быстрые переходы — статические ссылки, показываются сразу,
              независимо от загрузки/ошибки GET /admin/stats; цифры ниже не менялись. */}
          <section className="ac-links" aria-labelledby="ac-links-title">
            <h2 id="ac-links-title" className="ac-links__title">
              {t('p7.stats.quickLinks')}
            </h2>
            <nav className="ac-grid ac-links__grid" aria-label={t('p7.stats.quickLinks')}>
              {QUICK_LINKS.map((link) => (
                <Link key={link.to} to={link.to} className="ac-card ac-link">
                  <span className="ac-link__label">{t(link.label)}</span>
                  <span className="ac-link__arrow" aria-hidden="true">
                    →
                  </span>
                </Link>
              ))}
            </nav>
          </section>

          {stats.isLoading && (
            <div className="ac-grid" aria-busy="true">
              <span className="sr-only" role="status">
                {t('common.loading')}
              </span>
              {/* Скелетон: пять карточек-заготовок, пока не пришёл ответ. */}
              {[0, 1, 2, 3, 4].map((slot) => (
                <div key={slot} className="ac-card" aria-hidden="true">
                  <span className="ac-skeleton ac-skeleton--title" />
                  <span className="ac-skeleton" />
                  <span className="ac-skeleton" />
                  <span className="ac-skeleton ac-skeleton--short" />
                </div>
              ))}
            </div>
          )}

          {stats.isError && (
            <div className="ac-error" role="alert">
              <p className="error">{errorText(stats.error)}</p>
              <button
                type="button"
                className="btn btn-sm"
                onClick={() => void stats.refetch()}
              >
                {t('common.retry')}
              </button>
            </div>
          )}

          {data && (
            <div className="ac-grid">
              <section className="ac-card" aria-labelledby="ac-stats-users">
                <h2 id="ac-stats-users" className="ac-card__title">
                  {t('adminStats.users')}
                </h2>
                <dl className="ac-rows">
                  <StatRow label={t('adminStats.total')} value={data.users.total} />
                  <StatRow label={t('adminStats.students')} value={data.users.students} />
                  <StatRow label={t('adminStats.teachers')} value={data.users.teachers} />
                  <StatRow label={t('adminStats.admins')} value={data.users.admins} />
                  <StatRow label={t('adminStats.blocked')} value={data.users.blocked} />
                  <StatRow label={t('adminStats.new7')} value={data.users.newLast7} />
                </dl>
              </section>

              <section className="ac-card" aria-labelledby="ac-stats-courses">
                <h2 id="ac-stats-courses" className="ac-card__title">
                  {t('adminStats.courses')}
                </h2>
                <dl className="ac-rows">
                  <StatRow label={t('adminStats.total')} value={data.courses.total} />
                  <StatRow label={t('adminStats.statusDraft')} value={data.courses.draft} />
                  <StatRow label={t('adminStats.statusReview')} value={data.courses.review} />
                  <StatRow
                    label={t('adminStats.statusPublished')}
                    value={data.courses.published}
                  />
                  <StatRow
                    label={t('adminStats.statusArchived')}
                    value={data.courses.archived}
                  />
                </dl>
              </section>

              <section className="ac-card" aria-labelledby="ac-stats-payments">
                <h2 id="ac-stats-payments" className="ac-card__title">
                  {t('adminStats.payments')}
                </h2>
                <dl className="ac-rows">
                  <StatRow label={t('adminStats.total')} value={data.payments.count} />
                  <StatRow
                    label={t('adminStats.paidTotal')}
                    value={formatMoney(data.payments.succeededCents)}
                  />
                  <StatRow
                    label={t('adminStats.platformFee')}
                    value={formatMoney(data.payments.platformFeeCents)}
                  />
                  <StatRow
                    label={t('adminStats.teacherPayout')}
                    value={formatMoney(data.payments.teacherCents)}
                  />
                  <StatRow
                    label={t('adminStats.last30')}
                    value={formatMoney(data.payments.last30Cents)}
                  />
                </dl>
              </section>

              <section className="ac-card" aria-labelledby="ac-stats-enrollments">
                <h2 id="ac-stats-enrollments" className="ac-card__title">
                  {t('adminStats.enrollments')}
                </h2>
                <dl className="ac-rows">
                  <StatRow label={t('adminStats.total')} value={data.enrollments.total} />
                  <StatRow label={t('adminStats.active')} value={data.enrollments.active} />
                </dl>
              </section>

              <section className="ac-card" aria-labelledby="ac-stats-reviews">
                <h2 id="ac-stats-reviews" className="ac-card__title">
                  {/* Отдельного ключа «Отзывы» у сводки нет: reviews.title = «Отзывы». */}
                  {t('reviews.title')}
                </h2>
                <dl className="ac-rows">
                  <StatRow label={t('adminStats.reviewsTotal')} value={data.reviews.total} />
                  <StatRow
                    label={t('adminStats.avgRating')}
                    value={formatRating(data.reviews.avgRating)}
                  />
                </dl>
              </section>
            </div>
          )}
        </main>
      </div>
    </div>
  );
}
