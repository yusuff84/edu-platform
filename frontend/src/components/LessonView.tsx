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
        <fieldset key={q.id || String(qi)}>
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
      ))}
      <button type="submit" disabled={disabled}>
        {t('lesson.quizSubmit')}
      </button>
    </form>
  );
}

export function CodeEditor({
  onSubmit,
  disabled,
}: {
  onSubmit: (code: string, language: string) => void;
  disabled: boolean;
}): JSX.Element {
  const [code, setCode] = useState('');
  const [language, setLanguage] = useState('python');
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        onSubmit(code, language);
      }}
    >
      <label>
        {t('lesson.codeLanguage')}
        <select
          value={language}
          onChange={(e) => setLanguage(e.target.value)}
          disabled={disabled}
        >
          <option value="python">Python</option>
          <option value="js">JavaScript</option>
          <option value="htmlcss">HTML/CSS</option>
        </select>
      </label>
      <label>
        <textarea
          value={code}
          onChange={(e) => setCode(e.target.value)}
          placeholder={t('lesson.codePlaceholder')}
          rows={10}
          cols={60}
          disabled={disabled}
          aria-label={t('lesson.code')}
        />
      </label>
      <button type="submit" disabled={disabled || !code.trim()}>
        {t('lesson.codeRun')}
      </button>
    </form>
  );
}

export default function LessonView({ lesson, onComplete, completing }: Props): JSX.Element {
  if (lesson.kind === 'live') {
    return (
      <section>
        <h3>{lesson.title}</h3>
        <p>
          <strong>{t('lesson.live')}. </strong>
          {lesson.scheduledAt && (
            <span>
              {t('lesson.liveWhen')}: {new Date(lesson.scheduledAt).toLocaleString('ru-RU')}
            </span>
          )}
        </p>
        {lesson.liveUrl && (
          <p>
            <a href={lesson.liveUrl} target="_blank" rel="noreferrer">
              {t('lesson.liveLink')}
            </a>
          </p>
        )}
        {lesson.textContent && <p>{lesson.textContent}</p>}
        <button onClick={() => void onComplete({})} disabled={completing}>
          {completing ? t('learn.completing') : t('learn.complete')}
        </button>
      </section>
    );
  }

  if (lesson.kind === 'quiz') {
    return (
      <section>
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
      <section>
        <h3>{lesson.title}</h3>
        <p className="muted">{t('lesson.code')}</p>
        {lesson.textContent && <p>{lesson.textContent}</p>}
        <CodeEditor
          disabled={completing}
          onSubmit={(code, language) => void onComplete({ code, language })}
        />
      </section>
    );
  }

  // video_text по умолчанию
  return (
    <section>
      <h3>{lesson.title}</h3>
      {lesson.videoUrl && (
        <video src={lesson.videoUrl} controls style={{ maxWidth: '100%' }} />
      )}
      {lesson.textContent && <p>{lesson.textContent}</p>}
      <button onClick={() => void onComplete({})} disabled={completing}>
        {completing ? t('learn.completing') : t('learn.complete')}
      </button>
    </section>
  );
}
