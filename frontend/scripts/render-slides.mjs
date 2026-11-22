// Рендер слайдов видеоурока: node scripts/render-slides.mjs <spec.json> <outdir>
// spec.json: { meta: {course, footer, kicker, theme}, slides: [...] } — макеты см. scripts/course-content/video/slides.html.
import { chromium } from '@playwright/test';
import { readFileSync, mkdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const [specPath, outDir] = process.argv.slice(2);
const spec = JSON.parse(readFileSync(specPath, 'utf8'));
const here = path.dirname(fileURLToPath(import.meta.url));
const tpl = path.resolve(here, '../../scripts/course-content/video/slides.html');
mkdirSync(outDir, { recursive: true });
const browser = await chromium.launch({ channel: 'chrome' });
const page = await (await browser.newContext({ viewport: { width: 1280, height: 720 }, deviceScaleFactor: 1.5 })).newPage();
await page.goto('file://' + tpl, { waitUntil: 'networkidle' });
await page.evaluate(() => document.fonts.ready);
for (const [i, sl] of spec.slides.entries()) {
  await page.evaluate(([s, m]) => window.renderSlide(s, m), [sl, { ...spec.meta, index: i, total: spec.slides.length }]);
  await page.waitForTimeout(80);
  // Проверка: ничего не вылезло за слайд.
  const over = await page.evaluate(() => {
    const s = document.getElementById('s');
    return [...s.querySelectorAll('*')].some((el) => { const r = el.getBoundingClientRect(); return r.bottom > 700 || r.right > 1282; });
  });
  if (over) console.log(`! слайд ${i + 1}: содержимое не помещается`);
  await page.screenshot({ path: path.join(outDir, `slide_${String(i + 1).padStart(2, '0')}.png`) });
}
await browser.close();
console.log(`слайдов: ${spec.slides.length}`);
