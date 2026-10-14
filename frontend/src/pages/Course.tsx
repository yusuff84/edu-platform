import { useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { CourseStatus } from '../api/client';
import { ApiError, enroll, formatPrice, getCourse, me, myCourses } from '../api/client';
import Reviews from '../components/Reviews';
import { Stars } from '../components/CourseCard';
import { t } from '../i18n/t';
import '../styles/catalog.css';

/** Человеческая подпись статуса курса (для баннера автора/админа). */
function statusLabel(status: CourseStatus): string {
  switch (status) {
    case 'draft':
      return t('teach.statusDraft');
    case 'review':
      return t('teach.statusReview');
    case 'published':
      return t('teach.statusPublished');
    case 'archived':
      return t('teach.statusArchived');
  }
}

export default function Course(): JSX.Element {
  // slug — человекоприятный слаг из URL, не UUID; контракт: GET /courses/{slug}.
  const { slug = '' } = useParams();
  const qc = useQueryClient();
  const [message, setMessage] = useState('');

  // После T01 getCourse распаковывает конверт { course, modules } в CourseDetail.
  const courseQuery = useQuery({
    queryKey: ['course', slug],
    queryFn: () => getCourse(slug),
    enabled: Boolean(slug),
    retry: false,
  });
  const meQuery = useQuery({ queryKey: ['me'], queryFn: me, retry: false });
  // Активная запись: /me/courses после T01 — массив MyCourse (курс лежит в m.course).
  const myQuery = useQuery({
    queryKey: ['my-courses'],
    queryFn: myCourses,
    enabled: meQuery.isSuccess,
    retry: false,
  });

  const buyMutation = useMutation({
    mutationFn: (courseId: string) => enroll(courseId, 'mock'),
    onSuccess: (res) => {
      setMessage('');
      void qc.invalidateQueries({ queryKey: ['my-courses'] });
      if (res.redirectUrl) {
        window.location.href = res.redirectUrl;
        return;
      }
      // Бесплатный курс: запись активна сразу — покажем переход к обучению.
      if (res.enrollmentActive) setMessage(t('course.openLearn'));
    },
    onError: (e: unknown) => {
      if (e instanceof ApiError && e.status === 409) {
        // 409 «уже есть активная запись» — не ошибка, а сообщение.
        setMessage(t('course.alreadyBought'));
        void qc.invalidateQueries({ queryKey: ['my-courses'] });
      } else {
        setMessage(e instanceof Error ? e.message : t('common.error'));
      }
    },
  });

  if (courseQuery.isLoading) {
    return (
      <main>
        <p role="status">{t('course.loading')}</p>
      </main>
    );
  }
  if (courseQuery.isError || !courseQuery.data) {
    const status =
      courseQuery.error instanceof ApiError ? courseQuery.error.status : undefined;
    if (status === 404) {
      // Черновик/снятый курс бэкенд отдаёт как 404 для посторонних.
      return (
        <main>
          <h1>{t('course.notFound')}</h1>
          <p>{t('course.notFoundHint')}</p>
          <p>
            <Link to="/">{t('my.goCatalog')}</Link>
          </p>
        </main>
      );
    }
    return (
      <main>
        <h1>{t('course.notFound')}</h1>
        <p role="alert">{t('common.error')}</p>
        <button type="button" onClick={() => void courseQuery.refetch()}>
          {t('error.retry')}
        </button>{' '}
        <Link to="/">{t('my.goCatalog')}</Link>
      </main>
    );
  }

  const course = courseQuery.data;
  const user = meQuery.data;
  // Кнопка покупки — только когда список записей загружен (не мигает для записанных).
  const owned = (myQuery.data ?? []).some((m) => m.course.id === course.id);
  const isAuthor = user?.id === course.teacherId;
  const isAdmin = user?.role === 'admin';
  // Баннер статуса виден только автору/админу: остальным бэкенд такие курсы отдаёт 404.
  const showBanner =
    course.status !== 'published' && (isAuthor || isAdmin);
  // Курс вернулся в draft с комментарием = отклонён модератором.
  const rejected = course.status === 'draft' && Boolean(course.moderationComment);

  return (
    <main>
      <h1>{course.title}</h1>
      <p>{course.description}</p>
      <p>
        <Stars value={course.avgRating ?? 0} />{' '}
        {course.ratingsCount > 0 && (
          <span className="muted">
            ({course.ratingsCount} {t('reviews.title').toLowerCase()})
          </span>
        )}
      </p>
      <p>
        {t('course.price')}: <strong>{formatPrice(course.priceCents)}</strong>
      </p>

      {showBanner && (
        <section className="course-banner" aria-label={t('teach.status')}>
          <strong>
            {t('teach.status')}: {statusLabel(course.status)}
          </strong>
          {rejected && (
            <p>
              <strong>{t('teach.rejected')}</strong>
              {course.moderationComment && (
                <>
                  {' — '}
                  {t('teach.moderationComment')}: {course.moderationComment}
                </>
              )}
            </p>
          )}
        </section>
      )}

      {!user && !meQuery.isLoading && (
        <p>
          <Link to="/login">{t('course.loginToBuy')}</Link>
        </p>
      )}
      {user && !owned && !myQuery.isLoading && (
        <button
          type="button"
          onClick={() => buyMutation.mutate(course.id)}
          disabled={buyMutation.isPending}
        >
          {buyMutation.isPending
            ? t('course.buying')
            : course.priceCents === 0
              ? t('course.enrollFree')
              : t('course.buy')}
        </button>
      )}
      {owned && (
        <p>
          <Link to={`/learn/${course.id}`}>{t('course.openLearn')}</Link>
        </p>
      )}
      {message && (
        <p role="status" aria-live="polite">
          {message}
        </p>
      )}

      <h2>{t('course.program')}</h2>
      {course.modules.length === 0 && <p className="muted">{t('learn.noLessons')}</p>}
      {course.modules.length > 0 && (
        <p className="muted">
          {t('course.modulesCount')}: {course.modules.length}
        </p>
      )}
      {course.modules.map((m) => (
        <section key={m.id} className="course-module">
          <h3>{m.title}</h3>
          <p className="muted">
            {t('course.lessonsCount')}: {m.lessons.length}
          </p>
          <ul>
            {m.lessons.map((l) => (
              <li key={l.id}>
                {l.title} — {t(`kind.${l.kind}`)}
              </li>
            ))}
          </ul>
        </section>
      ))}

      <Reviews courseId={course.id} />
    </main>
  );
}
