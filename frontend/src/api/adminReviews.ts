// T84: API-клиент глобальной модерации отзывов (/admin/reviews).
// Контракт — Task/00-api-contract.md, раздел T83: GET /admin/reviews с
// фильтрами courseId/rating/q/from/to/sort/dir/page/limit и существующий
// DELETE /admin/reviews/{id} (204, 404). Транспорт берётся из client.ts
// (apiFetch/buildQuery), повторно fetch и разбор envelope здесь не пишем.
import { apiFetch, buildQuery } from './client';
// Удаление отзыва уже реализовано в adminConsole.ts (T25) и покрыто
// контрактом — переиспользуем, чтобы не дублировать путь запроса.
import { deleteReview } from './adminConsole';

/** Строка глобального списка отзывов (GET /admin/reviews, T83). */
export type AdminReview = {
  id: string;
  courseId: string;
  courseTitle: string;
  courseSlug: string;
  studentId: string;
  studentEmail: string;
  studentName: string;
  rating: number;
  /** Текст отзыва; у отзыва без текста backend отдаёт пустую строку. */
  body: string;
  createdAt: string;
  /** Ответ преподавателя; пустая строка — ответа нет. */
  teacherReply: string;
  teacherReplyAt: string | null;
};

/** Страница отзывов: total — до пагинации (конверт как в остальных API). */
export type AdminReviewsResponse = {
  items: AdminReview[];
  total: number;
  page: number;
  limit: number;
};

/** Поле сортировки: createdAt (по умолчанию) или rating (T83). */
export type AdminReviewsSort = 'createdAt' | 'rating';

/** Направление сортировки: asc или desc (по умолчанию desc). */
export type AdminReviewsDir = 'asc' | 'desc';

/** Параметры выборки; пустые значения buildQuery в query не отправляет. */
export type AdminReviewsQuery = {
  courseId?: string;
  /** Целое 1..5; иное значение backend отвергает 400. */
  rating?: number;
  q?: string;
  /** Нижняя граница периода по createdAt, ГГГГ-ММ-ДД (включительно). */
  from?: string;
  /** Верхняя граница периода по createdAt, ГГГГ-ММ-ДД (включительно). */
  to?: string;
  sort?: AdminReviewsSort;
  dir?: AdminReviewsDir;
  page?: number;
  limit?: number;
};

/**
 * GET /admin/reviews — отзывы всех курсов для модерации.
 * Пустые строки и undefined buildQuery пропускает: для backend это «любой».
 */
export function listAdminReviews(
  query: AdminReviewsQuery = {},
): Promise<AdminReviewsResponse> {
  return apiFetch<AdminReviewsResponse>(
    `/admin/reviews${buildQuery({
      courseId: query.courseId,
      rating: query.rating,
      q: query.q,
      from: query.from,
      to: query.to,
      sort: query.sort,
      dir: query.dir,
      page: query.page,
      limit: query.limit,
    })}`,
  );
}

/** DELETE /admin/reviews/{id} — 204 без тела; 404, если отзыва нет. */
export function deleteAdminReview(id: string): Promise<void> {
  return deleteReview(id);
}
