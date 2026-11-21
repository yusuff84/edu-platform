// Админ выдаёт пользователю платный курс → курс у пользователя в «Моих курсах».
import { chromium, devices } from '@playwright/test';
const base = process.argv[2] ?? 'http://127.0.0.1:3000';
const out = process.argv[3];
const courseTitle = process.env.GRANT_COURSE ?? 'Python с нуля';
const browser = await chromium.launch(process.env.AUDIT_CHANNEL ? { channel: process.env.AUDIT_CHANNEL } : {});
const errors = [];
const mk = async () => {
  const ctx = await browser.newContext({ ...devices['iPhone 13'], locale: 'ru-RU' });
  const page = await ctx.newPage();
  page.on('pageerror', (e) => errors.push(e.message));
  return page;
};
const login = async (page, email, password) => {
  await page.goto(base + '/login');
  await page.fill('input[type=email]', email);
  await page.fill('input[type=password]', password);
  await page.click('form button[type=submit]');
  await page.waitForTimeout(800);
};

// 1. Пользователь регистрируется сам.
const email = `gift${Date.now()}@example.com`;
const user = await mk();
await user.goto(base + '/register');
await user.fill('#register-email', email);
await user.fill('#register-password', 'giftpass123');
await user.click('form button[type=submit]');
await user.waitForURL((u) => !u.pathname.startsWith('/register'));
console.log('✓ пользователь зарегистрирован:', email);

// 2. Админ выдаёт платный курс.
const admin = await mk();
await login(admin, process.env.AUDIT_ADMIN_EMAIL, process.env.AUDIT_ADMIN_PASSWORD);
await admin.goto(base + '/admin/enrollments');
await admin.fill('#ae-grant-email', email);
const opt = admin.locator('#ae-grant-course option', { hasText: courseTitle }).first();
await admin.selectOption('#ae-grant-course', await opt.getAttribute('value'));
await admin.click('.ae-grant-form button[type=submit]');
await admin.locator('.ae-grant-ok').waitFor();
console.log('✓ админ:', await admin.locator('.ae-grant-ok').innerText());
if (out) await admin.screenshot({ path: out + '/grant-admin.png' });
await admin.click('.ae-grant-form button[type=submit]');
await admin.locator('.ae-grant-ok', { hasText: 'уже есть доступ' }).waitFor();
console.log('✓ повторная выдача:', await admin.locator('.ae-grant-ok').innerText());
await admin.fill('#ae-grant-email', 'nobody-x@example.com');
await admin.click('.ae-grant-form button[type=submit]');
await admin.locator('.ae-grant .error').waitFor();
console.log('✓ неизвестный email:', await admin.locator('.ae-grant .error').innerText());

// 3. У пользователя курс в «Моих курсах», урок открывается.
await user.goto(base + '/me');
await user.locator('.learning-dashboard-course', { hasText: courseTitle }).waitFor();
console.log('✓ курс в «Моих курсах»');
await user.locator('.learning-dashboard-course', { hasText: courseTitle }).locator('.btn-primary').click();
await user.waitForURL(/\/learn\//);
await user.locator('.study-continue, .lesson-pane').first().waitFor();
console.log('✓ обучение открывается:', user.url().replace(base, ''));
if (out) await user.screenshot({ path: out + '/grant-user.png' });
console.log(errors.length ? 'JS errors: ' + errors.join(' | ') : 'JS errors: нет');
await browser.close();
