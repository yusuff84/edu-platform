// T27: раздел «Курсы» супер-админки (/admin/courses).
// T50: рабочий инструмент модерации — состояние выборки в URL (q, status,
// teacherEmail, sort, dir, from, to, page), чипы статусов со счётчиками из
// сводки, сортировка, модерация прямо в строке (опубликовать / отклонить с
// комментарием / снять с публикации / в архив / вернуть из архива) и
// аккуратная пагинация. Раскрывающаяся панель отзывов (T27) и выгрузка CSV
// (T40) сохранены. Контракт волны P6 — Task/00-api-contract.md.
import { Fragment, useEffect, useRef, useState } from 'react';
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import { Link, useSearchParams } from 'react-router-dom';
import { ApiError, formatDate, formatPrice, moderateCourse } from '../api/client';
import type { CourseStatus } from '../api/client';
import {
  archiveAdminCourse,
  deleteReview,
  getAdminStats,
  listAdminCourses,
  listCourseReviews,
  unarchiveAdminCourse,
  unpublishAdminCourse,
} from '../api/adminConsole';
import type {
  AdminCourseDir,
  AdminCourseRow,
  AdminCourseSort,
  AdminReviewRow,
} from '../api/adminConsole';
import { t } from '../i18n/t';
import type { RuKey } from '../i18n/t';
import CsvExportButton, { csvDateStamp, exportTruncated } from '../components/CsvExportButton';
import '../styles/adminConsole.css';

/** Размеры страницы выборки: 20 по умолчанию, 50 — «широкий» режим (T50). */
const PAGE_LIMITS = [20, 50] as const;

/** Размер страницы по умолчанию — как в контракте и остальных списках консоли. */
const DEFAULT_LIMIT = 20;

/** Пауза дебаунса текстовых фильтров: пока пользователь печатает, запросы не уходят. */
const SEARCH_DEBOUNCE_MS = 300;

/** Размер страницы выгрузки CSV (T40): 100 строк за запрос. */
const EXPORT_PAGE_LIMIT = 100;

/** Потолок выгрузки CSV: не более 50 страниц (5000 строк) за один клик. */
const EXPORT_MAX_PAGES = 50;

/** Статусы курса в порядке контракта (и порядке чипов). */
const COURSE_STATUSES: CourseStatus[] = ['draft', 'review', 'published', 'archived'];

/** Варианты сортировки в порядке select (контракт P6 / T48). */
const COURSE_SORTS: AdminCourseSort[] = ['createdAt', 'updatedAt', 'title', 'price', 'students'];

/** Значения сортировки по умолчанию — те же, что и на backend (T48). */
const DEFAULT_SORT: AdminCourseSort = 'createdAt';
const DEFAULT_DIR: AdminCourseDir = 'desc';

/** Русские подписи статусов — ключи конструктора (бейдж строки). */
const STATUS_LABEL: Record<CourseStatus, RuKey> = {
  draft: 'teach.statusDraft',
  review: 'teach.statusReview',
  published: 'teach.statusPublished',
  archived: 'teach.statusArchived',
};

/** Подписи чипов статусов: множественное число для счётчиков выборки (T50). */
const CHIP_LABEL: Record<CourseStatus, RuKey> = {
  draft: 'adminCourses.statusDraft',
  review: 'adminCourses.statusReview',
  published: 'adminCourses.statusPublished',
  archived: 'adminCourses.statusArchived',
};

/** Подписи вариантов сортировки. */
const SORT_LABEL: Record<AdminCourseSort, RuKey> = {
  createdAt: 'adminCourses.sortCreated',
  updatedAt: 'adminCourses.sortUpdated',
  title: 'adminCourses.sortTitle',
  price: 'adminCourses.sortPrice',
  students: 'adminCourses.sortStudents',
};

/** Действие над курсом прямо из строки списка (T50). */
type CourseAction = 'approve' | 'archive' | 'unpublish' | 'unarchive';

/** Тост после успешного действия — ключи существующего словаря. */
const ACTION_DONE: Record<CourseAction, RuKey> = {
  approve: 'adminCourses.approvedDone',
  archive: 'adminCourses.archived',
  unpublish: 'adminCourses.unpublished',
  unarchive: 'adminCourses.unarchivedDone',
};

function isCourseStatus(value: string): value is CourseStatus {
  return (COURSE_STATUSES as readonly string[]).includes(value);
}

function isCourseSort(value: string): value is AdminCourseSort {
  return (COURSE_SORTS as readonly string[]).includes(value);
}

/**
 * Строгая проверка даты ГГГГ-ММ-ДД: и формат, и реальный календарный день
 * (2026-02-30 отбрасывается — backend тоже отвечает на неё 400).
 */
function isIsoDate(value: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const parsed = new Date(`${value}T00:00:00Z`);
  return !Number.isNaN(parsed.getTime()) && parsed.toISOString().slice(0, 10) === value;
}

/** Текст ошибки без краха страницы: 403/401/400/сеть/прочее (как в Admin.tsx). */
function errorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 403) return t('cabinet.needRole');
    if (error.status === 401) return t('error.needLogin');
    return error.message || t('common.error');
  }
  if (error instanceof TypeError) return t('error.network');
  return t('common.error');
}

/**
 * Раскрытая панель под строкой курса: отзывы курса и удаление отзыва.
 * Монтируется только при раскрытии — запрос уходит по факту показа.
 */
function ReviewsPanel({ courseId }: { courseId: string }): JSX.Element {
  const qc = useQueryClient();
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);

  const reviews = useQuery({
    queryKey: ['admin-course-reviews', courseId],
    queryFn: () => listCourseReviews(courseId),
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  const remove = useMutation({
    mutationFn: (id: string) => deleteReview(id),
    onSuccess: () => {
      setMsg({ ok: true, text: t('adminCourses.reviewRemoved') });
      // После удаления backend пересчитывает рейтинг курса — обновляем
      // и список отзывов, и сводку (средний рейтинг, количество отзывов).
      void qc.invalidateQueries({ queryKey: ['admin-course-reviews', courseId] });
      void qc.invalidateQueries({ queryKey: ['admin-stats'] });
    },
    onError: (e: unknown) => setMsg({ ok: false, text: errorText(e) }),
  });

  function onRemove(review: AdminReviewRow): void {
    if (!window.confirm(t('adminCourses.confirmRemoveReview'))) return;
    setMsg(null);
    remove.mutate(review.id);
  }

  const items = reviews.data?.items ?? [];

  return (
    <div className="ac-reviews">
      {msg && (
        <p
          role={msg.ok ? 'status' : 'alert'}
          className={msg.ok ? 'ac-msg ac-msg--ok' : 'ac-msg ac-msg--error'}
        >
          {msg.text}
        </p>
      )}

      {reviews.isLoading && (
        <p className="muted" role="status">
          {t('common.loading')}
        </p>
      )}

      {reviews.isError && (
        <div className="ac-error" role="alert">
          <p className="error">{errorText(reviews.error)}</p>
          <button
            type="button"
            className="btn btn-sm"
            onClick={() => void reviews.refetch()}
          >
            {t('common.retry')}
          </button>
        </div>
      )}

      {reviews.data && items.length === 0 && (
        <p className="muted">{t('adminCourses.reviewsEmpty')}</p>
      )}

      {items.length > 0 && (
        <ul className="ac-reviews__list">
          {items.map((review) => (
            <li key={review.id} className="ac-reviews__item">
              <div className="ac-reviews__head">
                <span
                  className="ac-stars"
                  role="img"
                  aria-label={`${t('reviews.ratingLabel')}: ${review.rating}`}
                >
                  {'★'.repeat(review.rating)}
                  {'☆'.repeat(Math.max(0, 5 - review.rating))}
                </span>
                <span className="muted">{review.studentEmail || review.studentId}</span>
                <span className="muted">{formatDate(review.createdAt)}</span>
                <button
                  type="button"
                  className="btn btn-sm ac-btn--danger"
                  disabled={remove.isPending}
                  onClick={() => onRemove(review)}
                >
                  {t('adminCourses.removeReview')}
                </button>
              </div>
              {review.body && <p className="ac-reviews__body">{review.body}</p>}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

/**
 * Модалка отклонения курса (T50): комментарий обязателен — пустой блокирует
 * кнопку; ошибка API показывается текстом прямо в окне.
 */
function RejectDialog({
  course,
  onClose,
  onDone,
}: {
  course: AdminCourseRow;
  onClose: () => void;
  onDone: (text: string) => void;
}): JSX.Element {
  const qc = useQueryClient();
  const [comment, setComment] = useState('');
  const [error, setError] = useState('');
  const areaRef = useRef<HTMLTextAreaElement | null>(null);

  // Фокус в поле комментария сразу при открытии окна.
  useEffect(() => {
    areaRef.current?.focus();
  }, []);

  const reject = useMutation({
    mutationFn: () =>
      moderateCourse(course.id, { decision: 'reject', comment: comment.trim() }),
    onSuccess: () => {
      // Курс вернулся в черновики — обновляем список и чипы статусов.
      void qc.invalidateQueries({ queryKey: ['admin-courses'] });
      void qc.invalidateQueries({ queryKey: ['admin-stats'] });
      onDone(t('adminCourses.rejectedDone'));
    },
    onError: (e: unknown) => setError(errorText(e)),
  });

  const canSubmit = comment.trim() !== '' && !reject.isPending;

  return (
    <div
      className="ac-modal"
      onMouseDown={(e) => {
        // Клик мимо окна закрывает модалку, но не во время запроса.
        if (e.target === e.currentTarget && !reject.isPending) onClose();
      }}
      onKeyDown={(e) => {
        if (e.key === 'Escape' && !reject.isPending) onClose();
      }}
    >
      <div
        className="ac-modal__box"
        role="dialog"
        aria-modal="true"
        aria-labelledby="ac-reject-title"
      >
        <h2 id="ac-reject-title" className="ac-modal__title">
          {t('adminCourses.reject')}
        </h2>
        <p className="muted ac-modal__course" title={course.title}>
          {course.title}
        </p>

        <label className="ac-field" htmlFor="ac-reject-comment">
          <span>{t('adminCourses.rejectComment')}</span>
          <textarea
            id="ac-reject-comment"
            ref={areaRef}
            className="input ac-modal__comment"
            rows={4}
            value={comment}
            onChange={(e) => {
              setComment(e.target.value);
              setError('');
            }}
          />
        </label>

        {error && (
          <p role="alert" className="ac-msg ac-msg--error">
            {error}
          </p>
        )}

        <div className="ac-modal__actions">
          <button
            type="button"
            className="btn btn-sm"
            disabled={reject.isPending}
            onClick={onClose}
          >
            {t('common.cancel')}
          </button>
          <button
            type="button"
            className="btn btn-sm ac-btn--danger"
            disabled={!canSubmit}
            onClick={() => {
              setError('');
              reject.mutate();
            }}
          >
            {reject.isPending ? t('common.loading') : t('adminCourses.reject')}
          </button>
        </div>
      </div>
    </div>
  );
}

export default function AdminCourses(): JSX.Element {
  const qc = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();

  // ---- Состояние выборки живёт в URL (T50): ссылкой можно поделиться ----
  // q — поиск по названию, teacherEmail — по автору, status — чип статуса,
  // sort/dir — сортировка, from/to — период создания, page — страница.
  const q = searchParams.get('q') ?? '';
  const teacherEmail = searchParams.get('teacherEmail') ?? '';
  const statusParam = searchParams.get('status') ?? '';
  const status: CourseStatus | '' = isCourseStatus(statusParam) ? statusParam : '';
  const sortParam = searchParams.get('sort') ?? '';
  const sort: AdminCourseSort = isCourseSort(sortParam) ? sortParam : DEFAULT_SORT;
  // Направление по умолчанию — desc (как на backend); в URL пишем только asc.
  const dir: AdminCourseDir = searchParams.get('dir') === 'asc' ? 'asc' : DEFAULT_DIR;
  const from = searchParams.get('from') ?? '';
  const to = searchParams.get('to') ?? '';
  const pageParam = Number(searchParams.get('page'));
  const page = Number.isInteger(pageParam) && pageParam >= 1 ? pageParam : 1;

  // Неверный период (формат или начало позже конца): запрос не уходит,
  // пользователь видит понятный текст admin.periodInvalid.
  const periodInvalid =
    (from !== '' && !isIsoDate(from)) ||
    (to !== '' && !isIsoDate(to)) ||
    (from !== '' && to !== '' && from > to);

  // Свежие параметры URL для отложенных обработчиков (debounce): ссылка
  // обновляется эффектом, поэтому таймер пишет поверх актуального URL.
  const paramsRef = useRef(searchParams);
  useEffect(() => {
    paramsRef.current = searchParams;
  }, [searchParams]);

  /** Точечное обновление query-параметров: null/'' — удалить параметр. */
  function setParams(patch: Record<string, string | null>): void {
    const next = new URLSearchParams(paramsRef.current);
    for (const [key, value] of Object.entries(patch)) {
      if (value === null || value === '') next.delete(key);
      else next.set(key, value);
    }
    setSearchParams(next);
  }

  // ---- Текстовые фильтры: ввод локально, в URL — с дебаунсом ----
  const [qInput, setQInput] = useState(q);
  const [teacherInput, setTeacherInput] = useState(teacherEmail);

  // URL — источник истины: сброс фильтров и переходы по истории обновляют поля.
  useEffect(() => {
    setQInput(q);
  }, [q]);
  useEffect(() => {
    setTeacherInput(teacherEmail);
  }, [teacherEmail]);

  useEffect(() => {
    if (qInput.trim() === q) return;
    const timer = window.setTimeout(() => {
      setParams({ q: qInput.trim() || null, page: null });
    }, SEARCH_DEBOUNCE_MS);
    return () => window.clearTimeout(timer);
    // setParams читает актуальный URL через paramsRef — в зависимостях не нужен.
  }, [qInput, q]);

  useEffect(() => {
    if (teacherInput.trim() === teacherEmail) return;
    const timer = window.setTimeout(() => {
      setParams({ teacherEmail: teacherInput.trim() || null, page: null });
    }, SEARCH_DEBOUNCE_MS);
    return () => window.clearTimeout(timer);
  }, [teacherInput, teacherEmail]);

  // ---- Размер страницы: настройка просмотра, в URL её нет ----
  const [limit, setLimit] = useState<number>(DEFAULT_LIMIT);

  // ---- Выборка курсов по фильтрам ----
  const courses = useQuery({
    queryKey: ['admin-courses', { q, status, teacherEmail, sort, dir, from, to, page, limit }],
    queryFn: () =>
      listAdminCourses({
        q: q || undefined,
        status: status || undefined,
        teacherEmail: teacherEmail || undefined,
        sort,
        dir,
        from: from || undefined,
        to: to || undefined,
        page,
        limit,
      }),
    // Неверный период блокирует запрос: backend ответил бы 400.
    enabled: !periodInvalid,
    // При смене страницы/фильтра старые строки остаются до прихода новых.
    placeholderData: keepPreviousData,
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  // ---- Чипы статусов: счётчики берём из сводки платформы ----
  const stats = useQuery({
    queryKey: ['admin-stats'],
    queryFn: getAdminStats,
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  /** Счётчик чипа: '' — «Все» (курсы всего), иначе — число курсов статуса. */
  function chipCount(value: CourseStatus | ''): number | null {
    const counts = stats.data?.courses;
    if (!counts) return null;
    return value === '' ? counts.total : counts[value];
  }

  // ---- Действия над курсом из строки списка ----
  const [actionMsg, setActionMsg] = useState<{ ok: boolean; text: string } | null>(null);
  // Курс, для которого открыта модалка отклонения.
  const [rejectCourse, setRejectCourse] = useState<AdminCourseRow | null>(null);
  // Курс, для которого раскрыта панель отзывов (одна за раз).
  const [reviewsCourseId, setReviewsCourseId] = useState<string | null>(null);

  const courseAction = useMutation({
    // Возвращаемые формы у действий разные (Course и {course}) — результат не
    // используется, важно лишь завершение запроса и его ошибка.
    mutationFn: (args: { id: string; action: CourseAction }): Promise<unknown> => {
      switch (args.action) {
        case 'approve':
          // Решение модерации — через общий клиент (contract: {decision}).
          return moderateCourse(args.id, { decision: 'approve' });
        case 'archive':
          return archiveAdminCourse(args.id);
        case 'unpublish':
          return unpublishAdminCourse(args.id);
        case 'unarchive':
          return unarchiveAdminCourse(args.id);
      }
    },
    onSuccess: (_res, args) => {
      setActionMsg({ ok: true, text: t(ACTION_DONE[args.action]) });
      // Смена статуса курса видна и в списке, и в чипах (сводка платформы).
      void qc.invalidateQueries({ queryKey: ['admin-courses'] });
      void qc.invalidateQueries({ queryKey: ['admin-stats'] });
    },
    // 409 (курс не в том статусе) и 404 приходят текстом ApiError.message.
    onError: (e: unknown) => setActionMsg({ ok: false, text: errorText(e) }),
  });

  /** Вопрос подтверждения действия: название курса подставляем сами —
   *  в словаре нет интерполяции (t.ts). */
  function confirmQuestion(course: AdminCourseRow, action: CourseAction): string {
    const question =
      action === 'approve'
        ? t('adminCourses.confirmApprove') // ключ уже с «?»
        : action === 'unarchive'
          ? `${t('adminCourses.unarchive')}?`
          : action === 'archive'
            ? `${t('adminCourses.confirmArchive')}?`
            : `${t('adminCourses.confirmUnpublish')}?`;
    return `${question} «${course.title}»`;
  }

  function onCourseAction(course: AdminCourseRow, action: CourseAction): void {
    if (!window.confirm(confirmQuestion(course, action))) return;
    setActionMsg(null);
    courseAction.mutate({ id: course.id, action });
  }

  // ---- Отклонение: модалка с обязательным комментарием ----
  function onRejected(text: string): void {
    setRejectCourse(null);
    setActionMsg({ ok: true, text });
  }

  // ---- Выгрузка CSV: вся выборка по текущим фильтрам ----
  // Идём постранично (limit=100) до total, но не более 50 страниц: один клик
  // выгружает не больше 5000 строк, чтобы не подвесить браузер и стенд.
  // Если total первой страницы больше загруженного — выборка усечена (T43).
  async function loadExportRows(): Promise<{
    rows: Array<Array<string | number | null>>;
    truncated: boolean;
  }> {
    const rows: Array<Array<string | number | null>> = [];
    let total = 0;
    for (let pageNumber = 1; pageNumber <= EXPORT_MAX_PAGES; pageNumber += 1) {
      const res = await listAdminCourses({
        q: q || undefined,
        status: status || undefined,
        teacherEmail: teacherEmail || undefined,
        sort,
        dir,
        from: from || undefined,
        to: to || undefined,
        page: pageNumber,
        limit: EXPORT_PAGE_LIMIT,
      });
      if (pageNumber === 1) total = res.total;
      for (const course of res.items) {
        rows.push([
          course.slug,
          course.title,
          course.teacherEmail,
          t(STATUS_LABEL[course.status]),
          formatPrice(course.priceCents),
          course.modulesCount,
          course.lessonsCount,
          course.studentsCount,
          formatDate(course.createdAt),
          formatDate(course.updatedAt),
        ]);
      }
      // Неполная страница — данных больше нет.
      if (res.items.length < EXPORT_PAGE_LIMIT || rows.length >= res.total) break;
    }
    return { rows, truncated: exportTruncated(total, rows.length) };
  }

  // Заголовки CSV: где есть подходящий ключ — берём его, иначе короткая русская
  // подпись прямо в CSV («Slug», «Создан» — это данные, а не интерфейс).
  const exportHeaders = [
    'Slug',
    t('adminCourses.colTitle'),
    t('adminCourses.colTeacher'),
    t('adminCourses.colStatus'),
    t('adminCourses.colPrice'),
    t('course.modulesCount'),
    t('adminCourses.colLessons'),
    t('adminCourses.colStudents'),
    'Создан',
    t('adminCourses.colUpdated'),
  ];

  // ---- Производные значения пагинации ----
  const data = courses.data;
  const total = data?.total ?? 0;
  const rangeFrom = total === 0 ? 0 : Math.min((page - 1) * limit + 1, total);
  const rangeTo = Math.min(page * limit, total);
  const isLastPage = total > 0 && rangeTo >= total;
  // Пагинатор прячем только на пустой первой странице: если данных не осталось,
  // а страница не первая, кнопка «Назад» вернёт список.
  const showPager = !!data && (total > 0 || page > 1);

  return (
    <div className="wrap">
      <div className="page">
        <main className="ac-page">
          <div className="ac-head">
            <h1>{t('adminCourses.title')}</h1>
            <span className="muted">
              {t('adminCourses.total')}: {total}
            </span>
          </div>

          {/* Чипы статусов со счётчиками: активный чип — текущий статус. */}
          <div className="ac-chips" role="group" aria-label={t('adminCourses.filterStatus')}>
            <button
              type="button"
              className={status === '' ? 'ac-chip ac-chip--active' : 'ac-chip'}
              aria-pressed={status === ''}
              onClick={() => setParams({ status: null, page: null })}
            >
              {t('adminCourses.chipAll')}
              <span className="ac-chip__count">{chipCount('') ?? '—'}</span>
            </button>
            {COURSE_STATUSES.map((s) => (
              <button
                key={s}
                type="button"
                className={status === s ? 'ac-chip ac-chip--active' : 'ac-chip'}
                aria-pressed={status === s}
                onClick={() => setParams({ status: status === s ? null : s, page: null })}
              >
                {t(CHIP_LABEL[s])}
                <span className="ac-chip__count">{chipCount(s) ?? '—'}</span>
              </button>
            ))}
          </div>

          <div className="ac-toolbar">
            <label className="ac-field" htmlFor="ac-courses-search">
              <span>{t('adminCourses.search')}</span>
              <input
                id="ac-courses-search"
                className="input ac-search"
                type="search"
                value={qInput}
                onChange={(e) => setQInput(e.target.value)}
                placeholder={t('adminCourses.search')}
              />
            </label>

            <label className="ac-field" htmlFor="ac-courses-teacher">
              <span>{t('adminCourses.searchTeacher')}</span>
              <input
                id="ac-courses-teacher"
                className="input ac-search"
                type="search"
                value={teacherInput}
                onChange={(e) => setTeacherInput(e.target.value)}
                placeholder={t('adminCourses.searchTeacher')}
              />
            </label>

            <div className="ac-field">
              <span id="ac-courses-sort-label">{t('adminCourses.sort')}</span>
              <div className="ac-sort" role="group" aria-labelledby="ac-courses-sort-label">
                <select
                  className="input"
                  value={sort}
                  aria-label={t('adminCourses.sort')}
                  onChange={(e) => {
                    const next = isCourseSort(e.target.value) ? e.target.value : DEFAULT_SORT;
                    // createdAt — значение по умолчанию: в URL его не держим.
                    setParams({ sort: next === DEFAULT_SORT ? null : next, page: null });
                  }}
                >
                  {COURSE_SORTS.map((s) => (
                    <option key={s} value={s}>
                      {t(SORT_LABEL[s])}
                    </option>
                  ))}
                </select>
                <button
                  type="button"
                  className="btn btn-sm ac-dir"
                  aria-pressed={dir === 'asc'}
                  title={dir === 'asc' ? t('adminCourses.dirAsc') : t('adminCourses.dirDesc')}
                  onClick={() => setParams({ dir: dir === 'asc' ? null : 'asc', page: null })}
                >
                  <span aria-hidden="true">{dir === 'asc' ? '↑' : '↓'}</span>{' '}
                  {dir === 'asc' ? t('adminCourses.dirAsc') : t('adminCourses.dirDesc')}
                </button>
              </div>
            </div>

            {/* Период по дате создания; границы включительные (контракт T48). */}
            <div className="ac-field">
              <span>{t('admin.period')}</span>
              <div className="ac-period" role="group" aria-label={t('admin.period')}>
                <label className="ac-period__item" htmlFor="ac-courses-from">
                  <span>{t('admin.periodFrom')}</span>
                  <input
                    id="ac-courses-from"
                    className="input"
                    type="date"
                    value={from}
                    aria-invalid={from !== '' && !isIsoDate(from) ? true : undefined}
                    onChange={(e) => setParams({ from: e.target.value || null, page: null })}
                  />
                </label>
                <label className="ac-period__item" htmlFor="ac-courses-to">
                  <span>{t('admin.periodTo')}</span>
                  <input
                    id="ac-courses-to"
                    className="input"
                    type="date"
                    value={to}
                    aria-invalid={to !== '' && !isIsoDate(to) ? true : undefined}
                    onChange={(e) => setParams({ to: e.target.value || null, page: null })}
                  />
                </label>
              </div>
            </div>

            <button
              type="button"
              className="btn ac-reset"
              onClick={() => {
                setQInput('');
                setTeacherInput('');
                setSearchParams({});
              }}
              disabled={searchParams.toString() === ''}
            >
              {t('adminCourses.reset')}
            </button>

            {/* T40: выгрузка всей выборки по текущим фильтрам. */}
            <CsvExportButton
              filename={`courses-${csvDateStamp()}.csv`}
              headers={exportHeaders}
              loadRows={loadExportRows}
              disabled={periodInvalid}
            />
          </div>

          {periodInvalid && (
            <p role="alert" className="error">
              {t('admin.periodInvalid')}
            </p>
          )}

          {actionMsg && (
            <p
              role={actionMsg.ok ? 'status' : 'alert'}
              className={actionMsg.ok ? 'ac-msg ac-msg--ok' : 'ac-msg ac-msg--error'}
            >
              {actionMsg.text}
            </p>
          )}

          {courses.isLoading && (
            <p className="muted" role="status">
              {t('common.loading')}
            </p>
          )}

          {courses.isError && (
            <div className="ac-error" role="alert">
              <p className="error">{errorText(courses.error)}</p>
              <button
                type="button"
                className="btn btn-sm"
                onClick={() => void courses.refetch()}
              >
                {t('common.retry')}
              </button>
            </div>
          )}

          {data && data.items.length === 0 && (
            <p className="muted">{t('adminCourses.empty')}</p>
          )}

          {data && data.items.length > 0 && (
            <div
              className="ac-table-wrap"
              role="region"
              aria-label={t('adminCourses.title')}
              tabIndex={0}
            >
              <table className="ac-table">
                <thead>
                  <tr>
                    <th scope="col">{t('adminCourses.colTitle')}</th>
                    <th scope="col">{t('adminCourses.colTeacher')}</th>
                    <th scope="col">{t('adminCourses.colStatus')}</th>
                    <th scope="col">{t('adminCourses.colPrice')}</th>
                    <th scope="col">{t('adminCourses.colStudents')}</th>
                    <th scope="col">{t('adminCourses.colLessons')}</th>
                    <th scope="col">{t('adminCourses.colUpdated')}</th>
                    {/* Ключа «Действия» у раздела курсов нет: adminUsers.colActions. */}
                    <th scope="col">{t('adminUsers.colActions')}</th>
                  </tr>
                </thead>
                <tbody>
                  {data.items.map((course) => {
                    const reviewsOpen = reviewsCourseId === course.id;
                    // На время запроса по этой строке её действия блокируются.
                    const rowBusy =
                      courseAction.isPending && courseAction.variables?.id === course.id;
                    return (
                      <Fragment key={course.id}>
                        <tr className={rowBusy ? 'ac-tr--busy' : undefined}>
                          <td
                            data-label={t('adminCourses.colTitle')}
                            className="ac-td-title"
                          >
                            {/* Название — ссылка на публичную страницу курса (новая вкладка). */}
                            <Link
                              to={`/courses/${encodeURIComponent(course.slug)}`}
                              target="_blank"
                              rel="noreferrer"
                              title={course.title}
                            >
                              {course.title}
                            </Link>
                          </td>
                          <td
                            data-label={t('adminCourses.colTeacher')}
                            className="ac-td-teacher"
                            title={course.teacherEmail}
                          >
                            {course.teacherEmail}
                          </td>
                          <td data-label={t('adminCourses.colStatus')}>
                            <span className={`ac-badge ac-badge--${course.status}`}>
                              {t(STATUS_LABEL[course.status])}
                            </span>
                          </td>
                          <td data-label={t('adminCourses.colPrice')}>
                            {/* Цена курса: 0 — «Бесплатно» (как в очереди модерации). */}
                            {formatPrice(course.priceCents)}
                          </td>
                          <td data-label={t('adminCourses.colStudents')}>
                            {course.studentsCount}
                          </td>
                          <td data-label={t('adminCourses.colLessons')}>
                            {course.lessonsCount}
                          </td>
                          <td data-label={t('adminCourses.colUpdated')}>
                            {formatDate(course.updatedAt)}
                          </td>
                          <td
                            data-label={t('adminUsers.colActions')}
                            className="ac-td-actions"
                          >
                            <div className="ac-actions">
                              <button
                                type="button"
                                className="btn btn-sm"
                                aria-expanded={reviewsOpen}
                                aria-controls={`ac-reviews-${course.id}`}
                                onClick={() =>
                                  setReviewsCourseId(reviewsOpen ? null : course.id)
                                }
                              >
                                {t('adminCourses.reviews')}
                              </button>
                              {/* Ученики курса — та же страница, что в кабинете учителя. */}
                              <Link
                                className="btn btn-sm"
                                to={`/teach/${encodeURIComponent(course.id)}/students`}
                              >
                                {t('adminCourses.students')}
                              </Link>
                              {course.status === 'review' && (
                                <button
                                  type="button"
                                  className="btn btn-sm btn-primary"
                                  disabled={rowBusy}
                                  onClick={() => onCourseAction(course, 'approve')}
                                >
                                  {t('adminCourses.approve')}
                                </button>
                              )}
                              {course.status === 'review' && (
                                <button
                                  type="button"
                                  className="btn btn-sm ac-btn--danger"
                                  disabled={rowBusy}
                                  onClick={() => {
                                    setActionMsg(null);
                                    setRejectCourse(course);
                                  }}
                                >
                                  {t('adminCourses.reject')}
                                </button>
                              )}
                              {course.status === 'published' && (
                                <button
                                  type="button"
                                  className="btn btn-sm"
                                  disabled={rowBusy}
                                  onClick={() => onCourseAction(course, 'unpublish')}
                                >
                                  {t('adminCourses.unpublish')}
                                </button>
                              )}
                              {course.status !== 'archived' && (
                                <button
                                  type="button"
                                  className="btn btn-sm"
                                  disabled={rowBusy}
                                  onClick={() => onCourseAction(course, 'archive')}
                                >
                                  {t('adminCourses.archive')}
                                </button>
                              )}
                              {course.status === 'archived' && (
                                <button
                                  type="button"
                                  className="btn btn-sm"
                                  disabled={rowBusy}
                                  onClick={() => onCourseAction(course, 'unarchive')}
                                >
                                  {t('adminCourses.unarchive')}
                                </button>
                              )}
                            </div>
                          </td>
                        </tr>
                        {reviewsOpen && (
                          <tr className="ac-tr-reviews">
                            <td
                              colSpan={8}
                              id={`ac-reviews-${course.id}`}
                              className="ac-td-reviews"
                            >
                              <ReviewsPanel courseId={course.id} />
                            </td>
                          </tr>
                        )}
                      </Fragment>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}

          {showPager && (
            <div className="ac-pager">
              <div className="ac-pager__nav">
                <button
                  type="button"
                  className="btn btn-sm"
                  disabled={page <= 1 || courses.isFetching}
                  onClick={() => setParams({ page: page > 2 ? String(page - 1) : null })}
                >
                  {t('admin.prev')}
                </button>
                <span className="muted">
                  {t('adminCourses.showing')} {rangeFrom}–{rangeTo} {t('admin.of')} {total}
                </span>
                <button
                  type="button"
                  className="btn btn-sm"
                  disabled={isLastPage || courses.isFetching}
                  onClick={() => setParams({ page: String(page + 1) })}
                >
                  {t('admin.next')}
                </button>
              </div>
              <label className="ac-pager__limit" htmlFor="ac-courses-limit">
                {t('adminCourses.perPage')}
                <select
                  id="ac-courses-limit"
                  className="input"
                  value={limit}
                  disabled={courses.isFetching}
                  onChange={(e) => {
                    setLimit(Number(e.target.value));
                    // Смена размера страницы возвращает на первую.
                    setParams({ page: null });
                  }}
                >
                  {PAGE_LIMITS.map((value) => (
                    <option key={value} value={value}>
                      {value}
                    </option>
                  ))}
                </select>
              </label>
            </div>
          )}

          {/* Модалка отклонения: монтируется только для выбранного курса. */}
          {rejectCourse && (
            <RejectDialog
              course={rejectCourse}
              onClose={() => setRejectCourse(null)}
              onDone={onRejected}
            />
          )}
        </main>
      </div>
    </div>
  );
}
