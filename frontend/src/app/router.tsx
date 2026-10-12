import { Link, Outlet, createBrowserRouter } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { logout, me } from '../api/client';
import { t } from '../i18n/t';
import Catalog from '../pages/Catalog';
import Course from '../pages/Course';
import { Login, Register } from '../pages/Auth';
import MyCourses from '../pages/MyCourses';
import Learn from '../pages/Learn';
import Constructor from '../pages/Constructor';
import Admin from '../pages/Admin';
import { AdminCab, StudentCab, TeacherCab } from '../pages/Cabinets';

function Layout(): JSX.Element {
  const qc = useQueryClient();
  const meQuery = useQuery({ queryKey: ['me'], queryFn: me, retry: false, staleTime: 30_000 });
  const user = meQuery.data;

  async function onLogout(): Promise<void> {
    await logout();
    await qc.invalidateQueries({ queryKey: ['me'] });
  }

  return (
    <div>
      <a href="#content" className="sr-only">
        {t('a11y.skipToContent')}
      </a>
      <header>
        <nav>
          <Link to="/">{t('app.title')}</Link>
          {' | '}
          <Link to="/">{t('nav.catalog')}</Link>
          {' | '}
          <Link to="/me">{t('nav.myCourses')}</Link>
          {(user?.role === 'teacher' || user?.role === 'admin') && (
            <>
              {' | '}
              <Link to="/teach">{t('nav.teach')}</Link>
            </>
          )}
          {user?.role === 'admin' && (
            <>
              {' | '}
              <Link to="/admin">{t('nav.admin')}</Link>
            </>
          )}
          {' | '}
          {user ? (
            <>
              <span>{user.email}</span>{' '}
              <button onClick={() => void onLogout()}>{t('nav.logout')}</button>
            </>
          ) : (
            <>
              <Link to="/login">{t('nav.login')}</Link>
              {' | '}
              <Link to="/register">{t('nav.register')}</Link>
            </>
          )}
        </nav>
      </header>
      <div id="content">
        <Outlet />
      </div>
    </div>
  );
}

export const router = createBrowserRouter([
  {
    element: <Layout />,
    children: [
      { path: '/', element: <Catalog /> },
      { path: '/courses/:slug', element: <Course /> },
      { path: '/login', element: <Login /> },
      { path: '/register', element: <Register /> },
      { path: '/me', element: <StudentCab /> },
      { path: '/my', element: <MyCourses /> },
      { path: '/teach', element: <TeacherCab /> },
      { path: '/teach/new', element: <Constructor /> },
      { path: '/teach/:id', element: <Constructor /> },
      { path: '/admin', element: <AdminCab /> },
      { path: '/admin-panel', element: <Admin /> },
      { path: '/learn/:courseId', element: <Learn /> },
      { path: '/learn/:courseId/:lessonId', element: <Learn /> },
    ],
  },
]);
