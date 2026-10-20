import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { getSimilarCourses } from '../api/client';
import { t } from '../i18n/t';
import CourseCard from './CourseCard';
import '../styles/similar.css';

/** Сколько карточек просим у бэкенда (контракт T79: по умолчанию 4, максимум 12). */
const SIMILAR_LIMIT = 4;

/** Число плейсхолдеров скелетона — столько же, сколько карточек в лимите. */
const SKELETON_ITEMS = [0, 1, 2, 3];

/**
 * T80: блок «Похожие курсы» на странице курса.
 *
 * Поведение по постановке:
 *  - запрос react-query по courseId с limit=4;
 *  - пустой список и ошибку не показываем — блок просто не рендерится;
 *  - во время загрузки — скелетон в стиле соседних блоков;
 *  - ссылка «Все курсы категории» ведёт в каталог с фильтром category.
 *
 * API переиспользуется как есть: CourseCard не меняется.
 */
export default function SimilarCourses({
  courseId,
  category,
}: {
  courseId: string;
  /** Слаг категории текущего курса; без него ссылка в каталог не нужна. */
  category?: string;
}): JSX.Element | null {
  const similarQuery = useQuery({
    queryKey: ['similar-courses', courseId],
    queryFn: () => getSimilarCourses(courseId, SIMILAR_LIMIT),
    enabled: Boolean(courseId),
    retry: false,
  });

  // Категория может прийти пустой (старые курсы) — ссылку тогда не рисуем.
  const catalogHref = category
    ? `/?category=${encodeURIComponent(category)}`
    : undefined;

  if (similarQuery.isLoading) {
    return (
      <section className="panel similar-courses" aria-busy="true">
        <div className="similar-courses-head">
          <h2>{t('p9.similar.title')}</h2>
          {catalogHref && (
            <Link className="similar-courses-all" to={catalogHref}>
              {t('p9.similar.all')}
            </Link>
          )}
        </div>
        <div className="grid similar-grid" aria-hidden="true">
          {SKELETON_ITEMS.map((n) => (
            <div key={n} className="similar-skeleton" />
          ))}
        </div>
        <span className="sr-only" role="status">
          {t('common.loading')}
        </span>
      </section>
    );
  }

  // Ошибку не показываем: блок скрыт целиком, страница курса остаётся рабочей.
  if (similarQuery.isError || !similarQuery.data || similarQuery.data.length === 0) {
    return null;
  }

  return (
    <section className="panel similar-courses" aria-labelledby="similar-courses-title">
      <div className="similar-courses-head">
        <h2 id="similar-courses-title">{t('p9.similar.title')}</h2>
        {catalogHref && (
          <Link className="similar-courses-all" to={catalogHref}>
            {t('p9.similar.all')}
          </Link>
        )}
      </div>
      <div className="grid similar-grid">
        {similarQuery.data.map((course) => (
          <CourseCard key={course.id} course={course} />
        ))}
      </div>
    </section>
  );
}
