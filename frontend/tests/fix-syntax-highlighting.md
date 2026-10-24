# Подсветка синтаксиса в уроках и редакторе кода

Ветка: `fix/syntax-highlighting`. Задача: вернуть подсветку кода в markdown,
шаблоне код-задачи, предпросмотре урока и редактируемом поле.

## Что изменено

- Добавлены зависимости `prism-react-renderer@^2.4.1` и
  `react-simple-code-editor@^0.14.1` (установлены через npm, обновлены
  `package.json` и `package-lock.json`).
- Новый компонент `src/components/CodeBlock.tsx`:
  - принимает `code`, `language`, опциональный `className`;
  - нормализует алиасы языков;
  - для неизвестного языка рендерит обычный `<pre><code>` без падения;
  - рендерит токены Prism обычными React-элементами, без
    `dangerouslySetInnerHTML` с пользовательским кодом;
  - сохраняет переносы, отступы, `tab-size`, горизонтальный скролл внутри
    `<pre>`; блок доступен с клавиатуры (`tabIndex`, `aria-label`).
- `LessonView.tsx`:
  - markdown-парсер сохраняет язык из info-string после ``` (```python, ```js);
  - fenced-блоки рендерятся через `CodeBlock`;
  - блоки без языка остаются обычным `<pre><code>` с прежним классом `.md-pre`;
  - шаблон `lesson.codeTask.template` рендерится через `CodeBlock` с языком
    `lesson.codeTask.language`;
  - `CodeEditor` переведён на `react-simple-code-editor`: controlled-значение,
    подсветка, сброс к шаблону, `disabled`, отправка формы, доступное имя поля,
    `spellCheck=false`, табы/отступы (`tabSize=4`, `insertSpaces`).
- `LessonPreview.tsx`: шаблон `task.template` рендерится через `CodeBlock`.
- `src/styles/learn.css`: светлая палитра токенов, стили `.code-block` и
  контейнера редактора в духе прежнего `.code-input` (фон `#f4f4f5`, рамка
  `#e5e5e5`, радиус 8px). Существующие `.md-pre`, `.code-template pre`,
  `.ce-template pre` не удалены и продолжают применяться.

## Языки

- Языки платформы: `python`, `js`/`javascript`, `htmlcss`.
- Алиасы markdown-fence: `py`, `python3`, `js`, `jsx`, `ts`, `typescript`,
  `tsx`, `html`, `htm`, `xml`, `svg`, `css`, `json`, `sql`, `yml`, `yaml`,
  `md`, `markdown`, `c`, `c++`, `cpp`, `go`, `rust`, `kotlin`, `swift`,
  `objc`, `graphql`, `coffee`, `plain`/`text` и др.
- `htmlcss`: если в коде есть HTML-теги — грамматика `markup`, иначе `css`.
- Если грамматики нет во встроенном Prism (например, `bash`/`shell`, `java`,
  `csharp`, `scss`), блок показывается без подсветки, но без ошибок.

## Проверки

- `npm run build` — PASS (`tsc --noEmit` + `vite build`).
  Есть предупреждение Vite о чанке > 500 kB: Prism добавляет вес бандла.
- `npm test` — PASS, 235/235 тестов.
- Дополнительный SSR smoke-check `CodeBlock`:
  - python: есть токены `keyword`, `comment`, `function`;
  - javascript: есть `keyword`, `comment`, `number`;
  - htmlcss (HTML): есть `tag`, `attr-name`, `attr-value`;
  - htmlcss (CSS): есть `selector`, `property`, `color`;
  - неизвестный язык: обычный `<pre><code>`, пользовательский `<script>`
    экранирован, сырого HTML в разметке нет.

## Ограничения

- `react-simple-code-editor` мягко переносит длинные строки внутри поля, чтобы
  не растягивать страницу; в read-only блоках используется горизонтальный
  скролл внутри `<pre>`.
- `htmlcss` — один язык платформы: смешанный HTML+CSS в одном блоке
  подсвечивается по выбранной грамматике (обычно `markup`), а не двумя сразу.
- Некоторые языки (bash/shell и др.) не входят во встроенный набор Prism и
  остаются без цветовой подсветки.
- Визуальная проверка скриншотами не выполнялась (в окружении недоступна).
