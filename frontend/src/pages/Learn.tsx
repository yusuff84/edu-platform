import { useMemo, useState } from 'react';
import { Link, NavLink, useParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, completeLesson, getCourse, myCourses } from '../api/client';
import LessonView from '../components/LessonView';
import { t } from '../i18n/t';

export default function Learn(): JSX.Element {
  const { courseId = '', lessonId } = useParams();
  const qc = useQueryClient();
  const [verdict, setVerdict] = useState('');
  const myQuery = useQuery({ queryKey: ['my-courses'], queryFn: myCourses });
  const meCourse = (myQuery.data ?? []).find((m) => m.courseId === courseId);
  const courseQuery = useQuery({
    queryKey: ['learn-course', courseId],
    queryFn: async () => {
      // Сначала пробуем найти slug через мои курсы; если нет — courseId может быть slug.
      if (meCourse) return getCourse(meCourse.slug);
      return getCourse(courseId);
    },
    enabled: Boolean(courseId),
  });

  const modules = useMemo(() => courseQuery.data?.modules ?? [], [courseQuery.data]);

  const allLessons = useMemo(() => {
    const mods = courseQuery.data?.modules ?? [];
    return mods.flatMap((m) => m.lessons.map((l) => ({ ...l, moduleTitle: m.title })));
  }, [courseQuery.data]);

  const activeLesson =
    allLessons.find((l) => l.id === lessonId) ??
    allLessons.find((l) => l.id === meCourse?.nextLessonId) ??
    allLessons[0];
  const activeIndex = activeLesson
    ? allLessons.findIndex((l) => l.id === activeLesson.id)
    : -1;
  const nextLesson =
    activeIndex >= 0 && activeIndex + 1 < allLessons.length
      ? allLessons[activeIndex + 1]
      : undefined;

  const mutation = useMutation({
    mutationFn: (payload: {
      quizAnswers?: Record<string, number>;
      code?: string;
      language?: string;
    }) => completeLesson(courseId, activeLesson?.id ?? '', payload),
    onSuccess: (res) => {
      setVerdict(res.accepted ? t('learn.completed') : t('learn.failed'));
      void qc.invalidateQueries({ queryKey: ['my-courses'] });
    },
    onError: (e: unknown) => {
      setVerdict(e instanceof ApiError ? e.message : t('common.error'));
    },
  });

  const percent = meCourse?.percent ?? 0;
  const title = courseQuery.data?.title ?? t('common.loading');
  const isOk = verdict === t('learn.completed');

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
          <div className="progress" aria-label={`${t('learn.progress')}: ${percent}%`}>
            <div className="progress-bar" style={{ width: `${percent}%` }} />
          </div>
        </div>

        <div className="lesson-shell">
          <nav className="side-nav" aria-label={t('course.program')}>
            {modules.map((m) => (
              <div key={m.id}>
                <span className="side-title">{m.title}</span>
                {m.lessons.map((l) => (
                  <NavLink
                    key={l.id}
                    to={`/learn/${courseId}/${l.id}`}
                    className={({ isActive }) => (isActive ? 'active' : undefined)}
                  >
                    {l.title}
                  </NavLink>
                ))}
              </div>
            ))}
            {allLessons.length === 0 && !courseQuery.isLoading && <p>{t('learn.noLessons')}</p>}
          </nav>
          <div className="lesson-pane">
            {!activeLesson && <p>{t('learn.selectLesson')}</p>}
            {activeLesson && (
              <LessonView
                lesson={activeLesson}
                completing={mutation.isPending}
                onComplete={(payload) => mutation.mutateAsync(payload).then(() => undefined)}
              />
            )}
            {verdict && (
              <p role="status" className={isOk ? 'verdict ok' : 'verdict fail'}>
                {verdict}
              </p>
            )}
            {activeLesson && nextLesson && (
              <div>
                <Link className="btn" to={`/learn/${courseId}/${nextLesson.id}`}>
                  {t('learn.continue')}: {nextLesson.title}
                </Link>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
