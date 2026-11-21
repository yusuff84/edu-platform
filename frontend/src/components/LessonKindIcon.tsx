/** Иконка типа урока в карточках программы: практикум — планшет с галочкой, текст — документ. */
export function LessonKindIcon({ kind }: { kind: 'quiz' | 'text' }): JSX.Element {
  if (kind === 'quiz') {
    return (
      <svg className="lesson-kind-icon" viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <rect x="5" y="4" width="14" height="17" rx="2.5" stroke="currentColor" strokeWidth="1.8" />
        <path d="M9 4.5V3.8A1.3 1.3 0 0 1 10.3 2.5h3.4A1.3 1.3 0 0 1 15 3.8v.7c0 .55-.45 1-1 1h-4c-.55 0-1-.45-1-1Z" fill="currentColor" />
        <path d="m8.8 13.2 2.2 2.2 4.4-4.6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    );
  }
  return (
    <svg className="lesson-kind-icon" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path d="M7 2.8h7.2L19 7.6V19a2.2 2.2 0 0 1-2.2 2.2H7A2.2 2.2 0 0 1 4.8 19V5A2.2 2.2 0 0 1 7 2.8Z" stroke="currentColor" strokeWidth="1.8" strokeLinejoin="round" />
      <path d="M14 3v4.6h4.8" stroke="currentColor" strokeWidth="1.8" strokeLinejoin="round" />
      <path d="M8.5 12h7M8.5 15.2h7M8.5 18.4h4.5" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
    </svg>
  );
}
