// «Мои курсы» студента (первая страница после входа).
// T30: честный прогресс — ТОЛЬКО с сервера: GET /me/courses/{id}/progress
// (api/meExtra.ts) для каждой записи одним массивом useQueries. Поля
// percent/nextLesson из GET /me/courses в интерфейсе не используются:
// раньше картину «дорисовывал» клиент, и она расходилась с сервером.
//
// Форма данных записи — строго по контракту GET /me/courses:
// MyCourse = { course: CourseItem, percent, nextLesson: NextLesson | null }.
// Плоских courseId/slug/title/nextLessonId в ответе НЕТ.

import { Link } from 'react-router-dom';
import { useQueries, useQuery } from '@tanstack/react-query';
import { ApiError, myCourses } from '../api/client';
import { courseProgress } from '../api/meExtra';
import { coverUrl } from '../api/teacherCover';
import { coverFor } from '../components/CourseCard';
import { meQueryOptions } from '../components/RouteGuards';
import { t } from '../i18n/t';
import { useSeo } from '../app/useSeo';
import '../styles/profile.css';
import '../styles/certificate.css';
import '../styles/favorites.css';

/** Время суток для приветствия в кабинете. */
function greetingForNow(): string {
  const hour = new Date().getHours();
  if (hour < 5) return 'Доброй ночи';
  if (hour < 12) return 'Доброе утро';
  if (hour < 18) return 'Добрый день';
  return 'Добрый вечер';
}

/** Короткое имя для заголовка: displayName → локальная часть email. */
function firstName(user?: { displayName?: string; email: string }): string {
  const displayName = user?.displayName?.trim();
  if (displayName) return displayName.split(/\s+/)[0];
  const emailName = user?.email.split('@')[0] ?? '';
  return emailName ? emailName.charAt(0).toLocaleUpperCase('ru') + emailName.slice(1) : '';
}

export default function MyCourses(): JSX.Element {
  useSeo('Мои курсы — Edu Platform');
  const query = useQuery({ queryKey: ['my-courses'], queryFn: myCourses });
  // Данные пользователя уже в кеше Layout/гвардов — отдельного запроса нет.
  const meQuery = useQuery(meQueryOptions);
  const items = query.data ?? [];

  // По одному запросу прогресса на курс — useQueries вместо хуков в цикле.
  // Ключ ['course-progress', id] совпадает с Learn.tsx: после complete урок
  // инвалидирует его, и карточки здесь получают свежие данные из кеша.
  const progressQueries = useQueries({
    queries: items.map((m) => ({
      queryKey: ['course-progress', m.course.id],
      queryFn: () => courseProgress(m.course.id),
      retry: false, // 403/404 — не временный сбой, повтор не поможет
    })),
  });

  const greeting = greetingForNow();
  const name = firstName(meQuery.data);
  const hello = name ? `${greeting}, ${name}!` : `${greeting}!`;

  // Загрузка
  if (query.isLoading) {
    return (
      <div className="wrap">
        <main className="page dashboard-page">
          <div className="panel">
            <p role="status">{t('my.loading')}</p>
          </div>
        </main>
      </div>
    );
  }

  // Ошибка сети / сервера — можно повторить без перезагрузки страницы
  if (query.isError) {
    return (
      <div className="wrap">
        <main className="page dashboard-page">
          <div className="panel">
            <p role="alert" className="error">
              {t('error.network')}
            </p>
            <button type="button" className="btn" onClick={() => void query.refetch()}>
              {t('common.retry')}
            </button>
          </div>
        </main>
      </div>
    );
  }

  // Пустой список
  if (items.length === 0) {
    return (
      <div className="wrap">
        <main className="page dashboard-page">
          <header className="dashboard-head">
            <div>
              <p className="dashboard-kicker">Личный кабинет</p>
              <h1>{hello}</h1>
              <p className="dashboard-sub">
                Здесь появится всё, что вы купите или начнёте проходить. Пока — самое
                время выбрать первый курс.
              </p>
            </div>
            <div className="dashboard-head-actions">
              {/* T74: избранное доступно и при пустом списке курсов. */}
              <Link className="btn btn-ghost btn-sm" to="/me/favorites">
                {t('p9.fav.title')}
              </Link>
            </div>
          </header>
          <div className="empty empty--dashboard">
            <span className="empty-illustration" aria-hidden="true">
              <svg viewBox="0 0 64 64" width="64" height="64" fill="none">
                <rect x="8" y="12" width="48" height="40" rx="10" stroke="currentColor" strokeWidth="2.5" />
                <path d="M20 24h24M20 32h16M20 40h10" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" />
                <circle cx="48" cy="47" r="9" fill="#14151a" />
                <path d="M44.5 47h7M48 43.5v7" stroke="#fff" strokeWidth="2.2" strokeLinecap="round" />
              </svg>
            </span>
            <h2>Пока здесь пусто</h2>
            <p>{t('my.empty')}</p>
            <Link className="btn btn-ink" to="/">
              {t('my.goCatalog')}
            </Link>
          </div>
        </main>
      </div>
    );
  }

  // Сводка — по уже загруженным прогрессам; пока ничего не загружено, средний
  // прогресс показываем как «—», а не выдуманный ноль.
  const loaded = progressQueries.flatMap((p) => (p.data ? [p.data] : []));
  const total = items.length;
  const done = loaded.filter((p) => p.completed).length;
  const avg =
    loaded.length > 0
      ? Math.round(loaded.reduce((sum, p) => sum + p.percent, 0) / loaded.length)
      : null;

  // «Продолжить просмотр»: первый незавершённый курс, для которого уже есть
  // честный серверный прогресс. Пока прогресс грузится, блок не показываем,
  // чтобы не подставлять случайный курс.
  const continueIndex = items.findIndex((_, i) => {
    const progressQuery = progressQueries[i];
    const data = progressQuery?.data;
    // Только успешный запрос: после отзыва доступа React Query может
    // сохранить старый data, и герой не должен вести в закрытый курс.
    return Boolean(
      progressQuery?.isSuccess && data && !data.completed && data.percent < 100,
    );
  });
  const continueItem = continueIndex >= 0 ? items[continueIndex] : null;
  const continueData = continueIndex >= 0 ? progressQueries[continueIndex].data : undefined;
  const continueCoverSrc = continueItem ? coverUrl(continueItem.course.coverPath) : undefined;
  const continueHref = continueItem
    ? continueData?.nextLessonId
      ? `/learn/${continueItem.course.id}/${continueData.nextLessonId}`
      : `/learn/${continueItem.course.id}`
    : '/';

  return (
    <div className="wrap">
      <main className="page dashboard-page">
        <header className="dashboard-head">
          <div>
            <p className="dashboard-kicker">Личный кабинет</p>
            <h1>{hello}</h1>
            <p className="dashboard-sub">
              Продолжите с того места, где остановились, или загляните в библиотеку
              курсов.
            </p>
          </div>
          <div className="dashboard-head-actions">
            <Link className="btn btn-ghost btn-sm" to="/me/favorites">
              {t('p9.fav.title')}
            </Link>
          </div>
        </header>

        {continueItem && (
          <section className="continue-hero" aria-labelledby="continue-title">
            <div className="continue-copy">
              <span className="continue-eyebrow">Продолжить просмотр</span>
              <h2 id="continue-title">{continueItem.course.title}</h2>
              <p>
                {continueData
                  ? `Пройдено ${continueData.completedLessons} из ${continueData.totalLessons} уроков. Небольшой шаг сегодня — и курс станет заметно ближе.`
                  : 'Вернитесь к курсу и продолжите обучение с того места, где остановились.'}
              </p>
              {continueData && (
                <div className="continue-progress">
                  <div
                    className="progress"
                    role="progressbar"
                    aria-valuemin={0}
                    aria-valuemax={100}
                    aria-valuenow={continueData.percent}
                    aria-label={`${t('my.progress')}: ${continueData.percent}%`}
                  >
                    <div
                      className="progress-bar"
                      style={{ width: `${continueData.percent}%` }}
                    />
                  </div>
                  <div className="continue-meta">
                    <span>
                      {continueData.completedLessons} / {continueData.totalLessons} уроков
                    </span>
                    <span>{continueData.percent}%</span>
                  </div>
                </div>
              )}
              <Link className="btn btn-ink" to={continueHref}>
                Продолжить обучение
                <svg width="17" height="17" viewBox="0 0 17 17" fill="none" aria-hidden="true">
                  <path
                    d="M3 8.5h10M9 4l4.5 4.5L9 13"
                    stroke="currentColor"
                    strokeWidth="1.8"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                  />
                </svg>
              </Link>
            </div>
            <div
              className={`continue-cover cover ${coverFor(continueItem.course.slug || continueItem.course.id)}`}
              aria-hidden="true"
            >
              <span className="tag">Курс</span>
              {continueCoverSrc ? (
                <img
                  className="continue-cover-img"
                  src={continueCoverSrc}
                  alt=""
                />
              ) : (
                <span className="continue-cover-letter">
                  {continueItem.course.title.charAt(0)}
                </span>
              )}
              {continueData && (
                <span className="continue-cover-percent">{continueData.percent}%</span>
              )}
            </div>
          </section>
        )}

        <div className="dashboard-stats">
          <div className="dashboard-stat">
            <span className="dashboard-stat-icon" aria-hidden="true">
              <svg viewBox="0 0 24 24" width="21" height="21" fill="none">
                <path
                  d="M4 5.5A2.5 2.5 0 0 1 6.5 3H20v16H6.5A2.5 2.5 0 0 0 4 21.5V5.5Z"
                  stroke="currentColor"
                  strokeWidth="1.8"
                  strokeLinejoin="round"
                />
                <path d="M4 5.5A2.5 2.5 0 0 0 6.5 8H20" stroke="currentColor" strokeWidth="1.8" />
              </svg>
            </span>
            <div>
              <b>{total}</b>
              <span>Курсов в библиотеке</span>
            </div>
          </div>
          <div className="dashboard-stat">
            <span className="dashboard-stat-icon dashboard-stat-icon--violet" aria-hidden="true">
              <svg viewBox="0 0 24 24" width="21" height="21" fill="none">
                <path
                  d="M5 19V9M12 19V5M19 19v-6"
                  stroke="currentColor"
                  strokeWidth="2.1"
                  strokeLinecap="round"
                />
              </svg>
            </span>
            <div>
              <b>{avg === null ? '—' : `${avg}%`}</b>
              <span>Средний прогресс</span>
            </div>
          </div>
          <div className="dashboard-stat">
            <span className="dashboard-stat-icon dashboard-stat-icon--green" aria-hidden="true">
              <svg viewBox="0 0 24 24" width="21" height="21" fill="none">
                <circle cx="12" cy="12" r="8.5" stroke="currentColor" strokeWidth="1.8" />
                <path
                  d="m8.2 12.2 2.4 2.4 5.2-5.2"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
              </svg>
            </span>
            <div>
              <b>{done}</b>
              <span>Завершено</span>
            </div>
          </div>
        </div>

        <div className="dashboard-section-head">
          <h2>Ваши курсы</h2>
          <span>Всего: {total}</span>
        </div>

        <div className={`enrolled-grid${items.length === 1 ? ' enrolled-grid--single' : ''}`}>
          {items.map((item, i) => {
            const course = item.course;
            const progress = progressQueries[i];
            const data = progress.data;
            const errStatus =
              progress.error instanceof ApiError ? progress.error.status : undefined;
            // 403 — активной записи нет (refunded/revoked): показываем статус
            // записи, а не выдуманные проценты. 404 — курс удалён.
            const revoked = errStatus === 403;
            const statusText = revoked
              ? t('my.progressRevoked')
              : t('my.progressFailed');

            // «Продолжить»: серверный nextLessonId, иначе — страница курса
            // (/learn/{id} сам выберет первый урок); при completed — тоже она.
            const nextLessonId = data?.nextLessonId ?? null;
            const href = nextLessonId
              ? `/learn/${course.id}/${nextLessonId}`
              : `/learn/${course.id}`;

            // T68: сертификат приходит аддитивным полем записи (пустая строка —
            // сертификата ещё нет); бейдж ведёт на публичную страницу по номеру.
            const certificateNumber = item.certificateNumber?.trim() ?? '';
            const completed = Boolean(data?.completed);
            const coverSrc = coverUrl(course.coverPath);

            return (
              <article key={course.id} className="panel enrolled-card">
                <div
                  className={`cover ${coverFor(course.slug || course.id)} enrolled-cover`}
                  aria-hidden="true"
                >
                  <span className="tag">Курс</span>
                  {coverSrc ? (
                    <img className="enrolled-cover-img" src={coverSrc} alt="" />
                  ) : (
                    <span className="enrolled-cover-letter">
                      {course.title.charAt(0)}
                    </span>
                  )}
                  {data && <span className="enrolled-percent">{data.percent}%</span>}
                </div>
                <div className="enrolled-body">
                  <h3>
                    <Link to={href}>{course.title}</Link>
                  </h3>

                  {data ? (
                    <>
                      <div
                        className="progress"
                        role="progressbar"
                        aria-valuemin={0}
                        aria-valuemax={100}
                        aria-valuenow={data.percent}
                        aria-label={`${t('my.progress')}: ${data.percent}%`}
                      >
                        <div className="progress-bar" style={{ width: `${data.percent}%` }} />
                      </div>
                      <div className="progress-meta">
                        <span>
                          {t('my.progress')}: {data.percent}%
                        </span>
                        {data.completed ? (
                          <span className="pf-course-done">{t('profile.courseDone')}</span>
                        ) : (
                          <span>
                            {data.completedLessons} / {data.totalLessons} уроков
                          </span>
                        )}
                      </div>
                    </>
                  ) : (
                    <>
                      {/* Прогресс ещё грузится или недоступен: нейтральная полоса
                          без «0 %» — выдуманные проценты не показываем. */}
                      <div
                        className={progress.isError ? 'progress' : 'progress pf-progress-pending'}
                        role="progressbar"
                        aria-busy={!progress.isError}
                        aria-valuetext={progress.isError ? statusText : t('common.loading')}
                        aria-label={t('my.progress')}
                      />
                      <div className="progress-meta">
                        <span className="pf-enroll-status">
                          {progress.isError ? statusText : t('common.loading')}
                        </span>
                      </div>
                    </>
                  )}

                  <div className="btn-row">
                    {!revoked && (
                      <Link
                        className={`btn btn-sm ${completed ? 'btn-soft' : 'btn-ink'}`}
                        to={href}
                      >
                        {completed ? 'Повторить' : t('my.continue')}
                      </Link>
                    )}
                    {certificateNumber && (
                      <Link
                        className="btn cert-badge"
                        to={`/certificates/${encodeURIComponent(certificateNumber)}`}
                      >
                        {t('p8.cert.badge')}
                      </Link>
                    )}
                    {progress.isError && !revoked && (
                      <button
                        type="button"
                        className="btn btn-sm"
                        onClick={() => void progress.refetch()}
                      >
                        {t('common.retry')}
                      </button>
                    )}
                  </div>
                </div>
              </article>
            );
          })}
        </div>
      </main>
    </div>
  );
}
