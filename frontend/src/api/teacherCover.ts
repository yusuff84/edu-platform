// T57: API-клиент обложки курса (контракт T54 из Task/00-api-contract.md,
// раздел «Эндпоинты волны P7»). Всё только с credentials:include (cookie-сессии),
// ошибки — через ApiError из api/client.ts.
//
// Почему не apiFetch: он всегда ставит заголовок Content-Type: application/json
// (см. api/client.ts), а multipart-запросу нужен Content-Type с границей формы,
// которую проставляет браузер/undici автоматически вместе с FormData. Ручной
// Content-Type здесь ломает boundary, поэтому транспорт ниже повторяет механику
// apiFetch (BASE, credentials:'include', разбор envelope-ошибок, 204 → undefined),
// но собран вручную и БЕЗ установки Content-Type.
//
// Про CSRF: в api/client.ts никакого X-CSRF-Token нет — сессия держится на
// cookie, Origin проставляет браузер, и apiFetch ничего дополнительного не
// прикрепляет. Ti же правила здесь: никаких самодельных заголовков
// («тем же механизмом, что и apiFetch» = ничего лишнего сверх cookies).

import { ApiError } from './client';

const BASE: string =
  (import.meta.env.VITE_API_URL as string | undefined) ?? '/api/v1';

/** Результат загрузки обложки (POST .../cover → 201 {"coverPath": "..."}). */
export interface CoverUploadResult {
  coverPath: string;
}

/** Допустимые типы обложки по контракту T54 — для accept у input[type=file]. */
export const COVER_ACCEPT = 'image/jpeg,image/png,image/webp';

/** Те же типы списком — для клиентской проверки File.type без строковых сюрпризов. */
export const COVER_TYPES: readonly string[] = ['image/jpeg', 'image/png', 'image/webp'];

/** Максимум по контракту T54: 5 МБ (то же, что на сервере). */
export const COVER_MAX_BYTES = 5 * 1024 * 1024;

/**
 * Разбор ответа как у apiFetch: 204 → undefined; ошибочный статус → ApiError
 * с текстом из envelope {"error": "…"} или запасной формулировкой.
 */
async function toResult<T>(res: Response): Promise<T> {
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  let data: unknown;
  try {
    data = text ? (JSON.parse(text) as unknown) : undefined;
  } catch {
    data = undefined; // не-JSON тело — считаем пустым
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

/**
 * T54: загрузка обложки курса. multipart/form-data с единственным полем `file`;
 * Content-Type НЕ ставим — границу формы проставляет браузер (Fetch API).
 * Успех 201: {"coverPath": "/uploads/covers/<имя>"}. Ошибки: 400/401/403/404/409.
 */
export function uploadCover(courseId: string, file: File): Promise<CoverUploadResult> {
  const form = new FormData();
  form.append('file', file);
  return fetch(`${BASE}/teacher/courses/${encodeURIComponent(courseId)}/cover`, {
    method: 'POST',
    credentials: 'include',
    body: form,
  }).then((res) => toResult<CoverUploadResult>(res));
}

/**
 * T54: убрать обложку курса. Успех 204 (обложки не было — тем же 204) → void.
 * Ошибки: 401/403/404/409 (архивный курс).
 */
export function removeCover(courseId: string): Promise<void> {
  return fetch(`${BASE}/teacher/courses/${encodeURIComponent(courseId)}/cover`, {
    method: 'DELETE',
    credentials: 'include',
  }).then((res) => toResult<void>(res));
}

/**
 * Путь из coverPath ("/uploads/covers/<имя>") → URL для <img>.
 * Если BASE — абсолютный URL (тест-сборка или кастомный деплой), путь
 * подставляется на origin API; при относительном BASE ("/api/v1") путь
 * остаётся как есть: /uploads раздаётся тем же origin (nginx proxy).
 */
export function coverUrl(coverPath: string | null | undefined): string | undefined {
  if (!coverPath) return undefined;
  const m = /^https?:\/\/[^/]+/.exec(BASE);
  return m ? `${m[0]}${coverPath}` : coverPath;
}
