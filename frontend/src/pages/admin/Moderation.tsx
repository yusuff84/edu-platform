import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { formatPrice, reviewQueue } from '../../api/client';
import type { Course } from '../../api/types';
import { contentKeys } from '../../api/adminContent';
import {
  AdminPage,
  EmptyState,
  ErrorNote,
  Icon,
  RowMenu,
  Spinner,
  fmtAgo,
  fmtDateTime,
  plural,
} from '../../components/admin/ui';
import { CourseThumb, useCourseOps } from './Courses';
import '../../styles/admin-content.css';

/** Курс очереди: к полям курса backend добавляет email автора и объём программы. */
type QueueCourse = Course & {
  teacherEmail?: string;
  modulesCount?: number;
  lessonsCount?: number;
};

export default function Moderation(): JSX.Element {
  const ops = useCourseOps();
  const queue = useQuery({
    queryKey: contentKeys.queue,
    queryFn: async () => (await reviewQueue()) as QueueCourse[],
  });
  const items = queue.data ?? [];

  return (
    <AdminPage
      title="Модерация"
      description={
        queue.data && items.length > 0
          ? `${items.length} ${plural(items.length, 'курс ждёт', 'курса ждут', 'курсов ждут')} проверки. Посмотрите курс и примите решение — автор получит ваш комментарий.`
          : 'Курсы, которые авторы отправили на проверку перед публикацией.'
      }
      actions={
        <button type="button" className="btn btn-ghost" onClick={() => void queue.refetch()} disabled={queue.isFetching}>
          <Icon name="refresh" size={17} /> Обновить
        </button>
      }
    >
      {queue.isError ? (
        <ErrorNote error={queue.error} onRetry={() => void queue.refetch()} />
      ) : queue.isLoading ? (
        <Spinner />
      ) : items.length === 0 ? (
        <div className="ax-card">
          <EmptyState
            icon="shield"
            title="Очередь пуста — всё проверено"
            text="Когда автор отправит курс на проверку, он появится здесь. Посмотреть все курсы можно в разделе «Курсы»."
            action={
              <Link className="btn btn-ghost" to="/admin/courses">
                Все курсы
              </Link>
            }
          />
        </div>
      ) : (
        <div className="acn-queue">
          {items.map((c) => {
            const busy = ops.busy?.id === c.id;
            const opsCourse = { id: c.id, slug: c.slug, title: c.title, status: c.status, teacherEmail: c.teacherEmail };
            return (
              <article key={c.id} className={`acn-qcard${busy ? ' is-busy' : ''}`}>
                <Link to={`/admin/courses/${c.id}`} className="acn-qcard-cover" tabIndex={-1} aria-hidden="true">
                  <CourseThumb title={c.title} coverPath={c.coverPath} wide />
                </Link>
                <div className="acn-qcard-body">
                  <div className="acn-qcard-top">
                    <h2 className="acn-qcard-title">
                      <Link to={`/admin/courses/${c.id}`}>{c.title}</Link>
                    </h2>
                    <RowMenu label={`Ещё: ${c.title}`} items={ops.menu(opsCourse)} />
                  </div>
                  <p className="acn-qcard-meta">
                    <Link to={`/admin/users/${c.teacherId}`}>{c.teacherEmail || 'Автор'}</Link>
                    <span aria-hidden="true"> · </span>
                    <span title={fmtDateTime(c.updatedAt)}>отправлен {fmtAgo(c.updatedAt)}</span>
                  </p>
                  {c.description && <p className="acn-qcard-desc">{c.description}</p>}
                  <dl className="acn-qcard-facts">
                    <div>
                      <dt>Модулей</dt>
                      <dd className="ax-num">{c.modulesCount ?? '—'}</dd>
                    </div>
                    <div>
                      <dt>Уроков</dt>
                      <dd className="ax-num">{c.lessonsCount ?? '—'}</dd>
                    </div>
                    <div>
                      <dt>Цена</dt>
                      <dd className="ax-num">{formatPrice(c.priceCents)}</dd>
                    </div>
                  </dl>
                  <div className="acn-qcard-links">
                    <a className="ax-link" href={`/courses/${encodeURIComponent(c.slug)}`} target="_blank" rel="noopener noreferrer">
                      <Icon name="eye" size={16} /> Предпросмотр на сайте
                    </a>
                    <Link className="ax-link" to={`/teach/${c.id}`}>
                      <Icon name="edit" size={16} /> Открыть в конструкторе
                    </Link>
                  </div>
                  <div className="acn-qcard-actions">
                    <button type="button" className="btn ax-btn-danger" disabled={busy} onClick={() => ops.reject(opsCourse)}>
                      <Icon name="x" size={17} /> Отклонить
                    </button>
                    <button type="button" className="btn btn-primary" disabled={busy} onClick={() => ops.approve(opsCourse)}>
                      <Icon name="check" size={17} /> Одобрить и опубликовать
                    </button>
                  </div>
                </div>
              </article>
            );
          })}
        </div>
      )}
      {ops.dialogs}
    </AdminPage>
  );
}
