import { Link } from 'react-router-dom';
import type { CourseItem } from '../api/client';
import { formatPrice } from '../api/client';
import { t } from '../i18n/t';
import '../styles/catalog.css';

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
 */
export default function CourseCard({ course }: { course: CourseItem }): JSX.Element {
  const desc =
    course.description.length > 140
      ? `${course.description.slice(0, 140)}…`
      : course.description;
  return (
    <article className="card">
      <Link to={`/courses/${course.slug}`} aria-label={course.title}>
        <h3>{course.title}</h3>
      </Link>
      <p className="muted">{desc}</p>
      <div className="card-row">
        <Stars value={course.avgRating ?? 0} />
        {course.ratingsCount > 0 && (
          <span className="muted">
            {course.ratingsCount} {t('reviews.title').toLowerCase()}
          </span>
        )}
      </div>
      <div className="card-row">
        <strong>{formatPrice(course.priceCents)}</strong>
      </div>
    </article>
  );
}
