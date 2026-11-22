import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { formatPrice, getCourseOutline } from '../api/client';
import type { CourseItem } from '../api/types';
import { coverUrl } from '../api/teacherCover';
import { coverFor } from './CourseCard';

function plural(n: number, one: string, few: string, many: string): string {
  const m10 = n % 10;
  const m100 = n % 100;
  if (m10 === 1 && m100 !== 11) return one;
  if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return few;
  return many;
}

/**
 * «Курс в центре внимания» на главной: обложка, короткое описание, реальные
 * цифры (модули, уроки, тесты) и программа по модулям — посетитель видит,
 * что внутри, ещё до перехода на страницу курса.
 */
export function FeaturedCourse({ course, loggedIn }: { course: CourseItem; loggedIn: boolean }): JSX.Element {
  const detail = useQuery({ queryKey: ['course', course.slug], queryFn: () => getCourseOutline(course.slug), staleTime: 60_000 });
  const modules = detail.data?.modules ?? [];
  const lessons = modules.flatMap((m) => m.lessons);
  const quizzes = lessons.filter((l) => l.kind === 'quiz').length;
  const videos = lessons.filter((l) => l.videoUrl).length;
  const cover = coverUrl(course.coverPath);
  const isFree = course.priceCents === 0;
  const startTo = loggedIn ? `/courses/${course.slug}` : '/register';

  return (
    <section className="featured" aria-labelledby="featured-title">
      <div className="featured-card">
        <div className="featured-media">
          {cover ? (
            <img src={cover} alt="" loading="lazy" />
          ) : (
            <div className={`cover ${coverFor(course.slug)}`} />
          )}
          {isFree && <span className="featured-badge">Бесплатно</span>}
        </div>
        <div className="featured-body">
          <span className="featured-kicker">Курс в центре внимания</span>
          <h2 id="featured-title">{course.title}</h2>
          <p className="featured-desc">{course.description.split('\n')[0]}</p>
          {modules.length > 0 && (
            <ul className="featured-facts" aria-label="Что в курсе">
              <li>
                <b>{modules.length}</b> {plural(modules.length, 'модуль', 'модуля', 'модулей')}
              </li>
              {videos > 0 && (
                <li>
                  <b>{videos}</b> {plural(videos, 'видеоурок', 'видеоурока', 'видеоуроков')}
                </li>
              )}
              <li>
                <b>{quizzes}</b> {plural(quizzes, 'тест', 'теста', 'тестов')}
              </li>
              <li>
                <b>✓</b> сертификат
              </li>
            </ul>
          )}
          <div className="featured-cta">
            <Link
              className="btn btn-primary"
              to={startTo}
              state={loggedIn ? undefined : { from: `/courses/${course.slug}` }}
            >
              {isFree ? 'Начать бесплатно' : `Записаться за ${formatPrice(course.priceCents)}`}
            </Link>
            <Link className="btn" to={`/courses/${course.slug}`}>
              Программа курса
            </Link>
          </div>
        </div>
      </div>

      {modules.length > 0 && (
        <div className="featured-program">
          <h3>Что вы узнаете</h3>
          <ol className="featured-modules">
            {modules.map((m, i) => (
              <li key={m.id}>
                <span className="featured-module-num" aria-hidden="true">
                  {i + 1}
                </span>
                <span className="featured-module-title">{m.title.replace(/^\d+[.\s]+/, '')}</span>
                <span className="featured-module-count">
                  {m.lessons.length} {plural(m.lessons.length, 'шаг', 'шага', 'шагов')}
                </span>
              </li>
            ))}
          </ol>
        </div>
      )}
    </section>
  );
}
