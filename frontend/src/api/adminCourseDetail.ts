// T58: API-клиент админ-страницы «Детали курса» (GET /admin/courses/{id}).
// Контракт — раздел «T55» в Task/00-api-contract.md (волна P7, ЗАМОРОЖЕН):
// форма ответа { course, modules, stats } строго по документу. Транспорт —
// общий apiFetch из client.ts (экспортирован для модулей-компаньонов), свой
// fetch и обработку envelope {"error"} здесь не дублируем.
//
// Отзывы курса остаются прежними (T25): listCourseReviews/deleteReview
// уже есть в adminConsole.ts — страница импортирует их оттуда, здесь их нет.

import { apiFetch } from './client';
import type { LessonKind } from './client';
import type { AdminCourseRow } from './adminConsole';

/** Строка курса в деталях: элемент GET /admin/courses (adminConsole.ts) + обложка (T54). */
export type AdminCourseDetailCourse = AdminCourseRow & {
  /** Путь к обложке (/uploads/covers/…); null/отсутствует — обложки нет (T54). */
  coverPath?: string | null;
};

/** Метаданные урока в деталях курса: ТОЛЬКО id/title/kind/position, тела уроков нет. */
export type AdminCourseDetailLesson = {
  id: string;
  title: string;
  kind: LessonKind;
  position: number;
};

/** Модуль в элементах деталей курса. */
export type AdminCourseDetailModule = {
  id: string;
  title: string;
  position: number;
  lessons: AdminCourseDetailLesson[];
};

/**
 * Показатели курса (GET /admin/courses/{id}, stats): studentsCount — все записи
 * (любой статус); activeCount — только active; paymentsCount/paidTotalCents —
 * только платежи succeeded; avgRating/reviewsCount — как в публичной карточке.
 */
export type AdminCourseStats = {
  studentsCount: number;
  activeCount: number;
  reviewsCount: number;
  avgRating: number;
  paymentsCount: number;
  paidTotalCents: number;
};

/** Ответ GET /admin/courses/{id} — конверт строго из трёх разделов. */
export type AdminCourseDetail = {
  course: AdminCourseDetailCourse;
  /** Модули отсортированы по position; уроки внутри — тоже по position. */
  modules: AdminCourseDetailModule[];
  stats: AdminCourseStats;
};

/**
 * GET /admin/courses/{id} — детали курса одним экраном.
 * Ошибки: 401 — не вошёл, 403 — не админ, 404 «Курс не найден» (ApiError.status).
 */
export function listAdminCourseDetail(courseId: string): Promise<AdminCourseDetail> {
  return apiFetch<AdminCourseDetail>(`/admin/courses/${encodeURIComponent(courseId)}`);
}
