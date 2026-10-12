import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { me, myTeaching } from '../api/client';
import { t } from '../i18n/t';
import MyCoursesPage from './MyCourses';
import AdminPage from './Admin';

export function RequireRole({
  roles,
  children,
}: {
  roles: Array<'student' | 'teacher' | 'admin'>;
  children: ReactNode;
}): JSX.Element {
  const query = useQuery({ queryKey: ['me'], queryFn: me, retry: false });
  if (query.isLoading) return <p>{t('cabinet.loading')}</p>;
  if (query.isError || !query.data)
    return (
      <p>
        {t('cabinet.needLogin')} — <Link to="/login">{t('cabinet.goLogin')}</Link>
      </p>
    );
  if (!roles.includes(query.data.role)) return <p>{t('cabinet.needRole')}</p>;
  return <>{children}</>;
}

export function StudentCab(): JSX.Element {
  return (
    <RequireRole roles={['student', 'teacher', 'admin']}>
      <MyCoursesPage />
    </RequireRole>
  );
}

export function TeacherCab(): JSX.Element {
  return (
    <RequireRole roles={['teacher', 'admin']}>
      <main>
        <h1>{t('cabinet.teacherTitle')}</h1>
        <p>
          <Link to="/teach/new">{t('teach.create')}</Link>
        </p>
        <TeacherCourses />
      </main>
    </RequireRole>
  );
}

function TeacherCourses(): JSX.Element {
  const { data, isLoading, isError } = useQuery({
    queryKey: ['teaching'],
    queryFn: myTeaching,
  });
  if (isLoading) return <p>{t('common.loading')}</p>;
  if (isError) return <p>{t('common.error')}</p>;
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

export function AdminCab(): JSX.Element {
  return (
    <RequireRole roles={['admin']}>
      <AdminPage />
    </RequireRole>
  );
}
