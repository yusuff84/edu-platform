// T82: публичная страница преподавателя /teachers/:id.
// Открывается без авторизации (ссылкой можно делиться); данные —
// GET /teachers/{id} по контракту P9 (T81). 404 — дружелюбное состояние.
// Стили — отдельный teacherProfile.css на переменных дизайн-системы.

import { Link, useParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { ApiError, getTeacherProfile } from '../api/client';
import CourseCard from '../components/CourseCard';
import { t } from '../i18n/t';
import '../styles/teacherProfile.css';

/** KPI-рейтинг: без оценок показываем «—», иначе одну цифру после запятой. */
function formatRating(value: number, count: number): string {
  if (count <= 0 || value <= 0) return '—';
  return value.toFixed(1).replace('.', ',');
}

/** Первая буква имени для декоративного аватара (пустое имя → «П»). */
function initial(name: string): string {
  const ch = name.trim().charAt(0);
  return ch ? ch.toUpperCase() : 'П';
}

/** Дружелюбный 404: преподавателя с таким id нет (или id пуст). */
function TeacherNotFound(): JSX.Element {
  return (
    <div className="wrap">
      <main className="page teacher-page">
        <div className="crumbs">
          <Link to="/">{t('nav.catalog')}</Link> /{' '}
          <span>{t('p9.teacher.title')}</span>
        </div>
        <div className="empty teacher-not-found">
          <h1>{t('p9.teacher.notFound')}</h1>
          <p>{t('p9.teacher.notFoundHint')}</p>
          <p>
            <Link className="btn btn-primary" to="/">
              {t('error.goCatalog')}
            </Link>
          </p>
        </div>
      </main>
    </div>
  );
}

export default function TeacherProfile(): JSX.Element {
  const { id = '' } = useParams();

  const query = useQuery({
    queryKey: ['teacher', id],
    queryFn: () => getTeacherProfile(id),
    enabled: Boolean(id),
    retry: false,
  });

  if (query.isLoading) {
    return (
      <div className="wrap">
        <main className="page teacher-page">
          <div className="panel" role="status">
            <p>{t('common.loading')}</p>
          </div>
        </main>
      </div>
    );
  }

  if (query.isError || !query.data) {
    const status = query.error instanceof ApiError ? query.error.status : undefined;
    if (status === 404 || !query.isError) return <TeacherNotFound />;
    return (
      <div className="wrap">
        <main className="page teacher-page">
          <div className="panel">
            <h1>{t('p9.teacher.title')}</h1>
            <p className="error" role="alert">
              {query.error instanceof ApiError ? query.error.message : t('error.network')}
            </p>
            <button type="button" className="btn" onClick={() => void query.refetch()}>
              {t('common.retry')}
            </button>
          </div>
        </main>
      </div>
    );
  }

  const { teacher, courses } = query.data;
  const items = courses ?? [];
  // displayName может быть пустым (поле необязательное) — нейтральный заголовок.
  const name = teacher.displayName?.trim() || t('p9.teacher.title');

  return (
    <div className="wrap">
      <main className="page teacher-page">
        <div className="crumbs">
          <Link to="/">{t('nav.catalog')}</Link> / <span>{name}</span>
        </div>

        <header className="teacher-hero">
          <div className="teacher-headline">
            <div className="teacher-avatar" aria-hidden="true">
              {initial(name)}
            </div>
            <div className="teacher-headline-text">
              <p className="teacher-kicker">{t('p9.teacher.title')}</p>
              <h1 className="teacher-name">{name}</h1>
            </div>
          </div>

          <dl className="teacher-kpis">
            <div className="teacher-kpi">
              <dt>{t('p9.teacher.coursesCount')}</dt>
              <dd>{teacher.coursesCount}</dd>
            </div>
            <div className="teacher-kpi">
              <dt>{t('p9.teacher.students')}</dt>
              <dd>{teacher.studentsCount}</dd>
            </div>
            <div className="teacher-kpi">
              <dt>{t('p9.teacher.rating')}</dt>
              <dd>{formatRating(teacher.avgRating, teacher.ratingsCount)}</dd>
            </div>
          </dl>
        </header>

        <section className="teacher-courses" aria-labelledby="teacher-courses-title">
          <div className="teacher-section-head">
            <h2 id="teacher-courses-title">{t('p9.teacher.courses')}</h2>
            <span className="muted">{items.length}</span>
          </div>

          {items.length === 0 ? (
            <div className="empty teacher-empty">
              <h3>{t('catalog.emptyCatalogTitle')}</h3>
              <p>У преподавателя пока нет опубликованных курсов.</p>
              <Link className="btn" to="/">
                {t('p9.teacher.allCourses')}
              </Link>
            </div>
          ) : (
            <div className="grid">
              {items.map((course) => (
                <CourseCard key={course.id} course={course} />
              ))}
            </div>
          )}
        </section>
      </main>
    </div>
  );
}
