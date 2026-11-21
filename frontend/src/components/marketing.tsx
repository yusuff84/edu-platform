import { useState } from 'react';
import type { FormEvent } from 'react';
import { useMutation } from '@tanstack/react-query';
import { createLead } from '../api/leads';
import type { LeadContactMethod } from '../api/leads';
import { CertificateDocument } from './CertificateDocument';

function CheckIcon(): JSX.Element {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true">
      <circle cx="12" cy="12" r="10" fill="#141414" />
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
              <span className="svis-simple-icon">{s.icon}</span>
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
            <ellipse cx="60" cy="90" rx="34" ry="6" fill="#141414" opacity=".12" />
            <rect x="28" y="14" width="64" height="64" rx="10" fill="#fff" />
            <path d="M28 24a10 10 0 0 1 10-10h44a10 10 0 0 1 10 10v10H28z" fill="#E63223" />
            <rect x="40" y="8" width="6" height="15" rx="3" fill="#141414" />
            <rect x="74" y="8" width="6" height="15" rx="3" fill="#141414" />
            <rect x="37" y="44" width="13" height="13" rx="3.5" fill="#ECEAE4" />
            <rect x="53.5" y="44" width="13" height="13" rx="3.5" fill="#ECEAE4" />
            <rect x="70" y="44" width="13" height="13" rx="3.5" fill="#3B5BFF" />
            <rect x="37" y="60" width="13" height="13" rx="3.5" fill="#FEC466" />
            <circle cx="68" cy="68" r="13" fill="#1F9D55" stroke="#fff" strokeWidth="3" />
            <path
              d="M61.5 68l4.5 4.5 9-9"
              stroke="#fff"
              strokeWidth="3"
              fill="none"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
        </div>
        <b>Свободный график без дедлайнов</b>
        <p>Обучение легко совмещать с работой и личной жизнью</p>
      </div>
      <div className="perk perk-2">
        <div className="perk-ico" aria-hidden="true">
          <svg width="136" height="76" viewBox="0 0 136 76">
            <path
              d="M32 46c-9-9-9-23 0-30 8-6 17-5 25 2l11 10 11-10c8-7 17-8 25-2 9 7 9 21 0 30-8 7-17 6-25-1l-11-9-11 9c-8 7-17 8-25 1z"
              fill="none"
              stroke="#141414"
              strokeWidth="13"
              strokeLinecap="round"
            />
            <path
              d="M68 36l11-8c8-7 17-8 25-2 9 7 9 21 0 30-8 7-17 6-25-1l-11-9"
              fill="none"
              stroke="#3B5BFF"
              strokeWidth="13"
              strokeLinecap="round"
            />
          </svg>
        </div>
        <b>Бессрочный доступ к курсу и обновлениям</b>
        <p>Можно вернуться к занятиям и освежить знания в любой момент</p>
      </div>
      <div className="perk dark">
        <div className="perk-ico" aria-hidden="true">
          <svg width="128" height="100" viewBox="0 0 128 100">
            <ellipse cx="64" cy="91" rx="38" ry="5" fill="#fff" opacity=".14" />
            <ellipse cx="88" cy="52" rx="21" ry="7.5" fill="#FEC466" />
            <path d="M67 52v22c0 4.1 9.4 7.5 21 7.5s21-3.4 21-7.5V52" fill="#D89A2B" />
            <path
              d="M67 63c0 4.1 9.4 7.5 21 7.5s21-3.4 21-7.5"
              fill="none"
              stroke="#A86F14"
              strokeWidth="2"
            />
            <ellipse cx="88" cy="52" rx="21" ry="7.5" fill="#FEC466" />
            <ellipse
              cx="88"
              cy="52"
              rx="13.5"
              ry="4.2"
              fill="none"
              stroke="#C9861A"
              strokeWidth="2"
            />
            <path d="M16 28l25-9 25 9v21c0 18-10.5 29-25 36-14.5-7-25-18-25-36z" fill="#3B5BFF" />
            <path
              d="M28 48.5l9.5 9.5L55 40"
              fill="none"
              stroke="#fff"
              strokeWidth="6"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
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
                <span className="cm" style={{ background: c.bg }}>
                  {c.glyph}
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
