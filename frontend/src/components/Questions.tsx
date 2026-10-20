// T72: блок «Вопросы и ответы» под уроком.
//
// Список вопросов публичный (GET /courses/{id}/questions?lessonId=...), поэтому
// он грузится и для гостя. Формы показаны только вошедшим: для гостя — текст
// «Войдите…», для вошедшего без записи — «Записывайтесь…». Признак записи
// страница обучения уже знает из myCourses() и передаёт пропом enrolled;
// если он не передан — форму показываем, а права окончательно проверяет сервер
// (403 с текстом выводится как ApiError.message).
//
// После создания/удаления инвалидируем запрос списка — счётчик и ответы
// обновляются с сервера, локальных копий не держим.

import { useEffect, useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  ApiError,
  answerQuestion,
  askQuestion,
  deleteAnswer,
  deleteQuestion,
  editAnswer,
  editQuestion,
  formatDate,
  listLessonQuestions,
  me,
} from '../api/client';
import type { LessonQuestion } from '../api/client';
import { t } from '../i18n/t';
import { pluralRu } from '../i18n/plural';
import '../styles/qa.css';

/** Тексты валидации: отдельных ключей словаря нет — литералы по контракту T71. */
const QUESTION_LENGTH_ERROR = 'Вопрос: от 10 до 2000 символов';
const ANSWER_LENGTH_ERROR = 'Ответ: от 3 до 2000 символов';

/** Текст ошибки: сервер отдаёт причину в ApiError.message. */
function errorText(e: unknown): string {
  if (e instanceof ApiError) return e.message || t('common.error');
  if (e instanceof Error) return e.message || t('common.error');
  return t('common.error');
}

export default function Questions({
  courseId,
  lessonId,
  enrolled,
}: {
  courseId: string;
  lessonId: string;
  /**
   * Признак активной записи из уже загруженных данных страницы обучения.
   * undefined — данных нет, форму показываем (сервер ответит 403 при отказе).
   */
  enrolled?: boolean;
}): JSX.Element {
  const qc = useQueryClient();

  const [page, setPage] = useState(1);
  const [questionDraft, setQuestionDraft] = useState('');
  const [questionError, setQuestionError] = useState('');
  const [answerForId, setAnswerForId] = useState<string | null>(null);
  const [answerDraft, setAnswerDraft] = useState('');
  const [answerError, setAnswerError] = useState('');
  const [editingQuestionId, setEditingQuestionId] = useState<string | null>(null);
  const [questionEditDraft, setQuestionEditDraft] = useState('');
  const [questionEditError, setQuestionEditError] = useState('');
  const [editingAnswer, setEditingAnswer] = useState<{
    questionId: string;
    answerId: string;
  } | null>(null);
  const [answerEditDraft, setAnswerEditDraft] = useState('');
  const [answerEditError, setAnswerEditError] = useState('');
  const [notice, setNotice] = useState('');
  const [actionError, setActionError] = useState('');

  // Текущий пользователь: ключ ['me'] общий с шапкой и отзывами — кеш делится.
  const meQuery = useQuery({ queryKey: ['me'], queryFn: me, retry: false });

  const questionsQuery = useQuery({
    queryKey: ['lesson-questions', courseId, lessonId, page],
    queryFn: () => listLessonQuestions(courseId, lessonId, page),
    retry: false,
  });

  // Инвалидация всего списка курса: после создания/удаления перечитываются
  // и текущая страница, и счётчик.
  const invalidateQuestions = (): void => {
    void qc.invalidateQueries({ queryKey: ['lesson-questions', courseId] });
  };

  const askMutation = useMutation({
    mutationFn: (body: string) => askQuestion(courseId, lessonId, body),
    onSuccess: () => {
      setQuestionDraft('');
      setQuestionError('');
      setActionError('');
      setNotice(t('p9.qa.sent'));
      // Новый вопрос — первым на первой странице.
      setPage(1);
      invalidateQuestions();
    },
    onError: (e: unknown) => {
      setNotice('');
      setQuestionError(errorText(e));
    },
  });

  const answerMutation = useMutation({
    mutationFn: (v: { questionId: string; body: string }) =>
      answerQuestion(courseId, v.questionId, v.body),
    onSuccess: () => {
      setAnswerForId(null);
      setAnswerDraft('');
      setAnswerError('');
      setActionError('');
      setNotice(t('p9.qa.answerSent'));
      invalidateQuestions();
    },
    onError: (e: unknown) => {
      setNotice('');
      setAnswerError(errorText(e));
    },
  });

  const editQuestionMutation = useMutation({
    mutationFn: (v: { questionId: string; body: string }) =>
      editQuestion(courseId, v.questionId, v.body),
    onSuccess: () => {
      setEditingQuestionId(null);
      setQuestionEditDraft('');
      setQuestionEditError('');
      setActionError('');
      setNotice(t('p9.qa.edited'));
      invalidateQuestions();
    },
    onError: (e: unknown) => {
      setNotice('');
      setQuestionEditError(errorText(e));
    },
  });

  const editAnswerMutation = useMutation({
    mutationFn: (v: { questionId: string; answerId: string; body: string }) =>
      editAnswer(courseId, v.questionId, v.answerId, v.body),
    onSuccess: () => {
      setEditingAnswer(null);
      setAnswerEditDraft('');
      setAnswerEditError('');
      setActionError('');
      setNotice(t('p9.qa.answerEdited'));
      invalidateQuestions();
    },
    onError: (e: unknown) => {
      setNotice('');
      setAnswerEditError(errorText(e));
    },
  });

  const deleteQuestionMutation = useMutation({
    mutationFn: (questionId: string) => deleteQuestion(courseId, questionId),
    onSuccess: () => {
      setActionError('');
      setNotice(t('p9.qa.deleted'));
      invalidateQuestions();
    },
    onError: (e: unknown) => {
      setNotice('');
      setActionError(errorText(e));
    },
  });

  const deleteAnswerMutation = useMutation({
    mutationFn: (v: { questionId: string; answerId: string }) =>
      deleteAnswer(courseId, v.questionId, v.answerId),
    onSuccess: () => {
      setActionError('');
      setAnswerError('');
      invalidateQuestions();
    },
    onError: (e: unknown) => {
      setNotice('');
      setActionError(errorText(e));
    },
  });

  // Смена урока — сбрасываем страницу и черновики (компонент может не
  // перемонтироваться, если его встроят без key).
  useEffect(() => {
    setPage(1);
    setQuestionDraft('');
    setQuestionError('');
    setAnswerForId(null);
    setAnswerDraft('');
    setAnswerError('');
    setEditingQuestionId(null);
    setQuestionEditDraft('');
    setQuestionEditError('');
    setEditingAnswer(null);
    setAnswerEditDraft('');
    setAnswerEditError('');
    setNotice('');
    setActionError('');
  }, [courseId, lessonId]);

  const data = questionsQuery.data;
  const questions = data?.items ?? [];
  const total = data?.total ?? 0;
  const limit = data?.limit && data.limit > 0 ? data.limit : 20;
  const pages = Math.max(1, Math.ceil(total / limit));

  // После удаления последнего вопроса страницы может не стать — уходим назад.
  useEffect(() => {
    if (page > pages) setPage(pages);
  }, [page, pages]);

  const user = meQuery.data;
  // Ответы и вопросы можно писать только вошедшим; запись проверяет сервер.
  const canWrite = Boolean(user) && enrolled !== false;
  const canAnswer = canWrite;

  const submitQuestion = (e: FormEvent<HTMLFormElement>): void => {
    e.preventDefault();
    const body = questionDraft.trim();
    if (body.length < 10 || body.length > 2000) {
      setQuestionError(QUESTION_LENGTH_ERROR);
      return;
    }
    setQuestionError('');
    setNotice('');
    askMutation.mutate(body);
  };

  const openAnswer = (questionId: string): void => {
    setAnswerForId(questionId);
    setAnswerDraft('');
    setAnswerError('');
    setNotice('');
    setActionError('');
  };

  const cancelAnswer = (): void => {
    setAnswerForId(null);
    setAnswerDraft('');
    setAnswerError('');
  };

  const submitAnswer = (
    e: FormEvent<HTMLFormElement>,
    questionId: string,
  ): void => {
    e.preventDefault();
    const body = answerDraft.trim();
    if (body.length < 3 || body.length > 2000) {
      setAnswerError(ANSWER_LENGTH_ERROR);
      return;
    }
    setAnswerError('');
    setNotice('');
    answerMutation.mutate({ questionId, body });
  };

  const startQuestionEdit = (q: LessonQuestion): void => {
    setEditingQuestionId(q.id);
    setQuestionEditDraft(q.body);
    setQuestionEditError('');
    setEditingAnswer(null);
    setAnswerEditError('');
    setAnswerForId(null);
    setAnswerError('');
    setNotice('');
    setActionError('');
  };

  const cancelQuestionEdit = (): void => {
    setEditingQuestionId(null);
    setQuestionEditDraft('');
    setQuestionEditError('');
  };

  const submitQuestionEdit = (
    e: FormEvent<HTMLFormElement>,
    questionId: string,
  ): void => {
    e.preventDefault();
    const body = questionEditDraft.trim();
    if (body.length < 10 || body.length > 2000) {
      setQuestionEditError(QUESTION_LENGTH_ERROR);
      return;
    }
    setQuestionEditError('');
    setNotice('');
    editQuestionMutation.mutate({ questionId, body });
  };

  const startAnswerEdit = (
    questionId: string,
    answerId: string,
    body: string,
  ): void => {
    setEditingAnswer({ questionId, answerId });
    setAnswerEditDraft(body);
    setAnswerEditError('');
    setEditingQuestionId(null);
    setQuestionEditError('');
    setAnswerForId(null);
    setAnswerError('');
    setNotice('');
    setActionError('');
  };

  const cancelAnswerEdit = (): void => {
    setEditingAnswer(null);
    setAnswerEditDraft('');
    setAnswerEditError('');
  };

  const submitAnswerEdit = (
    e: FormEvent<HTMLFormElement>,
    questionId: string,
    answerId: string,
  ): void => {
    e.preventDefault();
    const body = answerEditDraft.trim();
    if (body.length < 3 || body.length > 2000) {
      setAnswerEditError(ANSWER_LENGTH_ERROR);
      return;
    }
    setAnswerEditError('');
    setNotice('');
    editAnswerMutation.mutate({ questionId, answerId, body });
  };

  const removeQuestion = (questionId: string): void => {
    if (!window.confirm(t('p9.qa.deleteConfirm'))) return;
    setNotice('');
    setActionError('');
    deleteQuestionMutation.mutate(questionId);
  };

  const removeAnswer = (questionId: string, answerId: string): void => {
    if (!window.confirm(`${t('p9.qa.delete')}?`)) return;
    setNotice('');
    setActionError('');
    deleteAnswerMutation.mutate({ questionId, answerId });
  };

  return (
    <section className="panel qa" aria-label={t('p9.qa.title')}>
      <div className="qa-head">
        <h2>{t('p9.qa.title')}</h2>
        {data && (
          <span className="qa-count">
            {total}{' '}
            {pluralRu(total, t('plural.question.one'), t('plural.question.few'), t('plural.question.many'))}
          </span>
        )}
      </div>

      {notice && (
        <p className="qa-notice" role="status" aria-live="polite">
          {notice}
        </p>
      )}
      {actionError && (
        <p className="error" role="alert">
          {actionError}
        </p>
      )}

      {/* Форма вопроса: гость → вход, вошедший без записи → запись. */}
      {!meQuery.isLoading && !user && (
        <p className="muted">
          {t('p9.qa.loginToAsk')} — <Link to="/login">{t('nav.login')}</Link>
        </p>
      )}
      {user && enrolled === false && (
        <p className="muted">{t('p9.qa.enrollToAsk')}</p>
      )}
      {canWrite && (
        <div className="qa-ask">
          <h3>{t('p9.qa.ask')}</h3>
          <form className="qa-form" onSubmit={submitQuestion}>
            <label className="field" htmlFor="qa-question-text">
              <span>{t('p9.qa.askTitle')}</span>
              <textarea
                id="qa-question-text"
                className="input"
                rows={4}
                maxLength={2000}
                placeholder={t('p9.qa.placeholder')}
                value={questionDraft}
                aria-invalid={Boolean(questionError)}
                aria-describedby={questionError ? 'qa-question-error' : undefined}
                onChange={(e) => {
                  setQuestionDraft(e.target.value);
                  if (questionError) setQuestionError('');
                }}
              />
            </label>
            <div className="qa-form-foot">
              <span
                className={`qa-counter${questionDraft.length > 2000 ? ' over' : ''}`}
              >
                {questionDraft.length}/2000
              </span>
              <button
                type="submit"
                className="btn btn-primary"
                disabled={askMutation.isPending}
              >
                {t('p9.qa.send')}
              </button>
            </div>
            {questionError && (
              <p className="err" id="qa-question-error" role="alert">
                {questionError}
              </p>
            )}
          </form>
        </div>
      )}

      {questionsQuery.isLoading && <p role="status">{t('common.loading')}</p>}
      {questionsQuery.isError && (
        <>
          <p className="error" role="alert">
            {errorText(questionsQuery.error)}
          </p>
          <button
            type="button"
            className="btn"
            onClick={() => void questionsQuery.refetch()}
          >
            {t('common.retry')}
          </button>
        </>
      )}

      {questionsQuery.isSuccess && questions.length === 0 && (
        <div className="qa-empty">
          <p>
            <strong>{t('p9.qa.empty')}</strong>
          </p>
          <p className="muted">{t('p9.qa.emptyHint')}</p>
        </div>
      )}

      {questions.length > 0 && (
        <ol className="qa-list">
          {questions.map((q: LessonQuestion) => {
            const mine = Boolean(user) && user?.id === q.studentId;
            return (
              <li key={q.id} className="qa-question">
                <div className="qa-question-head">
                  <span className="qa-author">{q.studentName || '—'}</span>
                  <span className="muted qa-date">{formatDate(q.createdAt)}</span>
                  {mine && editingQuestionId !== q.id && (
                    <button
                      type="button"
                      className="btn btn-sm qa-edit"
                      onClick={() => startQuestionEdit(q)}
                      disabled={editQuestionMutation.isPending}
                      aria-label={`${t('p9.qa.edit')}: ${q.body}`}
                    >
                      {t('p9.qa.edit')}
                    </button>
                  )}
                  {mine && (
                    <button
                      type="button"
                      className="btn btn-sm qa-delete"
                      onClick={() => removeQuestion(q.id)}
                      disabled={deleteQuestionMutation.isPending}
                      aria-label={`${t('p9.qa.delete')}: ${q.body}`}
                    >
                      {t('p9.qa.delete')}
                    </button>
                  )}
                </div>
                {editingQuestionId === q.id ? (
                  <form
                    className="qa-form qa-edit-form"
                    onSubmit={(e) => submitQuestionEdit(e, q.id)}
                  >
                    <label className="field" htmlFor={`qa-question-edit-${q.id}`}>
                      <span>{t('p9.qa.edit')}</span>
                      <textarea
                        id={`qa-question-edit-${q.id}`}
                        className="input"
                        rows={4}
                        maxLength={2000}
                        value={questionEditDraft}
                        aria-invalid={Boolean(questionEditError)}
                        aria-describedby={
                          questionEditError
                            ? `qa-question-edit-error-${q.id}`
                            : undefined
                        }
                        onChange={(e) => {
                          setQuestionEditDraft(e.target.value);
                          if (questionEditError) setQuestionEditError('');
                        }}
                      />
                    </label>
                    <div className="qa-form-foot">
                      <span className="qa-counter">{questionEditDraft.length}/2000</span>
                      <div className="qa-actions">
                        <button
                          type="submit"
                          className="btn btn-primary btn-sm"
                          disabled={editQuestionMutation.isPending}
                        >
                          {t('p9.qa.save')}
                        </button>
                        <button
                          type="button"
                          className="btn btn-sm"
                          onClick={cancelQuestionEdit}
                          disabled={editQuestionMutation.isPending}
                        >
                          {t('p9.qa.cancel')}
                        </button>
                      </div>
                    </div>
                    {questionEditError && (
                      <p
                        className="err"
                        id={`qa-question-edit-error-${q.id}`}
                        role="alert"
                      >
                        {questionEditError}
                      </p>
                    )}
                  </form>
                ) : (
                  <p className="qa-body">{q.body}</p>
                )}

                {q.answers.length > 0 && (
                  <div className="qa-answers">
                    <h3 className="qa-answers-title">
                      {t('p9.qa.answers')}{' '}
                      <span className="qa-answers-count">{q.answers.length}</span>
                    </h3>
                    <ul className="qa-answer-list">
                      {q.answers.map((a) => (
                        <li
                          key={a.id}
                          className={`qa-answer${a.isTeacher ? ' qa-answer-teacher' : ''}`}
                        >
                          <div className="qa-answer-head">
                            <span className="qa-author">
                              {a.authorName || '—'}
                            </span>
                            {a.isTeacher && (
                              <span className="qa-badge">
                                {t('p9.qa.teacherBadge')}
                              </span>
                            )}
                            <span className="muted qa-date">
                              {formatDate(a.createdAt)}
                            </span>
                            {Boolean(user) &&
                              user?.id === a.authorId &&
                              !(editingAnswer?.questionId === q.id &&
                                editingAnswer.answerId === a.id) && (
                                <button
                                  type="button"
                                  className="btn btn-sm qa-edit"
                                  onClick={() =>
                                    startAnswerEdit(q.id, a.id, a.body)
                                  }
                                  disabled={editAnswerMutation.isPending}
                                  aria-label={`${t('p9.qa.edit')}: ${a.body}`}
                                >
                                  {t('p9.qa.edit')}
                                </button>
                              )}
                            {Boolean(user) && user?.id === a.authorId && (
                              <button
                                type="button"
                                className="btn btn-sm qa-delete"
                                onClick={() => removeAnswer(q.id, a.id)}
                                disabled={deleteAnswerMutation.isPending}
                                aria-label={`${t('p9.qa.delete')}: ${a.body}`}
                              >
                                {t('p9.qa.delete')}
                              </button>
                            )}
                          </div>
                          {editingAnswer?.questionId === q.id &&
                          editingAnswer.answerId === a.id ? (
                            <form
                              className="qa-form qa-edit-form"
                              onSubmit={(e) =>
                                submitAnswerEdit(e, q.id, a.id)
                              }
                            >
                              <label
                                className="field"
                                htmlFor={`qa-answer-edit-${a.id}`}
                              >
                                <span>{t('p9.qa.edit')}</span>
                                <textarea
                                  id={`qa-answer-edit-${a.id}`}
                                  className="input"
                                  rows={3}
                                  maxLength={2000}
                                  value={answerEditDraft}
                                  aria-invalid={Boolean(answerEditError)}
                                  aria-describedby={
                                    answerEditError
                                      ? `qa-answer-edit-error-${a.id}`
                                      : undefined
                                  }
                                  onChange={(e) => {
                                    setAnswerEditDraft(e.target.value);
                                    if (answerEditError) setAnswerEditError('');
                                  }}
                                />
                              </label>
                              <div className="qa-form-foot">
                                <span className="qa-counter">
                                  {answerEditDraft.length}/2000
                                </span>
                                <div className="qa-actions">
                                  <button
                                    type="submit"
                                    className="btn btn-primary btn-sm"
                                    disabled={editAnswerMutation.isPending}
                                  >
                                    {t('p9.qa.save')}
                                  </button>
                                  <button
                                    type="button"
                                    className="btn btn-sm"
                                    onClick={cancelAnswerEdit}
                                    disabled={editAnswerMutation.isPending}
                                  >
                                    {t('p9.qa.cancel')}
                                  </button>
                                </div>
                              </div>
                              {answerEditError && (
                                <p
                                  className="err"
                                  id={`qa-answer-edit-error-${a.id}`}
                                  role="alert"
                                >
                                  {answerEditError}
                                </p>
                              )}
                            </form>
                          ) : (
                            <p className="qa-body">{a.body}</p>
                          )}
                        </li>
                      ))}
                    </ul>
                  </div>
                )}

                {canAnswer && answerForId !== q.id && (
                  <button
                    type="button"
                    className="btn btn-sm qa-answer-toggle"
                    onClick={() => openAnswer(q.id)}
                  >
                    {t('p9.qa.answer')}
                  </button>
                )}

                {canAnswer && answerForId === q.id && (
                  <form
                    className="qa-form qa-answer-form"
                    onSubmit={(e) => submitAnswer(e, q.id)}
                  >
                    <label className="field" htmlFor={`qa-answer-${q.id}`}>
                      <span>{t('p9.qa.answer')}</span>
                      <textarea
                        id={`qa-answer-${q.id}`}
                        className="input"
                        rows={3}
                        maxLength={2000}
                        placeholder={t('p9.qa.answerPlaceholder')}
                        value={answerDraft}
                        aria-invalid={Boolean(answerError)}
                        aria-describedby={
                          answerError ? `qa-answer-error-${q.id}` : undefined
                        }
                        onChange={(e) => {
                          setAnswerDraft(e.target.value);
                          if (answerError) setAnswerError('');
                        }}
                      />
                    </label>
                    <div className="qa-form-foot">
                      <span
                        className={`qa-counter${
                          answerDraft.length > 2000 ? ' over' : ''
                        }`}
                      >
                        {answerDraft.length}/2000
                      </span>
                      <div className="qa-actions">
                        <button
                          type="submit"
                          className="btn btn-primary btn-sm"
                          disabled={answerMutation.isPending}
                        >
                          {t('p9.qa.answer')}
                        </button>
                        <button
                          type="button"
                          className="btn btn-sm"
                          onClick={cancelAnswer}
                          disabled={answerMutation.isPending}
                        >
                          {t('p9.qa.cancel')}
                        </button>
                      </div>
                    </div>
                    {answerError && (
                      <p
                        className="err"
                        id={`qa-answer-error-${q.id}`}
                        role="alert"
                      >
                        {answerError}
                      </p>
                    )}
                  </form>
                )}
              </li>
            );
          })}
        </ol>
      )}

      {pages > 1 && (
        <div className="qa-pager">
          <button
            type="button"
            className="btn btn-sm"
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            disabled={page <= 1}
            aria-label="Предыдущая страница"
          >
            ←
          </button>
          <span className="muted">
            {page} / {pages}
          </span>
          <button
            type="button"
            className="btn btn-sm"
            onClick={() => setPage((p) => Math.min(pages, p + 1))}
            disabled={page >= pages}
            aria-label="Следующая страница"
          >
            →
          </button>
        </div>
      )}
    </section>
  );
}
