/**
 * Каркас админки: боковое меню по разделам (на телефоне — шторка), быстрый
 * поиск по пользователям, курсам и сертификатам (⌘K / Ctrl+K), счётчики
 * «требует внимания» у пунктов меню. Страницы рендерятся в <Outlet/>.
 */
import { Suspense, useEffect, useMemo, useRef, useState } from 'react';
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { getAdminOverview, overviewQueryKey } from '../../api/adminHub';
import { listUsers } from '../../api/adminUsers';
import { listAdminCourses } from '../../api/adminConsole';
import { adminListCertificates } from '../../api/adminOps';
import { AdminUiProvider, Icon, ROLE_LABEL, Spinner, useDebounced, type IconName } from './ui';
import '../../styles/adminShell.css';

type NavItem = { to: string; label: string; icon: IconName; end?: boolean; badge?: number };
type NavGroup = { title: string; items: NavItem[] };

function useNavGroups(): NavGroup[] {
  const { data } = useQuery({
    queryKey: overviewQueryKey,
    queryFn: () => getAdminOverview(30),
    staleTime: 60_000,
    refetchInterval: 120_000,
  });
  const a = data?.attention;
  return [
    {
      title: '',
      items: [{ to: '/admin', label: 'Обзор', icon: 'home', end: true }],
    },
    {
      title: 'Люди',
      items: [
        { to: '/admin/users', label: 'Пользователи', icon: 'users' },
        { to: '/admin/enrollments', label: 'Доступы к курсам', icon: 'key' },
        { to: '/admin/certificates', label: 'Сертификаты', icon: 'award' },
        { to: '/admin/leads', label: 'Заявки', icon: 'inbox', badge: a?.newLeads },
      ],
    },
    {
      title: 'Контент',
      items: [
        { to: '/admin/courses', label: 'Курсы', icon: 'book' },
        { to: '/admin/moderation', label: 'Модерация', icon: 'shield', badge: a?.reviewQueue },
        { to: '/admin/questions', label: 'Вопросы учеников', icon: 'message', badge: a?.unansweredQuestions },
        { to: '/admin/reviews', label: 'Отзывы', icon: 'star', badge: a?.lowRatingNoReply },
      ],
    },
    {
      title: 'Продажи',
      items: [
        { to: '/admin/payments', label: 'Платежи', icon: 'card', badge: a?.failedPayments7d },
        { to: '/admin/promo-codes', label: 'Промокоды', icon: 'tag' },
      ],
    },
    {
      title: 'Система',
      items: [
        { to: '/admin/invites', label: 'Приглашения', icon: 'mail' },
        { to: '/admin/settings', label: 'Настройки сайта', icon: 'settings' },
        { to: '/admin/mcp', label: 'MCP для нейросети', icon: 'bot' },
        { to: '/admin/audit', label: 'Журнал действий', icon: 'history' },
      ],
    },
  ];
}

function NavList({ groups, onNavigate }: { groups: NavGroup[]; onNavigate?: () => void }): JSX.Element {
  return (
    <nav className="ax-nav" aria-label="Разделы админки">
      {groups.map((g) => (
        <div key={g.title || 'main'} className="ax-nav-group">
          {g.title && <p className="ax-nav-title">{g.title}</p>}
          {g.items.map((it) => (
            <NavLink
              key={it.to}
              to={it.to}
              end={it.end}
              className={({ isActive }) => `ax-nav-link${isActive ? ' is-active' : ''}`}
              onClick={onNavigate}
            >
              <Icon name={it.icon} size={19} />
              <span className="ax-nav-label">{it.label}</span>
              {it.badge ? <span className="ax-nav-badge">{it.badge > 99 ? '99+' : it.badge}</span> : null}
            </NavLink>
          ))}
        </div>
      ))}
    </nav>
  );
}

/* ---------- быстрый поиск ---------- */

type Hit = { key: string; group: string; title: string; sub: string; to: string; icon: IconName };

function QuickSearch({ open, onClose }: { open: boolean; onClose: () => void }): JSX.Element | null {
  const [q, setQ] = useState('');
  const dq = useDebounced(q.trim(), 250);
  const [active, setActive] = useState(0);
  const navigate = useNavigate();
  const input = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (open) {
      setQ('');
      setActive(0);
      window.setTimeout(() => input.current?.focus(), 0);
    }
  }, [open]);

  const enabled = open && dq.length >= 2;
  const users = useQuery({ queryKey: ['admin', 'qs', 'users', dq], queryFn: () => listUsers({ q: dq, limit: 5 }), enabled });
  const courses = useQuery({ queryKey: ['admin', 'qs', 'courses', dq], queryFn: () => listAdminCourses({ q: dq, limit: 5 }), enabled });
  const certs = useQuery({ queryKey: ['admin', 'qs', 'certs', dq], queryFn: () => adminListCertificates(dq), enabled });

  const hits = useMemo<Hit[]>(() => {
    if (!enabled) return [];
    const out: Hit[] = [];
    for (const u of users.data?.items ?? []) {
      out.push({ key: `u${u.id}`, group: 'Пользователи', title: u.displayName || u.email, sub: `${u.email} · ${ROLE_LABEL[u.role] ?? u.role}`, to: `/admin/users/${u.id}`, icon: 'user' });
    }
    for (const c of courses.data?.items ?? []) {
      out.push({ key: `c${c.id}`, group: 'Курсы', title: c.title, sub: c.teacherEmail, to: `/admin/courses/${c.id}`, icon: 'book' });
    }
    for (const c of (certs.data?.items ?? []).slice(0, 5)) {
      out.push({ key: `s${c.number}`, group: 'Сертификаты', title: `№ ${c.number}`, sub: `${c.studentName || c.email} · ${c.courseTitle}`, to: `/admin/certificates?q=${encodeURIComponent(c.number)}`, icon: 'award' });
    }
    return out;
  }, [enabled, users.data, courses.data, certs.data]);

  useEffect(() => setActive(0), [dq]);

  if (!open) return null;
  const loading = enabled && (users.isFetching || courses.isFetching || certs.isFetching);
  const go = (h: Hit): void => {
    onClose();
    navigate(h.to);
  };

  let lastGroup = '';
  return (
    <div className="ax-overlay ax-overlay--top" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="ax-qs" role="dialog" aria-modal="true" aria-label="Быстрый поиск">
        <div className="ax-qs-input">
          <Icon name="search" size={20} />
          <input
            ref={input}
            value={q}
            placeholder="Email, имя, курс или номер сертификата"
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Escape') onClose();
              if (e.key === 'ArrowDown') {
                e.preventDefault();
                setActive((i) => Math.min(hits.length - 1, i + 1));
              }
              if (e.key === 'ArrowUp') {
                e.preventDefault();
                setActive((i) => Math.max(0, i - 1));
              }
              if (e.key === 'Enter' && hits[active]) go(hits[active]);
            }}
            aria-label="Поиск по админке"
          />
          <button type="button" className="ax-icon-btn" onClick={onClose} aria-label="Закрыть">
            <Icon name="x" size={18} />
          </button>
        </div>
        <div className="ax-qs-results">
          {!enabled && <p className="ax-qs-hint">Введите хотя бы 2 символа. Стрелки — выбор, Enter — открыть.</p>}
          {enabled && !loading && hits.length === 0 && <p className="ax-qs-hint">Ничего не найдено</p>}
          {enabled && loading && hits.length === 0 && <p className="ax-qs-hint">Ищем…</p>}
          {hits.map((h, i) => {
            const head = h.group !== lastGroup ? h.group : '';
            lastGroup = h.group;
            return (
              <div key={h.key}>
                {head && <p className="ax-qs-group">{head}</p>}
                <button
                  type="button"
                  className={`ax-qs-hit${i === active ? ' is-active' : ''}`}
                  onMouseEnter={() => setActive(i)}
                  onClick={() => go(h)}
                >
                  <Icon name={h.icon} size={18} />
                  <span>
                    <strong>{h.title}</strong>
                    <small>{h.sub}</small>
                  </span>
                </button>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}

/* ---------- каркас ---------- */

export default function AdminLayout(): JSX.Element {
  const groups = useNavGroups();
  const [menuOpen, setMenuOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const location = useLocation();

  useEffect(() => {
    const onKey = (e: KeyboardEvent): void => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setSearchOpen(true);
      }
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, []);

  useEffect(() => setMenuOpen(false), [location.pathname]);

  // На телефоне шапка сайта «прилипает» сверху — полоса разделов встаёт под неё.
  const shell = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const top = document.querySelector<HTMLElement>('.topbar');
    if (!top || !shell.current) return;
    const apply = (): void => {
      const sticky = getComputedStyle(top).position === 'sticky';
      shell.current?.style.setProperty('--ax-topbar-h', sticky ? `${top.offsetHeight}px` : '0px');
    };
    apply();
    const ro = new ResizeObserver(apply);
    ro.observe(top);
    window.addEventListener('resize', apply);
    return () => {
      ro.disconnect();
      window.removeEventListener('resize', apply);
    };
  }, []);

  useEffect(() => {
    if (!menuOpen) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') setMenuOpen(false);
    };
    document.addEventListener('keydown', onKey);
    return () => {
      document.body.style.overflow = prev;
      document.removeEventListener('keydown', onKey);
    };
  }, [menuOpen]);

  const current = groups.flatMap((g) => g.items).find((it) =>
    it.end ? location.pathname === it.to : location.pathname === it.to || location.pathname.startsWith(it.to + '/'),
  );
  const totalBadges = groups.flatMap((g) => g.items).reduce((s, it) => s + (it.badge ?? 0), 0);
  const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform);

  return (
    <AdminUiProvider>
      <div className="ax-shell" ref={shell}>
        <aside className="ax-side">
          <div className="ax-side-head">
            <Link to="/admin" className="ax-brand">
              <span className="ax-brand-mark" aria-hidden="true">S</span>
              <span>
                Админка
                <small>skillex.space</small>
              </span>
            </Link>
          </div>
          <button type="button" className="ax-side-search" onClick={() => setSearchOpen(true)}>
            <Icon name="search" size={18} />
            <span>Поиск</span>
            <kbd>{isMac ? '⌘K' : 'Ctrl K'}</kbd>
          </button>
          <NavList groups={groups} />
          <div className="ax-side-foot">
            <Link to="/" className="ax-nav-link">
              <Icon name="globe" size={19} />
              <span className="ax-nav-label">На сайт</span>
            </Link>
            <Link to="/teach/new" className="ax-nav-link">
              <Icon name="plus" size={19} />
              <span className="ax-nav-label">Новый курс</span>
            </Link>
          </div>
        </aside>

        <div className="ax-mobilebar">
          <button type="button" className="ax-mobilebar-menu" onClick={() => setMenuOpen(true)} aria-expanded={menuOpen}>
            <Icon name="menu" size={20} />
            <span>{current?.label ?? 'Админка'}</span>
            {totalBadges > 0 && <span className="ax-nav-badge">{totalBadges > 99 ? '99+' : totalBadges}</span>}
          </button>
          <button type="button" className="ax-icon-btn" onClick={() => setSearchOpen(true)} aria-label="Поиск">
            <Icon name="search" size={20} />
          </button>
        </div>

        {menuOpen && (
          <div className="ax-overlay" onMouseDown={(e) => e.target === e.currentTarget && setMenuOpen(false)}>
            <div className="ax-sheet" role="dialog" aria-modal="true" aria-label="Разделы админки">
              <div className="ax-drawer-head">
                <h2 className="ax-drawer-title">Разделы админки</h2>
                <button type="button" className="ax-icon-btn" onClick={() => setMenuOpen(false)} aria-label="Закрыть меню">
                  <Icon name="x" size={20} />
                </button>
              </div>
              <NavList groups={groups} onNavigate={() => setMenuOpen(false)} />
            </div>
          </div>
        )}

        <main className="ax-main" id="admin-main">
          <Suspense fallback={<Spinner />}>
            <Outlet />
          </Suspense>
        </main>
      </div>
      <QuickSearch open={searchOpen} onClose={() => setSearchOpen(false)} />
    </AdminUiProvider>
  );
}
