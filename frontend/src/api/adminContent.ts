// Раздел «Контент» новой админки: курсы, модерация, вопросы, отзывы.
// Здесь — только общие справочники, ключи запросов и обёртки над
// существующими API (adminConsole, adminHub, client). Свой транспорт не пишем.
import { useCallback, useEffect, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { apiFetch } from './client';
import { listAdminCourses, type AdminCourseRow } from './adminConsole';
import { listAdminReviews, type AdminReview } from './adminHub';
import type { CourseStatus, LessonKind } from './types';

export type CourseTone = 'neutral' | 'ok' | 'warn' | 'danger' | 'info' | 'brand';

/** Статус курса → подпись и цвет бейджа. */
export const COURSE_STATUS: Record<CourseStatus, { label: string; tone: CourseTone }> = {
  draft: { label: 'Черновик', tone: 'neutral' },
  review: { label: 'На проверке', tone: 'warn' },
  published: { label: 'Опубликован', tone: 'ok' },
  archived: { label: 'В архиве', tone: 'info' },
};

export const LESSON_KIND: Record<LessonKind, string> = {
  video_text: 'Видео и текст',
  live: 'Онлайн-занятие',
  quiz: 'Тест',
  code: 'Задание с кодом',
};

/** Ключи запросов раздела — чтобы инвалидировать одним вызовом. */
export const contentKeys = {
  courses: ['admin', 'content', 'courses'] as const,
  courseOptions: ['admin', 'content', 'course-options'] as const,
  course: (id: string) => ['admin', 'content', 'course', id] as const,
  queue: ['admin', 'content', 'queue'] as const,
  questions: ['admin', 'content', 'questions'] as const,
  reviews: ['admin', 'content', 'reviews'] as const,
  promo: (courseId: string) => ['admin', 'content', 'promo', courseId] as const,
  enrollments: (courseId: string) => ['admin', 'content', 'enrollments', courseId] as const,
  stats: ['admin', 'stats'] as const,
};

export type CourseOption = { id: string; title: string; status: CourseStatus };

/** Все курсы платформы (для фильтров «по курсу»): постранично по 100, не больше 20 страниц. */
export async function listCourseOptions(): Promise<CourseOption[]> {
  const out: CourseOption[] = [];
  for (let page = 1; page <= 20; page += 1) {
    const res = await listAdminCourses({ page, limit: 100, sort: 'title', dir: 'asc' });
    out.push(...res.items.map((c: AdminCourseRow) => ({ id: c.id, title: c.title, status: c.status })));
    if (res.items.length < 100 || out.length >= res.total) break;
  }
  return out;
}

/**
 * Все отзывы по фильтрам (для «низкие оценки без ответа»: у API фильтр
 * только по точной оценке, поэтому собираем всё и фильтруем на клиенте).
 */
export async function listAllAdminReviews(params: {
  q?: string;
  courseId?: string;
  noReply?: 1;
}): Promise<AdminReview[]> {
  const out: AdminReview[] = [];
  for (let page = 1; page <= 30; page += 1) {
    const res = await listAdminReviews({ ...params, page, limit: 100 });
    out.push(...res.items);
    if (res.items.length < 100 || out.length >= res.total) break;
  }
  return out;
}

/** Обложка курса: относительный путь /uploads/... или полный URL. */
export function coverUrl(path: string | null | undefined): string | null {
  if (!path) return null;
  return path;
}

/* ------------------------------------------------------------------ */
/* Состояние фильтров: в URL (на странице) или локально (во вкладке).   */
/* ------------------------------------------------------------------ */

export type FilterState = {
  get: (key: string) => string;
  set: (patch: Record<string, string | null>, opts?: { replace?: boolean }) => void;
  reset: () => void;
  isEmpty: (ignore?: string[]) => boolean;
};

/**
 * Фильтры списка. mode='url' — значения живут в адресе (ссылкой можно
 * поделиться, «назад» работает); mode='local' — внутри компонента
 * (для вкладок на странице курса, чтобы не мешать её собственным параметрам).
 */
export function useFilterState(mode: 'url' | 'local'): FilterState {
  const [params, setParams] = useSearchParams();
  const [local, setLocal] = useState<URLSearchParams>(() => new URLSearchParams());
  const current = mode === 'url' ? params : local;

  const set = useCallback(
    (patch: Record<string, string | null>, opts?: { replace?: boolean }) => {
      const apply = (prev: URLSearchParams): URLSearchParams => {
        const next = new URLSearchParams(prev);
        for (const [k, v] of Object.entries(patch)) {
          if (v === null || v === '') next.delete(k);
          else next.set(k, v);
        }
        return next;
      };
      if (mode === 'url') setParams(apply, { replace: opts?.replace });
      else setLocal(apply);
    },
    [mode, setParams],
  );

  const reset = useCallback(() => {
    if (mode === 'url') setParams(new URLSearchParams());
    else setLocal(new URLSearchParams());
  }, [mode, setParams]);

  return {
    get: (key) => current.get(key) ?? '',
    set,
    reset,
    isEmpty: (ignore = []) => [...current.keys()].every((k) => ignore.includes(k)),
  };
}

/**
 * Поле поиска, привязанное к фильтру: ввод мгновенный, запись в фильтр — с
 * задержкой. Внешний сброс (кнопка «Сбросить», «назад») обновляет поле.
 */
export function useDebouncedInput(
  value: string,
  write: (v: string) => void,
  delay = 300,
): [string, (v: string) => void] {
  const [input, setInput] = useState(value);
  const written = useRef(value);
  const writeRef = useRef(write);
  writeRef.current = write;

  useEffect(() => {
    if (value !== written.current) {
      written.current = value;
      setInput(value);
    }
  }, [value]);

  useEffect(() => {
    const id = window.setTimeout(() => {
      const v = input.trim();
      if (v !== written.current) {
        written.current = v;
        writeRef.current(v);
      }
    }, delay);
    return () => window.clearTimeout(id);
  }, [input, delay]);

  return [input, setInput];
}

/** Номер страницы из фильтра (1, если пусто или мусор). */
export function pageOf(raw: string): number {
  const n = Number(raw);
  return Number.isInteger(n) && n >= 1 ? n : 1;
}

/** DELETE /courses/{id}/questions/{qid}/answers/{aid} — удалить ответ (админу доступно в любом курсе). */
export function deleteQuestionAnswer(courseId: string, questionId: string, answerId: string): Promise<void> {
  return apiFetch<void>(
    `/courses/${encodeURIComponent(courseId)}/questions/${encodeURIComponent(questionId)}/answers/${encodeURIComponent(answerId)}`,
    { method: 'DELETE' },
  );
}
