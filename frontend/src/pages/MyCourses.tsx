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
import { coverFor } from '../components/CourseCard';
import { t } from '../i18n/t';
import '../styles/profile.css';

export default function MyCourses(): JSX.Element {
  const query = useQuery({ queryKey: ['my-courses'], queryFn: myCourses });
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

  // Загрузка
  if (query.isLoading) {
    return (
      <div className="wrap">
        <div className="page">
          <div className="panel">
            <p role="status">{t('my.loading')}</p>
          </div>
        </div>
      </div>
    );
  }

  // Ошибка сети / сервера — можно повторить без перезагрузки страницы
  if (query.isError) {
    return (
      <div className="wrap">
        <div className="page">
          <div className="panel">
            <p role="alert" className="error">
              {t('error.network')}
            </p>
            <button type="button" onClick={() => void query.refetch()}>
              {t('common.retry')}
            </button>
          </div>
        </div>
      </div>
    );
  }

  // Пустой список
  if (items.length === 0) {
    return (
      <div className="wrap">
        <div className="page">
          <h1>{t('cabinet.studentTitle')}</h1>
          <div className="empty">
            <p>{t('my.empty')}</p>
            <p>
              <Link className="btn btn-primary" to="/">
                {t('my.goCatalog')}
              </Link>
            </p>
          </div>
        </div>
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

  return (
    <div className="wrap">
      <div className="page">
        <h1>{t('cabinet.studentTitle')}</h1>
        <div className="cab-stats">
          <div>
            <b>{total}</b>
            <span>Курсов</span>
          </div>
          <div>
            <b>{avg === null ? '—' : `${avg}%`}</b>
            <span>Средний прогресс</span>
          </div>
          <div>
            <b>{done}</b>
            <span>Завершено</span>
          </div>
        </div>
        <div className="enrolled-grid">
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

            return (
              <div key={course.id} className="panel">
                <div
                  className={`cover ${coverFor(course.slug || course.id)} enrolled-cover`}
                  aria-hidden="true"
                >
                  <span className="tag">Курс</span>
                </div>
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
                          {t('profile.lessonsDone')}: {data.completedLessons} / {data.totalLessons}
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
                    <Link className="btn btn-primary" to={href}>
                      {t('my.continue')}
                    </Link>
                  )}
                  {progress.isError && !revoked && (
                    <button type="button" className="btn" onClick={() => void progress.refetch()}>
                      {t('common.retry')}
                    </button>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
