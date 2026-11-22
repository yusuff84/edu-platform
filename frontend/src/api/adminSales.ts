// Админка, раздел «Продажи и доступы»: обёртки, которых нет в остальных api-файлах.
// Транспорт — apiFetch/buildQuery из client.ts (backend: handler/admin_payments.go,
// admin_payment_sync.go).
import { useCallback, useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { apiFetch, buildQuery } from './client';
import type { AdminEnrollmentPage } from './adminEnrollments';
import { listAdminCourses, type AdminCourseRow } from './adminConsole';
import type { PaymentJournalItem } from './types';

/** Строка журнала платежей с аддитивными полями промокода (backend отдаёт их всегда). */
export type SalesPayment = PaymentJournalItem & {
  /** Применённый промокод; пустая строка — без кода. */
  promoCode?: string;
  /** Размер скидки по промокоду в копейках. */
  discountCents?: number;
};

/** POST /admin/payments/{id}/sync — сверить статус платежа с провайдером. */
export function syncPayment(id: string): Promise<SalesPayment> {
  return apiFetch<{ payment: SalesPayment }>(`/admin/payments/${encodeURIComponent(id)}/sync`, {
    method: 'POST',
  }).then((res) => res.payment);
}

/** Ключ кэша полного списка курсов для фильтров и форм раздела. */
export const salesCoursesKey = ['admin', 'sales', 'courses'] as const;

/**
 * Все курсы платформы (постранично по 100, не больше 20 страниц) —
 * для фильтров «Курс» и выбора курса в формах.
 */
export async function listAllAdminCourses(): Promise<AdminCourseRow[]> {
  const out: AdminCourseRow[] = [];
  for (let page = 1; page <= 20; page += 1) {
    const res = await listAdminCourses({ page, limit: 100, sort: 'title', dir: 'asc' });
    out.push(...res.items);
    if (res.items.length < 100 || out.length >= res.total) break;
  }
  return out.sort((a, b) => a.title.localeCompare(b.title, 'ru'));
}

/* ------------------------------------------------------------------ */
/* Фильтры списков в адресной строке                                    */
/* ------------------------------------------------------------------ */

/**
 * Состояние фильтров в URL: get(key) и set({key: value|null}).
 * Любое изменение фильтра (кроме самой страницы) сбрасывает page на первую.
 * replace — для частых изменений (ввод в поиске), чтобы не засорять историю.
 */
export function useUrlFilters(): {
  params: URLSearchParams;
  get: (key: string) => string;
  set: (patch: Record<string, string | null>, opts?: { replace?: boolean }) => void;
} {
  const [params, setParams] = useSearchParams();
  const set = useCallback(
    (patch: Record<string, string | null>, opts?: { replace?: boolean }) => {
      setParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          for (const [k, v] of Object.entries(patch)) {
            if (v === null || v === '') next.delete(k);
            else next.set(k, v);
          }
          if (!('page' in patch)) next.delete('page');
          return next;
        },
        { replace: opts?.replace },
      );
    },
    [setParams],
  );
  const get = useCallback((key: string) => params.get(key) ?? '', [params]);
  return { params, get, set };
}

/**
 * Поле поиска, связанное с параметром URL: локальное значение + задержка 300 мс.
 * Переходы «назад/вперёд» обновляют поле из URL.
 */
export function useUrlSearch(
  key: string,
  urlValue: string,
  set: (patch: Record<string, string | null>, opts?: { replace?: boolean }) => void,
): [string, (v: string) => void] {
  const [value, setValue] = useState(urlValue);
  const [debounced, setDebounced] = useState(urlValue);
  useEffect(() => {
    const id = window.setTimeout(() => setDebounced(value), 300);
    return () => window.clearTimeout(id);
  }, [value]);
  useEffect(() => {
    const v = debounced.trim();
    if (v !== urlValue) set({ [key]: v || null }, { replace: true });
    // urlValue намеренно не в зависимостях: реагируем только на ввод.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debounced]);
  useEffect(() => {
    setValue((cur) => (cur.trim() === urlValue ? cur : urlValue));
  }, [urlValue]);
  return [value, setValue];
}

/* ------------------------------------------------------------------ */
/* Даты                                                                 */
/* ------------------------------------------------------------------ */

/** Локальная дата ГГГГ-ММ-ДД со сдвигом на days дней от сегодня. */
export function isoDay(days = 0): string {
  const d = new Date();
  d.setDate(d.getDate() + days);
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${d.getFullYear()}-${m}-${day}`;
}

/** Простая проверка email перед отправкой формы. */
export function looksLikeEmail(v: string): boolean {
  return /^[^\s@]+@[^\s@]+$/.test(v.trim());
}

/** GET /admin/enrollments с фильтром studentId (его нет в обёртке adminEnrollments.ts). */
export function listSalesEnrollments(params: {
  q?: string;
  courseId?: string;
  studentId?: string;
  status?: string;
  page?: number;
  limit?: number;
}): Promise<AdminEnrollmentPage> {
  return apiFetch<AdminEnrollmentPage>(
    `/admin/enrollments${buildQuery({
      q: params.q || undefined,
      courseId: params.courseId || undefined,
      studentId: params.studentId || undefined,
      status: params.status || undefined,
      page: params.page,
      limit: params.limit,
    })}`,
  );
}
