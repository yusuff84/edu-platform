import { useMemo, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
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

  return (
    <main>
      <p>
        <Link to="/me">{t('common.back')}</Link>
      </p>
      <h1>{courseQuery.data?.title ?? t('common.loading')}</h1>
      <div>
        <span>
          {t('learn.progress')}: {percent}%
        </span>
        <div className="progress" aria-label={`${t('learn.progress')}: ${percent}%`}>
          <div className="progress-bar" style={{ width: `${percent}%` }} />
        </div>
      </div>

      <div className="learn-grid">
        <nav aria-label={t('course.program')}>
          <ul>
            {allLessons.map((l) => (
              <li key={l.id}>
                <Link to={`/learn/${courseId}/${l.id}`}>
                  {l.moduleTitle} — {l.title}
                </Link>
              </li>
            ))}
          </ul>
          {allLessons.length === 0 && !courseQuery.isLoading && <p>{t('learn.noLessons')}</p>}
        </nav>
        <div>
          {!activeLesson && <p>{t('learn.selectLesson')}</p>}
          {activeLesson && (
            <LessonView
              lesson={activeLesson}
              completing={mutation.isPending}
              onComplete={(payload) => mutation.mutateAsync(payload).then(() => undefined)}
            />
          )}
          {verdict && <p role="status">{verdict}</p>}
          {activeLesson && nextLesson && (
            <p>
              <Link to={`/learn/${courseId}/${nextLesson.id}`}>
                {t('learn.continue')}: {nextLesson.title}
              </Link>
            </p>
          )}
        </div>
      </div>
    </main>
  );
}
