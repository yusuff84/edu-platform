/**
 * Безопасный markdown-рендер уроков: строит React-элементы из текста, сырой
 * HTML не интерпретируется. Стили — styles/markdown.css.
 *
 * Поддерживается:
 *  - абзацы, заголовки (##, ###), списки (-, 1.), цитаты (>), блоки кода (```), разделитель (---);
 *  - **жирный**, *курсив*, `код`, [ссылка](url), ![картинка](url), ==маркер==, [[Ctrl]] — клавиша;
 *  - картинка отдельным абзацем — рисунок с подписью (подпись = alt);
 *  - таблицы: | A | B | + строка |---|---|;
 *  - выноски: цитата, первая строка которой «[!ТИП] Заголовок»:
 *      SUMMARY (Главное), NOTE (Заметка), TIP (Совет), IMPORTANT (Важно),
 *      WARNING (Внимание), DANGER (Опасно), EXAMPLE (Пример), TERM (Термин),
 *      PRACTICE (Попробуйте сами), CHECK (Проверьте себя).
 *    Внутри выноски — полноценный markdown (списки, код, таблицы).
 */
import type { ReactNode } from 'react';
import '../styles/markdown.css';

type CalloutType =
  | 'summary'
  | 'note'
  | 'tip'
  | 'important'
  | 'warning'
  | 'danger'
  | 'example'
  | 'term'
  | 'practice'
  | 'check';

type MdBlock =
  | { type: 'p'; lines: string[] }
  | { type: 'ul'; items: string[] }
  | { type: 'ol'; items: string[]; start: number }
  | { type: 'code'; text: string; lang: string }
  | { type: 'quote'; lines: string[] }
  | { type: 'callout'; kind: CalloutType; title: string; body: string }
  | { type: 'table'; head: string[]; align: ('left' | 'center' | 'right')[]; rows: string[][] }
  | { type: 'figure'; alt: string; url: string }
  | { type: 'hr' }
  | { type: 'h'; level: number; text: string };

const CALLOUTS: Record<CalloutType, { label: string; icon: string }> = {
  summary: { label: 'Главное', icon: '★' },
  note: { label: 'Заметка', icon: 'i' },
  tip: { label: 'Совет', icon: '✓' },
  important: { label: 'Важно', icon: '!' },
  warning: { label: 'Внимание', icon: '!' },
  danger: { label: 'Опасно', icon: '×' },
  example: { label: 'Пример', icon: '›' },
  term: { label: 'Термин', icon: 'Аа' },
  practice: { label: 'Попробуйте сами', icon: '▶' },
  check: { label: 'Проверьте себя', icon: '?' },
};

const CALLOUT_ALIASES: Record<string, CalloutType> = {
  SUMMARY: 'summary', TLDR: 'summary', 'ГЛАВНОЕ': 'summary', 'КОРОТКО': 'summary',
  NOTE: 'note', INFO: 'note', 'ЗАМЕТКА': 'note',
  TIP: 'tip', HINT: 'tip', 'СОВЕТ': 'tip',
  IMPORTANT: 'important', 'ВАЖНО': 'important',
  WARNING: 'warning', CAUTION: 'warning', 'ВНИМАНИЕ': 'warning',
  DANGER: 'danger', 'ОПАСНО': 'danger',
  EXAMPLE: 'example', 'ПРИМЕР': 'example',
  TERM: 'term', DEFINITION: 'term', 'ТЕРМИН': 'term',
  PRACTICE: 'practice', TRY: 'practice', 'ПРАКТИКА': 'practice',
  CHECK: 'check', QUESTION: 'check', 'ПРОВЕРЬТЕ': 'check',
};

const RE_FENCE = /^\s*```\s*([\w+-]*)\s*$/;
const RE_FENCE_ANY = /^\s*```/;
const RE_HEADING = /^(#{1,6})\s+(.*)$/;
const RE_QUOTE = /^\s*>\s?/;
const RE_UL = /^\s*[-*+]\s+/;
const RE_OL = /^\s*(\d+)[.)]\s+/;
const RE_HR = /^\s*(-{3,}|\*{3,}|_{3,})\s*$/;
const RE_TABLE_ROW = /^\s*\|.*\|\s*$/;
const RE_TABLE_SEP = /^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?\s*$/;
const RE_CALLOUT = /^\[!([A-Za-zА-Яа-яЁё]+)\]\s*(.*)$/;
const RE_FIGURE = /^!\[([^\]]*)\]\(([^\s)]+)\)$/;
// Начало нового блока — чтобы абзац обрывался на списке/заголовке/цитате/коде/таблице.
const RE_BLOCK_START = /^\s*([-*+]\s|\d+[.)]\s|>|#{1,6}\s|```|\|)/;

function splitRow(line: string): string[] {
  let s = line.trim();
  if (s.startsWith('|')) s = s.slice(1);
  if (s.endsWith('|')) s = s.slice(0, -1);
  // «\|» внутри ячейки — буквальная вертикальная черта.
  return s.split(/(?<!\\)\|/).map((c) => c.replace(/\\\|/g, '|').trim());
}

/** Разбирает markdown на блоки. */
export function parseBlocks(src: string): MdBlock[] {
  const lines = src.replace(/\r\n?/g, '\n').split('\n');
  const blocks: MdBlock[] = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (line.trim() === '') {
      i++;
      continue;
    }
    if (RE_FENCE_ANY.test(line)) {
      const lang = RE_FENCE.exec(line)?.[1] ?? '';
      // Блок кода внутри пункта списка набран с отступом — срезаем его.
      const indent = line.length - line.trimStart().length;
      const text: string[] = [];
      i++;
      while (i < lines.length && !RE_FENCE_ANY.test(lines[i])) {
        const ln = lines[i];
        const lead = ln.length - ln.trimStart().length;
        text.push(ln.slice(Math.min(indent, lead)));
        i++;
      }
      i++;
      blocks.push({ type: 'code', text: text.join('\n'), lang });
      continue;
    }
    if (RE_HR.test(line)) {
      blocks.push({ type: 'hr' });
      i++;
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
      const head = RE_CALLOUT.exec(quote[0].trim());
      const kind = head ? CALLOUT_ALIASES[head[1].toUpperCase()] : undefined;
      if (head && kind) {
        blocks.push({ type: 'callout', kind, title: head[2].trim(), body: quote.slice(1).join('\n') });
      } else {
        blocks.push({ type: 'quote', lines: quote });
      }
      continue;
    }
    if (RE_TABLE_ROW.test(line) && i + 1 < lines.length && RE_TABLE_SEP.test(lines[i + 1])) {
      const head = splitRow(line);
      const align = splitRow(lines[i + 1]).map((c) =>
        c.startsWith(':') && c.endsWith(':') ? 'center' : c.endsWith(':') ? 'right' : 'left',
      ) as ('left' | 'center' | 'right')[];
      i += 2;
      const rows: string[][] = [];
      while (i < lines.length && RE_TABLE_ROW.test(lines[i])) {
        const r = splitRow(lines[i]);
        rows.push(head.map((_, k) => r[k] ?? ''));
        i++;
      }
      blocks.push({ type: 'table', head, align, rows });
      continue;
    }
    if (RE_UL.test(line)) {
      const items: string[] = [];
      while (i < lines.length && (RE_UL.test(lines[i]) || (lines[i].startsWith('  ') && lines[i].trim() !== '' && items.length > 0 && !RE_OL.test(lines[i]) && !RE_FENCE_ANY.test(lines[i])))) {
        if (RE_UL.test(lines[i])) items.push(lines[i].replace(RE_UL, ''));
        else items[items.length - 1] += ' ' + lines[i].trim();
        i++;
      }
      blocks.push({ type: 'ul', items });
      continue;
    }
    if (RE_OL.test(line)) {
      const start = Number(RE_OL.exec(line)?.[1] ?? 1);
      const items: string[] = [];
      while (i < lines.length && (RE_OL.test(lines[i]) || (lines[i].startsWith('   ') && lines[i].trim() !== '' && items.length > 0 && !RE_UL.test(lines[i]) && !RE_FENCE_ANY.test(lines[i])))) {
        if (RE_OL.test(lines[i])) items.push(lines[i].replace(RE_OL, ''));
        else items[items.length - 1] += ' ' + lines[i].trim();
        i++;
      }
      blocks.push({ type: 'ol', items, start });
      continue;
    }
    const fig = RE_FIGURE.exec(line.trim());
    if (fig && (i + 1 >= lines.length || lines[i + 1].trim() === '')) {
      blocks.push({ type: 'figure', alt: fig[1], url: fig[2] });
      i++;
      continue;
    }
    const para: string[] = [];
    while (i < lines.length && lines[i].trim() !== '' && !RE_BLOCK_START.test(lines[i])) {
      para.push(lines[i].trim());
      i++;
    }
    if (para.length === 0) {
      // Строка похожа на начало блока, но не распозналась (например, «|» без таблицы) — обычный абзац.
      para.push(lines[i].trim());
      i++;
    }
    blocks.push({ type: 'p', lines: para });
  }
  return blocks;
}

// Порядок важен: ссылки/картинки, клавиши, маркер, жирный-курсив, жирный, код, курсив.
const RE_INLINE =
  /(!?\[[^\]\n]*\]\([^\s)]+\)|\[\[[^\]\n]{1,24}\]\]|==[^=\n]+==|\*\*\*[^*\n]+\*\*\*|\*\*[^*\n]+\*\*|`[^`]+`|\*(?:[^*\n]|\*\*[^*\n]+\*\*)+\*)/g;

export function safeMarkdownUrl(value: string): string | null {
  if (value.startsWith('/') && !value.startsWith('//') && !value.includes('\\')) return value;
  try {
    const url = new URL(value);
    return url.protocol === 'https:' || url.protocol === 'http:' ? url.href : null;
  } catch {
    return null;
  }
}

export function renderInline(text: string, keyBase: string): ReactNode[] {
  return text
    .split(RE_INLINE)
    .filter((part) => part !== '')
    .map((part, idx) => {
      const key = `${keyBase}-${idx}`;
      const media = /^(!?)\[([^\]]*)\]\(([^\s)]+)\)$/.exec(part);
      if (media) {
        const url = safeMarkdownUrl(media[3]);
        if (!url) return media[2];
        return media[1] ? (
          <img key={key} className="md-img-inline" src={url} alt={media[2]} loading="lazy" />
        ) : (
          <a key={key} href={url} target="_blank" rel="noopener noreferrer">
            {media[2]}
          </a>
        );
      }
      if (part.length > 4 && part.startsWith('[[') && part.endsWith(']]')) {
        return (
          <kbd key={key} className="md-kbd">
            {part.slice(2, -2)}
          </kbd>
        );
      }
      if (part.length > 4 && part.startsWith('==') && part.endsWith('==')) {
        return (
          <mark key={key} className="md-mark">
            {renderInline(part.slice(2, -2), key)}
          </mark>
        );
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
        return <code key={key}>{part.slice(1, -1)}</code>;
      }
      if (part.length > 2 && part.startsWith('*') && part.endsWith('*')) {
        return <em key={key}>{renderInline(part.slice(1, -1), key)}</em>;
      }
      return part;
    });
}

function Blocks({ source, keyBase }: { source: string; keyBase: string }): JSX.Element {
  const blocks = parseBlocks(source);
  return (
    <>
      {blocks.map((b, i) => {
        const key = `${keyBase}b${i}`;
        switch (b.type) {
          case 'h': {
            const inline = renderInline(b.text, key);
            if (b.level <= 2) return <h4 key={key}>{inline}</h4>;
            if (b.level === 3) return <h5 key={key}>{inline}</h5>;
            return <h6 key={key}>{inline}</h6>;
          }
          case 'hr':
            return <hr key={key} className="md-hr" />;
          case 'code':
            return (
              <pre key={key} className="md-pre" data-lang={b.lang || undefined}>
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
          case 'callout': {
            const meta = CALLOUTS[b.kind];
            return (
              <aside key={key} className={`md-callout md-callout--${b.kind}`}>
                <div className="md-callout-head">
                  <span className="md-callout-icon" aria-hidden="true">
                    {meta.icon}
                  </span>
                  <span className="md-callout-label">{meta.label}</span>
                  {b.title && <span className="md-callout-title">{renderInline(b.title, `${key}-t`)}</span>}
                </div>
                {b.body.trim() && (
                  <div className="md-callout-body">
                    <Blocks source={b.body} keyBase={`${key}-`} />
                  </div>
                )}
              </aside>
            );
          }
          case 'table':
            return (
              <div key={key} className="md-table-wrap" role="region" aria-label="Таблица" tabIndex={0}>
                <table className="md-table">
                  <thead>
                    <tr>
                      {b.head.map((h, j) => (
                        <th key={j} style={{ textAlign: b.align[j] }}>
                          {renderInline(h, `${key}-h${j}`)}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {b.rows.map((r, j) => (
                      <tr key={j}>
                        {r.map((c, k) => (
                          <td key={k} style={{ textAlign: b.align[k] }} data-label={b.head[k]}>
                            {renderInline(c, `${key}-${j}-${k}`)}
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            );
          case 'figure': {
            const url = safeMarkdownUrl(b.url);
            if (!url) return <p key={key}>{b.alt}</p>;
            return (
              <figure key={key} className="md-figure">
                <img src={url} alt={b.alt} loading="lazy" />
                {b.alt && <figcaption>{b.alt}</figcaption>}
              </figure>
            );
          }
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
              <ol
                key={key}
                start={b.start !== 1 ? b.start : undefined}
                style={b.start !== 1 ? { counterReset: `md-step ${b.start - 1}` } : undefined}
              >
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
    </>
  );
}

export function Markdown({ source }: { source: string }): JSX.Element {
  return (
    <div className="md">
      <Blocks source={source} keyBase="" />
    </div>
  );
}
