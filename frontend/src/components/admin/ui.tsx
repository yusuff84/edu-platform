/**
 * Общие элементы интерфейса админки (стили — styles/adminShell.css, префикс ax-).
 * Все разделы админки собираются только из них, чтобы выглядеть одинаково
 * на десктопе и на телефоне.
 */
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { createPortal } from 'react-dom';
import { Link } from 'react-router-dom';
import { ApiError, formatMoney } from '../../api/client';

/* ------------------------------------------------------------------ */
/* Иконки (линейные, 24×24, stroke = currentColor)                      */
/* ------------------------------------------------------------------ */

const ICONS: Record<string, ReactNode> = {
  home: <><path d="M3 10.5 12 3l9 7.5" /><path d="M5 9.5V21h14V9.5" /><path d="M10 21v-6h4v6" /></>,
  users: <><circle cx="9" cy="8" r="3.5" /><path d="M2.5 20c.6-3.6 3.2-5.5 6.5-5.5s5.9 1.9 6.5 5.5" /><path d="M16 4.6a3.5 3.5 0 0 1 0 6.8" /><path d="M18 14.8c2 .7 3.2 2.4 3.5 5.2" /></>,
  user: <><circle cx="12" cy="8" r="4" /><path d="M4 21c.8-4.2 4-6.5 8-6.5s7.2 2.3 8 6.5" /></>,
  key: <><circle cx="8" cy="15" r="4" /><path d="m11 12 9-9" /><path d="m17 6 3 3" /><path d="m14.5 8.5 2 2" /></>,
  award: <><circle cx="12" cy="9" r="6" /><path d="m8.5 14 -1.5 7 5-2.5 5 2.5-1.5-7" /></>,
  inbox: <><path d="M3 13h5l1.5 3h5L16 13h5" /><path d="M5 5h14l2 8v6H3v-6z" /></>,
  book: <><path d="M4 4.5A1.5 1.5 0 0 1 5.5 3H20v15H5.5A1.5 1.5 0 0 0 4 19.5z" /><path d="M4 19.5A1.5 1.5 0 0 0 5.5 21H20" /><path d="M8 7h8" /></>,
  shield: <><path d="M12 3 4.5 6v5.5c0 4.6 3.2 8.2 7.5 9.5 4.3-1.3 7.5-4.9 7.5-9.5V6z" /><path d="m8.8 12 2.2 2.2 4.4-4.4" /></>,
  message: <><path d="M21 12a8 8 0 0 1-11.6 7.1L4 20.5l1.4-4.9A8 8 0 1 1 21 12z" /><path d="M9 10.5a3 3 0 1 1 3.8 2.9c-.5.1-.8.6-.8 1.1v.5" /><path d="M12 17.5h.01" /></>,
  star: <path d="m12 3 2.8 5.8 6.2.9-4.5 4.4 1.1 6.3L12 17.4l-5.6 3 1.1-6.3L3 9.7l6.2-.9z" />,
  card: <><rect x="2.5" y="5" width="19" height="14" rx="2.5" /><path d="M2.5 10h19" /><path d="M6.5 15h4" /></>,
  tag: <><path d="M3 12V4.5A1.5 1.5 0 0 1 4.5 3H12l9 9-9 9z" /><circle cx="7.5" cy="7.5" r="1.5" /></>,
  mail: <><rect x="3" y="5" width="18" height="14" rx="2.5" /><path d="m3.5 6.5 8.5 6.5 8.5-6.5" /></>,
  settings: <><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z" /></>,
  bot: <><rect x="4" y="8" width="16" height="12" rx="3" /><path d="M12 4v4" /><circle cx="12" cy="3.5" r="1" /><path d="M9 13v1.5" /><path d="M15 13v1.5" /><path d="M2 13v3" /><path d="M22 13v3" /></>,
  history: <><path d="M3 12a9 9 0 1 0 2.6-6.4L3 8" /><path d="M3 3v5h5" /><path d="M12 7v5l3 2" /></>,
  search: <><circle cx="11" cy="11" r="7" /><path d="m20 20-3.5-3.5" /></>,
  menu: <><path d="M4 6h16" /><path d="M4 12h16" /><path d="M4 18h16" /></>,
  x: <><path d="M18 6 6 18" /><path d="m6 6 12 12" /></>,
  more: <><circle cx="12" cy="5" r="1.4" /><circle cx="12" cy="12" r="1.4" /><circle cx="12" cy="19" r="1.4" /></>,
  chevronRight: <path d="m9 6 6 6-6 6" />,
  chevronLeft: <path d="m15 6-6 6 6 6" />,
  arrowLeft: <><path d="M19 12H5" /><path d="m12 19-7-7 7-7" /></>,
  external: <><path d="M14 4h6v6" /><path d="M20 4 10 14" /><path d="M19 14v5a1.5 1.5 0 0 1-1.5 1.5h-12A1.5 1.5 0 0 1 4 19V6.5A1.5 1.5 0 0 1 5.5 5H10" /></>,
  copy: <><rect x="8" y="8" width="13" height="13" rx="2.5" /><path d="M16 8V5.5A2.5 2.5 0 0 0 13.5 3h-8A2.5 2.5 0 0 0 3 5.5v8A2.5 2.5 0 0 0 5.5 16H8" /></>,
  check: <path d="m5 12.5 4.5 4.5L19 7.5" />,
  alert: <><path d="M12 3 2 20.5h20z" /><path d="M12 10v4.5" /><path d="M12 17.5h.01" /></>,
  download: <><path d="M12 3v12" /><path d="m7 10 5 5 5-5" /><path d="M4 20h16" /></>,
  plus: <><path d="M12 5v14" /><path d="M5 12h14" /></>,
  refresh: <><path d="M20 11a8 8 0 0 0-14.3-4.7L4 8" /><path d="M4 3v5h5" /><path d="M4 13a8 8 0 0 0 14.3 4.7L20 16" /><path d="M20 21v-5h-5" /></>,
  trash: <><path d="M4 7h16" /><path d="M9.5 7V4.5h5V7" /><path d="M6 7l1 13.5h10L18 7" /></>,
  edit: <><path d="M4 20h4L19 9l-4-4L4 16z" /><path d="m13.5 6.5 4 4" /></>,
  eye: <><path d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7S2 12 2 12z" /><circle cx="12" cy="12" r="3" /></>,
  lock: <><rect x="4.5" y="10.5" width="15" height="10" rx="2.5" /><path d="M8 10.5V7.5a4 4 0 0 1 8 0v3" /></>,
  unlock: <><rect x="4.5" y="10.5" width="15" height="10" rx="2.5" /><path d="M8 10.5V7.5a4 4 0 0 1 7.7-1.5" /></>,
  logout: <><path d="M15 4h3.5A1.5 1.5 0 0 1 20 5.5v13a1.5 1.5 0 0 1-1.5 1.5H15" /><path d="M10 17l-5-5 5-5" /><path d="M5 12h11" /></>,
  chart: <><path d="M4 20V10" /><path d="M10 20V4" /><path d="M16 20v-7" /><path d="M21 20H3" /></>,
  filter: <path d="M3 5h18l-7 8.5V20l-4-2v-4.5z" />,
  send: <><path d="M21 3 10 14" /><path d="m21 3-7 18-4-7-7-4z" /></>,
  globe: <><circle cx="12" cy="12" r="9" /><path d="M3 12h18" /><path d="M12 3c2.5 2.6 3.7 5.6 3.7 9s-1.2 6.4-3.7 9c-2.5-2.6-3.7-5.6-3.7-9S9.5 5.6 12 3z" /></>,
  bolt: <path d="M13 2 4 14h7l-1 8 9-12h-7z" />,
  clock: <><circle cx="12" cy="12" r="9" /><path d="M12 7v5l3.5 2" /></>,
};

export type IconName = keyof typeof ICONS;

export function Icon({ name, size = 20, className }: { name: IconName; size?: number; className?: string }): JSX.Element {
  return (
    <svg
      className={className ? `ax-icon ${className}` : 'ax-icon'}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {ICONS[name]}
    </svg>
  );
}

/* ------------------------------------------------------------------ */
/* Форматирование                                                       */
/* ------------------------------------------------------------------ */

export const fmtMoney = (cents: number): string => formatMoney(cents);

export function fmtDate(value: string | null | undefined): string {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short', year: 'numeric' });
}

export function fmtDateTime(value: string | null | undefined): string {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleString('ru-RU', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });
}

/** «5 минут назад», «вчера», «3 дня назад» — для лент и списков. */
export function fmtAgo(value: string | null | undefined): string {
  if (!value) return '—';
  const d = new Date(value);
  const diff = Date.now() - d.getTime();
  if (Number.isNaN(diff)) return '—';
  const min = Math.round(diff / 60000);
  if (min < 1) return 'только что';
  if (min < 60) return `${min} мин назад`;
  const h = Math.round(min / 60);
  if (h < 24) return `${h} ч назад`;
  const days = Math.round(h / 24);
  if (days === 1) return 'вчера';
  if (days < 7) return `${days} ${plural(days, 'день', 'дня', 'дней')} назад`;
  return fmtDate(value);
}

export function plural(n: number, one: string, few: string, many: string): string {
  const m10 = n % 10;
  const m100 = n % 100;
  if (m10 === 1 && m100 !== 11) return one;
  if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return few;
  return many;
}

/** Текст ошибки API для показа пользователю. */
export function errorText(error: unknown): string {
  if (error instanceof ApiError) return error.message || `Ошибка ${error.status}`;
  if (error instanceof Error) return error.message;
  return 'Что-то пошло не так';
}

export const ROLE_LABEL: Record<string, string> = {
  student: 'Ученик',
  teacher: 'Преподаватель',
  admin: 'Администратор',
};

/** Хук «значение с задержкой» — для полей поиска. */
export function useDebounced<T>(value: T, delay = 300): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const id = window.setTimeout(() => setV(value), delay);
    return () => window.clearTimeout(id);
  }, [value, delay]);
  return v;
}

/* ------------------------------------------------------------------ */
/* Каркас страницы                                                      */
/* ------------------------------------------------------------------ */

export function AdminPage({
  title,
  description,
  actions,
  back,
  children,
}: {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  /** Ссылка «назад» над заголовком (для страниц деталей). */
  back?: { to: string; label: string };
  children: ReactNode;
}): JSX.Element {
  return (
    <div className="ax-page">
      <header className="ax-page-head">
        <div className="ax-page-titles">
          {back && (
            <Link className="ax-back" to={back.to}>
              <Icon name="arrowLeft" size={16} /> {back.label}
            </Link>
          )}
          <h1 className="ax-title">{title}</h1>
          {description && <p className="ax-subtitle">{description}</p>}
        </div>
        {actions && <div className="ax-page-actions">{actions}</div>}
      </header>
      {children}
    </div>
  );
}

export function Card({
  title,
  description,
  actions,
  children,
  flush,
  className,
  id,
}: {
  title?: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  children?: ReactNode;
  /** Без внутренних отступов (для таблиц во всю ширину карточки). */
  flush?: boolean;
  className?: string;
  id?: string;
}): JSX.Element {
  return (
    <section id={id} className={`ax-card${flush ? ' ax-card--flush' : ''}${className ? ` ${className}` : ''}`}>
      {(title || actions) && (
        <div className="ax-card-head">
          <div>
            {title && <h2 className="ax-card-title">{title}</h2>}
            {description && <p className="ax-card-desc">{description}</p>}
          </div>
          {actions && <div className="ax-card-actions">{actions}</div>}
        </div>
      )}
      {children}
    </section>
  );
}

/** Панель фильтров над списком: поиск + селекты + кнопки, переносится на телефоне. */
export function Toolbar({ children, end }: { children: ReactNode; end?: ReactNode }): JSX.Element {
  return (
    <div className="ax-toolbar">
      <div className="ax-toolbar-main">{children}</div>
      {end && <div className="ax-toolbar-end">{end}</div>}
    </div>
  );
}

export function SearchInput({
  value,
  onChange,
  placeholder = 'Поиск',
  autoFocus,
  label,
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  autoFocus?: boolean;
  label?: string;
}): JSX.Element {
  return (
    <label className="ax-search">
      <Icon name="search" size={18} />
      <span className="sr-only">{label ?? placeholder}</span>
      <input
        type="search"
        value={value}
        placeholder={placeholder}
        autoFocus={autoFocus}
        onChange={(e) => onChange(e.target.value)}
      />
      {value && (
        <button type="button" className="ax-search-clear" aria-label="Очистить" onClick={() => onChange('')}>
          <Icon name="x" size={16} />
        </button>
      )}
    </label>
  );
}

/** Компактный селект для фильтров. */
export function FilterSelect({
  value,
  onChange,
  options,
  label,
}: {
  value: string;
  onChange: (v: string) => void;
  options: { value: string; label: string }[];
  label: string;
}): JSX.Element {
  return (
    <label className="ax-filter">
      <span className="sr-only">{label}</span>
      <select value={value} onChange={(e) => onChange(e.target.value)} aria-label={label}>
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </label>
  );
}

/** Сегментированный переключатель (вкладки-фильтры): «Все / Новые / Обработанные». */
export function Segmented<T extends string>({
  value,
  onChange,
  options,
  label,
}: {
  value: T;
  onChange: (v: T) => void;
  options: { value: T; label: string; count?: number }[];
  label: string;
}): JSX.Element {
  return (
    <div className="ax-seg" role="tablist" aria-label={label}>
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          role="tab"
          aria-selected={o.value === value}
          className={o.value === value ? 'is-active' : ''}
          onClick={() => onChange(o.value)}
        >
          {o.label}
          {o.count !== undefined && <span className="ax-seg-count">{o.count}</span>}
        </button>
      ))}
    </div>
  );
}

export type Tone = 'neutral' | 'ok' | 'warn' | 'danger' | 'info' | 'brand';

export function Badge({ tone = 'neutral', children }: { tone?: Tone; children: ReactNode }): JSX.Element {
  return <span className={`ax-badge ax-badge--${tone}`}>{children}</span>;
}

/** Кружок с инициалами — для пользователей в списках. */
export function Avatar({ name, size = 36 }: { name: string; size?: number }): JSX.Element {
  const letters = (name || '?')
    .replace(/@.*/, '')
    .split(/[\s._-]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0]?.toUpperCase() ?? '')
    .join('');
  let h = 0;
  for (const ch of name) h = (h * 31 + ch.charCodeAt(0)) % 360;
  return (
    <span
      className="ax-avatar"
      style={{ width: size, height: size, fontSize: size * 0.38, background: `hsl(${h} 70% 92%)`, color: `hsl(${h} 45% 32%)` }}
      aria-hidden="true"
    >
      {letters || '?'}
    </span>
  );
}

export function EmptyState({
  icon = 'inbox',
  title,
  text,
  action,
}: {
  icon?: IconName;
  title: string;
  text?: ReactNode;
  action?: ReactNode;
}): JSX.Element {
  return (
    <div className="ax-empty">
      <span className="ax-empty-icon">
        <Icon name={icon} size={26} />
      </span>
      <p className="ax-empty-title">{title}</p>
      {text && <p className="ax-empty-text">{text}</p>}
      {action}
    </div>
  );
}

export function Spinner({ label = 'Загрузка…' }: { label?: string }): JSX.Element {
  return (
    <div className="ax-loading" role="status">
      <span className="ax-spinner" aria-hidden="true" />
      {label}
    </div>
  );
}

export function ErrorNote({ error, onRetry }: { error: unknown; onRetry?: () => void }): JSX.Element {
  return (
    <div className="ax-error" role="alert">
      <Icon name="alert" size={18} />
      <span>{errorText(error)}</span>
      {onRetry && (
        <button type="button" className="ax-link" onClick={onRetry}>
          Повторить
        </button>
      )}
    </div>
  );
}

/** Подсказка/пояснение внутри карточки. */
export function Note({ tone = 'info', children }: { tone?: 'info' | 'warn' | 'ok'; children: ReactNode }): JSX.Element {
  return <div className={`ax-note ax-note--${tone}`}>{children}</div>;
}

/* ------------------------------------------------------------------ */
/* Таблица, которая на телефоне превращается в карточки                  */
/* ------------------------------------------------------------------ */

export type Column<T> = {
  key: string;
  header: ReactNode;
  render: (row: T) => ReactNode;
  /** Ширина колонки на десктопе (CSS). */
  width?: string;
  align?: 'left' | 'right' | 'center';
  /** На телефоне: 'title' — заголовок карточки, 'hidden' — не показывать, иначе — строка «подпись: значение». */
  mobile?: 'title' | 'hidden' | 'meta';
};

export function DataTable<T>({
  columns,
  rows,
  rowKey,
  onRowClick,
  empty,
  actions,
  caption,
}: {
  columns: Column<T>[];
  rows: T[];
  rowKey: (row: T) => string;
  onRowClick?: (row: T) => void;
  empty?: ReactNode;
  /** Действия строки (обычно <RowMenu/>), справа. */
  actions?: (row: T) => ReactNode;
  caption?: string;
}): JSX.Element {
  if (rows.length === 0) return <>{empty ?? <EmptyState title="Ничего не найдено" />}</>;
  const titleCol = columns.find((c) => c.mobile === 'title') ?? columns[0];
  return (
    <>
      <div className="ax-table-wrap">
        <table className="ax-table">
          {caption && <caption className="sr-only">{caption}</caption>}
          <thead>
            <tr>
              {columns.map((c) => (
                <th key={c.key} style={{ width: c.width, textAlign: c.align }} scope="col">
                  {c.header}
                </th>
              ))}
              {actions && (
                <th className="ax-col-actions" scope="col">
                  <span className="sr-only">Действия</span>
                </th>
              )}
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr
                key={rowKey(row)}
                className={onRowClick ? 'is-clickable' : undefined}
                onClick={onRowClick ? (e) => {
                  if ((e.target as HTMLElement).closest('a,button,input,select,textarea,label')) return;
                  onRowClick(row);
                } : undefined}
              >
                {columns.map((c) => (
                  <td key={c.key} style={{ textAlign: c.align }}>
                    {c.render(row)}
                  </td>
                ))}
                {actions && <td className="ax-col-actions">{actions(row)}</td>}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <ul className="ax-cards">
        {rows.map((row) => (
          <li
            key={rowKey(row)}
            className={onRowClick ? 'ax-cardrow is-clickable' : 'ax-cardrow'}
            onClick={onRowClick ? (e) => {
              if ((e.target as HTMLElement).closest('a,button,input,select,textarea,label')) return;
              onRowClick(row);
            } : undefined}
          >
            <div className="ax-cardrow-head">
              <div className="ax-cardrow-title">{titleCol.render(row)}</div>
              {actions && <div className="ax-cardrow-actions">{actions(row)}</div>}
            </div>
            <dl className="ax-cardrow-meta">
              {columns
                .filter((c) => c !== titleCol && c.mobile !== 'hidden')
                .map((c) => (
                  <div key={c.key}>
                    <dt>{c.header}</dt>
                    <dd>{c.render(row)}</dd>
                  </div>
                ))}
            </dl>
          </li>
        ))}
      </ul>
    </>
  );
}

export function Pager({
  page,
  total,
  limit,
  onPage,
}: {
  page: number;
  total: number;
  limit: number;
  onPage: (p: number) => void;
}): JSX.Element | null {
  if (total <= limit && page === 1) {
    return total > 0 ? <p className="ax-pager-info">Всего: {total}</p> : null;
  }
  const last = Math.max(1, Math.ceil(total / limit));
  const from = total === 0 ? 0 : (page - 1) * limit + 1;
  const to = Math.min(page * limit, total);
  return (
    <nav className="ax-pager" aria-label="Страницы">
      <button type="button" className="ax-icon-btn" disabled={page <= 1} onClick={() => onPage(page - 1)} aria-label="Предыдущая страница">
        <Icon name="chevronLeft" size={18} />
      </button>
      <span>
        {from}–{to} из {total}
      </span>
      <button type="button" className="ax-icon-btn" disabled={page >= last} onClick={() => onPage(page + 1)} aria-label="Следующая страница">
        <Icon name="chevronRight" size={18} />
      </button>
    </nav>
  );
}

/* ------------------------------------------------------------------ */
/* Меню действий строки (⋮)                                             */
/* ------------------------------------------------------------------ */

export type MenuItem =
  | {
      label: string;
      icon?: IconName;
      onClick?: () => void;
      to?: string;
      href?: string;
      danger?: boolean;
      disabled?: boolean;
    }
  | 'divider';

export function RowMenu({ items, label = 'Действия' }: { items: MenuItem[]; label?: string }): JSX.Element {
  const [open, setOpen] = useState(false);
  const btn = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState<{ top: number; left: number; up: boolean }>({ top: 0, left: 0, up: false });
  const id = useId();

  const place = useCallback(() => {
    const r = btn.current?.getBoundingClientRect();
    if (!r) return;
    const width = 240;
    const up = r.bottom + 320 > window.innerHeight && r.top > 320;
    setPos({
      top: up ? r.top : r.bottom + 6,
      left: Math.max(8, Math.min(window.innerWidth - width - 8, r.right - width)),
      up,
    });
  }, []);

  useEffect(() => {
    if (!open) return;
    place();
    const onDoc = (e: MouseEvent): void => {
      if (!menu.current?.contains(e.target as Node) && !btn.current?.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') {
        setOpen(false);
        btn.current?.focus();
      }
    };
    const onScroll = (): void => setOpen(false);
    document.addEventListener('mousedown', onDoc);
    document.addEventListener('keydown', onKey);
    window.addEventListener('scroll', onScroll, true);
    window.addEventListener('resize', onScroll);
    menu.current?.querySelector<HTMLElement>('[role=menuitem]:not([aria-disabled=true])')?.focus();
    return () => {
      document.removeEventListener('mousedown', onDoc);
      document.removeEventListener('keydown', onKey);
      window.removeEventListener('scroll', onScroll, true);
      window.removeEventListener('resize', onScroll);
    };
  }, [open, place]);

  const run = (it: Exclude<MenuItem, 'divider'>): void => {
    if (it.disabled) return;
    setOpen(false);
    it.onClick?.();
  };

  return (
    <>
      <button
        ref={btn}
        type="button"
        className="ax-icon-btn"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? id : undefined}
        aria-label={label}
        onClick={(e) => {
          e.stopPropagation();
          setOpen((v) => !v);
        }}
      >
        <Icon name="more" size={20} />
      </button>
      {open &&
        createPortal(
          <div
            ref={menu}
            id={id}
            role="menu"
            className={`ax-menu${pos.up ? ' is-up' : ''}`}
            style={{ top: pos.top, left: pos.left }}
            onKeyDown={(e) => {
              if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
              e.preventDefault();
              const els = Array.from(menu.current?.querySelectorAll<HTMLElement>('[role=menuitem]') ?? []);
              const i = els.indexOf(document.activeElement as HTMLElement);
              const next = els[(i + (e.key === 'ArrowDown' ? 1 : -1) + els.length) % els.length];
              next?.focus();
            }}
          >
            {items.map((it, i) => {
              if (it === 'divider') return <div key={`d${i}`} className="ax-menu-divider" role="separator" />;
              const cls = `ax-menu-item${it.danger ? ' is-danger' : ''}`;
              const inner = (
                <>
                  {it.icon && <Icon name={it.icon} size={18} />}
                  <span>{it.label}</span>
                </>
              );
              if (it.to && !it.disabled) {
                return (
                  <Link key={it.label} role="menuitem" className={cls} to={it.to} onClick={() => setOpen(false)}>
                    {inner}
                  </Link>
                );
              }
              if (it.href && !it.disabled) {
                return (
                  <a key={it.label} role="menuitem" className={cls} href={it.href} target="_blank" rel="noopener noreferrer" onClick={() => setOpen(false)}>
                    {inner}
                  </a>
                );
              }
              return (
                <button
                  key={it.label}
                  type="button"
                  role="menuitem"
                  className={cls}
                  aria-disabled={it.disabled || undefined}
                  onClick={() => run(it)}
                >
                  {inner}
                </button>
              );
            })}
          </div>,
          document.body,
        )}
    </>
  );
}

/* ------------------------------------------------------------------ */
/* Боковая панель (drawer) и диалог                                     */
/* ------------------------------------------------------------------ */

function useLockScroll(active: boolean): void {
  useEffect(() => {
    if (!active) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.body.style.overflow = prev;
    };
  }, [active]);
}

export function Drawer({
  open,
  onClose,
  title,
  subtitle,
  children,
  footer,
  wide,
}: {
  open: boolean;
  onClose: () => void;
  title: ReactNode;
  subtitle?: ReactNode;
  children: ReactNode;
  footer?: ReactNode;
  wide?: boolean;
}): JSX.Element | null {
  useLockScroll(open);
  const panel = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const prev = document.activeElement as HTMLElement | null;
    panel.current?.focus();
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('keydown', onKey);
      prev?.focus?.();
    };
  }, [open, onClose]);
  if (!open) return null;
  return createPortal(
    <div className="ax-overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div
        ref={panel}
        className={`ax-drawer${wide ? ' ax-drawer--wide' : ''}`}
        role="dialog"
        aria-modal="true"
        aria-label={typeof title === 'string' ? title : undefined}
        tabIndex={-1}
      >
        <div className="ax-drawer-head">
          <div>
            <h2 className="ax-drawer-title">{title}</h2>
            {subtitle && <p className="ax-drawer-sub">{subtitle}</p>}
          </div>
          <button type="button" className="ax-icon-btn" onClick={onClose} aria-label="Закрыть">
            <Icon name="x" size={20} />
          </button>
        </div>
        <div className="ax-drawer-body">{children}</div>
        {footer && <div className="ax-drawer-foot">{footer}</div>}
      </div>
    </div>,
    document.body,
  );
}

export function Modal({
  open,
  onClose,
  title,
  children,
  footer,
}: {
  open: boolean;
  onClose: () => void;
  title: ReactNode;
  children: ReactNode;
  footer?: ReactNode;
}): JSX.Element | null {
  useLockScroll(open);
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const prev = document.activeElement as HTMLElement | null;
    const first = box.current?.querySelector<HTMLElement>('input,select,textarea,button:not(.ax-modal-x)');
    (first ?? box.current)?.focus();
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('keydown', onKey);
      prev?.focus?.();
    };
  }, [open, onClose]);
  if (!open) return null;
  return createPortal(
    <div className="ax-overlay ax-overlay--center" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div ref={box} className="ax-modal" role="dialog" aria-modal="true" aria-label={typeof title === 'string' ? title : undefined} tabIndex={-1}>
        <div className="ax-modal-head">
          <h2 className="ax-modal-title">{title}</h2>
          <button type="button" className="ax-icon-btn ax-modal-x" onClick={onClose} aria-label="Закрыть">
            <Icon name="x" size={20} />
          </button>
        </div>
        <div className="ax-modal-body">{children}</div>
        {footer && <div className="ax-modal-foot">{footer}</div>}
      </div>
    </div>,
    document.body,
  );
}

/* ------------------------------------------------------------------ */
/* Подтверждения и уведомления (провайдеры — в AdminLayout)              */
/* ------------------------------------------------------------------ */

type ConfirmOptions = {
  title: string;
  text?: ReactNode;
  confirmLabel?: string;
  danger?: boolean;
  /** Если задано — нужно ввести это слово для подтверждения (для необратимых действий). */
  typeToConfirm?: string;
};

type ToastItem = { id: number; tone: 'ok' | 'error' | 'info'; text: string };

type UiCtx = {
  confirm: (o: ConfirmOptions) => Promise<boolean>;
  toast: (text: string, tone?: ToastItem['tone']) => void;
};

const UiContext = createContext<UiCtx | null>(null);

export function AdminUiProvider({ children }: { children: ReactNode }): JSX.Element {
  const [dialog, setDialog] = useState<(ConfirmOptions & { resolve: (v: boolean) => void }) | null>(null);
  const [typed, setTyped] = useState('');
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const seq = useRef(0);

  const confirm = useCallback(
    (o: ConfirmOptions) =>
      new Promise<boolean>((resolve) => {
        setTyped('');
        setDialog({ ...o, resolve });
      }),
    [],
  );
  const toast = useCallback((text: string, tone: ToastItem['tone'] = 'ok') => {
    const id = ++seq.current;
    setToasts((t) => [...t, { id, tone, text }]);
    window.setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), tone === 'error' ? 6000 : 3500);
  }, []);
  const value = useMemo(() => ({ confirm, toast }), [confirm, toast]);

  const close = (v: boolean): void => {
    dialog?.resolve(v);
    setDialog(null);
  };
  const blocked = Boolean(dialog?.typeToConfirm) && typed.trim() !== dialog?.typeToConfirm;

  return (
    <UiContext.Provider value={value}>
      {children}
      <Modal
        open={dialog !== null}
        onClose={() => close(false)}
        title={dialog?.title ?? ''}
        footer={
          <>
            <button type="button" className="btn btn-ghost" onClick={() => close(false)}>
              Отмена
            </button>
            <button
              type="button"
              className={dialog?.danger ? 'btn ax-btn-danger' : 'btn btn-primary'}
              disabled={blocked}
              onClick={() => close(true)}
            >
              {dialog?.confirmLabel ?? 'Подтвердить'}
            </button>
          </>
        }
      >
        {dialog?.text && <div className="ax-confirm-text">{dialog.text}</div>}
        {dialog?.typeToConfirm && (
          <Field label={`Чтобы подтвердить, введите «${dialog.typeToConfirm}»`}>
            <input className="ax-input" value={typed} onChange={(e) => setTyped(e.target.value)} autoComplete="off" />
          </Field>
        )}
      </Modal>
      {createPortal(
        <div className="ax-toasts" aria-live="polite">
          {toasts.map((t) => (
            <div key={t.id} className={`ax-toast ax-toast--${t.tone}`}>
              <Icon name={t.tone === 'error' ? 'alert' : 'check'} size={18} />
              <span>{t.text}</span>
            </div>
          ))}
        </div>,
        document.body,
      )}
    </UiContext.Provider>
  );
}

/** confirm() и toast() внутри админки. */
export function useAdminUi(): UiCtx {
  const ctx = useContext(UiContext);
  if (!ctx) {
    return {
      confirm: async (o) => window.confirm(o.title),
      toast: (text) => console.info(text),
    };
  }
  return ctx;
}

/* ------------------------------------------------------------------ */
/* Формы                                                                */
/* ------------------------------------------------------------------ */

export function Field({
  label,
  hint,
  error,
  children,
}: {
  label: ReactNode;
  hint?: ReactNode;
  error?: ReactNode;
  children: ReactNode;
}): JSX.Element {
  return (
    <label className="ax-field">
      <span className="ax-field-label">{label}</span>
      {children}
      {hint && !error && <span className="ax-field-hint">{hint}</span>}
      {error && <span className="ax-field-error">{error}</span>}
    </label>
  );
}

/** Переключатель «вкл/выкл». */
export function Switch({
  checked,
  onChange,
  label,
  hint,
  disabled,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  label: ReactNode;
  hint?: ReactNode;
  disabled?: boolean;
}): JSX.Element {
  return (
    <label className={`ax-switch${disabled ? ' is-disabled' : ''}`}>
      <input type="checkbox" role="switch" checked={checked} disabled={disabled} onChange={(e) => onChange(e.target.checked)} />
      <span className="ax-switch-track" aria-hidden="true" />
      <span className="ax-switch-text">
        <span>{label}</span>
        {hint && <small>{hint}</small>}
      </span>
    </label>
  );
}

export function CopyButton({ text, label = 'Скопировать' }: { text: string; label?: string }): JSX.Element {
  const [done, setDone] = useState(false);
  return (
    <button
      type="button"
      className="btn btn-ghost btn-sm ax-copy"
      onClick={() => {
        void navigator.clipboard?.writeText(text).then(() => {
          setDone(true);
          window.setTimeout(() => setDone(false), 1600);
        });
      }}
    >
      <Icon name={done ? 'check' : 'copy'} size={16} /> {done ? 'Скопировано' : label}
    </button>
  );
}

/* ------------------------------------------------------------------ */
/* Сводные карточки и вкладки                                            */
/* ------------------------------------------------------------------ */

export function StatCard({
  label,
  value,
  hint,
  icon,
  to,
  tone = 'neutral',
}: {
  label: string;
  value: ReactNode;
  hint?: ReactNode;
  icon?: IconName;
  to?: string;
  tone?: Tone;
}): JSX.Element {
  const body = (
    <>
      <div className="ax-stat-top">
        <span className="ax-stat-label">{label}</span>
        {icon && (
          <span className={`ax-stat-icon ax-tone--${tone}`}>
            <Icon name={icon} size={18} />
          </span>
        )}
      </div>
      <div className="ax-stat-value">{value}</div>
      {hint && <div className="ax-stat-hint">{hint}</div>}
    </>
  );
  return to ? (
    <Link className="ax-stat is-link" to={to}>
      {body}
    </Link>
  ) : (
    <div className="ax-stat">{body}</div>
  );
}

export function Tabs<T extends string>({
  tabs,
  value,
  onChange,
  label,
}: {
  tabs: { id: T; label: string; count?: number }[];
  value: T;
  onChange: (v: T) => void;
  label: string;
}): JSX.Element {
  return (
    <div className="ax-tabs" role="tablist" aria-label={label}>
      {tabs.map((t) => (
        <button
          key={t.id}
          type="button"
          role="tab"
          aria-selected={t.id === value}
          className={t.id === value ? 'is-active' : ''}
          onClick={() => onChange(t.id)}
        >
          {t.label}
          {t.count !== undefined && t.count > 0 && <span className="ax-tab-count">{t.count}</span>}
        </button>
      ))}
    </div>
  );
}

/** Пара «подпись — значение» для карточек деталей. */
export function InfoList({ items }: { items: { label: string; value: ReactNode }[] }): JSX.Element {
  return (
    <dl className="ax-info">
      {items.map((it) => (
        <div key={it.label}>
          <dt>{it.label}</dt>
          <dd>{it.value}</dd>
        </div>
      ))}
    </dl>
  );
}

/** Выгрузка массива строк в CSV (Excel-совместимый, с BOM и «;»). */
export function downloadCsv(fileName: string, header: string[], rows: (string | number)[][]): void {
  const cell = (v: string | number): string => {
    const s = String(v ?? '');
    return /[";\n\r]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
  };
  const text = [header, ...rows].map((r) => r.map(cell).join(';')).join('\r\n');
  const blob = new Blob(['﻿' + text], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = fileName;
  document.body.appendChild(a);
  a.click();
  a.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}
