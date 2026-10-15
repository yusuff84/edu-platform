import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { myCourses } from '../api/client';
import { coverFor } from '../components/CourseCard';
import { t } from '../i18n/t';

export default function MyCourses(): JSX.Element {
  const query = useQuery({ queryKey: ['my-courses'], queryFn: myCourses });

  if (query.isLoading)
    return (
      <div className="wrap">
        <div className="page">
          <div className="panel">
            <p>{t('my.loading')}</p>
          </div>
        </div>
      </div>
    );
  if (query.isError)
    return (
      <div className="wrap">
        <div className="page">
          <div className="panel">
            <p>{t('common.error')}</p>
          </div>
        </div>
      </div>
    );
  const items = query.data ?? [];
  if (items.length === 0)
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

  const total = items.length;
  const avg =
    total > 0 ? Math.round(items.reduce((sum, m) => sum + (m.percent ?? 0), 0) / total) : 0;
  const done = items.filter((m) => m.percent >= 100).length;

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
            <b>{avg}%</b>
            <span>Средний прогресс</span>
          </div>
          <div>
            <b>{done}</b>
            <span>Завершено</span>
          </div>
        </div>
        <div className="enrolled-grid">
          {items.map((m) => {
            const href = m.nextLessonId
              ? `/learn/${m.courseId}/${m.nextLessonId}`
              : `/learn/${m.courseId}`;
            return (
              <div key={m.courseId} className="panel">
                <div className={`cover ${coverFor(m.slug)} enrolled-cover`} aria-hidden="true">
                  <span className="tag">Курс</span>
                </div>
                <h3>
                  <Link to={href}>{m.title}</Link>
                </h3>
                <div className="progress" aria-label={`${t('my.progress')}: ${m.percent}%`}>
                  <div className="progress-bar" style={{ width: `${m.percent}%` }} />
                </div>
                <div className="progress-meta">
                  <span>
                    {t('my.progress')}: {m.percent}%
                  </span>
                  <span>{m.nextLessonTitle ?? ''}</span>
                </div>
                <div className="btn-row">
                  <Link className="btn btn-primary" to={href}>
                    {t('my.continue')}
                  </Link>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
