import { Suspense, lazy, useCallback, useEffect } from 'react';
import { Link, NavLink, Navigate, Outlet, createBrowserRouter, useLocation } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { getPublicSettings, logout } from '../api/client';
import { t } from '../i18n/t';
import Catalog from '../pages/Catalog';
import Course from '../pages/Course';
import { Login, Register } from '../pages/Auth';
import { ErrorBoundary, RouteError } from '../components/ErrorBoundary';
import { RequireAuth, RequireRole, meQueryOptions } from '../components/RouteGuards';
import '../styles/shell.css';

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
const Admin = lazy(() => import('../pages/Admin'));
// T26/T27/T29/T30: подразделы, разведённые по задачам отдельными страницами,
// чтобы агенты правили каждый свой файл и не конфликтовали.
const AdminUsers = lazy(() => import('../pages/AdminUsers'));
const AdminCourses = lazy(() => import('../pages/AdminCourses'));
const AdminStats = lazy(() => import('../pages/AdminStats'));
// T86: глобальная модерация Q&A — отдельный чанк подраздела «Вопросы и ответы».
const AdminQa = lazy(() => import('../pages/AdminQa'));
// T58: страница «Детали курса» — отдельный чанк подраздела «Курсы».
const AdminCourseDetail = lazy(() => import('../pages/AdminCourseDetail'));
// T49: экран «Записи на курсы» — отдельная страница подраздела.
const AdminEnrollments = lazy(() => import('../pages/AdminEnrollments'));
// T84: глобальная модерация отзывов — отдельный чанк админки.
const AdminReviews = lazy(() => import('../pages/AdminReviews'));
// T90: глобальный журнал промокодов — отдельный чанк админки.
const AdminPromoCodes = lazy(() => import('../pages/AdminPromoCodes'));
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
    // resetQueries сбрасывает кэш ['me'] и перезапрашивает: после выхода
    // пользователь гарантированно исчезает из шапки и гвардов (401 → логин).
    await qc.resetQueries({ queryKey: ['me'] });
  }, [qc]);

  return (
    <div>
      <ScrollToTop />
      <a href="#content" className="sr-only">
        {t('a11y.skipToContent')}
      </a>
      {promoQuery.data?.promoBarEnabled && (
        <div className="promo">
          <span>Первый курс со скидкой 20% для новых студентов</span>
          <Link to="/">Выбрать курс</Link>
        </div>
      )}
      <header className="topbar">
        <div className="wrap topbar-in">
          <Link className="logo" to="/">
            <span className="logo-mark">E</span>Edu&nbsp;Platform
          </Link>
          <nav className="nav" aria-label={t('nav.catalog')}>
            <NavLink to="/" end className={({ isActive }) => (isActive ? 'active' : '')}>
              {t('nav.catalog')}
            </NavLink>
            {/* «Мои курсы» (/me) — за гвардом RequireAuth: гостю ссылку не показываем. */}
            {user && (
              <NavLink to="/me" className={({ isActive }) => (isActive ? 'active' : '')}>
                {t('nav.myCourses')}
              </NavLink>
            )}
            {/* T74: «Избранное» — рядом с «Мои курсы», только вошедшим. */}
            {user && (
              <NavLink
                to="/me/favorites"
                className={({ isActive }) => (isActive ? 'active' : '')}
              >
                {t('p9.fav.title')}
              </NavLink>
            )}
            {user && (
              <NavLink to="/profile" className={({ isActive }) => (isActive ? 'active' : '')}>
                {t('nav.profile')}
              </NavLink>
            )}
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
        {/* Граница ошибок поддерева: крах страницы не убивает шапку.
            Внутри — Suspense для ленивых маршрутов (T18): ошибка динамического
            импорта тоже попадает в ErrorBoundary, а не роняет приложение. */}
        <ErrorBoundary>
          <Suspense fallback={<RouteFallback />}>
            <Outlet />
          </Suspense>
        </ErrorBoundary>
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
            {/* Авторизационные ссылки — по состоянию сессии, как в шапке. */}
            {user ? (
              <Link to="/me">Мои курсы</Link>
            ) : (
              <>
                <Link to="/login">Вход</Link>
                <Link to="/register">Регистрация</Link>
              </>
            )}
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
      { path: '/admin', element: <RequireRole roles={['admin']}><Admin /></RequireRole> },
      // Подразделы супер-админки — отдельные страницы со своим роутом
      // (каждая со своим владельцем-задачей).
      { path: '/admin/users', element: <RequireRole roles={['admin']}><AdminUsers /></RequireRole> },
      { path: '/admin/courses', element: <RequireRole roles={['admin']}><AdminCourses /></RequireRole> },
      // T58: детали курса — строго ПОСЛЕ /admin/courses (react-router сам
      // разрулит конфликт с /admin/courses/review: статичным путём владеет T07).
      { path: '/admin/courses/:id', element: <RequireRole roles={['admin']}><AdminCourseDetail /></RequireRole> },
      { path: '/admin/stats', element: <RequireRole roles={['admin']}><AdminStats /></RequireRole> },
      { path: '/admin/enrollments', element: <RequireRole roles={['admin']}><AdminEnrollments /></RequireRole> },
      // T84: модерация отзывов всех курсов — только admin.
      { path: '/admin/reviews', element: <RequireRole roles={['admin']}><AdminReviews /></RequireRole> },
      // T86: глобальная модерация вопросов и ответов по всем курсам.
      { path: '/admin/qa', element: <RequireRole roles={['admin']}><AdminQa /></RequireRole> },
      // T90: все промокоды платформы — только супер-админ.
      { path: '/admin/promo-codes', element: <RequireRole roles={['admin']}><AdminPromoCodes /></RequireRole> },
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
