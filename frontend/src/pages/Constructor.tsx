// Конструктор курса (T06): /teach/new — создание курса + список моих курсов,
// /teach/:id — редактирование существующего курса (программа, модули, уроки,
// отправка на модерацию).
//
// Ограничения API (см. Task/00-api-contract.md): единственное управление
// контентом — создание модуля и урока. Переименование, удаление и изменение
// порядка модулей/уроков, а также редактирование после создания в API нет —
// в UI они не рисуются как рабочие действия (teach.apiLimitNote).
//
// Все тела запросов собираются только из полей, относящихся к kind урока:
// backend включает DisallowUnknownFields() — лишнее поле = 400.

import { useState } from 'react';
import type { FormEvent } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import type { FieldErrors } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import {
  ApiError,
  createCourse,
  createLesson,
  createModule,
  formatDate,
  formatPrice,
  getCourse,
  myTeaching,
  submitCourse,
} from '../api/client';
import type { Course, CourseStatus, CreateLessonInput, LessonKind } from '../api/client';
import { t } from '../i18n/t';
import '../styles/constructor.css';

// ---------- Константы ----------

const KINDS: LessonKind[] = ['video_text', 'live', 'quiz', 'code'];

// Языки кодовых заданий по контракту (п.10). Названия — технические идентификаторы.
const LANGUAGES = ['python', 'js', 'htmlcss'] as const;
type Language = (typeof LANGUAGES)[number];

const STATUS_LABELS: Record<CourseStatus, string> = {
  draft: t('teach.statusDraft'),
  review: t('teach.statusReview'),
  published: t('teach.statusPublished'),
  archived: t('teach.statusArchived'),
};

// Сообщения на русском без ключа словаря (см. отчёт T06: кандидаты в ru.json).
// Схема создания курса ниже — по образцу прежнего кода страницы.
const MSG_SUBMITTED = 'Курс отправлен на модерацию';
const MSG_CONTENT_LOCKED = 'Контент курса можно добавлять только в режиме черновика.';
const MSG_MODULE_REQUIRED = 'Укажите название модуля';
const MSG_LESSON_MODULE = 'Выберите модуль, к которому относится урок';
const MSG_LESSON_TITLE = 'Укажите название урока';
const MSG_VIDEO_EMPTY = 'Добавьте текст урока или ссылку на видео';
const MSG_LIVE_URL = 'Укажите ссылку на эфир';
const MSG_QUIZ_NO_QUESTIONS = 'Добавьте хотя бы один вопрос';
const MSG_QUIZ_TEXT = 'Укажите текст вопроса';
const MSG_QUIZ_OPTIONS = 'В вопросе должно быть минимум 2 варианта ответа';
const MSG_QUIZ_CORRECT = 'Отметьте правильный вариант ответа';
const MSG_CODE_NO_TESTS = 'Добавьте хотя бы один тест';
const MSG_TEST_NAME = 'Укажите название теста';
const MSG_TEST_EXPECTED = 'Укажите ожидаемый результат';

// ---------- Схема создания курса ----------

const courseSchema = z.object({
  title: z.string().min(3, 'Название слишком короткое'),
  description: z.string().min(10, 'Описание слишком короткое'),
  priceRub: z.coerce.number().min(0, 'Цена не может быть отрицательной'),
});

type CourseForm = z.infer<typeof courseSchema>;

// ---------- Состояние формы урока ----------

/** Вопрос квиза в форме (до отправки; id вопросов генерирует backend). */
interface QuestionState {
  text: string;
  options: string[];
  /** Индекс правильного варианта [0, options.length). */
  correct: number;
  explanation: string;
}

/** Тест кодового задания в форме. */
interface TestState {
  name: string;
  input: string;
  expected: string;
  hidden: boolean;
}

/** Полное состояние формы создания урока; отправляются только поля kind. */
interface LessonDraft {
  title: string;
  kind: LessonKind;
  moduleId: string;
  // video_text
  bodyMd: string;
  videoUrl: string;
  // live
  liveUrl: string;
  /** Значение input[type=datetime-local]; в запрос уходит как RFC3339. */
  startsAt: string;
  // quiz
  questions: QuestionState[];
  // code
  language: Language;
  template: string;
  tests: TestState[];
}

function emptyLesson(moduleId: string): LessonDraft {
  return {
    title: '',
    kind: 'video_text',
    moduleId,
    bodyMd: '',
    videoUrl: '',
    liveUrl: '',
    startsAt: '',
    questions: [{ text: '', options: ['', ''], correct: 0, explanation: '' }],
    language: 'python',
    template: '',
    tests: [{ name: '', input: '', expected: '', hidden: false }],
  };
}

// ---------- Валидация формы урока ----------

interface FieldError {
  id: string;
  message: string;
}

/**
 * Проверка формы урока до отправки. `id` — DOM-идентификатор поля,
 * по нему выводится сообщение и ставится фокус на первое ошибочное поле.
 */
function validateLesson(d: LessonDraft): { errors: Record<string, string>; first: string | null } {
  const list: FieldError[] = [];
  const put = (id: string, message: string): void => {
    list.push({ id, message });
  };

  if (!d.moduleId) put('lesson-module', MSG_LESSON_MODULE);
  if (!d.title.trim()) put('lesson-title', MSG_LESSON_TITLE);

  if (d.kind === 'video_text' && !d.bodyMd.trim() && !d.videoUrl.trim()) {
    put('lesson-body', MSG_VIDEO_EMPTY);
  }
  if (d.kind === 'live' && !d.liveUrl.trim()) {
    put('lesson-live-url', MSG_LIVE_URL);
  }
  if (d.kind === 'quiz') {
    if (d.questions.length === 0) put('lesson-quiz', MSG_QUIZ_NO_QUESTIONS);
    d.questions.forEach((q, qi) => {
      if (!q.text.trim()) put(`q-${qi}-text`, MSG_QUIZ_TEXT);
      const filled = q.options.filter((o) => o.trim()).length;
      if (filled < 2) put(`q-${qi}-options`, MSG_QUIZ_OPTIONS);
      if (q.correct < 0 || q.correct >= q.options.length) put(`q-${qi}-options`, MSG_QUIZ_CORRECT);
    });
  }
  if (d.kind === 'code') {
    if (d.tests.length === 0) put('code-tests', MSG_CODE_NO_TESTS);
    d.tests.forEach((x, ti) => {
      if (!x.name.trim()) put(`test-${ti}-name`, MSG_TEST_NAME);
      if (!x.expected.trim()) put(`test-${ti}-expected`, MSG_TEST_EXPECTED);
    });
  }

  const errors: Record<string, string> = {};
  for (const e of list) {
    if (!(e.id in errors)) errors[e.id] = e.message;
  }
  return { errors, first: list[0]?.id ?? null };
}

// ---------- Сборка тела запроса ----------

/** Локальное время datetime-local → RFC3339 (ISO с зоной). */
function toIso(local: string): string | undefined {
  if (!local) return undefined;
  const d = new Date(local);
  if (Number.isNaN(d.getTime())) return undefined;
  return d.toISOString();
}

/** Собирает CreateLessonInput строго из полей выбранного kind. */
function buildLessonInput(d: LessonDraft): CreateLessonInput {
  const base = { moduleId: d.moduleId, title: d.title.trim(), kind: d.kind };
  switch (d.kind) {
    case 'video_text': {
      const input: CreateLessonInput = { ...base };
      const body = d.bodyMd.trim();
      const video = d.videoUrl.trim();
      if (body) input.bodyMd = body;
      if (video) input.videoUrl = video;
      return input;
    }
    case 'live': {
      const input: CreateLessonInput = { ...base, liveUrl: d.liveUrl.trim() };
      const iso = toIso(d.startsAt);
      // startsAt шлём по контракту (п.10); backend его пока не сохраняет — см. отчёт.
      if (iso) input.startsAt = iso;
      return input;
    }
    case 'quiz': {
      return {
        ...base,
        quiz: {
          questions: d.questions.map((q) => {
            const question: NonNullable<CreateLessonInput['quiz']>['questions'][number] = {
              text: q.text.trim(),
              options: q.options.map((o) => o.trim()),
              correct: q.correct,
            };
            const explanation = q.explanation.trim();
            if (explanation) question.explanation = explanation;
            return question;
          }),
        },
      };
    }
    case 'code': {
      const codeTask: NonNullable<CreateLessonInput['codeTask']> = {
        language: d.language,
        tests: d.tests.map((x) => {
          const test = { name: x.name.trim(), input: x.input, expected: x.expected.trim() };
          return x.hidden ? { ...test, hidden: true } : test;
        }),
      };
      if (d.template.trim()) codeTask.template = d.template;
      return { ...base, codeTask };
    }
  }
}

// ---------- Утилиты ----------

/** Текст ошибки запроса: сеть / API / прочее. */
function errText(e: unknown): string {
  if (e instanceof TypeError) return t('error.network');
  if (e instanceof ApiError) return e.message;
  return t('common.error');
}

function focusField(id: string): void {
  document.getElementById(id)?.focus();
}

// ---------- Компонент ----------

export default function Constructor(): JSX.Element {
  const { id } = useParams();
  const isNew = !id || id === 'new';
  const navigate = useNavigate();
  const qc = useQueryClient();

  // Единое уведомление страницы (успех/ошибка).
  const [notice, setNotice] = useState<{ text: string; kind: 'ok' | 'error' } | null>(null);

  // ---------- Форма создания курса (/teach/new) ----------
  const { register, handleSubmit, formState } = useForm<CourseForm>({
    resolver: zodResolver(courseSchema),
    defaultValues: { title: '', description: '', priceRub: 0 },
  });

  function onInvalidCourse(errs: FieldErrors<CourseForm>): void {
    focusField(errs.title ? 'course-title' : errs.description ? 'course-desc' : 'course-price');
  }

  // ---------- Данные ----------
  // Ключи запросов сохранены: ['teaching'] и ['teach-course', id].
  const teachingQuery = useQuery({ queryKey: ['teaching'], queryFn: myTeaching });

  // Курс конструктора ищем по id из URL в своих курсах (myTeaching возвращает массив),
  // а детали с модулями берём через getCourse по slug.
  const ownCourse: Course | undefined = isNew
    ? undefined
    : teachingQuery.data?.find((c) => c.id === id);

  const detailQuery = useQuery({
    queryKey: ['teach-course', id],
    queryFn: () => getCourse(ownCourse?.slug ?? ''),
    enabled: !isNew && Boolean(ownCourse?.slug),
    retry: false,
  });

  const course = detailQuery.data;
  const modules = course?.modules ?? [];
  const editable = course?.status === 'draft';

  // ---------- Форма создания курса ----------
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
    onError: (e: unknown) => setNotice({ text: errText(e), kind: 'error' }),
  });

  // ---------- Модуль ----------
  const [moduleTitle, setModuleTitle] = useState('');
  const [moduleError, setModuleError] = useState('');

  const moduleMutation = useMutation({
    mutationFn: () => createModule(course?.id ?? '', { title: moduleTitle.trim() }),
    onSuccess: () => {
      setModuleTitle('');
      setModuleError('');
      setNotice({ text: t('teach.saved'), kind: 'ok' });
      void qc.invalidateQueries({ queryKey: ['teach-course'] });
      void qc.invalidateQueries({ queryKey: ['teaching'] });
    },
    onError: (e: unknown) => setNotice({ text: errText(e), kind: 'error' }),
  });

  function onModuleSubmit(e: FormEvent): void {
    e.preventDefault();
    if (!moduleTitle.trim()) {
      setModuleError(MSG_MODULE_REQUIRED);
      focusField('module-title');
      return;
    }
    setModuleError('');
    moduleMutation.mutate();
  }

  // ---------- Урок ----------
  const [lesson, setLesson] = useState<LessonDraft>(() => emptyLesson(''));
  const [lessonErrors, setLessonErrors] = useState<Record<string, string>>({});

  const lessonMutation = useMutation({
    mutationFn: (draft: LessonDraft) => createLesson(course?.id ?? '', buildLessonInput(draft)),
    onSuccess: () => {
      // moduleId сохраняем: можно добавить ещё урок в тот же модуль.
      setLesson((prev) => emptyLesson(prev.moduleId));
      setLessonErrors({});
      setNotice({ text: t('teach.saved'), kind: 'ok' });
      void qc.invalidateQueries({ queryKey: ['teach-course'] });
      void qc.invalidateQueries({ queryKey: ['teaching'] });
    },
    onError: (e: unknown) => setNotice({ text: errText(e), kind: 'error' }),
  });

  function onLessonSubmit(e: FormEvent): void {
    e.preventDefault();
    const { errors, first } = validateLesson(lesson);
    setLessonErrors(errors);
    if (first) {
      focusField(first);
      return;
    }
    lessonMutation.mutate(lesson);
  }

  function setLessonField<K extends keyof LessonDraft>(key: K, value: LessonDraft[K]): void {
    setLesson((prev) => ({ ...prev, [key]: value }));
  }

  function updateQuestion(qi: number, patch: Partial<QuestionState>): void {
    setLesson((prev) => ({
      ...prev,
      questions: prev.questions.map((q, i) => (i === qi ? { ...q, ...patch } : q)),
    }));
  }

  function addQuestion(): void {
    setLesson((prev) => ({
      ...prev,
      questions: [...prev.questions, { text: '', options: ['', ''], correct: 0, explanation: '' }],
    }));
  }

  function removeQuestion(qi: number): void {
    setLesson((prev) => ({ ...prev, questions: prev.questions.filter((_, i) => i !== qi) }));
  }

  function setOption(qi: number, oi: number, value: string): void {
    setLesson((prev) => ({
      ...prev,
      questions: prev.questions.map((q, i) =>
        i === qi ? { ...q, options: q.options.map((o, j) => (j === oi ? value : o)) } : q,
      ),
    }));
  }

  function addOption(qi: number): void {
    setLesson((prev) => ({
      ...prev,
      questions: prev.questions.map((q, i) => (i === qi ? { ...q, options: [...q.options, ''] } : q)),
    }));
  }

  function removeOption(qi: number, oi: number): void {
    setLesson((prev) => ({
      ...prev,
      questions: prev.questions.map((q, i) => {
        if (i !== qi) return q;
        const options = q.options.filter((_, j) => j !== oi);
        const correct = Math.max(0, Math.min(q.correct, options.length - 1));
        return { ...q, options, correct };
      }),
    }));
  }

  function updateTest(ti: number, patch: Partial<TestState>): void {
    setLesson((prev) => ({
      ...prev,
      tests: prev.tests.map((x, i) => (i === ti ? { ...x, ...patch } : x)),
    }));
  }

  function addTest(): void {
    setLesson((prev) => ({
      ...prev,
      tests: [...prev.tests, { name: '', input: '', expected: '', hidden: false }],
    }));
  }

  function removeTest(ti: number): void {
    setLesson((prev) => ({ ...prev, tests: prev.tests.filter((_, i) => i !== ti) }));
  }

  // ---------- Отправка курса на модерацию ----------
  function submitError(e: unknown): void {
    let text = errText(e);
    if (e instanceof ApiError && e.status === 400) text = `${text} ${t('teach.hintEmptyCourse')}`;
    setNotice({ text, kind: 'error' });
  }

  const submitMutation = useMutation({
    mutationFn: () => submitCourse(course?.id ?? ''),
    onSuccess: () => {
      setNotice({ text: MSG_SUBMITTED, kind: 'ok' });
      void qc.invalidateQueries({ queryKey: ['teach-course'] });
      void qc.invalidateQueries({ queryKey: ['teaching'] });
    },
    onError: submitError,
  });

  function onSubmitForReview(): void {
    if (window.confirm(t('teach.confirmSubmit'))) submitMutation.mutate();
  }

  // Отправка из карточки списка курсов.
  const listSubmitMutation = useMutation({
    mutationFn: (courseId: string) => submitCourse(courseId),
    onSuccess: () => {
      setNotice({ text: MSG_SUBMITTED, kind: 'ok' });
      void qc.invalidateQueries({ queryKey: ['teach-course'] });
      void qc.invalidateQueries({ queryKey: ['teaching'] });
    },
    onError: submitError,
  });

  function onListSubmit(courseId: string): void {
    if (window.confirm(t('teach.confirmSubmit'))) listSubmitMutation.mutate(courseId);
  }

  // ---------- Рендер ----------

  return (
    <main>
      <h1>{isNew ? t('teach.newTitle') : course?.title ?? t('teach.myCourse')}</h1>

      {/* ===== /teach/new: создание курса + список моих курсов ===== */}
      {isNew && (
        <>
          <form onSubmit={handleSubmit((v) => createMutation.mutate(v), onInvalidCourse)} noValidate>
            <label htmlFor="course-title">{t('teach.courseTitle')}</label>
            <input id="course-title" {...register('title')} />
            {formState.errors.title && <p className="error">{formState.errors.title.message}</p>}

            <label htmlFor="course-desc">{t('teach.courseDesc')}</label>
            <textarea id="course-desc" rows={4} {...register('description')} />
            {formState.errors.description && (
              <p className="error">{formState.errors.description.message}</p>
            )}

            <label htmlFor="course-price">{t('teach.coursePrice')}</label>
            <input id="course-price" type="number" min={0} {...register('priceRub')} />
            {formState.errors.priceRub && <p className="error">{formState.errors.priceRub.message}</p>}

            <button type="submit" disabled={createMutation.isPending}>
              {createMutation.isPending ? t('teach.saving') : t('teach.save')}
            </button>
          </form>

          <section className="teach-section">
            <h2>{t('nav.myCourses')}</h2>
            {teachingQuery.isLoading && <p>{t('common.loading')}</p>}
            {teachingQuery.isError && (
              <p className="error">
                {errText(teachingQuery.error)}{' '}
                <button type="button" onClick={() => void teachingQuery.refetch()}>
                  {t('common.retry')}
                </button>
              </p>
            )}
            {teachingQuery.data && teachingQuery.data.length === 0 && <p>{t('teach.empty')}</p>}
            {teachingQuery.data && teachingQuery.data.length > 0 && (
              <ul className="teach-card-grid">
                {teachingQuery.data.map((c) => (
                  <li key={c.id} className="card teach-card">
                    <h3>
                      <Link to={`/teach/${c.id}`}>{c.title}</Link>
                    </h3>
                    <p>
                      <span className={`teach-badge teach-badge-${c.status}`}>
                        {STATUS_LABELS[c.status]}
                      </span>{' '}
                      <strong>{formatPrice(c.priceCents)}</strong>
                    </p>
                    {c.status === 'draft' && c.moderationComment && (
                      <p className="teach-rejected">
                        {t('teach.rejected')}: {c.moderationComment}
                      </p>
                    )}
                    <p className="muted teach-note">
                      Обновлено: {formatDate(c.updatedAt)}
                    </p>
                    <div className="teach-card-actions">
                      <Link className="teach-link-btn" to={`/teach/${c.id}`}>
                        Редактировать
                      </Link>
                      {c.status === 'draft' && (
                        <button
                          type="button"
                          onClick={() => onListSubmit(c.id)}
                          disabled={
                            listSubmitMutation.isPending && listSubmitMutation.variables === c.id
                          }
                        >
                          {t('teach.submitReview')}
                        </button>
                      )}
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </section>
        </>
      )}

      {/* ===== /teach/:id: редактор курса ===== */}
      {!isNew && (
        <>
          {teachingQuery.isLoading && <p>{t('common.loading')}</p>}

          {teachingQuery.isError && !teachingQuery.isLoading && (
            <p className="error">
              {errText(teachingQuery.error)}{' '}
              <button type="button" onClick={() => void teachingQuery.refetch()}>
                {t('common.retry')}
              </button>
            </p>
          )}

          {teachingQuery.isSuccess && !ownCourse && (
            <section>
              <h2>{t('course.notFound')}</h2>
              <p className="muted">{t('course.notFoundHint')}</p>
              <p>
                <Link to="/teach">{t('nav.teach')}</Link>
              </p>
            </section>
          )}

          {ownCourse && detailQuery.isLoading && <p>{t('common.loading')}</p>}

          {ownCourse && detailQuery.isError && (
            <section>
              <h2>{t('common.error')}</h2>
              <p className="error">
                {detailQuery.error instanceof ApiError && detailQuery.error.status === 403
                  ? t('error.noAccess')
                  : detailQuery.error instanceof ApiError && detailQuery.error.status === 404
                    ? t('course.notFound')
                    : errText(detailQuery.error)}
              </p>
              <button type="button" onClick={() => void detailQuery.refetch()}>
                {t('common.retry')}
              </button>
            </section>
          )}

          {course && (
            <>
              <p>
                <span className={`teach-badge teach-badge-${course.status}`}>
                  {STATUS_LABELS[course.status]}
                </span>{' '}
                {t('teach.status')}: {STATUS_LABELS[course.status]} · {formatPrice(course.priceCents)}
              </p>

              {/* Комментарий модератора: цикл «отклонили → исправил». */}
              {course.status === 'draft' && course.moderationComment && (
                <section className="teach-banner" role="note">
                  <h2>{t('teach.rejected')}</h2>
                  <p>
                    <strong>{t('teach.moderationComment')}:</strong> {course.moderationComment}
                  </p>
                </section>
              )}

              <p>
                <Link to="/teach">{t('common.back')}</Link>
              </p>

              {/* Отправка на модерацию — только для черновика. */}
              {editable ? (
                <div className="teach-section">
                  <div className="teach-card-actions">
                    <button
                      type="button"
                      onClick={onSubmitForReview}
                      disabled={submitMutation.isPending}
                    >
                      {submitMutation.isPending ? t('teach.saving') : t('teach.submitReview')}
                    </button>
                  </div>
                  <p className="teach-note">{t('teach.confirmSubmit')}</p>
                </div>
              ) : (
                <p className="teach-note">{MSG_CONTENT_LOCKED}</p>
              )}

              {/* Создание модулей и уроков — только в черновике. */}
              {editable && (
                <>
                  <section className="teach-section">
                    <h2>{t('teach.addModule')}</h2>
                    <form onSubmit={onModuleSubmit} noValidate>
                      <label htmlFor="module-title">{t('teach.moduleTitle')}</label>
                      <input
                        id="module-title"
                        value={moduleTitle}
                        onChange={(e) => {
                          setModuleTitle(e.target.value);
                          setModuleError('');
                        }}
                      />
                      {moduleError && <p className="error">{moduleError}</p>}
                      <button
                        type="submit"
                        disabled={!moduleTitle.trim() || moduleMutation.isPending}
                      >
                        {moduleMutation.isPending ? t('teach.saving') : t('teach.addModule')}
                      </button>
                    </form>
                  </section>

                  <section className="teach-section">
                    <h2>{t('teach.addLesson')}</h2>
                    {modules.length === 0 && <p className="muted">{t('teach.hintEmptyCourse')}</p>}
                    <form onSubmit={onLessonSubmit} noValidate>
                      <label htmlFor="lesson-module">Модуль</label>
                      <select
                        id="lesson-module"
                        value={lesson.moduleId}
                        onChange={(e) => setLessonField('moduleId', e.target.value)}
                      >
                        <option value="">—</option>
                        {modules.map((m) => (
                          <option key={m.id} value={m.id}>
                            {m.title}
                          </option>
                        ))}
                      </select>
                      {lessonErrors['lesson-module'] && (
                        <p className="error">{lessonErrors['lesson-module']}</p>
                      )}

                      <label htmlFor="lesson-title">{t('teach.lessonTitle')}</label>
                      <input
                        id="lesson-title"
                        value={lesson.title}
                        onChange={(e) => setLessonField('title', e.target.value)}
                      />
                      {lessonErrors['lesson-title'] && (
                        <p className="error">{lessonErrors['lesson-title']}</p>
                      )}

                      <label htmlFor="lesson-kind">{t('teach.lessonKind')}</label>
                      <select
                        id="lesson-kind"
                        value={lesson.kind}
                        onChange={(e) => setLessonField('kind', e.target.value as LessonKind)}
                      >
                        {KINDS.map((k) => (
                          <option key={k} value={k}>
                            {t(`kind.${k}`)}
                          </option>
                        ))}
                      </select>

                      {lesson.kind === 'video_text' && (
                        <div id="lesson-body" className="teach-focusable" tabIndex={-1}>
                          <label htmlFor="lesson-body-md">{t('teach.bodyMd')}</label>
                          <textarea
                            id="lesson-body-md"
                            rows={5}
                            value={lesson.bodyMd}
                            onChange={(e) => setLessonField('bodyMd', e.target.value)}
                          />
                          <label htmlFor="lesson-video-url">{t('teach.videoUrl')}</label>
                          <input
                            id="lesson-video-url"
                            value={lesson.videoUrl}
                            onChange={(e) => setLessonField('videoUrl', e.target.value)}
                          />
                          {lessonErrors['lesson-body'] && (
                            <p className="error">{lessonErrors['lesson-body']}</p>
                          )}
                        </div>
                      )}

                      {lesson.kind === 'live' && (
                        <div>
                          <label htmlFor="lesson-live-url">{t('teach.liveUrl')}</label>
                          <input
                            id="lesson-live-url"
                            value={lesson.liveUrl}
                            onChange={(e) => setLessonField('liveUrl', e.target.value)}
                          />
                          {lessonErrors['lesson-live-url'] && (
                            <p className="error">{lessonErrors['lesson-live-url']}</p>
                          )}
                          <label htmlFor="lesson-starts-at">{t('teach.startsAt')}</label>
                          <input
                            id="lesson-starts-at"
                            type="datetime-local"
                            value={lesson.startsAt}
                            onChange={(e) => setLessonField('startsAt', e.target.value)}
                          />
                        </div>
                      )}

                      {lesson.kind === 'quiz' && (
                        <div id="lesson-quiz" className="teach-focusable" tabIndex={-1}>
                          <p className="teach-counter">Вопросов: {lesson.questions.length}</p>
                          {lessonErrors['lesson-quiz'] && (
                            <p className="error">{lessonErrors['lesson-quiz']}</p>
                          )}
                          {lesson.questions.map((q, qi) => (
                            <div key={qi} className="teach-q-box">
                              <label htmlFor={`q-${qi}-text`}>
                                {t('teach.questionText')} {qi + 1}
                              </label>
                              <input
                                id={`q-${qi}-text`}
                                value={q.text}
                                onChange={(e) => updateQuestion(qi, { text: e.target.value })}
                              />
                              {lessonErrors[`q-${qi}-text`] && (
                                <p className="error">{lessonErrors[`q-${qi}-text`]}</p>
                              )}

                              <fieldset
                                id={`q-${qi}-options`}
                                className="teach-options teach-focusable"
                                tabIndex={-1}
                              >
                                <legend>{t('teach.correctOption')}</legend>
                                {q.options.map((opt, oi) => (
                                  <div key={oi} className="teach-option-row">
                                    <label className="teach-correct-radio">
                                      <input
                                        type="radio"
                                        name={`q-correct-${qi}`}
                                        checked={q.correct === oi}
                                        onChange={() => updateQuestion(qi, { correct: oi })}
                                      />
                                      <span className="sr-only">{t('teach.correctOption')}</span>
                                    </label>
                                    <label htmlFor={`q-${qi}-opt-${oi}`}>
                                      {t('teach.questionOption')} {oi + 1}
                                    </label>
                                    <input
                                      id={`q-${qi}-opt-${oi}`}
                                      value={opt}
                                      onChange={(e) => setOption(qi, oi, e.target.value)}
                                    />
                                    <button type="button" onClick={() => removeOption(qi, oi)}>
                                      {t('common.delete')}
                                    </button>
                                  </div>
                                ))}
                                <button type="button" onClick={() => addOption(qi)}>
                                  {t('teach.addOption')}
                                </button>
                                {lessonErrors[`q-${qi}-options`] && (
                                  <p className="error">{lessonErrors[`q-${qi}-options`]}</p>
                                )}
                              </fieldset>

                              <label htmlFor={`q-${qi}-explanation`}>{t('lesson.explanation')}</label>
                              <textarea
                                id={`q-${qi}-explanation`}
                                rows={2}
                                value={q.explanation}
                                onChange={(e) => updateQuestion(qi, { explanation: e.target.value })}
                              />

                              <button type="button" onClick={() => removeQuestion(qi)}>
                                {t('teach.deleteQuestion')}
                              </button>
                            </div>
                          ))}
                          <button type="button" onClick={addQuestion}>
                            {t('teach.addQuestion')}
                          </button>
                        </div>
                      )}

                      {lesson.kind === 'code' && (
                        <div>
                          <label htmlFor="lesson-language">{t('lesson.codeLanguage')}</label>
                          <select
                            id="lesson-language"
                            value={lesson.language}
                            onChange={(e) => setLessonField('language', e.target.value as Language)}
                          >
                            {LANGUAGES.map((l) => (
                              <option key={l} value={l}>
                                {l}
                              </option>
                            ))}
                          </select>

                          <label htmlFor="lesson-template">{t('lesson.codeTemplate')}</label>
                          <textarea
                            id="lesson-template"
                            rows={4}
                            value={lesson.template}
                            onChange={(e) => setLessonField('template', e.target.value)}
                          />

                          <div id="code-tests" className="teach-focusable" tabIndex={-1}>
                            <h3>Тесты</h3>
                            {lesson.tests.map((x, ti) => (
                              <div key={ti} className="teach-test-box">
                                <label htmlFor={`test-${ti}-name`}>{t('teach.testName')}</label>
                                <input
                                  id={`test-${ti}-name`}
                                  value={x.name}
                                  onChange={(e) => updateTest(ti, { name: e.target.value })}
                                />
                                {lessonErrors[`test-${ti}-name`] && (
                                  <p className="error">{lessonErrors[`test-${ti}-name`]}</p>
                                )}

                                <label htmlFor={`test-${ti}-input`}>{t('teach.testInput')}</label>
                                <input
                                  id={`test-${ti}-input`}
                                  value={x.input}
                                  onChange={(e) => updateTest(ti, { input: e.target.value })}
                                />

                                <label htmlFor={`test-${ti}-expected`}>
                                  {t('teach.testExpected')}
                                </label>
                                <input
                                  id={`test-${ti}-expected`}
                                  value={x.expected}
                                  onChange={(e) => updateTest(ti, { expected: e.target.value })}
                                />
                                {lessonErrors[`test-${ti}-expected`] && (
                                  <p className="error">{lessonErrors[`test-${ti}-expected`]}</p>
                                )}

                                <label className="teach-check">
                                  <input
                                    type="checkbox"
                                    checked={x.hidden}
                                    onChange={(e) => updateTest(ti, { hidden: e.target.checked })}
                                  />
                                  {t('teach.testHidden')}
                                </label>

                                {lesson.tests.length > 1 && (
                                  <button type="button" onClick={() => removeTest(ti)}>
                                    {t('common.delete')}
                                  </button>
                                )}
                              </div>
                            ))}
                            {lessonErrors['code-tests'] && (
                              <p className="error">{lessonErrors['code-tests']}</p>
                            )}
                            <button type="button" onClick={addTest}>
                              Добавить тест
                            </button>
                          </div>
                        </div>
                      )}

                      <button type="submit" disabled={lessonMutation.isPending}>
                        {lessonMutation.isPending ? t('teach.saving') : t('teach.addLesson')}
                      </button>
                    </form>
                  </section>
                </>
              )}

              {/* Программа курса (только чтение). */}
              <section className="teach-section">
                <h2>{t('course.program')}</h2>
                {modules.length === 0 ? (
                  <p className="muted">{t('learn.empty')}</p>
                ) : (
                  <ol className="teach-modules">
                    {modules.map((m) => (
                      <li key={m.id} className="teach-module">
                        <h3>{m.title}</h3>
                        {m.lessons.length === 0 ? (
                          <p className="muted">{t('learn.noLessons')}</p>
                        ) : (
                          <ul className="teach-lesson-list">
                            {m.lessons.map((l) => (
                              <li key={l.id}>
                                <span className="teach-lesson-kind">{t(`kind.${l.kind}`)}</span>
                                {l.title}
                              </li>
                            ))}
                          </ul>
                        )}
                      </li>
                    ))}
                  </ol>
                )}
                <p className="teach-note">{t('teach.apiLimitNote')}</p>
              </section>
            </>
          )}
        </>
      )}

      {notice && (
        <p role="status" className={notice.kind === 'error' ? 'error' : 'teach-success'}>
          {notice.text}
        </p>
      )}
    </main>
  );
}
