// Подсветка синтаксиса для read-only блоков кода и редактора.
//
// Используем prism-react-renderer: он не делает сетевых запросов, не требует
// CDN и отдаёт токены, которые рендерятся обычными React-элементами.
// Пользовательский код нигде не вставляется через dangerouslySetInnerHTML:
// текст токенов экранируется React автоматически.
//
// Языки платформы: python, js (javascript) и htmlcss (HTML/CSS).
// Для htmlcss грамматика выбирается по содержимому: есть HTML-теги — markup,
// иначе css. Неизвестный язык показывается обычным <pre><code> без падения.

import { Fragment } from 'react';
import type { ReactNode } from 'react';
import { Highlight, Prism } from 'prism-react-renderer';
import type { PrismTheme } from 'prism-react-renderer';

/** Светлая тема: цвета токенов задаются классами .token.* в learn.css. */
const CODE_THEME: PrismTheme = {
  plain: { color: '#1f2328' },
  styles: [],
};

// Алиасы markdown-fence и поля language урока.
// Значение — грамматика Prism; htmlcss разбирается отдельно (см. resolveLanguage).
const LANGUAGE_ALIASES: Record<string, string> = {
  js: 'javascript',
  mjs: 'javascript',
  cjs: 'javascript',
  node: 'javascript',
  nodejs: 'javascript',
  javascript: 'javascript',
  jsx: 'jsx',
  react: 'jsx',
  ts: 'typescript',
  typescript: 'typescript',
  tsx: 'tsx',
  py: 'python',
  python3: 'python',
  python: 'python',
  html: 'markup',
  htm: 'markup',
  html5: 'markup',
  xhtml: 'markup',
  xml: 'markup',
  svg: 'markup',
  vue: 'markup',
  htmlcss: 'htmlcss',
  css: 'css',
  css3: 'css',
  json: 'json',
  jsonc: 'json',
  sql: 'sql',
  yml: 'yaml',
  yaml: 'yaml',
  md: 'markdown',
  markdown: 'markdown',
  c: 'c',
  'c++': 'cpp',
  cpp: 'cpp',
  'c#': 'csharp',
  cs: 'csharp',
  csharp: 'csharp',
  go: 'go',
  golang: 'go',
  rs: 'rust',
  rust: 'rust',
  java: 'java',
  kotlin: 'kotlin',
  kt: 'kotlin',
  kts: 'kts',
  swift: 'swift',
  objc: 'objectivec',
  objectivec: 'objectivec',
  graphql: 'graphql',
  gql: 'graphql',
  coffee: 'coffeescript',
  coffeescript: 'coffeescript',
  plain: 'plain',
  plaintext: 'plain',
  text: 'plain',
  txt: 'plain',
};

/** Есть ли у Prism грамматика (а не служебный метод вроде extend). */
function hasGrammar(language: string): boolean {
  const grammar = Prism.languages[language];
  return typeof grammar === 'object' && grammar !== null;
}

/**
 * Приводит язык к имени грамматики Prism.
 * Возвращает null, если язык не поддерживается: вызывающий код покажет
 * обычный текст. Для htmlcss возвращает специальное значение 'htmlcss'.
 */
export function normalizeLanguage(language?: string): string | null {
  const raw = (language ?? '')
    .trim()
    .toLowerCase()
    .replace(/^language-/, '')
    .replace(/^\./, '');
  if (!raw) return null;
  const normalized = LANGUAGE_ALIASES[raw] ?? raw;
  if (normalized === 'htmlcss') return 'htmlcss';
  return hasGrammar(normalized) ? normalized : null;
}

/**
 * Выбирает грамматику с учётом содержимого.
 * htmlcss — это HTML/CSS одним языком платформы; для чистого CSS
 * подсветка markup была бы бесполезной, поэтому смотрим на разметку.
 */
export function resolveLanguage(language: string | undefined, code: string): string | null {
  const normalized = normalizeLanguage(language);
  if (normalized === 'htmlcss') {
    // Именно тег/комментарий/декларация, а не сравнение «a < b» в CSS.
    return /<(?:!|\?|\/|[a-z][\w:-]*)(?:\s|>|\/)/i.test(code) ? 'markup' : 'css';
  }
  return normalized;
}

type TokenLike = {
  type: string;
  content: string | TokenLike | Array<string | TokenLike>;
  alias?: string | string[];
};

/** Рекурсивно превращает токены Prism в React-элементы (без innerHTML). */
function renderToken(token: string | TokenLike, key: string): ReactNode {
  if (typeof token === 'string') return token;
  const aliases = token.alias
    ? Array.isArray(token.alias)
      ? token.alias
      : [token.alias]
    : [];
  const className = ['token', token.type, ...aliases].join(' ');
  const content = token.content;
  const children = Array.isArray(content)
    ? content.map((child, i) => renderToken(child, `${key}-${i}`))
    : typeof content === 'string'
      ? content
      : renderToken(content, `${key}-n`);
  return (
    <span key={key} className={className}>
      {children}
    </span>
  );
}

/**
 * Подсвеченный код в виде ReactNode — для react-simple-code-editor.
 * Для неизвестного языка возвращает обычный текст (тоже ReactNode,
 * поэтому редактор не вставляет его через innerHTML).
 */
export function highlightCode(code: string, language?: string): ReactNode {
  const resolved = resolveLanguage(language, code);
  if (!resolved) return <>{code}<br /></>;
  const grammar = Prism.languages[resolved];
  if (!grammar || typeof grammar !== 'object') return <>{code}<br /></>;
  const tokens = Prism.tokenize(code, grammar) as Array<string | TokenLike>;
  // <br /> в конце — как в строковом варианте react-simple-code-editor:
  // так последняя пустая строка не схлопывается и textarea не «съезжает».
  return <>{tokens.map((token, i) => renderToken(token, String(i)))}<br /></>;
}

export interface CodeBlockProps {
  code: string;
  /** Язык из markdown-fence или codeTask.language. */
  language?: string;
  /** Дополнительный класс для <pre> (например, md-pre). */
  className?: string;
}

/**
 * Блок кода с подсветкой. Длинные строки прокручиваются внутри <pre>,
 * переносы и отступы сохраняются, блок доступен с клавиатуры.
 */
export default function CodeBlock({ code, language, className }: CodeBlockProps): JSX.Element {
  const resolved = resolveLanguage(language, code);
  const classes = ['code-block', 'code-highlight', className].filter(Boolean).join(' ');
  const label = resolved ? `Код (${language})` : 'Код';

  if (!resolved) {
    return (
      <pre className={classes} tabIndex={0} aria-label={label}>
        <code>{code}</code>
      </pre>
    );
  }

  return (
    <Highlight prism={Prism} theme={CODE_THEME} language={resolved} code={code}>
      {({ tokens, getTokenProps }) => (
        <pre
          className={`${classes} language-${resolved}`}
          tabIndex={0}
          aria-label={label}
          data-language={resolved}
        >
          <code>
            {tokens.map((line, i) => (
              <Fragment key={i}>
                {line.map((token, j) => (
                  <span key={j} {...getTokenProps({ token })} />
                ))}
                {i < tokens.length - 1 ? '\n' : null}
              </Fragment>
            ))}
          </code>
        </pre>
      )}
    </Highlight>
  );
}
