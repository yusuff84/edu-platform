// Конструктор курса (T06): /teach/new — создание курса + список моих курсов,
// /teach/:id — редактирование существующего курса (программа, модули, уроки,
// отправка на модерацию).
//
// T19 закрыл бэклог T06: правка карточки и переименование модуля (T13),
// правка урока, удаление модулей/уроков/курса, изменение порядка, счётчики
// модулей и уроков в карточках списка. Контент (уроки, порядок, удаление)
// правится только в черновике — API отвечает 409 иначе; карточка курса и
// название модуля — метаданные, правятся в любом статусе.
//
// API-вызовы CRUD (PATCH/DELETE/reorder) объявлены локально: api/client.ts —
// чужая зона (T16), расширить его в T19 нельзя. crudFetch повторяет apiFetch.
//
// T28 достроил рабочее место учителя: предпросмотр черновика урока рядом с
// формой (components/LessonPreview.tsx), сводка структуры, перенос урока между
// модулями, перестановка вопросов/вариантов/тестов и жизненный цикл курса
// (копия, архив, отзыв с модерации). Операции T21 живут в api/teacherExtra.ts.
//
// T36: источник курса — GET /teacher/courses/{id} (владелец или админ) под
// ключом ['teach-course', id], общим с экраном «Ученики курса»: админ видит
// чужой курс, правки карточки подсвечиваются на обоих экранах. В архиве
// карточка заморожена (PATCH → 409, T33) — вместо кнопки правки подсказка
// teach.cardArchivedHint.
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
  myTeaching,
  submitCourse,
} from '../api/client';
import type { Course, CourseStatus, CreateLessonInput, Lesson, LessonKind, Module } from '../api/client';
import {
  archiveCourse,
  duplicateCourse,
  moveLesson as moveLessonToModule,
  unarchiveCourse,
  withdrawCourse,
} from '../api/teacherExtra';
// T36: детали курса конструктора — из api/teacherStudents.ts (client.ts не наш,
// добавление туда getTeacherCourseDetail принадлежит соседней волне).
import { getTeacherCourseDetail } from '../api/teacherStudents';
import LessonPreview from '../components/LessonPreview';
import type { LessonPreviewData } from '../components/LessonPreview';
import { t } from '../i18n/t';
import '../styles/constructor.css';
import '../styles/constructorExtra.css';

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
// T19: подтверждения удаления — teach.confirmDelete существует в словаре;
// отдельного ключа для модуля/урока нет (см. отчёт: teach.confirmDeleteModule).
const MSG_DELETE = t('teach.confirmDelete');

// ---------- T19: локальный транспорт CRUD (client.ts не менялся) ----------

const API_BASE: string =
  (import.meta.env.VITE_API_URL as string | undefined) ?? '/api/v1';

/** Локальный аналог apiFetch из api/client.ts (PATCH/DELETE/reorder). */
async function crudFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
    ...init,
  });
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  let data: unknown;
  try {
    data = text ? (JSON.parse(text) as unknown) : undefined;
  } catch {
    data = undefined;
  }
  if (!res.ok) {
    const errObj =
      typeof data === 'object' && data !== null ? (data as Record<string, unknown>) : {};
    let msg = typeof errObj.error === 'string' ? errObj.error : '';
    if (res.status === 401 && !msg) msg = 'Требуется вход в аккаунт';
    if (!msg) msg = `Ошибка запроса (${res.status})`;
    throw new ApiError(res.status, msg);
  }
  return data as T;
}

/** Строка GET /teacher/courses с T19-счётчиками программы. */
interface TeacherCourse extends Course {
  modulesCount?: number;
  lessonsCount?: number;
}

interface ReorderItem {
  id: string;
  position: number;
}

function patchCourseCard(
  courseId: string,
  body: { title?: string; description?: string; priceCents?: number },
): Promise<Course> {
  // Тело только из переданных полей: пустые не шлём, чтобы не затереть данные.
  const b: Record<string, unknown> = {};
  if (body.title !== undefined) b.title = body.title;
  if (body.description !== undefined) b.description = body.description;
  if (body.priceCents !== undefined) b.priceCents = body.priceCents;
  return crudFetch<Course>(`/teacher/courses/${courseId}`, {
    method: 'PATCH',
    body: JSON.stringify(b),
  });
}

function patchModuleTitle(courseId: string, moduleId: string, title: string): Promise<Module> {
  return crudFetch<Module>(`/teacher/courses/${courseId}/modules/${moduleId}`, {
    method: 'PATCH',
    body: JSON.stringify({ title }),
  });
}

function patchLesson(
  courseId: string,
  lessonId: string,
  body: Record<string, unknown>,
): Promise<Lesson> {
  return crudFetch<Lesson>(`/teacher/courses/${courseId}/lessons/${lessonId}`, {
    method: 'PATCH',
    body: JSON.stringify(body),
  });
}

function deleteCourse(courseId: string): Promise<void> {
  return crudFetch<void>(`/teacher/courses/${courseId}`, { method: 'DELETE' });
}

function deleteModule(courseId: string, moduleId: string): Promise<void> {
  return crudFetch<void>(`/teacher/courses/${courseId}/modules/${moduleId}`, {
    method: 'DELETE',
  });
}

function deleteLesson(courseId: string, lessonId: string): Promise<void> {
  return crudFetch<void>(`/teacher/courses/${courseId}/lessons/${lessonId}`, {
    method: 'DELETE',
  });
}

function reorderModules(courseId: string, items: ReorderItem[]): Promise<void> {
  return crudFetch<void>(`/teacher/courses/${courseId}/modules/reorder`, {
    method: 'POST',
    body: JSON.stringify({ items }),
  });
}

function reorderLessons(courseId: string, items: ReorderItem[]): Promise<void> {
  return crudFetch<void>(`/teacher/courses/${courseId}/lessons/reorder`, {
    method: 'POST',
    body: JSON.stringify({ items }),
  });
}

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
  /** T19: id исходного вопроса при редактировании — сохраняем, чтобы ответы студентов не «отвязались». */
  id?: string;
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

// ---------- T19: редактирование урока ----------

/** RFC3339 → значение input[type=datetime-local] (локальное время). */
function toLocalInput(iso: string | undefined): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const pad = (n: number): string => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(
    d.getMinutes(),
  )}`;
}

/**
 * Урок (полная форма владельца из GET /courses/{slug}) → состояние формы.
 * moduleId берём из модуля-владельца: в lessonView его нет.
 */
function lessonToDraft(l: Lesson, moduleId: string): LessonDraft {
  const d = emptyLesson(moduleId);
  d.title = l.title;
  d.kind = l.kind;
  d.bodyMd = l.bodyMd ?? '';
  d.videoUrl = l.videoUrl ?? '';
  d.liveUrl = l.liveUrl ?? '';
  d.startsAt = toLocalInput(l.startsAt);
  if (l.kind === 'quiz' && l.quiz) {
    d.questions = l.quiz.questions.map((q) => ({
      id: q.id,
      text: q.text,
      options: [...q.options],
      correct: q.correct ?? 0,
      explanation: q.explanation ?? '',
    }));
    if (d.questions.length === 0) {
      d.questions = [{ text: '', options: ['', ''], correct: 0, explanation: '' }];
    }
  }
  if (l.kind === 'code' && l.codeTask) {
    d.language = (LANGUAGES as readonly string[]).includes(l.codeTask.language)
      ? (l.codeTask.language as Language)
      : 'python';
    d.template = l.codeTask.template ?? '';
    d.tests = l.codeTask.tests.map((x) => ({
      name: x.name,
      input: x.input,
      expected: x.expected,
      hidden: x.hidden ?? false,
    }));
    if (d.tests.length === 0) {
      d.tests = [{ name: '', input: '', expected: '', hidden: false }];
    }
  }
  return d;
}

/**
 * T28: черновик формы → данные предпросмотра. Отдаём только поля текущего
 * kind — чужие блоки (quiz при video_text и т.п.) в панель не попадают.
 */
function lessonToPreview(d: LessonDraft): LessonPreviewData {
  const data: LessonPreviewData = {
    title: d.title,
    kind: d.kind,
    bodyMd: d.bodyMd,
    videoUrl: d.videoUrl,
    liveUrl: d.liveUrl,
    startsAt: d.startsAt,
  };
  if (d.kind === 'quiz') {
    data.quiz = {
      questions: d.questions.map((q) => {
        const question: NonNullable<LessonPreviewData['quiz']>['questions'][number] = {
          text: q.text,
          options: [...q.options],
          correct: q.correct,
        };
        if (q.explanation) question.explanation = q.explanation;
        return question;
      }),
    };
  }
  if (d.kind === 'code') {
    const codeTask: NonNullable<LessonPreviewData['codeTask']> = {
      language: d.language,
      tests: d.tests.map((x) => {
        const test: { name: string; input: string; expected: string; hidden?: boolean } = {
          name: x.name,
          input: x.input,
          expected: x.expected,
        };
        if (x.hidden) test.hidden = true;
        return test;
      }),
    };
    if (d.template) codeTask.template = d.template;
    data.codeTask = codeTask;
  }
  return data;
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
      // startsAt шлём по контракту (п.10). Сохранение починено в T16
      // (backend/internal/handler/teacher.go) — значение доезжает до урока и валидируется.
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
      if (d.template.trim()) codeTask.template = d.template.trim();
      return { ...base, codeTask };
    }
  }
}

/**
 * T19: тело PATCH .../lessons/{id}. В отличие от создания, отправляем ВСЕ
 * поля kind — пустая строка очищает значение на backend (merge по указателям).
 * moduleId шлём для совместимости с формой (backend его игнорирует), kind —
 * текущий: смена типа урока запрещена (400).
 */
function buildLessonPatchBody(d: LessonDraft): Record<string, unknown> {
  const body: Record<string, unknown> = {
    moduleId: d.moduleId,
    title: d.title.trim(),
    kind: d.kind,
  };
  switch (d.kind) {
    case 'video_text':
      body.bodyMd = d.bodyMd.trim();
      body.videoUrl = d.videoUrl.trim();
      break;
    case 'live':
      body.liveUrl = d.liveUrl.trim();
      // Пустая строка — очистить дату эфира.
      body.startsAt = toIso(d.startsAt) ?? '';
      break;
    case 'quiz':
      body.quiz = {
        questions: d.questions.map((q) => {
          const question: Record<string, unknown> = {
            text: q.text.trim(),
            options: q.options.map((o) => o.trim()),
            correct: q.correct,
          };
          // Сохраняем id исходных вопросов: ответы студентов привязаны к ним.
          if (q.id) question.id = q.id;
          const explanation = q.explanation.trim();
          if (explanation) question.explanation = explanation;
          return question;
        }),
      };
      break;
    case 'code':
      body.codeTask = {
        language: d.language,
        template: d.template.trim(),
        tests: d.tests.map((x) => {
          const test: Record<string, unknown> = {
            name: x.name.trim(),
            input: x.input,
            expected: x.expected.trim(),
          };
          if (x.hidden) test.hidden = true;
          return test;
        }),
      };
      break;
  }
  return body;
}

// ---------- Утилиты ----------

/** Текст ошибки запроса: сеть / API / прочее. */
function errText(e: unknown): string {
  if (e instanceof TypeError) return t('error.network');
  if (e instanceof ApiError) return e.message;
  return t('common.error');
}

/**
 * T36: текст ошибки загрузки курса конструктора. 403/404 — курса нет или нет
 * доступа к нему (ownCourse на бэкенде), прочие сбои — общая формулировка.
 */
function courseErrorText(e: unknown): string {
  if (e instanceof ApiError && (e.status === 403 || e.status === 404)) {
    return t('courseStudents.noAccess');
  }
  return t('teach.courseLoadFailed');
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
  // T36: курс конструктора грузится напрямую из GET /teacher/courses/{id}
  // (владелец или админ) под ключом ['teach-course', id] — тем же, что и на
  // экране «Ученики курса»: правки карточки видны обоим экранам сразу.
  const detailQuery = useQuery({
    queryKey: ['teach-course', id],
    queryFn: () => getTeacherCourseDetail(id ?? ''),
    enabled: !isNew,
    retry: false,
  });

  const course = detailQuery.data?.course;
  const modules = detailQuery.data?.modules ?? [];
  const editable = course?.status === 'draft';
  // T36: в архиве карточка заморожена — PATCH отвечает 409 (контракт T33),
  // поэтому форму правки заранее не показываем, а выводим подсказку.
  const cardFrozen = course?.status === 'archived';

  // Список своих курсов нужен странице /teach/new (и инвалидациям после
  // мутаций): курс конструктора из него больше не ищется.
  const teachingQuery = useQuery({ queryKey: ['teaching'], queryFn: myTeaching });
  // T28: сводка «модулей/уроков» считается по уже загруженному конверту
  // { course, modules } (T36: тот же ответ, что и у экрана «Ученики курса»).
  const lessonsTotal = modules.reduce((n, m) => n + m.lessons.length, 0);

  // ---------- T19: локальное состояние CRUD ----------

  // Правка карточки курса (T13 PATCH /teacher/courses/{id}): в любом статусе,
  // кроме архива (T36/T33 — там карточка заморожена и PATCH отвечает 409).
  const [cardEditing, setCardEditing] = useState(false);
  const [cardDraft, setCardDraft] = useState({ title: '', description: '', priceRub: 0 });

  // Переименование модуля (T13 PATCH .../modules/{moduleID}): в любом статусе.
  const [renamingModuleId, setRenamingModuleId] = useState<string | null>(null);
  const [renameTitle, setRenameTitle] = useState('');

  // Правка урока (T19 PATCH .../lessons/{lessonID}): только в черновике.
  // Форма создания и редактирования — одна; editId !== null = режим правки.
  const [editingLessonId, setEditingLessonId] = useState<string | null>(null);

  // T28: раскрыта ли панель предпросмотра черновика урока (рядом с формой).
  const [previewOpen, setPreviewOpen] = useState(false);

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

  // ---------- T19: карточка курса ----------

  const cardMutation = useMutation({
    mutationFn: () =>
      patchCourseCard(course?.id ?? '', {
        title: cardDraft.title.trim(),
        description: cardDraft.description.trim(),
        priceCents: Math.round(cardDraft.priceRub * 100),
      }),
    onSuccess: () => {
      setCardEditing(false);
      setNotice({ text: t('teach.saved'), kind: 'ok' });
      void qc.invalidateQueries({ queryKey: ['teaching'] });
      void qc.invalidateQueries({ queryKey: ['teach-course'] });
    },
    onError: (e: unknown) => setNotice({ text: errText(e), kind: 'error' }),
  });

  function startCardEdit(): void {
    if (!course || cardFrozen) return;
    setCardDraft({
      title: course.title,
      description: course.description,
      priceRub: course.priceCents / 100,
    });
    setCardEditing(true);
  }

  function onCardSubmit(e: FormEvent): void {
    e.preventDefault();
    if (
      cardDraft.title.trim().length < 3 ||
      cardDraft.description.trim().length < 10 ||
      cardDraft.priceRub < 0
    ) {
      setNotice({ text: 'Проверьте поля карточки: название от 3 символов, описание от 10, цена не меньше 0', kind: 'error' });
      return;
    }
    cardMutation.mutate();
  }

  // ---------- T19: переименование модуля ----------

  const renameMutation = useMutation({
    mutationFn: (v: { moduleId: string; title: string }) =>
      patchModuleTitle(course?.id ?? '', v.moduleId, v.title),
    onSuccess: () => {
      setRenamingModuleId(null);
      setNotice({ text: t('teach.saved'), kind: 'ok' });
      void qc.invalidateQueries({ queryKey: ['teach-course'] });
      void qc.invalidateQueries({ queryKey: ['teaching'] });
    },
    onError: (e: unknown) => setNotice({ text: errText(e), kind: 'error' }),
  });

  function onRenameSubmit(e: FormEvent): void {
    e.preventDefault();
    if (!renamingModuleId || !renameTitle.trim()) return;
    renameMutation.mutate({ moduleId: renamingModuleId, title: renameTitle.trim() });
  }

  // ---------- T19: удаления (курс, модуль, урок) ----------

  const deleteCourseMutation = useMutation({
    mutationFn: (courseId: string) => deleteCourse(courseId),
    onSuccess: () => {
      setNotice({ text: 'Курс удалён', kind: 'ok' });
      void qc.invalidateQueries({ queryKey: ['teaching'] });
      void qc.invalidateQueries({ queryKey: ['teach-course'] });
    },
    onError: (e: unknown) => setNotice({ text: errText(e), kind: 'error' }),
  });

  function onDeleteCourse(courseId: string): void {
    if (window.confirm(MSG_DELETE)) deleteCourseMutation.mutate(courseId);
  }

  const deleteModuleMutation = useMutation({
    mutationFn: (moduleId: string) => deleteModule(course?.id ?? '', moduleId),
    onSuccess: () => {
      setNotice({ text: 'Модуль удалён', kind: 'ok' });
      void qc.invalidateQueries({ queryKey: ['teach-course'] });
      void qc.invalidateQueries({ queryKey: ['teaching'] });
    },
    onError: (e: unknown) => setNotice({ text: errText(e), kind: 'error' }),
  });

  function onDeleteModule(moduleId: string): void {
    if (window.confirm(MSG_DELETE)) deleteModuleMutation.mutate(moduleId);
  }

  const deleteLessonMutation = useMutation({
    mutationFn: (lessonId: string) => deleteLesson(course?.id ?? '', lessonId),
    onSuccess: (_data, lessonId) => {
      // Если удалили редактируемый урок — сбрасываем форму в режим создания.
      if (editingLessonId === lessonId) {
        setEditingLessonId(null);
        setLesson((prev) => emptyLesson(prev.moduleId));
        setLessonErrors({});
      }
      setNotice({ text: 'Урок удалён', kind: 'ok' });
      void qc.invalidateQueries({ queryKey: ['teach-course'] });
      void qc.invalidateQueries({ queryKey: ['teaching'] });
    },
    onError: (e: unknown) => setNotice({ text: errText(e), kind: 'error' }),
  });

  function onDeleteLesson(lessonId: string): void {
    if (window.confirm(MSG_DELETE)) deleteLessonMutation.mutate(lessonId);
  }

  // ---------- T19: порядок модулей и уроков ----------

  const reorderModulesMutation = useMutation({
    mutationFn: (items: ReorderItem[]) => reorderModules(course?.id ?? '', items),
    onSuccess: () => {
      setNotice({ text: 'Порядок сохранён', kind: 'ok' });
      void qc.invalidateQueries({ queryKey: ['teach-course'] });
    },
    onError: (e: unknown) => setNotice({ text: errText(e), kind: 'error' }),
  });

  /** Сдвиг модуля на ±1: шлём полный список позиций 1..N (идемпотентно). */
  function moveModule(index: number, dir: -1 | 1): void {
    const next = [...modules];
    const j = index + dir;
    if (j < 0 || j >= next.length) return;
    [next[index], next[j]] = [next[j], next[index]];
    reorderModulesMutation.mutate(next.map((m, i) => ({ id: m.id, position: i + 1 })));
  }

  const reorderLessonsMutation = useMutation({
    mutationFn: (items: ReorderItem[]) => reorderLessons(course?.id ?? '', items),
    onSuccess: () => {
      setNotice({ text: 'Порядок сохранён', kind: 'ok' });
      void qc.invalidateQueries({ queryKey: ['teach-course'] });
    },
    onError: (e: unknown) => setNotice({ text: errText(e), kind: 'error' }),
  });

  /**
   * Сдвиг урока на ±1 внутри модуля. Позиция — порядковый номер внутри модуля;
   * API ждёт полный список уроков курса, поэтому пересобираем все модули.
   */
  function moveLesson(moduleIndex: number, lessonIndex: number, dir: -1 | 1): void {
    const items: ReorderItem[] = [];
    for (let mi = 0; mi < modules.length; mi++) {
      const lessons = [...modules[mi].lessons];
      if (mi === moduleIndex) {
        const j = lessonIndex + dir;
        if (j < 0 || j >= lessons.length) return;
        [lessons[lessonIndex], lessons[j]] = [lessons[j], lessons[lessonIndex]];
      }
      lessons.forEach((l, i) => items.push({ id: l.id, position: i + 1 }));
    }
    reorderLessonsMutation.mutate(items);
  }

  // ---------- T28: жизненный цикл курса (контракт T21) ----------
  // Кнопки показываются строго по матрице статусов из ТЗ, подтверждения —
  // ключи teach.confirm*, сообщения об успехе — teach.*ed.

  const duplicateMutation = useMutation({
    mutationFn: () => duplicateCourse(course?.id ?? ''),
    onSuccess: (res) => {
      // Переходим на копию с чистым редактором урока и закрытым предпросмотром.
      // moduleId копии другой — сбрасываем его (пустая строка), а не оставляем
      // id модуля исходного курса.
      setEditingLessonId(null);
      setLesson(emptyLesson(''));
      setLessonErrors({});
      setPreviewOpen(false);
      setNotice({ text: t('teach.duplicated'), kind: 'ok' });
      // T36: копия грузится конструктором из GET /teacher/courses/{id} по своему
      // id, поэтому дожидаться обновления списка ['teaching'] не нужно — просто
      // помечаем его устаревшим.
      void qc.invalidateQueries({ queryKey: ['teaching'] });
      void qc.invalidateQueries({ queryKey: ['teach-course'] });
      navigate(`/teach/${res.course.id}`);
    },
    onError: (e: unknown) => setNotice({ text: errText(e), kind: 'error' }),
  });

  function onDuplicateCourse(): void {
    if (window.confirm(t('teach.confirmDuplicate'))) duplicateMutation.mutate();
  }

  const archiveMutation = useMutation({
    mutationFn: () => archiveCourse(course?.id ?? ''),
    onSuccess: () => {
      setNotice({ text: t('teach.archived'), kind: 'ok' });
      void qc.invalidateQueries({ queryKey: ['teach-course'] });
      void qc.invalidateQueries({ queryKey: ['teaching'] });
    },
    onError: (e: unknown) => setNotice({ text: errText(e), kind: 'error' }),
  });

  function onArchiveCourse(): void {
    if (window.confirm(t('teach.confirmArchive'))) archiveMutation.mutate();
  }

  const unarchiveMutation = useMutation({
    mutationFn: () => unarchiveCourse(course?.id ?? ''),
    onSuccess: () => {
      setNotice({ text: t('teach.unarchived'), kind: 'ok' });
      void qc.invalidateQueries({ queryKey: ['teach-course'] });
      void qc.invalidateQueries({ queryKey: ['teaching'] });
    },
    onError: (e: unknown) => setNotice({ text: errText(e), kind: 'error' }),
  });

  // Возврат из архива — обратимое действие, но подтверждение ставим: учитель
  // может нажать случайно, а курс сразу станет черновиком.
  function onUnarchiveCourse(): void {
    if (window.confirm(t('teach.confirmUnarchive'))) unarchiveMutation.mutate();
  }

  const withdrawMutation = useMutation({
    mutationFn: () => withdrawCourse(course?.id ?? ''),
    onSuccess: () => {
      setNotice({ text: t('teach.withdrawn'), kind: 'ok' });
      void qc.invalidateQueries({ queryKey: ['teach-course'] });
      void qc.invalidateQueries({ queryKey: ['teaching'] });
    },
    onError: (e: unknown) => setNotice({ text: errText(e), kind: 'error' }),
  });

  function onWithdrawCourse(): void {
    if (window.confirm(t('teach.confirmWithdraw'))) withdrawMutation.mutate();
  }

  // ---------- T28: перенос урока в другой модуль (T21 move) ----------

  const moveLessonMutation = useMutation({
    mutationFn: (v: { lessonId: string; moduleId: string }) =>
      moveLessonToModule(course?.id ?? '', v.lessonId, v.moduleId),
    onSuccess: (_data, v) => {
      // Форма правки продолжает указывать на урок — обновляем его модуль.
      setLesson((prev) => ({ ...prev, moduleId: v.moduleId }));
      setNotice({ text: t('teach.moveLessonDone'), kind: 'ok' });
      void qc.invalidateQueries({ queryKey: ['teach-course'] });
      void qc.invalidateQueries({ queryKey: ['teaching'] });
    },
    onError: (e: unknown) => setNotice({ text: errText(e), kind: 'error' }),
  });

  /** Выбор в селекте «Перенести в модуль…»; пустое значение — no-op. */
  function onMoveLessonToModule(moduleId: string): void {
    if (!editingLessonId || !moduleId) return;
    moveLessonMutation.mutate({ lessonId: editingLessonId, moduleId });
  }

  // ---------- Урок ----------
  const [lesson, setLesson] = useState<LessonDraft>(() => emptyLesson(''));
  const [lessonErrors, setLessonErrors] = useState<Record<string, string>>({});

  // editId !== null — режим правки (T19 PATCH), иначе создание.
  const lessonMutation = useMutation({
    mutationFn: (v: { draft: LessonDraft; editId: string | null }) =>
      v.editId
        ? patchLesson(course?.id ?? '', v.editId, buildLessonPatchBody(v.draft))
        : createLesson(course?.id ?? '', buildLessonInput(v.draft)),
    onSuccess: (_data, v) => {
      // moduleId сохраняем: можно добавить ещё урок в тот же модуль.
      setEditingLessonId(null);
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
    lessonMutation.mutate({ draft: lesson, editId: editingLessonId });
  }

  /** T19: переход в режим правки урока — форма заполняется текущими данными. */
  function startEditLesson(l: Lesson, moduleId: string): void {
    setEditingLessonId(l.id);
    setLesson(lessonToDraft(l, moduleId));
    setLessonErrors({});
    setNotice(null);
    // Форма урока выше программы — прокручиваем к ней и ставим фокус.
    focusField('lesson-title');
    document.getElementById('lesson-form')?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }

  /** T19: выход из режима правки без сохранения. */
  function cancelEditLesson(): void {
    setEditingLessonId(null);
    setLesson((prev) => emptyLesson(prev.moduleId));
    setLessonErrors({});
    setNotice(null);
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

  /**
   * T28: перестановка вопросов квиза. Меняем местами целиком объекты
   * QuestionState — `correct` хранится внутри вопроса и едет вместе с ним,
   * отдельный пересчёт индекса не нужен.
   */
  function moveQuestion(qi: number, dir: -1 | 1): void {
    setLesson((prev) => {
      const j = qi + dir;
      if (j < 0 || j >= prev.questions.length) return prev;
      const questions = [...prev.questions];
      [questions[qi], questions[j]] = [questions[j], questions[qi]];
      return { ...prev, questions };
    });
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

  /**
   * T28: перестановка вариантов ответа с сохранением ключа. `correct` — это
   * индекс варианта, поэтому после обмена соседних позиций его нужно
   * пересчитать: если правильный вариант уехал на новую позицию — correct
   * получает её; если обмен его не касался — индекс не меняется.
   */
  function moveOption(qi: number, oi: number, dir: -1 | 1): void {
    setLesson((prev) => ({
      ...prev,
      questions: prev.questions.map((q, i) => {
        if (i !== qi) return q;
        const j = oi + dir;
        if (j < 0 || j >= q.options.length) return q;
        const options = [...q.options];
        [options[oi], options[j]] = [options[j], options[oi]];
        // Инвариант: correct указывает на тот же по смыслу вариант, что и до
        // перестановки (меняется только его позиция).
        let correct = q.correct;
        if (correct === oi) correct = j;
        else if (correct === j) correct = oi;
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

  /**
   * T28: перестановка тестов код-задачи. У теста нет ссылочного индекса —
   * порядок тестов и есть порядок проверки, поэтому достаточно обмена
   * соседних элементов массива.
   */
  function moveTest(ti: number, dir: -1 | 1): void {
    setLesson((prev) => {
      const j = ti + dir;
      if (j < 0 || j >= prev.tests.length) return prev;
      const tests = [...prev.tests];
      [tests[ti], tests[j]] = [tests[j], tests[ti]];
      return { ...prev, tests };
    });
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
    <main className="wrap">
      <div className="page">
      <h1>{isNew ? t('teach.newTitle') : course?.title ?? t('teach.myCourse')}</h1>

      {/* ===== /teach/new: создание курса + список моих курсов ===== */}
      {isNew && (
        <>
          <form className="form-grid" onSubmit={handleSubmit((v) => createMutation.mutate(v), onInvalidCourse)} noValidate>
            <label htmlFor="course-title">{t('teach.courseTitle')}</label>
            <input id="course-title" className="input" {...register('title')} />
            {formState.errors.title && <p className="error">{formState.errors.title.message}</p>}

            <label htmlFor="course-desc">{t('teach.courseDesc')}</label>
            <textarea id="course-desc" className="input" rows={4} {...register('description')} />
            {formState.errors.description && (
              <p className="error">{formState.errors.description.message}</p>
            )}

            <label htmlFor="course-price">{t('teach.coursePrice')}</label>
            <input id="course-price" className="input" type="number" min={0} {...register('priceRub')} />
            {formState.errors.priceRub && <p className="error">{formState.errors.priceRub.message}</p>}

            <button type="submit" className="btn btn-primary" disabled={createMutation.isPending}>
              {createMutation.isPending ? t('teach.saving') : t('teach.save')}
            </button>
          </form>

          <section className="teach-section panel">
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
                {teachingQuery.data.map((raw) => {
                  // Счётчики T19 приходят в элементах списка (GET /teacher/courses).
                  const c = raw as TeacherCourse;
                  return (
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
                      Модулей: {c.modulesCount ?? 0} · Уроков: {c.lessonsCount ?? 0} · Обновлено:{' '}
                      {formatDate(c.updatedAt)}
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
                      {c.status === 'draft' && (
                        <button
                          type="button"
                          className="teach-danger"
                          onClick={() => onDeleteCourse(c.id)}
                          disabled={
                            deleteCourseMutation.isPending &&
                            deleteCourseMutation.variables === c.id
                          }
                        >
                          {t('common.delete')}
                        </button>
                      )}
                    </div>
                  </li>
                  );
                })}
              </ul>
            )}
          </section>
        </>
      )}

      {/* ===== /teach/:id: редактор курса ===== */}
      {!isNew && (
        <>
          {/* T36: курс грузится напрямую по id — сначала загрузка, затем ошибка
              (403/404 — курса нет или нет доступа, прочее — сбой загрузки)
              либо сам курс. */}
          {detailQuery.isLoading && <p role="status">{t('teach.courseLoading')}</p>}

          {detailQuery.isError && (
            <section>
              <h2>{t('common.error')}</h2>
              <p className="error">{courseErrorText(detailQuery.error)}</p>
              <p>
                <button type="button" onClick={() => void detailQuery.refetch()}>
                  {t('common.retry')}
                </button>
              </p>
              <p>
                <Link to="/teach">{t('nav.teach')}</Link>
              </p>
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

              {/* T28: сводка структуры — по уже загруженному CourseDetail. */}
              <p className="ce-structure">
                <strong>{t('teach.structure')}:</strong> {t('course.modulesCount')}:{' '}
                {modules.length} · {t('teach.lessonsCount')}: {lessonsTotal}
              </p>

              {/* T28: жизненный цикл — набор кнопок строго по статусу (матрица ТЗ):
                  draft → копия/архив, review → отзыв, published → копия/архив/
                  публичная страница, archived → возврат из архива/копия. */}
              <div className="ce-lifecycle">
                {course.status === 'archived' && (
                  <button
                    type="button"
                    onClick={onUnarchiveCourse}
                    disabled={unarchiveMutation.isPending}
                  >
                    {unarchiveMutation.isPending ? t('teach.saving') : t('teach.unarchive')}
                  </button>
                )}
                {course.status === 'review' && (
                  <button
                    type="button"
                    onClick={onWithdrawCourse}
                    disabled={withdrawMutation.isPending}
                  >
                    {withdrawMutation.isPending ? t('teach.saving') : t('teach.withdraw')}
                  </button>
                )}
                {(course.status === 'draft' ||
                  course.status === 'published' ||
                  course.status === 'archived') && (
                  <button
                    type="button"
                    onClick={onDuplicateCourse}
                    disabled={duplicateMutation.isPending}
                  >
                    {duplicateMutation.isPending ? t('teach.saving') : t('teach.duplicate')}
                  </button>
                )}
                {(course.status === 'draft' || course.status === 'published') && (
                  <button
                    type="button"
                    onClick={onArchiveCourse}
                    disabled={archiveMutation.isPending}
                  >
                    {archiveMutation.isPending ? t('teach.saving') : t('teach.archive')}
                  </button>
                )}
                {course.status === 'published' && (
                  <a
                    className="ce-public-link"
                    href={`/courses/${course.slug}`}
                    target="_blank"
                    rel="noreferrer"
                  >
                    {t('teach.publicPage')}
                  </a>
                )}
              </div>

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
                {' · '}
                {/* T29: список учеников курса — отдельная страница. */}
                <Link to={`/teach/${course.id}/students`}>{t('courseStudents.title')}</Link>
              </p>

              {/* T19: правка карточки (T13 PATCH) — метаданные, доступна в любом
                  статусе, кроме архива: T36/T33 — там карточка заморожена
                  (PATCH отвечает 409), вместо кнопки выводится подсказка.
                  Удаление курса в архиве остаётся доступным. */}
              <section className="teach-section panel">
                <h2>{t('teach.courseTitle')}</h2>
                {cardFrozen && <p className="teach-note">{t('teach.cardArchivedHint')}</p>}
                {cardEditing && !cardFrozen ? (
                  <form className="form-grid" onSubmit={onCardSubmit} noValidate>
                    <label htmlFor="card-title">{t('teach.courseTitle')}</label>
                    <input
                      id="card-title"
                      className="input"
                      value={cardDraft.title}
                      onChange={(e) => setCardDraft((p) => ({ ...p, title: e.target.value }))}
                    />
                    <label htmlFor="card-desc">{t('teach.courseDesc')}</label>
                    <textarea
                      id="card-desc"
                      className="input"
                      rows={4}
                      value={cardDraft.description}
                      onChange={(e) => setCardDraft((p) => ({ ...p, description: e.target.value }))}
                    />
                    <label htmlFor="card-price">{t('teach.coursePrice')}</label>
                    <input
                      id="card-price"
                      className="input"
                      type="number"
                      min={0}
                      value={cardDraft.priceRub}
                      onChange={(e) =>
                        setCardDraft((p) => ({ ...p, priceRub: Number(e.target.value) }))
                      }
                    />
                    <div className="teach-card-actions">
                      <button
                        type="submit"
                        className="btn btn-primary"
                        disabled={cardMutation.isPending}
                      >
                        {cardMutation.isPending ? t('teach.saving') : t('teach.save')}
                      </button>
                      <button type="button" onClick={() => setCardEditing(false)}>
                        {t('common.cancel')}
                      </button>
                    </div>
                  </form>
                ) : (
                  <div className="teach-card-actions">
                    {!cardFrozen && (
                      <button type="button" onClick={startCardEdit}>
                        Редактировать карточку
                      </button>
                    )}
                    <button
                      type="button"
                      className="teach-danger"
                      onClick={() => onDeleteCourse(course.id)}
                      disabled={deleteCourseMutation.isPending}
                    >
                      {t('common.delete')} курс
                    </button>
                  </div>
                )}
              </section>

              {/* Отправка на модерацию — только для черновика. */}
              {editable ? (
                <div className="teach-section panel">
                  <div className="teach-card-actions">
                    <button
                      type="button"
                      className="btn btn-primary"
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
                  <section className="teach-section panel">
                    <h2>{t('teach.addModule')}</h2>
                    <form onSubmit={onModuleSubmit} noValidate>
                      <label htmlFor="module-title">{t('teach.moduleTitle')}</label>
                      <input
                        id="module-title"
                        className="input"
                        value={moduleTitle}
                        onChange={(e) => {
                          setModuleTitle(e.target.value);
                          setModuleError('');
                        }}
                      />
                      {moduleError && <p className="error">{moduleError}</p>}
                      <button
                        type="submit"
                        className="btn"
                        disabled={!moduleTitle.trim() || moduleMutation.isPending}
                      >
                        {moduleMutation.isPending ? t('teach.saving') : t('teach.addModule')}
                      </button>
                    </form>
                  </section>

                  <section
                    className={`teach-section panel ce-editor${previewOpen ? ' ce-editor-split' : ''}`}
                  >
                    <div className="ce-editor-head">
                      <h2>{editingLessonId ? t('teach.editLesson') : t('teach.addLesson')}</h2>
                      {/* T28: предпросмотр — раскрывающаяся панель рядом с формой
                          (не модалка: учитель правит и сразу смотрит). */}
                      <button
                        type="button"
                        className="ce-toggle-preview"
                        onClick={() => setPreviewOpen((v) => !v)}
                        aria-expanded={previewOpen}
                        aria-controls="lesson-preview"
                      >
                        {t('teach.preview')}
                      </button>
                    </div>
                    {modules.length === 0 && <p className="muted">{t('teach.hintEmptyCourse')}</p>}
                    <form id="lesson-form" onSubmit={onLessonSubmit} noValidate className="form-grid">
                      <label htmlFor="lesson-module">Модуль</label>
                      <select
                        id="lesson-module"
                        className="select"
                        value={lesson.moduleId}
                        // T19: у сохранённого урока модуль меняет селект
                        // «Перенести в модуль…» ниже (T28), а не это поле.
                        disabled={Boolean(editingLessonId)}
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

                      {/* T28: перенос урока в другой модуль — только в черновике
                          (в остальных статусах backend отвечает 409). */}
                      {editingLessonId && editable && modules.length > 1 && (
                        <div className="ce-move-row">
                          <label htmlFor="lesson-move">{t('teach.moveLesson')}</label>
                          <select
                            id="lesson-move"
                            className="select"
                            value=""
                            disabled={moveLessonMutation.isPending}
                            onChange={(e) => onMoveLessonToModule(e.target.value)}
                          >
                            <option value="">—</option>
                            {modules
                              .filter((m) => m.id !== lesson.moduleId)
                              .map((m) => (
                                <option key={m.id} value={m.id}>
                                  {m.title}
                                </option>
                              ))}
                          </select>
                        </div>
                      )}

                      <label htmlFor="lesson-title">{t('teach.lessonTitle')}</label>
                      <input
                        id="lesson-title"
                        className="input"
                        value={lesson.title}
                        onChange={(e) => setLessonField('title', e.target.value)}
                      />
                      {lessonErrors['lesson-title'] && (
                        <p className="error">{lessonErrors['lesson-title']}</p>
                      )}

                      <label htmlFor="lesson-kind">{t('teach.lessonKind')}</label>
                      <select
                        id="lesson-kind"
                        className="select"
                        value={lesson.kind}
                        // T19: тип урока после создания менять нельзя (400).
                        disabled={Boolean(editingLessonId)}
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
                            className="input"
                            rows={5}
                            value={lesson.bodyMd}
                            onChange={(e) => setLessonField('bodyMd', e.target.value)}
                          />
                          <label htmlFor="lesson-video-url">{t('teach.videoUrl')}</label>
                          <input
                            id="lesson-video-url"
                            className="input"
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
                            className="input"
                            value={lesson.liveUrl}
                            onChange={(e) => setLessonField('liveUrl', e.target.value)}
                          />
                          {lessonErrors['lesson-live-url'] && (
                            <p className="error">{lessonErrors['lesson-live-url']}</p>
                          )}
                          <label htmlFor="lesson-starts-at">{t('teach.startsAt')}</label>
                          <input
                            id="lesson-starts-at"
                            className="input"
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
                              <div className="ce-q-head">
                                <label htmlFor={`q-${qi}-text`}>
                                  {t('teach.questionText')} {qi + 1}
                                </label>
                                {/* T28: перестановка вопросов; correct едет с вопросом. */}
                                <span className="ce-reorder">
                                  <button
                                    type="button"
                                    onClick={() => moveQuestion(qi, -1)}
                                    disabled={qi === 0}
                                  >
                                    {t('teach.moveUp')}
                                  </button>
                                  <button
                                    type="button"
                                    onClick={() => moveQuestion(qi, 1)}
                                    disabled={qi === lesson.questions.length - 1}
                                  >
                                    {t('teach.moveDown')}
                                  </button>
                                </span>
                              </div>
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
                                  <div key={oi} className="teach-option-row ce-option-row">
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
                                    {/* T28: перестановка вариантов с пересчётом
                                        индекса correct — ключ следует за вариантом. */}
                                    <button
                                      type="button"
                                      onClick={() => moveOption(qi, oi, -1)}
                                      disabled={oi === 0}
                                    >
                                      {t('teach.moveUp')}
                                    </button>
                                    <button
                                      type="button"
                                      onClick={() => moveOption(qi, oi, 1)}
                                      disabled={oi === q.options.length - 1}
                                    >
                                      {t('teach.moveDown')}
                                    </button>
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

                          {/* T62: класс для типографики заголовка «Тесты» (ce-tests-box). */}
                          <div id="code-tests" className="teach-focusable ce-tests-box" tabIndex={-1}>
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

                                <span className="ce-reorder">
                                  {/* T28: порядок тестов = порядок проверки; ссылок
                                      на индексы в codeTask нет — обмен безопасен. */}
                                  <button
                                    type="button"
                                    onClick={() => moveTest(ti, -1)}
                                    disabled={ti === 0}
                                  >
                                    {t('teach.moveUp')}
                                  </button>
                                  <button
                                    type="button"
                                    onClick={() => moveTest(ti, 1)}
                                    disabled={ti === lesson.tests.length - 1}
                                  >
                                    {t('teach.moveDown')}
                                  </button>
                                  {lesson.tests.length > 1 && (
                                    <button type="button" onClick={() => removeTest(ti)}>
                                      {t('common.delete')}
                                    </button>
                                  )}
                                </span>
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

                      <div className="teach-card-actions">
                        <button
                          type="submit"
                          className="btn btn-primary"
                          disabled={lessonMutation.isPending}
                        >
                          {lessonMutation.isPending
                            ? t('teach.saving')
                            : editingLessonId
                              ? t('teach.save')
                              : t('teach.addLesson')}
                        </button>
                        {editingLessonId && (
                          <button type="button" onClick={cancelEditLesson}>
                            {t('common.cancel')}
                          </button>
                        )}
                      </div>
                    </form>
                    {/* T28: панель предпросмотра «как увидит ученик» — собирается
                        из текущего черновика формы, ещё не сохранённого. */}
                    {previewOpen && (
                      <aside
                        id="lesson-preview"
                        className="ce-preview-pane"
                        aria-label={t('teach.previewTitle')}
                      >
                        <h3>{t('teach.previewTitle')}</h3>
                        <p className="teach-note ce-preview-hint">{t('teach.previewHint')}</p>
                        <LessonPreview draft={lessonToPreview(lesson)} />
                      </aside>
                    )}
                  </section>
                </>
              )}

              {/* Программа курса: порядок и удаление — только в черновике
                  (T19), переименование модуля — в любом статусе (T13). */}
              <section className="teach-section panel">
                <h2>{t('course.program')}</h2>
                {modules.length === 0 ? (
                  <p className="muted">{t('learn.empty')}</p>
                ) : (
                  <ol className="teach-modules">
                    {modules.map((m, mi) => (
                      <li key={m.id} className="teach-module">
                        {renamingModuleId === m.id ? (
                          <form className="teach-rename-row" onSubmit={onRenameSubmit} noValidate>
                            <input
                              id="module-rename"
                              className="input"
                              value={renameTitle}
                              onChange={(e) => setRenameTitle(e.target.value)}
                              autoFocus
                            />
                            <button
                              type="submit"
                              className="btn"
                              disabled={!renameTitle.trim() || renameMutation.isPending}
                            >
                              {renameMutation.isPending ? t('teach.saving') : t('common.save')}
                            </button>
                            <button type="button" onClick={() => setRenamingModuleId(null)}>
                              {t('common.cancel')}
                            </button>
                          </form>
                        ) : (
                          <div className="teach-module-head">
                            <h3>{m.title}</h3>
                            <div className="teach-card-actions">
                              <button
                                type="button"
                                onClick={() => {
                                  setRenamingModuleId(m.id);
                                  setRenameTitle(m.title);
                                }}
                              >
                                {t('teach.editModule')}
                              </button>
                              {editable && (
                                <>
                                  <button
                                    type="button"
                                    onClick={() => moveModule(mi, -1)}
                                    disabled={
                                      mi === 0 || reorderModulesMutation.isPending
                                    }
                                  >
                                    {t('teach.moveUp')}
                                  </button>
                                  <button
                                    type="button"
                                    onClick={() => moveModule(mi, 1)}
                                    disabled={
                                      mi === modules.length - 1 ||
                                      reorderModulesMutation.isPending
                                    }
                                  >
                                    {t('teach.moveDown')}
                                  </button>
                                  <button
                                    type="button"
                                    className="teach-danger"
                                    onClick={() => onDeleteModule(m.id)}
                                    disabled={
                                      m.lessons.length > 0 ||
                                      (deleteModuleMutation.isPending &&
                                        deleteModuleMutation.variables === m.id)
                                    }
                                    title={
                                      m.lessons.length > 0
                                        ? 'В модуле есть уроки: сначала удалите их'
                                        : undefined
                                    }
                                  >
                                    {t('teach.deleteModule')}
                                  </button>
                                </>
                              )}
                            </div>
                          </div>
                        )}
                        {m.lessons.length === 0 ? (
                          <p className="muted">{t('learn.noLessons')}</p>
                        ) : (
                          <ul className="teach-lesson-list">
                            {m.lessons.map((l, li) => (
                              <li key={l.id} className="teach-lesson-row">
                                <span className="teach-lesson-kind">{t(`kind.${l.kind}`)}</span>
                                <span className="teach-lesson-name">{l.title}</span>
                                {editable && (
                                  <span className="teach-row-actions">
                                    <button
                                      type="button"
                                      onClick={() => moveLesson(mi, li, -1)}
                                      disabled={
                                        li === 0 || reorderLessonsMutation.isPending
                                      }
                                    >
                                      {t('teach.moveUp')}
                                    </button>
                                    <button
                                      type="button"
                                      onClick={() => moveLesson(mi, li, 1)}
                                      disabled={
                                        li === m.lessons.length - 1 ||
                                        reorderLessonsMutation.isPending
                                      }
                                    >
                                      {t('teach.moveDown')}
                                    </button>
                                    <button
                                      type="button"
                                      onClick={() => startEditLesson(l, m.id)}
                                    >
                                      {t('teach.editLesson')}
                                    </button>
                                    <button
                                      type="button"
                                      className="teach-danger"
                                      onClick={() => onDeleteLesson(l.id)}
                                      disabled={
                                        deleteLessonMutation.isPending &&
                                        deleteLessonMutation.variables === l.id
                                      }
                                    >
                                      {t('teach.deleteLesson')}
                                    </button>
                                  </span>
                                )}
                              </li>
                            ))}
                          </ul>
                        )}
                      </li>
                    ))}
                  </ol>
                )}
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
      </div>
    </main>
  );
}
