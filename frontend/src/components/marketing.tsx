import { useState } from 'react';
import type { FormEvent } from 'react';
import { useMutation } from '@tanstack/react-query';
import { createLead } from '../api/leads';
import type { LeadContactMethod } from '../api/leads';
import { CertificateDocument } from './CertificateDocument';
import { SupportIllustration } from './SupportIllustrations';

function CheckIcon(): JSX.Element {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" aria-hidden="true">
      <circle cx="12" cy="12" r="11" fill="#1F9D55" />
      <path
        d="M8 12.5l2.7 2.7L16.5 9"
        fill="none"
        stroke="#fff"
        strokeWidth="2.2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

type SupportKey = 'lessons' | 'tests' | 'questions' | 'certificate';

const SUPPORT_TABS: Array<{ key: SupportKey; label: string }> = [
  { key: 'lessons', label: 'Видео и конспекты' },
  { key: 'tests', label: 'Проверка знаний' },
  { key: 'questions', label: 'Вопросы к уроку' },
  { key: 'certificate', label: 'Сертификат' },
];

// Только то, что платформа действительно умеет сегодня (без выдуманных
// кураторов, цифр, «центра карьеры» и трудоустройства).
const SUPPORT_TEXT: Record<
  SupportKey,
  { title: string; text: string; icon: string; card: string; cardSmall: string }
> = {
  lessons: {
    title: 'Короткие уроки, которые удобно смотреть',
    text: 'Каждая тема — видеоурок и текстовый конспект. Смотрите с телефона или компьютера, возвращайтесь к материалам в любой момент.',
    icon: '▶',
    card: 'Видеоурок + конспект',
    cardSmall: 'Прогресс сохраняется автоматически',
  },
  tests: {
    title: 'Тесты с автоматической проверкой',
    text: 'После тем — тесты. Ответы проверяются сразу: видно, что усвоено, а что стоит повторить. Пересдать можно в любой момент.',
    icon: '✓',
    card: 'Зачтено',
    cardSmall: 'Результат — сразу после отправки',
  },
  questions: {
    title: 'Можно спросить, если непонятно',
    text: 'Под каждым уроком есть раздел вопросов и ответов: спросите преподавателя или помогите другим ученикам.',
    icon: '?',
    card: 'Вопросы и ответы',
    cardSmall: 'Под каждым уроком курса',
  },
  certificate: {
    title: 'Сертификат о прохождении курса',
    text: 'Пройдите все уроки и тесты — платформа выдаст именной сертификат с номером и публичной страницей для проверки.',
    icon: '★',
    card: 'Сертификат',
    cardSmall: 'С номером и ссылкой для проверки',
  },
};

export function SupportBlock(): JSX.Element {
  const [tab, setTab] = useState<SupportKey>('lessons');
  const s = SUPPORT_TEXT[tab];
  return (
    <div className="support">
      <h2>Как устроено обучение</h2>
      <div className="support-card">
        <div className="seg" role="tablist">
          {SUPPORT_TABS.map((t) => (
            <button
              key={t.key}
              type="button"
              role="tab"
              aria-selected={tab === t.key}
              className={tab === t.key ? 'active' : ''}
              onClick={() => setTab(t.key)}
            >
              {t.label}
            </button>
          ))}
        </div>
        <div className="support-grid">
          <div>
            <h3>{s.title}</h3>
            <p>{s.text}</p>
          </div>
          <div className="support-visual support-visual--simple" aria-hidden="true">
            <div className="svis-simple" key={tab}>
              <SupportIllustration kind={tab} />
              <p className="sv-title">{s.card}</p>
              <p className="sv-small">{s.cardSmall}</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

export function PerksBlock(): JSX.Element {
  return (
    <div className="perks">
      <div className="perk perk-1">
        <div className="perk-ico" aria-hidden="true">
          <svg width="120" height="100" viewBox="0 0 120 100">
            <ellipse cx="60" cy="91" rx="32" ry="6" fill="#141414" opacity=".12" />
            <circle cx="58" cy="48" r="34" fill="#fff" stroke="#E2DDD1" strokeWidth="3" />
            <circle cx="58" cy="48" r="27" fill="none" stroke="#ECEAE4" strokeWidth="2" strokeDasharray="2 12.1" />
            <rect x="56" y="18" width="4" height="8" rx="2" fill="#141414" />
            <rect x="56" y="70" width="4" height="8" rx="2" fill="#ECEAE4" />
            <rect x="28" y="46" width="8" height="4" rx="2" fill="#ECEAE4" />
            <rect x="80" y="46" width="8" height="4" rx="2" fill="#ECEAE4" />
            <path d="M58 48V30" stroke="#3B5BFF" strokeWidth="5" strokeLinecap="round" />
            <path d="M58 48l13 9" stroke="#141414" strokeWidth="5" strokeLinecap="round" />
            <circle cx="58" cy="48" r="5" fill="#E63223" stroke="#fff" strokeWidth="2" />
            <path d="M96 12l2.8 6.7 6.7 2.8-6.7 2.8L96 31l-2.8-6.7-6.7-2.8 6.7-2.8z" fill="#FEC466" />
          </svg>
        </div>
        <b>Свободный график без дедлайнов</b>
        <p>Обучение легко совмещать с работой и личной жизнью</p>
      </div>
      <div className="perk perk-2">
        <div className="perk-ico" aria-hidden="true">
          <svg width="120" height="100" viewBox="0 0 120 100">
            <ellipse cx="60" cy="91" rx="32" ry="6" fill="#141414" opacity=".12" />
            <path d="M44 46V30a16 16 0 0 1 31-5.5" stroke="#FEC466" strokeWidth="8" fill="none" strokeLinecap="round" />
            <rect x="32" y="44" width="56" height="42" rx="11" fill="#3B5BFF" />
            <circle cx="60" cy="61" r="6" fill="#fff" />
            <rect x="57.5" y="62" width="5" height="12" rx="2.5" fill="#fff" />
            <circle cx="88" cy="78" r="14" fill="#1F9D55" stroke="#fff" strokeWidth="3" />
            <path d="M80.5 78c0-3 2-5 4.2-5 3.6 0 3.6 10 7.2 10 2.2 0 4.2-2 4.2-5s-2-5-4.2-5c-3.6 0-3.6 10-7.2 10-2.2 0-4.2-2-4.2-5Z" fill="none" stroke="#fff" strokeWidth="2.6" strokeLinejoin="round" />
          </svg>
        </div>
        <b>Бессрочный доступ к курсу и обновлениям</b>
        <p>Можно вернуться к занятиям и освежить знания в любой момент</p>
      </div>
      <div className="perk dark">
        <div className="perk-ico" aria-hidden="true">
          <svg width="120" height="100" viewBox="0 0 120 100">
            <ellipse cx="60" cy="91" rx="30" ry="6" fill="#141414" opacity=".12" />
            <g transform="rotate(35 60 50)">
              <path d="M53 66h14l-7 18z" fill="#FEC466" />
              <path d="M56 66h8l-4 10z" fill="#E63223" />
              <path d="M46 52l-12 14 14-3z" fill="#E63223" />
              <path d="M74 52l12 14-14-3z" fill="#E63223" />
              <path d="M60 10c13 11 17 30 14 56H46c-3-26 1-45 14-56z" fill="#fff" stroke="#E2DDD1" strokeWidth="2.5" strokeLinejoin="round" />
              <circle cx="60" cy="36" r="8" fill="#3B5BFF" stroke="#fff" strokeWidth="3" />
              <rect x="50" y="56" width="20" height="5" rx="2.5" fill="#ECEAE4" />
            </g>
            <circle cx="24" cy="26" r="3.5" fill="#FEC466" />
            <path d="M98 70l2.4 5.6 5.6 2.4-5.6 2.4L98 86l-2.4-5.6-5.6-2.4 5.6-2.4z" fill="#FEC466" />
          </svg>
        </div>
        <b>Бесплатный старт</b>
        <p>Начните с бесплатного курса — без оплаты и привязки карты.</p>
      </div>
    </div>
  );
}

export function DiplomaBlock({ courseTitle }: { courseTitle?: string }): JSX.Element {
  return (
    <div className="diploma diploma--real">
      <div>
        <h2>Сертификат о&nbsp;прохождении курса</h2>
        <p>
          Пройдите все уроки и&nbsp;тесты — платформа сразу выдаст именной сертификат. У&nbsp;каждого
          сертификата есть номер и&nbsp;QR-код: по&nbsp;нему любой может проверить подлинность.
          Сертификат можно скачать в&nbsp;PDF, отправить работодателю или приложить к&nbsp;резюме.
        </p>
      </div>
      <div className="diploma-preview" aria-hidden="true">
        <CertificateDocument
          number="EDU-2026-PREVIEW"
          studentName="Ваше имя"
          courseTitle={courseTitle || 'Название курса'}
          issuedAt={new Date().toISOString()}
          verifyUrl="https://skillex.space/certificates/EDU-2026-PREVIEW"
        />
      </div>
    </div>
  );
}

const FAQ: Array<{ q: string; a: string }> = [
  { q: 'Это правда бесплатно?', a: 'Да. Бесплатные курсы открываются сразу после регистрации — без оплаты и без привязки карты.' },
  { q: 'Нужна ли подготовка?', a: 'Нет. Курсы рассчитаны на начинающих: всё объясняется простым языком, с примерами из жизни.' },
  { q: 'Сколько времени займёт обучение?', a: 'Учитесь в своём темпе: уроки короткие, прогресс сохраняется. Можно проходить по 15–20 минут в день.' },
  { q: 'Можно учиться с телефона?', a: 'Да. Видео, конспекты и тесты удобно проходить со смартфона — и продолжить с компьютера с того же места.' },
  { q: 'Как получить сертификат?', a: 'Пройдите все уроки и тесты курса — сертификат появится автоматически. Его можно скачать в PDF и поделиться ссылкой.' },
  { q: 'Что если не получается пройти тест?', a: 'Количество попыток не ограничено. Вернитесь к материалам урока, а если что-то непонятно — задайте вопрос под уроком.' },
];

export function FaqBlock(): JSX.Element {
  return (
    <section className="faq" aria-labelledby="faq-title">
      <h2 id="faq-title">Частые вопросы</h2>
      <div className="faq-list">
        {FAQ.map((item) => (
          <details key={item.q} className="faq-item">
            <summary>{item.q}</summary>
            <p>{item.a}</p>
          </details>
        ))}
      </div>
    </section>
  );
}

export function FinalCta({ onChoose }: { onChoose: () => void }): JSX.Element {
  return (
    <div className="cta">
      <svg className="cta-f cta-f1" viewBox="0 0 120 120" aria-hidden="true">
        <rect width="120" height="120" rx="30" fill="#E3D4F7" />
        <path d="M26 120c3-22 16-32 34-32s31 10 34 32z" fill="#3B6EA5" />
        <circle cx="60" cy="60" r="24" fill="#F0C39E" />
        <path
          d="M36 58c-2-18 10-30 24-30s26 12 24 30l9 3c3 1 2 6-2 6H37c-4 0-5-5-2-6z"
          fill="#2F4BFF"
        />
        <rect x="36" y="56" width="48" height="6" rx="3" fill="#1F3AE0" />
      </svg>
      <svg className="cta-f cta-f2" viewBox="0 0 120 120" aria-hidden="true">
        <rect width="120" height="120" rx="30" fill="#C6F24E" />
        <circle cx="66" cy="46" r="25" fill="none" stroke="#F055D0" strokeWidth="12" />
        <circle cx="46" cy="26" r="10" fill="#FEC94E" />
        <path d="M0 92q32-30 62-12t58-8v48H0z" fill="#3B5BFF" />
      </svg>
      <svg className="cta-f cta-f3" viewBox="0 0 100 112" aria-hidden="true">
        <g fill="#3B5BFF">
          <rect x="28" y="16" width="17" height="54" rx="8.5" transform="rotate(-8 36 43)" />
          <rect x="55" y="12" width="17" height="58" rx="8.5" transform="rotate(7 63 41)" />
          <rect x="26" y="58" width="50" height="40" rx="17" />
          <rect x="64" y="66" width="20" height="15" rx="7.5" transform="rotate(32 74 73)" />
        </g>
        <rect
          x="28"
          y="16"
          width="17"
          height="54"
          rx="8.5"
          transform="rotate(-8 36 43)"
          fill="#fff"
          opacity=".14"
        />
      </svg>
      <h2>Сделайте первый шаг к&nbsp;жизни, которую хотите</h2>
      <p>Начните с бесплатного курса — регистрация займёт минуту.</p>
      <button type="button" className="btn btn-blue" onClick={onChoose}>
        Выбрать курс
      </button>
    </div>
  );
}

const CONTACTS = [
  { key: 'phone', label: 'По телефону', bg: '#1F9D55', glyph: 'Т' },
  { key: 'telegram', label: 'В Telegram', bg: '#229ED9', glyph: 'Т' },
  { key: 'vk', label: 'Во VK', bg: '#0077FF', glyph: 'VK' },
  { key: 'max', label: 'В MAX', bg: '#7B5CFF', glyph: 'M' },
];

/** Иллюстрация «консультация» — в стиле иконок сайта: облака диалога, галочка, искорка, тень. */
function LeadIllustration(): JSX.Element {
  return (
    <svg className="lead-illu" viewBox="0 0 240 150" aria-hidden="true">
      <ellipse cx="120" cy="140" rx="84" ry="8" fill="#141414" opacity=".1" />
      {/* вопрос ученика */}
      <path d="M40 18h96a16 16 0 0 1 16 16v34a16 16 0 0 1-16 16H82l-22 18V84H40a16 16 0 0 1-16-16V34a16 16 0 0 1 16-16Z" fill="#3B5BFF" />
      <rect x="44" y="38" width="76" height="7" rx="3.5" fill="#fff" />
      <rect x="44" y="54" width="52" height="7" rx="3.5" fill="#fff" opacity=".55" />
      {/* ответ консультанта */}
      <path d="M118 64h86a14 14 0 0 1 14 14v28a14 14 0 0 1-14 14h-8v16l-19-16h-59a14 14 0 0 1-14-14V78a14 14 0 0 1 14-14Z" fill="#fff" stroke="#E2DDD1" strokeWidth="2.5" strokeLinejoin="round" />
      <rect x="122" y="82" width="62" height="6" rx="3" fill="#ECEAE4" />
      <rect x="122" y="96" width="44" height="6" rx="3" fill="#ECEAE4" />
      <circle cx="200" cy="64" r="16" fill="#1F9D55" stroke="#fff" strokeWidth="4" />
      <path d="M192 64l5.5 5.5 11-11" stroke="#fff" strokeWidth="3.5" fill="none" strokeLinecap="round" strokeLinejoin="round" />
      {/* искорки */}
      <path d="M176 14l3 7.2 7.2 3-7.2 3-3 7.2-3-7.2-7.2-3 7.2-3z" fill="#FEC466" />
      <circle cx="20" cy="104" r="6" fill="#E63223" />
    </svg>
  );
}

/** Иконки способов связи — круглые, как иконки приложений: градиент, крупный знак. */
const CONTACT_GRADIENT: Record<string, [string, string]> = {
  phone: ['#34C759', '#1F9D55'],
  telegram: ['#3EB4F0', '#1D8FD6'],
  vk: ['#2E8BFF', '#0060E6'],
  max: ['#9B7BFF', '#6A3DFF'],
};

function ContactIcon({ kind }: { kind: string }): JSX.Element {
  const [from, to] = CONTACT_GRADIENT[kind] ?? CONTACT_GRADIENT.max;
  const id = `cg-${kind}`;
  return (
    <svg width="36" height="36" viewBox="0 0 36 36" aria-hidden="true">
      <defs>
        <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor={from} />
          <stop offset="1" stopColor={to} />
        </linearGradient>
      </defs>
      <circle cx="18" cy="18" r="18" fill={`url(#${id})`} />
      {kind === 'phone' && (
        <path
          fill="#fff"
          d="M13.2 9.6c.5-.5 1.3-.5 1.8 0l2.1 2.6c.4.5.4 1.2 0 1.7l-1.3 1.5a11.4 11.4 0 0 0 4.8 4.8l1.5-1.3c.5-.4 1.2-.4 1.7 0l2.6 2.1c.5.5.5 1.3 0 1.8l-1.4 1.6c-.8.9-2.1 1.2-3.2.8-5-1.9-8.8-5.7-10.7-10.7-.4-1.1-.1-2.4.8-3.2z"
        />
      )}
      {kind === 'telegram' && (
        <path
          fill="#fff"
          d="M25.6 10.3 8.9 16.8c-.9.3-.9 1.6 0 1.9l4.2 1.4 1.6 5.1c.2.6 1 .8 1.5.3l2.4-2.2 4.2 3.1c.5.4 1.3.1 1.5-.6l2.8-14.3c.2-.8-.6-1.5-1.5-1.2Zm-3.3 4-7.2 6.6-.3 2.9-1.3-4 8.4-5.8c.3-.2.6.1.4.3Z"
        />
      )}
      {kind === 'vk' && (
        <text x="18" y="22.5" textAnchor="middle" fontSize="12.5" fontWeight="800" fill="#fff" fontFamily="inherit" letterSpacing="-0.4">
          VK
        </text>
      )}
      {kind === 'max' && (
        <>
          <path fill="#fff" d="M18 9.5c-5.2 0-9.4 3.5-9.4 7.9 0 2.3 1.1 4.3 2.9 5.7l-.6 3.4 3.6-1.8c1.1.4 2.3.6 3.5.6 5.2 0 9.4-3.5 9.4-7.9s-4.2-7.9-9.4-7.9Z" />
          <circle cx="14" cy="17.4" r="1.4" fill={to} />
          <circle cx="18" cy="17.4" r="1.4" fill={to} />
          <circle cx="22" cy="17.4" r="1.4" fill={to} />
        </>
      )}
    </svg>
  );
}

export function LeadBlock(): JSX.Element {
  const [form, setForm] = useState({
    name: '',
    phone: '',
    email: '',
    contactMethod: 'phone' as LeadContactMethod,
    consent: false,
    website: '',
  });
  const mutation = useMutation({ mutationFn: createLead });
  const sent = mutation.isSuccess;
  const set = <K extends keyof typeof form>(key: K, value: (typeof form)[K]): void =>
    setForm((prev) => ({ ...prev, [key]: value }));
  function onSubmit(e: FormEvent): void {
    e.preventDefault();
    if (mutation.isPending || sent) return;
    mutation.mutate(form);
  }
  return (
    <div className="lead-zone" id="lead">
      <div className="lead-card">
        <div>
          <h2>Поможем подобрать онлайн-курс под ваши цели</h2>
          <p className="lead-sub">
            Оставьте заявку — свяжемся, ответим на вопросы и поможем выбрать курс под ваш
            уровень и цели.
          </p>
          <ul className="lead-list">
            <li>
              <CheckIcon />
              <span>Бесплатно проконсультируем по обучению</span>
            </li>
            <li>
              <CheckIcon />
              <span>Расскажем, как устроены курсы и проверка знаний</span>
            </li>
            <li>
              <CheckIcon />
              <span>Подберём курс под ваш уровень</span>
            </li>
            <li>
              <CheckIcon />
              <span>Ответим на вопросы до начала обучения</span>
            </li>
          </ul>
          <LeadIllustration />
        </div>
        <form className="lead-form" onSubmit={onSubmit} noValidate={false}>
          <input
            className="input"
            required
            minLength={2}
            maxLength={100}
            placeholder="Имя"
            autoComplete="name"
            aria-label="Имя"
            value={form.name}
            onChange={(e) => set('name', e.target.value)}
            disabled={sent}
          />
          <input
            className="input"
            required
            type="tel"
            placeholder="Телефон"
            inputMode="tel"
            autoComplete="tel"
            aria-label="Телефон"
            value={form.phone}
            onChange={(e) => set('phone', e.target.value)}
            disabled={sent}
          />
          <input
            className="input"
            type="email"
            placeholder="Электронная почта (необязательно)"
            autoComplete="email"
            aria-label="Электронная почта"
            value={form.email}
            onChange={(e) => set('email', e.target.value)}
            disabled={sent}
          />
          {/* Ловушка для ботов: людям не видна и недоступна с клавиатуры. */}
          <input
            className="lead-hp"
            type="text"
            name="website"
            tabIndex={-1}
            autoComplete="off"
            aria-hidden="true"
            value={form.website}
            onChange={(e) => set('website', e.target.value)}
          />
          <p className="sv-title">Как с вами связаться?</p>
          <div className="contact-grid">
            {CONTACTS.map((c) => (
              <label className="contact" key={c.key}>
                <input
                  type="radio"
                  name="cm"
                  checked={form.contactMethod === c.key}
                  onChange={() => set('contactMethod', c.key as LeadContactMethod)}
                  disabled={sent}
                />
                <span className="cm cm--round">
                  <ContactIcon kind={c.key} />
                </span>
                {c.label}
              </label>
            ))}
          </div>
          <label className="consent">
            <input
              type="checkbox"
              required
              checked={form.consent}
              onChange={(e) => set('consent', e.target.checked)}
              disabled={sent}
            />
            <span>Я соглашаюсь на обработку персональных данных</span>
          </label>
          <button className="btn btn-primary" type="submit" disabled={sent || mutation.isPending}>
            {sent ? 'Заявка отправлена' : mutation.isPending ? 'Отправляем…' : 'Получить консультацию'}
          </button>
          {mutation.isError && (
            <p className="error" role="alert">
              {mutation.error instanceof Error ? mutation.error.message : 'Не удалось отправить заявку'}
            </p>
          )}
          {sent && (
            <p className="sv-small" role="status" style={{ color: '#14663A', fontWeight: 800 }}>
              Заявка отправлена! Свяжемся с вами в ближайшее время.
            </p>
          )}
        </form>
      </div>
    </div>
  );
}
