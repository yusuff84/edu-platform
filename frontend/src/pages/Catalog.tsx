import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { listCourses, me } from '../api/client';
import CourseCard from '../components/CourseCard';
import { t } from '../i18n/t';

export default function Catalog(): JSX.Element {
  const [q, setQ] = useState('');
  const [submitted, setSubmitted] = useState('');
  const query = useQuery({
    queryKey: ['courses', submitted],
    queryFn: () => listCourses({ q: submitted || undefined, page: 1, limit: 20 }),
  });
  const meQuery = useQuery({ queryKey: ['me'], queryFn: me, retry: false });
  const role = meQuery.data?.role;
  const isSearching = submitted.trim().length > 0;
  const isEmpty = Boolean(query.data) && (query.data?.items.length ?? 0) === 0;

  return (
    <main>
      <h1>{t('catalog.title')}</h1>
      <form
        role="search"
        aria-label={t('a11y.searchLabel')}
        onSubmit={(e) => {
          e.preventDefault();
          setSubmitted(q.trim());
        }}
      >
        <label htmlFor="catalog-q" className="sr-only">
          {t('a11y.searchLabel')}
        </label>
        <input
          id="catalog-q"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder={t('catalog.searchPlaceholder')}
        />
        <button type="submit">{t('catalog.searchButton')}</button>
      </form>

      {query.isLoading && <p>{t('catalog.loading')}</p>}
      {query.isError && <p>{t('catalog.error')}</p>}

      {isEmpty && isSearching && (
        <section aria-label={t('catalog.title')}>
          <p>{t('catalog.empty')}</p>
          <button
            type="button"
            onClick={() => {
              setQ('');
              setSubmitted('');
            }}
          >
            {t('catalog.emptySearchReset')}
          </button>
        </section>
      )}

      {isEmpty && !isSearching && !query.isError && (
        <section aria-label={t('catalog.emptyCatalogTitle')}>
          <h2>{t('catalog.emptyCatalogTitle')}</h2>
          <p>{t('catalog.emptyCatalogHint')}</p>
          {(role === 'teacher' || role === 'admin') && (
            <p>
              <Link to="/teach/new">{t('teach.create')}</Link>
            </p>
          )}
          {!meQuery.data && (
            <p>
              <Link to="/login">{t('nav.login')}</Link>
            </p>
          )}
        </section>
      )}

      <div className="grid">
        {(query.data?.items ?? []).map((c) => (
          <CourseCard key={c.id} course={c} />
        ))}
      </div>
    </main>
  );
}
