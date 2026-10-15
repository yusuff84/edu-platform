import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { myCourses } from '../api/client';
import { coverFor } from '../components/CourseCard';
import { t } from '../i18n/t';

/**
 * «Мои курсы» студента (первая страница после входа).
 * Форма данных — строго по контракту GET /me/courses:
 * MyCourse = { course: CourseItem, percent, nextLesson: NextLesson | null }.
 * Плоских courseId/slug/title/nextLessonId в ответе НЕТ.
 */
export default function MyCourses(): JSX.Element {
  const query = useQuery({ queryKey: ['my-courses'], queryFn: myCourses });

  // Загрузка
  if (query.isLoading) {
    return (
      <div className="wrap">
        <div className="page">
          <div className="panel">
            <p role="status">{t('my.loading')}</p>
          </div>
        </div>
      </div>
    );
  }

  // Ошибка сети / сервера — можно повторить без перезагрузки страницы
  if (query.isError) {
    return (
      <div className="wrap">
        <div className="page">
          <div className="panel">
            <p role="alert" className="error">
              {t('error.network')}
            </p>
            <button type="button" onClick={() => void query.refetch()}>
              {t('common.retry')}
            </button>
          </div>
        </div>
      </div>
    );
  }

  const items = query.data ?? [];

  // Пустой список
  if (items.length === 0) {
    return (
      <div className="wrap">
        <div className="page">
          <h1>{t('cabinet.studentTitle')}</h1>
          <div className="empty">
            <p>{t('my.empty')}</p>
            <p>
              <Link className="btn btn-primary" to="/">
                {t('my.goCatalog')}
              </Link>
            </p>
          </div>
        </div>
      </div>
    );
  }

  const total = items.length;
  const avg =
    total > 0 ? Math.round(items.reduce((sum, m) => sum + (m.percent ?? 0), 0) / total) : 0;
  const done = items.filter((m) => m.percent >= 100).length;

  return (
    <div className="wrap">
      <div className="page">
        <h1>{t('cabinet.studentTitle')}</h1>
        <div className="cab-stats">
          <div>
            <b>{total}</b>
            <span>Курсов</span>
          </div>
          <div>
            <b>{avg}%</b>
            <span>Средний прогресс</span>
          </div>
          <div>
            <b>{done}</b>
            <span>Завершено</span>
          </div>
        </div>
        <div className="enrolled-grid">
          {items.map((item) => {
            const course = item.course;
            // nextLesson — объект {lessonId, title, kind} | null (контракт, п.17)
            const nextLessonId = item.nextLesson?.lessonId;
            const href = nextLessonId
              ? `/learn/${course.id}/${nextLessonId}`
              : `/learn/${course.id}`;
            return (
              <div key={course.id} className="panel">
                <div
                  className={`cover ${coverFor(course.slug || course.id)} enrolled-cover`}
                  aria-hidden="true"
                >
                  <span className="tag">Курс</span>
                </div>
                <h3>
                  <Link to={href}>{course.title}</Link>
                </h3>
                <div className="progress" aria-label={`${t('my.progress')}: ${item.percent}%`}>
                  <div className="progress-bar" style={{ width: `${item.percent}%` }} />
                </div>
                <div className="progress-meta">
                  <span>
                    {t('my.progress')}: {item.percent}%
                  </span>
                  <span>{item.nextLesson?.title ?? ''}</span>
                </div>
                <div className="btn-row">
                  <Link className="btn btn-primary" to={href}>
                    {t('my.continue')}
                  </Link>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
