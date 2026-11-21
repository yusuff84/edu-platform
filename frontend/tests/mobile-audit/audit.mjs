// Мобильный аудит: полностраничные скриншоты + автоматические проверки
// (горизонтальный скролл, элементы за краем экрана, мелкие зоны нажатия,
// мелкий шрифт в полях ввода — iOS зумит страницу при фокусе, если < 16px).
//
// Запуск:
//   node tests/mobile-audit/audit.mjs <baseURL> <outDir> <routes.json> [width,...]
// routes.json: [{ "name": "catalog", "path": "/", "login": "student" | null,
//                "actions": [ { "click": "selector" } | { "wait": 500 } ] }]
// Учётки берутся из env: AUDIT_<ROLE>_EMAIL / AUDIT_<ROLE>_PASSWORD.
import { chromium, devices } from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';

const [baseURL, outDir, routesFile, widthsArg] = process.argv.slice(2);
const widths = (widthsArg ?? '390').split(',').map(Number);
const routes = JSON.parse(fs.readFileSync(routesFile, 'utf8'));
fs.mkdirSync(outDir, { recursive: true });

const browser = await chromium.launch(process.env.AUDIT_CHANNEL ? { channel: process.env.AUDIT_CHANNEL } : {});
const report = [];

async function login(context, role) {
  const email = process.env[`AUDIT_${role.toUpperCase()}_EMAIL`];
  const password = process.env[`AUDIT_${role.toUpperCase()}_PASSWORD`];
  const page = await context.newPage();
  await page.goto(baseURL + '/login');
  await page.fill('input[type=email]', email);
  await page.fill('input[type=password]', password);
  await Promise.all([
    page.waitForResponse(r => r.url().includes('/auth/login')),
    page.click('form button[type=submit]'),
  ]);
  await page.waitForTimeout(500);
  await page.close();
}

const checks = () => {
  const vw = document.documentElement.clientWidth;
  const out = { scrollWidth: document.documentElement.scrollWidth, vw, overflow: [], smallTargets: [], smallInputs: [] };
  const describe = el => {
    const cls = typeof el.className === 'string' ? el.className.trim().split(/\s+/).slice(0, 3).join('.') : '';
    const txt = (el.innerText || el.value || el.getAttribute('aria-label') || '').trim().replace(/\s+/g, ' ').slice(0, 40);
    return `${el.tagName.toLowerCase()}${cls ? '.' + cls : ''}${txt ? ` «${txt}»` : ''}`;
  };
  const visible = el => {
    const s = getComputedStyle(el);
    if (s.visibility === 'hidden' || s.display === 'none' || Number(s.opacity) === 0) return false;
    const r = el.getBoundingClientRect();
    return r.width > 0 && r.height > 0;
  };
  // Элементы, выходящие за правый край (без тех, что внутри прокручиваемых контейнеров).
  for (const el of document.querySelectorAll('body *')) {
    if (!visible(el)) continue;
    const r = el.getBoundingClientRect();
    if (r.right > vw + 1 || r.left < -1) {
      let p = el.parentElement, clipped = false;
      while (p && p !== document.body) {
        const s = getComputedStyle(p);
        if (/(auto|scroll|hidden|clip)/.test(s.overflowX)) { clipped = true; break; }
        p = p.parentElement;
      }
      if (!clipped && !el.classList.contains('sr-only')) out.overflow.push(`${describe(el)} [${Math.round(r.left)}..${Math.round(r.right)}]`);
    }
  }
  for (const el of document.querySelectorAll('a, button, input:not([type=hidden]), select, textarea, [role=button], summary, label')) {
    if (!visible(el)) continue;
    const r = el.getBoundingClientRect();
    const isInline = el.tagName === 'A' && getComputedStyle(el).display === 'inline' && el.closest('p, li');
    if (!isInline && el.tagName !== 'LABEL' && (r.height < 36 || r.width < 36)) out.smallTargets.push(`${describe(el)} ${Math.round(r.width)}x${Math.round(r.height)}`);
    if (/^(INPUT|SELECT|TEXTAREA)$/.test(el.tagName) && !['checkbox', 'radio', 'range', 'file'].includes(el.type)) {
      const fs = parseFloat(getComputedStyle(el).fontSize);
      if (fs < 16) out.smallInputs.push(`${describe(el)} ${fs}px`);
    }
  }
  out.overflow = [...new Set(out.overflow)].slice(0, 25);
  out.smallTargets = [...new Set(out.smallTargets)].slice(0, 40);
  out.smallInputs = [...new Set(out.smallInputs)].slice(0, 20);
  return out;
};

for (const width of widths) {
  const contexts = {};
  const ctxFor = async role => {
    const key = role ?? 'anon';
    if (!contexts[key]) {
      contexts[key] = await browser.newContext({
        ...(width >= 1024 ? devices['Desktop Chrome'] : devices['iPhone 13']),
        viewport: { width, height: width >= 1024 ? 860 : 844 },
        deviceScaleFactor: Number(process.env.AUDIT_DSF ?? 1),
        locale: 'ru-RU',
      });
      if (role) await login(contexts[key], role);
    }
    return contexts[key];
  };
  for (const route of routes) {
    const ctx = await ctxFor(route.login);
    const page = await ctx.newPage();
    const errors = [];
    page.on('pageerror', e => errors.push(String(e.message).slice(0, 200)));
    page.on('console', m => { if (m.type() === 'error') errors.push(m.text().slice(0, 200)); });
    try {
      await page.goto(baseURL + route.path, { waitUntil: 'networkidle', timeout: 30000 });
    } catch (e) {
      errors.push('goto: ' + e.message.slice(0, 120));
    }
    await page.waitForTimeout(route.wait ?? 600);
    for (const a of route.actions ?? []) {
      try {
        if (a.click) await page.click(a.click, { timeout: 5000 });
        if (a.fill) await page.fill(a.fill[0], a.fill[1]);
        if (a.wait) await page.waitForTimeout(a.wait);
        if (a.scroll) await page.evaluate(y => window.scrollTo(0, y), a.scroll);
      } catch (e) { errors.push('action: ' + e.message.split('\n')[0].slice(0, 160)); }
    }
    await page.waitForTimeout(300);
    const res = await page.evaluate(checks);
    const file = path.join(outDir, `${route.name}-${width}.png`);
    await page.screenshot({ path: file, fullPage: !route.viewportOnly });
    report.push({ route: route.name, width, url: page.url(), ...res, errors });
    await page.close();
  }
  for (const c of Object.values(contexts)) await c.close();
}
await browser.close();
fs.writeFileSync(path.join(outDir, 'report.json'), JSON.stringify(report, null, 2));
for (const r of report) {
  const flag = r.scrollWidth > r.vw ? `  ⚠ H-SCROLL ${r.scrollWidth}>${r.vw}` : '';
  console.log(`\n### ${r.route} @${r.width}${flag}  ${r.url}`);
  if (r.overflow.length) console.log('  overflow:', r.overflow.join(' | '));
  if (r.smallTargets.length) console.log('  small targets:', r.smallTargets.join(' | '));
  if (r.smallInputs.length) console.log('  small inputs:', r.smallInputs.join(' | '));
  if (r.errors.length) console.log('  errors:', r.errors.join(' | '));
}
