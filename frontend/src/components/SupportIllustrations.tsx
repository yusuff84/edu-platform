/**
 * Иллюстрации блока «Как устроено обучение» — в стиле иконок преимуществ
 * (календарь, бесконечность, щит): плоские фигуры, палитра сайта, мягкая тень.
 */
type Kind = 'lessons' | 'tests' | 'questions' | 'certificate';

const Shadow = (): JSX.Element => <ellipse cx="60" cy="91" rx="36" ry="6" fill="#141414" opacity=".12" />;

const Check = ({ cx, cy }: { cx: number; cy: number }): JSX.Element => (
  <>
    <circle cx={cx} cy={cy} r="13" fill="#1F9D55" stroke="#fff" strokeWidth="3" />
    <path d={`M${cx - 6.5} ${cy}l4.5 4.5 9-9`} stroke="#fff" strokeWidth="3" fill="none" strokeLinecap="round" strokeLinejoin="round" />
  </>
);

export function SupportIllustration({ kind }: { kind: Kind }): JSX.Element {
  return (
    <svg className="svis-illu" width="120" height="100" viewBox="0 0 120 100" aria-hidden="true">
      <Shadow />
      {kind === 'lessons' && (
        <>
          {/* Карточка урока: синий экран с видео сверху, конспект снизу, галочка «прогресс сохранён». */}
          <rect x="24" y="12" width="72" height="68" rx="11" fill="#fff" stroke="#E2DDD1" strokeWidth="2" />
          <path d="M24 23a11 11 0 0 1 11-11h50a11 11 0 0 1 11 11v26H24z" fill="#3B5BFF" />
          <circle cx="60" cy="31" r="10.5" fill="#fff" />
          <path d="M57 25.5v11l9-5.5z" fill="#E63223" />
          <rect x="32" y="43" width="56" height="3" rx="1.5" fill="#fff" opacity=".35" />
          <rect x="32" y="43" width="26" height="3" rx="1.5" fill="#FEC466" />
          <rect x="32" y="56" width="40" height="4" rx="2" fill="#ECEAE4" />
          <rect x="32" y="64" width="28" height="4" rx="2" fill="#ECEAE4" />
          <Check cx={89} cy={73} />
        </>
      )}
      {kind === 'tests' && (
        <>
          <rect x="28" y="14" width="60" height="72" rx="10" fill="#3B5BFF" />
          <rect x="34" y="24" width="48" height="56" rx="6" fill="#fff" />
          <rect x="47" y="8" width="22" height="13" rx="4" fill="#141414" />
          <rect x="41" y="33" width="8" height="8" rx="2.5" fill="#1F9D55" />
          <rect x="53" y="35" width="22" height="4" rx="2" fill="#ECEAE4" />
          <rect x="41" y="47" width="8" height="8" rx="2.5" fill="#1F9D55" />
          <rect x="53" y="49" width="22" height="4" rx="2" fill="#ECEAE4" />
          <rect x="41" y="61" width="8" height="8" rx="2.5" fill="#ECEAE4" />
          <rect x="53" y="63" width="16" height="4" rx="2" fill="#ECEAE4" />
          <Check cx={84} cy={72} />
        </>
      )}
      {kind === 'questions' && (
        <>
          <path d="M22 14h48a12 12 0 0 1 12 12v18a12 12 0 0 1-12 12H40l-12 10v-10h-6a12 12 0 0 1-12-12V26a12 12 0 0 1 12-12Z" fill="#3B5BFF" />
          <text x="46" y="45" textAnchor="middle" fontSize="28" fontWeight="800" fill="#fff" fontFamily="inherit">?</text>
          <path d="M58 46h40a10 10 0 0 1 10 10v14a10 10 0 0 1-10 10h-4v9l-11-9H58a10 10 0 0 1-10-10V56a10 10 0 0 1 10-10Z" fill="#fff" stroke="#E2DDD1" strokeWidth="2" strokeLinejoin="round" />
          <circle cx="66" cy="63" r="3.5" fill="#141414" />
          <circle cx="78" cy="63" r="3.5" fill="#141414" />
          <circle cx="90" cy="63" r="3.5" fill="#141414" />
          <path d="M98 14l2.2 5.3 5.3 2.2-5.3 2.2L98 29l-2.2-5.3-5.3-2.2 5.3-2.2z" fill="#FEC466" />
        </>
      )}
      {kind === 'certificate' && (
        <>
          <rect x="12" y="16" width="80" height="58" rx="8" fill="#fff" stroke="#E2DDD1" strokeWidth="2" />
          <rect x="17" y="21" width="70" height="48" rx="5" fill="none" stroke="#3B5BFF" strokeWidth="2" />
          <rect x="30" y="31" width="44" height="5" rx="2.5" fill="#3B5BFF" />
          <rect x="26" y="42" width="52" height="4" rx="2" fill="#ECEAE4" />
          <rect x="32" y="50" width="40" height="4" rx="2" fill="#ECEAE4" />
          {/* медаль */}
          <path d="M80 70l-6 18 8-4 5 7 4-17z" fill="#E63223" />
          <path d="M96 70l6 18-8-4-5 7-4-17z" fill="#B71F14" />
          <circle cx="88" cy="64" r="15" fill="#FEC466" stroke="#fff" strokeWidth="3" />
          <path d="M88 55.5l2.6 5.3 5.8.8-4.2 4.1 1 5.8-5.2-2.7-5.2 2.7 1-5.8-4.2-4.1 5.8-.8z" fill="#fff" />
        </>
      )}
    </svg>
  );
}
