// Предпросмотр текста урока: node scripts/preview-lesson.mjs <урок.md> <out-префикс> [заголовок]
// Снимает <out>-d.png (ширина 1100) и <out>-m.png (iPhone, 390). Картинки media/… берутся из каталога рядом с .md
// (или из --media <каталог>). Печатает предупреждения: горизонтальная прокрутка, битые картинки.
import { build } from 'esbuild';
import { chromium, devices } from '@playwright/test';
import { readFileSync, mkdirSync, writeFileSync, existsSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const args = process.argv.slice(2);
const mi = args.indexOf('--media');
const mediaDir = mi >= 0 ? path.resolve(args.splice(mi, 2)[1]) : null;
const [mdPath, outPrefix, title = path.basename(args[0], '.md')] = args;
const here = path.dirname(fileURLToPath(import.meta.url));
const dist = path.join(here, '.preview-dist');
if (!existsSync(path.join(dist, 'entry.js')) || process.env.REBUILD) {
  mkdirSync(dist, { recursive: true });
  await build({
    entryPoints: [path.join(here, 'preview/entry.tsx')], bundle: true, outdir: dist, format: 'esm', jsx: 'automatic',
    loader: { '.png': 'dataurl', '.svg': 'dataurl', '.woff2': 'file' }, logLevel: 'error', define: { 'process.env.NODE_ENV': '"production"' },
  });
}
let md = readFileSync(mdPath, 'utf8');
// media/<файл> ищется в --media <каталог курса> или рядом с .md.
const base = mediaDir ?? path.dirname(path.resolve(mdPath));
// Абсолютный путь «/Users/…»: страница открыта по file://, а рендерер пропускает только http(s) и пути от «/».
md = md.replace(/\]\((media\/[^)\s]+)\)/g, (m, rel) => `](${path.join(base, rel).replace(/ /g, '%20')})`);
const html = `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<link href="https://fonts.googleapis.com/css2?family=Manrope:wght@400;500;600;700;800&display=swap" rel="stylesheet">
<link rel="stylesheet" href="file://${dist}/entry.css"></head><body style="background:#f4f3ef"><div id="root"></div>
<script>window.__MD=${JSON.stringify(md)};window.__TITLE=${JSON.stringify(title)};</script>
<script type="module" src="file://${dist}/entry.js"></script></body></html>`;
const htmlPath = path.join(dist, `p-${process.pid}.html`);
writeFileSync(htmlPath, html);
const browser = await chromium.launch({ channel: 'chrome', args: ['--allow-file-access-from-files'] });
for (const [tag, opts] of [['d', { viewport: { width: 1100, height: 900 } }], ['m', { ...devices['iPhone 13'] }]]) {
  const page = await (await browser.newContext(opts)).newPage();
  const errs = [];
  page.on('pageerror', (e) => errs.push(e.message));
  await page.goto('file://' + htmlPath, { waitUntil: 'networkidle' });
  // Картинки на сайте грузятся лениво — для снимка и проверки загружаем все сразу.
  await page.evaluate(async () => {
    const imgs = [...document.images];
    imgs.forEach((i) => { i.loading = 'eager'; });
    await Promise.all(imgs.map((i) => (i.complete ? null : new Promise((r) => { i.onload = r; i.onerror = r; }))));
  });
  await page.waitForTimeout(300);
  const info = await page.evaluate(() => ({
    h: document.documentElement.scrollWidth > document.documentElement.clientWidth,
    broken: [...document.images].filter((i) => !i.complete || i.naturalWidth === 0).map((i) => i.getAttribute('src')),
  }));
  if (info.h) console.log(`! ${tag}: горизонтальная прокрутка`);
  if (info.broken.length) console.log(`! битые картинки: ${info.broken.join(', ')}`);
  if (errs.length) console.log(`! ошибки: ${errs.join(' | ')}`);
  await page.screenshot({ path: `${outPrefix}-${tag}.png`, fullPage: true });
}
await browser.close();
console.log('ok', outPrefix);
