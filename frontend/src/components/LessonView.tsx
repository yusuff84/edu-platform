// Просмотр урока: все четыре kind (video_text, live, quiz, code).
// Формы полей строго по контракту Task/00-api-contract.md:
//   - текст урока — bodyMd (markdown-строка контракта);
//   - квиз — quiz.questions[] c полем text, ответы { [questionId]: index варианта };
//   - эфир — liveUrl + startsAt;
//   - код — codeTask.{language, template, tests}, редактор инициализируется шаблоном.
// Вердикт «Зачтено / Не зачтено» показывает родитель (Learn.tsx) по полю passed.
// Оформление — Skillbox-style (panel/pill/btn из styles.css).

import { useId, useState } from 'react';
import type { ReactNode } from 'react';
import type { CompleteLessonInput, Lesson } from '../api/client';
import { formatDate } from '../api/client';
import { t } from '../i18n/t';
import '../styles/learn.css';
import { videoPosterUrl } from '../app/media';

interface Props {
  lesson: Lesson;
  /** Отправить урок на завершение: {} для video/live, quizAnswers/code — для quiz/code. */
  onComplete: (payload: CompleteLessonInput) => void | Promise<void>;
  /** Идёт отправка/проверка на сервере (кнопки заблокированы). */
  completing: boolean;
  /** Урок уже засчитан — вместо кнопки «Отметить пройденным» показываем статус. */
  done?: boolean;
}

/** Кнопка завершения урока (видео/текст/эфир) или статус «пройдено». */
function CompleteAction({ lesson, done, completing, onComplete }: Props): JSX.Element {
  if (done) {
    return (
      <p className="lesson-done-note" role="status">
        <span aria-hidden="true">✓</span> {t('learn.completed')}
      </p>
    );
  }
  return (
    <button
      type="button"
      className="btn btn-primary lesson-complete-btn"
      onClick={() => void onComplete({})}
      disabled={completing}
      aria-label={`${t('learn.complete')}: ${lesson.title}`}
    >
      {completing ? t('learn.completing') : t('learn.complete')}
    </button>
  );
}

// ---------- Безопасный markdown-рендер ----------

type MdBlock =
  | { type: 'p'; lines: string[] }
  | { type: 'ul'; items: string[] }
  | { type: 'ol'; items: string[] }
  | { type: 'code'; text: string }
  | { type: 'quote'; lines: string[] }
  | { type: 'h'; level: number; text: string };

const RE_FENCE = /^\s*```/;
const RE_HEADING = /^(#{1,6})\s+(.*)$/;
const RE_QUOTE = /^\s*>\s?/;
const RE_UL = /^\s*[-*+]\s+/;
const RE_OL = /^\s*\d+[.)]\s+/;
// Начало нового блока — чтобы абзац обрывался на списке/заголовке/цитате/коде.
const RE_BLOCK_START = /^\s*([-*+]\s|\d+[.)]\s|>|#{1,6}\s|```)/;

/** Разбирает markdown на блоки. Поддерживает абзацы, списки, цитаты, заголовки и ```-код. */
function parseBlocks(src: string): MdBlock[] {
  const lines = src.replace(/\r\n?/g, '\n').split('\n');
  const blocks: MdBlock[] = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (line.trim() === '') {
      i++;
      continue;
    }
    if (RE_FENCE.test(line)) {
      // Ограждённый блок кода: собираем до закрывающих «```» (или до конца).
      const text: string[] = [];
      i++;
      while (i < lines.length && !RE_FENCE.test(lines[i])) {
        text.push(lines[i]);
        i++;
      }
      i++; // пропускаем закрывающую «```»
      blocks.push({ type: 'code', text: text.join('\n') });
      continue;
    }
    const heading = RE_HEADING.exec(line);
    if (heading) {
      blocks.push({ type: 'h', level: heading[1].length, text: heading[2] });
      i++;
      continue;
    }
    if (RE_QUOTE.test(line)) {
      const quote: string[] = [];
      while (i < lines.length && RE_QUOTE.test(lines[i])) {
        quote.push(lines[i].replace(RE_QUOTE, ''));
        i++;
      }
      blocks.push({ type: 'quote', lines: quote });
      continue;
    }
    if (RE_UL.test(line)) {
      const items: string[] = [];
      while (i < lines.length && RE_UL.test(lines[i])) {
        items.push(lines[i].replace(RE_UL, ''));
        i++;
      }
      blocks.push({ type: 'ul', items });
      continue;
    }
    if (RE_OL.test(line)) {
      const items: string[] = [];
      while (i < lines.length && RE_OL.test(lines[i])) {
        items.push(lines[i].replace(RE_OL, ''));
        i++;
      }
      blocks.push({ type: 'ol', items });
      continue;
    }
    const para: string[] = [];
    while (i < lines.length && lines[i].trim() !== '' && !RE_BLOCK_START.test(lines[i])) {
      para.push(lines[i].trim());
      i++;
    }
    blocks.push({ type: 'p', lines: para });
  }
  return blocks;
}

// **жирный**, *курсив*, `код` — без интерпретации HTML (текст рендерится как текст).
const RE_INLINE = /(!?\[[^\]\n]*\]\([^\s)]+\)|\*\*\*[^*\n]+\*\*\*|\*\*[^*\n]+\*\*|`[^`]+`|\*(?:[^*\n]|\*\*[^*\n]+\*\*)+\*)/g;

function safeMarkdownUrl(value: string): string | null {
  if (value.startsWith('/') && !value.startsWith('//') && !value.includes('\\')) return value;
  try {
    const url = new URL(value);
    return url.protocol === 'https:' || url.protocol === 'http:' ? url.href : null;
  } catch {
    return null;
  }
}

function renderInline(text: string, keyBase: string): ReactNode[] {
  return text
    .split(RE_INLINE)
    .filter((part) => part !== '')
    .map((part, idx) => {
      const key = `${keyBase}-${idx}`;
      const media = /^(!?)\[([^\]]*)\]\(([^\s)]+)\)$/.exec(part);
      if (media) {
        const url = safeMarkdownUrl(media[3]);
        if (!url) return media[2];
        return media[1]
          ? <img key={key} src={url} alt={media[2]} loading="lazy" style={{ maxWidth: '100%', height: 'auto' }} />
          : <a key={key} href={url} target="_blank" rel="noopener noreferrer">{media[2]}</a>;
      }
      if (part.length > 6 && part.startsWith('***') && part.endsWith('***')) {
        return (
          <strong key={key}>
            <em>{part.slice(3, -3)}</em>
          </strong>
        );
      }
      if (part.length > 4 && part.startsWith('**') && part.endsWith('**') && !part.slice(2, -2).includes('*')) {
        return <strong key={key}>{part.slice(2, -2)}</strong>;
      }
      if (part.length > 2 && part.startsWith('`') && part.endsWith('`')) {
        return (
          <code key={key}>{part.slice(1, -1)}</code>
        );
      }
      if (part.length > 2 && part.startsWith('*') && part.endsWith('*')) {
        // Курсив может содержать **жирный** внутри («***Термин** — определение.*»).
        return <em key={key}>{renderInline(part.slice(1, -1), key)}</em>;
      }
      return part;
    });
}

/**
 * Минимальный безопасный markdown-рендерер: строит React-элементы из текста,
 * HTML не интерпретируется (сырой HTML-вставкой в разметку не пользуемся).
 */
export function Markdown({ source }: { source: string }): JSX.Element {
  const blocks = parseBlocks(source);
  return (
    <div className="md">
      {blocks.map((b, i) => {
        const key = `b${i}`;
        switch (b.type) {
          case 'h': {
            const inline = renderInline(b.text, key);
            if (b.level <= 2) return <h4 key={key}>{inline}</h4>;
            if (b.level === 3) return <h5 key={key}>{inline}</h5>;
            return <h6 key={key}>{inline}</h6>;
          }
          case 'code':
            return (
              <pre key={key} className="md-pre">
                <code>{b.text}</code>
              </pre>
            );
          case 'quote':
            return (
              <blockquote key={key} className="md-quote">
                {b.lines.map((ln, j) => (
                  <p key={j}>{renderInline(ln, `${key}-${j}`)}</p>
                ))}
              </blockquote>
            );
          case 'ul':
            return (
              <ul key={key}>
                {b.items.map((it, j) => (
                  <li key={j}>{renderInline(it, `${key}-${j}`)}</li>
                ))}
              </ul>
            );
          case 'ol':
            return (
              <ol key={key}>
                {b.items.map((it, j) => (
                  <li key={j}>{renderInline(it, `${key}-${j}`)}</li>
                ))}
              </ol>
            );
          default:
            return (
              <p key={key}>
                {b.lines.map((ln, j) => (
                  <span key={j}>
                    {j > 0 ? ' ' : ''}
                    {renderInline(ln, `${key}-${j}`)}
                  </span>
                ))}
              </p>
            );
        }
      })}
    </div>
  );
}

// ---------- Квиз ----------

export function QuizForm({
  lesson,
  onSubmit,
  disabled,
}: {
  lesson: Lesson;
  onSubmit: (answers: Record<string, number>) => void;
  disabled: boolean;
}): JSX.Element {
  const questions = lesson.quiz?.questions ?? [];
  const [answers, setAnswers] = useState<Record<string, number>>({});
  const [current, setCurrent] = useState(0);
  const allAnswered =
    questions.length > 0 && questions.every((q) => answers[q.id] !== undefined);
  const question = questions[current];

  return (
    <form
      className="quiz"
      onSubmit={(e) => {
        e.preventDefault();
        if (!disabled && allAnswered) onSubmit(answers);
      }}
    >
      <nav className="quiz-question-nav" aria-label="Навигация по вопросам">
        {questions.map((q, index) => (
          <button
            key={q.id || `q${index}`}
            type="button"
            className={`quiz-question-number${current === index ? ' is-current' : ''}${answers[q.id] !== undefined ? ' is-answered' : ''}`}
            aria-current={current === index ? 'step' : undefined}
            aria-label={`Вопрос ${index + 1}${answers[q.id] !== undefined ? ', ответ выбран' : ', ответ не выбран'}`}
            onClick={() => setCurrent(index)}
            disabled={disabled}
          >
            {index + 1}
          </button>
        ))}
      </nav>
      <p className="quiz-progress" aria-live="polite">Вопрос {current + 1} из {questions.length}</p>
      {question && (
        <fieldset className="quiz-current-question" key={question.id || current}>
          <legend>{question.text}</legend>
          <div className="quiz-options">
            {question.options.map((option, index) => {
              const inputId = `quiz-${lesson.id}-${current}-${index}`;
              return (
                <label key={index} className={`quiz-option-card${answers[question.id] === index ? ' is-selected' : ''}`} htmlFor={inputId}>
                  <input
                    id={inputId}
                    type="radio"
                    name={`quiz-${lesson.id}-${question.id || current}`}
                    value={index}
                    checked={answers[question.id] === index}
                    onChange={() => setAnswers((previous) => ({ ...previous, [question.id]: index }))}
                    disabled={disabled}
                  />
                  <span>{option}</span>
                </label>
              );
            })}
          </div>
        </fieldset>
      )}
      <div className="quiz-controls">
        <button type="button" className="btn" onClick={() => setCurrent((index) => Math.max(0, index - 1))} disabled={disabled || current === 0}>
          Назад
        </button>
        {current < questions.length - 1 ? (
          <button type="button" className="btn btn-primary" onClick={() => setCurrent((index) => Math.min(questions.length - 1, index + 1))} disabled={disabled}>
            Далее
          </button>
        ) : (
          <button type="submit" className="btn btn-primary" disabled={disabled || !allAnswered} aria-label={t('lesson.quizSubmit')}>
            {disabled ? t('lesson.checking') : t('lesson.quizSubmit')}
          </button>
        )}
      </div>
      {!allAnswered && current === questions.length - 1 && (
        <p className="quiz-required" role="status">Ответьте на каждый вопрос перед отправкой.</p>
      )}
    </form>
  );
}

// ---------- Код ----------

export function CodeEditor({
  lesson,
  onSubmit,
  disabled,
}: {
  lesson: Lesson;
  onSubmit: (code: string, language: string) => void;
  disabled: boolean;
}): JSX.Element {
  const template = lesson.codeTask?.template ?? '';
  // Язык задания фиксирован сервером; клиент передаёт его вместе с кодом.
  const language = lesson.codeTask?.language || 'python';
  const [code, setCode] = useState(template);
  const inputId = useId();
  return (
    <form
      className="code-editor"
      onSubmit={(e) => {
        e.preventDefault();
        if (!disabled && code.trim()) onSubmit(code, language);
      }}
    >
      <label className="sr-only" htmlFor={inputId}>
        {t('lesson.code')}
      </label>
      <textarea
        id={inputId}
        value={code}
        onChange={(e) => setCode(e.target.value)}
        placeholder={t('lesson.codePlaceholder')}
        rows={12}
        spellCheck={false}
        disabled={disabled}
        className="code-input"
      />
      <div className="code-toolbar">
        <button
          type="button"
          className="btn"
          onClick={() => setCode(template)}
          disabled={disabled || code === template}
          aria-label={t('code.reset')}
        >
          {t('code.reset')}
        </button>
        <button
          type="submit"
          className="btn btn-primary"
          disabled={disabled || !code.trim()}
          aria-label={t('lesson.codeRun')}
        >
          {disabled ? t('lesson.checking') : t('lesson.codeRun')}
        </button>
      </div>
    </form>
  );
}

// ---------- Урок ----------

export default function LessonView({ lesson, onComplete, completing, done }: Props): JSX.Element {
  if (lesson.kind === 'live') {
    const startsAt = lesson.startsAt;
    const inFuture = startsAt ? new Date(startsAt).getTime() > Date.now() : false;
    return (
      <section className="panel">
        <span className="pill live">{t('lesson.live')}</span>
        <h3 style={{ marginTop: '10px' }}>{lesson.title}</h3>
        {lesson.bodyMd && <Markdown source={lesson.bodyMd} />}
        {startsAt ? (
          <p>
            {inFuture ? t('lesson.liveStarts') : t('lesson.liveWhen')}:{' '}
            <time dateTime={startsAt}>{formatDate(startsAt)}</time>
          </p>
        ) : (
          <p className="muted">{t('lesson.liveNotScheduled')}</p>
        )}
        {lesson.liveUrl && (
          <p>
            <a className="btn" href={lesson.liveUrl} target="_blank" rel="noreferrer">
              {t('lesson.liveLink')}
            </a>
          </p>
        )}
        <CompleteAction lesson={lesson} done={done} completing={completing} onComplete={onComplete} />
      </section>
    );
  }

  if (lesson.kind === 'quiz') {
    const questions = lesson.quiz?.questions ?? [];
    return (
      <section className="panel">
        <h3>{lesson.title}</h3>
        <p className="muted">{t('lesson.quiz')}</p>
        {lesson.bodyMd && <Markdown source={lesson.bodyMd} />}
        {questions.length === 0 ? (
          <p role="alert" className="error">
            {t('error.generic')}
          </p>
        ) : (
          <QuizForm
            lesson={lesson}
            disabled={completing}
            onSubmit={(quizAnswers) => void onComplete({ quizAnswers })}
          />
        )}
      </section>
    );
  }

  if (lesson.kind === 'code') {
    return (
      <section className="panel">
        <h3>{lesson.title}</h3>
        <p className="muted">{t('lesson.code')}</p>
        {/* Условие задачи — bodyMd; шаблон — codeTask.template (обязательно показываем). */}
        {lesson.bodyMd && (
          <>
            <h4>{t('lesson.assignment')}</h4>
            <Markdown source={lesson.bodyMd} />
          </>
        )}
        {lesson.codeTask?.template && (
          <div className="code-template">
            <h4>{t('lesson.codeTemplate')}</h4>
            <pre>
              <code>{lesson.codeTask.template}</code>
            </pre>
          </div>
        )}
        <CodeEditor
          lesson={lesson}
          disabled={completing}
          onSubmit={(code, language) => void onComplete({ code, language })}
        />
      </section>
    );
  }

  // video_text по умолчанию
  return (
    <section className="panel">
      <h3>{lesson.title}</h3>
      {lesson.videoUrl && (
        // playsInline: без него iPhone открывает видео только на весь экран.
        <video
          className="lesson-video"
          src={lesson.videoUrl}
          poster={videoPosterUrl(lesson.videoUrl) ?? undefined}
          controls
          playsInline
          preload="metadata"
          style={{ maxWidth: '100%' }}
        />
      )}
      {lesson.bodyMd && <Markdown source={lesson.bodyMd} />}
      <CompleteAction lesson={lesson} done={done} completing={completing} onComplete={onComplete} />
    </section>
  );
}
