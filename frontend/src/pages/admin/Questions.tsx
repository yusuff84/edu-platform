import { useState } from 'react';
import { Link } from 'react-router-dom';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { answerQuestion, deleteQuestion, listAdminQuestions, overviewQueryKey, type AdminQuestion } from '../../api/adminHub';
import {
  contentKeys,
  deleteQuestionAnswer,
  listCourseOptions,
  pageOf,
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
  Icon,
  Pager,
  RowMenu,
  Segmented,
  Spinner,
  Toolbar,
  errorText,
  fmtAgo,
  fmtDateTime,
  useAdminUi,
} from '../../components/admin/ui';
import '../../styles/admin-content.css';

const LIMIT = 20;

/** Карточка вопроса ученика: контекст, ответы, ответ прямо в карточке, удаление. */
export function QuestionCard({ question, showCourse = true }: { question: AdminQuestion; showCourse?: boolean }): JSX.Element {
  const qc = useQueryClient();
  const { confirm, toast } = useAdminUi();
  const [text, setText] = useState('');
  const [touched, setTouched] = useState(false);
  const refresh = (): void => {
    void qc.invalidateQueries({ queryKey: contentKeys.questions });
    void qc.invalidateQueries({ queryKey: overviewQueryKey });
  };

  const answer = useMutation({
    mutationFn: () => answerQuestion(question.courseId, question.id, text.trim()),
    onSuccess: () => {
      setText('');
      setTouched(false);
      toast('Ответ опубликован — ученик увидит его под уроком');
      refresh();
    },
    onError: (e) => toast(errorText(e), 'error'),
  });
  const remove = useMutation({
    mutationFn: () => deleteQuestion(question.courseId, question.id),
    onSuccess: () => {
      toast('Вопрос удалён');
      refresh();
    },
    onError: (e) => toast(errorText(e), 'error'),
  });
  const removeAnswer = useMutation({
    mutationFn: (answerId: string) => deleteQuestionAnswer(question.courseId, question.id, answerId),
    onSuccess: () => {
      toast('Ответ удалён');
      refresh();
    },
    onError: (e) => toast(errorText(e), 'error'),
  });

  const len = text.trim().length;
  const invalid = len < 3 || len > 2000;
  const name = question.studentName || question.studentEmail || 'Ученик';
  const answers = question.answers ?? [];
  const busy = answer.isPending || remove.isPending || removeAnswer.isPending;

  async function onDelete(): Promise<void> {
    const ok = await confirm({
      title: 'Удалить вопрос?',
      text: 'Вопрос и все ответы на него исчезнут из урока. Это нельзя отменить.',
      confirmLabel: 'Удалить вопрос',
      danger: true,
      typeToConfirm: 'удалить',
    });
    if (ok) remove.mutate();
  }

  async function onDeleteAnswer(id: string): Promise<void> {
    const ok = await confirm({
      title: 'Удалить ответ?',
      text: 'Ответ исчезнет из обсуждения урока. Это нельзя отменить.',
      confirmLabel: 'Удалить ответ',
      danger: true,
    });
    if (ok) removeAnswer.mutate(id);
  }

  return (
    <article className={`acn-item${busy ? ' is-busy' : ''}`}>
      <header className="acn-item-head">
        <div className="acn-item-who">
          <Avatar name={name} size={36} />
          <span className="ax-person-text">
            <Link className="acn-strong-link" to={`/admin/users/${question.studentId}`}>
              {name}
            </Link>
            <small>
              {question.studentName && question.studentEmail ? `${question.studentEmail} · ` : ''}
              <time dateTime={question.createdAt} title={fmtDateTime(question.createdAt)}>
                {fmtAgo(question.createdAt)}
              </time>
            </small>
          </span>
        </div>
        <div className="acn-item-side">
          {question.answered ? <Badge tone="ok">Есть ответ</Badge> : <Badge tone="warn">Без ответа</Badge>}
          <RowMenu
            label="Действия с вопросом"
            items={[
              { label: 'Карточка курса', icon: 'book', to: `/admin/courses/${question.courseId}` },
              { label: 'Открыть урок', icon: 'external', href: `/learn/${question.courseId}/${question.lessonId}` },
              'divider',
              { label: 'Удалить вопрос', icon: 'trash', danger: true, onClick: () => void onDelete() },
            ]}
          />
        </div>
      </header>

      <p className="acn-item-ctx">
        {showCourse && (
          <>
            <Link to={`/admin/courses/${question.courseId}`}>{question.courseTitle}</Link>
            <span aria-hidden="true"> → </span>
          </>
        )}
        <span>{question.lessonTitle || 'Урок'}</span>
      </p>

      <p className="acn-item-body">{question.body}</p>

      {answers.length > 0 && (
        <ul className="acn-answers" aria-label="Ответы">
          {answers.map((a) => (
            <li key={a.id} className={a.isTeacher ? 'is-teacher' : undefined}>
              <div className="acn-answer-head">
                <span className="acn-answer-who">
                  <strong>{a.authorName || 'Без имени'}</strong>
                  {a.isTeacher && <Badge tone="brand">преподаватель</Badge>}
                  <small className="ax-muted" title={fmtDateTime(a.createdAt)}>
                    {fmtAgo(a.createdAt)}
                  </small>
                </span>
                <button
                  type="button"
                  className="ax-icon-btn acn-answer-del"
                  aria-label="Удалить ответ"
                  title="Удалить ответ"
                  disabled={busy}
                  onClick={() => void onDeleteAnswer(a.id)}
                >
                  <Icon name="trash" size={16} />
                </button>
              </div>
              <p>{a.body}</p>
            </li>
          ))}
        </ul>
      )}

      <form
        className="acn-answer-form"
        onSubmit={(e) => {
          e.preventDefault();
          setTouched(true);
          if (!invalid) answer.mutate();
        }}
      >
        <label className="sr-only" htmlFor={`qa-${question.id}`}>
          Ваш ответ
        </label>
        <textarea
          id={`qa-${question.id}`}
          className="ax-textarea"
          rows={2}
          maxLength={2000}
          placeholder={answers.length ? 'Дополнить ответ…' : 'Ответить ученику…'}
          value={text}
          onChange={(e) => setText(e.target.value)}
        />
        {touched && invalid && <span className="ax-field-error">Ответ — от 3 до 2000 символов</span>}
        <div className="acn-answer-actions">
          <span className="ax-muted acn-hint">Ответ появится под уроком с пометкой «преподаватель».</span>
          <button type="submit" className="btn btn-primary btn-sm" disabled={answer.isPending || len === 0}>
            {answer.isPending ? 'Отправляем…' : 'Ответить'}
          </button>
        </div>
      </form>
    </article>
  );
}

type Status = 'unanswered' | 'answered' | 'all';

/** Лента вопросов с фильтрами (страница «Вопросы» и вкладка курса). */
export function QuestionsBoard({ mode, courseId }: { mode: 'url' | 'local'; courseId?: string }): JSX.Element {
  const f = useFilterState(mode);
  const statusRaw = f.get('status');
  const status: Status = statusRaw === 'answered' || statusRaw === 'all' ? statusRaw : 'unanswered';
  const course = courseId ?? f.get('course');
  const page = pageOf(f.get('page'));

  const courseOptions = useQuery({
    queryKey: contentKeys.courseOptions,
    queryFn: listCourseOptions,
    enabled: !courseId,
    staleTime: 60_000,
  });
  const list = useQuery({
    queryKey: [...contentKeys.questions, status, course, page],
    queryFn: () => listAdminQuestions({ status, courseId: course || undefined, page, limit: LIMIT }),
    placeholderData: keepPreviousData,
  });
  // Общее число вопросов (для счётчиков «С ответом» и «Все»).
  const totals = useQuery({
    queryKey: [...contentKeys.questions, 'all-count', course],
    queryFn: () => listAdminQuestions({ status: 'all', courseId: course || undefined, page: 1, limit: 1 }),
  });
  const allCount = totals.data?.total;
  const unanswered = list.data?.unanswered ?? totals.data?.unanswered;

  const data = list.data;
  return (
    <div className="acn-board">
      <div className="acn-board-filters">
        <Toolbar>
          <Segmented<Status>
            label="Статус вопроса"
            value={status}
            onChange={(v) => f.set({ status: v === 'unanswered' ? null : v, page: null })}
            options={[
              { value: 'unanswered', label: 'Без ответа', count: unanswered },
              { value: 'answered', label: 'С ответом', count: allCount !== undefined && unanswered !== undefined ? allCount - unanswered : undefined },
              { value: 'all', label: 'Все', count: allCount },
            ]}
          />
          {!courseId && (
            <FilterSelect
              label="Курс"
              value={course}
              onChange={(v) => f.set({ course: v, page: null })}
              options={[{ value: '', label: 'Все курсы' }, ...(courseOptions.data ?? []).map((c) => ({ value: c.id, label: c.title }))]}
            />
          )}
        </Toolbar>
      </div>

      {list.isError ? (
        <ErrorNote error={list.error} onRetry={() => void list.refetch()} />
      ) : !data ? (
        <Spinner />
      ) : data.items.length === 0 ? (
        status === 'unanswered' ? (
          <EmptyState
            icon="check"
            title="Все вопросы отвечены"
            text={allCount ? 'Новых вопросов без ответа нет. Отвеченные — на вкладке «С ответом».' : 'Ученики пока не задавали вопросов к урокам.'}
          />
        ) : (
          <EmptyState icon="message" title="Вопросов нет" text="Когда ученики зададут вопросы к урокам, они появятся здесь." />
        )
      ) : (
        <>
          <div className={`acn-list${list.isFetching && list.isPlaceholderData ? ' acn-fetching' : ''}`}>
            {data.items.map((qq) => (
              <QuestionCard key={qq.id} question={qq} showCourse={!courseId} />
            ))}
          </div>
          <Pager page={page} total={data.total} limit={LIMIT} onPage={(p) => f.set({ page: p > 1 ? String(p) : null })} />
        </>
      )}
    </div>
  );
}

export default function Questions(): JSX.Element {
  return (
    <AdminPage
      title="Вопросы учеников"
      description="Вопросы к урокам во всех курсах. Отвечайте прямо здесь — ответ появится под уроком."
    >
      <Card>
        <QuestionsBoard mode="url" />
      </Card>
    </AdminPage>
  );
}
