import { useEffect, useId, useRef, useState } from 'react';
import { Link, NavLink, useLocation } from 'react-router-dom';
import type { User } from '../api/client';
import { t } from '../i18n/t';

interface NavItem {
  to: string;
  label: string;
  icon: string;
  end?: boolean;
}

/** Пункты навигации по роли — общий источник для шапки (десктоп) и мобильного меню. */
export function navItems(user: User | null | undefined): NavItem[] {
  const items: NavItem[] = [{ to: '/', label: t('nav.catalog'), icon: '⌂', end: true }];
  if (user) {
    items.push(
      { to: '/me', label: t('nav.myCourses'), icon: '▶', end: true },
      { to: '/me/favorites', label: t('p9.fav.title'), icon: '♡' },
      { to: '/profile', label: t('nav.profile'), icon: '◉' },
    );
  }
  if (user?.role === 'teacher' || user?.role === 'admin') {
    items.push({ to: '/teach', label: t('nav.teach'), icon: '✎' });
  }
  if (user?.role === 'admin') {
    items.push({ to: '/admin', label: t('nav.admin'), icon: '⚙' });
  }
  return items;
}

/**
 * Мобильное меню (≤960px, где полноразмерная навигация шапки скрыта).
 * Кнопка-«бургер» открывает панель поверх страницы: пункты по роли,
 * учётка и выход (или вход/регистрация для гостя). Закрывается по Esc,
 * по тапу на фон и при переходе на другой маршрут; прокрутка страницы
 * под открытой панелью блокируется.
 */
export function MobileNav({
  user,
  onLogout,
}: {
  user: User | null | undefined;
  onLogout: () => Promise<void>;
}): JSX.Element {
  const [open, setOpen] = useState(false);
  const { pathname } = useLocation();
  const panelId = useId();
  const buttonRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);

  // Переход по ссылке закрывает меню.
  useEffect(() => {
    setOpen(false);
  }, [pathname]);

  useEffect(() => {
    if (!open) return undefined;
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') {
        setOpen(false);
        buttonRef.current?.focus();
      }
    };
    document.addEventListener('keydown', onKey);
    // Фокус на первый пункт — клавиатура и скринридер сразу внутри меню.
    panelRef.current?.querySelector<HTMLElement>('nav a')?.focus();
    return () => {
      document.body.style.overflow = prevOverflow;
      document.removeEventListener('keydown', onKey);
    };
  }, [open]);

  const items = navItems(user);

  return (
    <>
      <button
        ref={buttonRef}
        type="button"
        className={`mnav-toggle${open ? ' is-open' : ''}`}
        aria-expanded={open}
        aria-controls={panelId}
        aria-label={open ? 'Закрыть меню' : 'Открыть меню'}
        onClick={() => setOpen((v) => !v)}
      >
        <span aria-hidden="true" />
        <span aria-hidden="true" />
        <span aria-hidden="true" />
      </button>
      <div
        className={`mnav-backdrop${open ? ' is-open' : ''}`}
        aria-hidden="true"
        onClick={() => setOpen(false)}
      />
      <div
        ref={panelRef}
        id={panelId}
        className={`mnav-panel${open ? ' is-open' : ''}`}
        role="dialog"
        aria-modal="true"
        aria-label="Меню"
        hidden={!open}
      >
        <button
          type="button"
          className="mnav-close"
          aria-label="Закрыть меню"
          onClick={() => {
            setOpen(false);
            buttonRef.current?.focus();
          }}
        >
          ×
        </button>
        {user && (
          <div className="mnav-user">
            <span className="mnav-avatar" aria-hidden="true">
              {(user.displayName || user.email).trim().charAt(0).toUpperCase()}
            </span>
            <span className="mnav-user-text">
              {user.displayName && <b>{user.displayName}</b>}
              <span>{user.email}</span>
            </span>
          </div>
        )}
        <nav aria-label="Основная навигация">
          {items.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) => `mnav-link${isActive ? ' active' : ''}`}
            >
              <span className="mnav-icon" aria-hidden="true">{item.icon}</span>
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="mnav-actions">
          {user ? (
            <button
              type="button"
              className="btn mnav-logout"
              onClick={() => {
                setOpen(false);
                void onLogout();
              }}
            >
              {t('nav.logout')}
            </button>
          ) : (
            <>
              <Link className="btn btn-primary" to="/register">
                Начать учиться
              </Link>
              <Link className="btn" to="/login">
                {t('nav.login')}
              </Link>
            </>
          )}
        </div>
      </div>
    </>
  );
}
