// T29: экран «Ученики курса» кабинета учителя (/teach/:id/students).
// Контракт — Task/T22-teacher-students.md (бэкенд пишется параллельно), поля
// читаются буквально: studentId, email, displayName, enrolledAt, status,
// completedLessons, totalLessons, percent, lastActivityAt.
//
// Визуальный язык прогресс-полосы повторяет MyCourses.tsx (общие классы
// .progress/.progress-bar из styles.css), таблица и пагинация — по образцу
// Admin.tsx. MyCourses.tsx/Admin.tsx/client.ts при этом не меняются.

import { useEffect, useState } from 'react';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { ApiError, formatDate, formatMoney } from '../api/client';
import type { CourseStatus } from '../api/client';
import { courseStats, getTeacherCourse, listCourseStudents } from '../api/teacherStudents';
import type { EnrollmentStatus } from '../api/teacherStudents';
import { t } from '../i18n/t';
import type { RuKey } from '../i18n/t';
// Стили страницы изолированы в styles/courseStudents.css — общий styles.css не трогаем.
import '../styles/courseStudents.css';

// ---------- Константы ----------

/** Пагинация: 20 записей на страницу (контракт T22, значение по умолчанию). */
const LIMIT = 20;

/** Дебаунс поиска до записи в URL, мс (как в Catalog.tsx). */
const DEBOUNCE_MS = 300;

/** Статусы записи на курс → ключи словаря (courseStudents.*). */
const ENROLLMENT_LABEL: Record<EnrollmentStatus, RuKey> = {
  active: 'courseStudents.statusActive',
  refunded: 'courseStudents.statusRefunded',
  revoked: 'courseStudents.statusRevoked',
};

/** Статусы курса для шапки — те же подписи, что в конструкторе (teach.*). */
const COURSE_STATUS_LABEL: Record<CourseStatus, RuKey> = {
  draft: 'teach.statusDraft',
  review: 'teach.statusReview',
  published: 'teach.statusPublished',
  archived: 'teach.statusArchived',
};

// 403/404: текст из ТЗ T29, ключ добавлен координатором после волны.
const MSG_NO_ACCESS = t('courseStudents.noAccess');

// ---------- Утилиты ----------

/** Подпись статуса записи: неизвестное значение с сервера не роняет страницу. */
function enrollmentLabel(status: EnrollmentStatus): string {
  const key = ENROLLMENT_LABEL[status] as RuKey | undefined;
  return key ? t(key) : status;
}

/** Подпись статуса курса (фолбэк — сырое значение, если статус незнакомый). */
function courseStatusLabel(status: CourseStatus): string {
  const key = COURSE_STATUS_LABEL[status] as RuKey | undefined;
  return key ? t(key) : status;
}

/** Текст ошибки страницы: 403/404 — нет курса/доступа, 401 — вход, сеть — как в проекте. */
function errorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 403 || error.status === 404) return MSG_NO_ACCESS;
    if (error.status === 401) return t('error.needLogin');
    return error.message || t('common.error');
  }
  if (error instanceof TypeError) return t('error.network');
  return t('common.error');
}

/** Процент 0..100 (защита от неожиданного значения: стиль width не должен ломаться). */
function clampPercent(value: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.min(100, Math.max(0, Math.round(value)));
}

// ---------- Страница ----------

export default function CourseStudents(): JSX.Element {
  const { id } = useParams();
  const courseId = id ?? '';

  // ---- Поиск и страница живут в URL (?q=, ?page=): ссылку можно передать,
  // ---- «назад/вперёд» браузера и перезагрузка сохраняют состояние.
  const [searchParams, setSearchParams] = useSearchParams();
  const urlQ = searchParams.get('q') ?? '';
  const pageParam = Number(searchParams.get('page'));
  const page = Number.isInteger(pageParam) && pageParam >= 1 ? pageParam : 1;

  // Локальное значение поля ввода; в URL попадает после дебаунса.
  const [q, setQ] = useState(urlQ);

  // Изменение URL извне (кнопка «назад», внешняя ссылка) → текст в поле.
  useEffect(() => {
    setQ(urlQ);
  }, [urlQ]);

  // Дебаунс 300 мс: запрос уходит в ?q=, страница сбрасывается на первую.
  useEffect(() => {
    const next = q.trim();
    if (next === urlQ) return;
    const timer = window.setTimeout(() => {
      setSearchParams(
        (prev) => {
          const p = new URLSearchParams(prev);
          if (next) p.set('q', next);
          else p.delete('q');
          p.delete('page');
          return p;
        },
        { replace: true },
      );
    }, DEBOUNCE_MS);
    return () => window.clearTimeout(timer);
  }, [q, urlQ, setSearchParams]);

  /** Переход по страницам пагинации (?page=; первая страница — без параметра). */
  function goToPage(next: number, replace = false): void {
    setSearchParams(
      (prev) => {
        const p = new URLSearchParams(prev);
        if (next <= 1) p.delete('page');
        else p.set('page', String(next));
        return p;
      },
      { replace },
    );
  }

  // ---- Запросы ----

  // Шапка: название и статус курса. Эндпоинт T13 GET /teacher/courses/{id}
  // проверяет владение (владелец или админ) — обёртка в api/teacherStudents.ts.
  const courseQuery = useQuery({
    queryKey: ['teacher-course', courseId],
    queryFn: () => getTeacherCourse(courseId),
    enabled: courseId !== '',
    retry: false,
  });

  const statsQuery = useQuery({
    queryKey: ['course-students-stats', courseId],
    queryFn: () => courseStats(courseId),
    enabled: courseId !== '',
    // 4xx — не временный сбой, ретраим только сетевые ошибки (как в Admin.tsx).
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  const listQuery = useQuery({
    queryKey: ['course-students', courseId, urlQ, page, LIMIT],
    queryFn: () => listCourseStudents(courseId, { q: urlQ || undefined, page, limit: LIMIT }),
    enabled: courseId !== '',
    // Данные предыдущей страницы держим на экране, пока грузится новая.
    placeholderData: keepPreviousData,
    retry: (failureCount, error) => !(error instanceof ApiError) && failureCount < 2,
  });

  const course = courseQuery.data;
  const stats = statsQuery.data;
  const data = listQuery.data;

  const total = data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / LIMIT));
  const rangeFrom = total === 0 ? 0 : (page - 1) * LIMIT + 1;
  const rangeTo = Math.min(page * LIMIT, total);
  const isLastPage = total > 0 && rangeTo >= total;
  const showPager = Boolean(data) && (total > 0 || page > 1);

  // Страница могла выйти за пределы выдачи — вернуться на последнюю (как в Catalog.tsx);
  // replace, чтобы автокоррекция не копилась в истории переходов.
  useEffect(() => {
    if (data && page > totalPages) goToPage(totalPages, true);
  }, [data, page, totalPages]);

  const isLoading = courseQuery.isLoading || statsQuery.isLoading || listQuery.isLoading;
  const error = courseQuery.error ?? statsQuery.error ?? listQuery.error;

  function retryAll(): void {
    void courseQuery.refetch();
    void statsQuery.refetch();
    void listQuery.refetch();
  }

  return (
    <div className="wrap">
      <div className="page">
        <main className="course-students-page">
          {/* Шапка: ссылка назад в конструктор, заголовок, название и статус курса */}
          <p className="cs-back">
            <Link to={`/teach/${courseId}`}>{t('courseStudents.backToCourse')}</Link>
          </p>
          <h1>{t('courseStudents.title')}</h1>
          {course && (
            <p className="cs-course">
              <span className="cs-course-title">{course.title}</span>
              <span className={`cs-course-status cs-course-status--${course.status}`}>
                {courseStatusLabel(course.status)}
              </span>
            </p>
          )}

          {/* Загрузка */}
          {isLoading && (
            <div className="panel">
              <p role="status">{t('common.loading')}</p>
            </div>
          )}

          {/* Ошибка (403/404 — понятный текст, есть кнопка повтора) */}
          {!isLoading && error && (
            <div className="panel">
              <p role="alert" className="error">
                {errorText(error)}
              </p>
              <p>
                <button type="button" className="btn" onClick={retryAll}>
                  {t('common.retry')}
                </button>
              </p>
            </div>
          )}

          {!isLoading && !error && (
            <>
              {/* Плитки статистики (courseStats) */}
              {stats && (
                <div className="cs-stats">
                  <div className="cs-stat">
                    <b>{stats.studentsTotal}</b>
                    <span>{t('courseStudents.statTotal')}</span>
                  </div>
                  <div className="cs-stat">
                    <b>{stats.studentsActive}</b>
                    <span>{t('courseStudents.statActive')}</span>
                  </div>
                  <div className="cs-stat">
                    <b>{stats.studentsCompleted}</b>
                    <span>{t('courseStudents.statCompleted')}</span>
                  </div>
                  <div className="cs-stat">
                    <b>{stats.avgPercent}%</b>
                    <span>{t('courseStudents.statAvg')}</span>
                  </div>
                  <div className="cs-stat">
                    <b>{formatMoney(stats.revenueCents)}</b>
                    <span>{t('courseStudents.statRevenue')}</span>
                  </div>
                </div>
              )}

              {/* Поиск по email/имени (debounce 300 мс, сброс на первую страницу) */}
              <div className="cs-toolbar">
                <label className="cs-search">
                  <span className="cs-search-label">{t('courseStudents.search')}</span>
                  <input
                    className="input"
                    type="search"
                    value={q}
                    onChange={(e) => setQ(e.target.value)}
                    placeholder={t('courseStudents.search')}
                    autoComplete="off"
                  />
                </label>
              </div>

              {/* Пустое состояние — не пустая таблица */}
              {data && data.items.length === 0 && (
                <div className="empty cs-empty">
                  <p>{t('courseStudents.empty')}</p>
                </div>
              )}

              {/* Таблица учеников */}
              {data && data.items.length > 0 && (
                <div
                  className="cs-table-wrap"
                  role="region"
                  aria-label={t('courseStudents.title')}
                  tabIndex={0}
                >
                  <table className="cs-table">
                    <thead>
                      <tr>
                        <th scope="col">{t('courseStudents.colStudent')}</th>
                        <th scope="col">{t('courseStudents.colEnrolled')}</th>
                        <th scope="col">{t('courseStudents.colProgress')}</th>
                        <th scope="col">{t('courseStudents.colStatus')}</th>
                        <th scope="col">{t('courseStudents.colLastActivity')}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {data.items.map((row) => {
                        const name = row.displayName.trim();
                        const percent = clampPercent(row.percent);
                        return (
                          <tr key={row.studentId}>
                            <td className="cs-td-student" data-label={t('courseStudents.colStudent')}>
                              <span className="cs-student">
                                <b>{name || row.email}</b>
                                {name !== '' && <span className="muted">{row.email}</span>}
                              </span>
                            </td>
                            <td data-label={t('courseStudents.colEnrolled')}>
                              {formatDate(row.enrolledAt)}
                            </td>
                            <td data-label={t('courseStudents.colProgress')}>
                              <span className="cs-progress">
                                <span
                                  className="progress"
                                  role="progressbar"
                                  aria-label={t('courseStudents.colProgress')}
                                  aria-valuemin={0}
                                  aria-valuemax={100}
                                  aria-valuenow={percent}
                                  aria-valuetext={`${row.completedLessons} из ${row.totalLessons}`}
                                >
                                  <span
                                    className="progress-bar"
                                    style={{ width: `${percent}%` }}
                                  />
                                </span>
                                <span className="cs-progress-meta">
                                  {row.completedLessons} из {row.totalLessons}
                                </span>
                              </span>
                            </td>
                            <td data-label={t('courseStudents.colStatus')}>
                              <span className={`cs-badge cs-badge--${row.status}`}>
                                {enrollmentLabel(row.status)}
                              </span>
                            </td>
                            <td data-label={t('courseStudents.colLastActivity')}>
                              {row.lastActivityAt === null ? (
                                <span className="muted">{t('courseStudents.neverActive')}</span>
                              ) : (
                                formatDate(row.lastActivityAt)
                              )}
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              )}

              {/* Пагинация по 20 (серверная) */}
              {showPager && data && (
                <div className="cs-pager">
                  <button
                    type="button"
                    className="btn"
                    disabled={page <= 1 || listQuery.isFetching}
                    onClick={() => goToPage(page - 1)}
                  >
                    {t('common.prev')}
                  </button>
                  <span className="muted">
                    {t('admin.showing')} {rangeFrom}–{rangeTo} {t('admin.of')} {total}
                  </span>
                  <button
                    type="button"
                    className="btn"
                    disabled={isLastPage || listQuery.isFetching}
                    onClick={() => goToPage(page + 1)}
                  >
                    {t('common.next')}
                  </button>
                </div>
              )}
            </>
          )}
        </main>
      </div>
    </div>
  );
}
