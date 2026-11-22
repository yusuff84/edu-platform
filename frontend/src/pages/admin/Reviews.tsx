import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { deleteReview } from '../../api/adminConsole';
import { clearReviewReply, listAdminReviews, replyToReview, type AdminReview } from '../../api/adminHub';
import {
  contentKeys,
  listAllAdminReviews,
  listCourseOptions,
  pageOf,
  useDebouncedInput,
  useFilterState,
} from '../../api/adminContent';
import {
  AdminPage,
  Avatar,
  Badge,
  Card,
  EmptyState,
  ErrorNote,
  FilterSelect,
  Note,
  Pager,
  RowMenu,
  SearchInput,
  Segmented,
  Spinner,
  Switch,
  Toolbar,
  errorText,
  fmtAgo,
  fmtDate,
  fmtDateTime,
  useAdminUi,
} from '../../components/admin/ui';
import '../../styles/admin-content.css';

const LIMIT = 20;

function Stars({ rating }: { rating: number }): JSX.Element {
  const r = Math.max(0, Math.min(5, Math.round(rating)));
  return (
    <span className={`acn-stars acn-stars--${r}`} role="img" aria-label={`Оценка ${r} из 5`}>
      <span aria-hidden="true">{'★'.repeat(r)}</span>
      <span aria-hidden="true" className="acn-stars-off">
        {'★'.repeat(5 - r)}
      </span>
    </span>
  );
}

/** Карточка отзыва: оценка, текст, автор, курс; публичный ответ; удаление. */
function ReviewCard({ review, showCourse = true }: { review: AdminReview; showCourse?: boolean }): JSX.Element {
  const qc = useQueryClient();
  const { confirm, toast } = useAdminUi();
  const [editing, setEditing] = useState(false);
  const [text, setText] = useState(review.teacherReply ?? '');
  const [touched, setTouched] = useState(false);
  const refresh = (): void => void qc.invalidateQueries({ queryKey: ['admin'] });

  const save = useMutation({
    mutationFn: () => replyToReview(review.courseId, review.id, text.trim()),
    onSuccess: () => {
      setEditing(false);
      setTouched(false);
      toast(review.teacherReply ? 'Ответ обновлён' : 'Ответ опубликован');
      refresh();
    },
    onError: (e) => toast(errorText(e), 'error'),
  });
  const clear = useMutation({
    mutationFn: () => clearReviewReply(review.courseId, review.id),
    onSuccess: () => {
      setText('');
      toast('Ответ удалён');
      refresh();
    },
    onError: (e) => toast(errorText(e), 'error'),
  });
  const remove = useMutation({
    mutationFn: () => deleteReview(review.id),
    onSuccess: () => {
      toast('Отзыв удалён, рейтинг курса пересчитан');
      refresh();
    },
    onError: (e) => toast(errorText(e), 'error'),
  });

  const len = text.trim().length;
  const invalid = len < 3 || len > 2000;
  const name = review.studentName || review.studentEmail || 'Ученик';
  const busy = save.isPending || clear.isPending || remove.isPending;

  async function onDelete(): Promise<void> {
    const ok = await confirm({
      title: 'Удалить отзыв?',
      text: (
        <>
          Отзыв {review.studentEmail ? <b>{review.studentEmail}</b> : 'ученика'} с оценкой {review.rating} исчезнет со страницы
          курса, рейтинг пересчитается. Это нельзя отменить.
        </>
      ),
      confirmLabel: 'Удалить отзыв',
      danger: true,
      typeToConfirm: 'удалить',
    });
    if (ok) remove.mutate();
  }

  async function onClear(): Promise<void> {
    const ok = await confirm({
      title: 'Удалить публичный ответ?',
      text: 'Ответ пропадёт со страницы курса. Сам отзыв останется.',
      confirmLabel: 'Удалить ответ',
      danger: true,
    });
    if (ok) clear.mutate();
  }

  return (
    <article className={`acn-item${busy ? ' is-busy' : ''}`}>
      <header className="acn-item-head">
        <div className="acn-item-who">
          <Avatar name={name} size={36} />
          <span className="ax-person-text">
            <Link className="acn-strong-link" to={`/admin/users/${review.studentId}`}>
              {name}
            </Link>
            <small>
              {review.studentName && review.studentEmail ? `${review.studentEmail} · ` : ''}
              <time dateTime={review.createdAt} title={fmtDateTime(review.createdAt)}>
                {fmtAgo(review.createdAt)}
              </time>
            </small>
          </span>
        </div>
        <RowMenu
          label="Действия с отзывом"
          items={[
            { label: review.teacherReply ? 'Изменить ответ' : 'Ответить публично', icon: 'message', onClick: () => setEditing(true) },
            { label: 'Карточка курса', icon: 'book', to: `/admin/courses/${review.courseId}` },
            { label: 'Отзывы на странице курса', icon: 'external', href: `/courses/${encodeURIComponent(review.courseSlug)}#reviews` },
            'divider',
            { label: 'Удалить отзыв', icon: 'trash', danger: true, onClick: () => void onDelete() },
          ]}
        />
      </header>

      <div className="acn-review-rating">
        <Stars rating={review.rating} />
        {review.rating <= 3 && !review.teacherReply && <Badge tone="danger">Низкая оценка без ответа</Badge>}
      </div>

      {showCourse && (
        <p className="acn-item-ctx">
          <Link to={`/admin/courses/${review.courseId}`}>{review.courseTitle}</Link>
        </p>
      )}

      {review.body ? <p className="acn-item-body">{review.body}</p> : <p className="acn-item-body ax-muted">Оценка без текста</p>}

      {review.teacherReply && !editing && (
        <div className="acn-reply">
          <div className="acn-reply-head">
            <span>
              <Badge tone="brand">Публичный ответ</Badge>
              {review.teacherReplyAt && <small className="ax-muted"> {fmtDate(review.teacherReplyAt)}</small>}
            </span>
            <span className="acn-reply-actions">
              <button
                type="button"
                className="ax-link"
                onClick={() => {
                  setText(review.teacherReply);
                  setEditing(true);
                }}
              >
                Изменить
              </button>
              <button type="button" className="ax-link acn-link-danger" onClick={() => void onClear()} disabled={busy}>
                Удалить
              </button>
            </span>
          </div>
          <p>{review.teacherReply}</p>
        </div>
      )}

      {editing ? (
        <form
          className="acn-answer-form"
          onSubmit={(e) => {
            e.preventDefault();
            setTouched(true);
            if (!invalid) save.mutate();
          }}
        >
          <label className="sr-only" htmlFor={`rv-${review.id}`}>
            Публичный ответ на отзыв
          </label>
          <textarea
            id={`rv-${review.id}`}
            className="ax-textarea"
            rows={3}
            autoFocus
            maxLength={2000}
            placeholder="Ответ увидят все посетители страницы курса"
            value={text}
            onChange={(e) => setText(e.target.value)}
          />
          {touched && invalid && <span className="ax-field-error">Ответ — от 3 до 2000 символов</span>}
          <div className="acn-answer-actions">
            <button
              type="button"
              className="btn btn-ghost btn-sm"
              onClick={() => {
                setEditing(false);
                setTouched(false);
                setText(review.teacherReply ?? '');
              }}
            >
              Отмена
            </button>
            <button type="submit" className="btn btn-primary btn-sm" disabled={save.isPending}>
              {save.isPending ? 'Сохраняем…' : review.teacherReply ? 'Сохранить ответ' : 'Опубликовать ответ'}
            </button>
          </div>
        </form>
      ) : (
        !review.teacherReply && (
          <button type="button" className="btn btn-ghost btn-sm acn-reply-btn" onClick={() => setEditing(true)}>
            Ответить публично
          </button>
        )
      )}
    </article>
  );
}

type RatingFilter = '' | '5' | '4' | '3' | '2' | '1' | 'low';
const RATINGS: RatingFilter[] = ['', '5', '4', '3', '2', '1', 'low'];

/**
 * Лента отзывов с фильтрами. На странице «Отзывы» фильтры в адресе,
 * во вкладке карточки курса — локальные (курс зафиксирован).
 */
export function ReviewsBoard({ mode, courseId }: { mode: 'url' | 'local'; courseId?: string }): JSX.Element {
  const f = useFilterState(mode);

  // Ссылка с «Обзора»: ?filter=low → оценки 1–3 без ответа.
  const legacyLow = f.get('filter') === 'low';
  useEffect(() => {
    if (legacyLow) f.set({ filter: null, rating: 'low', noReply: '1', page: null }, { replace: true });
  }, [legacyLow, f]);

  const ratingRaw = legacyLow ? 'low' : f.get('rating');
  const rating: RatingFilter = (RATINGS as string[]).includes(ratingRaw) ? (ratingRaw as RatingFilter) : '';
  const noReply = legacyLow || f.get('noReply') === '1';
  const course = courseId ?? f.get('course');
  const q = f.get('q');
  const page = pageOf(f.get('page'));
  const [qInput, setQInput] = useDebouncedInput(q, (v) => f.set({ q: v, page: null }));

  const courseOptions = useQuery({
    queryKey: contentKeys.courseOptions,
    queryFn: listCourseOptions,
    enabled: !courseId,
    staleTime: 60_000,
  });

  const base = { q: q || undefined, courseId: course || undefined, noReply: noReply ? (1 as const) : undefined };
  const low = rating === 'low';
  const paged = useQuery({
    queryKey: [...contentKeys.reviews, 'page', base, rating, page],
    queryFn: () => listAdminReviews({ ...base, rating: rating ? Number(rating) : undefined, page, limit: LIMIT }),
    enabled: !low,
    placeholderData: keepPreviousData,
  });
  const all = useQuery({
    queryKey: [...contentKeys.reviews, 'all', base],
    queryFn: () => listAllAdminReviews(base),
    enabled: low,
  });

  const view = useMemo(() => {
    if (low) {
      if (!all.data) return null;
      const items = all.data.filter((r) => r.rating <= 3);
      return { items: items.slice((page - 1) * LIMIT, page * LIMIT), total: items.length };
    }
    return paged.data ? { items: paged.data.items, total: paged.data.total } : null;
  }, [low, all.data, paged.data, page]);
  const active = low ? all : paged;
  const filtered = Boolean(rating || noReply || q || (!courseId && course));

  const ratingOptions: { value: RatingFilter; label: string }[] = [
    { value: '', label: 'Все' },
    { value: '5', label: '5 ★' },
    { value: '4', label: '4 ★' },
    { value: '3', label: '3 ★' },
    { value: '2', label: '2 ★' },
    { value: '1', label: '1 ★' },
    { value: 'low', label: 'Низкие (1–3)' },
  ];

  return (
    <div className="acn-board">
      <div className="acn-board-filters">
        <Segmented label="Оценка" value={rating} onChange={(v) => f.set({ rating: v, page: null })} options={ratingOptions} />
        <Toolbar
          end={
            filtered ? (
              <button type="button" className="btn btn-ghost" onClick={() => f.set({ rating: null, noReply: null, q: null, course: null, page: null })}>
                Сбросить
              </button>
            ) : undefined
          }
        >
          <SearchInput value={qInput} onChange={setQInput} placeholder="Текст, ученик или курс" label="Поиск по отзывам" />
          {!courseId && (
            <FilterSelect
              label="Курс"
              value={course}
              onChange={(v) => f.set({ course: v, page: null })}
              options={[{ value: '', label: 'Все курсы' }, ...(courseOptions.data ?? []).map((c) => ({ value: c.id, label: c.title }))]}
            />
          )}
          <div className="acn-switch">
            <Switch checked={noReply} onChange={(v) => f.set({ noReply: v ? '1' : null, page: null })} label="Только без ответа" />
          </div>
        </Toolbar>
        {low && noReply && (
          <Note tone="warn">Показаны оценки от 1 до 3 без ответа — на них стоит ответить в первую очередь.</Note>
        )}
      </div>

      {active.isError ? (
        <ErrorNote error={active.error} onRetry={() => void active.refetch()} />
      ) : !view ? (
        <Spinner />
      ) : view.items.length === 0 ? (
        filtered ? (
          <EmptyState icon="search" title="Таких отзывов нет" text="Попробуйте изменить фильтры." />
        ) : (
          <EmptyState icon="star" title="Отзывов пока нет" text="Когда ученики оценят курс, отзывы появятся здесь." />
        )
      ) : (
        <>
          <div className={`acn-list${active.isFetching && !active.isLoading ? ' acn-fetching' : ''}`}>
            {view.items.map((r) => (
              <ReviewCard key={`${r.id}:${r.teacherReply}`} review={r} showCourse={!courseId} />
            ))}
          </div>
          <Pager page={page} total={view.total} limit={LIMIT} onPage={(p) => f.set({ page: p > 1 ? String(p) : null })} />
        </>
      )}
    </div>
  );
}

export default function Reviews(): JSX.Element {
  return (
    <AdminPage
      title="Отзывы"
      description="Отзывы учеников по всем курсам. Отвечайте публично от имени школы и убирайте недопустимые отзывы."
    >
      <Card>
        <ReviewsBoard mode="url" />
      </Card>
    </AdminPage>
  );
}
