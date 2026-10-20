// Типизированный клиент API. Все запросы идут с credentials:include (cookie-сессии).
// Базовый URL: /api/v1 (проксируется nginx/vite на api:8080).
// Доменные типы — в types.ts; здесь только транспорт (apiFetch, ApiError) и функции.
//
// ВАЖНО: backend включает DisallowUnknownFields() — любое лишнее поле в теле
// запроса = 400. Тела собираются явно, только из разрешённых контрактом полей.

import type {
  AdminSettings,
  CategoryStat,
  CertificatePublic,
  CompleteLessonInput,
  CompleteResult,
  Course,
  CourseDetail,
  CourseItem,
  CourseMinRatingFilter,
  CoursePriceFilter,
  CourseReviewsPage,
  CourseSort,
  CreateLessonInput,
  EnrollmentResult,
  Invite,
  Module,
  Lesson,
  LessonAnswer,
  LessonQuestion,
  LessonQuestionsPage,
  MyCertificate,
  MyCourse,
  Paged,
  PaymentJournalPage,
  PaymentStatus,
  PromoCode,
  PromoValidation,
  Provider,
  PublicSettings,
  ModerationDecision,
  Review,
  User,
} from './types';

// Реэкспорт типов: страницы импортируют типы из '../api/client' без правок.
export type {
  AdminSettings,
  CategoryStat,
  CertificatePublic,
  CodeTask,
  CodeTest,
  CompleteLessonInput,
  CompleteResult,
  Course,
  CourseCategory,
  CourseDetail,
  CourseItem,
  CourseMinRatingFilter,
  CoursePriceFilter,
  CourseSort,
  CourseStatus,
  CreateLessonInput,
  EnrollmentResult,
  Invite,
  Lesson,
  LessonAnswer,
  LessonKind,
  LessonQuestion,
  LessonQuestionsPage,
  Module,
  ModerationDecision,
  MyCertificate,
  MyCourse,
  NextLesson,
  Paged,
  PaymentJournalItem,
  PaymentJournalPage,
  PaymentStatus,
  PaymentTotals,
  PromoCode,
  PromoValidation,
  Provider,
  PublicSettings,
  QuizData,
  QuizQuestion,
  QuizQuestionInput,
  Review,
  ReviewBreakdown,
  Role,
  User,
} from './types';

const BASE: string =
  (import.meta.env.VITE_API_URL as string | undefined) ?? '/api/v1';

/** Ошибка API: HTTP-статус + текст из envelope {"error": "..."} или запасной текст. */
export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

// ---------- Транспорт ----------

// apiFetch экспортируется для модулей-компаньонов (api/adminUsers.ts,
// api/adminConsole.ts, api/teacherExtra.ts, api/teacherStudents.ts,
// api/meExtra.ts): задачи P3 пишутся параллельно и не должны ни дублировать
// транспорт, ни конфликтовать в этом файле.
export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
    ...init,
  });
  // 204 No Content (например, logout) — тела нет, возвращаем undefined.
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  let data: unknown;
  try {
    data = text ? (JSON.parse(text) as unknown) : undefined;
  } catch {
    data = undefined; // не-JSON тело — считаем пустым
  }
  if (!res.ok) {
    const errObj = typeof data === 'object' && data !== null ? (data as Record<string, unknown>) : {};
    let msg = typeof errObj.error === 'string' ? errObj.error : '';
    if (res.status === 401 && !msg) msg = 'Требуется вход в аккаунт';
    if (!msg) msg = `Ошибка запроса (${res.status})`;
    throw new ApiError(res.status, msg);
  }
  return data as T;
}

/** Собирает строку query из параметров, пропуская пустые значения.
 *  Экспортируется для модулей-компаньонов (см. комментарий у apiFetch). */
export function buildQuery(params?: Record<string, string | number | undefined>): string {
  if (!params) return '';
  const qs = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '') qs.set(key, String(value));
  }
  const s = qs.toString();
  return s ? `?${s}` : '';
}

// ---------- Auth ----------

export function register(input: {
  email: string;
  password: string;
  inviteToken?: string;
}): Promise<User> {
  // Тело собираем явно: inviteToken уходит только если передан.
  const body: Record<string, string> = {
    email: input.email,
    password: input.password,
  };
  if (input.inviteToken) body.inviteToken = input.inviteToken;
  return apiFetch<User>('/auth/register', {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

export function login(input: { email: string; password: string }): Promise<User> {
  return apiFetch<User>('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email: input.email, password: input.password }),
  });
}

export function logout(): Promise<void> {
  // Backend отвечает 204 (обрабатывается в apiFetch) или 200 {"status":"ok"}.
  return apiFetch<void>('/auth/logout', { method: 'POST' });
}

export function me(): Promise<User> {
  return apiFetch<User>('/auth/me');
}

// ---------- Каталог ----------

export function listCourses(params?: {
  q?: string;
  /** T65: слаг категории; пустое значение не отправляется. */
  category?: string;
  /** T65: free | paid; пустое значение не отправляется. */
  price?: CoursePriceFilter;
  /** T65: 3.5 | 4 | 4.5; пустое значение не отправляется. */
  minRating?: CourseMinRatingFilter;
  /** T65: popular | new | rating | price_asc | price_desc. */
  sort?: CourseSort;
  page?: number;
  limit?: number;
}): Promise<Paged<CourseItem>> {
  return apiFetch<Paged<CourseItem>>(
    `/courses${buildQuery({
      q: params?.q,
      category: params?.category,
      price: params?.price,
      minRating: params?.minRating,
      sort: params?.sort,
      page: params?.page,
      limit: params?.limit,
    })}`,
  );
}

/** T65: категории каталога; конверт { items } распаковывается в массив. */
export function getCategories(): Promise<CategoryStat[]> {
  return apiFetch<{ items: CategoryStat[] }>('/categories').then((res) => res.items);
}

/** Страница курса: конверт { course, modules } распаковывается в CourseDetail. */
export function getCourse(slug: string): Promise<CourseDetail> {
  return apiFetch<{ course: Course; modules: Module[] }>(
    `/courses/${encodeURIComponent(slug)}`,
  ).then((res) => ({ ...res.course, modules: res.modules }));
}

// ---------- Учитель ----------

export function createCourse(input: {
  title: string;
  description: string;
  priceCents: number;
  /** Кастомный slug (латиница, цифры, дефис, 3–120); если пуст — backend сгенерирует. */
  slug?: string;
  /** По умолчанию backend ставит RUB. */
  currency?: string;
}): Promise<Course> {
  // Тело строго из createCourseRequest: title, description, slug, priceCents, currency.
  const body: Record<string, string | number> = {
    title: input.title,
    description: input.description,
    priceCents: input.priceCents,
  };
  if (input.slug) body.slug = input.slug;
  if (input.currency) body.currency = input.currency;
  return apiFetch<Course>('/teacher/courses', {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

/** Свои курсы: конверт { items, total } → массив. */
export function myTeaching(): Promise<Course[]> {
  return apiFetch<{ items: Course[]; total: number }>('/teacher/courses').then(
    (res) => res.items,
  );
}

export function createModule(
  courseId: string,
  input: { title: string },
): Promise<Module> {
  return apiFetch<Module>(`/teacher/courses/${courseId}/modules`, {
    method: 'POST',
    body: JSON.stringify({ title: input.title }),
  });
}

/**
 * Создание урока. Тело собирается динамически: отправляются только поля,
 * относящиеся к kind, — иначе DisallowUnknownFields ответит 400.
 */
export function createLesson(
  courseId: string,
  input: CreateLessonInput,
): Promise<Lesson> {
  const body: Record<string, unknown> = {
    moduleId: input.moduleId,
    title: input.title,
    kind: input.kind,
  };
  switch (input.kind) {
    case 'video_text':
      if (input.bodyMd) body.bodyMd = input.bodyMd;
      if (input.videoUrl) body.videoUrl = input.videoUrl;
      break;
    case 'live':
      if (input.liveUrl) body.liveUrl = input.liveUrl;
      if (input.startsAt) body.startsAt = input.startsAt;
      break;
    case 'quiz':
      if (input.quiz) body.quiz = { questions: input.quiz.questions };
      break;
    case 'code':
      if (input.codeTask) {
        const task: Record<string, unknown> = {
          language: input.codeTask.language,
          tests: input.codeTask.tests,
        };
        if (input.codeTask.template) task.template = input.codeTask.template;
        body.codeTask = task;
      }
      break;
  }
  return apiFetch<Lesson>(`/teacher/courses/${courseId}/lessons`, {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

export function submitCourse(courseId: string): Promise<Course> {
  return apiFetch<Course>(`/teacher/courses/${courseId}/submit`, {
    method: 'POST',
  });
}

// ---------- Админ ----------

/** Токен приглашения показывается один раз — сохранять в UI сразу. */
export function createInvite(email: string): Promise<Invite> {
  return apiFetch<Invite>('/admin/invites', {
    method: 'POST',
    body: JSON.stringify({ email }),
  });
}

/** Очередь модерации: конверт { items, total } → массив. */
export function reviewQueue(): Promise<Course[]> {
  return apiFetch<{ items: Course[]; total: number }>('/admin/courses/review').then(
    (res) => res.items,
  );
}

export function moderateCourse(
  courseId: string,
  input: { decision: ModerationDecision; comment?: string },
): Promise<Course> {
  // comment обязателен при reject — backend сам вернёт 400 без него.
  const body: Record<string, string> = { decision: input.decision };
  if (input.comment) body.comment = input.comment;
  return apiFetch<Course>(`/admin/courses/${courseId}/moderate`, {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

// ---------- US6: журнал платежей и настройки ----------

export function paymentJournal(params?: {
  page?: number;
  limit?: number;
  status?: PaymentStatus;
  courseId?: string;
  /** Подстрока по email или имени студента (T31); пустая — без фильтра. */
  q?: string;
  /** Нижняя граница периода, ГГГГ-ММ-ДД (включительно); пустая — без фильтра. */
  from?: string;
  /** Верхняя граница периода, ГГГГ-ММ-ДД (включительно); пустая — без фильтра. */
  to?: string;
}): Promise<PaymentJournalPage> {
  return apiFetch<PaymentJournalPage>(
    `/admin/payments${buildQuery({
      page: params?.page,
      limit: params?.limit,
      status: params?.status,
      courseId: params?.courseId,
      q: params?.q,
      from: params?.from,
      to: params?.to,
    })}`,
  );
}

export function getAdminSettings(): Promise<AdminSettings> {
  return apiFetch<AdminSettings>('/admin/settings');
}

export function updateAdminSettings(commissionPct: number): Promise<AdminSettings> {
  return apiFetch<AdminSettings>('/admin/settings', {
    method: 'PUT',
    body: JSON.stringify({ commissionPct }),
  });
}

/**
 * Пост-P7: частичное обновление настроек — только промо-баннер. Отдельная
 * функция (а не расширение updateAdminSettings), чтобы не менять прежний
 * контракт вызова из формы комиссии.
 */
export function updateAdminSettingsPromo(promoBarEnabled: boolean): Promise<AdminSettings> {
  return apiFetch<AdminSettings>('/admin/settings', {
    method: 'PUT',
    body: JSON.stringify({ promoBarEnabled }),
  });
}

/** Публичные настройки интерфейса (промо-баннер) — доступны без входа. */
export function getPublicSettings(): Promise<PublicSettings> {
  return apiFetch<PublicSettings>('/settings/public');
}

// ---------- Покупка / обучение ----------

export function enroll(
  courseId: string,
  provider: Provider = 'mock',
  promoCode?: string,
): Promise<EnrollmentResult> {
  // P8 (T63): promoCode — необязательное аддитивное поле; пустое не отправляем,
  // иначе backend проверит его как обычный код и ответит 400.
  const body: Record<string, string> = { courseId, provider };
  if (promoCode && promoCode.trim() !== '') body.promoCode = promoCode.trim();
  return apiFetch<EnrollmentResult>('/enrollments', {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

// ---------- P8: промокоды (T63/T64) ----------

/**
 * Создание промокода (POST /teacher/courses/{id}/promo-codes).
 * Конверт ответа { promoCode } распаковывается в PromoCode.
 * maxUses/expiresAt необязательны: не переданные поля в тело не попадают
 * (backend включает DisallowUnknownFields, а expiresAt=null валит RFC3339).
 */
export function createPromoCode(
  courseId: string,
  payload: {
    code: string;
    discountPct: number;
    /** 0 — без ограничения (значение backend по умолчанию). */
    maxUses?: number;
    /** RFC3339 в будущем; null/undefined — бессрочно. */
    expiresAt?: string | null;
  },
): Promise<PromoCode> {
  const body: Record<string, string | number> = {
    code: payload.code,
    discountPct: payload.discountPct,
  };
  if (payload.maxUses !== undefined) body.maxUses = payload.maxUses;
  if (payload.expiresAt) body.expiresAt = payload.expiresAt;
  return apiFetch<{ promoCode: PromoCode }>(
    `/teacher/courses/${courseId}/promo-codes`,
    { method: 'POST', body: JSON.stringify(body) },
  ).then((res) => res.promoCode);
}

/** Список промокодов курса: конверт { items } → массив (по createdAt убыв.). */
export function listPromoCodes(courseId: string): Promise<PromoCode[]> {
  return apiFetch<{ items: PromoCode[] }>(
    `/teacher/courses/${courseId}/promo-codes`,
  ).then((res) => res.items);
}

/** Деактивация промокода (DELETE, 204; повторный вызов идемпотентен). */
export function deactivatePromoCode(
  courseId: string,
  promoId: string,
): Promise<void> {
  return apiFetch<void>(
    `/teacher/courses/${courseId}/promo-codes/${promoId}`,
    { method: 'DELETE' },
  );
}

/** Проверка промокода на странице курса (любой вошедший пользователь). */
export function validatePromoCode(
  courseId: string,
  code: string,
): Promise<PromoValidation> {
  return apiFetch<PromoValidation>(
    `/courses/${courseId}/promo/validate`,
    { method: 'POST', body: JSON.stringify({ code }) },
  );
}

/** Мои курсы: конверт { items, total } → массив (MyCourse уже плоский). */
export function myCourses(): Promise<MyCourse[]> {
  return apiFetch<{ items: MyCourse[]; total: number }>('/me/courses').then(
    (res) => res.items,
  );
}

// ---------- Сертификаты ----------

/** Публичный сертификат по номеру (без авторизации). 404 — сертификата нет. */
export function getCertificate(number: string): Promise<CertificatePublic> {
  return apiFetch<CertificatePublic>(
    `/certificates/${encodeURIComponent(number)}`,
  );
}

/** Мои сертификаты: конверт { items } → массив, новые первыми. */
export function myCertificates(): Promise<MyCertificate[]> {
  return apiFetch<{ items: MyCertificate[] }>('/me/certificates').then(
    (res) => res.items,
  );
}

/**
 * Завершение урока. Для video_text/live тело можно не слать вовсе
 * (пустой объект = 400 на backend) — body ставим только если есть данные.
 */
export function completeLesson(
  courseId: string,
  lessonId: string,
  input?: CompleteLessonInput,
): Promise<CompleteResult> {
  const body: Record<string, unknown> = {};
  if (input?.quizAnswers && Object.keys(input.quizAnswers).length > 0) {
    body.quizAnswers = input.quizAnswers;
  }
  if (input?.code) {
    body.code = input.code;
    if (input.language) body.language = input.language;
  }
  const init: RequestInit = { method: 'POST' };
  if (Object.keys(body).length > 0) init.body = JSON.stringify(body);
  return apiFetch<CompleteResult>(
    `/me/courses/${courseId}/lessons/${lessonId}/complete`,
    init,
  );
}

// ---------- Отзывы ----------

/** Список отзывов: конверт { items, total, avgRating, ratingsCount } → массив. */
export function listReviews(courseId: string): Promise<Review[]> {
  return apiFetch<{
    items: Review[];
    total: number;
    avgRating: number;
    ratingsCount: number;
  }>(`/courses/${encodeURIComponent(courseId)}/reviews`).then((res) => res.items);
}

export function createReview(
  courseId: string,
  input: { rating: number; body: string },
): Promise<Review> {
  // Ключ текста отзыва — body, не text.
  return apiFetch<Review>(`/courses/${encodeURIComponent(courseId)}/reviews`, {
    method: 'POST',
    body: JSON.stringify({ rating: input.rating, body: input.body }),
  });
}

/**
 * T69: страница отзывов курса вместе с конвертом (avgRating/ratingsCount/
 * breakdown). `rating` необязателен; при фильтре items/total — по оценке,
 * статистика в конверте всё равно по всем отзывам.
 */
export function getCourseReviews(
  courseId: string,
  params?: { rating?: number },
): Promise<CourseReviewsPage> {
  return apiFetch<CourseReviewsPage>(
    `/courses/${encodeURIComponent(courseId)}/reviews${buildQuery({
      rating: params?.rating,
    })}`,
  );
}

/** T69: ответ преподавателя на отзыв (PUT, владелец курса/admin). */
export function replyToReview(
  courseId: string,
  reviewId: string,
  body: string,
): Promise<Review> {
  return apiFetch<{ review: Review }>(
    `/teacher/courses/${encodeURIComponent(courseId)}/reviews/${encodeURIComponent(reviewId)}/reply`,
    {
      method: 'PUT',
      body: JSON.stringify({ body }),
    },
  ).then((res) => res.review);
}

/** T69: удаление ответа преподавателя; 204, в том числе если ответа не было. */
export function deleteReviewReply(
  courseId: string,
  reviewId: string,
): Promise<void> {
  return apiFetch<void>(
    `/teacher/courses/${encodeURIComponent(courseId)}/reviews/${encodeURIComponent(reviewId)}/reply`,
    { method: 'DELETE' },
  );
}

// ---------- P9: вопросы и ответы по урокам ----------

/**
 * Вопросы курса или одного урока (T71): конверт с пагинацией возвращается
 * целиком (items/total/page/limit). `lessonId` пустой — все вопросы курса.
 * Вопросы приходят новыми первыми, ответы внутри — старыми первыми.
 */
export function listLessonQuestions(
  courseId: string,
  lessonId?: string,
  page?: number,
): Promise<LessonQuestionsPage> {
  return apiFetch<LessonQuestionsPage>(
    `/courses/${encodeURIComponent(courseId)}/questions${buildQuery({
      lessonId,
      page,
    })}`,
  );
}

/** Задать вопрос к уроку; конверт ответа { question } разворачивается. */
export function askQuestion(
  courseId: string,
  lessonId: string,
  body: string,
): Promise<LessonQuestion> {
  return apiFetch<{ question: LessonQuestion }>(
    `/courses/${encodeURIComponent(courseId)}/questions`,
    {
      method: 'POST',
      body: JSON.stringify({ lessonId, body }),
    },
  ).then((res) => res.question);
}

/** Ответить на вопрос; конверт ответа { answer } разворачивается. */
export function answerQuestion(
  courseId: string,
  questionId: string,
  body: string,
): Promise<LessonAnswer> {
  return apiFetch<{ answer: LessonAnswer }>(
    `/courses/${encodeURIComponent(courseId)}/questions/${encodeURIComponent(questionId)}/answers`,
    {
      method: 'POST',
      body: JSON.stringify({ body }),
    },
  ).then((res) => res.answer);
}

/** Удалить вопрос вместе с ответами; успех — 204. */
export function deleteQuestion(
  courseId: string,
  questionId: string,
): Promise<void> {
  return apiFetch<void>(
    `/courses/${encodeURIComponent(courseId)}/questions/${encodeURIComponent(questionId)}`,
    { method: 'DELETE' },
  );
}

/** Удалить ответ на вопрос; успех — 204. */
export function deleteAnswer(
  courseId: string,
  questionId: string,
  answerId: string,
): Promise<void> {
  return apiFetch<void>(
    `/courses/${encodeURIComponent(courseId)}/questions/${encodeURIComponent(questionId)}/answers/${encodeURIComponent(answerId)}`,
    { method: 'DELETE' },
  );
}

// ---------- Утилиты ----------

/**
 * Денежная сумма в рублях без подмены смысла: 0 — это «0 ₽», а не «Бесплатно».
 * Для итогов журнала платежей, комиссий, возвратов (отрицательные суммы)
 * и любого счёта, где ноль — ноль.
 */
export function formatMoney(cents: number): string {
  return `${new Intl.NumberFormat('ru-RU').format(cents / 100)} ₽`;
}

/**
 * Цена курса: 0 и меньше — «Бесплатно» (приемлемо только для цены).
 * Для счетов и итогов есть formatMoney.
 */
export function formatPrice(priceCents: number): string {
  if (priceCents <= 0) return 'Бесплатно';
  return formatMoney(priceCents);
}

/** RFC3339 → локальная дата-время ru-RU. Пустое или некорректное значение → «—». */
export function formatDate(value: string | null | undefined): string {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleString('ru-RU', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export function useApiBase(): string {
  return BASE;
}
