import { useState } from 'react';
import type { Lesson } from '../api/client';
import { t } from '../i18n/t';

interface Props {
  lesson: Lesson;
  onComplete: (payload: {
    quizAnswers?: Record<string, number>;
    code?: string;
    language?: string;
  }) => Promise<void>;
  completing: boolean;
}

export function QuizForm({
  lesson,
  onSubmit,
  disabled,
}: {
  lesson: Lesson;
  onSubmit: (answers: Record<string, number>) => void;
  disabled: boolean;
}): JSX.Element {
  const [answers, setAnswers] = useState<Record<string, number>>({});
  const quiz = lesson.quiz ?? [];
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        onSubmit(answers);
      }}
    >
      {quiz.map((q, qi) => (
        <div className="quiz-q" key={q.id || String(qi)}>
          <fieldset>
            <legend>{q.question}</legend>
            {q.options.map((opt, oi) => (
              <label key={oi} className="radio">
                <input
                  type="radio"
                  name={`q-${qi}`}
                  checked={answers[q.id] === oi}
                  onChange={() => setAnswers((a) => ({ ...a, [q.id]: oi }))}
                  disabled={disabled}
                />
                {opt}
              </label>
            ))}
          </fieldset>
        </div>
      ))}
      <button type="submit" className="btn btn-primary" disabled={disabled}>
        {t('lesson.quizSubmit')}
      </button>
    </form>
  );
}

export function CodeEditor({
  onSubmit,
  disabled,
  initialCode = '',
}: {
  onSubmit: (code: string, language: string) => void;
  disabled: boolean;
  initialCode?: string;
}): JSX.Element {
  const [code, setCode] = useState(initialCode);
  const [language, setLanguage] = useState('python');
  return (
    <form
      className="form-grid"
      onSubmit={(e) => {
        e.preventDefault();
        onSubmit(code, language);
      }}
    >
      <div className="field">
        <label htmlFor="code-lang">{t('lesson.codeLanguage')}</label>
        <select
          id="code-lang"
          className="select"
          value={language}
          onChange={(e) => setLanguage(e.target.value)}
          disabled={disabled}
        >
          <option value="python">Python</option>
          <option value="js">JavaScript</option>
          <option value="htmlcss">HTML/CSS</option>
        </select>
      </div>
      <div className="field">
        <label htmlFor="code-text">{t('lesson.code')}</label>
        <textarea
          id="code-text"
          className="code-edit"
          value={code}
          onChange={(e) => setCode(e.target.value)}
          placeholder={t('lesson.codePlaceholder')}
          rows={10}
          cols={60}
          disabled={disabled}
          aria-label={t('lesson.code')}
        />
      </div>
      <div>
        <button
          type="submit"
          className="btn btn-primary"
          disabled={disabled || !code.trim()}
        >
          {t('lesson.codeRun')}
        </button>
      </div>
    </form>
  );
}

export default function LessonView({ lesson, onComplete, completing }: Props): JSX.Element {
  if (lesson.kind === 'live') {
    return (
      <section className="panel">
        <span className="pill live">{t('lesson.live')}</span>
        <h3 style={{ marginTop: '10px' }}>{lesson.title}</h3>
        <p>
          {lesson.scheduledAt && (
            <span>
              {t('lesson.liveWhen')}: {new Date(lesson.scheduledAt).toLocaleString('ru-RU')}
            </span>
          )}
        </p>
        {lesson.liveUrl && (
          <p>
            <a className="btn" href={lesson.liveUrl} target="_blank" rel="noreferrer">
              {t('lesson.liveLink')}
            </a>
          </p>
        )}
        {lesson.textContent && <p>{lesson.textContent}</p>}
        <button
          className="btn btn-primary"
          onClick={() => void onComplete({})}
          disabled={completing}
        >
          {completing ? t('learn.completing') : t('learn.complete')}
        </button>
      </section>
    );
  }

  if (lesson.kind === 'quiz') {
    return (
      <section className="panel">
        <h3>{lesson.title}</h3>
        <p className="muted">{t('lesson.quiz')}</p>
        {lesson.textContent && <p>{lesson.textContent}</p>}
        <QuizForm
          lesson={lesson}
          disabled={completing}
          onSubmit={(quizAnswers) => void onComplete({ quizAnswers })}
        />
      </section>
    );
  }

  if (lesson.kind === 'code') {
    return (
      <section className="panel">
        <h3>{lesson.title}</h3>
        <p className="muted">{t('lesson.code')}</p>
        {lesson.textContent && <p>{lesson.textContent}</p>}
        <CodeEditor
          disabled={completing}
          initialCode={lesson.codeTemplate}
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
      {lesson.textContent && <p>{lesson.textContent}</p>}
      <button
        className="btn btn-primary"
        onClick={() => void onComplete({})}
        disabled={completing}
      >
        {completing ? t('learn.completing') : t('learn.complete')}
      </button>
    </section>
  );
}
