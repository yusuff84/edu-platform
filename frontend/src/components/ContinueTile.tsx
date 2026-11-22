import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { getCourseOutline } from '../api/client';
import { LessonKindIcon } from './LessonKindIcon';
import { VideoPoster } from './VideoPoster';
import { videoPosterUrl } from '../app/media';

interface Props {
  courseId: string;
  courseSlug: string;
  courseTitle: string;
  nextLessonId?: string | null;
  percent?: number;
}

/**
 * Плитка «Продолжить обучение»: показывает урок, на котором остановились, —
 * кадр из видео (видеоурок) или иконку практикума / конспекта, название шага
 * и процент прохождения курса.
 */
export function ContinueTile({ courseId, courseSlug, courseTitle, nextLessonId, percent }: Props): JSX.Element {
  const course = useQuery({ queryKey: ['course', courseSlug], queryFn: () => getCourseOutline(courseSlug), enabled: Boolean(courseSlug), staleTime: 60_000 });
  const lessons = (course.data?.modules ?? []).flatMap((m) => m.lessons);
  const lesson = (nextLessonId ? lessons.find((l) => l.id === nextLessonId) : undefined) ?? lessons[0];
  const href = lesson ? `/learn/${courseId}/${lesson.id}` : `/learn/${courseId}`;
  const isVideo = Boolean(lesson?.videoUrl && videoPosterUrl(lesson.videoUrl));
  const kindLabel = !lesson ? '' : lesson.videoUrl ? 'Видеоурок' : lesson.kind === 'quiz' ? 'Практикум' : 'Конспект';

  return (
    <Link className="continue-tile" to={href}>
      <span className={`continue-tile-art${isVideo ? ' has-video' : ''}`} aria-hidden="true">
        {isVideo && <VideoPoster videoUrl={lesson?.videoUrl} className="continue-tile-poster" />}
        {lesson && !lesson.videoUrl ? (
          <LessonKindIcon kind={lesson.kind === 'quiz' ? 'quiz' : 'text'} />
        ) : (
          <span className="continue-tile-play">▶</span>
        )}
        {kindLabel && <span className="continue-tile-kind">{kindLabel}</span>}
      </span>
      <span className="continue-tile-body">
        <span className="continue-tile-course">{courseTitle}</span>
        <b className="continue-tile-lesson">{lesson ? lesson.title : 'Начать обучение'}</b>
        <span className="continue-tile-progress">
          <span className="continue-tile-bar" aria-hidden="true">
            <i style={{ width: `${percent ?? 0}%` }} />
          </span>
          <span>{typeof percent === 'number' ? `${percent}% пройдено` : '…'}</span>
        </span>
      </span>
    </Link>
  );
}
