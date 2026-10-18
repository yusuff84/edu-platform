// T30: клиент кабинета ученика — прогресс по курсу, профиль, история платежей.
// Контракт — Task/T23-me-extra.md (бэкенд пишется параллельно), поля и коды
// берём буквально. Транспорт — apiFetch/buildQuery из client.ts: не дублируем
// логику запросов и не правим общий client.ts (файл не этой задачи).
import { apiFetch, buildQuery } from './client';
import type { PaymentStatus, User } from './types';

/** Прогресс ученика по курсу (GET /me/courses/{id}/progress). */
export type CourseProgress = {
  courseId: string;
  /** id пройденных уроков в порядке программы; всегда массив (пустой — []). */
  completedLessonIds: string[];
  completedLessons: number;
  totalLessons: number;
  /** completedLessons * 100 / totalLessons, вниз; 0 при totalLessons == 0. */
  percent: number;
  /** Первый непройденный урок по порядку; null — если всё пройдено. */
  nextLessonId: string | null;
  completed: boolean;
};

/** Строка истории платежей (GET /me/payments). */
export type PaymentRow = {
  id: string;
  courseId: string;
  /** Название курса; пустая строка, если курс удалён. */
  courseTitle: string;
  amountCents: number;
  currency: string;
  status: PaymentStatus;
  provider: string;
  createdAt: string;
  /** null — платёж ещё не обработан. */
  processedAt: string | null;
};

/** Страница истории платежей (конверт пагинации контракта T23). */
export type PaymentPage = { items: PaymentRow[]; total: number; page: number; limit: number };

/**
 * Профиль из GET /me/profile: все поля User плюс createdAt.
 * createdAt в общий types.ts не добавлен — этот файл не принадлежит T30,
 * поэтому поле расширяем здесь (подтип User совместим со спекой задачи).
 */
export type MeProfile = User & { createdAt: string };

/** Прогресс ученика по курсу. 403 — нет активной записи, 404 — курса нет. */
export function courseProgress(courseId: string): Promise<CourseProgress> {
  return apiFetch<CourseProgress>(`/me/courses/${encodeURIComponent(courseId)}/progress`);
}

/** Профиль текущего ученика. */
export function meProfile(): Promise<MeProfile> {
  return apiFetch<MeProfile>('/me/profile');
}

/** Сохранение отображаемого имени. Тело ровно { displayName } — лишнее поле → 400. */
export function updateProfile(displayName: string): Promise<MeProfile> {
  return apiFetch<MeProfile>('/me/profile', {
    method: 'PATCH',
    body: JSON.stringify({ displayName }),
  });
}

/** История платежей ученика, новые первыми. */
export function myPayments(params?: { page?: number; limit?: number }): Promise<PaymentPage> {
  return apiFetch<PaymentPage>(
    `/me/payments${buildQuery({ page: params?.page, limit: params?.limit })}`,
  );
}
