import { useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import {
  ApiError,
  createCourse,
  createLesson,
  createModule,
  getCourse,
  myTeaching,
  submitCourse,
} from '../api/client';
import type { LessonKind } from '../api/client';
import { t } from '../i18n/t';

const courseSchema = z.object({
  title: z.string().min(3, 'Название слишком короткое'),
  description: z.string().min(10, 'Описание слишком короткое'),
  priceRub: z.coerce.number().min(0, 'Цена не может быть отрицательной'),
});

type CourseForm = z.infer<typeof courseSchema>;

const KINDS: LessonKind[] = ['video_text', 'live', 'quiz', 'code'];

export default function Constructor(): JSX.Element {
  const { id } = useParams();
  const isNew = !id || id === 'new';
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [message, setMessage] = useState('');
  const [moduleTitle, setModuleTitle] = useState('');
  const [lessonTitle, setLessonTitle] = useState('');
  const [lessonKind, setLessonKind] = useState<LessonKind>('video_text');
  const [lessonModuleId, setLessonModuleId] = useState('');

  const { register, handleSubmit, formState } = useForm<CourseForm>({
    resolver: zodResolver(courseSchema),
    defaultValues: { title: '', description: '', priceRub: 0 },
  });

  const teachingQuery = useQuery({ queryKey: ['teaching'], queryFn: myTeaching });
  const detailQuery = useQuery({
    queryKey: ['teach-course', id],
    queryFn: () => getCourse(findSlug() ?? ''),
    enabled: !isNew && Boolean(findSlug()),
  });

  function findSlug(): string | undefined {
    return teachingQuery.data?.find((c) => c.id === id)?.slug;
  }

  const createMutation = useMutation({
    mutationFn: (v: CourseForm) =>
      createCourse({
        title: v.title,
        description: v.description,
        priceCents: Math.round(v.priceRub * 100),
      }),
    onSuccess: (c) => {
      void qc.invalidateQueries({ queryKey: ['teaching'] });
      navigate(`/teach/${c.id}`);
    },
    onError: (e: unknown) => setMessage(e instanceof Error ? e.message : t('common.error')),
  });

  const activeCourseId = isNew ? undefined : id;

  const moduleMutation = useMutation({
    mutationFn: () => createModule(activeCourseId ?? '', { title: moduleTitle }),
    onSuccess: () => {
      setModuleTitle('');
      void qc.invalidateQueries({ queryKey: ['teach-course'] });
      void qc.invalidateQueries({ queryKey: ['teaching'] });
    },
    onError: (e: unknown) => setMessage(e instanceof Error ? e.message : t('common.error')),
  });

  const lessonMutation = useMutation({
    mutationFn: () =>
      createLesson(activeCourseId ?? '', {
        moduleId: lessonModuleId,
        title: lessonTitle,
        kind: lessonKind,
      }),
    onSuccess: () => {
      setLessonTitle('');
      void qc.invalidateQueries({ queryKey: ['teach-course'] });
    },
    onError: (e: unknown) => setMessage(e instanceof Error ? e.message : t('common.error')),
  });

  const submitMutation = useMutation({
    mutationFn: () => submitCourse(activeCourseId ?? ''),
    onSuccess: () => {
      setMessage('');
      void qc.invalidateQueries({ queryKey: ['teaching'] });
    },
    onError: (e: unknown) => {
      if (e instanceof ApiError && e.status === 400) setMessage(t('teach.hintEmptyCourse'));
      else setMessage(e instanceof Error ? e.message : t('common.error'));
    },
  });

  const modules = detailQuery.data?.modules ?? [];

  return (
    <main>
      <h1>{isNew ? t('teach.newTitle') : t('teach.create')}</h1>

      {isNew && (
        <form onSubmit={handleSubmit((v) => createMutation.mutate(v))}>
          <label>
            {t('teach.courseTitle')}
            <input {...register('title')} />
            {formState.errors.title && (
              <span className="error">{formState.errors.title.message}</span>
            )}
          </label>
          <label>
            {t('teach.courseDesc')}
            <textarea {...register('description')} rows={4} cols={50} />
            {formState.errors.description && (
              <span className="error">{formState.errors.description.message}</span>
            )}
          </label>
          <label>
            {t('teach.coursePrice')}
            <input type="number" min={0} {...register('priceRub')} />
          </label>
          <button type="submit" disabled={createMutation.isPending}>
            {createMutation.isPending ? t('teach.saving') : t('teach.save')}
          </button>
        </form>
      )}

      {!isNew && (
        <>
          <p>
            {t('teach.status')}: {detailQuery.data?.status ?? '…'}
          </p>
          <section>
            <h2>{t('teach.addModule')}</h2>
            <input
              value={moduleTitle}
              onChange={(e) => setModuleTitle(e.target.value)}
              placeholder={t('teach.moduleTitle')}
            />
            <button
              onClick={() => moduleMutation.mutate()}
              disabled={!moduleTitle.trim() || moduleMutation.isPending}
            >
              {t('teach.addModule')}
            </button>
          </section>

          <section>
            <h2>{t('teach.addLesson')}</h2>
            <label>
              {t('teach.addModule')}
              <select value={lessonModuleId} onChange={(e) => setLessonModuleId(e.target.value)}>
                <option value="">—</option>
                {modules.map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.title}
                  </option>
                ))}
              </select>
            </label>
            <input
              value={lessonTitle}
              onChange={(e) => setLessonTitle(e.target.value)}
              placeholder={t('teach.lessonTitle')}
            />
            <label>
              {t('teach.lessonKind')}
              <select
                value={lessonKind}
                onChange={(e) => setLessonKind(e.target.value as LessonKind)}
              >
                {KINDS.map((k) => (
                  <option key={k} value={k}>
                    {t(`kind.${k}`)}
                  </option>
                ))}
              </select>
            </label>
            <button
              onClick={() => lessonMutation.mutate()}
              disabled={!lessonModuleId || !lessonTitle.trim() || lessonMutation.isPending}
            >
              {t('teach.addLesson')}
            </button>
          </section>

          <section>
            <h2>{t('course.program')}</h2>
            {modules.map((m) => (
              <div key={m.id}>
                <h3>{m.title}</h3>
                <ul>
                  {m.lessons.map((l) => (
                    <li key={l.id}>
                      {l.title} — {t(`kind.${l.kind}`)}
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </section>

          <button onClick={() => submitMutation.mutate()} disabled={submitMutation.isPending}>
            {t('teach.submitReview')}
          </button>
        </>
      )}

      {message && <p role="status">{message}</p>}
    </main>
  );
}
