import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { createInvite, moderateCourse, reviewQueue } from '../api/client';
import { t } from '../i18n/t';

const inviteSchema = z.object({
  email: z.string().min(1, 'Укажите email').email('Некорректный email'),
});

type InviteForm = z.infer<typeof inviteSchema>;

export default function Admin(): JSX.Element {
  const qc = useQueryClient();
  const [inviteMsg, setInviteMsg] = useState('');
  const [comment, setComment] = useState('');
  const queue = useQuery({ queryKey: ['review-queue'], queryFn: reviewQueue });

  const { register, handleSubmit, reset, formState } = useForm<InviteForm>({
    resolver: zodResolver(inviteSchema),
  });

  const inviteMutation = useMutation({
    mutationFn: (v: InviteForm) => createInvite(v.email),
    onSuccess: () => {
      setInviteMsg(t('auth.success'));
      reset();
    },
    onError: (e: unknown) => setInviteMsg(e instanceof Error ? e.message : t('common.error')),
  });

  const moderateMutation = useMutation({
    mutationFn: (args: { courseId: string; decision: 'approve' | 'reject' }) =>
      moderateCourse(args.courseId, { decision: args.decision, comment }),
    onSuccess: () => {
      setComment('');
      void qc.invalidateQueries({ queryKey: ['review-queue'] });
    },
  });

  return (
    <div className="wrap">
      <div className="page">
        <h1>{t('cabinet.adminTitle')}</h1>

        <div className="two two-even">
          <section className="panel">
            <h2>{t('admin.invites')}</h2>
            <form className="form-grid" onSubmit={handleSubmit((v) => inviteMutation.mutate(v))}>
              <div className="field">
                <label htmlFor="invite-email">{t('admin.inviteEmail')}</label>
                <input id="invite-email" className="input" type="email" {...register('email')} />
                {formState.errors.email && (
                  <span className="error">{formState.errors.email.message}</span>
                )}
              </div>
              <div>
                <button type="submit" className="btn btn-primary" disabled={inviteMutation.isPending}>
                  {t('admin.inviteSend')}
                </button>
              </div>
            </form>
            {inviteMsg && <p role="status">{inviteMsg}</p>}
          </section>

          <section className="panel">
            <h2>{t('admin.reviewQueue')}</h2>
            {queue.isLoading && <p>{t('common.loading')}</p>}
            {queue.isError && <p>{t('common.error')}</p>}
            {queue.data?.length === 0 && <p>{t('admin.reviewEmpty')}</p>}
            <ul className="teach-list">
              {(queue.data ?? []).map((c) => (
                <li key={c.id} className="l-item" style={{ flexWrap: 'wrap' }}>
                  <strong>{c.title}</strong>
                  <span className="spacer" />
                  <span className="status st-wait">{c.status}</span>
                  <div style={{ display: 'grid', gap: '8px', width: '100%' }}>
                    <input
                      className="input"
                      value={comment}
                      onChange={(e) => setComment(e.target.value)}
                      placeholder={t('admin.comment')}
                      aria-label={t('admin.comment')}
                    />
                    <div className="btn-row" style={{ marginTop: 0 }}>
                      <button
                        className="btn btn-primary"
                        onClick={() =>
                          moderateMutation.mutate({ courseId: c.id, decision: 'approve' })
                        }
                        disabled={moderateMutation.isPending}
                      >
                        {t('admin.approve')}
                      </button>
                      <button
                        className="btn"
                        onClick={() => moderateMutation.mutate({ courseId: c.id, decision: 'reject' })}
                        disabled={moderateMutation.isPending}
                      >
                        {t('admin.reject')}
                      </button>
                    </div>
                  </div>
                </li>
              ))}
            </ul>
          </section>
        </div>

        <section className="panel" style={{ marginTop: '22px' }}>
          <h2>{t('admin.journal')}</h2>
          <p className="muted">{t('admin.journalEmpty')}</p>
        </section>
      </div>
    </div>
  );
}
