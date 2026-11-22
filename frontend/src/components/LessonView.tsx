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
import { Markdown } from './Markdown';

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

// Markdown-рендер вынесен в components/Markdown.tsx (выноски, таблицы, рисунки и т.п.).
export { Markdown };

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
