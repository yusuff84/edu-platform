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
    <main>
      <h1>{t('cabinet.adminTitle')}</h1>

      <section>
        <h2>{t('admin.invites')}</h2>
        <form onSubmit={handleSubmit((v) => inviteMutation.mutate(v))}>
          <label>
            {t('admin.inviteEmail')}
            <input type="email" {...register('email')} />
            {formState.errors.email && (
              <span className="error">{formState.errors.email.message}</span>
            )}
          </label>
          <button type="submit" disabled={inviteMutation.isPending}>
            {t('admin.inviteSend')}
          </button>
        </form>
        {inviteMsg && <p role="status">{inviteMsg}</p>}
      </section>

      <section>
        <h2>{t('admin.reviewQueue')}</h2>
        {queue.isLoading && <p>{t('common.loading')}</p>}
        {queue.isError && <p>{t('common.error')}</p>}
        {queue.data?.length === 0 && <p>{t('admin.reviewEmpty')}</p>}
        <ul>
          {(queue.data ?? []).map((c) => (
            <li key={c.id}>
              <strong>{c.title}</strong>
              <div>
                <input
                  value={comment}
                  onChange={(e) => setComment(e.target.value)}
                  placeholder={t('admin.comment')}
                  aria-label={t('admin.comment')}
                />
                <button
                  onClick={() =>
                    moderateMutation.mutate({ courseId: c.id, decision: 'approve' })
                  }
                  disabled={moderateMutation.isPending}
                >
                  {t('admin.approve')}
                </button>
                <button
                  onClick={() => moderateMutation.mutate({ courseId: c.id, decision: 'reject' })}
                  disabled={moderateMutation.isPending}
                >
                  {t('admin.reject')}
                </button>
              </div>
            </li>
          ))}
        </ul>
      </section>

      <section>
        <h2>{t('admin.journal')}</h2>
        <p className="muted">{t('admin.journalEmpty')}</p>
      </section>
    </main>
  );
}
