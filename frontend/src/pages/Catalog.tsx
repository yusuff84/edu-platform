import { useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
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
  const [q, setQ] = useState('');
  const [submitted, setSubmitted] = useState('');
  const [seg, setSeg] = useState<Seg>('often');
  const [activePill, setActivePill] = useState('Популярное');
  const cardsRef = useRef<HTMLDivElement>(null);
  const query = useQuery({
    queryKey: ['courses', submitted],
    queryFn: () => listCourses({ q: submitted || undefined, page: 1, limit: 20 }),
  });
  const meQuery = useQuery({ queryKey: ['me'], queryFn: me, retry: false });
  const role = meQuery.data?.role;

  const items = useMemo(() => {
    const list = [...(query.data?.items ?? [])];
    if (seg === 'often') list.sort((a, b) => (b.reviewsCount ?? 0) - (a.reviewsCount ?? 0));
    if (seg === 'rating') list.sort((a, b) => b.avgRating - a.avgRating);
    if (seg === 'price') list.sort((a, b) => a.priceCents - b.priceCents);
    return list;
  }, [query.data, seg]);

  const total = query.data?.total ?? 0;
  const avgRating =
    items.length > 0 ? items.reduce((n, c) => n + c.avgRating, 0) / items.length : 0;
  const totalReviews = items.reduce((n, c) => n + (c.reviewsCount ?? 0), 0);

  const isSearching = submitted.trim().length > 0;
  const isEmpty = Boolean(query.data) && items.length === 0;

  function applySearch(term: string): void {
    setQ(term);
    setSubmitted(term.trim());
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
                onClick={() => {
                  setActivePill(p.label);
                  setSeg('often');
                  applySearch(p.q);
                }}
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
              setSubmitted(q.trim());
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

        {query.isLoading && <p>{t('catalog.loading')}</p>}
        {query.isError && <p>{t('catalog.error')}</p>}

        {isEmpty && (
          <div className="empty">
            <h3>{isSearching ? 'Курсов пока нет' : t('catalog.emptyCatalogTitle')}</h3>
            <p style={{ margin: '0 0 14px' }}>
              {isSearching
                ? 'Попробуйте изменить запрос или сбросить фильтры — подберём что-нибудь подходящее.'
                : t('catalog.emptyCatalogHint')}
            </p>
            {isSearching ? (
              <button
                type="button"
                className="btn"
                onClick={() => {
                  setActivePill('Популярное');
                  applySearch('');
                }}
              >
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
