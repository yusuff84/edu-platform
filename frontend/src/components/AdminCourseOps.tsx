import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { adminPublishCourse, adminSetCourseOwner } from '../api/adminOps';

/** Действия администратора над курсом: опубликовать сразу, передать другому преподавателю. */
export function AdminCourseOps({ courseId, status }: { courseId: string; status: string }): JSX.Element {
  const qc = useQueryClient();
  const [email, setEmail] = useState('');
  const [note, setNote] = useState('');
  const run = useMutation({
    mutationFn: async (fn: () => Promise<string>) => fn(),
    onSuccess: (m) => {
      setNote(m);
      void qc.invalidateQueries();
    },
    onError: (e) => setNote(e instanceof Error ? e.message : 'Ошибка'),
  });
  return (
    <section className="panel aua-section">
      <h2>Действия администратора</h2>
      <div className="aua-actions">
        {status !== 'published' && (
          <button
            type="button"
            className="btn btn-sm btn-primary"
            disabled={run.isPending}
            onClick={() => {
              if (window.confirm('Опубликовать курс сразу, без очереди модерации?'))
                run.mutate(async () => {
                  await adminPublishCourse(courseId);
                  return 'Курс опубликован';
                });
            }}
          >
            Опубликовать сейчас
          </button>
        )}
      </div>
      <form
        className="aua-inline"
        onSubmit={(e) => {
          e.preventDefault();
          if (!email.includes('@')) return;
          if (window.confirm(`Передать курс пользователю ${email}?`))
            run.mutate(async () => {
              const r = await adminSetCourseOwner(courseId, email.trim());
              setEmail('');
              return `Курс передан: ${r.teacherEmail}`;
            });
        }}
      >
        <input
          className="input"
          type="email"
          placeholder="Email нового владельца (преподаватель)"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          aria-label="Email нового владельца курса"
        />
        <button type="submit" className="btn btn-sm" disabled={!email.includes('@') || run.isPending}>
          Передать курс
        </button>
      </form>
      {note && <p className="aua-note" role="status">{note}</p>}
    </section>
  );
}
