import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import {
  ApiError,
  createReview,
  formatDate,
  listReviews,
  me,
  myCourses,
} from '../api/client';
import { t } from '../i18n/t';
import '../styles/catalog.css';

// Сообщения валидации: ключей словаря для них нет (см. отчёт T04) —
// оставляем литералы, как и было в исходной форме.
const schema = z.object({
  rating: z.coerce
    .number()
    .int('Оценка от 1 до 5')
    .min(1, 'Оценка от 1 до 5')
    .max(5, 'Оценка от 1 до 5'),
  // Ключ текста отзыва в контракте — body (не text): {rating, body}.
  body: z
    .string()
    .trim()
    .min(1, 'Напишите текст отзыва')
    .max(2000, 'Отзыв слишком длинный'),
});

type Form = z.infer<typeof schema>;

export default function Reviews({ courseId }: { courseId: string }): JSX.Element {
  const qc = useQueryClient();
  const [message, setMessage] = useState('');

  const meQuery = useQuery({ queryKey: ['me'], queryFn: me, retry: false });
  // После T01 список — плоский массив Review; рейтинг курса из него НЕ берём
  // (клиент теряет avgRating/ratingsCount конверта) — он на странице курса из getCourse.
  const reviewsQuery = useQuery({
    queryKey: ['reviews', courseId],
    queryFn: () => listReviews(courseId),
    retry: false,
  });
  // Активная запись на курс — условие показа формы; ключ общий с Course, дедупликация.
  const myQuery = useQuery({
    queryKey: ['my-courses'],
    queryFn: myCourses,
    enabled: meQuery.isSuccess,
    retry: false,
  });

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<Form>({
    resolver: zodResolver(schema),
    defaultValues: { rating: 5, body: '' },
  });

  const mutation = useMutation({
    // Тело запроса строго по контракту: { rating, body }.
    mutationFn: (v: Form) => createReview(courseId, { rating: v.rating, body: v.body }),
    onSuccess: () => {
      setMessage(t('reviews.success'));
      reset({ rating: 5, body: '' });
      void qc.invalidateQueries({ queryKey: ['reviews', courseId] });
      void qc.invalidateQueries({ queryKey: ['course'] });
    },
    onError: (e: unknown) => {
      if (e instanceof ApiError && e.status === 409) {
        setMessage(t('reviews.duplicate'));
        void qc.invalidateQueries({ queryKey: ['reviews', courseId] });
      } else if (e instanceof ApiError && e.status === 403) {
        setMessage(t('reviews.needEnroll'));
      } else {
        setMessage(e instanceof Error ? e.message : t('common.error'));
      }
    },
  });

  const user = meQuery.data;
  const reviews = reviewsQuery.data ?? [];
  // «Свой» отзыв — строго по studentId (email автора бэкенд не отдаёт).
  const myReview = user ? reviews.find((r) => r.studentId === user.id) : undefined;
  const enrolled = (myQuery.data ?? []).some((m) => m.course.id === courseId);
  // Форма видна, только если: вошли + активная запись + своего отзыва ещё нет.
  const formVisible =
    Boolean(user) && !myQuery.isLoading && !meQuery.isLoading && enrolled && !myReview;

  return (
    <section aria-label={t('reviews.title')} className="reviews">
      <h2>{t('reviews.title')}</h2>

      {reviewsQuery.isLoading && <p role="status">{t('reviews.loading')}</p>}
      {reviewsQuery.isError && (
        <>
          <p role="alert">{t('reviews.error')}</p>
          <button type="button" onClick={() => void reviewsQuery.refetch()}>
            {t('common.retry')}
          </button>
        </>
      )}
      {reviewsQuery.isSuccess && reviews.length === 0 && (
        <p className="muted">{t('reviews.empty')}</p>
      )}

      <ul className="list">
        {reviews.map((r) => (
          <li key={r.id} className="card review-item">
            <div className="card-row">
              <span
                role="img"
                aria-label={`${t('reviews.ratingLabel')}: ${r.rating}`}
              >
                {'★'.repeat(r.rating)}
                {'☆'.repeat(Math.max(0, 5 - r.rating))}
              </span>
              <span className="muted">{formatDate(r.createdAt)}</span>
            </div>
            {r.studentId === user?.id && (
              <p>
                <strong>{t('reviews.yourReview')}</strong>
              </p>
            )}
            {r.body && <p>{r.body}</p>}
          </li>
        ))}
      </ul>

      {!user && !meQuery.isLoading && (
        <p className="muted">
          {t('reviews.loginToReview')} — <Link to="/login">{t('nav.login')}</Link>
        </p>
      )}
      {user && !myQuery.isLoading && !enrolled && !myReview && (
        <p className="muted">{t('reviews.needEnroll')}</p>
      )}

      {formVisible && (
        <form
          className="review-form"
          onSubmit={handleSubmit((v) => mutation.mutate(v))}
        >
          <fieldset className="rating-group">
            <legend>{t('reviews.yourRating')}</legend>
            {[1, 2, 3, 4, 5].map((n) => (
              <label key={n} className="radio">
                <input
                  type="radio"
                  value={n}
                  {...register('rating')}
                  defaultChecked={n === 5}
                />
                {n} ★
              </label>
            ))}
            {errors.rating && <span className="error">{errors.rating.message}</span>}
          </fieldset>
          <label>
            {t('reviews.yourText')}
            <textarea {...register('body')} rows={4} cols={50} />
          </label>
          {errors.body && <span className="error">{errors.body.message}</span>}
          <button type="submit" disabled={mutation.isPending}>
            {t('reviews.submit')}
          </button>
        </form>
      )}

      {/* Результат отправки — для скринридеров и визуально. */}
      {message && (
        <p role="status" aria-live="polite">
          {message}
        </p>
      )}
    </section>
  );
}
