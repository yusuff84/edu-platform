import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { listCourses, me } from '../api/client';
import CourseCard from '../components/CourseCard';
import {
  DiplomaBlock,
  DreamsBlock,
  FinalCta,
  LeadBlock,
  PerksBlock,
  SupportBlock,
} from '../components/marketing';
import { t } from '../i18n/t';
import '../styles/catalog.css';

/** Размер страницы каталога (контракт: limit по умолч. 20, максимум 100). */
const LIMIT = 12;
/** Сколько скелетонов показывать во время загрузки. */
const SKELETONS = 6;
/** Задержка поиска, мс. */
const DEBOUNCE_MS = 300;

type Seg = 'often' | 'rating' | 'price';

const PILLS: Array<{ label: string; q: string }> = [
  { label: 'Популярное', q: '' },
  { label: 'Программирование', q: 'python' },
  { label: 'Дизайн', q: 'дизайн' },
  { label: 'Данные', q: 'sql' },
  { label: 'Маркетинг', q: 'маркетинг' },
  { label: 'Нейросети', q: 'нейро' },
];

const SEGS: Array<{ key: Seg; label: string }> = [
  { key: 'often', label: 'Часто выбирают' },
  { key: 'rating', label: 'Высокий рейтинг' },
  { key: 'price', label: 'Доступные' },
];

export default function Catalog(): JSX.Element {
  const [searchParams, setSearchParams] = useSearchParams();
  // Поиск живёт в URL (?q=) — ссылку можно передать, «назад/вперёд» работает.
  const urlQ = searchParams.get('q') ?? '';

  // Локальное значение поля ввода; в URL попадает после дебаунса.
  const [q, setQ] = useState(urlQ);
  const [page, setPage] = useState(1);
  const [seg, setSeg] = useState<Seg>('often');
  const [activePill, setActivePill] = useState('Популярное');
  const cardsRef = useRef<HTMLDivElement>(null);

  // Изменение URL извне (кнопка «назад», внешняя ссылка) → текст в поле.
  useEffect(() => {
    setQ(urlQ);
  }, [urlQ]);

  // Дебаунс 300 мс: пишем запрос в ?q= и сбрасываем страницу на первую.
  useEffect(() => {
    const next = q.trim();
    if (next === urlQ) return;
    const id = window.setTimeout(() => {
      setSearchParams(
        (prev) => {
          const p = new URLSearchParams(prev);
          if (next) p.set('q', next);
          else p.delete('q');
          return p;
        },
        { replace: true },
      );
      setPage(1);
    }, DEBOUNCE_MS);
    return () => window.clearTimeout(id);
  }, [q, urlQ, setSearchParams]);

  const query = useQuery({
    // Данные предыдущей страницы держим на экране, пока грузится новая.
    queryKey: ['courses', urlQ, page, LIMIT],
    queryFn: () => listCourses({ q: urlQ || undefined, page, limit: LIMIT }),
    placeholderData: keepPreviousData,
    retry: false,
  });
  const meQuery = useQuery({ queryKey: ['me'], queryFn: me, retry: false });
  const role = meQuery.data?.role;

  // Клиентская сортировка вкладок поверх серверной выдачи страницы.
  const items = useMemo(() => {
    const list = [...(query.data?.items ?? [])];
    if (seg === 'often') list.sort((a, b) => (b.ratingsCount ?? 0) - (a.ratingsCount ?? 0));
    if (seg === 'rating') list.sort((a, b) => b.avgRating - a.avgRating);
    if (seg === 'price') list.sort((a, b) => a.priceCents - b.priceCents);
    return list;
  }, [query.data, seg]);

  const data = query.data;
  const total = data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / LIMIT));

  // Страница могла выйти за пределы выдачи (поиск уменьшил total) — вернуться на последнюю.
  useEffect(() => {
    if (data && page > totalPages) setPage(totalPages);
  }, [data, page, totalPages]);

  const avgRating =
    items.length > 0 ? items.reduce((n, c) => n + c.avgRating, 0) / items.length : 0;
  const totalReviews = items.reduce((n, c) => n + (c.ratingsCount ?? 0), 0);

  const isSearching = urlQ.trim().length > 0;
  const isEmpty = Boolean(data) && items.length === 0;
  // «Показано N из M»: сколько курсов пользователь уже увидел листанием.
  const shown = data ? Math.min(total, (page - 1) * LIMIT + items.length) : 0;

  /** Применить текущий текст поля сразу (по кнопке «Найти» или пилюле), без дебаунса. */
  function applySearch(value: string): void {
    const next = value.trim();
    setSearchParams(
      (prev) => {
        const p = new URLSearchParams(prev);
        if (next) p.set('q', next);
        else p.delete('q');
        return p;
      },
      { replace: true },
    );
    setPage(1);
  }

  function resetSearch(): void {
    setQ('');
    setActivePill('Популярное');
    applySearch('');
  }

  function onPill(pill: { label: string; q: string }): void {
    setActivePill(pill.label);
    setSeg('often');
    setQ(pill.q);
    applySearch(pill.q);
  }

  function scrollToCards(): void {
    cardsRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }

  return (
    <div>
      <section className="hero">
        <div className="hero-card">
          <span className="eyebrow">Маркетплейс IT-курсов · для новичков</span>
          <h1>Понятные IT-курсы с нуля — с практикой и наставником</h1>
          <p className="lead">
            Выберите трек: Python, веб, данные или дизайн. Все программы — с живыми разборами,
            квизами и кодовыми задачами. Оплата в рублях, возврат в первые 7 дней.
          </p>
          <div className="hero-cta">
            <button type="button" className="btn btn-primary" onClick={scrollToCards}>
              Выбрать курс
            </button>
            <Link className="btn btn-ghost" to="/me">
              Мои курсы
            </Link>
          </div>
          <div className="stats">
            <div>
              <b>{total}</b>
              <span>курсов в каталоге</span>
            </div>
            <div>
              <b>{avgRating > 0 ? avgRating.toFixed(1).replace('.', ',') : '—'}</b>
              <span>средняя оценка</span>
            </div>
            <div>
              <b>{totalReviews}</b>
              <span>отзывов студентов</span>
            </div>
          </div>
          <div className="cats">
            {PILLS.map((p) => (
              <button
                key={p.label}
                type="button"
                className={`cat${activePill === p.label ? ' active' : ''}`}
                onClick={() => onPill(p)}
              >
                {p.label}
              </button>
            ))}
            <a
              className="cat dark"
              href="#lead"
              onClick={(e) => {
                e.preventDefault();
                document.getElementById('lead')?.scrollIntoView({ behavior: 'smooth' });
              }}
            >
              Помогите выбрать
            </a>
          </div>
        </div>
      </section>

      <div className="wrap">
        <div className="mini-head">
          <h2>
            Курсы для карьеры
            <br />и жизни
          </h2>
          <div className="seg" role="tablist">
            {SEGS.map((s) => (
              <button
                key={s.key}
                type="button"
                role="tab"
                aria-selected={seg === s.key}
                className={seg === s.key ? 'active' : ''}
                onClick={() => setSeg(s.key)}
              >
                {s.label}
              </button>
            ))}
          </div>
          <form
            role="search"
            aria-label={t('a11y.searchLabel')}
            className="search-row"
            onSubmit={(e) => {
              e.preventDefault();
              applySearch(q);
            }}
          >
            <div className="search-wrap">
              <svg width="16" height="16" viewBox="0 0 24 24" aria-hidden="true">
                <circle cx="11" cy="11" r="7" fill="none" stroke="currentColor" strokeWidth="2" />
                <path
                  d="M16.5 16.5 21 21"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                />
              </svg>
              <label htmlFor="catalog-q" className="sr-only">
                {t('a11y.searchLabel')}
              </label>
              <input
                id="catalog-q"
                className="input"
                value={q}
                onChange={(e) => setQ(e.target.value)}
                placeholder={t('catalog.searchPlaceholder')}
              />
            </div>
            <button type="submit" className="btn btn-sm">
              {t('catalog.searchButton')}
            </button>
          </form>
        </div>

        <div className="count-line" ref={cardsRef}>
          <h2>{t('catalog.title')}</h2>
          <span>
            Найдено: {items.length} из {total}
          </span>
        </div>

        {query.isLoading && (
          <>
            <p role="status">{t('catalog.loading')}</p>
            <div className="catalog-skeletons" aria-hidden="true">
              {Array.from({ length: SKELETONS }, (_, i) => (
                <div key={i} className="catalog-skeleton" />
              ))}
            </div>
          </>
        )}

        {query.isError && (
          <>
            <p role="alert">{t('catalog.error')}</p>
            <button type="button" className="btn" onClick={() => void query.refetch()}>
              {t('error.retry')}
            </button>
          </>
        )}

        {isEmpty && (
          <div className="empty">
            <h3>{isSearching ? 'Курсов пока нет' : t('catalog.emptyCatalogTitle')}</h3>
            <p style={{ margin: '0 0 14px' }}>
              {isSearching
                ? 'Попробуйте изменить запрос или сбросить фильтры — подберём что-нибудь подходящее.'
                : t('catalog.emptyCatalogHint')}
            </p>
            {isSearching ? (
              <button type="button" className="btn" onClick={resetSearch}>
                Сбросить фильтры
              </button>
            ) : (
              <>
                {(role === 'teacher' || role === 'admin') && (
                  <p>
                    <Link className="btn" to="/teach/new">
                      {t('teach.create')}
                    </Link>
                  </p>
                )}
                {!meQuery.data && (
                  <p>
                    <Link className="btn" to="/login">
                      {t('nav.login')}
                    </Link>
                  </p>
                )}
              </>
            )}
          </div>
        )}

        <div className="grid">
          {items.map((c) => (
            <CourseCard key={c.id} course={c} />
          ))}
        </div>

        {data && total > LIMIT && (
          <nav className="catalog-pager" aria-label={t('catalog.title')}>
            <button
              type="button"
              className="btn btn-sm"
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={page <= 1}
            >
              {t('common.prev')}
            </button>
            <span className="muted" aria-live="polite">
              {t('admin.showing')} {shown} {t('admin.of')} {total}
            </span>
            <button
              type="button"
              className="btn btn-sm"
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              disabled={page >= totalPages}
            >
              {t('common.next')}
            </button>
          </nav>
        )}

        <SupportBlock />
        <PerksBlock />
        <DreamsBlock />
        <DiplomaBlock />
        <FinalCta onChoose={scrollToCards} />
      </div>
      <LeadBlock />
    </div>
  );
}
