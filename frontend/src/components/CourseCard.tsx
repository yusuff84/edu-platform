import type { MouseEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { CourseItem } from '../api/client';
import {
  ApiError,
  addFavorite,
  formatPrice,
  myFavorites,
  removeFavorite,
} from '../api/client';
import { coverUrl } from '../api/teacherCover';
import { meQueryOptions } from './RouteGuards';
import { t } from '../i18n/t';
import type { RuKey } from '../i18n/t';
import { pluralRu } from '../i18n/plural';
import '../styles/catalog.css';
import '../styles/favorites.css';

const COVERS = ['cover-py', 'cover-html', 'cover-js', 'cover-sql', 'cover-fig', 'cover-linux'];

/** T66: подписи категорий курса (контракт T65, ключи p8.cat.*). */
const CATEGORY_LABEL: Record<string, RuKey> = {
  programming: 'p8.cat.programming',
  design: 'p8.cat.design',
  data: 'p8.cat.data',
  marketing: 'p8.cat.marketing',
  ai: 'p8.cat.ai',
  other: 'p8.cat.other',
};

/** Название категории по слагу; неизвестный слаг показываем как есть. */
function categoryLabel(slug: string): string {
  const key = CATEGORY_LABEL[slug];
  return key ? t(key) : slug;
}

export function coverFor(key: string): string {
  let h = 0;
  for (let i = 0; i < key.length; i += 1) h = (h * 31 + key.charCodeAt(i)) >>> 0;
  return COVERS[h % COVERS.length];
}

/** Звёзды рейтинга: дробное значение округляется, 0 → «пока без оценок». */
export function Stars({ value }: { value: number }): JSX.Element {
  const filled = Math.max(0, Math.min(5, Math.round(value)));
  return (
    <span role="img" aria-label={`${t('a11y.ratingLabel')}: ${value.toFixed(1)}`}>
      {'★'.repeat(filled)}
      {'☆'.repeat(5 - filled)}
      {' '}
      <span>{value > 0 ? value.toFixed(1) : t('catalog.ratingNo')}</span>
    </span>
  );
}

/**
 * Карточка курса для каталога.
 * Рендерим только поля CourseItem: автора, обложку и статус бэкенд
 * в ответе каталога не отдаёт — пустышки не показываем.
 * Оформление — Skillbox-style плитка (cover/tag/tile-title из styles.css),
 * обложка детерминирована от slug/id через coverFor.
 */
export default function CourseCard({ course }: { course: CourseItem }): JSX.Element {
  const navigate = useNavigate();
  const qc = useQueryClient();

  // Сессия — общий ['me'] (Layout/RouteGuards): гостю сердечко только ведёт
  // на вход, запрос избранного для него не отправляем.
  const meQuery = useQuery(meQueryOptions);
  const user = meQuery.data;

  // Признака isFavorite в CourseItem нет (контракт T73): состояние берём из
  // общего списка ['favorites']. React Query дедуплицирует запрос по ключу,
  // поэтому все карточки страницы используют один ответ.
  const favoritesQuery = useQuery({
    queryKey: ['favorites'],
    queryFn: myFavorites,
    enabled: Boolean(user),
    retry: false,
    staleTime: 60_000,
  });
  const favorite = Boolean(favoritesQuery.data?.some((c) => c.id === course.id));

  // Toggle: оптимистично правим кэш ['favorites'] (сердечко откликается
  // сразу), при ошибке откатываем, после ответа инвалидируем — список и
  // состояние сходятся с сервером.
  const toggleFavorite = useMutation({
    mutationFn: () =>
      favorite ? removeFavorite(course.id) : addFavorite(course.id),
    onMutate: async () => {
      await qc.cancelQueries({ queryKey: ['favorites'] });
      const prev = qc.getQueryData<CourseItem[]>(['favorites']);
      if (prev) {
        qc.setQueryData<CourseItem[]>(
          ['favorites'],
          favorite ? prev.filter((c) => c.id !== course.id) : [course, ...prev],
        );
      } else if (!favorite) {
        qc.setQueryData<CourseItem[]>(['favorites'], [course]);
      }
      return { prev };
    },
    onError: (err, _vars, ctx) => {
      if (ctx?.prev !== undefined) qc.setQueryData(['favorites'], ctx.prev);
      // Сессия истекла — как и гостю, предлагаем войти заново.
      if (err instanceof ApiError && err.status === 401) navigate('/login');
    },
    onSettled: () => qc.invalidateQueries({ queryKey: ['favorites'] }),
  });

  /** Клик по сердечку: не открывает курс и не шлёт запрос гостю. */
  function onFavoriteClick(e: MouseEvent<HTMLButtonElement>): void {
    e.preventDefault();
    e.stopPropagation();
    // Пока сессия определяется, не уводим возможного пользователя на логин.
    if (meQuery.isLoading) return;
    if (!user) {
      navigate('/login');
      return;
    }
    if (!toggleFavorite.isPending) toggleFavorite.mutate();
  }

  const desc =
    course.description.length > 140
      ? `${course.description.slice(0, 140)}…`
      : course.description;
  const meta = [
    course.avgRating > 0
      ? `★ ${course.avgRating.toFixed(1).replace('.', ',')}`
      : null,
    formatPrice(course.priceCents),
  ]
    .filter(Boolean)
    .join(' · ');
  return (
    <article className="course-card">
      {/* T74: кнопка — сосед ссылки, а не вложенный в неё интерактив:
          клик по сердечку не открывает курс (плюс stopPropagation). */}
      <button
        type="button"
        className={`fav-btn${favorite ? ' is-active' : ''}`}
        aria-label={t(favorite ? 'p9.fav.remove' : 'p9.fav.add')}
        aria-pressed={favorite}
        title={user ? undefined : t('p9.fav.loginHint')}
        disabled={toggleFavorite.isPending}
        onClick={onFavoriteClick}
      >
        <svg viewBox="0 0 24 24" width="19" height="19" aria-hidden="true">
          <path
            d="M12 20.6 4.9 15.6C2 13.5 1.5 9.5 3.6 6.9a5 5 0 0 1 7.5-.3l.9.9.9-.9a5 5 0 0 1 7.5.3c2.1 2.6 1.6 6.6-1.3 8.7Z"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.7"
            strokeLinejoin="round"
          />
        </svg>
      </button>
      {/* Без aria-label: доступное имя собирается из видимого текста карточки
          (название, рейтинг, цена), иначе Lighthouse видит несовпадение. */}
      <Link className="tile" to={`/courses/${course.slug}`}>
        {/* T57: есть coverPath (T54) — подложку рисует настоящий кадр; из-за
            градиента-подложки и onError плитка не «ломается», если файл
            недоступен (показывается прежний вид). */}
        <div className={`cover ${coverFor(course.slug || course.id)}`}>
          {course.coverPath && (
            <img
              className="tile-cover-img"
              src={coverUrl(course.coverPath)}
              alt=""
              loading="lazy"
              onError={(e) => {
                e.currentTarget.style.display = 'none';
              }}
            />
          )}
        </div>
        <span className="tag">Курс</span>
        <span className="tile-title">
          {course.title}
          {meta && <small>{meta}</small>}
        </span>
      </Link>
      {course.category && (
        <span className="catalog-category-badge">{categoryLabel(course.category)}</span>
      )}
      <p className="muted">{desc}</p>
      <div className="card-row">
        <Stars value={course.avgRating ?? 0} />
        {course.ratingsCount > 0 && (
          <span className="muted">
            {course.ratingsCount}{' '}
            {pluralRu(
              course.ratingsCount,
              t('plural.review.one'),
              t('plural.review.few'),
              t('plural.review.many'),
            )}
          </span>
        )}
      </div>
      <div className="card-row">
        <strong>{formatPrice(course.priceCents)}</strong>
      </div>
    </article>
  );
}
