import { Link, NavLink, Outlet, createBrowserRouter } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { logout, me } from '../api/client';
import { t } from '../i18n/t';
import Catalog from '../pages/Catalog';
import Course from '../pages/Course';
import { Login, Register } from '../pages/Auth';
import MyCourses from '../pages/MyCourses';
import Learn from '../pages/Learn';
import Constructor from '../pages/Constructor';
import MockPay from '../pages/MockPay';
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
      <div className="promo">
        <span>Первый курс со скидкой 20% для новых студентов</span>
        <Link to="/">Выбрать курс</Link>
      </div>
      <header className="topbar">
        <div className="wrap topbar-in">
          <Link className="logo" to="/">
            <span className="logo-mark">E</span>Edu&nbsp;Platform
          </Link>
          <nav className="nav" aria-label={t('nav.catalog')}>
            <NavLink to="/" end className={({ isActive }) => (isActive ? 'active' : '')}>
              {t('nav.catalog')}
            </NavLink>
            <NavLink to="/me" className={({ isActive }) => (isActive ? 'active' : '')}>
              {t('nav.myCourses')}
            </NavLink>
            {(user?.role === 'teacher' || user?.role === 'admin') && (
              <NavLink to="/teach" className={({ isActive }) => (isActive ? 'active' : '')}>
                {t('nav.teach')}
              </NavLink>
            )}
            {user?.role === 'admin' && (
              <NavLink to="/admin" className={({ isActive }) => (isActive ? 'active' : '')}>
                {t('nav.admin')}
              </NavLink>
            )}
          </nav>
          <div className="top-actions">
            {user ? (
              <>
                <span className="user-email">{user.email}</span>{' '}
                <button
                  type="button"
                  className="btn btn-ghost btn-sm"
                  onClick={() => void onLogout()}
                >
                  {t('nav.logout')}
                </button>
              </>
            ) : (
              <>
                <Link className="btn btn-ghost btn-sm" to="/login">
                  {t('nav.login')}
                </Link>
                <Link className="btn btn-primary btn-sm" to="/register">
                  Начать учиться
                </Link>
              </>
            )}
          </div>
        </div>
      </header>
      <div id="content">
        <Outlet />
      </div>
      <footer className="footer">
        <div className="wrap footer-in">
          <div>
            <Link className="logo" to="/">
              <span className="logo-mark">E</span>Edu&nbsp;Platform
            </Link>
            <p>
              <small>
                Маркетплейс IT-курсов для начинающих. Учим спокойно, проверяем честно,
                поддерживаем на каждом шаге.
              </small>
            </p>
            <p className="contact-line">
              <b>8 (800) 600-78-47</b>
              <br />
              <small>hello@edu-platform.ru · Москва</small>
            </p>
            <small>© Edu Platform, 2026</small>
          </div>
          <div>
            <b>Направления</b>
            <Link to="/">Программирование</Link>
            <Link to="/">Дизайн</Link>
            <Link to="/">Данные</Link>
            <Link to="/">Маркетинг</Link>
            <Link to="/">Нейросети</Link>
            <Link to="/">Все курсы</Link>
          </div>
          <div>
            <b>Студенту</b>
            <Link to="/">Каталог</Link>
            <Link to="/me">Мои курсы</Link>
            <Link to="/login">Вход</Link>
            <Link to="/register">Регистрация</Link>
          </div>
          <div>
            <b>Авторам</b>
            <Link to="/teach">Конструктор курса</Link>
            <Link to="/admin">Модерация</Link>
            <Link to="/admin">Приглашения</Link>
          </div>
        </div>
      </footer>
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
      { path: '/pay/mock/:providerPaymentId', element: <MockPay /> },
    ],
  },
]);
