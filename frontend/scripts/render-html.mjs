// Рендер схемы/иллюстрации для урока: node scripts/render-html.mjs <file.html> <out.png> [ширина=1600]
// Снимается элемент #art (или body), с плотностью 1.5 — чётко на ретине и не тяжело.
import { chromium } from '@playwright/test';
const [src, out, w = '1600'] = process.argv.slice(2);
const browser = await chromium.launch({ channel: 'chrome' });
const page = await (await browser.newContext({ viewport: { width: Number(w), height: 900 }, deviceScaleFactor: 1.5 })).newPage();
await page.goto('file://' + (src.startsWith('/') ? src : process.cwd() + '/' + src), { waitUntil: 'networkidle' });
await page.evaluate(() => document.fonts.ready);
const el = (await page.$('#art')) ?? (await page.$('body'));
await el.screenshot({ path: out });
await browser.close();
console.log('ok', out);
