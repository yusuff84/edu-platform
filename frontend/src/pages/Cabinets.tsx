import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { myTeaching } from '../api/client';
import { t } from '../i18n/t';

/**
 * Кабинет учителя. Гвард роли (teacher | admin) стоит в router.tsx —
 * RequireRole переиспользует общий query ['me'] без дублей запроса.
 * /me рендерит MyCourses напрямую (дубль StudentCab → MyCourses убран).
 */
export function TeacherCab(): JSX.Element {
  return (
    <main>
      <h1>{t('cabinet.teacherTitle')}</h1>
      <p>
        <Link to="/teach/new">{t('teach.create')}</Link>
      </p>
      <TeacherCourses />
    </main>
  );
}

function TeacherCourses(): JSX.Element {
  const { data, isLoading, isError } = useQuery({
    queryKey: ['teaching'],
    queryFn: myTeaching,
  });
  if (isLoading) return <p role="status">{t('common.loading')}</p>;
  if (isError) return <p role="alert">{t('common.error')}</p>;
  if (!data || data.length === 0) return <p>{t('teach.empty')}</p>;
  return (
    <ul>
      {data.map((c) => (
        <li key={c.id}>
          <Link to={`/teach/${c.id}`}>{c.title}</Link> — {t('teach.status')}: {c.status}
        </li>
      ))}
    </ul>
  );
}
