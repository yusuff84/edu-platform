import type { ReactNode } from 'react';
import { Link, Navigate, useLocation } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { me } from '../api/client';
import type { Role } from '../api/client';
import { t } from '../i18n/t';
import '../styles/shell.css';

/**
 * Общие опции query ['me'].
 * Layout (router.tsx) и оба гварда используют ОДИН И ТОТ ЖЕ объект опций:
 * TanStack Query дедуплицирует запросы по ключу, поэтому второго хода
 * на сервер нет, а закешированный пользователь рендерится мгновенно —
 * без «моргания» авторизации между защищёнными страницами.
 * staleTime 30с — переходы внутри сессии не дёргают /auth/me;
 * gcTime 10мин — данные пользователя переживают паузу между переходами.
 */
export const meQueryOptions = {
  queryKey: ['me'],
  queryFn: me,
  retry: false,
  staleTime: 30_000,
  gcTime: 10 * 60_000,
} as const;

/** Скелетон: рендерится вместо контента, пока определяется сессия (не контент!). */
function AuthSkeleton(): JSX.Element {
  return (
    <div className="shell-skeleton" role="status" aria-live="polite">
      <span className="shell-skeleton-line" />
      <span className="shell-skeleton-line shell-skeleton-line--short" />
    </div>
  );
}

/**
 * Страница «нет доступа»: пользователь авторизован, но роль не подходит.
 * Сознательно НЕ редиректим на /login — сессия есть, дело в правах.
 */
function NoAccess(): JSX.Element {
  return (
    <main className="shell-center">
      <h1>{t('error.noAccess')}</h1>
      <p>{t('cabinet.needRole')}</p>
      <p>
        <Link to="/me">{t('nav.myCourses')}</Link>
        {' · '}
        <Link to="/">{t('error.goCatalog')}</Link>
      </p>
    </main>
  );
}

/** Полный адрес внутри SPA, который нужно вернуть после входа. */
export type LoginRedirectLocation = {
  pathname: string;
  search: string;
  hash: string;
};

/**
 * State для /login: from — не только pathname, но и query/hash.
 * Auth.tsx уже принимает объект Location, поэтому после входа пользователь
 * возвращается ровно на ту страницу, с которой его перенаправил гвард.
 */
export function loginRedirectState(location: LoginRedirectLocation): {
  from: LoginRedirectLocation;
} {
  return {
    from: {
      pathname: location.pathname,
      search: location.search,
      hash: location.hash,
    },
  };
}

/**
 * Гвард «нужен вход». Пропускает только авторизованных.
 * Сессия = query ['me'] — та же, что грузится в Layout, без дублей запроса.
 */
export function RequireAuth({ children }: { children: ReactNode }): JSX.Element {
  const location = useLocation();
  const query = useQuery(meQueryOptions);

  if (query.isLoading) return <AuthSkeleton />;
  // Нет сессии (401) — на логин, с запоминанием полного адреса (pathname+search+hash).
  if (query.isError || !query.data)
    return <Navigate to="/login" state={loginRedirectState(location)} replace />;
  return <>{children}</>;
}

/**
 * Гвард роли. Нет сессии → /login; роль не в списке → страница «нет доступа».
 * /teach доступен и преподавателю, и админу: roles={['teacher', 'admin']}.
 */
export function RequireRole({
  roles,
  children,
}: {
  roles: Role[];
  children: ReactNode;
}): JSX.Element {
  const location = useLocation();
  const query = useQuery(meQueryOptions);

  if (query.isLoading) return <AuthSkeleton />;
  if (query.isError || !query.data)
    return <Navigate to="/login" state={loginRedirectState(location)} replace />;
  if (!roles.includes(query.data.role)) return <NoAccess />;
  return <>{children}</>;
}
