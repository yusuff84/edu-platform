/**
 * Главная: продающий первый экран и блок «Кому подойдёт».
 * Все цифры — настоящие (из каталога), без выдуманных отзывов и статистики.
 * Стили — styles/home.css.
 */
import { Link } from 'react-router-dom';
import type { CourseItem } from '../api/client';
import { formatPrice } from '../api/client';
import { coverUrl } from '../api/teacherCover';
import { pluralRu } from '../i18n/plural';
import '../styles/home.css';

function scrollToId(id: string): void {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
}

function Check(): JSX.Element {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="m5 12.5 4.5 4.5L19 7.5" />
    </svg>
  );
}

/** Карточка курса в композиции первого экрана. */
function HeroCourse({ course, className }: { course: CourseItem; className: string }): JSX.Element {
  const src = coverUrl(course.coverPath);
  const lessons = course.lessonsCount ?? 0;
  return (
    <Link to={`/courses/${course.slug}`} className={`hh-course ${className}`} tabIndex={-1} aria-hidden="true">
      {src ? <img src={src} alt="" loading="eager" /> : <span className="hh-course-fallback" />}
      <span className="hh-course-body">
        <strong>{course.title}</strong>
        <small>
          {lessons > 0 ? `${lessons} ${pluralRu(lessons, 'урок', 'урока', 'уроков')} · ` : ''}
          {formatPrice(course.priceCents)}
        </small>
      </span>
    </Link>
  );
}

export function HomeHero({
  courses,
  total,
  loggedIn,
  onChoose,
}: {
  courses: CourseItem[];
  total: number;
  loggedIn: boolean;
  onChoose: () => void;
}): JSX.Element {
  const lessons = courses.reduce((sum, c) => sum + (c.lessonsCount ?? 0), 0);
  const hasFree = courses.some((c) => c.priceCents <= 0);
  const showcase = courses.filter((c) => c.coverPath).slice(0, 2);
  const stats = [
    total > 0 && { value: String(total), label: pluralRu(total, 'курс', 'курса', 'курсов') + ' с нуля' },
    lessons > 0 && { value: String(lessons), label: pluralRu(lessons, 'урок', 'урока', 'уроков') + ' и тестов' },
    { value: '24/7', label: 'доступ с телефона и ПК' },
    { value: 'QR', label: 'сертификат с проверкой' },
  ].filter(Boolean) as { value: string; label: string }[];

  return (
    <section className="hh" aria-labelledby="hh-title">
      <div className="wrap hh-in">
        <div className="hh-text">
          <span className="hh-eyebrow">
            <span className="hh-dot" aria-hidden="true" />
            Онлайн-курсы по IT{hasFree ? ' · есть бесплатные' : ''}
          </span>
          <h1 id="hh-title">
            Разберитесь в IT с нуля — <span>понятно и на практике</span>
          </h1>
          <p className="hh-lead">
            Короткие уроки со схемами и видеообзорами, тесты с автопроверкой после каждой темы. Учитесь в своём
            темпе с телефона или компьютера — прогресс сохраняется.
          </p>
          <div className="hh-cta">
            {loggedIn ? (
              <Link className="btn btn-primary hh-btn" to="/me">
                Продолжить обучение
              </Link>
            ) : (
              <Link className="btn btn-primary hh-btn" to="/register">
                Начать учиться бесплатно
              </Link>
            )}
            <button type="button" className="btn hh-btn hh-btn-ghost" onClick={onChoose}>
              Смотреть курсы
            </button>
          </div>
          <ul className="hh-points">
            <li>
              <Check /> Без подготовки: всё объясняем с нуля
            </li>
            <li>
              <Check /> Практика и тесты в каждом модуле
            </li>
            <li>
              <Check /> Именной сертификат по итогам
            </li>
          </ul>
        </div>

        <div className="hh-visual" aria-hidden="true">
          {showcase[0] && <HeroCourse course={showcase[0]} className="hh-course--a" />}
          {showcase[1] && <HeroCourse course={showcase[1]} className="hh-course--b" />}
          <div className="hh-float hh-float--test">
            <span className="hh-float-icon hh-float-icon--ok">
              <Check />
            </span>
            <span>
              <strong>Тест пройден</strong>
              <small>6 из 6 верно</small>
            </span>
          </div>
          <div className="hh-float hh-float--progress">
            <small>Прогресс курса</small>
            <span className="hh-bar">
              <i style={{ width: '68%' }} />
            </span>
            <strong>68%</strong>
          </div>
          <div className="hh-float hh-float--cert">
            <span className="hh-float-icon">★</span>
            <span>
              <strong>Сертификат</strong>
              <small>с QR-кодом для проверки</small>
            </span>
          </div>
        </div>
      </div>

      <div className="wrap">
        <dl className="hh-stats">
          {stats.map((s) => (
            <div key={s.label}>
              <dt>{s.value}</dt>
              <dd>{s.label}</dd>
            </div>
          ))}
        </dl>
      </div>
    </section>
  );
}

const AUDIENCE = [
  {
    title: 'Новичкам без опыта',
    text: 'Не нужно ничего знать заранее: термины объясняем простыми словами, сложное — схемами.',
    icon: (
      <path d="M12 3 2 8l10 5 10-5-10-5Zm-6 7.5V15c0 1.7 2.7 3 6 3s6-1.3 6-3v-4.5" />
    ),
  },
  {
    title: 'Тем, кто присматривается к IT',
    text: 'Попробуйте профессию на практике — например, системного администратора — прежде чем менять работу.',
    icon: <path d="M4 7h16v12H4zM9 7V5h6v2M4 12h16" />,
  },
  {
    title: 'Тем, кто работает за компьютером',
    text: 'Разберётесь в сетях, Windows и безопасности — и перестанете зависеть от «знакомого айтишника».',
    icon: <path d="M3 5h18v11H3zM8 20h8M12 16v4" />,
  },
  {
    title: 'Для себя и близких',
    text: 'Научитесь защищать аккаунты, деньги и данные семьи от мошенников и утечек.',
    icon: <path d="M12 3 4.5 6v5.5c0 4.6 3.2 8.2 7.5 9.5 4.3-1.3 7.5-4.9 7.5-9.5V6L12 3Zm-3 9 2 2 4-4" />,
  },
];

export function AudienceBlock(): JSX.Element {
  return (
    <section className="hh-aud" aria-labelledby="hh-aud-title">
      <h2 id="hh-aud-title">Кому подойдут курсы</h2>
      <div className="hh-aud-grid">
        {AUDIENCE.map((a) => (
          <article key={a.title} className="hh-aud-card">
            <span className="hh-aud-icon" aria-hidden="true">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
                {a.icon}
              </svg>
            </span>
            <h3>{a.title}</h3>
            <p>{a.text}</p>
          </article>
        ))}
      </div>
      <button type="button" className="btn btn-ghost hh-aud-cta" onClick={() => scrollToId('lead')}>
        Не уверены, что выбрать? Поможем подобрать курс
      </button>
    </section>
  );
}
