// Просмотр урока: все четыре kind (video_text, live, quiz, code).
// Формы полей строго по контракту Task/00-api-contract.md:
//   - текст урока — bodyMd (markdown-строка контракта);
//   - квиз — quiz.questions[] c полем text, ответы { [questionId]: index } или
//     массив индексов для вопроса с multi (FR-010);
//   - эфир — liveUrl + startsAt;
//   - код — codeTask.{language, template, tests}, редактор инициализируется шаблоном.
// Вердикт «Зачтено / Не зачтено» показывает родитель (Learn.tsx) по полю passed.
// Оформление — Skillbox-style (panel/pill/btn из styles.css).

import { useId, useState } from 'react';
import type { ReactNode } from 'react';
import Editor from 'react-simple-code-editor';
import type { CompleteLessonInput, Lesson } from '../api/client';
import { formatDate } from '../api/client';
import { t } from '../i18n/t';
import CodeBlock, { highlightCode } from './CodeBlock';
import '../styles/learn.css';

interface Props {
  lesson: Lesson;
  /** Отправить урок на завершение: {} для video/live, quizAnswers/code — для quiz/code. */
  onComplete: (payload: CompleteLessonInput) => void | Promise<void>;
  /** Идёт отправка/проверка на сервере (кнопки заблокированы). */
  completing: boolean;
}

// ---------- Безопасный markdown-рендер ----------

type MdBlock =
  | { type: 'p'; lines: string[] }
  | { type: 'ul'; items: string[] }
  | { type: 'ol'; items: string[] }
  | { type: 'code'; text: string; language?: string }
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
      // Язык берём из info-string после ```: ```python, ```js и т.п.
      const info = line.trim().slice(3).trim();
      const language = info ? info.split(/\s+/)[0] : undefined;
      const text: string[] = [];
      i++;
      while (i < lines.length && !RE_FENCE.test(lines[i])) {
        text.push(lines[i]);
        i++;
      }
      i++; // пропускаем закрывающую «```»
      blocks.push({ type: 'code', text: text.join('\n'), language });
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
const RE_INLINE = /(\*\*[^*]+\*\*|`[^`]+`|\*[^*\n]+\*)/g;

function renderInline(text: string, keyBase: string): ReactNode[] {
  return text
    .split(RE_INLINE)
    .filter((part) => part !== '')
    .map((part, idx) => {
      const key = `${keyBase}-${idx}`;
      if (part.length > 4 && part.startsWith('**') && part.endsWith('**')) {
        return <strong key={key}>{part.slice(2, -2)}</strong>;
      }
      if (part.length > 2 && part.startsWith('`') && part.endsWith('`')) {
        return (
          <code key={key}>{part.slice(1, -1)}</code>
        );
      }
      if (part.length > 2 && part.startsWith('*') && part.endsWith('*')) {
        return <em key={key}>{part.slice(1, -1)}</em>;
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
            return <CodeBlock key={key} code={b.text} language={b.language} className="md-pre" />;
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
  onSubmit: (answers: Record<string, number | number[]>) => void;
  disabled: boolean;
}): JSX.Element {
  const questions = lesson.quiz?.questions ?? [];
  // Ответ всегда храним массивом: для одиночного вопроса в нём один индекс,
  // для multi — все отмеченные (FR-010).
  const [answers, setAnswers] = useState<Record<string, number[]>>({});
  // Пояснения показываем только после отправки ответов на проверку.
  const [answered, setAnswered] = useState(false);
  // Отправка заблокирована, пока не выбраны ответы на все вопросы.
  const allAnswered =
    questions.length > 0 &&
    questions.every((q) => {
      const picked = answers[q.id];
      return q.multi ? (picked?.length ?? 0) > 0 : picked?.length === 1;
    });
  return (
    <form
      className="quiz"
      onSubmit={(e) => {
        e.preventDefault();
        if (!disabled && allAnswered) {
          const payload: Record<string, number | number[]> = {};
          for (const q of questions) {
            const picked = answers[q.id] ?? [];
            // Одиночный вопрос отправляем числом (старый контракт), multi —
            // массивом индексов.
            payload[q.id] = q.multi ? picked : (picked[0] ?? -1);
          }
          setAnswered(true);
          onSubmit(payload);
        }
      }}
    >
      {questions.map((q, qi) => (
        <fieldset key={q.id || `q${qi}`}>
          <legend>{q.text}</legend>
          {q.options.map((opt, oi) => {
            const picked = answers[q.id] ?? [];
            const checked = q.multi ? picked.includes(oi) : picked[0] === oi;
            return (
              <label key={oi} className="radio">
                <input
                  type={q.multi ? 'checkbox' : 'radio'}
                  name={`q-${q.id || qi}`}
                  checked={checked}
                  onChange={() =>
                    setAnswers((a) => {
                      const current = a[q.id] ?? [];
                      if (!q.multi) return { ...a, [q.id]: [oi] };
                      const next = current.includes(oi)
                        ? current.filter((x) => x !== oi)
                        : [...current, oi].sort((x, y) => x - y);
                      return { ...a, [q.id]: next };
                    })
                  }
                  disabled={disabled}
                />{' '}
                {opt}
              </label>
            );
          })}
          {/* Пояснение показываем после ответа (backend отдаёт explanation в публичном виде). */}
          {answered && q.explanation && (
            <p className="quiz-explanation">
              <strong>{t('lesson.explanation')}:</strong> {q.explanation}
            </p>
          )}
        </fieldset>
      ))}
      <button
        type="submit"
        className="btn btn-primary"
        disabled={disabled || !allAnswered}
        aria-label={t('lesson.quizSubmit')}
      >
        {disabled ? t('lesson.checking') : t('lesson.quizSubmit')}
      </button>
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
      {/* Редактор с подсветкой: controlled value, табы и отступы, без innerHTML. */}
      <Editor
        value={code}
        onValueChange={setCode}
        highlight={(value) => highlightCode(value, language)}
        padding={12}
        tabSize={4}
        insertSpaces
        textareaId={inputId}
        disabled={disabled}
        placeholder={t('lesson.codePlaceholder')}
        className="code-input code-editor-input code-highlight"
        textareaClassName="code-editor-textarea"
        preClassName="code-editor-pre"
        style={{ minHeight: '16rem' }}
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

export default function LessonView({ lesson, onComplete, completing }: Props): JSX.Element {
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
        <button
          type="button"
          className="btn btn-primary"
          onClick={() => void onComplete({})}
          disabled={completing}
          aria-label={`${t('learn.complete')}: ${lesson.title}`}
        >
          {completing ? t('learn.completing') : t('learn.complete')}
        </button>
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
            <CodeBlock code={lesson.codeTask.template} language={lesson.codeTask.language} />
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
        <video className="lesson-video" src={lesson.videoUrl} controls style={{ maxWidth: '100%' }} />
      )}
      {lesson.bodyMd && <Markdown source={lesson.bodyMd} />}
      <button
        type="button"
        className="btn btn-primary"
        onClick={() => void onComplete({})}
        disabled={completing}
        aria-label={`${t('learn.complete')}: ${lesson.title}`}
      >
        {completing ? t('learn.completing') : t('learn.complete')}
      </button>
    </section>
  );
}
