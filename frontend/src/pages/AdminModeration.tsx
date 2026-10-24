// Очередь модерации курсов — отдельная страница /admin/moderation.
// Логика и разметка перенесены из секции #queue страницы Admin.tsx без
// изменений поведения: решение approve/reject, комментарий (обязателен при
// отклонении), ошибки по каждому курсу и повторная загрузка очереди.
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { formatDate, formatPrice, moderateCourse, reviewQueue } from '../api/client';
import type { ModerationDecision } from '../api/client';
import { t } from '../i18n/t';
import { useSeo } from '../app/useSeo';
import { errorText, shortId } from './adminShared';
import type { QueueCourse } from './adminShared';
// Стили админки изолированы в styles/admin.css — общий styles.css не трогаем.
import '../styles/admin.css';

export default function AdminModeration(): JSX.Element {
  useSeo(`${t('admin.reviewQueue')} — ${t('cabinet.adminTitle')} — Edu Platform`);
  const qc = useQueryClient();

  // Ключ ['review-queue'] сохранён — его инвалидация после решения по модерации.
  const queue = useQuery({ queryKey: ['review-queue'], queryFn: reviewQueue });

  // Комментарий и ошибка валидации — по каждому курсу отдельно.
  const [comments, setComments] = useState<Record<string, string>>({});
  const [commentErrors, setCommentErrors] = useState<Record<string, boolean>>({});
  const [moderateError, setModerateError] = useState('');

  const moderateMutation = useMutation({
    mutationFn: (args: { id: string; decision: ModerationDecision; comment?: string }) =>
      moderateCourse(args.id, { decision: args.decision, comment: args.comment }),
    onSuccess: (_data, args) => {
      setComments((prev) => ({ ...prev, [args.id]: '' }));
      setCommentErrors((prev) => ({ ...prev, [args.id]: false }));
      setModerateError('');
      void qc.invalidateQueries({ queryKey: ['review-queue'] });
    },
    onError: (e: unknown) => setModerateError(errorText(e)),
  });

  function onModerate(id: string, decision: ModerationDecision): void {
    const comment = (comments[id] ?? '').trim();
    if (decision === 'reject' && !comment) {
      // Без комментария backend отвечает 400 — ловим на клиенте.
      setCommentErrors((prev) => ({ ...prev, [id]: true }));
      return;
    }
    setCommentErrors((prev) => ({ ...prev, [id]: false }));
    moderateMutation.mutate({ id, decision, comment: comment || undefined });
  }

  // Очередь модерации: поля T48 могут ещё не приходить — берём безопасный вид.
  const queueItems = (queue.data ?? []) as QueueCourse[];

  return (
    <div className="wrap">
      <div className="page">
        <main className="admin">
          <p className="muted">
            <Link to="/admin">{t('p10.nav.backToAdmin')}</Link>
          </p>
          <h1>{t('admin.reviewQueue')}</h1>

          <section className="admin-section panel" aria-label={t('admin.reviewQueue')}>
            {queue.isLoading && <p>{t('admin.loading')}</p>}
            {queue.isError && (
              <div className="admin-error" role="alert">
                <p className="error">{errorText(queue.error)}</p>
                <button type="button" className="btn" onClick={() => void queue.refetch()}>
                  {t('common.retry')}
                </button>
              </div>
            )}
            {queue.data && queue.data.length === 0 && (
              <p className="muted">{t('admin.reviewEmpty')}</p>
            )}
            {moderateError && (
              <p role="alert" className="error">
                {moderateError}
              </p>
            )}

            {queue.data && queue.data.length > 0 && (
              <ul className="admin-queue list teach-list">
                {queueItems.map((c) => (
                  <li key={c.id} className="card admin-queue-card l-item">
                    <div className="admin-queue-card__head">
                      <strong>{c.title}</strong>
                      <span className="muted">{formatDate(c.createdAt)}</span>
                    </div>
                    <dl className="admin-meta">
                      <div>
                        <dt>{t('admin.queueTeacher')}</dt>
                        <dd>
                          {/* teacherEmail приходит после T48: до этого — прежний короткий id. */}
                          {c.teacherEmail ? c.teacherEmail : <code>{shortId(c.teacherId)}</code>}
                        </dd>
                      </div>
                      {typeof c.modulesCount === 'number' && (
                        <div>
                          <dt>{t('admin.queueModules')}</dt>
                          <dd>{c.modulesCount}</dd>
                        </div>
                      )}
                      {typeof c.lessonsCount === 'number' && (
                        <div>
                          <dt>{t('admin.queueLessons')}</dt>
                          <dd>{c.lessonsCount}</dd>
                        </div>
                      )}
                      <div>
                        <dt>{t('course.price')}</dt>
                        {/* Это цена курса — для неё «Бесплатно» при 0 уместно (как в каталоге). */}
                        <dd>{formatPrice(c.priceCents)}</dd>
                      </div>
                    </dl>
                    <div className="admin-queue-actions btn-row">
                      <label className="admin-comment">
                        <span className="sr-only">{t('admin.comment')}</span>
                        <input
                          className="input"
                          value={comments[c.id] ?? ''}
                          onChange={(e) =>
                            setComments((prev) => ({ ...prev, [c.id]: e.target.value }))
                          }
                          placeholder={t('admin.comment')}
                          aria-invalid={commentErrors[c.id] ? true : undefined}
                        />
                      </label>
                      <button
                        type="button"
                        className="btn btn-primary"
                        onClick={() => onModerate(c.id, 'approve')}
                        disabled={moderateMutation.isPending}
                      >
                        {t('admin.approve')}
                      </button>
                      <button
                        type="button"
                        className="btn admin-btn--danger"
                        onClick={() => onModerate(c.id, 'reject')}
                        disabled={moderateMutation.isPending}
                      >
                        {t('admin.reject')}
                      </button>
                      {/* Ссылка на публичную страницу курса — посмотреть перед решением. */}
                      <Link className="btn" to={`/courses/${c.slug}`}>
                        {t('admin.queueOpen')}
                      </Link>
                    </div>
                    {commentErrors[c.id] && (
                      <p role="alert" className="error">
                        {t('admin.rejectCommentRequired')}
                      </p>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </section>
        </main>
      </div>
    </div>
  );
}
