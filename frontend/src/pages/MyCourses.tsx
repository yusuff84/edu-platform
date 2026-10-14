import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { myCourses } from '../api/client';
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
      <main>
        <h1>{t('cabinet.studentTitle')}</h1>
        <p role="status">{t('my.loading')}</p>
      </main>
    );
  }

  // Ошибка сети / сервера — можно повторить без перезагрузки страницы
  if (query.isError) {
    return (
      <main>
        <h1>{t('cabinet.studentTitle')}</h1>
        <p role="alert" className="error">
          {t('error.network')}
        </p>
        <button type="button" onClick={() => void query.refetch()}>
          {t('common.retry')}
        </button>
      </main>
    );
  }

  const items = query.data ?? [];

  // Пустой список
  if (items.length === 0) {
    return (
      <main>
        <h1>{t('cabinet.studentTitle')}</h1>
        <p>{t('my.empty')}</p>
        <p>
          <Link to="/">{t('my.goCatalog')}</Link>
        </p>
      </main>
    );
  }

  return (
    <main>
      <h1>{t('cabinet.studentTitle')}</h1>
      <ul className="list">
        {items.map((item) => {
          const course = item.course;
          // nextLesson — объект {lessonId, title, kind} | null (контракт, п.17)
          const nextLessonId = item.nextLesson?.lessonId;
          const learnHref = nextLessonId
            ? `/learn/${course.id}/${nextLessonId}`
            : `/learn/${course.id}`;
          return (
            <li key={course.id} className="card">
              <Link to={learnHref}>
                <strong>{course.title}</strong>
              </Link>
              <div className="progress" aria-label={`${t('my.progress')}: ${item.percent}%`}>
                <div className="progress-bar" style={{ width: `${item.percent}%` }} />
              </div>
              <span>
                {t('my.progress')}: {item.percent}%
              </span>{' '}
              <Link to={learnHref}>{t('my.continue')}</Link>
            </li>
          );
        })}
      </ul>
    </main>
  );
}
