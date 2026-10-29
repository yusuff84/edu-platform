import { Suspense, lazy, useCallback, useEffect, useRef, useState } from 'react';
import { Link, NavLink, Navigate, Outlet, createBrowserRouter, useLocation } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { getPublicSettings, logout } from '../api/client';
import { t } from '../i18n/t';
import Catalog from '../pages/Catalog';
import { Login, Register } from '../pages/Auth';
import { ErrorBoundary, RouteError } from '../components/ErrorBoundary';
import { RequireAuth, RequireRole, meQueryOptions } from '../components/RouteGuards';
import '../styles/shell.css';

// T18: route-based code splitting. Каталог и вход остаются статическими — это
// первый экран анонимного посетителя, он не должен ждать догрузки. Страница
// курса вынесена в отдельный чанк: она тянет Markdown/Prism и редактор кода,
// которым не место в стартовом бандле. Остальные разделы (админка, конструктор,
// обучение, кабинеты, 404) видны после входа/роли или открываются редко — их
// чанки подгружаются по факту рендера, из главного чанка они исключены.
// P11: прямой заход на /courses/:slug работает так же: Layout рендерит
// Suspense-fallback, пока грузится чанк Course.
const Course = lazy(() => import('../pages/Course'));
const MyCourses = lazy(() => import('../pages/MyCourses'));
// T74: избранное — личный раздел вошедшего, отдельный чанк.
const Favorites = lazy(() => import('../pages/Favorites'));
const Learn = lazy(() => import('../pages/Learn'));
const Constructor = lazy(() => import('../pages/Constructor'));
const Admin = lazy(() => import('../pages/Admin'));
// Разделы, которые раньше жили якорями на одной странице /admin, вынесены
// в самостоятельные маршруты со своими чанками.
const AdminModeration = lazy(() => import('../pages/AdminModeration'));
const AdminPayments = lazy(() => import('../pages/AdminPayments'));
const AdminSettings = lazy(() => import('../pages/AdminSettings'));
const AdminInvites = lazy(() => import('../pages/AdminInvites'));
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

/**
 * Прокрутка к началу страницы при смене маршрута (иначе позиция скролла
 * «уезжает»). Если в URL есть якорь (#lead и т.п.) — прокручиваем
 * к нему: так осмысленные ссылки в футере работают и при переходе с другой
 * страницы, а не только при открытии URL напрямую.
 */
function ScrollToTop(): null {
  const { pathname, hash } = useLocation();
  useEffect(() => {
    if (hash) {
      let id = hash.slice(1);
      try {
        id = decodeURIComponent(id);
      } catch {
        // Некорректный percent-encoding — оставляем якорь как есть.
      }
      // Даём React зафиксировать разметку страницы до поиска элемента.
      const timer = window.setTimeout(() => {
        document.getElementById(id)?.scrollIntoView({ block: 'start' });
      }, 0);
      return () => window.clearTimeout(timer);
    }
    window.scrollTo(0, 0);
    return undefined;
  }, [pathname, hash]);
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

/** Короткое имя для шапки: displayName → локальная часть email. */
function shortUserName(user: { displayName?: string; email: string }): string {
  const displayName = user.displayName?.trim();
  if (displayName) return displayName.split(/\s+/)[0];
  const emailName = user.email.split('@')[0] ?? '';
  return emailName ? emailName.charAt(0).toLocaleUpperCase('ru') + emailName.slice(1) : 'Профиль';
}

/** Инициалы для круглого аватара в шапке. */
function userInitials(user: { displayName?: string; email: string }): string {
  const source = user.displayName?.trim() || user.email.split('@')[0] || '';
  const letters = source
    .split(/[\s._-]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part.charAt(0).toLocaleUpperCase('ru'))
    .join('');
  return letters || 'E';
}

function Layout(): JSX.Element {
  const qc = useQueryClient();
  const location = useLocation();
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const mobileNavRef = useRef<HTMLElement>(null);
  const burgerRef = useRef<HTMLButtonElement>(null);
  const wasMobileNavOpen = useRef(false);
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

  // Мобильное меню — часть шапки: закрываем его при переходе на другой
  // маршрут и по Escape, чтобы оно не оставалось поверх новой страницы.
  useEffect(() => {
    setMobileNavOpen(false);
  }, [location.pathname, location.search, location.hash]);

  useEffect(() => {
    if (!mobileNavOpen) return undefined;
    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key === 'Escape') setMobileNavOpen(false);
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [mobileNavOpen]);

  // При открытии уводим фокус в первый пункт меню, при закрытии — возвращаем
  // на burger. Так клавиатурный сценарий не теряется за полноэкранным scrim.
  useEffect(() => {
    if (mobileNavOpen) {
      mobileNavRef.current?.querySelector<HTMLElement>('a')?.focus();
    } else if (wasMobileNavOpen.current) {
      burgerRef.current?.focus();
    }
    wasMobileNavOpen.current = mobileNavOpen;
  }, [mobileNavOpen]);

  // Пока открыт drawer, фон не прокручивается под ним.
  useEffect(() => {
    if (!mobileNavOpen) return undefined;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.body.style.overflow = previousOverflow;
    };
  }, [mobileNavOpen]);

  // T18: обработчик стабилен между рендерами (qc из контекста не меняется) —
  // кнопка не пересоздаёт onClick при каждом обновлении ['me'] и навигации.
  const onLogout = useCallback(async (): Promise<void> => {
    setMobileNavOpen(false);
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
          {/* P11: aria-label описывает всю навигацию, а не только «Каталог».
              Литерал: отдельного ключа в ru.json нет (словарём владеет T02). */}
          <nav
            ref={mobileNavRef}
            id="primary-nav"
            className={`nav${mobileNavOpen ? ' nav--open' : ''}`}
            aria-label="Основная навигация"
            onClick={(event) => {
              if ((event.target as HTMLElement).closest('a')) setMobileNavOpen(false);
            }}
          >
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
            <div className="nav-mobile-auth">
              {user ? (
                <button
                  type="button"
                  className="nav-mobile-logout"
                  onClick={() => void onLogout()}
                >
                  {t('nav.logout')}
                </button>
              ) : (
                <>
                  <Link to="/login">{t('nav.login')}</Link>
                  <Link to="/register">Начать учиться</Link>
                </>
              )}
            </div>
          </nav>
          <div className="top-actions">
            {user ? (
              <>
                <Link
                  className="user-chip"
                  to="/profile"
                  title={user.email}
                  aria-label={`Профиль: ${shortUserName(user)}`}
                >
                  <span className="user-avatar" aria-hidden="true">
                    {userInitials(user)}
                  </span>
                  <span className="user-chip-text">
                    <span className="user-chip-name">{shortUserName(user)}</span>
                    <span className="user-chip-sub">Профиль</span>
                  </span>
                </Link>
                <button
                  type="button"
                  className="btn btn-ghost btn-sm topbar-logout"
                  onClick={() => void onLogout()}
                >
                  {t('nav.logout')}
                </button>
              </>
            ) : (
              <>
                <Link className="btn btn-ghost btn-sm topbar-auth" to="/login">
                  {t('nav.login')}
                </Link>
                <Link className="btn btn-primary btn-sm topbar-auth" to="/register">
                  Начать учиться
                </Link>
              </>
            )}
            <button
              ref={burgerRef}
              type="button"
              className="topbar-burger"
              aria-expanded={mobileNavOpen}
              aria-controls="primary-nav"
              aria-label={mobileNavOpen ? 'Закрыть меню' : 'Открыть меню'}
              onClick={() => setMobileNavOpen((open) => !open)}
            >
              <span className="topbar-burger-lines" aria-hidden="true">
                <span />
                <span />
                <span />
              </span>
            </button>
          </div>
        </div>
      </header>
      {mobileNavOpen && (
        <button
          type="button"
          className="nav-scrim"
          aria-label="Закрыть меню"
          onClick={() => setMobileNavOpen(false)}
        />
      )}
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
            {/* P11: ссылки направлений ведут в каталог с фильтром по категории,
                а не на общий «/» (иначе все пункты выглядели одинаково). */}
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
            {/* P11: осмысленные якоря на существующий блок подбора курса
                в каталоге (#lead). Новых страниц под эти ссылки не заводим. */}
            <Link to="/#lead">Помощь с выбором</Link>
            <Link to="/#lead">Условия возврата</Link>
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
            {/* P11: админ-разделы видны только администратору — тот же
                query ['me'], что и в шапке/гвардах, без нового запроса. */}
            {user?.role === 'admin' && (
              <>
                <Link to="/admin/moderation">Модерация</Link>
                <Link to="/admin/invites">Приглашения</Link>
              </>
            )}
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
      // Бывшие секции /admin#... — теперь самостоятельные страницы.
      // Статичные пути не конфликтуют с /admin/courses/:id (разные сегменты).
      { path: '/admin/moderation', element: <RequireRole roles={['admin']}><AdminModeration /></RequireRole> },
      { path: '/admin/payments', element: <RequireRole roles={['admin']}><AdminPayments /></RequireRole> },
      { path: '/admin/settings', element: <RequireRole roles={['admin']}><AdminSettings /></RequireRole> },
      { path: '/admin/invites', element: <RequireRole roles={['admin']}><AdminInvites /></RequireRole> },
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
