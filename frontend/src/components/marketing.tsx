import { useState } from 'react';
import type { FormEvent, MouseEvent } from 'react';
import { Link } from 'react-router-dom';

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

/** Контакт поддержки — тот же, что в подвале (router.tsx). */
export const SUPPORT_EMAIL = 'hello@edu-platform.ru';
export const SUPPORT_PHONE = '8 (800) 600-78-47';
export const SUPPORT_PHONE_TEL = '+78006007847';
/** Публичный контакт поддержки в Telegram. */
export const SUPPORT_TELEGRAM_URL = 'https://t.me/edu_platform';
export const SUPPORT_TELEGRAM_LABEL = '@edu_platform';

/** Ссылка на каталог с выбранной категорией (фильтр читается из URL). */
export function catalogCategoryHref(slug: string): string {
  return `/?category=${encodeURIComponent(slug)}`;
}

/** Согласованные маршруты: якоря формы заявки и раздела условий возврата. */
export const LEAD_ANCHOR = '/#lead';
export const REFUND_ROUTE = '/#refund';

/**
 * Скролл к якорю, если он есть на текущей странице. Если элемента нет
 * (переход с другой страницы), не мешаем Link: после монтирования каталога
 * якорь обработает эффект в Catalog.tsx.
 */
function scrollToAnchor(e: MouseEvent<HTMLAnchorElement>, href: string): void {
  // Не мешаем открытию в новой вкладке/окне и нестандартным кликам.
  if (e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
  const id = href.startsWith('/#') ? href.slice(2) : '';
  if (!id) return;
  const target = document.getElementById(id);
  if (!target) return;
  e.preventDefault();
  target.scrollIntoView({ behavior: 'smooth', block: 'start' });
}

type SupportKey = 'curators' | 'career' | 'practice' | 'community';

const SUPPORT_TABS: Array<{ key: SupportKey; label: string }> = [
  { key: 'curators', label: 'Опытные кураторы' },
  { key: 'career', label: 'Центр карьеры' },
  { key: 'practice', label: 'Практика и проекты' },
  { key: 'community', label: 'Сообщество' },
];

export const SUPPORT_TEXT: Record<
  SupportKey,
  { title: string; text: string; btn: string; to: string }
> = {
  curators: {
    title: 'Даём подробную обратную связь',
    text: 'Каждую вашу работу проверит куратор-эксперт. Это практикующий специалист с опытом от 5 лет. Он расскажет, что улучшить, и объяснит сложное простыми словами.',
    btn: 'Примеры обратной связи',
    to: catalogCategoryHref('programming'),
  },
  career: {
    title: 'Доводим до оффера',
    text: 'Центр карьеры поможет с резюме, портфолио и собеседованиями. Готовим к техническим интервью и сопровождаем до выхода на работу.',
    btn: 'Как мы трудоустраиваем',
    to: LEAD_ANCHOR,
  },
  practice: {
    title: 'Учим на реальных проектах',
    text: 'Каждый модуль заканчивается проектом в портфолио: парсер, лендинг, дашборд. Код-ревью обязателен — как в настоящей команде.',
    btn: 'Проекты студентов',
    to: catalogCategoryHref('design'),
  },
  community: {
    title: 'Вы не одни',
    text: 'Чаты потоков, живые эфиры по четвергам и наставник, который отвечает в течение дня. Новички поддерживают друг друга.',
    btn: 'Познакомиться',
    to: LEAD_ANCHOR,
  },
};

function CheckGreen(): JSX.Element {
  return (
    <svg width="11" height="11" viewBox="0 0 24 24" aria-hidden="true">
      <path
        d="M4 12.5l5 5L20 6.5"
        fill="none"
        stroke="#14663A"
        strokeWidth="3"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function CheckLight(): JSX.Element {
  return (
    <svg width="11" height="11" viewBox="0 0 24 24" aria-hidden="true">
      <path
        d="M4 12.5l5 5L20 6.5"
        fill="none"
        stroke="#7EE2A8"
        strokeWidth="3"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function StarSmall(): JSX.Element {
  return (
    <svg width="12" height="12" viewBox="0 0 24 24" aria-hidden="true">
      <path
        fill="currentColor"
        d="M12 2.6l2.9 5.9 6.5.9-4.7 4.6 1.1 6.5L12 17.4l-5.8 3.1 1.1-6.5L2.6 9.4l6.5-.9z"
      />
    </svg>
  );
}

export function SupportBlock(): JSX.Element {
  const [tab, setTab] = useState<SupportKey>('curators');
  const s = SUPPORT_TEXT[tab];
  return (
    <div className="support">
      <h2>Создаём опору на всём пути</h2>
      <div className="support-card">
        {/* Переключатель разделов — не вкладки: tabpanel/стрелок нет, поэтому
            используем aria-pressed, а не role=tab (иначе скринридер ждёт tabpanel). */}
        <div className="seg" role="group" aria-label="Разделы поддержки">
          {SUPPORT_TABS.map((t) => (
            <button
              key={t.key}
              type="button"
              aria-pressed={tab === t.key}
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
            <Link className="btn" to={s.to} onClick={(e) => scrollToAnchor(e, s.to)}>
              {s.btn}
            </Link>
          </div>
          <div className="support-visual" aria-hidden="true">
            <div className={`svis svis-curators${tab === 'curators' ? ' active' : ''}`}>
              <div className="sv-card sv-dark" style={{ left: 26, top: 26, width: 264 }}>
                <div className="sv-row" style={{ marginBottom: 10 }}>
                  <span style={{ display: 'flex', gap: 6 }}>
                    <i style={{ width: 10, height: 10, borderRadius: '50%', background: '#FF5F57' }} />
                    <i style={{ width: 10, height: 10, borderRadius: '50%', background: '#FEBC2E' }} />
                    <i style={{ width: 10, height: 10, borderRadius: '50%', background: '#28C840' }} />
                  </span>
                  <span className="sv-small" style={{ margin: '0 0 0 10px' }}>
                    review.py
                  </span>
                </div>
                <div className="sv-code">
                  <span className="sv-dim">12&nbsp;&nbsp;if user.is_ready():</span>
                  <br />
                  <span className="sv-add">13 +&nbsp;&nbsp;&nbsp;&nbsp;ship_lesson(user)</span>
                  <br />
                  <span className="sv-del">13 -&nbsp;&nbsp;&nbsp;&nbsp;pass&nbsp;&nbsp;# заглушка</span>
                </div>
                <div style={{ marginTop: 12 }}>
                  <span className="sv-pill sv-ok">
                    <CheckGreen />
                    Принято
                  </span>
                </div>
              </div>
              <div
                className="sv-ava"
                style={{
                  right: 36,
                  top: 30,
                  width: 76,
                  height: 76,
                  background: 'linear-gradient(140deg,#8A6FA8,#4A3566)',
                  fontSize: 22,
                }}
              >
                АШ
              </div>
              <div className="sv-card" style={{ right: 24, top: 122, maxWidth: 232 }}>
                <p className="sv-title">Айдар Ш. · куратор, 6 лет в Python</p>
                <p className="sv-small">
                  «Отличный разбор ветвлений. Дальше — циклы, заодно докрутим твой парсер».
                </p>
              </div>
              <div className="sv-card" style={{ left: 26, bottom: 26 }}>
                <span className="sv-pill" style={{ background: '#141414', color: '#fff' }}>
                  <StarSmall />
                  4,9 · 212 ревью за месяц
                </span>
                <p className="sv-small">Отвечает в течение дня</p>
              </div>
            </div>
            <div className={`svis svis-career${tab === 'career' ? ' active' : ''}`}>
              <div className="sv-card" style={{ left: 26, top: 28, width: 272 }}>
                <span className="sv-pill sv-ok">Оффер</span>
                <p className="sv-title" style={{ fontSize: 17, marginTop: 10 }}>
                  Junior Python-разработчик
                </p>
                <p className="sv-small">120 000 ₽ · удалённо · финтех</p>
                <div style={{ marginTop: 12 }}>
                  <span className="sv-pill" style={{ background: 'var(--brand)', color: '#fff' }}>
                    Принять оффер
                  </span>
                </div>
              </div>
              <div
                className="sv-ava"
                style={{ right: 40, top: 32, width: 64, height: 64, background: '#141414', fontSize: 12 }}
              >
                HR
              </div>
              <div className="sv-card" style={{ right: 24, bottom: 26, maxWidth: 252 }}>
                <p className="sv-title">Путь до оффера</p>
                <p className="sv-small">Резюме → 12 откликов → 4 интервью</p>
                <div className="sv-bar">
                  <i style={{ width: '82%' }} />
                </div>
                <p className="sv-small">Средний срок — 3 недели после защиты</p>
              </div>
              <div className="sv-card" style={{ left: 26, bottom: 26 }}>
                <p className="sv-title" style={{ fontSize: 22 }}>
                  +38%
                </p>
                <p className="sv-small">к зарплате выпускников</p>
              </div>
            </div>
            <div className={`svis svis-practice${tab === 'practice' ? ' active' : ''}`}>
              <div className="sv-card sv-dark" style={{ left: 26, top: 26, width: 272 }}>
                <p className="sv-title" style={{ color: '#fff' }}>
                  $ pytest tests/
                </p>
                <div className="sv-code" style={{ marginTop: 8 }}>
                  <span className="sv-add">
                    <CheckLight /> test_parser PASSED
                  </span>
                  <br />
                  <span className="sv-add">
                    <CheckLight /> test_quiz PASSED
                  </span>
                  <br />
                  <span className="sv-add">
                    <CheckLight /> test_deploy PASSED
                  </span>
                  <br />
                  <span className="sv-dim">3 passed in 0.42s</span>
                </div>
              </div>
              <div className="sv-card" style={{ right: 24, top: 28, maxWidth: 196 }}>
                <span className="sv-pill" style={{ background: '#141414', color: '#fff' }}>
                  3 проекта
                </span>
                <p className="sv-small">Парсер, лендинг и дашборд — в портфолио на GitHub</p>
              </div>
              <div
                className="sv-card sv-dark"
                style={{ left: 26, bottom: 26, right: 24, maxWidth: 'none' }}
              >
                {['main.py', 'quiz.py', 'README.md'].map((f) => (
                  <div className="sv-file" key={f}>
                    <span className="sv-check">
                      <CheckGreen />
                    </span>
                    {f}
                  </div>
                ))}
              </div>
            </div>
            <div className={`svis svis-community${tab === 'community' ? ' active' : ''}`}>
              <div
                style={{
                  position: 'absolute',
                  left: 26,
                  top: 30,
                  display: 'grid',
                  gap: 10,
                  justifyItems: 'start',
                }}
              >
                <div className="sv-msg in">Кто-нибудь разобрал flex-раскладку?</div>
                <div className="sv-msg out" style={{ justifySelf: 'end' }}>
                  Я! Скину разбор вечером
                </div>
                <div className="sv-msg in">А эфир по портфолио будет?</div>
              </div>
              <div className="sv-card" style={{ right: 24, top: 28, maxWidth: 208 }}>
                <div className="sv-stack">
                  <span style={{ background: '#E63223' }}>МК</span>
                  <span style={{ background: '#1F9D55' }}>ДО</span>
                  <span style={{ background: '#141414' }}>+9</span>
                </div>
                <p className="sv-small">1 240 человек в чате потока</p>
              </div>
              <div className="sv-card" style={{ right: 24, bottom: 26 }}>
                <span className="sv-pill" style={{ border: '1.5px solid var(--line)' }}>
                  <span className="sv-live-dot" />
                  эфир · чт 19:00
                </span>
                <p className="sv-small">Разбор проектов вживую</p>
              </div>
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
        <b>Вернём деньги, если не найдёте работу</b>
        <p>
          Это зафиксировано юридически. Подробнее в{' '}
          <Link to={REFUND_ROUTE} onClick={(e) => scrollToAnchor(e, REFUND_ROUTE)}>
            Условиях возврата
          </Link>
          .
        </p>
      </div>
    </div>
  );
}

/** Краткие условия возврата — якорь /#refund, на который ведёт PerksBlock. */
export function RefundBlock(): JSX.Element {
  return (
    <section
      id="refund"
      className="panel"
      aria-labelledby="refund-title"
      style={{ marginTop: 24 }}
    >
      <h2 id="refund-title">Условия возврата</h2>
      <p style={{ margin: 0 }}>
        Если курс не подошёл, вернём стоимость в течение 7 дней после оплаты. Напишите на{' '}
        <a href={`mailto:${SUPPORT_EMAIL}`}>{SUPPORT_EMAIL}</a> или позвоните{' '}
        <a href={`tel:${SUPPORT_PHONE_TEL}`}>{SUPPORT_PHONE}</a> — подскажем, как оформить
        возврат.
      </p>
    </section>
  );
}

export const DREAMS: Array<{ cls: string; hot?: boolean; n: string; text: string; to: string }> = [
  {
    cls: 'dream-1',
    n: '12 курсов',
    text: 'Ни от кого не зависеть и работать на себя',
    to: catalogCategoryHref('programming'),
  },
  {
    cls: 'dream-2',
    n: '22 курса',
    text: 'Стартовать в IT без программирования',
    to: catalogCategoryHref('design'),
  },
  {
    cls: 'dream-3',
    hot: true,
    n: '27 курсов',
    text: 'Работать на фрилансе из любой точки мира',
    to: catalogCategoryHref('marketing'),
  },
  {
    cls: 'dream-4',
    n: '5 курсов',
    text: 'Зарабатывать на креативном контенте',
    to: catalogCategoryHref('design'),
  },
  {
    cls: 'dream-5',
    n: '5 курсов',
    text: 'Научиться грамотно инвестировать и тратить',
    to: catalogCategoryHref('data'),
  },
];

export function DreamsBlock(): JSX.Element {
  return (
    <div className="dreams">
      <h2>Сделайте первый шаг к жизни вашей мечты</h2>
      <div className="dreams-row">
        {DREAMS.map((d) => (
          <Link key={d.text} className={`dream ${d.cls}${d.hot ? ' hot' : ''}`} to={d.to}>
            <span className="n">{d.n}</span>
            <p>{d.text}</p>
          </Link>
        ))}
      </div>
    </div>
  );
}

export function DiplomaBlock(): JSX.Element {
  return (
    <div className="diploma">
      <div>
        <h2>Выдаём дипломы и&nbsp;сертификаты</h2>
        <p>
          После окончания курса вы получите диплом о&nbsp;профессиональной переподготовке или
          сертификат установленного образца — тип документа можно уточнить у&nbsp;консультанта.
          Мы обучаем по&nbsp;государственной лицензии — её номер подскажет{' '}
          <a href="#lead">консультант</a>.
        </p>
      </div>
      <div className="diploma-visual" aria-hidden="true">
        <div className="doc doc-dipl">
          <h4>Диплом · переподготовка</h4>
          <div className="doc-name" style={{ fontSize: 15 }}>
            Python-разработчик
          </div>
          <div className="doc-line" style={{ width: '92%' }} />
          <div className="doc-line" style={{ width: '78%' }} />
          <div className="doc-line" style={{ width: '85%' }} />
          <div className="doc-line" style={{ width: '60%' }} />
          <div className="doc-seal">2026</div>
        </div>
        <div className="doc doc-cert">
          <h4>Сертификат</h4>
          <div className="doc-name">Иван Петров</div>
          <div className="doc-line" style={{ width: '70%' }} />
          <p className="doc-prof">
            Профессия
            <br />
            Python-разработчик
          </p>
          <div className="doc-sign">подпись</div>
          <div className="doc-seal">EP</div>
        </div>
      </div>
    </div>
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
      <p>Подберём курс под вашу цель — от первой профессии до фриланса.</p>
      <button type="button" className="btn btn-blue" onClick={onChoose}>
        Выбрать курс
      </button>
    </div>
  );
}

export type ContactKey = 'phone' | 'telegram' | 'vk' | 'max';

const CONTACTS: Array<{ key: ContactKey; label: string; bg: string; glyph: string }> = [
  { key: 'phone', label: 'По телефону', bg: '#1F9D55', glyph: 'Т' },
  { key: 'telegram', label: 'В Telegram', bg: '#229ED9', glyph: 'Т' },
  { key: 'vk', label: 'Во VK', bg: '#0077FF', glyph: 'VK' },
  { key: 'max', label: 'В MAX', bg: '#7B5CFF', glyph: 'M' },
];

const CONTACT_PLACEHOLDER: Record<ContactKey, string> = {
  phone: 'Телефон, например +7 999 123-45-67',
  telegram: 'Telegram: @username',
  vk: 'VK: ссылка или @ник',
  max: 'MAX: номер или @ник',
};

/** Состояние формы заявки. Состав полей совпадает с будущим POST /leads. */
export interface LeadFormData {
  name: string;
  contact: string;
  email: string;
  method: ContactKey;
  consent: boolean;
}

export interface LeadFieldErrors {
  name?: string;
  contact?: string;
  email?: string;
  consent?: string;
}

const EMPTY_LEAD: LeadFormData = {
  name: '',
  contact: '',
  email: '',
  method: 'phone',
  consent: false,
};

/** Подпись способа связи для письма. */
function contactLabel(method: ContactKey): string {
  return CONTACTS.find((c) => c.key === method)?.label ?? 'Связаться';
}

/**
 * Проверка формы перед отправкой. Email необязателен, но если указан —
 * должен быть похож на адрес; телефон проверяем по количеству цифр.
 */
export function validateLead(data: LeadFormData): LeadFieldErrors {
  const errors: LeadFieldErrors = {};

  const name = data.name.trim();
  if (!name) errors.name = 'Укажите имя';
  else if (name.length < 2) errors.name = 'Имя должно содержать хотя бы 2 символа';

  const contact = data.contact.trim();
  if (!contact) {
    errors.contact =
      data.method === 'phone' ? 'Укажите номер телефона' : 'Укажите контакт для связи';
  } else if (data.method === 'phone') {
    const digits = contact.replace(/\D/g, '');
    if (digits.length < 10 || digits.length > 15) {
      errors.contact = 'Проверьте номер: нужно 10–15 цифр';
    }
  } else if (contact.length < 2) {
    errors.contact = 'Укажите контакт для связи';
  }

  const email = data.email.trim();
  if (email && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
    errors.email = 'Проверьте адрес электронной почты';
  }

  if (!data.consent) errors.consent = 'Нужно согласие на обработку персональных данных';

  return errors;
}

/** Текст заявки — общий для mailto и ручного копирования. */
export function buildLeadMessage(data: LeadFormData): string {
  const label = contactLabel(data.method);
  const lines = [
    `Имя: ${data.name.trim()}`,
    `Контакт (${label}): ${data.contact.trim()}`,
  ];
  const email = data.email.trim();
  if (email) lines.push(`Email: ${email}`);
  lines.push(`Предпочтительный способ связи: ${label}`);
  lines.push('Согласие на обработку персональных данных: да');
  return lines.join('\n');
}

/** Текст для кнопки «Скопировать»: заявка + адрес, куда её отправить. */
export function buildLeadCopyText(data: LeadFormData): string {
  return `${buildLeadMessage(data)}\n\nОтправить на: ${SUPPORT_EMAIL}`;
}

/**
 * mailto-ссылка с темой и телом заявки — резервный канал, если вебхук
 * не настроен или недоступен.
 */
export function buildLeadMailto(data: LeadFormData): string {
  const subject = `Заявка на консультацию — ${data.name.trim()}`;
  const body = buildLeadMessage(data);
  return `mailto:${SUPPORT_EMAIL}?subject=${encodeURIComponent(subject)}&body=${encodeURIComponent(body)}`;
}

/**
 * Адрес вебхука заявок из env. Пустая строка — онлайн-отправка выключена.
 * import.meta.env может отсутствовать вне Vite (например, в Node-тестах),
 * поэтому читаем его через безопасный cast и проверку на undefined.
 */
export const LEADS_WEBHOOK_URL: string = (
  (import.meta.env as Record<string, string | undefined> | undefined)?.VITE_LEADS_WEBHOOK_URL ??
  ''
).trim();

/** Тело POST на вебхук: только данные формы, без служебных полей. */
export function buildLeadPayload(data: LeadFormData): LeadFormData {
  return {
    name: data.name.trim(),
    contact: data.contact.trim(),
    email: data.email.trim(),
    method: data.method,
    consent: data.consent,
  };
}

/**
 * Отправка заявки на внешний вебхук. Успех — только HTTP 2xx; любой другой
 * ответ или сетевая ошибка пробрасываются, чтобы форма показала retry.
 */
export async function sendLead(
  url: string,
  data: LeadFormData,
  fetchImpl: typeof fetch = fetch,
): Promise<void> {
  const res = await fetchImpl(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Accept: 'application/json',
    },
    body: JSON.stringify(buildLeadPayload(data)),
  });
  if (!res.ok) {
    throw new Error(`Вебхук заявок ответил ${res.status}`);
  }
}

type LeadStatus = 'idle' | 'sending' | 'sent' | 'error' | 'unavailable';

export function LeadBlock(): JSX.Element {
  const [form, setForm] = useState<LeadFormData>(EMPTY_LEAD);
  const [errors, setErrors] = useState<LeadFieldErrors>({});
  const [status, setStatus] = useState<LeadStatus>('idle');
  const [copied, setCopied] = useState(false);
  const [errorText, setErrorText] = useState('');

  /** Обновление поля + сброс прежнего подтверждения/ошибки этого поля. */
  function updateField<K extends keyof LeadFormData>(field: K, value: LeadFormData[K]): void {
    setForm((prev) => ({ ...prev, [field]: value }));
    setStatus((prev) => (prev === 'sent' || prev === 'error' ? 'idle' : prev));
    setCopied(false);
    setErrorText('');
    setErrors((prev) => {
      const next = { ...prev };
      if (field === 'method') delete next.contact;
      else delete next[field as keyof LeadFieldErrors];
      return next;
    });
  }

  async function submitLead(): Promise<void> {
    if (status === 'sending') return;

    const nextErrors = validateLead(form);
    setErrors(nextErrors);
    setCopied(false);
    setErrorText('');

    const firstError = nextErrors.name
      ? 'lead-name'
      : nextErrors.contact
        ? 'lead-contact'
        : nextErrors.email
          ? 'lead-email'
          : nextErrors.consent
            ? 'lead-consent'
            : null;
    if (firstError) {
      setStatus('idle');
      document.getElementById(firstError)?.focus();
      return;
    }

    // Без вебхука не изображаем успех: показываем контакты и сохраняем данные.
    if (!LEADS_WEBHOOK_URL) {
      setStatus('unavailable');
      return;
    }

    setStatus('sending');
    try {
      await sendLead(LEADS_WEBHOOK_URL, form);
      setStatus('sent');
    } catch {
      setStatus('error');
      setErrorText(
        'Не удалось отправить заявку. Проверьте соединение и попробуйте ещё раз — данные остались в форме.',
      );
    }
  }

  function onSubmit(e: FormEvent<HTMLFormElement>): void {
    e.preventDefault();
    void submitLead();
  }

  async function copyLeadText(): Promise<void> {
    try {
      if (!navigator.clipboard?.writeText) throw new Error('clipboard unavailable');
      await navigator.clipboard.writeText(buildLeadCopyText(form));
      setCopied(true);
      setErrorText('');
    } catch {
      setCopied(false);
      setErrorText(
        `Не удалось скопировать автоматически. Напишите на ${SUPPORT_EMAIL} или позвоните ${SUPPORT_PHONE}.`,
      );
    }
  }

  return (
    <div className="lead-zone" id="lead">
      <div className="lead-card">
        <div>
          <h2>Поможем подобрать онлайн-курс под ваши цели</h2>
          <p className="lead-sub">
            Оставьте заявку, получите консультацию и подборку курсов, которые подойдут именно
            вам: от программирования до дизайна и аналитики.
          </p>
          <ul className="lead-list">
            <li>
              <CheckIcon />
              <span>Проведём бесплатную карьерную консультацию</span>
            </li>
            <li>
              <CheckIcon />
              <span>Расскажем об условиях обучения и помощи с трудоустройством</span>
            </li>
            <li>
              <CheckIcon />
              <span>Поможем выбрать программу с дипломом или сертификатом</span>
            </li>
            <li>
              <CheckIcon />
              <span>Покажем кейсы и отзывы наших студентов</span>
            </li>
          </ul>
        </div>
        <form className="lead-form" onSubmit={onSubmit} noValidate>
          <input
            id="lead-name"
            name="name"
            className="input"
            required
            value={form.name}
            onChange={(e) => updateField('name', e.target.value)}
            placeholder="Имя"
            autoComplete="name"
            aria-label="Имя"
            aria-invalid={errors.name ? true : undefined}
            aria-describedby={errors.name ? 'lead-name-error' : undefined}
          />
          {errors.name && (
            <p className="error" id="lead-name-error">
              {errors.name}
            </p>
          )}

          <input
            id="lead-contact"
            name="contact"
            className="input"
            required
            value={form.contact}
            onChange={(e) => updateField('contact', e.target.value)}
            placeholder={CONTACT_PLACEHOLDER[form.method]}
            inputMode={form.method === 'phone' ? 'tel' : 'text'}
            autoComplete={form.method === 'phone' ? 'tel' : 'off'}
            aria-label={form.method === 'phone' ? 'Телефон' : 'Контакт для связи'}
            aria-invalid={errors.contact ? true : undefined}
            aria-describedby={errors.contact ? 'lead-contact-error' : undefined}
          />
          {errors.contact && (
            <p className="error" id="lead-contact-error">
              {errors.contact}
            </p>
          )}

          <input
            id="lead-email"
            name="email"
            className="input"
            type="email"
            value={form.email}
            onChange={(e) => updateField('email', e.target.value)}
            placeholder="Электронная почта (необязательно)"
            autoComplete="email"
            aria-label="Электронная почта"
            aria-invalid={errors.email ? true : undefined}
            aria-describedby={errors.email ? 'lead-email-error' : undefined}
          />
          {errors.email && (
            <p className="error" id="lead-email-error">
              {errors.email}
            </p>
          )}

          <p className="sv-title">Как с вами связаться?</p>
          <div className="contact-grid">
            {CONTACTS.map((c) => (
              <label className="contact" key={c.key}>
                <input
                  type="radio"
                  name="contactMethod"
                  value={c.key}
                  checked={form.method === c.key}
                  onChange={() => updateField('method', c.key)}
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
              id="lead-consent"
              name="consent"
              type="checkbox"
              required
              checked={form.consent}
              onChange={(e) => updateField('consent', e.target.checked)}
              aria-invalid={errors.consent ? true : undefined}
              aria-describedby={errors.consent ? 'lead-consent-error' : undefined}
            />
            <span>Я соглашаюсь на обработку персональных данных</span>
          </label>
          {errors.consent && (
            <p className="error" id="lead-consent-error">
              {errors.consent}
            </p>
          )}

          {!LEADS_WEBHOOK_URL && status !== 'unavailable' && (
            <p className="notice" style={{ margin: 0 }}>
              Онлайн-форма временно недоступна. Позвоните{' '}
              <a href={`tel:${SUPPORT_PHONE_TEL}`}>{SUPPORT_PHONE}</a> или напишите в{' '}
              <a href={SUPPORT_TELEGRAM_URL} target="_blank" rel="noreferrer">
                Telegram
              </a>{' '}
              — поможем с подбором курса.
            </p>
          )}

          <button
            className="btn btn-primary"
            type="submit"
            disabled={status === 'sending' || status === 'sent'}
          >
            {status === 'sending'
              ? 'Отправляем…'
              : status === 'sent'
                ? 'Заявка отправлена'
                : status === 'error'
                  ? 'Повторить отправку'
                  : 'Отправить заявку'}
          </button>

          {status === 'sent' && (
            <p style={{ color: '#14663A', fontWeight: 800, margin: 0 }} role="status">
              Заявка отправлена. Перезвоним в течение дня.
            </p>
          )}

          {status === 'error' && (
            <p className="error" style={{ margin: 0 }} role="alert">
              {errorText}
            </p>
          )}

          {status === 'unavailable' && (
            <div style={{ display: 'grid', gap: 8, marginTop: 4 }}>
              <p className="error" style={{ margin: 0 }} role="alert">
                Онлайн-форма временно недоступна. Данные не потеряны — их можно скопировать и
                отправить нам.
              </p>
              <p className="sv-small" style={{ margin: 0 }}>
                Позвоните <a href={`tel:${SUPPORT_PHONE_TEL}`}>{SUPPORT_PHONE}</a>, напишите в{' '}
                <a href={SUPPORT_TELEGRAM_URL} target="_blank" rel="noreferrer">
                  Telegram ({SUPPORT_TELEGRAM_LABEL})
                </a>{' '}
                или на <a href={`mailto:${SUPPORT_EMAIL}`}>{SUPPORT_EMAIL}</a>.
              </p>
              <textarea
                className="input"
                readOnly
                rows={4}
                value={buildLeadCopyText(form)}
                aria-label="Текст заявки для копирования"
              />
              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                <button
                  type="button"
                  className="btn btn-sm"
                  onClick={() => void copyLeadText()}
                >
                  {copied ? 'Текст заявки скопирован' : 'Скопировать текст заявки'}
                </button>
                <a className="btn btn-sm" href={buildLeadMailto(form)}>
                  Отправить по почте
                </a>
              </div>
              {copied && (
                <p className="sv-small" style={{ margin: 0 }} role="status">
                  Вставьте скопированный текст в письмо или мессенджер.
                </p>
              )}
              {errorText && (
                <p className="error" style={{ margin: 0 }} role="alert">
                  {errorText}
                </p>
              )}
            </div>
          )}
        </form>
      </div>
    </div>
  );
}
