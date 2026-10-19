import { useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import {
  ApiError,
  createReview,
  deleteReviewReply,
  formatDate,
  getCourseReviews,
  me,
  myCourses,
  replyToReview,
} from '../api/client';
import { t } from '../i18n/t';
import '../styles/catalog.css';
import '../styles/reviews-extra.css';

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

/** T69: текст ответа по контракту — после trim 3..2000; ключа словаря нет. */
const REPLY_LENGTH_ERROR = 'Ответ: от 3 до 2000 символов';

const RATING_ROWS = ['5', '4', '3', '2', '1'] as const;
type RatingKey = (typeof RATING_ROWS)[number];

export default function Reviews({
  courseId,
  canReply = false,
}: {
  courseId: string;
  /** T69: владелец курса или admin — можно отвечать на отзывы. */
  canReply?: boolean;
}): JSX.Element {
  const qc = useQueryClient();
  const [message, setMessage] = useState('');
  const [ratingFilter, setRatingFilter] = useState<number | null>(null);
  const [replyForId, setReplyForId] = useState<string | null>(null);
  const [replyDraft, setReplyDraft] = useState('');
  const [replyError, setReplyError] = useState('');

  const meQuery = useQuery({ queryKey: ['me'], queryFn: me, retry: false });
  // T69: статистика и «свой отзыв» — всегда по всем отзывам (без фильтра).
  // При выбранном фильтре это отдельный запрос; без фильтра ключ совпадает
  // с ключом списка и React Query дедуплицирует запросы.
  const statsQuery = useQuery({
    queryKey: ['reviews', courseId, 'all'],
    queryFn: () => getCourseReviews(courseId),
    retry: false,
  });
  // Список с учётом фильтра по оценке. Конверт несёт breakdown/avgRating/
  // ratingsCount по всем отзывам, items/total — по выбранной оценке.
  const reviewsQuery = useQuery({
    queryKey: ['reviews', courseId, ratingFilter ?? 'all'],
    queryFn: () =>
      getCourseReviews(courseId, ratingFilter ? { rating: ratingFilter } : undefined),
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

  /** T69: после любого изменения отзывов обновляем список/статистику и курс. */
  const invalidateReviews = (): void => {
    void qc.invalidateQueries({ queryKey: ['reviews', courseId] });
    void qc.invalidateQueries({ queryKey: ['course'] });
  };

  const mutation = useMutation({
    // Тело запроса строго по контракту: { rating, body }.
    mutationFn: (v: Form) => createReview(courseId, { rating: v.rating, body: v.body }),
    onSuccess: () => {
      setMessage(t('reviews.success'));
      reset({ rating: 5, body: '' });
      invalidateReviews();
    },
    onError: (e: unknown) => {
      if (e instanceof ApiError && e.status === 409) {
        setMessage(t('reviews.duplicate'));
        invalidateReviews();
      } else if (e instanceof ApiError && e.status === 403) {
        setMessage(t('reviews.needEnroll'));
      } else {
        setMessage(e instanceof Error ? e.message : t('common.error'));
      }
    },
  });

  const replyMutation = useMutation({
    mutationFn: (v: { reviewId: string; body: string }) =>
      replyToReview(courseId, v.reviewId, v.body),
    onSuccess: () => {
      setMessage(t('p8.reviews.replySaved'));
      setReplyForId(null);
      setReplyDraft('');
      setReplyError('');
      invalidateReviews();
    },
    onError: (e: unknown) => {
      // Ошибки сервера (400/403/404) показываем текстом ApiError.message.
      setReplyError(e instanceof Error ? e.message : t('common.error'));
    },
  });

  const deleteReplyMutation = useMutation({
    mutationFn: (reviewId: string) => deleteReviewReply(courseId, reviewId),
    onSuccess: () => {
      setMessage('');
      setReplyForId(null);
      setReplyDraft('');
      setReplyError('');
      invalidateReviews();
    },
    onError: (e: unknown) => {
      setReplyError(e instanceof Error ? e.message : t('common.error'));
    },
  });

  const user = meQuery.data;
  const reviews = reviewsQuery.data?.items ?? [];
  const stats = statsQuery.data;
  // «Свой» отзыв — строго по studentId (email автора бэкенд не отдаёт).
  // Ищем по полному списку, а не по отфильтрованному: иначе при фильтре
  // форма создания показалась бы повторно (сервер ответил бы 409).
  const myReview = user
    ? (statsQuery.data?.items ?? []).find((r) => r.studentId === user.id)
    : undefined;
  const enrolled = (myQuery.data ?? []).some((m) => m.course.id === courseId);
  // Форма видна, только если: вошли + активная запись + своего отзыва ещё нет.
  const formVisible =
    Boolean(user) && !myQuery.isLoading && !meQuery.isLoading && enrolled && !myReview;
  const ratingsCount = stats?.ratingsCount ?? 0;
  const breakdown = stats?.breakdown;
  const maxCount = Math.max(
    1,
    ...RATING_ROWS.map((key) => breakdown?.[key as RatingKey] ?? 0),
  );

  const toggleRating = (rating: number): void => {
    setRatingFilter((current) => (current === rating ? null : rating));
  };

  const openReply = (reviewId: string, currentReply?: string): void => {
    setReplyForId(reviewId);
    setReplyDraft(currentReply ?? '');
    setReplyError('');
    setMessage('');
  };

  const cancelReply = (): void => {
    setReplyForId(null);
    setReplyDraft('');
    setReplyError('');
  };

  const submitReply = (e: FormEvent<HTMLFormElement>): void => {
    e.preventDefault();
    if (!replyForId) return;
    const body = replyDraft.trim();
    if (body.length < 3 || body.length > 2000) {
      setReplyError(REPLY_LENGTH_ERROR);
      return;
    }
    setReplyError('');
    replyMutation.mutate({ reviewId: replyForId, body });
  };

  const removeReply = (): void => {
    if (!replyForId) return;
    if (!window.confirm(`${t('p8.reviews.replyDelete')}?`)) return;
    deleteReplyMutation.mutate(replyForId);
  };

  const retry = (): void => {
    void reviewsQuery.refetch();
    if (ratingFilter !== null) void statsQuery.refetch();
  };

  return (
    <section aria-label={t('reviews.title')} className="panel reviews">
      <h2>{t('reviews.title')}</h2>

      {stats && (
        <div className="reviews-stats">
          <div className="reviews-summary">
            <div className="reviews-avg">{stats.avgRating.toFixed(1)}</div>
            <div className="reviews-avg-stars" aria-hidden="true">
              {'★'.repeat(Math.round(stats.avgRating))}
              {'☆'.repeat(Math.max(0, 5 - Math.round(stats.avgRating)))}
            </div>
            <div className="reviews-total">
              {ratingsCount} {t('course.reviewsCount')}
            </div>
          </div>
          <div className="reviews-hist">
            <div className="reviews-hist-head">
              <span>{t('p8.reviews.ratings')}</span>
              <button
                type="button"
                className="reviews-reset"
                onClick={() => setRatingFilter(null)}
                aria-pressed={ratingFilter === null}
                disabled={ratingFilter === null}
              >
                {t('p8.reviews.all')}
              </button>
            </div>
            {RATING_ROWS.map((key) => {
              const rating = Number(key);
              const count = breakdown?.[key as RatingKey] ?? 0;
              const percent = Math.round((count / maxCount) * 100);
              return (
                <button
                  key={key}
                  type="button"
                  className="reviews-hist-row"
                  aria-pressed={ratingFilter === rating}
                  aria-label={`${rating} ★ — ${count}`}
                  onClick={() => toggleRating(rating)}
                >
                  <span className="reviews-hist-label">{rating} ★</span>
                  <span className="reviews-bar" aria-hidden="true">
                    <span className="reviews-bar-fill" style={{ width: `${percent}%` }} />
                  </span>
                  <span className="reviews-hist-count">{count}</span>
                </button>
              );
            })}
          </div>
        </div>
      )}

      {reviewsQuery.isLoading && <p role="status">{t('reviews.loading')}</p>}
      {reviewsQuery.isError && (
        <>
          <p role="alert">{t('reviews.error')}</p>
          <button type="button" className="btn" onClick={retry}>
            {t('common.retry')}
          </button>
        </>
      )}
      {reviewsQuery.isSuccess && reviews.length === 0 && (
        <p className="muted">
          {ratingFilter !== null ? t('p8.reviews.emptyRating') : t('reviews.empty')}
        </p>
      )}

      <ul className="list">
        {reviews.map((r) => (
          <li key={r.id} className="card review-item review">
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

            {r.teacherReply && (
              <div className="reviews-reply">
                <div className="reviews-reply-head">
                  <span>{t('p8.reviews.teacher')}</span>
                  <span className="muted">{formatDate(r.teacherReplyAt)}</span>
                </div>
                <p>{r.teacherReply}</p>
              </div>
            )}

            {canReply &&
              (replyForId === r.id ? (
                <form className="reviews-reply-form" onSubmit={submitReply}>
                  <label className="field" htmlFor={`review-reply-${r.id}`}>
                    <span>{t('p8.reviews.replyFor')}</span>
                    <textarea
                      id={`review-reply-${r.id}`}
                      className="input"
                      rows={4}
                      maxLength={2000}
                      placeholder={t('p8.reviews.replyPlaceholder')}
                      value={replyDraft}
                      aria-invalid={Boolean(replyError)}
                      aria-describedby={
                        replyError ? `review-reply-error-${r.id}` : undefined
                      }
                      onChange={(e) => {
                        setReplyDraft(e.target.value);
                        if (replyError) setReplyError('');
                      }}
                    />
                  </label>
                  <span
                    className={`reviews-counter${
                      replyDraft.length > 2000 ? ' over' : ''
                    }`}
                  >
                    {replyDraft.length}/2000
                  </span>
                  {replyError && (
                    <p
                      className="err"
                      id={`review-reply-error-${r.id}`}
                      role="alert"
                    >
                      {replyError}
                    </p>
                  )}
                  <div className="reviews-actions">
                    <button
                      type="submit"
                      className="btn btn-primary btn-sm"
                      disabled={replyMutation.isPending}
                    >
                      {t('p8.reviews.replySave')}
                    </button>
                    <button
                      type="button"
                      className="btn btn-sm"
                      onClick={cancelReply}
                      disabled={replyMutation.isPending}
                    >
                      {t('p8.reviews.replyCancel')}
                    </button>
                    {r.teacherReply && (
                      <button
                        type="button"
                        className="btn btn-ghost btn-sm reviews-delete-reply"
                        onClick={removeReply}
                        disabled={deleteReplyMutation.isPending}
                      >
                        {t('p8.reviews.replyDelete')}
                      </button>
                    )}
                  </div>
                </form>
              ) : (
                <div className="reviews-actions">
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    onClick={() => openReply(r.id, r.teacherReply)}
                  >
                    {r.teacherReply ? t('p8.reviews.editReply') : t('p8.reviews.reply')}
                  </button>
                </div>
              ))}
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
          className="review-form form-grid"
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
            {errors.rating && <span className="error err">{errors.rating.message}</span>}
          </fieldset>
          <div className="field">
            <label htmlFor="review-text">{t('reviews.yourText')}</label>
            <textarea id="review-text" className="input" {...register('body')} rows={4} cols={50} />
            {errors.body && <span className="error err">{errors.body.message}</span>}
          </div>
          <div>
            <button type="submit" className="btn btn-primary" disabled={mutation.isPending}>
              {t('reviews.submit')}
            </button>
          </div>
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
