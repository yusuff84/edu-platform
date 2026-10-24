// Предпросмотр черновика урока (T28).
//
// Компонент принимает НЕ сохранённый урок, а собранный из формы объект
// LessonPreviewData — учитель видит результат до отправки на сервер.
// bodyMd рендерится общим Markdown из LessonView (свой markdown-рендерер
// не пишем), оформление — только классы .ce-* из constructorExtra.css.
//
// Это предпросмотр глазами учителя, поэтому в нём ВИДНЫ ключ квиза
// («Верный ответ») и скрытые тесты код-задачи («Скрытый тест»): ученику
// они не показываются.

import { Markdown } from './LessonView';
import CodeBlock from './CodeBlock';
import { formatDate } from '../api/client';
import type { LessonKind } from '../api/client';
import { t } from '../i18n/t';
import '../styles/constructorExtra.css';

/** Черновик урока, собранный из формы Constructor (до отправки на сервер). */
export type LessonPreviewData = {
  title: string;
  kind: LessonKind;
  bodyMd: string;
  videoUrl: string;
  liveUrl: string;
  startsAt: string;
  quiz?: {
    questions: Array<{
      text: string;
      options: string[];
      correct: number;
      explanation?: string;
    }>;
  };
  codeTask?: {
    language: string;
    template?: string;
    tests: Array<{ name: string; input: string; expected: string; hidden?: boolean }>;
  };
};

// Ключи добавлены координатором после волны (см. отчёт T28):
// пустой черновик и пометка правильного варианта в предпросмотре.
const MSG_PREVIEW_EMPTY = t('teach.previewEmpty');
const MSG_CORRECT = t('teach.correctAnswer');

/** Есть ли в черновике хоть что-то, что стоит показывать. */
function hasContent(d: LessonPreviewData): boolean {
  if (
    d.title.trim() ||
    d.bodyMd.trim() ||
    d.videoUrl.trim() ||
    d.liveUrl.trim() ||
    d.startsAt.trim()
  ) {
    return true;
  }
  if (d.quiz?.questions.some((q) => q.text.trim() || q.options.some((o) => o.trim()))) {
    return true;
  }
  if (
    d.codeTask &&
    (d.codeTask.template?.trim() ||
      d.codeTask.tests.some((x) => x.name.trim() || x.input.trim() || x.expected.trim()))
  ) {
    return true;
  }
  return false;
}

/** Вопросы квиза: правильный вариант подсвечен, пояснение показано. */
function QuizPreview({
  questions,
}: {
  questions: NonNullable<LessonPreviewData['quiz']>['questions'];
}): JSX.Element | null {
  // Пустые вопросы (ещё не заполнены) не рисуем: незачем показывать пустую рамку.
  const visible = questions.filter(
    (q) => q.text.trim() || q.options.some((o) => o.trim()),
  );
  if (visible.length === 0) return null;
  return (
    <ol className="ce-quiz">
      {visible.map((q, qi) => (
        <li key={qi} className="ce-quiz-item">
          <p className="ce-quiz-text">
            {qi + 1}. {q.text.trim() || '—'}
          </p>
          <ul className="ce-quiz-options">
            {q.options.map((opt, oi) => {
              const isCorrect = oi === q.correct;
              return (
                <li
                  key={oi}
                  className={isCorrect ? 'ce-quiz-option ce-quiz-option-correct' : 'ce-quiz-option'}
                >
                  <span className="ce-quiz-radio" aria-hidden="true">
                    {isCorrect ? '●' : '○'}
                  </span>
                  <span className="ce-quiz-option-text">{opt.trim() || '—'}</span>
                  {isCorrect && <span className="ce-badge-ok">{MSG_CORRECT}</span>}
                </li>
              );
            })}
          </ul>
          {q.explanation && q.explanation.trim() && (
            <p className="ce-quiz-explanation">
              <strong>{t('lesson.explanation')}:</strong> {q.explanation}
            </p>
          )}
        </li>
      ))}
    </ol>
  );
}

/** Код-задача: язык, шаблон и таблица тестов; скрытые тесты помечены. */
function CodePreview({
  task,
}: {
  task: NonNullable<LessonPreviewData['codeTask']>;
}): JSX.Element {
  // Пустые строки таблицы не рисуем — в черновике это ещё не тесты.
  const tests = task.tests.filter(
    (x) => x.name.trim() || x.input.trim() || x.expected.trim(),
  );
  return (
    <div className="ce-code">
      <p className="muted ce-code-language">
        {t('lesson.codeLanguage')}: {task.language}
      </p>
      {task.template && task.template.trim() && (
        <div className="ce-template">
          <h4>{t('lesson.codeTemplate')}</h4>
          <CodeBlock code={task.template} language={task.language} />
        </div>
      )}
      {tests.length > 0 && (
        <table className="ce-tests">
          <thead>
            <tr>
              <th>{t('teach.testName')}</th>
              <th>{t('teach.testInput')}</th>
              <th>{t('teach.testExpected')}</th>
            </tr>
          </thead>
          <tbody>
            {tests.map((x, i) => (
              <tr key={i}>
                <td>
                  {x.name.trim() || '—'}
                  {x.hidden && <span className="ce-badge-hidden">{t('teach.testHidden')}</span>}
                </td>
                <td className="ce-test-io">{x.input.trim() || '—'}</td>
                <td className="ce-test-io">{x.expected.trim() || '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

export default function LessonPreview({ draft }: { draft: LessonPreviewData }): JSX.Element {
  if (!hasContent(draft)) {
    return (
      <p className="muted ce-preview-empty" role="note">
        {MSG_PREVIEW_EMPTY}
      </p>
    );
  }

  // Начало эфира: как в LessonView — в будущем «Эфир начнётся», иначе «Начало».
  const startsAt = draft.startsAt.trim();
  const inFuture = startsAt ? new Date(startsAt).getTime() > Date.now() : false;

  return (
    <div className="ce-preview">
      <span className="teach-lesson-kind">{t(`kind.${draft.kind}`)}</span>
      {draft.title.trim() && <h4 className="ce-preview-title">{draft.title}</h4>}
      {draft.bodyMd.trim() && <Markdown source={draft.bodyMd} />}

      {draft.kind === 'video_text' && draft.videoUrl.trim() && (
        <p className="ce-video-link">
          {t('teach.videoUrl')}:{' '}
          <a href={draft.videoUrl} target="_blank" rel="noreferrer">
            {draft.videoUrl}
          </a>
        </p>
      )}

      {draft.kind === 'live' && (
        <>
          {startsAt && (
            <p className="muted">
              {inFuture ? t('lesson.liveStarts') : t('lesson.liveWhen')}:{' '}
              <time dateTime={startsAt}>{formatDate(startsAt)}</time>
            </p>
          )}
          {draft.liveUrl.trim() && (
            <p>
              <a className="btn" href={draft.liveUrl} target="_blank" rel="noreferrer">
                {t('lesson.liveLink')}
              </a>
            </p>
          )}
        </>
      )}

      {draft.kind === 'quiz' && draft.quiz && <QuizPreview questions={draft.quiz.questions} />}
      {draft.kind === 'code' && draft.codeTask && <CodePreview task={draft.codeTask} />}
    </div>
  );
}
