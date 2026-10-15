import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { ApiError, createReview, listReviews, me } from '../api/client';
import { t } from '../i18n/t';

const schema = z.object({
  rating: z.coerce.number().min(1, 'Оценка от 1 до 5').max(5, 'Оценка от 1 до 5'),
  text: z.string().min(3, 'Отзыв слишком короткий').max(2000, 'Отзыв слишком длинный'),
});

type Form = z.infer<typeof schema>;

export default function Reviews({ courseId }: { courseId: string }): JSX.Element {
  const qc = useQueryClient();
  const [message, setMessage] = useState('');
  const [denied, setDenied] = useState(false);
  const [duplicate, setDuplicate] = useState(false);
  const meQuery = useQuery({ queryKey: ['me'], queryFn: me, retry: false });
  const reviewsQuery = useQuery({
    queryKey: ['reviews', courseId],
    queryFn: () => listReviews(courseId),
  });
  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<Form>({ resolver: zodResolver(schema), defaultValues: { rating: 5, text: '' } });

  const mutation = useMutation({
    mutationFn: (v: Form) => createReview(courseId, v),
    onSuccess: () => {
      setMessage(t('reviews.success'));
      setDenied(false);
      setDuplicate(false);
      reset();
      void qc.invalidateQueries({ queryKey: ['reviews', courseId] });
      void qc.invalidateQueries({ queryKey: ['course'] });
    },
    onError: (e: unknown) => {
      if (e instanceof ApiError && e.status === 409) {
        setDuplicate(true);
        setDenied(false);
        setMessage(t('reviews.duplicate'));
        void qc.invalidateQueries({ queryKey: ['reviews', courseId] });
      } else if (e instanceof ApiError && e.status === 403) {
        setDenied(true);
        setDuplicate(false);
        setMessage(t('reviews.enrollHint'));
      } else setMessage(e instanceof Error ? e.message : t('common.error'));
    },
  });

  const myEmail = meQuery.data?.email;
  const myReview = duplicate
    ? (reviewsQuery.data ?? []).find((r) => r.authorEmail === myEmail)
    : undefined;

  return (
    <section className="panel" aria-label={t('reviews.title')}>
      <h2>{t('reviews.title')}</h2>
      {reviewsQuery.isLoading && <p>{t('common.loading')}</p>}
      {reviewsQuery.isError && <p>{t('common.error')}</p>}
      {reviewsQuery.data?.length === 0 && !duplicate && (
        <p className="muted">{t('reviews.empty')}</p>
      )}
      <ul style={{ listStyle: 'none', margin: 0, padding: 0 }}>
        {(reviewsQuery.data ?? []).map((r) => (
          <li key={r.id} className="review">
            <div>
              <strong>{r.authorEmail}</strong> <span className="rating">★ {r.rating}</span>
            </div>
            <p>{r.text}</p>
          </li>
        ))}
      </ul>

      {!meQuery.data && !meQuery.isLoading && (
        <p className="muted">
          {t('reviews.loginToReview')} — <Link to="/login">{t('nav.login')}</Link>
        </p>
      )}

      {denied && (
        <p role="note" className="muted">
          {t('reviews.enrollHint')}
        </p>
      )}

      {duplicate && myReview && (
        <div role="note" aria-label={t('reviews.yourReview')}>
          <p>
            <strong>{t('reviews.yourReview')}:</strong> {'★'.repeat(myReview.rating)}
          </p>
          <p>{myReview.text}</p>
        </div>
      )}

      {meQuery.data && !duplicate && (
        <form className="form-grid" onSubmit={handleSubmit((v) => mutation.mutate(v))}>
          <div className="field">
            <label htmlFor="review-rating">{t('reviews.yourRating')}</label>
            <input
              id="review-rating"
              className="input"
              type="number"
              min={1}
              max={5}
              {...register('rating')}
            />
            {errors.rating && <p className="err">{errors.rating.message}</p>}
          </div>
          <div className="field">
            <label htmlFor="review-text">{t('reviews.yourText')}</label>
            <textarea
              id="review-text"
              className="input"
              {...register('text')}
              rows={4}
            />
            {errors.text && <p className="err">{errors.text.message}</p>}
          </div>
          <div>
            <button
              type="submit"
              className="btn btn-primary"
              disabled={mutation.isPending}
            >
              {t('reviews.submit')}
            </button>
          </div>
        </form>
      )}
      {message && <p role="status">{message}</p>}
    </section>
  );
}
