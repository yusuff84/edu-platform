// Типизированный клиент API. Все запросы идут с credentials:include (cookie-сессии).
// Базовый URL: /api/v1 (проксируется nginx/vite на api:8080).

const BASE: string =
  (import.meta.env.VITE_API_URL as string | undefined) ?? '/api/v1';

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
    ...init,
  });
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  const data: unknown = text ? (JSON.parse(text) as unknown) : undefined;
  if (!res.ok) {
    const msg =
      typeof data === 'object' && data !== null && ('error' in data || 'message' in data)
        ? String((data as Record<string, unknown>).error ?? (data as Record<string, unknown>).message)
        : `Ошибка запроса (${res.status})`;
    throw new ApiError(res.status, msg);
  }
  return data as T;
}

// ---------- Типы ----------

export type Role = 'student' | 'teacher' | 'admin';
export type LessonKind = 'video_text' | 'live' | 'quiz' | 'code';
export type CourseStatus = 'draft' | 'review' | 'published' | 'rejected';

export interface User {
  id: string;
  email: string;
  role: Role;
}

export interface QuizQuestion {
  id: string;
  question: string;
  options: string[];
}

export interface Lesson {
  id: string;
  title: string;
  kind: LessonKind;
  position: number;
  videoUrl?: string;
  textContent?: string;
  liveUrl?: string;
  scheduledAt?: string;
  quiz?: QuizQuestion[];
  codeTemplate?: string;
}

export interface Module {
  id: string;
  title: string;
  position: number;
  lessons: Lesson[];
}

export interface Course {
  id: string;
  slug: string;
  title: string;
  description: string;
  priceCents: number;
  status: CourseStatus;
  avgRating: number;
  reviewsCount: number;
  teacherName?: string;
  teacherId?: string;
  coverUrl?: string;
  modules?: Module[];
  lessonsCount?: number;
  modulesCount?: number;
}

export interface Paged<T> {
  items: T[];
  total: number;
  page: number;
  limit: number;
}

export interface MyCourse {
  courseId: string;
  slug: string;
  title: string;
  coverUrl?: string;
  percent: number;
  nextLessonId?: string | null;
  nextLessonTitle?: string | null;
}

export interface Review {
  id: string;
  authorEmail: string;
  rating: number;
  text: string;
  createdAt: string;
}

export interface CompleteResult {
  accepted: boolean;
  message?: string;
  percent: number;
  nextLessonId?: string | null;
}

export interface ModerationItem extends Course {
  submittedAt?: string;
}

// ---------- Auth ----------

export function register(input: {
  email: string;
  password: string;
  inviteToken?: string;
}): Promise<User> {
  return apiFetch<User>('/auth/register', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export function login(input: { email: string; password: string }): Promise<User> {
  return apiFetch<User>('/auth/login', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export function logout(): Promise<void> {
  return apiFetch<void>('/auth/logout', { method: 'POST' });
}

export function me(): Promise<User> {
  return apiFetch<User>('/auth/me');
}

// ---------- Каталог ----------

// Сервер в ряде мест отдаёт ratingsCount вместо reviewsCount,
// детали курса — вложенно {course, modules}, списки — {items, total}.
// Маппим всё к плоским типам ниже, чтобы компоненты не знали о формате API.
interface RawCourse extends Omit<Course, 'reviewsCount' | 'modules'> {
  reviewsCount?: number;
  ratingsCount?: number;
  modules?: RawModule[];
}

interface RawQuizQuestion {
  id: string;
  text: string;
  options?: string[];
}

interface RawLesson {
  id: string;
  title: string;
  kind: string;
  position: number;
  bodyMd?: string;
  videoUrl?: string;
  liveUrl?: string;
  startsAt?: string;
  quiz?: { questions?: RawQuizQuestion[] };
  codeTask?: { language?: string; template?: string };
}

interface RawModule {
  id: string;
  title: string;
  position: number;
  lessons?: RawLesson[];
}

function toLesson(l: RawLesson): Lesson {
  return {
    id: l.id,
    title: l.title,
    kind: l.kind as LessonKind,
    position: l.position,
    videoUrl: l.videoUrl,
    textContent: l.bodyMd,
    liveUrl: l.liveUrl,
    scheduledAt: l.startsAt,
    quiz: l.quiz?.questions?.map((q) => ({
      id: q.id,
      question: q.text,
      options: q.options ?? [],
    })),
    codeTemplate: l.codeTask?.template,
  };
}

function toCourse(c: RawCourse): Course {
  return {
    ...(c as Course),
    reviewsCount: c.reviewsCount ?? c.ratingsCount ?? 0,
    modules: (c.modules ?? []).map((m) => ({
      id: m.id,
      title: m.title,
      position: m.position,
      lessons: (m.lessons ?? []).map(toLesson),
    })),
  };
}

export function listCourses(params?: {
  q?: string;
  page?: number;
  limit?: number;
}): Promise<Paged<Course>> {
  const qs = new URLSearchParams();
  if (params?.q) qs.set('q', params.q);
  if (params?.page) qs.set('page', String(params.page));
  if (params?.limit) qs.set('limit', String(params.limit));
  const suffix = qs.toString() ? `?${qs.toString()}` : '';
  return apiFetch<Paged<RawCourse>>(`/courses${suffix}`).then((data) => ({
    ...data,
    items: (data.items ?? []).map(toCourse),
  }));
}

export async function getCourse(slug: string): Promise<Course> {
  const data = await apiFetch<{ course: RawCourse; modules?: RawModule[] }>(
    `/courses/${encodeURIComponent(slug)}`,
  );
  return toCourse({ ...data.course, modules: data.modules });
}

// ---------- Учитель ----------

export function createCourse(input: {
  title: string;
  description: string;
  priceCents: number;
}): Promise<Course> {
  return apiFetch<Course>('/teacher/courses', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export async function myTeaching(): Promise<Course[]> {
  const data = await apiFetch<{ items?: RawCourse[] }>(`/teacher/courses`);
  return (data.items ?? []).map(toCourse);
}

export function createModule(
  courseId: string,
  input: { title: string },
): Promise<Module> {
  return apiFetch<Module>(`/teacher/courses/${courseId}/modules`, {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export function createLesson(
  courseId: string,
  input: {
    moduleId: string;
    title: string;
    kind: LessonKind;
    videoUrl?: string;
    textContent?: string;
    liveUrl?: string;
    scheduledAt?: string;
    quiz?: QuizQuestion[];
    codeTask?: string;
  },
): Promise<Lesson> {
  return apiFetch<Lesson>(`/teacher/courses/${courseId}/lessons`, {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export function submitCourse(courseId: string): Promise<Course> {
  return apiFetch<Course>(`/teacher/courses/${courseId}/submit`, {
    method: 'POST',
  });
}

// ---------- Админ ----------

export function createInvite(email: string): Promise<{ token: string }> {
  return apiFetch<{ token: string }>('/admin/invites', {
    method: 'POST',
    body: JSON.stringify({ email }),
  });
}

export async function reviewQueue(): Promise<Course[]> {
  const data = await apiFetch<{ items?: RawCourse[] }>('/admin/courses/review');
  return (data.items ?? []).map(toCourse);
}

export function moderateCourse(
  courseId: string,
  input: { decision: 'approve' | 'reject'; comment?: string },
): Promise<Course> {
  return apiFetch<Course>(`/admin/courses/${courseId}/moderate`, {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

// ---------- Покупка / обучение ----------

export function enroll(
  courseId: string,
  provider: 'mock' | 'stripe' | 'yookassa' = 'mock',
): Promise<{ redirectUrl?: string; enrolled?: boolean }> {
  return apiFetch<{ redirectUrl?: string; enrolled?: boolean }>('/enrollments', {
    method: 'POST',
    body: JSON.stringify({ courseId, provider }),
  });
}

export async function myCourses(): Promise<MyCourse[]> {
  const data = await apiFetch<{
    items?: Array<{
      course: RawCourse;
      percent?: number;
      nextLesson?: { lessonId?: string; title?: string } | null;
    }>;
  }>('/me/courses');
  return (data.items ?? []).map((it) => ({
    courseId: it.course.id,
    slug: it.course.slug,
    title: it.course.title,
    percent: it.percent ?? 0,
    nextLessonId: it.nextLesson?.lessonId ?? null,
    nextLessonTitle: it.nextLesson?.title ?? null,
  }));
}

export async function completeLesson(
  courseId: string,
  lessonId: string,
  input: { quizAnswers?: Record<string, number>; code?: string; language?: string },
): Promise<CompleteResult> {
  const data = await apiFetch<{
    passed?: boolean;
    completed?: boolean;
    percent?: number;
    message?: string;
    nextLesson?: { lessonId?: string } | null;
  }>(`/me/courses/${courseId}/lessons/${lessonId}/complete`, {
    method: 'POST',
    body: JSON.stringify(input),
  });
  return {
    accepted: Boolean(data.passed ?? data.completed),
    message: data.message,
    percent: data.percent ?? 0,
    nextLessonId: data.nextLesson?.lessonId ?? null,
  };
}

// ---------- Отзывы ----------

export async function listReviews(courseId: string): Promise<Review[]> {
  const data = await apiFetch<{ items?: Review[] }>(`/courses/${courseId}/reviews`);
  return data.items ?? [];
}

export function createReview(
  courseId: string,
  input: { rating: number; text: string },
): Promise<Review> {
  return apiFetch<Review>(`/courses/${courseId}/reviews`, {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

// ---------- Mock-оплата (только для демо-провайдера) ----------

export function confirmMockPayment(providerPaymentId: string): Promise<{ status: string }> {
  return apiFetch<{ status: string }>('/payments/webhook/mock', {
    method: 'POST',
    body: JSON.stringify({ providerPaymentId, status: 'succeeded' }),
  });
}

// ---------- Утилиты ----------

export function formatPrice(priceCents: number): string {
  if (priceCents <= 0) return 'Бесплатно';
  const rub = priceCents / 100;
  return `${new Intl.NumberFormat('ru-RU').format(rub)} ₽`;
}

export function useApiBase(): string {
  return BASE;
}
