import { useEffect, useRef, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { getCategories, listCourses, me } from '../api/client';
import type {
  CategoryStat,
  CourseMinRatingFilter,
  CoursePriceFilter,
  CourseSort,
} from '../api/client';
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
import type { RuKey } from '../i18n/t';
import { useSeo } from '../app/useSeo';
import '../styles/catalog.css';
import '../styles/catalog-filters.css';

/** Размер страницы каталога (контракт: limit по умолч. 20, максимум 100). */
const LIMIT = 12;
/** Сколько скелетонов показывать во время загрузки. */
const SKELETONS = 6;
/** Задержка поиска, мс. */
const DEBOUNCE_MS = 300;

// ---------- T66: параметры фильтров каталога ----------

/** Фиксированный список категорий (контракт T65). */
const CATEGORY_SLUGS = ['programming', 'design', 'data', 'marketing', 'ai', 'other'] as const;
type CategorySlug = (typeof CATEGORY_SLUGS)[number];

/** Подписи категорий — только готовые ключи p8.cat.*. */
const CATEGORY_LABEL: Record<CategorySlug, RuKey> = {
  programming: 'p8.cat.programming',
  design: 'p8.cat.design',
  data: 'p8.cat.data',
  marketing: 'p8.cat.marketing',
  ai: 'p8.cat.ai',
  other: 'p8.cat.other',
};

const PRICE_FILTERS: CoursePriceFilter[] = ['free', 'paid'];

const PRICE_LABEL: Record<CoursePriceFilter, RuKey> = {
  free: 'p8.catalog.priceFree',
  paid: 'p8.catalog.pricePaid',
};

const RATING_FILTERS: CourseMinRatingFilter[] = ['3.5', '4', '4.5'];

const RATING_LABEL: Record<CourseMinRatingFilter, RuKey> = {
  '3.5': 'p8.catalog.rating35',
  '4': 'p8.catalog.rating40',
  '4.5': 'p8.catalog.rating45',
};

const SORT_OPTIONS: CourseSort[] = ['popular', 'new', 'rating', 'price_asc', 'price_desc'];

const SORT_LABEL: Record<CourseSort, RuKey> = {
  popular: 'p8.catalog.sortPopular',
  new: 'p8.catalog.sortNew',
  rating: 'p8.catalog.sortRating',
  price_asc: 'p8.catalog.sortPriceAsc',
  price_desc: 'p8.catalog.sortPriceDesc',
};

/** Сортировка по умолчанию — как на backend; в URL её не дублируем. */
const DEFAULT_SORT: CourseSort = 'popular';

/** Параметры фильтров, которые пишутся в URL. */
type FilterParam = 'category' | 'price' | 'minRating' | 'sort';

/** Значение из фиксированного списка или undefined (неизвестное не применяем). */
function readOneOf<T extends string>(
  value: string | null,
  allowed: readonly T[],
): T | undefined {
  return value !== null && (allowed as readonly string[]).includes(value)
    ? (value as T)
    : undefined;
}

/** Номер страницы из URL: целое ≥ 1, иначе 1. */
function readPage(value: string | null): number {
  const n = Number(value);
  return Number.isInteger(n) && n >= 1 ? n : 1;
}

/** Название категории по слагу; неизвестный слаг показываем как есть. */
function categoryLabel(slug: string): string {
  const key = CATEGORY_LABEL[slug as CategorySlug];
  return key ? t(key) : slug;
}

export default function Catalog(): JSX.Element {
  // Каталог — точка входа: возвращаем базовые title/description после карточек.
  useSeo();
  const [searchParams, setSearchParams] = useSearchParams();

  // Единственный источник истины для активных фильтров — URL (ссылка шарится,
  // «назад/вперёд» восстанавливают состояние). Локально живёт только черновик
  // строки поиска (до debounce) и свёрнутость панели на узких экранах.
  const urlQ = searchParams.get('q') ?? '';
  const category = readOneOf(searchParams.get('category'), CATEGORY_SLUGS);
  const price = readOneOf(searchParams.get('price'), PRICE_FILTERS);
  const minRating = readOneOf(searchParams.get('minRating'), RATING_FILTERS);
  const sort = readOneOf(searchParams.get('sort'), SORT_OPTIONS) ?? DEFAULT_SORT;
  const page = readPage(searchParams.get('page'));

  const [q, setQ] = useState(urlQ);
  const [filtersOpen, setFiltersOpen] = useState(false);
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
          p.delete('page');
          return p;
        },
        { replace: true },
      );
    }, DEBOUNCE_MS);
    return () => window.clearTimeout(id);
  }, [q, urlQ, setSearchParams]);

  const query = useQuery({
    // Данные предыдущей страницы держим на экране, пока грузится новая.
    queryKey: [
      'courses',
      {
        q: urlQ,
        category: category ?? '',
        price: price ?? '',
        minRating: minRating ?? '',
        sort,
        page,
        limit: LIMIT,
      },
    ],
    queryFn: () =>
      listCourses({
        q: urlQ || undefined,
        category: category || undefined,
        price,
        minRating,
        // popular — серверный дефолт: не засоряем запрос и URL.
        sort: sort === DEFAULT_SORT ? undefined : sort,
        page,
        limit: LIMIT,
      }),
    placeholderData: keepPreviousData,
    retry: false,
  });
  const meQuery = useQuery({ queryKey: ['me'], queryFn: me, retry: false });
  const role = meQuery.data?.role;

  // Категории и счётчики курсов — с сервера; пока их нет (или запрос упал),
  // показываем фиксированный список без счётчиков, чтобы фильтр работал.
  const categoriesQuery = useQuery({
    queryKey: ['categories'],
    queryFn: getCategories,
    retry: false,
  });
  const categories: CategoryStat[] =
    categoriesQuery.data && categoriesQuery.data.length > 0
      ? categoriesQuery.data
      : CATEGORY_SLUGS.map((slug) => ({ slug, coursesCount: 0 }));

  const data = query.data;
  const items = data?.items ?? [];
  const total = data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / LIMIT));

  // Страница могла выйти за пределы выдачи (фильтры уменьшили total) — вернуться на последнюю.
  useEffect(() => {
    if (!data || page <= totalPages) return;
    setSearchParams(
      (prev) => {
        const p = new URLSearchParams(prev);
        if (totalPages > 1) p.set('page', String(totalPages));
        else p.delete('page');
        return p;
      },
      { replace: true },
    );
  }, [data, page, totalPages, setSearchParams]);

  const avgRating =
    items.length > 0 ? items.reduce((n, c) => n + c.avgRating, 0) / items.length : 0;
  const totalReviews = items.reduce((n, c) => n + (c.ratingsCount ?? 0), 0);

  const filtersActive = Boolean(
    urlQ.trim() || category || price || minRating || sort !== DEFAULT_SORT,
  );
  const isEmpty = Boolean(data) && items.length === 0;
  // «Показано N из M»: сколько курсов пользователь уже увидел листанием.
  const shown = data ? Math.min(total, (page - 1) * LIMIT + items.length) : 0;

  /** Точечная запись фильтра в URL; пустое значение удаляет параметр. */
  function setFilter(name: FilterParam, value: string): void {
    setSearchParams((prev) => {
      const p = new URLSearchParams(prev);
      if (value) p.set(name, value);
      else p.delete(name);
      // Любое изменение фильтра — с первой страницы.
      p.delete('page');
      return p;
    });
  }

  /** Применить текущий текст поля сразу (по кнопке «Найти»), без дебаунса. */
  function applySearch(value: string): void {
    const next = value.trim();
    setQ(next);
    if (next === urlQ) return;
    setSearchParams((prev) => {
      const p = new URLSearchParams(prev);
      if (next) p.set('q', next);
      else p.delete('q');
      p.delete('page');
      return p;
    });
  }

  /** Сбросить все фильтры и поиск, вернуться на первую страницу. */
  function resetFilters(): void {
    setQ('');
    setSearchParams((prev) => {
      const p = new URLSearchParams(prev);
      for (const key of ['q', 'category', 'price', 'minRating', 'sort', 'page']) {
        p.delete(key);
      }
      return p;
    });
  }

  /** Переход по страницам — тоже через URL (page=1 не дублируем). */
  function goToPage(next: number): void {
    setSearchParams((prev) => {
      const p = new URLSearchParams(prev);
      if (next > 1) p.set('page', String(next));
      else p.delete('page');
      return p;
    });
  }

  function scrollToCards(): void {
    cardsRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }

  return (
    <main className="page catalog-page">
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
            {/* «Мои курсы» — только вошедшим: /me за гвардом RequireAuth. */}
            {meQuery.data && (
              <Link className="btn btn-ghost" to="/me">
                Мои курсы
              </Link>
            )}
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

        {/* Узкие экраны: панель фильтров сворачивается этим тогглом. */}
        <button
          type="button"
          className="btn btn-sm catalog-filters-toggle"
          aria-expanded={filtersOpen}
          aria-controls="catalog-filters"
          onClick={() => setFiltersOpen((open) => !open)}
        >
          {t(filtersOpen ? 'p8.catalog.filtersHide' : 'p8.catalog.filtersShow')}
        </button>

        <section
          id="catalog-filters"
          className={`panel catalog-filters${filtersOpen ? ' is-open' : ''}`}
          aria-label={t('p8.catalog.filters')}
        >
          <div className="catalog-filters__grid">
            <div className="catalog-filters__group catalog-filters__group--categories">
              <span className="catalog-filters__label" id="catalog-category-label">
                {t('p8.catalog.category')}
              </span>
              <div
                className="catalog-chips"
                role="group"
                aria-labelledby="catalog-category-label"
              >
                <button
                  type="button"
                  className={`catalog-chip${category ? '' : ' is-active'}`}
                  aria-pressed={!category}
                  onClick={() => setFilter('category', '')}
                >
                  {t('p8.catalog.allCategories')}
                </button>
                {categories.map((c) => {
                  const active = category === c.slug;
                  return (
                    <button
                      key={c.slug}
                      type="button"
                      className={`catalog-chip${active ? ' is-active' : ''}`}
                      aria-pressed={active}
                      onClick={() => setFilter('category', c.slug)}
                    >
                      {categoryLabel(c.slug)}
                      {c.coursesCount > 0 && (
                        <span className="catalog-chip__count">{c.coursesCount}</span>
                      )}
                    </button>
                  );
                })}
              </div>
            </div>

            <div className="catalog-filters__group">
              <span className="catalog-filters__label" id="catalog-price-label">
                {t('p8.catalog.price')}
              </span>
              <div className="catalog-chips" role="group" aria-labelledby="catalog-price-label">
                <button
                  type="button"
                  className={`catalog-chip${price ? '' : ' is-active'}`}
                  aria-pressed={!price}
                  onClick={() => setFilter('price', '')}
                >
                  {t('p8.catalog.priceAny')}
                </button>
                {PRICE_FILTERS.map((value) => {
                  const active = price === value;
                  return (
                    <button
                      key={value}
                      type="button"
                      className={`catalog-chip${active ? ' is-active' : ''}`}
                      aria-pressed={active}
                      onClick={() => setFilter('price', value)}
                    >
                      {t(PRICE_LABEL[value])}
                    </button>
                  );
                })}
              </div>
            </div>

            <div className="catalog-filters__group">
              <span className="catalog-filters__label" id="catalog-rating-label">
                {t('p8.catalog.rating')}
              </span>
              <div
                className="catalog-chips"
                role="group"
                aria-labelledby="catalog-rating-label"
              >
                <button
                  type="button"
                  className={`catalog-chip${minRating ? '' : ' is-active'}`}
                  aria-pressed={!minRating}
                  onClick={() => setFilter('minRating', '')}
                >
                  {t('p8.catalog.ratingAny')}
                </button>
                {RATING_FILTERS.map((value) => {
                  const active = minRating === value;
                  return (
                    <button
                      key={value}
                      type="button"
                      className={`catalog-chip${active ? ' is-active' : ''}`}
                      aria-pressed={active}
                      onClick={() => setFilter('minRating', value)}
                    >
                      {t(RATING_LABEL[value])}
                    </button>
                  );
                })}
              </div>
            </div>

            <label className="catalog-filters__group catalog-filters__sort" htmlFor="catalog-sort">
              <span className="catalog-filters__label">{t('p8.catalog.sort')}</span>
              <select
                id="catalog-sort"
                className="select"
                value={sort}
                onChange={(e) =>
                  setFilter('sort', e.target.value === DEFAULT_SORT ? '' : e.target.value)
                }
              >
                {SORT_OPTIONS.map((value) => (
                  <option key={value} value={value}>
                    {t(SORT_LABEL[value])}
                  </option>
                ))}
              </select>
            </label>

            <div className="catalog-filters__actions">
              <button
                type="button"
                className="btn btn-sm"
                onClick={resetFilters}
                disabled={!filtersActive && page === 1}
              >
                {t('p8.catalog.reset')}
              </button>
            </div>
          </div>
        </section>

        <div className="count-line" ref={cardsRef}>
          <h2>{t('catalog.title')}</h2>
          <span aria-live="polite">
            {t('p8.catalog.found')}: {total}
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
            <h3>
              {filtersActive ? t('p8.catalog.nothingFound') : t('catalog.emptyCatalogTitle')}
            </h3>
            <p style={{ margin: '0 0 14px' }}>
              {filtersActive
                ? t('p8.catalog.nothingFoundHint')
                : t('catalog.emptyCatalogHint')}
            </p>
            {filtersActive ? (
              <button type="button" className="btn" onClick={resetFilters}>
                {t('p8.catalog.reset')}
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
              onClick={() => goToPage(page - 1)}
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
              onClick={() => goToPage(page + 1)}
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
    </main>
  );
}
