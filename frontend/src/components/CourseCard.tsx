import { Link } from 'react-router-dom';
import type { Course } from '../api/client';
import { formatPrice } from '../api/client';
import { t } from '../i18n/t';

export function Stars({ value }: { value: number }): JSX.Element {
  const filled = Math.round(value);
  return (
    <span role="img" aria-label={`${t('a11y.ratingLabel')}: ${value.toFixed(1)}`}>
      {'★'.repeat(Math.max(0, Math.min(5, filled)))}
      {'☆'.repeat(5 - Math.max(0, Math.min(5, filled)))}
      {' '}
      <span>{value > 0 ? value.toFixed(1) : t('catalog.ratingNo')}</span>
    </span>
  );
}

export default function CourseCard({ course }: { course: Course }): JSX.Element {
  return (
    <article className="card">
      <Link to={`/courses/${course.slug}`} aria-label={course.title}>
        <h3>{course.title}</h3>
      </Link>
      <p className="muted">{course.description.slice(0, 140)}</p>
      <div className="card-row">
        <Stars value={course.avgRating ?? 0} />
      </div>
      <div className="card-row">
        <strong>{formatPrice(course.priceCents)}</strong>
        {course.teacherName && (
          <span className="muted">
            {t('course.teacher')}: {course.teacherName}
          </span>
        )}
      </div>
    </article>
  );
}
