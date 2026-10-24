// T27: API-клиент супер-админских разделов «Сводка» и «Курсы».
// Контракт — Task/T25-admin-console.md (поля и коды берутся оттуда буквально,
// бэкенд T25 пишется параллельно). Транспорт — общий apiFetch/buildQuery из
// client.ts: он экспортирован специально для модулей-компаньонов волны P3,
// дублировать fetch и обработку envelope {"error"} здесь нельзя.
import {
  apiFetch,
  buildQuery,
  deleteAnswer as deleteAnswerApi,
  deleteQuestion as deleteQuestionApi,
} from './client';
import type { CourseStatus, Role } from './client';

/** Агрегаты сводки (GET /admin/stats). Денежные суммы — в копейках. */
export type AdminStats = {
  users: {
    total: number;
    students: number;
    teachers: number;
    admins: number;
    blocked: number;
    newLast7: number;
  };
  courses: {
    total: number;
    draft: number;
    review: number;
    published: number;
    archived: number;
  };
  payments: {
    count: number;
    succeededCents: number;
    platformFeeCents: number;
    teacherCents: number;
    last30Cents: number;
  };
  enrollments: {
    total: number;
    active: number;
  };
  reviews: {
    total: number;
    avgRating: number;
  };
};

/** Строка курса в админском списке (GET /admin/courses). */
export type AdminCourseRow = {
  id: string;
  slug: string;
  title: string;
  teacherId: string;
  teacherEmail: string;
  status: CourseStatus;
  priceCents: number;
  createdAt: string;
  updatedAt: string;
  modulesCount: number;
  lessonsCount: number;
  studentsCount: number;
  /**
   * T87: комментарий модерации в GET /admin/courses/{id}. В элементах
   * списка GET /admin/courses поле не отдаётся, поэтому оно необязательное.
   */
  moderationComment?: string;
};

/** Курс в ответах действий админки (форма строки GET /admin/courses). */
export type AdminCourse = AdminCourseRow;

/** Страница админского списка курсов: total — до пагинации. */
export type AdminCoursePage = {
  items: AdminCourseRow[];
  total: number;
  page: number;
  limit: number;
};

/** Поле сортировки списка курсов (контракт P6, T48): иное backend отвергает 400. */
export type AdminCourseSort = 'createdAt' | 'updatedAt' | 'title' | 'price' | 'students';

/** Направление сортировки (контракт P6, T48): по умолчанию — desc. */
export type AdminCourseDir = 'asc' | 'desc';

/** Параметры админского списка курсов (все необязательны, кроме новых — аддитивных). */
export type AdminCourseListParams = {
  status?: string;
  q?: string;
  page?: number;
  limit?: number;
  /** Поле сортировки; по умолчанию backend считает createdAt. */
  sort?: AdminCourseSort;
  /** Направление сортировки; по умолчанию backend считает desc. */
  dir?: AdminCourseDir;
  /** Подстрока по email преподавателя без учёта регистра (T48). */
  teacherEmail?: string;
  /** Нижняя граница периода по createdAt, ГГГГ-ММ-ДД (включительно, T48). */
  from?: string;
  /** Верхняя граница периода по createdAt, ГГГГ-ММ-ДД (включительно, T48). */
  to?: string;
};

/** Строка отзыва курса (GET /admin/courses/{id}/reviews). */
export type AdminReviewRow = {
  id: string;
  studentId: string;
  studentEmail: string;
  rating: number;
  body: string;
  createdAt: string;
};

/**
 * Активная сессия пользователя (T93).
 * GET /admin/users/{id}/sessions отдаёт только неистёкшие сессии, новые первыми.
 */
export type AdminSession = {
  id: string;
  createdAt: string;
  expiresAt: string;
};

/** Ответ списка сессий: items всегда массив (может быть пустым). */
export type AdminSessionsResponse = {
  items: AdminSession[];
};

/** GET /admin/stats — все агрегаты сводки одним запросом. */
export function getAdminStats(): Promise<AdminStats> {
  return apiFetch<AdminStats>('/admin/stats');
}

/**
 * GET /admin/courses — курсы всех статусов.
 * Пустые параметры buildQuery не отправляет; невалидный status backend
 * отвергает 400 (в UI такому значению появиться неоткуда — фильтр закрытый).
 * Параметры P6 (T48): sort/dir/teacherEmail/from/to — необязательные и
 * аддитивные: старый backend их просто игнорирует.
 */
export function listAdminCourses(params: AdminCourseListParams): Promise<AdminCoursePage> {
  return apiFetch<AdminCoursePage>(
    `/admin/courses${buildQuery({
      status: params.status,
      q: params.q,
      page: params.page,
      limit: params.limit,
      sort: params.sort,
      dir: params.dir,
      teacherEmail: params.teacherEmail,
      from: params.from,
      to: params.to,
    })}`,
  );
}

/** POST /admin/courses/{id}/archive — 200 {course}; 409, если уже в архиве; 404. */
export function archiveAdminCourse(id: string): Promise<{ course: AdminCourseRow }> {
  return apiFetch<{ course: AdminCourseRow }>(
    `/admin/courses/${encodeURIComponent(id)}/archive`,
    { method: 'POST' },
  );
}

/** POST /admin/courses/{id}/unpublish — published → draft; иначе 409; 404. */
export function unpublishAdminCourse(id: string): Promise<{ course: AdminCourseRow }> {
  return apiFetch<{ course: AdminCourseRow }>(
    `/admin/courses/${encodeURIComponent(id)}/unpublish`,
    { method: 'POST' },
  );
}

/**
 * POST /admin/courses/{id}/publish — принудительная публикация (T87):
 * draft → published или review → published без очереди модерации.
 * published — идемпотентный 200; archived — 409; 404, если курса нет.
 */
export function publishAdminCourse(id: string): Promise<{ course: AdminCourse }> {
  return apiFetch<{ course: AdminCourse }>(
    `/admin/courses/${encodeURIComponent(id)}/publish`,
    { method: 'POST' },
  );
}

/**
 * POST /admin/courses/{id}/unarchive — вернуть курс из архива: archived → draft
 * (комментарий модерации сохраняется, контракт P6 / T48). Иначе 409; 404.
 */
export function unarchiveAdminCourse(id: string): Promise<{ course: AdminCourseRow }> {
  return apiFetch<{ course: AdminCourseRow }>(
    `/admin/courses/${encodeURIComponent(id)}/unarchive`,
    { method: 'POST' },
  );
}

/** GET /admin/courses/{id}/reviews — items всегда массив; 404, если курса нет. */
export function listCourseReviews(courseId: string): Promise<{ items: AdminReviewRow[] }> {
  return apiFetch<{ items: AdminReviewRow[] }>(
    `/admin/courses/${encodeURIComponent(courseId)}/reviews`,
  );
}

/** DELETE /admin/reviews/{id} — 204 без тела; 404, если отзыва нет. */
export function deleteReview(id: string): Promise<void> {
  return apiFetch<void>(`/admin/reviews/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  });
}

// ---------- P10 / T85: глобальная модерация Q&A ----------

/**
 * Ответ под вопросом в админском списке (GET /admin/questions).
 * Поле в поле по контракту T85; `authorRole` — та же роль, что в P9 Q&A.
 */
export type AdminAnswer = {
  id: string;
  authorId: string;
  authorEmail: string;
  authorName: string;
  authorRole: Role;
  body: string;
  createdAt: string;
};

/** Вопрос в админском списке: курс/урок, автор и все ответы ветки. */
export type AdminQuestion = {
  id: string;
  courseId: string;
  courseTitle: string;
  courseSlug: string;
  lessonId: string;
  lessonTitle: string;
  studentId: string;
  studentEmail: string;
  studentName: string;
  body: string;
  createdAt: string;
  /** Ответы приходят старыми первыми; каскадное удаление — вместе с вопросом. */
  answers: AdminAnswer[];
};

/** Страница глобального списка вопросов (конверт пагинации). */
export type AdminQuestionsResponse = {
  items: AdminQuestion[];
  total: number;
  page: number;
  limit: number;
};

/** Фильтры GET /admin/questions; пустые значения buildQuery не отправляет. */
export type AdminQuestionsQuery = {
  courseId?: string;
  lessonId?: string;
  q?: string;
  page?: number;
  limit?: number;
};

/**
 * GET /admin/questions — все вопросы платформы (новые первыми), с фильтрами
 * courseId/lessonId/q и пагинацией. Неверные UUID/пагинация — 400.
 */
export function listAdminQuestions(
  query: AdminQuestionsQuery = {},
): Promise<AdminQuestionsResponse> {
  return apiFetch<AdminQuestionsResponse>(
    `/admin/questions${buildQuery({
      courseId: query.courseId,
      lessonId: query.lessonId,
      q: query.q,
      page: query.page,
      limit: query.limit,
    })}`,
  );
}

/**
 * DELETE /courses/{courseId}/questions/{questionId} — 204, ответы удаляются
 * каскадом. Тонкая обёртка над существующим клиентом P9: путь и кодирование
 * id не дублируем. Admin может удалять чужие вопросы (контракт T85).
 */
export function deleteAdminQuestion(courseId: string, questionId: string): Promise<void> {
  return deleteQuestionApi(courseId, questionId);
}

/**
 * DELETE /courses/{courseId}/questions/{questionId}/answers/{answerId} — 204.
 * Тонкая обёртка над существующим клиентом P9 (см. deleteAdminQuestion).
 */
export function deleteAdminAnswer(
  courseId: string,
  questionId: string,
  answerId: string,
): Promise<void> {
  return deleteAnswerApi(courseId, questionId, answerId);
}

/** GET /admin/users/{id}/sessions — 200 {items}; 404, если пользователя нет. */
export function listAdminUserSessions(userId: string): Promise<AdminSessionsResponse> {
  return apiFetch<AdminSessionsResponse>(
    `/admin/users/${encodeURIComponent(userId)}/sessions`,
  );
}

/**
 * DELETE /admin/users/{id}/sessions — 204 без тела: завершает все сессии
 * пользователя, включая текущую сессию админа; 404, если пользователя нет.
 */
export function revokeAdminUserSessions(userId: string): Promise<void> {
  return apiFetch<void>(`/admin/users/${encodeURIComponent(userId)}/sessions`, {
    method: 'DELETE',
  });
}

// ---------- T90: глобальный журнал промокодов (контракт T89) ----------

/** Строка глобального журнала промокодов (GET /admin/promo-codes). */
export type AdminPromoCode = {
  id: string;
  courseId: string;
  courseTitle: string;
  courseSlug: string;
  code: string;
  discountPct: number;
  maxUses: number;
  usedCount: number;
  /** RFC3339 или null — код без срока действия. */
  expiresAt: string | null;
  active: boolean;
  createdAt: string;
};

/** Страница журнала промокодов: total — до пагинации. */
export type AdminPromoCodesResponse = {
  items: AdminPromoCode[];
  total: number;
  page: number;
  limit: number;
};

/** Параметры GET /admin/promo-codes (все необязательны). */
export type AdminPromoCodesQuery = {
  /** UUID курса; пустая строка не отправляется. */
  courseId?: string;
  /** Код или название курса без учёта регистра. */
  q?: string;
  /** true — только активные, false — только неактивные. */
  active?: boolean;
  page?: number;
  limit?: number;
};

/** Тело PATCH /admin/promo-codes/{id}: хотя бы одно поле. */
export type AdminPromoCodePatch = {
  discountPct?: number;
  maxUses?: number;
  /** RFC3339 в будущем или null — снять срок. */
  expiresAt?: string | null;
  active?: boolean;
};

/**
 * GET /admin/promo-codes — все промокоды платформы.
 * active=false передаётся явно (buildQuery пропускает только пустые
 * значения, поэтому boolean приводим к строке сами).
 */
export function listAdminPromoCodes(
  query: AdminPromoCodesQuery = {},
): Promise<AdminPromoCodesResponse> {
  return apiFetch<AdminPromoCodesResponse>(
    `/admin/promo-codes${buildQuery({
      courseId: query.courseId,
      q: query.q,
      active: query.active === undefined ? undefined : String(query.active),
      page: query.page,
      limit: query.limit,
    })}`,
  );
}

/**
 * PATCH /admin/promo-codes/{id} — частичное обновление.
 * Тело собирается только из переданных полей: expiresAt=null отправляется
 * явно (снять срок), а не выбрасывается, в отличие от создания кода (T63).
 * Ответ — конверт {promoCode}; возвращаем саму запись.
 */
export function updateAdminPromoCode(
  id: string,
  patch: AdminPromoCodePatch,
): Promise<AdminPromoCode> {
  const body: AdminPromoCodePatch = {};
  if (patch.discountPct !== undefined) body.discountPct = patch.discountPct;
  if (patch.maxUses !== undefined) body.maxUses = patch.maxUses;
  if (patch.expiresAt !== undefined) body.expiresAt = patch.expiresAt;
  if (patch.active !== undefined) body.active = patch.active;
  return apiFetch<{ promoCode: AdminPromoCode }>(
    `/admin/promo-codes/${encodeURIComponent(id)}`,
    { method: 'PATCH', body: JSON.stringify(body) },
  ).then((res) => res.promoCode);
}
