// Снимает ../covers/<slug>.html в ../covers/<slug>.jpg (1600×1000).
// Запуск из frontend/ (там установлен playwright): node ../scripts/course-content/showcase/render_covers.mjs [slug…]
import { readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';

// playwright берём из frontend/node_modules (скрипт запускается из frontend/).
const { chromium } = createRequire(join(process.cwd(), 'package.json'))('@playwright/test');
const covers = join(dirname(fileURLToPath(import.meta.url)), '..', 'covers');
const only = process.argv.slice(2);
const slugs = readdirSync(covers)
  .filter((f) => f.endsWith('.html'))
  .map((f) => f.slice(0, -5))
  .filter((s) => (only.length ? only.includes(s) : true));

const browser = await chromium.launch({ channel: 'chrome' });
const page = await (await browser.newContext({ viewport: { width: 1600, height: 1000 } })).newPage();
for (const slug of slugs) {
  await page.goto('file://' + join(covers, slug + '.html'));
  await page.screenshot({ path: join(covers, slug + '.jpg'), type: 'jpeg', quality: 88 });
  console.log('ok', slug);
}
await browser.close();
