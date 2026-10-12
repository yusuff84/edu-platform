import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { myCourses } from '../api/client';
import { t } from '../i18n/t';

export default function MyCourses(): JSX.Element {
  const query = useQuery({ queryKey: ['my-courses'], queryFn: myCourses });

  if (query.isLoading) return <p>{t('my.loading')}</p>;
  if (query.isError) return <p>{t('common.error')}</p>;
  const items = query.data ?? [];
  if (items.length === 0)
    return (
      <main>
        <h1>{t('cabinet.studentTitle')}</h1>
        <p>{t('my.empty')}</p>
        <p>
          <Link to="/">{t('my.goCatalog')}</Link>
        </p>
      </main>
    );

  return (
    <main>
      <h1>{t('cabinet.studentTitle')}</h1>
      <ul className="list">
        {items.map((m) => (
          <li key={m.courseId} className="card">
            <Link to={m.nextLessonId ? `/learn/${m.courseId}/${m.nextLessonId}` : `/learn/${m.courseId}`}>
              <strong>{m.title}</strong>
            </Link>
            <div className="progress" aria-label={`${t('my.progress')}: ${m.percent}%`}>
              <div className="progress-bar" style={{ width: `${m.percent}%` }} />
            </div>
            <span>
              {t('my.progress')}: {m.percent}%
            </span>{' '}
            <Link
              to={m.nextLessonId ? `/learn/${m.courseId}/${m.nextLessonId}` : `/learn/${m.courseId}`}
            >
              {t('my.continue')}
            </Link>
          </li>
        ))}
      </ul>
    </main>
  );
}
