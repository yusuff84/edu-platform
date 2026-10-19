import { useEffect, useState } from 'react';
import type { FormEvent } from 'react';
import { Link, useParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { CourseStatus, PromoValidation } from '../api/client';
import {
  ApiError,
  enroll,
  formatMoney,
  formatPrice,
  getCourse,
  me,
  myCourses,
  validatePromoCode,
} from '../api/client';
import { coverUrl } from '../api/teacherCover';
import Reviews from '../components/Reviews';
import { t } from '../i18n/t';
import '../styles/catalog.css';
import '../styles/promo.css';

/** Человеческая подпись статуса курса (для баннера автора/админа). */
function statusLabel(status: CourseStatus): string {
  switch (status) {
    case 'draft':
      return t('teach.statusDraft');
    case 'review':
      return t('teach.statusReview');
    case 'published':
      return t('teach.statusPublished');
    case 'archived':
      return t('teach.statusArchived');
  }
}

const COVERS = ['cover-py', 'cover-html', 'cover-js', 'cover-sql', 'cover-fig', 'cover-linux'];

/** Фон обложки по слагу. Локальная копия (не зависим от слияния CourseCard). */
function coverFor(key: string): string {
  let h = 0;
  for (let i = 0; i < key.length; i += 1) h = (h * 31 + key.charCodeAt(i)) >>> 0;
  return COVERS[h % COVERS.length];
}

const ALL_CLOSED = '__closed';

export default function Course(): JSX.Element {
  // slug — человекоприятный слаг из URL, не UUID; контракт: GET /courses/{slug}.
  const { slug = '' } = useParams();
  const qc = useQueryClient();
  const [message, setMessage] = useState('');
  const [openModuleId, setOpenModuleId] = useState<string | null>(null);
  // P8: чекаут с промокодом (только платный курс, вошедший студент без записи).
  const [promoOpen, setPromoOpen] = useState(false);
  const [promoInput, setPromoInput] = useState('');
  const [promoValidation, setPromoValidation] = useState<PromoValidation | null>(null);
  const [promoError, setPromoError] = useState('');
  // После T01 getCourse распаковывает конверт { course, modules } в CourseDetail.
  const courseQuery = useQuery({
    queryKey: ['course', slug],
    queryFn: () => getCourse(slug),
    enabled: Boolean(slug),
    retry: false,
  });
  const meQuery = useQuery({ queryKey: ['me'], queryFn: me, retry: false });
  // Активная запись: /me/courses после T01 — массив MyCourse (курс лежит в m.course).
  const myQuery = useQuery({
    queryKey: ['my-courses'],
    queryFn: myCourses,
    enabled: meQuery.isSuccess,
    retry: false,
  });

  const buyMutation = useMutation({
    mutationFn: (vars: { courseId: string; promoCode?: string }) =>
      enroll(vars.courseId, 'mock', vars.promoCode),
    onSuccess: (res) => {
      setMessage('');
      void qc.invalidateQueries({ queryKey: ['my-courses'] });
      if (res.redirectUrl) {
        window.location.href = res.redirectUrl;
        return;
      }
      // Бесплатный курс: запись активна сразу — покажем переход к обучению.
      if (res.enrollmentActive) setMessage(t('course.openLearn'));
    },
    onError: (e: unknown) => {
      if (e instanceof ApiError && e.status === 409) {
        // 409 «уже есть активная запись» — не ошибка, а сообщение.
        setMessage(t('course.alreadyBought'));
        void qc.invalidateQueries({ queryKey: ['my-courses'] });
      } else {
        // Ошибки промокода сервер объясняет текстом (400) — показываем как есть.
        setMessage(e instanceof Error ? e.message : t('common.error'));
      }
    },
  });

  // Проверка промокода до покупки: итоговая цена приходит с сервера.
  const promoMutation = useMutation({
    mutationFn: (code: string) => validatePromoCode(courseQuery.data?.id ?? '', code),
    onSuccess: (res) => {
      setPromoValidation(res);
      setPromoError('');
      setMessage('');
    },
    onError: (e: unknown) => {
      setPromoValidation(null);
      setPromoError(e instanceof Error ? e.message : t('common.error'));
    },
  });

  function onPromoApply(e: FormEvent): void {
    e.preventDefault();
    const code = promoInput.trim();
    if (!code) return;
    promoMutation.mutate(code);
  }

  function onPromoRemove(): void {
    setPromoValidation(null);
    setPromoInput('');
    setPromoError('');
  }

  // При переходе между курсами slug меняется, а компонент остаётся тем же:
  // применённый промокод относится только к предыдущему курсу — сбрасываем.
  const courseId = courseQuery.data?.id;
  useEffect(() => {
    setPromoOpen(false);
    setPromoInput('');
    setPromoValidation(null);
    setPromoError('');
    setMessage('');
  }, [courseId]);

  if (courseQuery.isLoading)
    return (
      <div className="wrap">
        <main className="page">
          <p role="status">{t('course.loading')}</p>
        </main>
      </div>
    );
  if (courseQuery.isError || !courseQuery.data) {
    const status =
      courseQuery.error instanceof ApiError ? courseQuery.error.status : undefined;
    if (status === 404) {
      // Черновик/снятый курс бэкенд отдаёт как 404 для посторонних.
      return (
        <div className="wrap">
          <main className="page">
            <h1>{t('course.notFound')}</h1>
            <p>{t('course.notFoundHint')}</p>
            <p>
              <Link className="btn" to="/">
                {t('my.goCatalog')}
              </Link>
            </p>
          </main>
        </div>
      );
    }
    return (
      <div className="wrap">
        <main className="page">
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
        </main>
      </div>
    );
  }

  const course = courseQuery.data;
  const user = meQuery.data;
  // Кнопка покупки — только когда список записей загружен (не мигает для записанных).
  const owned = (myQuery.data ?? []).some((m) => m.course.id === course.id);
  const modules = course.modules ?? [];
  // Контракт не отдаёт lessonsCount/modulesCount — считаем из модулей.
  const lessonsCount = modules.reduce((n, m) => n + m.lessons.length, 0);
  const modulesCount = modules.length;
  const isAuthor = user?.id === course.teacherId;
  const isAdmin = user?.role === 'admin';
  // Баннер статуса виден только автору/админу: остальным бэкенд такие курсы отдаёт 404.
  const showBanner = course.status !== 'published' && (isAuthor || isAdmin);
  // Курс вернулся в draft с комментарием = отклонён модератором.
  const rejected = course.status === 'draft' && Boolean(course.moderationComment);
  const isOpen = (id: string, index: number): boolean =>
    openModuleId === null ? index === 0 : openModuleId === id;
  const toggle = (id: string, index: number): void =>
    setOpenModuleId(isOpen(id, index) ? ALL_CLOSED : id);
  // T57: обложка из T54 («/uploads/covers/<имя>»); нет обложки — прежний вид.
  const coverSrc = coverUrl(course.coverPath);
  // P8: промо-блок — только платный курс, вошедший студент без активной
  // записи. Бесплатный курс сохраняет прежнее поведение (без промокодов).
  const showPromo =
    Boolean(user) && !owned && !myQuery.isLoading && course.priceCents > 0;

  return (
    <div className="wrap">
      <main className="page">
        <div className="crumbs">
          <Link to="/">{t('catalog.title')}</Link> / {course.title}
        </div>

        {showBanner && (
          <section className="course-banner" aria-label={t('teach.status')}>
            <strong>
              {t('teach.status')}: {statusLabel(course.status)}
            </strong>
            {rejected && (
              <p>
                <strong>{t('teach.rejected')}</strong>
                {course.moderationComment && (
                  <>
                    {' — '}
                    {t('teach.moderationComment')}: {course.moderationComment}
                  </>
                )}
              </p>
            )}
          </section>
        )}

        <div className="two">
          <div style={{ display: 'grid', gap: 22, alignContent: 'start' }}>
            <section className="panel">
              <div className="pill-row">
                <span className="pill">Курс</span>
              </div>
              {coverSrc && (
                <img className="cover-hero" src={coverSrc} alt={t('p7.cover.alt')} />
              )}
              <h1 style={{ marginTop: 12 }}>{course.title}</h1>
              <p className="muted">{course.description}</p>
              <div className="meta">
                <span className="rating">★ {(course.avgRating ?? 0).toFixed(1)}</span>
                <span>({course.ratingsCount ?? 0})</span>
                <span>
                  {lessonsCount} {t('course.lessonsCount')}
                </span>
                <span>
                  {modulesCount} {t('course.modulesCount')}
                </span>
              </div>
            </section>

            <section className="panel">
              <h2>{t('course.program')}</h2>
              {modules.length === 0 && <p className="muted">{t('learn.noLessons')}</p>}
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
            {/* T57: есть обложка — настоящий кадр вместо загадочного градиента. */}
            {coverSrc ? (
              <img className="cover-buy" src={coverSrc} alt="" />
            ) : (
              <div
                className={`cover ${coverFor(course.slug)}`}
                style={{ height: 150, borderRadius: 12 }}
              />
            )}
            {promoValidation ? (
              <div className="price promo-price">
                <s className="promo-price-old">
                  {formatPrice(promoValidation.priceCents)}
                </s>{' '}
                <span>{formatPrice(promoValidation.finalPriceCents)}</span>
              </div>
            ) : (
              <div className="price">{formatPrice(course.priceCents)}</div>
            )}

            {/* P8: промокод в чекауте — тоггл, проверка кода и снятие скидки. */}
            {showPromo && (
              <div className="checkout-promo">
                {!promoValidation && (
                  <>
                    <button
                      type="button"
                      className="promo-toggle"
                      onClick={() => {
                        setPromoOpen((v) => !v);
                        setPromoError('');
                      }}
                      aria-expanded={promoOpen}
                      aria-controls="checkout-promo-form"
                    >
                      {t('p8.checkout.promoToggle')}
                    </button>
                    {promoOpen && (
                      <form
                        id="checkout-promo-form"
                        className="promo-apply"
                        onSubmit={onPromoApply}
                        noValidate
                      >
                        <label htmlFor="checkout-promo-code">
                          {t('p8.checkout.promoLabel')}
                        </label>
                        <div className="promo-apply-row">
                          <input
                            id="checkout-promo-code"
                            className="input"
                            value={promoInput}
                            onChange={(e) => setPromoInput(e.target.value)}
                            autoComplete="off"
                          />
                          <button
                            type="submit"
                            className="btn"
                            disabled={!promoInput.trim() || promoMutation.isPending}
                          >
                            {t('p8.checkout.apply')}
                          </button>
                        </div>
                        {promoError && (
                          <p className="error promo-error" role="alert">
                            {promoError}
                          </p>
                        )}
                      </form>
                    )}
                  </>
                )}
                {promoValidation && (
                  <div className="promo-applied">
                    <p className="promo-applied-line" role="status">
                      <span aria-hidden="true">✓</span> {t('p8.checkout.applied')}:{' '}
                      <strong>{promoValidation.code}</strong> (−{promoValidation.discountPct}%)
                    </p>
                    <button type="button" className="promo-remove" onClick={onPromoRemove}>
                      {t('p8.checkout.remove')}
                    </button>
                  </div>
                )}
              </div>
            )}

            {!user && !meQuery.isLoading && (
              <p className="muted">{t('course.loginToBuy')}</p>
            )}
            {user && !owned && !myQuery.isLoading && (
              <button
                type="button"
                className="btn btn-primary"
                onClick={() =>
                  buyMutation.mutate({
                    courseId: course.id,
                    promoCode: promoValidation?.code,
                  })
                }
                disabled={buyMutation.isPending}
              >
                {buyMutation.isPending
                  ? t('course.buying')
                  : course.priceCents === 0
                    ? t('course.enrollFree')
                    : promoValidation
                      ? `${t('p8.checkout.buyFor')} ${formatMoney(promoValidation.finalPriceCents)}`
                      : t('course.buy')}
              </button>
            )}
            {owned && (
              <Link className="btn btn-primary" to={`/learn/${course.id}`}>
                {t('course.openLearn')}
              </Link>
            )}
            {message && (
              <p role="status" aria-live="polite">
                {message}
              </p>
            )}
            <p className="notice">Возврат в течение 7 дней без лишних вопросов.</p>
          </aside>
        </div>
      </main>
    </div>
  );
}
