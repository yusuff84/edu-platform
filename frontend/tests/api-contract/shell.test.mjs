// P11: проверки публичного shell (router/Learn/Course) без запуска браузера.
//
// Часть проверок — по исходнику router.tsx: чистых helper-функций для ссылок
// и guard'а там нет (Layout завязан на react-router и query ['me']), поэтому
// фиксируем контракт разметки: категории в футере, aria-label, ленивый Course
// и админ-ссылки только под role === 'admin'. Helper «Чему вы научитесь»
// (courseOutcomes) экспортируется из Course.tsx и проверяется через Vite SSR.

import { after, before, test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const FRONTEND_ROOT = fileURLToPath(new URL('../..', import.meta.url));
const ROUTER_SOURCE = readFileSync(
  new URL('../../src/app/router.tsx', import.meta.url),
  'utf8',
);
const LEARN_SOURCE = readFileSync(
  new URL('../../src/pages/Learn.tsx', import.meta.url),
  'utf8',
);

let vite;
let courseOutcomes;

before(async () => {
  vite = await createServer({
    root: FRONTEND_ROOT,
    configFile: false,
    mode: 'test',
    logLevel: 'silent',
    server: { middlewareMode: true },
    appType: 'custom',
  });
  ({ courseOutcomes } = await vite.ssrLoadModule('/src/pages/Course.tsx'));
});

after(async () => {
  if (vite) await vite.close();
});

// ---------- router: ссылки направлений ----------

test('router: направления в футере ведут в каталог с фильтром category', () => {
  for (const slug of ['programming', 'design', 'data', 'marketing', 'ai']) {
    assert.match(
      ROUTER_SOURCE,
      new RegExp(`to="/\\?category=${slug}"`),
      `нет ссылки на каталог с фильтром category=${slug}`,
    );
  }
  assert.doesNotMatch(
    ROUTER_SOURCE,
    /<Link to="\/">(Программирование|Дизайн|Данные|Маркетинг|Нейросети)<\/Link>/,
    'направление всё ещё ведёт на общий «/»',
  );
});

test('router: у основной навигации осмысленный aria-label', () => {
  assert.match(ROUTER_SOURCE, /<nav className="nav" aria-label="Основная навигация">/);
});

// ---------- router: guard админ-ссылок в футере ----------

test('router: админ-ссылки футера показываются только под role === admin', () => {
  const footerStart = ROUTER_SOURCE.indexOf('<footer');
  const footerEnd = ROUTER_SOURCE.indexOf('</footer>');
  assert.ok(footerStart >= 0 && footerEnd > footerStart, 'футер не найден');

  const footer = ROUTER_SOURCE.slice(footerStart, footerEnd);
  const guardAt = footer.indexOf("user?.role === 'admin'");
  assert.ok(guardAt >= 0, 'в футере нет проверки роли admin');

  for (const href of ['/admin/moderation', '/admin/invites']) {
    const linkAt = footer.indexOf(`to="${href}"`);
    assert.ok(linkAt > guardAt, `${href} не закрыт guard'ом role === 'admin'`);
  }
});

// ---------- router: ленивый Course ----------

test('router: Course загружается отдельным чанком (React.lazy)', () => {
  assert.match(
    ROUTER_SOURCE,
    /const Course = lazy\(\(\) => import\('\.\.\/pages\/Course'\)\);/,
  );
  assert.doesNotMatch(ROUTER_SOURCE, /import Course from '\.\.\/pages\/Course';/);
  // Suspense вокруг Outlet уже есть — прямой заход на /courses/:slug получит fallback.
  assert.match(ROUTER_SOURCE, /<Suspense fallback=\{<RouteFallback \/>\}>/);
});

test('router: «Условия возврата» — существующий якорь, без новой страницы', () => {
  assert.match(ROUTER_SOURCE, /<Link to="\/#lead">Условия возврата<\/Link>/);
  assert.doesNotMatch(ROUTER_SOURCE, /RefundTerms|\/refund/);
});

// ---------- Learn: вердикт привязан к уроку из переменных мутации ----------

test('Learn: completeLesson получает lessonId в переменных, вердикт — из variables', () => {
  assert.match(LEARN_SOURCE, /interface CompleteLessonVars/);
  assert.match(
    LEARN_SOURCE,
    /completeLesson\(targetCourseId, targetLessonId, payload\)/,
  );
  assert.match(LEARN_SOURCE, /setVerdictState\(\{ lessonId: variables\.lessonId, result: res \}\)/);
  assert.match(LEARN_SOURCE, /setErrorState\(\{ lessonId: variables\.lessonId, message: errorText\(e\) \}\)/);
  assert.doesNotMatch(
    LEARN_SOURCE,
    /completeLesson\(courseId, activeLesson\.id/,
    'запрос всё ещё зависит от текущего activeLesson',
  );
});

// ---------- Course: «Чему вы научитесь» из данных курса ----------

test('courseOutcomes: пункты из модулей, без выдуманных IT-обещаний', () => {
  const modules = [
    { title: 'Основы', position: 1 },
    { title: 'Функции', position: 2 },
    { title: 'Файлы', position: 3 },
  ];
  assert.deepEqual(
    courseOutcomes({ category: 'programming', description: 'Любое описание', modules }),
    ['Основы', 'Функции', 'Файлы'],
  );
});

test('courseOutcomes: без модулей берёт части описания как есть', () => {
  assert.deepEqual(
    courseOutcomes({
      category: 'design',
      description: 'Базовый курс Python: синтаксис, функции, практика в песочнице.',
      modules: [],
    }),
    ['Синтаксис', 'Функции', 'Практика в песочнице'],
  );
});

test('courseOutcomes: для не-IT направлений блок скрыт (пустой список)', () => {
  assert.deepEqual(
    courseOutcomes({
      category: 'marketing',
      description: 'Продвижение продукта, воронки, реклама.',
      modules: [{ title: 'Введение', position: 1 }],
    }),
    [],
  );
  assert.deepEqual(
    courseOutcomes({
      category: 'other',
      description: '',
      modules: [],
    }),
    [],
  );
});
