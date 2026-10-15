// Экран обучения: список уроков по модулям, прогресс, просмотр урока и вердикт.
//
// Ключевое: курс в URL известен по id (/learn/:courseId), а getCourse() принимает
// slug (backend GET /courses/{slug} ждёт слаг, не UUID). Slug берём из элемента
// myCourses(), найденного по course.id; если курса нет в «моих» — не ходим в
// getCourse() с UUID, а показываем «нет доступа».
//
// Вердикт определяется полем passed результата CompleteResult.

import { useMemo, useState } from 'react';
import { Link, NavLink, useParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, completeLesson, getCourse, myCourses } from '../api/client';
import type { CompleteLessonInput, CompleteResult, Lesson } from '../api/client';
import LessonView from '../components/LessonView';
import { t } from '../i18n/t';
import '../styles/learn.css';

/** Урок с названием модуля — для плоского списка и подсветки. */
type FlatLesson = Lesson & { moduleTitle: string };

/** Вердикт по уроку, привязанный к уроку (чтобы не «мигал» при переходах). */
interface VerdictState {
  lessonId: string;
  result: CompleteResult;
}

/** Текст ошибки запроса завершения, привязанный к уроку. */
interface ErrorState {
  lessonId: string;
  message: string;
}

/** Понятный текст ошибки completeLesson по статусу. */
function errorText(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.status === 403) return t('learn.noAccess');
    if (e.status === 502) return t('lesson.runnerUnavailable');
    return e.message || t('common.error');
  }
  return t('common.error');
}

export default function Learn(): JSX.Element {
  const { courseId = '', lessonId } = useParams();
  const qc = useQueryClient();

  const [verdictState, setVerdictState] = useState<VerdictState | null>(null);
  const [errorState, setErrorState] = useState<ErrorState | null>(null);
  // Уроки, завершённые в этой сессии (сервер не отдаёт список завершённых).
  const [completedNow, setCompletedNow] = useState<ReadonlySet<string>>(new Set());

  // Шарим кеш списка «моих курсов» с MyCourses.tsx (ключ в репозитории — 'my-courses').
  const myQuery = useQuery({ queryKey: ['my-courses'], queryFn: myCourses });

  // курс известен по id, API ждёт slug → находим slug в «моих курсах».
  const meCourse = (myQuery.data ?? []).find((m) => m.course.id === courseId);
  const slug = meCourse?.course.slug ?? '';

  // getCourse() вызывается ТОЛЬКО со slug; до известности slug запрос не стартует.
  const courseQuery = useQuery({
    queryKey: ['learn-course', courseId],
    queryFn: () => getCourse(slug),
    enabled: myQuery.isSuccess && Boolean(courseId) && Boolean(slug),
    retry: false,
  });

  const modules = useMemo(() => {
    const mods = courseQuery.data?.modules ?? [];
    return [...mods]
      .sort((a, b) => a.position - b.position)
      .map((m) => ({
        ...m,
        lessons: [...m.lessons].sort((a, b) => a.position - b.position),
      }));
  }, [courseQuery.data]);

  const flatLessons: FlatLesson[] = useMemo(
    () =>
      modules.flatMap((m) => m.lessons.map((l) => ({ ...l, moduleTitle: m.title }))),
    [modules],
  );

  // Актуальный прогресс: серверный percent из последнего завершения, иначе из «моих курсов».
  const percent = verdictState?.result.percent ?? meCourse?.percent ?? 0;

  // Следующий урок: после успешного завершения — из ответа сервера, иначе из «моих курсов».
  const serverNextLessonId = verdictState?.result.passed
    ? (verdictState.result.nextLesson?.lessonId ?? null)
    : (meCourse?.nextLesson?.lessonId ?? null);

  // Отметка завершённых уроков. Контракт не отдаёт список завершённых уроков,
  // поэтому считаем завершёнными все уроки до nextLesson в порядке курса
  // (backend возвращает первым именно первый незавершённый) плюс те,
  // что завершены в этой сессии.
  const completedIds = useMemo(() => {
    const ids = new Set(completedNow);
    if (percent >= 100) {
      flatLessons.forEach((l) => ids.add(l.id));
      return ids;
    }
    if (serverNextLessonId) {
      const idx = flatLessons.findIndex((l) => l.id === serverNextLessonId);
      for (let i = 0; i < idx; i++) ids.add(flatLessons[i].id);
    }
    return ids;
  }, [flatLessons, completedNow, percent, serverNextLessonId]);

  // Активный урок: из URL, иначе «следующий», иначе первый.
  const lessonFromUrl = lessonId ? flatLessons.find((l) => l.id === lessonId) : undefined;
  const nextByServer = serverNextLessonId
    ? flatLessons.find((l) => l.id === serverNextLessonId)
    : undefined;
  const activeLesson: FlatLesson | undefined =
    lessonFromUrl ?? nextByServer ?? flatLessons[0];

  // Линейный «следующий» для кнопки продолжения, если сервер не дал nextLesson.
  const activeIndex = activeLesson ? flatLessons.indexOf(activeLesson) : -1;
  const flatNext =
    activeIndex >= 0 && activeIndex + 1 < flatLessons.length
      ? flatLessons[activeIndex + 1]
      : undefined;

  const mutation = useMutation({
    mutationFn: (payload: CompleteLessonInput) => {
      if (!activeLesson) {
        return Promise.reject(new Error('нет активного урока'));
      }
      return completeLesson(courseId, activeLesson.id, payload);
    },
    onSuccess: (res) => {
      if (!activeLesson) return;
      setVerdictState({ lessonId: activeLesson.id, result: res });
      setErrorState(null);
      if (res.completed) {
        setCompletedNow((prev) => new Set(prev).add(activeLesson.id));
      }
      // Инвалидируем оба варианта ключа списка «моих курсов»: в этой волне
      // страница использует 'my-courses', при переименовании — 'me-courses'.
      void qc.invalidateQueries({ queryKey: ['my-courses'] });
      void qc.invalidateQueries({ queryKey: ['me-courses'] });
    },
    onError: (e: unknown) => {
      if (!activeLesson) return;
      setVerdictState(null);
      setErrorState({ lessonId: activeLesson.id, message: errorText(e) });
    },
  });

  // Вердикт и ошибка показываются только для активного урока.
  const verdict = verdictState && verdictState.lessonId === activeLesson?.id
    ? verdictState.result
    : null;
  const verdictError =
    errorState && errorState.lessonId === activeLesson?.id ? errorState.message : null;

  // ---------- Состояния загрузки / ошибки / доступа ----------

  if (myQuery.isLoading) {
    return (
      <div className="wrap">
        <div className="page">
          <div className="panel">
            <p role="status">{t('learn.loading')}</p>
          </div>
        </div>
      </div>
    );
  }

  if (myQuery.isError) {
    return (
      <div className="wrap">
        <div className="page">
          <div className="panel">
            <h1>{t('learn.title')}</h1>
            <p className="error" role="alert">
              {errorText(myQuery.error)}
            </p>
            <button type="button" onClick={() => void myQuery.refetch()}>
              {t('common.retry')}
            </button>
          </div>
        </div>
      </div>
    );
  }

  if (!meCourse) {
    // Курса нет в «моих курсах» — getCourse() с UUID не вызываем (всегда 404).
    return (
      <div className="wrap">
        <div className="page">
          <div className="panel">
            <h1>{t('learn.title')}</h1>
            <p role="alert">{t('learn.noAccess')}</p>
            <p>
              <Link to="/">{t('my.goCatalog')}</Link>
            </p>
          </div>
        </div>
      </div>
    );
  }

  if (courseQuery.isLoading) {
    return (
      <div className="wrap">
        <div className="page">
          <div className="panel">
            <h1>{meCourse.course.title}</h1>
            <p role="status">{t('learn.loading')}</p>
          </div>
        </div>
      </div>
    );
  }

  if (courseQuery.isError || !courseQuery.data) {
    const status =
      courseQuery.error instanceof ApiError ? courseQuery.error.status : undefined;
    return (
      <div className="wrap">
        <div className="page">
          <div className="panel">
            <h1>{meCourse.course.title}</h1>
            <p className="error" role="alert">
              {status === 404 ? t('course.notFound') : errorText(courseQuery.error)}
            </p>
            {status === 404 ? (
              <p>
                <Link to="/">{t('my.goCatalog')}</Link>
              </p>
            ) : (
              <button type="button" onClick={() => void courseQuery.refetch()}>
                {t('common.retry')}
              </button>
            )}
          </div>
        </div>
      </div>
    );
  }

  if (flatLessons.length === 0) {
    return (
      <div className="wrap">
        <div className="page">
          <div className="crumbs">
            <Link to="/">{t('nav.catalog')}</Link> / <span>{courseQuery.data.title}</span>
          </div>
          <h1>{courseQuery.data.title}</h1>
          <p>{t('learn.empty')}</p>
          <p>
            <Link to="/me">{t('common.back')}</Link>
          </p>
        </div>
      </div>
    );
  }

  const showLessonNotFound = Boolean(lessonId) && !lessonFromUrl;

  // После ранних return уроки гарантированно есть — activeLesson определён.
  const currentLesson: FlatLesson = activeLesson ?? flatLessons[0];
  const title = courseQuery.data.title;

  // ---------- Рендер ----------

  return (
    <div className="wrap">
      <div className="page">
        <div className="crumbs">
          <Link to="/">{t('nav.catalog')}</Link> / <span>{title}</span>
        </div>
        <h1>{title}</h1>
        <div style={{ margin: '14px 0 20px' }}>
          <span className="muted">
            {t('learn.progress')}: {percent}%
          </span>
          <div
            className="progress"
            role="progressbar"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={percent}
            aria-label={`${t('learn.progress')}: ${percent}%`}
          >
            <div className="progress-bar" style={{ width: `${percent}%` }} />
          </div>
        </div>

        <div className="lesson-shell">
          <nav className="side-nav" aria-label={t('course.program')}>
            {modules.map((m) => (
              <div key={m.id}>
                <span className="side-title">{m.title}</span>
                {m.lessons.map((l) => {
                  const done = completedIds.has(l.id);
                  const isActive = l.id === currentLesson.id;
                  const isNext = l.id === serverNextLessonId;
                  return (
                    <NavLink
                      key={l.id}
                      to={`/learn/${courseId}/${l.id}`}
                      className={({ isActive: navActive }) =>
                        [
                          'learn-link',
                          done ? 'learn-link-done' : '',
                          isActive || navActive ? 'active learn-link-active' : '',
                        ]
                          .filter(Boolean)
                          .join(' ')
                      }
                      aria-current={isActive ? 'true' : undefined}
                    >
                      <span className="learn-link-check" aria-hidden="true">
                        {done ? '✓ ' : ''}
                      </span>
                      {l.title}
                      <span className="learn-link-kind"> · {t(`kind.${l.kind}`)}</span>
                      {done && (
                        <span className="sr-only"> — {t('learn.completed')}</span>
                      )}
                      {isNext && (
                        <span className="learn-link-next"> — {t('learn.nextLesson')}</span>
                      )}
                    </NavLink>
                  );
                })}
              </div>
            ))}
            {flatLessons.length === 0 && !courseQuery.isLoading && (
              <p>{t('learn.noLessons')}</p>
            )}
          </nav>
          <div className="lesson-pane">
            {!activeLesson && <p>{t('learn.selectLesson')}</p>}
            {showLessonNotFound ? (
              <div role="alert" className="learn-notice">
                <p className="error">{t('learn.lessonNotFound')}</p>
                <p>
                  <Link to={`/learn/${courseId}/${currentLesson.id}`}>
                    {t('learn.continue')}
                  </Link>
                </p>
              </div>
            ) : (
              <>
                <LessonView
                  key={currentLesson.id}
                  lesson={currentLesson}
                  completing={mutation.isPending}
                  onComplete={(payload) => mutation.mutate(payload)}
                />

                {/* Вердикт для скринридеров и визуально. */}
                <div className="learn-verdict" role="status" aria-live="polite">
                  {verdictError && <p className="error">{verdictError}</p>}
                  {verdict?.passed && (
                    <p className="learn-verdict-pass">
                      {t('learn.verdictPassed')}
                      {typeof verdict.percent === 'number'
                        ? ` — ${t('learn.progress')}: ${verdict.percent}%`
                        : ''}
                    </p>
                  )}
                  {verdict && !verdict.passed && (
                    <p className="learn-verdict-fail">
                      {t('learn.verdictFailed')}
                      {verdict.message ? ` — ${verdict.message}` : ''}
                      {typeof verdict.correct === 'number' && typeof verdict.total === 'number'
                        ? ` (${verdict.correct}/${verdict.total})`
                        : ''}
                    </p>
                  )}
                </div>

                {verdict?.passed && verdict.nextLesson && (
                  <p>
                    <Link
                      className="btn learn-next-link"
                      to={`/learn/${courseId}/${verdict.nextLesson.lessonId}`}
                    >
                      {t('lesson.next')}: {verdict.nextLesson.title}
                    </Link>
                  </p>
                )}
                {!verdict?.passed && flatNext && (
                  <p>
                    <Link
                      className="btn learn-next-link"
                      to={`/learn/${courseId}/${flatNext.id}`}
                    >
                      {t('learn.continue')}: {flatNext.title}
                    </Link>
                  </p>
                )}
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
