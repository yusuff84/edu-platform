// T86: раздел «Вопросы и ответы» супер-админки (/admin/qa).
// Глобальный список вопросов всех курсов (GET /admin/questions, T85):
// фильтры courseId/lessonId/q в URL, пагинация, раскрытие ветки ответов,
// удаление вопроса или отдельного ответа через существующие DELETE P9.
// Стили изолированы в styles/adminQa.css; общий styles.css не изменяется.
import { useEffect, useMemo, useState } from 'react';
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import type { QueryClient } from '@tanstack/react-query';
import { Link, useSearchParams } from 'react-router-dom';
import { ApiError, formatDate } from '../api/client';
import {
  deleteAdminAnswer,
  deleteAdminQuestion,
  listAdminCourses,
  listAdminQuestions,
} from '../api/adminConsole';
import type {
  AdminAnswer,
  AdminQuestion,
  AdminQuestionsResponse,
} from '../api/adminConsole';
import { getTeacherCourseDetail } from '../api/teacherStudents';
import { t } from '../i18n/t';
import { useSeo } from '../app/useSeo';
import '../styles/adminQa.css';

// ---------- Константы ----------

/** Размер страницы (контракт T85: 20..100, по умолчанию 20). */
const PAGE_LIMIT = 20;

/** Справочник курсов для фильтра: один запрос, максимум контракта (100). */
const COURSE_OPTIONS_LIMIT = 100;

/** Задержка поиска: запрос уходит после паузы в наборе (как в AdminUsers). */
const SEARCH_DEBOUNCE_MS = 300;

/** Префикс ключа кэша списка: инвалидируется вместе со всеми фильтрами. */
const LIST_KEY = ['admin-questions'] as const;

const COURSE_OPTIONS_KEY = ['admin-qa-course-options'] as const;
const LESSONS_KEY = ['admin-qa-lessons'] as const;

/** Опасное действие, ожидающее инлайн-подтверждения. */
type PendingConfirm =
  | { kind: 'question'; questionId: string }
  | { kind: 'answer'; questionId: string; answerId: string };

// ---------- Утилиты ----------

/** Текст ошибки без краха страницы (схема AdminUsers/Admin). */
function errorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 403) return t('cabinet.needRole');
    if (error.status === 401) return t('error.needLogin');
    return error.message || t('common.error');
  }
  if (error instanceof TypeError) return t('error.network');
  return t('common.error');
}

/** Короткий id — запасной вариант, если у автора нет имени/email. */
function shortId(id: string): string {
  return id.length > 8 ? id.slice(0, 8) : id;
}

/** Имя автора: имя → email → короткий id (как в журнале платежей). */
function authorLabel(name: string, email: string, id: string): string {
  return name || email || shortId(id);
}

/** Убрать вопрос из кэша сразу после успешного DELETE — без ожидания refetch. */
function removeCachedQuestion(qc: QueryClient, id: string): void {
  qc.setQueriesData<AdminQuestionsResponse>({ queryKey: LIST_KEY }, (prev) => {
    if (!prev) return prev;
    const items = prev.items.filter((q) => q.id !== id);
    if (items.length === prev.items.length) return prev;
    return { ...prev, items, total: Math.max(0, prev.total - 1) };
  });
}

/** Убрать ответ из кэша; вопрос с оставшимися ответами остаётся на месте. */
function removeCachedAnswer(
  qc: QueryClient,
  questionId: string,
  answerId: string,
): void {
  qc.setQueriesData<AdminQuestionsResponse>({ queryKey: LIST_KEY }, (prev) => {
    if (!prev) return prev;
    let changed = false;
    const items = prev.items.map((question) => {
      if (question.id !== questionId) return question;
      const answers = question.answers.filter((a) => a.id !== answerId);
      if (answers.length === question.answers.length) return question;
      changed = true;
      return { ...question, answers };
    });
    return changed ? { ...prev, items } : prev;
  });
}

/** Бейдж роли автора ответа: преподаватель — отдельным ключом P9. */
function AnswerRoleBadge({ role }: { role: AdminAnswer['authorRole'] }): JSX.Element | null {
  if (role === 'teacher') {
    return <span className="aq-badge aq-badge--teacher">{t('p9.qa.teacherBadge')}</span>;
  }
  if (role === 'student') {
    return <span className="aq-badge aq-badge--student">{t('p10.qa.student')}</span>;
  }
  // Для роли admin отдельного замороженного ключа нет — бейдж не рисуем.
  return null;
}

// ---------- Страница ----------

export default function AdminQa(): JSX.Element {
  useSeo(`${t('p10.qa.title')} — Edu Platform`);
  const qc = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();

  // ---- URL-состояние: courseId/lessonId/q/page (ссылка шарится) ----
  const courseId = searchParams.get('courseId') ?? '';
  const lessonId = searchParams.get('lessonId') ?? '';
  const q = searchParams.get('q') ?? '';
  const pageParam = Number(searchParams.get('page'));
  const page = Number.isInteger(pageParam) && pageParam >= 1 ? pageParam : 1;

  /** Точечное обновление query-параметров: null/'' — удалить параметр. */
  function setParams(patch: Record<string, string | null>): void {
    const next = new URLSearchParams(searchParams);
    for (const [key, value] of Object.entries(patch)) {
      if (value === null || value === '') next.delete(key);
      else next.set(key, value);
    }
    setSearchParams(next);
  }

  // Поле поиска правится локально, в URL уходит после паузы (debounce).
  const [searchInput, setSearchInput] = useState(q);
  useEffect(() => {
    // URL — источник истины: «Назад» в браузере и переходы по истории
    // обновляют поле поиска.
    setSearchInput(q);
  }, [q]);

  useEffect(() => {
    const trimmed = searchInput.trim();
    if (trimmed === q) return;
    const timer = window.setTimeout(() => {
      const next = new URLSearchParams(searchParams);
      if (trimmed) next.set('q', trimmed);
      else next.delete('q');
      next.delete('page');
      setSearchParams(next, { replace: true });
    }, SEARCH_DEBOUNCE_MS);
    return () => window.clearTimeout(timer);
  }, [searchInput, q, searchParams, setSearchParams]);

  // Урок без выбранного курса в UI не выбрать: чистим осиротевший lessonId.
  useEffect(() => {
    if (courseId !== '' || lessonId === '') return;
    const next = new URLSearchParams(searchParams);
    next.delete('lessonId');
    next.delete('page');
    setSearchParams(next, { replace: true });
  }, [courseId, lessonId, searchParams, setSearchParams]);

  // ---- Справочник курсов для фильтра ----
  const courses = useQuery({
    queryKey: COURSE_OPTIONS_KEY,
    queryFn: () =>
      listAdminCourses({
        page: 1,
        limit: COURSE_OPTIONS_LIMIT,
        sort: 'title',
        dir: 'asc',
      }),
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  const courseOptions = useMemo<Array<{ id: string; title: string }>>(() => {
    const items = (courses.data?.items ?? []).map((course) => ({
      id: course.id,
      title: course.title,
    }));
    items.sort((a, b) => a.title.localeCompare(b.title, 'ru'));
    // Курс, выбранный через URL, должен быть виден в select, даже если его
    // нет среди первых 100, — иначе фильтр «невидим» и его нельзя снять.
    if (courseId !== '' && !items.some((course) => course.id === courseId)) {
      items.unshift({ id: courseId, title: courseId });
    }
    return items;
  }, [courses.data, courseId]);

  // ---- Уроки выбранного курса ----
  const lessons = useQuery({
    queryKey: [...LESSONS_KEY, courseId],
    queryFn: () => getTeacherCourseDetail(courseId),
    enabled: courseId !== '',
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  const lessonOptions = useMemo<Array<{ id: string; title: string }>>(() => {
    if (courseId === '') return [];
    const items = (lessons.data?.modules ?? []).flatMap((module) =>
      module.lessons.map((lesson) => ({ id: lesson.id, title: lesson.title })),
    );
    if (lessonId !== '' && !items.some((lesson) => lesson.id === lessonId)) {
      items.unshift({ id: lessonId, title: lessonId });
    }
    return items;
  }, [lessons.data, courseId, lessonId]);

  // ---- Список вопросов ----
  const questions = useQuery({
    queryKey: [...LIST_KEY, { courseId, lessonId, q, page }],
    queryFn: () =>
      listAdminQuestions({
        courseId: courseId || undefined,
        lessonId: lessonId || undefined,
        q: q || undefined,
        page,
        limit: PAGE_LIMIT,
      }),
    // При смене страницы/фильтра старые карточки остаются до прихода новых.
    placeholderData: keepPreviousData,
    // 4xx — не временный сбой, ретраим только сетевые ошибки.
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  // ---- Локальные сообщения об операциях ----
  const [notice, setNotice] = useState('');
  const [rowErrors, setRowErrors] = useState<Record<string, string>>({});
  const [confirm, setConfirm] = useState<PendingConfirm | null>(null);
  // Раскрытые ветки ответов: id вопросов, у которых нажали «Развернуть».
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});

  function setRowError(key: string, text: string): void {
    setRowErrors((prev) => ({ ...prev, [key]: text }));
  }

  function clearRowError(key: string): void {
    setRowErrors((prev) => {
      if (!prev[key]) return prev;
      const next = { ...prev };
      delete next[key];
      return next;
    });
  }

  function toggleExpanded(questionId: string): void {
    setExpanded((prev) => ({ ...prev, [questionId]: !prev[questionId] }));
  }

  // ---- Мутации удаления ----
  const questionMutation = useMutation({
    mutationFn: (vars: { courseId: string; questionId: string }) =>
      deleteAdminQuestion(vars.courseId, vars.questionId),
    onSuccess: (_data, vars) => {
      clearRowError(`q:${vars.questionId}`);
      setConfirm(null);
      setNotice(t('p10.qa.deleted'));
      removeCachedQuestion(qc, vars.questionId);
      // Удалили последнюю карточку страницы — возвращаемся на предыдущую.
      const data = questions.data;
      if (data && data.items.length === 1 && page > 1) {
        setParams({ page: String(page - 1) });
      }
      void qc.invalidateQueries({ queryKey: LIST_KEY });
    },
    onError: (e: unknown, vars) => {
      setConfirm(null);
      setNotice('');
      // 403/404 и прочие — текст сервера (errorText).
      setRowError(`q:${vars.questionId}`, errorText(e));
    },
  });

  const answerMutation = useMutation({
    mutationFn: (vars: { courseId: string; questionId: string; answerId: string }) =>
      deleteAdminAnswer(vars.courseId, vars.questionId, vars.answerId),
    onSuccess: (_data, vars) => {
      clearRowError(`a:${vars.answerId}`);
      setConfirm(null);
      setNotice(t('p10.qa.deleted'));
      removeCachedAnswer(qc, vars.questionId, vars.answerId);
      void qc.invalidateQueries({ queryKey: LIST_KEY });
    },
    onError: (e: unknown, vars) => {
      setConfirm(null);
      setNotice('');
      setRowError(`a:${vars.answerId}`, errorText(e));
    },
  });

  const busy = questionMutation.isPending || answerMutation.isPending;

  // ---- Обработчики действий ----
  function askDeleteQuestion(question: AdminQuestion): void {
    clearRowError(`q:${question.id}`);
    setNotice('');
    setConfirm({ kind: 'question', questionId: question.id });
  }

  function askDeleteAnswer(questionId: string, answerId: string): void {
    clearRowError(`a:${answerId}`);
    setNotice('');
    setConfirm({ kind: 'answer', questionId, answerId });
  }

  function confirmDeleteQuestion(question: AdminQuestion): void {
    if (!confirm || confirm.kind !== 'question' || confirm.questionId !== question.id) {
      return;
    }
    questionMutation.mutate({
      courseId: question.courseId,
      questionId: question.id,
    });
  }

  function confirmDeleteAnswer(courseIdValue: string, questionId: string, answerId: string): void {
    if (
      !confirm ||
      confirm.kind !== 'answer' ||
      confirm.questionId !== questionId ||
      confirm.answerId !== answerId
    ) {
      return;
    }
    answerMutation.mutate({ courseId: courseIdValue, questionId, answerId });
  }

  // ---- Производные значения списка ----
  const data = questions.data;
  const total = data?.total ?? 0;
  const rangeFrom = total === 0 ? 0 : (page - 1) * PAGE_LIMIT + 1;
  const rangeTo = Math.min(page * PAGE_LIMIT, total);
  const isLastPage = total > 0 && rangeTo >= total;
  // Пагинатор прячем только на пустой первой странице: если данных не
  // осталось, а страница не первая, кнопка «Назад» вернёт список.
  const showPager = !!data && (total > 0 || page > 1);

  return (
    <div className="wrap">
      <div className="page">
        <main className="aq-page">
          <p className="aq-back">
            <Link to="/admin">{t('cabinet.adminTitle')}</Link>
          </p>

          <div className="aq-head">
            <div>
              <h1>{t('p10.qa.title')}</h1>
              <p className="muted">{t('p10.qa.subtitle')}</p>
            </div>
            {/* Отдельного ключа «Обновить» в p10.qa.* нет — используем common.retry. */}
            <button
              type="button"
              className="btn"
              onClick={() => void questions.refetch()}
              disabled={questions.isFetching}
            >
              {t('common.retry')}
            </button>
          </div>

          <section className="panel">
            {/* ---------- Фильтры ---------- */}
            <div className="aq-filters">
              <label className="aq-field aq-field--search" htmlFor="aq-search">
                <span>{t('p10.qa.search')}</span>
                <input
                  id="aq-search"
                  className="input"
                  type="search"
                  value={searchInput}
                  placeholder={t('p10.qa.search')}
                  onChange={(e) => setSearchInput(e.target.value)}
                />
              </label>

              <label className="aq-field" htmlFor="aq-course">
                <span>{t('p10.qa.filterCourse')}</span>
                <select
                  id="aq-course"
                  className="input aq-select"
                  value={courseId}
                  disabled={courses.isLoading}
                  onChange={(e) =>
                    setParams({
                      courseId: e.target.value || null,
                      lessonId: null,
                      page: null,
                    })
                  }
                >
                  <option value="">{t('p10.qa.allCourses')}</option>
                  {courseOptions.map((course) => (
                    <option key={course.id} value={course.id}>
                      {course.title}
                    </option>
                  ))}
                </select>
              </label>

              <label className="aq-field" htmlFor="aq-lesson">
                <span>{t('p10.qa.filterLesson')}</span>
                <select
                  id="aq-lesson"
                  className="input aq-select"
                  value={lessonId}
                  disabled={courseId === '' || lessons.isLoading}
                  onChange={(e) =>
                    setParams({ lessonId: e.target.value || null, page: null })
                  }
                >
                  <option value="">{t('p10.qa.allLessons')}</option>
                  {lessonOptions.map((lesson) => (
                    <option key={lesson.id} value={lesson.id}>
                      {lesson.title}
                    </option>
                  ))}
                </select>
              </label>
            </div>

            {courses.isError && (
              <p className="error aq-hint" role="alert">
                {errorText(courses.error)}
              </p>
            )}
            {lessons.isError && courseId !== '' && (
              <p className="error aq-hint" role="alert">
                {errorText(lessons.error)}
              </p>
            )}

            {data && (
              <p className="aq-total muted">
                {t('admin.showing')} {rangeFrom}–{rangeTo} {t('admin.of')} {total}
              </p>
            )}

            {notice && (
              <p className="aq-notice" role="status" aria-live="polite">
                {notice}
              </p>
            )}

            {/* ---------- Состояния запроса ---------- */}
            {questions.isLoading && (
              <div className="aq-skeleton" role="status" aria-live="polite">
                <span className="aq-skeleton-line" />
                <span className="aq-skeleton-line" />
                <span className="aq-skeleton-line" />
                <span className="sr-only">{t('common.loading')}</span>
              </div>
            )}

            {questions.isError && (
              <div className="aq-error" role="alert">
                <p className="error">{errorText(questions.error)}</p>
                <button type="button" className="btn" onClick={() => void questions.refetch()}>
                  {t('common.retry')}
                </button>
              </div>
            )}

            {data && data.items.length === 0 && <p className="muted">{t('p10.qa.empty')}</p>}

            {/* ---------- Карточки вопросов ---------- */}
            {data && data.items.length > 0 && (
              <ul className="aq-list" aria-label={t('p10.qa.title')}>
                {data.items.map((question) => {
                  const answersCount = question.answers.length;
                  const isExpanded = expanded[question.id] === true;
                  const questionConfirm =
                    confirm?.kind === 'question' && confirm.questionId === question.id
                      ? confirm
                      : null;
                  const questionError = rowErrors[`q:${question.id}`];

                  return (
                    <li
                      key={question.id}
                      className="aq-card"
                      aria-busy={busy || undefined}
                    >
                      <div className="aq-card__head">
                        <div className="aq-meta">
                          {question.courseSlug ? (
                            <Link
                              className="aq-link"
                              to={`/courses/${encodeURIComponent(question.courseSlug)}`}
                              title={t('p10.qa.openCourse')}
                            >
                              {question.courseTitle || '—'}
                            </Link>
                          ) : (
                            <span>{question.courseTitle || '—'}</span>
                          )}
                          <span aria-hidden="true">·</span>
                          <span>
                            {t('p10.qa.lesson')}:{' '}
                            <Link
                              className="aq-link"
                              to={`/admin/courses/${encodeURIComponent(question.courseId)}`}
                            >
                              {question.lessonTitle || '—'}
                            </Link>
                          </span>
                          <span aria-hidden="true">·</span>
                          <span>
                            {t('p10.qa.created')}: {formatDate(question.createdAt)}
                          </span>
                        </div>
                        <span className="aq-count">
                          {answersCount} · {t('p10.qa.answers')}
                        </span>
                      </div>

                      <p className="aq-author">
                        <span className="aq-badge aq-badge--student">
                          {t('p10.qa.student')}
                        </span>
                        <span className="aq-author-name">
                          {authorLabel(
                            question.studentName,
                            question.studentEmail,
                            question.studentId,
                          )}
                        </span>
                        {question.studentName && question.studentEmail && (
                          <span className="muted aq-author-email">
                            {question.studentEmail}
                          </span>
                        )}
                      </p>

                      <p className="aq-body">{question.body}</p>

                      <div className="aq-actions">
                        {answersCount > 0 && (
                          <button
                            type="button"
                            className="btn btn-sm"
                            aria-expanded={isExpanded}
                            aria-controls={`aq-answers-${question.id}`}
                            onClick={() => toggleExpanded(question.id)}
                          >
                            {isExpanded ? t('p10.qa.collapse') : t('p10.qa.expand')}
                          </button>
                        )}

                        {questionConfirm ? (
                          <span className="aq-confirm">
                            <span>{t('p10.qa.confirmQuestion')}</span>
                            <button
                              type="button"
                              className="btn btn-sm aq-btn--danger"
                              disabled={busy}
                              onClick={() => confirmDeleteQuestion(question)}
                            >
                              {t('common.confirm')}
                            </button>
                            <button
                              type="button"
                              className="btn btn-sm"
                              disabled={busy}
                              onClick={() => setConfirm(null)}
                            >
                              {t('common.cancel')}
                            </button>
                          </span>
                        ) : (
                          <button
                            type="button"
                            className="btn btn-sm aq-btn--danger"
                            disabled={busy}
                            onClick={() => askDeleteQuestion(question)}
                          >
                            {t('p10.qa.deleteQuestion')}
                          </button>
                        )}
                      </div>

                      {questionError && (
                        <p className="error aq-row-error" role="alert">
                          {questionError}
                        </p>
                      )}

                      {answersCount > 0 && (
                        <section
                          id={`aq-answers-${question.id}`}
                          className="aq-answers"
                          aria-label={t('p10.qa.answers')}
                          hidden={!isExpanded}
                        >
                          <ul className="aq-answer-list">
                            {question.answers.map((answer) => {
                              const answerConfirm =
                                confirm?.kind === 'answer' &&
                                confirm.questionId === question.id &&
                                confirm.answerId === answer.id
                                  ? confirm
                                  : null;
                              const answerError = rowErrors[`a:${answer.id}`];

                              return (
                                <li key={answer.id} className="aq-answer">
                                  <div className="aq-answer__head">
                                    <AnswerRoleBadge role={answer.authorRole} />
                                    <span className="aq-author-name">
                                      {authorLabel(
                                        answer.authorName,
                                        answer.authorEmail,
                                        answer.authorId,
                                      )}
                                    </span>
                                    {answer.authorName && answer.authorEmail && (
                                      <span className="muted aq-author-email">
                                        {answer.authorEmail}
                                      </span>
                                    )}
                                    <span className="muted aq-answer-date">
                                      {formatDate(answer.createdAt)}
                                    </span>
                                  </div>

                                  <p className="aq-body">{answer.body}</p>

                                  <div className="aq-answer__actions">
                                    {answerConfirm ? (
                                      <span className="aq-confirm">
                                        <span>{t('p10.qa.confirmAnswer')}</span>
                                        <button
                                          type="button"
                                          className="btn btn-sm aq-btn--danger"
                                          disabled={busy}
                                          onClick={() =>
                                            confirmDeleteAnswer(
                                              question.courseId,
                                              question.id,
                                              answer.id,
                                            )
                                          }
                                        >
                                          {t('common.confirm')}
                                        </button>
                                        <button
                                          type="button"
                                          className="btn btn-sm"
                                          disabled={busy}
                                          onClick={() => setConfirm(null)}
                                        >
                                          {t('common.cancel')}
                                        </button>
                                      </span>
                                    ) : (
                                      <button
                                        type="button"
                                        className="btn btn-sm aq-btn--danger"
                                        disabled={busy}
                                        onClick={() =>
                                          askDeleteAnswer(question.id, answer.id)
                                        }
                                      >
                                        {t('p10.qa.deleteAnswer')}
                                      </button>
                                    )}
                                  </div>

                                  {answerError && (
                                    <p className="error aq-row-error" role="alert">
                                      {answerError}
                                    </p>
                                  )}
                                </li>
                              );
                            })}
                          </ul>
                        </section>
                      )}
                    </li>
                  );
                })}
              </ul>
            )}

            {/* ---------- Пагинация ---------- */}
            {showPager && data && (
              <div className="aq-pager">
                <button
                  type="button"
                  className="btn"
                  disabled={page <= 1 || questions.isFetching}
                  onClick={() => setParams({ page: String(page - 1) })}
                >
                  {t('admin.prev')}
                </button>
                <span className="muted">
                  {t('admin.showing')} {rangeFrom}–{rangeTo} {t('admin.of')} {total}
                </span>
                <button
                  type="button"
                  className="btn"
                  disabled={isLastPage || questions.isFetching}
                  onClick={() => setParams({ page: String(page + 1) })}
                >
                  {t('admin.next')}
                </button>
              </div>
            )}
          </section>
        </main>
      </div>
    </div>
  );
}
