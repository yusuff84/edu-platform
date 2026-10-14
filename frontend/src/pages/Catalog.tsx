import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { listCourses, me } from '../api/client';
import CourseCard from '../components/CourseCard';
import { t } from '../i18n/t';
import '../styles/catalog.css';

/** Размер страницы каталога (контракт: limit по умолч. 20, максимум 100). */
const LIMIT = 12;
/** Сколько скелетонов показывать во время загрузки. */
const SKELETONS = 6;
/** Задержка поиска, мс. */
const DEBOUNCE_MS = 300;

export default function Catalog(): JSX.Element {
  const [searchParams, setSearchParams] = useSearchParams();
  // Поиск живёт в URL (?q=) — ссылку можно передать, «назад/вперёд» работает.
  const urlQ = searchParams.get('q') ?? '';

  // Локальное значение поля ввода; в URL попадает после дебаунса.
  const [q, setQ] = useState(urlQ);
  const [page, setPage] = useState(1);

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

  const data = query.data;
  const items = data?.items ?? [];
  const total = data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / LIMIT));

  // Страница могла выйти за пределы выдачи (поиск уменьшил total) — вернуться на последнюю.
  useEffect(() => {
    if (data && page > totalPages) setPage(totalPages);
  }, [data, page, totalPages]);

  const isSearching = urlQ.trim().length > 0;
  const isEmpty = Boolean(data) && items.length === 0;
  // «Показано N из M»: сколько курсов пользователь уже увидел листанием.
  const shown = data ? Math.min(total, (page - 1) * LIMIT + items.length) : 0;

  /** Применить текущий текст поля сразу (по кнопке «Найти»), без дебаунса. */
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
    applySearch('');
  }

  return (
    <main>
      <h1>{t('catalog.title')}</h1>
      <form
        className="catalog-search"
        role="search"
        aria-label={t('a11y.searchLabel')}
        onSubmit={(e) => {
          e.preventDefault();
          applySearch(q);
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
          <button type="button" onClick={() => void query.refetch()}>
            {t('error.retry')}
          </button>
        </>
      )}

      {isEmpty && isSearching && (
        <section aria-label={t('catalog.title')}>
          <p>{t('catalog.empty')}</p>
          <button type="button" onClick={resetSearch}>
            {t('catalog.emptySearchReset')}
          </button>
        </section>
      )}

      {isEmpty && !isSearching && !query.isError && (
        <section aria-label={t('catalog.emptyCatalogTitle')}>
          <h2>{t('catalog.emptyCatalogTitle')}</h2>
          <p>{t('catalog.emptyCatalogHint')}</p>
          {(meQuery.data?.role === 'teacher' || meQuery.data?.role === 'admin') && (
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
        {items.map((c) => (
          <CourseCard key={c.id} course={c} />
        ))}
      </div>

      {data && total > LIMIT && (
        <nav className="catalog-pager" aria-label={t('catalog.title')}>
          <button
            type="button"
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
            onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
            disabled={page >= totalPages}
          >
            {t('common.next')}
          </button>
        </nav>
      )}
    </main>
  );
}
