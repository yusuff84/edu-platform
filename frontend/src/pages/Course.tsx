import { useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, enroll, formatPrice, getCourse, me, myCourses } from '../api/client';
import Reviews from '../components/Reviews';
import { Stars } from '../components/CourseCard';
import { t } from '../i18n/t';

export default function Course(): JSX.Element {
  const { slug = '' } = useParams();
  const qc = useQueryClient();
  const [message, setMessage] = useState('');
  const [alreadyOwned, setAlreadyOwned] = useState(false);
  const courseQuery = useQuery({
    queryKey: ['course', slug],
    queryFn: () => getCourse(slug),
    enabled: Boolean(slug),
    retry: false,
  });
  const meQuery = useQuery({ queryKey: ['me'], queryFn: me, retry: false });
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
      setAlreadyOwned(false);
      void qc.invalidateQueries({ queryKey: ['my-courses'] });
      if (res.redirectUrl) window.location.href = res.redirectUrl;
    },
    onError: (e: unknown) => {
      if (e instanceof ApiError && e.status === 409) {
        setAlreadyOwned(true);
        setMessage(t('course.alreadyBought'));
        void qc.invalidateQueries({ queryKey: ['my-courses'] });
      } else setMessage(e instanceof Error ? e.message : t('common.error'));
    },
  });

  if (courseQuery.isLoading) return <p>{t('course.loading')}</p>;
  if (courseQuery.isError || !courseQuery.data) {
    const status =
      courseQuery.error instanceof ApiError ? courseQuery.error.status : undefined;
    if (status === 404 || !courseQuery.isError) {
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
          {t('common.retry')}
        </button>{' '}
        <Link to="/">{t('my.goCatalog')}</Link>
      </main>
    );
  }
  const course = courseQuery.data;
  const owned =
    alreadyOwned || (myQuery.data ?? []).some((m) => m.courseId === course.id);

  return (
    <main>
      <h1>{course.title}</h1>
      <p>{course.description}</p>
      <p>
        <Stars value={course.avgRating ?? 0} /> ({course.reviewsCount ?? 0})
      </p>
      <p>
        {t('course.price')}: <strong>{formatPrice(course.priceCents)}</strong>
      </p>
      {course.teacherName && (
        <p>
          {t('course.teacher')}: {course.teacherName}
        </p>
      )}

      {!meQuery.data && <p>{t('course.loginToBuy')}</p>}
      {meQuery.data && !owned && (
        <button onClick={() => buyMutation.mutate(course.id)} disabled={buyMutation.isPending}>
          {buyMutation.isPending ? t('course.buying') : t('course.buy')}
        </button>
      )}
      {owned && (
        <p>
          <Link to={`/learn/${course.id}`}>{t('course.openLearn')}</Link>
        </p>
      )}
      {message && <p role="status">{message}</p>}

      <h2>{t('course.program')}</h2>
      {(course.modules ?? []).map((m) => (
        <section key={m.id}>
          <h3>{m.title}</h3>
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
