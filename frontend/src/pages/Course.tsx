import { useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, enroll, formatPrice, getCourse, me, myCourses } from '../api/client';
import Reviews from '../components/Reviews';
import { coverFor } from '../components/CourseCard';
import { t } from '../i18n/t';

const ALL_CLOSED = '__closed';

export default function Course(): JSX.Element {
  const { slug = '' } = useParams();
  const qc = useQueryClient();
  const [message, setMessage] = useState('');
  const [alreadyOwned, setAlreadyOwned] = useState(false);
  const [openModuleId, setOpenModuleId] = useState<string | null>(null);
  const courseQuery = useQuery({
    queryKey: ['course', slug],
    queryFn: () => getCourse(slug),
    enabled: Boolean(slug),
    retry: false,
  });
  const meQuery = useQuery({ queryKey: ['me'], queryFn: me, retry: false });
  const myQuery = useQuery({
    queryKey: ['my-courses'],
    queryFn: myCourses,
    enabled: meQuery.isSuccess,
    retry: false,
  });

  const buyMutation = useMutation({
    mutationFn: (courseId: string) => enroll(courseId, 'mock'),
    onSuccess: (res) => {
      setMessage('');
      setAlreadyOwned(false);
      void qc.invalidateQueries({ queryKey: ['my-courses'] });
      if (res.redirectUrl) window.location.href = res.redirectUrl;
    },
    onError: (e: unknown) => {
      if (e instanceof ApiError && e.status === 409) {
        setAlreadyOwned(true);
        setMessage(t('course.alreadyBought'));
        void qc.invalidateQueries({ queryKey: ['my-courses'] });
      } else setMessage(e instanceof Error ? e.message : t('common.error'));
    },
  });

  if (courseQuery.isLoading)
    return (
      <div className="wrap">
        <div className="page">
          <p>{t('course.loading')}</p>
        </div>
      </div>
    );
  if (courseQuery.isError || !courseQuery.data) {
    const status =
      courseQuery.error instanceof ApiError ? courseQuery.error.status : undefined;
    if (status === 404 || !courseQuery.isError) {
      return (
        <div className="wrap">
          <div className="page">
            <h1>{t('course.notFound')}</h1>
            <p>{t('course.notFoundHint')}</p>
            <p>
              <Link className="btn" to="/">
                {t('my.goCatalog')}
              </Link>
            </p>
          </div>
        </div>
      );
    }
    return (
      <div className="wrap">
        <div className="page">
          <h1>{t('course.notFound')}</h1>
          <p role="alert">{t('common.error')}</p>
          <p>
            <button
              type="button"
              className="btn"
              onClick={() => void courseQuery.refetch()}
            >
              {t('common.retry')}
            </button>{' '}
            <Link className="btn" to="/">
              {t('my.goCatalog')}
            </Link>
          </p>
        </div>
      </div>
    );
  }
  const course = courseQuery.data;
  const owned =
    alreadyOwned || (myQuery.data ?? []).some((m) => m.courseId === course.id);
  const modules = course.modules ?? [];
  const lessonsCount =
    course.lessonsCount ?? modules.reduce((n, m) => n + m.lessons.length, 0);
  const modulesCount = course.modulesCount ?? modules.length;
  const teacherInitials = (course.teacherName ?? '')
    .trim()
    .split(/\s+/)
    .filter(Boolean)
    .map((w) => w.charAt(0))
    .slice(0, 2)
    .join('')
    .toUpperCase();
  const isOpen = (id: string, index: number): boolean =>
    openModuleId === null ? index === 0 : openModuleId === id;
  const toggle = (id: string, index: number): void =>
    setOpenModuleId(isOpen(id, index) ? ALL_CLOSED : id);

  return (
    <div className="wrap">
      <div className="page">
        <div className="crumbs">
          <Link to="/">{t('catalog.title')}</Link> / {course.title}
        </div>
        <div className="two">
          <div style={{ display: 'grid', gap: 22, alignContent: 'start' }}>
            <section className="panel">
              <div className="pill-row">
                <span className="pill">Курс</span>
              </div>
              <h1 style={{ marginTop: 12 }}>{course.title}</h1>
              <p className="muted">{course.description}</p>
              <div className="meta">
                <span className="rating">★ {(course.avgRating ?? 0).toFixed(1)}</span>
                <span>({course.reviewsCount ?? 0})</span>
                <span>
                  {lessonsCount} {t('course.lessons')}
                </span>
                <span>
                  {modulesCount} {t('course.modules')}
                </span>
              </div>
            </section>

            <section className="panel">
              <h2>{t('course.program')}</h2>
              <div style={{ marginTop: 12 }}>
                {modules.map((m, i) => {
                  const open = isOpen(m.id, i);
                  return (
                    <div key={m.id} className="mod">
                      <button
                        type="button"
                        className="mod-head"
                        aria-expanded={open}
                        onClick={() => toggle(m.id, i)}
                      >
                        <span>{m.title}</span>
                        <svg
                          width="16"
                          height="16"
                          viewBox="0 0 16 16"
                          fill="none"
                          aria-hidden="true"
                          style={{
                            flex: 'none',
                            transform: open ? 'rotate(180deg)' : undefined,
                          }}
                        >
                          <path
                            d="M4 6l4 4 4-4"
                            stroke="currentColor"
                            strokeWidth="2"
                            strokeLinecap="round"
                            strokeLinejoin="round"
                          />
                        </svg>
                      </button>
                      {open && (
                        <div className="mod-body">
                          {m.lessons.map((l) =>
                            owned ? (
                              <Link
                                key={l.id}
                                className="lesson-link"
                                to={`/learn/${course.id}/${l.id}`}
                              >
                                <span>{l.title}</span>
                                <span className="pill">{t(`kind.${l.kind}`)}</span>
                              </Link>
                            ) : (
                              <span key={l.id} className="lesson-link">
                                <span>{l.title}</span>
                                <span className="pill">{t(`kind.${l.kind}`)}</span>
                              </span>
                            ),
                          )}
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            </section>

            <section className="panel">
              <h2>Чему вы научитесь</h2>
              <div className="outcomes">
                <div>✓ Писать программы</div>
                <div>✓ Читать ошибки</div>
                <div>✓ Собрать мини-проект</div>
                <div>✓ Задавать вопросы куратору</div>
              </div>
            </section>

            <Reviews courseId={course.id} />
          </div>

          <aside className="panel buybox">
            <div
              className={`cover ${coverFor(course.slug)}`}
              style={{ height: 150, borderRadius: 12 }}
            />
            <div className="price">{formatPrice(course.priceCents)}</div>
            {!meQuery.data && <p className="muted">{t('course.loginToBuy')}</p>}
            {meQuery.data && !owned && (
              <button
                type="button"
                className="btn btn-primary"
                onClick={() => buyMutation.mutate(course.id)}
                disabled={buyMutation.isPending}
              >
                {buyMutation.isPending ? t('course.buying') : t('course.buy')}
              </button>
            )}
            {owned && (
              <Link className="btn btn-primary" to={`/learn/${course.id}`}>
                {t('course.openLearn')}
              </Link>
            )}
            {message && <p role="status">{message}</p>}
            <p className="notice">Возврат в течение 7 дней без лишних вопросов.</p>
            {course.teacherName && (
              <div className="teacher">
                <div className="avatar" aria-hidden="true">
                  {teacherInitials}
                </div>
                <div>
                  <div className="muted">{t('course.teacher')}</div>
                  <div>
                    <strong>{course.teacherName}</strong>
                  </div>
                </div>
              </div>
            )}
          </aside>
        </div>
      </div>
    </div>
  );
}
