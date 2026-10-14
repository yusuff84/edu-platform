import { useEffect } from 'react';
import { Link, NavLink, Navigate, Outlet, createBrowserRouter, useLocation } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { logout } from '../api/client';
import { t } from '../i18n/t';
import Catalog from '../pages/Catalog';
import Course from '../pages/Course';
import { Login, Register } from '../pages/Auth';
import MyCourses from '../pages/MyCourses';
import Learn from '../pages/Learn';
import Constructor from '../pages/Constructor';
import Admin from '../pages/Admin';
import { TeacherCab } from '../pages/Cabinets';
import NotFound from '../pages/NotFound';
import { ErrorBoundary, RouteError } from '../components/ErrorBoundary';
import { RequireAuth, RequireRole, meQueryOptions } from '../components/RouteGuards';
import '../styles/shell.css';

/** Прокрутка к началу страницы при смене маршрута (иначе позиция скролла «уезжает»). */
function ScrollToTop(): null {
  const { pathname } = useLocation();
  useEffect(() => {
    window.scrollTo(0, 0);
  }, [pathname]);
  return null;
}

function Layout(): JSX.Element {
  const qc = useQueryClient();
  // Один query ['me'] на всё приложение: Layout и гварды (RouteGuards.tsx)
  // используют общие meQueryOptions — без второго запроса и «моргания».
  const meQuery = useQuery(meQueryOptions);
  const user = meQuery.data;

  async function onLogout(): Promise<void> {
    await logout();
    // resetQueries сбрасывает кэш ['me'] и перезапрашивает: после выхода
    // пользователь гарантированно исчезает из шапки и гвардов (401 → логин).
    await qc.resetQueries({ queryKey: ['me'] });
  }

  return (
    <div>
      <ScrollToTop />
      <a href="#content" className="sr-only">
        {t('a11y.skipToContent')}
      </a>
      <header>
        <nav>
          <Link to="/">{t('app.title')}</Link>
          {' | '}
          {/* NavLink сам ставит aria-current="page" на активном пункте (доступность). */}
          <NavLink to="/" end>
            {t('nav.catalog')}
          </NavLink>
          {' | '}
          <NavLink to="/me">{t('nav.myCourses')}</NavLink>
          {(user?.role === 'teacher' || user?.role === 'admin') && (
            <>
              {' | '}
              <NavLink to="/teach">{t('nav.teach')}</NavLink>
            </>
          )}
          {user?.role === 'admin' && (
            <>
              {' | '}
              <NavLink to="/admin">{t('nav.admin')}</NavLink>
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
        {/* Граница ошибок поддерева: крах страницы не убивает шапку. */}
        <ErrorBoundary>
          <Outlet />
        </ErrorBoundary>
      </div>
    </div>
  );
}

export const router = createBrowserRouter([
  {
    element: <Layout />,
    // Ошибки уровня роутера (в т.ч. рендера Layout) — экран восстановления, не белый экран.
    errorElement: <RouteError />,
    children: [
      { path: '/', element: <Catalog /> },
      { path: '/courses/:slug', element: <Course /> },
      { path: '/login', element: <Login /> },
      { path: '/register', element: <Register /> },
      // Кабинеты и обучение — за гвардами (RouteGuards.tsx, query ['me']).
      { path: '/me', element: <RequireAuth><MyCourses /></RequireAuth> },
      { path: '/teach', element: <RequireRole roles={['teacher', 'admin']}><TeacherCab /></RequireRole> },
      { path: '/teach/new', element: <RequireRole roles={['teacher', 'admin']}><Constructor /></RequireRole> },
      { path: '/teach/:id', element: <RequireRole roles={['teacher', 'admin']}><Constructor /></RequireRole> },
      { path: '/admin', element: <RequireRole roles={['admin']}><Admin /></RequireRole> },
      // Learn требует авторизации, но не требует роли.
      // errorElement на /learn/*: ошибка урока не убивает весь кабинет.
      {
        path: '/learn/:courseId',
        element: <RequireAuth><Learn /></RequireAuth>,
        errorElement: <RouteError />,
      },
      {
        path: '/learn/:courseId/:lessonId',
        element: <RequireAuth><Learn /></RequireAuth>,
        errorElement: <RouteError />,
      },
      // Старые пути — редиректы на канонические, чтобы ссылки не ломались.
      { path: '/my', element: <Navigate to="/me" replace /> },
      { path: '/admin-panel', element: <Navigate to="/admin" replace /> },
      // Неизвестный URL — 404-страница, не белый экран.
      { path: '*', element: <NotFound /> },
    ],
  },
]);
