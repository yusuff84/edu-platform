import { Link } from 'react-router-dom';
import type { Course } from '../api/client';
import { formatPrice } from '../api/client';
import { t } from '../i18n/t';

const COVERS = ['cover-py', 'cover-html', 'cover-js', 'cover-sql', 'cover-fig', 'cover-linux'];

export function coverFor(key: string): string {
  let h = 0;
  for (let i = 0; i < key.length; i += 1) h = (h * 31 + key.charCodeAt(i)) >>> 0;
  return COVERS[h % COVERS.length];
}

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
  const lessons =
    course.lessonsCount ?? course.modules?.reduce((n, m) => n + m.lessons.length, 0) ?? 0;
  const meta = [
    lessons > 0 ? `${lessons} уроков` : null,
    course.teacherName ?? null,
    course.avgRating > 0
      ? `★ ${course.avgRating.toFixed(1).replace('.', ',')}`
      : formatPrice(course.priceCents),
  ]
    .filter(Boolean)
    .join(' · ');
  return (
    <article>
      <Link className="tile" to={`/courses/${course.slug}`} aria-label={course.title}>
        <div className={`cover ${coverFor(course.slug || course.id)}`} />
        <span className="tag">Курс</span>
        <span className="tile-title">
          {course.title}
          {meta && <small>{meta}</small>}
        </span>
      </Link>
    </article>
  );
}
