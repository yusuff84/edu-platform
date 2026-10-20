import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { me, myTeaching } from '../api/client';
import { t } from '../i18n/t';
import { useSeo } from '../app/useSeo';
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
  if (query.isLoading)
    return (
      <div className="wrap">
        <div className="page">
          <div className="panel">
            <p>{t('cabinet.loading')}</p>
          </div>
        </div>
      </div>
    );
  if (query.isError || !query.data)
    return (
      <div className="wrap">
        <div className="page">
          <div className="panel">
            <p>
              {t('cabinet.needLogin')} — <Link to="/login">{t('cabinet.goLogin')}</Link>
            </p>
          </div>
        </div>
      </div>
    );
  if (!roles.includes(query.data.role))
    return (
      <div className="wrap">
        <div className="page">
          <div className="panel">
            <p>{t('cabinet.needRole')}</p>
          </div>
        </div>
      </div>
    );
  return <>{children}</>;
}

export function StudentCab(): JSX.Element {
  useSeo('Кабинет ученика — Edu Platform');
  return (
    <RequireRole roles={['student', 'teacher', 'admin']}>
      <MyCoursesPage />
    </RequireRole>
  );
}

/**
 * Кабинет учителя. Основной гвард роли стоит в router.tsx (RouteGuards.tsx,
 * общий query ['me']); локальный RequireRole ниже оставлен как второй рубеж
 * и для совместимости с разводкой, где кабинеты гвардируются сами. StudentCab
 * и AdminCab сохранены — их использует такая разводка маршрутов.
 */
export function TeacherCab(): JSX.Element {
  useSeo('Кабинет преподавателя — Edu Platform');
  return (
    <RequireRole roles={['teacher', 'admin']}>
      <div className="wrap">
        <main className="page">
          <h1>{t('cabinet.teacherTitle')}</h1>
          <p>
            <Link className="btn btn-primary" to="/teach/new">
              {t('teach.create')}
            </Link>{' '}
            {/* T78: переход на аналитику преподавателя. */}
            <Link className="btn btn-ghost" to="/teach/stats">
              {t('p9.stats.title')}
            </Link>
          </p>
          <TeacherCourses />
        </main>
      </div>
    </RequireRole>
  );
}

function statusMeta(status: string): { className: string; label: string } {
  switch (status) {
    case 'published':
      return { className: 'status st-ok', label: 'опубликован' };
    case 'rejected':
      return { className: 'status st-bad', label: 'отклонён' };
    case 'review':
      return { className: 'status st-wait', label: 'на проверке' };
    case 'draft':
    default:
      return { className: 'status st-wait', label: 'черновик' };
  }
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
    <ul className="teach-list">
      {data.map((c) => {
        const meta = statusMeta(c.status);
        return (
          <li key={c.id} className="l-item">
            <Link to={`/teach/${c.id}`}>{c.title}</Link>
            <span className="spacer" aria-hidden="true" />
            <span className={meta.className}>{meta.label}</span>
          </li>
        );
      })}
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
