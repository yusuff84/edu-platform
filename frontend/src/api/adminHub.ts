// Новая админка: сводка, журнал действий, сквозные списки (backend: handler/admin_hub.go).
import { apiFetch, buildQuery } from './client';

export type OverviewAttention = {
  newLeads: number;
  reviewQueue: number;
  unansweredQuestions: number;
  pendingPayments: number;
  failedPayments7d: number;
  reviewsNoReply: number;
  lowRatingNoReply: number;
};

export type OverviewDay = {
  date: string;
  signups: number;
  enrollments: number;
  revenueCents: number;
  payments: number;
};

export type OverviewUser = {
  id: string;
  email: string;
  displayName: string;
  role: 'student' | 'teacher' | 'admin';
  createdAt: string;
};

export type OverviewPayment = {
  id: string;
  studentId: string;
  studentEmail: string;
  courseId: string;
  courseTitle: string;
  amountCents: number;
  status: 'pending' | 'succeeded' | 'failed' | 'refunded';
  createdAt: string;
};

export type AuditEntry = {
  id: string;
  actorId: string;
  actorEmail: string;
  action: string;
  ip: string;
  createdAt: string;
};

export type AdminOverview = {
  attention: OverviewAttention;
  series: OverviewDay[];
  recentUsers: OverviewUser[];
  recentPayments: OverviewPayment[];
  recentAudit: AuditEntry[];
};

export const overviewQueryKey = ['admin', 'overview'] as const;

export function getAdminOverview(days = 30) {
  return apiFetch<AdminOverview>(`/admin/overview${buildQuery({ days })}`);
}

export function listAudit(params: { q?: string; page?: number; limit?: number }) {
  return apiFetch<{ items: AuditEntry[]; total: number; page: number; limit: number }>(
    `/admin/audit${buildQuery(params)}`,
  );
}

export type AdminQuestionAnswer = {
  id: string;
  questionId: string;
  authorId: string;
  authorName: string;
  authorRole: string;
  isTeacher: boolean;
  body: string;
  createdAt: string;
};

export type AdminQuestion = {
  id: string;
  courseId: string;
  lessonId: string;
  lessonTitle: string;
  studentId: string;
  studentName: string;
  studentEmail: string;
  body: string;
  createdAt: string;
  answers: AdminQuestionAnswer[] | null;
  courseTitle: string;
  courseSlug: string;
  answered: boolean;
};

export function listAdminQuestions(params: {
  status?: 'unanswered' | 'answered' | 'all';
  courseId?: string;
  page?: number;
  limit?: number;
}) {
  return apiFetch<{ items: AdminQuestion[]; total: number; unanswered: number; page: number; limit: number }>(
    `/admin/questions${buildQuery(params)}`,
  );
}

/** Ответ на вопрос к уроку (существующий маршрут; админ может отвечать в любом курсе). */
export function answerQuestion(courseId: string, questionId: string, body: string) {
  return apiFetch<unknown>(
    `/courses/${encodeURIComponent(courseId)}/questions/${encodeURIComponent(questionId)}/answers`,
    { method: 'POST', body: JSON.stringify({ body }) },
  );
}

export function deleteQuestion(courseId: string, questionId: string) {
  return apiFetch<void>(`/courses/${encodeURIComponent(courseId)}/questions/${encodeURIComponent(questionId)}`, {
    method: 'DELETE',
  });
}

export type AdminReview = {
  id: string;
  studentId: string;
  courseId: string;
  rating: number;
  body?: string;
  createdAt: string;
  teacherReply: string;
  teacherReplyAt: string | null;
  courseTitle: string;
  courseSlug: string;
  studentEmail: string;
  studentName: string;
};

export function listAdminReviews(params: {
  rating?: number;
  q?: string;
  courseId?: string;
  noReply?: 1;
  page?: number;
  limit?: number;
}) {
  return apiFetch<{ items: AdminReview[]; total: number; page: number; limit: number }>(
    `/admin/reviews${buildQuery(params)}`,
  );
}

/** Публичный ответ на отзыв (маршрут преподавателя; админу доступен для любого курса). */
export function replyToReview(courseId: string, reviewId: string, body: string) {
  return apiFetch<unknown>(
    `/teacher/courses/${encodeURIComponent(courseId)}/reviews/${encodeURIComponent(reviewId)}/reply`,
    { method: 'PUT', body: JSON.stringify({ body }) },
  );
}

export function clearReviewReply(courseId: string, reviewId: string) {
  return apiFetch<void>(
    `/teacher/courses/${encodeURIComponent(courseId)}/reviews/${encodeURIComponent(reviewId)}/reply`,
    { method: 'DELETE' },
  );
}

export type AdminPromoCode = {
  id: string;
  courseId: string;
  code: string;
  discountPct: number;
  maxUses: number;
  usedCount: number;
  expiresAt: string | null;
  active: boolean;
  createdAt: string;
  courseTitle: string;
  coursePriceCents: number;
};

export function listAdminPromoCodes(courseId?: string) {
  return apiFetch<{ items: AdminPromoCode[]; total: number }>(`/admin/promo-codes${buildQuery({ courseId })}`);
}
