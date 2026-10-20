// T74: страница «Избранное» — список сохранённых курсов (/me/favorites).
// Признака isFavorite в CourseItem нет (контракт T73): источник состояния —
// GET /me/favorites. Ключ ['favorites'] общий с сердечками CourseCard, поэтому
// снятие с избранного на этой странице сразу убирает карточку из кэша.
// Доступ — только вошедшим (RequireAuth в router.tsx).

import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { myFavorites } from '../api/client';
import CourseCard from '../components/CourseCard';
import { t } from '../i18n/t';
import { useSeo } from '../app/useSeo';
import '../styles/favorites.css';

/** Сколько скелетонов показывать, пока список грузится. */
const SKELETONS = 6;

export default function Favorites(): JSX.Element {
  useSeo('Избранное — Edu Platform');
  const query = useQuery({
    queryKey: ['favorites'],
    queryFn: myFavorites,
    retry: false,
  });
  const items = query.data ?? [];

  return (
    <div className="wrap">
      <main className="page favorites-page">
        <div className="favorites-head">
          <h1>{t('p9.fav.title')}</h1>
          <Link className="btn btn-sm" to="/">
            {t('p9.fav.goCatalog')}
          </Link>
        </div>

        {query.isLoading && (
          <>
            <p role="status">{t('common.loading')}</p>
            <div className="fav-skeletons" aria-hidden="true">
              {Array.from({ length: SKELETONS }, (_, i) => (
                <div key={i} className="fav-skeleton" />
              ))}
            </div>
          </>
        )}

        {query.isError && (
          <div className="panel">
            <p role="alert" className="error">
              {t('error.network')}
            </p>
            <button type="button" className="btn" onClick={() => void query.refetch()}>
              {t('common.retry')}
            </button>
          </div>
        )}

        {query.isSuccess && items.length === 0 && (
          <div className="empty">
            <h3>{t('p9.fav.empty')}</h3>
            <p>{t('p9.fav.emptyHint')}</p>
            <p>
              <Link className="btn btn-primary" to="/">
                {t('p9.fav.goCatalog')}
              </Link>
            </p>
          </div>
        )}

        {items.length > 0 && (
          <div className="grid">
            {items.map((course) => (
              <CourseCard key={course.id} course={course} />
            ))}
          </div>
        )}
      </main>
    </div>
  );
}
