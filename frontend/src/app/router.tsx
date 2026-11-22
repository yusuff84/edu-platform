import { Suspense, lazy, useCallback, useEffect, useState } from 'react';
import { Link, NavLink, Navigate, Outlet, createBrowserRouter, useLocation } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { getPublicSettings, logout } from '../api/client';
import { setSessionUser } from './sessionCache';
import { t } from '../i18n/t';
import Catalog from '../pages/Catalog';
const Course = lazy(() => import('../pages/Course'));
const Login = lazy(() => import('../pages/Auth').then(module => ({ default: module.Login })));
const Register = lazy(() => import('../pages/Auth').then(module => ({ default: module.Register })));
const ForgotPassword = lazy(() => import('../pages/AccountRecovery').then(module => ({ default: module.ForgotPassword })));
const ResetPassword = lazy(() => import('../pages/AccountRecovery').then(module => ({ default: module.ResetPassword })));
const VerifyEmail = lazy(() => import('../pages/AccountRecovery').then(module => ({ default: module.VerifyEmail })));
import { ErrorBoundary, RouteError } from '../components/ErrorBoundary';
import { RequireAuth, RequireRole, meQueryOptions } from '../components/RouteGuards';
import '../styles/shell.css';
import { MobileNav, navItems } from '../components/MobileNav';

const PROMO_DISMISS_KEY = 'skx-promo-dismissed';

// T18: route-based code splitting. Каталог, страница курса и вход остаются
// статическими — это первый экран анонимного посетителя, он не должен ждать
// догрузки. Админка, конструктор, обучение, кабинеты и 404 видны только после
// входа/роли или встречаются редко — их чанки подгружаются по факту рендера
// (после прохождения гвардов), из главного чанка они исключены.
const MyCourses = lazy(() => import('../pages/MyCourses'));
// T74: избранное — личный раздел вошедшего, отдельный чанк.
const Favorites = lazy(() => import('../pages/Favorites'));
const Learn = lazy(() => import('../pages/Learn'));
const Constructor = lazy(() => import('../pages/Constructor'));
// Админка: каркас с боковым меню и разделы — отдельные чанки (pages/admin/*).
const AdminLayout = lazy(() => import('../components/admin/AdminLayout'));
const AdminOverview = lazy(() => import('../pages/admin/Overview'));
const AdminUsers = lazy(() => import('../pages/admin/Users'));
const AdminUserDetail = lazy(() => import('../pages/admin/UserDetail'));
const AdminCourses = lazy(() => import('../pages/admin/Courses'));
const AdminCourseDetail = lazy(() => import('../pages/admin/CourseDetail'));
const AdminModeration = lazy(() => import('../pages/admin/Moderation'));
const AdminQuestions = lazy(() => import('../pages/admin/Questions'));
const AdminReviews = lazy(() => import('../pages/admin/Reviews'));
const AdminEnrollments = lazy(() => import('../pages/admin/Enrollments'));
const AdminCertificates = lazy(() => import('../pages/admin/Certificates'));
const AdminPayments = lazy(() => import('../pages/admin/Payments'));
const AdminPromoCodes = lazy(() => import('../pages/admin/PromoCodes'));
const AdminLeads = lazy(() => import('../pages/admin/Leads'));
const AdminInvites = lazy(() => import('../pages/admin/Invites'));
const AdminSettings = lazy(() => import('../pages/admin/Settings'));
const AdminMcp = lazy(() => import('../pages/admin/Mcp'));
const AdminAudit = lazy(() => import('../pages/admin/Audit'));
const CourseStudents = lazy(() => import('../pages/CourseStudents'));
// T78: аналитика преподавателя — отдельный чанк подраздела кабинета.
const TeacherStats = lazy(() => import('../pages/TeacherStats'));
const Profile = lazy(() => import('../pages/Profile'));
// T68: публичная страница сертификата — редкая, отдельный чанк.
const Certificate = lazy(() => import('../pages/Certificate'));
// T82: публичный профиль преподавателя — отдельный чанк.
const TeacherProfile = lazy(() => import('../pages/TeacherProfile'));
// В Cabinets.tsx нет default-экспорта — берём именованный TeacherCab.
const TeacherCab = lazy(async () => {
  const m = await import('../pages/Cabinets');
  return { default: m.TeacherCab };
});
const NotFound = lazy(() => import('../pages/NotFound'));

/** Прокрутка к началу страницы при смене маршрута (иначе позиция скролла «уезжает»). */
function ScrollToTop(): null {
  const { pathname } = useLocation();
  useEffect(() => {
    window.scrollTo(0, 0);
  }, [pathname]);
  return null;
}

/**
 * Состояние загрузки ленивого маршрута (T18): скелетон как в гвардах
 * (классы shell.css) + русский текст из словаря. Отдельного ключа под чанки
 * в ru.json нет (словарём владеет T02) — используем общий common.loading.
 */
function RouteFallback(): JSX.Element {
  return (
    <div className="shell-skeleton" role="status" aria-live="polite">
      <span className="shell-skeleton-line" />
      <span className="shell-skeleton-line shell-skeleton-line--short" />
      <span className="sr-only">{t('common.loading')}</span>
    </div>
  );
}

function Layout(): JSX.Element {
  const qc = useQueryClient();
  // Один query ['me'] на всё приложение: Layout и гварды (RouteGuards.tsx)
  // используют общие meQueryOptions — без второго запроса и «моргания».
  const meQuery = useQuery(meQueryOptions);
  const user = meQuery.data;
  const location = useLocation();

  // Пост-P7: промо-баннер — тогл из админки, поэтому спрашиваем бэкенд, а не
  // рендерим всегда. Флаг публичный (доступен без сессии); пока не ответил —
  // полосу не показываем, чтобы выключенная акция не «мелькала» при загрузке.
  const promoQuery = useQuery({
    queryKey: ['public-settings'],
    queryFn: getPublicSettings,
    retry: false,
    staleTime: 60_000,
  });

  // T18: обработчик стабилен между рендерами (qc из контекста не меняется) —
  // кнопка не пересоздаёт onClick при каждом обновлении ['me'] и навигации.
  const onLogout = useCallback(async (): Promise<void> => {
    await logout();
    await setSessionUser(qc, null);
  }, [qc]);

  // Промо-полоса: закрывается крестиком (запоминаем в браузере) и не
  // показывается там, где мешает фокусу — в уроке и на экранах входа.
  const [promoDismissed, setPromoDismissed] = useState<boolean>(() => {
    try {
      return window.localStorage.getItem(PROMO_DISMISS_KEY) === '1';
    } catch {
      return false;
    }
  });
  const hidePromoHere = /^\/(learn|login|register|forgot-password|reset-password|verify-email|admin)(\/|$)/.test(
    location.pathname,
  );
  const showPromo = Boolean(promoQuery.data?.promoBarEnabled) && !promoDismissed && !hidePromoHere;
  const dismissPromo = (): void => {
    setPromoDismissed(true);
    try {
      window.localStorage.setItem(PROMO_DISMISS_KEY, '1');
    } catch {
      /* приватный режим — просто скрываем до перезагрузки */
    }
  };
  // В уроке футер не нужен: ниже урока — нижняя панель навигации по шагам.
  const isLesson = /^\/learn\/[^/]+\/[^/]+/.test(location.pathname);
  // Админка — рабочее приложение: без футера, на всю ширину.
  const isAdmin = /^\/admin(\/|$)/.test(location.pathname);
  const isAuthor = user?.role === 'teacher' || user?.role === 'admin';
  // Тексты промо-полосы и контакты — из админки (пустое — значения по умолчанию).
  const site = promoQuery.data?.site ?? {};
  const phone = site.contact_phone || '8 (800) 600-78-47';
  const contactEmail = site.contact_email || 'hello@edu-platform.ru';

  return (
    <div className={isLesson ? 'app app--lesson' : isAdmin ? 'app app--admin' : 'app'}>
      <ScrollToTop />
      <a href="#content" className="sr-only">
        {t('a11y.skipToContent')}
      </a>
      {showPromo && (
        <div className="promo">
          <span>{site.promo_text || 'Начните учиться бесплатно — регистрация займёт минуту'}</span>
          {/^https:\/\//.test(site.promo_link_url ?? '') ? (
            <a href={site.promo_link_url} rel="noopener noreferrer">{site.promo_link_text || 'Выбрать курс'}</a>
          ) : (
            <Link to={site.promo_link_url || '/'}>{site.promo_link_text || 'Выбрать курс'}</Link>
          )}
          <button type="button" className="promo-close" aria-label="Скрыть предложение" onClick={dismissPromo}>
            ×
          </button>
        </div>
      )}
      <header className="topbar">
        <div className="wrap topbar-in">
          <Link className="logo" to="/" aria-label="skillex.space — на главную">
            <span className="logo-mark" aria-hidden="true">S</span>skillex.space
          </Link>
          <nav className="nav" aria-label="Основная навигация">
            {navItems(user).map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.end}
                className={({ isActive }) => (isActive ? 'active' : '')}
              >
                {item.label}
              </NavLink>
            ))}
          </nav>
          <div className="top-actions">
            {user ? (
              <>
                <span className="user-email" title={user.email}>{user.email}</span>{' '}
                <button
                  type="button"
                  className="btn btn-ghost btn-sm top-logout"
                  onClick={() => void onLogout()}
                >
                  {t('nav.logout')}
                </button>
              </>
            ) : (
              <>
                <Link className="btn btn-ghost btn-sm top-login" to="/login">
                  {t('nav.login')}
                </Link>
                <Link className="btn btn-primary btn-sm top-start" to="/register">
                  Начать учиться
                </Link>
              </>
            )}
            <MobileNav user={user} onLogout={onLogout} />
          </div>
        </div>
      </header>
      <div id="content">
        {/* Граница ошибок поддерева: крах страницы не убивает шапку.
            Внутри — Suspense для ленивых маршрутов (T18): ошибка динамического
            импорта тоже попадает в ErrorBoundary, а не роняет приложение. */}
        <ErrorBoundary>
          <Suspense fallback={<RouteFallback />}>
            <Outlet />
          </Suspense>
        </ErrorBoundary>
      </div>
      {!isAdmin && (
      <footer className="footer">
        <div className="wrap footer-in">
          <div className="footer-brand">
            <Link className="logo" to="/">
              <span className="logo-mark" aria-hidden="true">S</span>skillex.space
            </Link>
            <p>
              <small>
                {site.footer_tagline ||
                  'Маркетплейс IT-курсов для начинающих. Учим спокойно, проверяем честно, поддерживаем на каждом шаге.'}
              </small>
            </p>
            <p className="contact-line">
              <a href={`tel:${phone.replace(/[^\d+]/g, '')}`}><b>{phone}</b></a>
              <a href={`mailto:${contactEmail}`}><small>{contactEmail}</small></a>
              <small>{site.contact_city || 'Москва'}</small>
            </p>
            <small>© skillex.space, 2026</small>
          </div>
          <div>
            <b>Направления</b>
            <Link to="/?category=programming">Программирование</Link>
            <Link to="/?category=design">Дизайн</Link>
            <Link to="/?category=data">Данные</Link>
            <Link to="/?category=marketing">Маркетинг</Link>
            <Link to="/?category=ai">Нейросети</Link>
            <Link to="/">Все курсы</Link>
          </div>
          <div>
            <b>Студенту</b>
            <Link to="/">Каталог</Link>
            {/* Авторизационные ссылки — по состоянию сессии, как в шапке. */}
            {user ? (
              <>
                <Link to="/me">Мои курсы</Link>
                <Link to="/profile">Профиль</Link>
              </>
            ) : (
              <>
                <Link to="/login">Вход</Link>
                <Link to="/register">Регистрация</Link>
              </>
            )}
          </div>
          {isAuthor && (
            <div>
              <b>Авторам</b>
              <Link to="/teach">Конструктор курса</Link>
              {user?.role === 'admin' && <Link to="/admin/moderation">Модерация</Link>}
              {user?.role === 'admin' && <Link to="/admin/invites">Приглашения</Link>}
            </div>
          )}
        </div>
      </footer>
      )}
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
      // T82: профиль преподавателя публичный — доступен без входа.
      { path: '/teachers/:id', element: <TeacherProfile /> },
      // T68: сертификат публичный — доступен без входа, можно делиться ссылкой.
      { path: '/certificates/:number', element: <Certificate /> },
      { path: '/login', element: <Login /> },
      { path: '/register', element: <Register /> },
      { path: '/forgot-password', element: <ForgotPassword /> },
      { path: '/reset-password', element: <ResetPassword /> },
      { path: '/verify-email', element: <VerifyEmail /> },
      // Кабинеты и обучение — за гвардами (RouteGuards.tsx, query ['me']).
      { path: '/me', element: <RequireAuth><MyCourses /></RequireAuth> },
      // T74: избранное — личный раздел, только вошедшим.
      { path: '/me/favorites', element: <RequireAuth><Favorites /></RequireAuth> },
      { path: '/teach', element: <RequireRole roles={['teacher', 'admin']}><TeacherCab /></RequireRole> },
      { path: '/teach/new', element: <RequireRole roles={['teacher', 'admin']}><Constructor /></RequireRole> },
      // T78: /teach/stats обязан идти ДО /teach/:id, иначе параметрический
      // маршрут перехватит «stats» и откроет конструктор.
      { path: '/teach/stats', element: <RequireRole roles={['teacher', 'admin']}><TeacherStats /></RequireRole> },
      { path: '/teach/:id', element: <RequireRole roles={['teacher', 'admin']}><Constructor /></RequireRole> },
      // Админка: общий каркас (боковое меню, поиск, уведомления) + разделы.
      {
        path: '/admin',
        element: <RequireRole roles={['admin']}><AdminLayout /></RequireRole>,
        children: [
          { index: true, element: <AdminOverview /> },
          { path: 'users', element: <AdminUsers /> },
          { path: 'users/:id', element: <AdminUserDetail /> },
          { path: 'courses', element: <AdminCourses /> },
          { path: 'courses/:id', element: <AdminCourseDetail /> },
          { path: 'moderation', element: <AdminModeration /> },
          { path: 'questions', element: <AdminQuestions /> },
          { path: 'reviews', element: <AdminReviews /> },
          { path: 'enrollments', element: <AdminEnrollments /> },
          { path: 'certificates', element: <AdminCertificates /> },
          { path: 'payments', element: <AdminPayments /> },
          { path: 'promo-codes', element: <AdminPromoCodes /> },
          { path: 'leads', element: <AdminLeads /> },
          { path: 'invites', element: <AdminInvites /> },
          { path: 'settings', element: <AdminSettings /> },
          { path: 'mcp', element: <AdminMcp /> },
          { path: 'audit', element: <AdminAudit /> },
          { path: 'stats', element: <Navigate to="/admin" replace /> },
          { path: '*', element: <Navigate to="/admin" replace /> },
        ],
      },
      // Ученики конкретного курса — автору курса и админу.
      { path: '/teach/:id/students', element: <RequireRole roles={['teacher', 'admin']}><CourseStudents /></RequireRole> },
      // Личный кабинет ученика: профиль и история платежей.
      { path: '/profile', element: <RequireAuth><Profile /></RequireAuth> },
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
