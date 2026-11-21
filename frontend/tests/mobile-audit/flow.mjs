// Сквозной мобильный сценарий ученика (локальный стенд).
import { chromium, devices } from '@playwright/test';
const base = process.argv[2] ?? 'http://127.0.0.1:3000';
const browser = await chromium.launch(process.env.AUDIT_CHANNEL ? { channel: process.env.AUDIT_CHANNEL } : {});
const ctx = await browser.newContext({ ...devices['iPhone 13'], locale: 'ru-RU' });
const page = await ctx.newPage();
const errors = [];
page.on('pageerror', (e) => errors.push(e.message));
const step = (m) => console.log('✓', m);
const email = `flow${Date.now()}@example.com`;

await page.goto(base + '/courses/cifrovaya-gigiena');
await page.locator('.course-sticky-cta .btn').click();
await page.waitForURL('**/register'); step('гость: нижняя кнопка курса ведёт на регистрацию');
await page.fill('#register-email', email);
await page.fill('#register-password', 'flowpass123');
await page.click('form button[type=submit]');
await page.waitForURL('**/courses/cifrovaya-gigiena'); step('после регистрации вернулись на страницу курса');
await page.locator('.course-sticky-cta .btn', { hasText: 'Записаться' }).click();
await page.locator('.course-sticky-cta .btn', { hasText: 'Перейти к обучению' }).waitFor();
step('запись на бесплатный курс → кнопка «Перейти к обучению»');
await page.locator('.course-sticky-cta .btn').click();
await page.waitForURL(/\/learn\/[^/]+$/);
await page.locator('.study-continue').click();
await page.waitForURL(/\/learn\/[^/]+\/[^/]+$/); step('«Начать обучение» открывает первый шаг');
await page.locator('.lesson-complete-btn').click();
await page.locator('.lesson-done-note').waitFor(); step('«Отметить пройденным» → статус «Урок пройден»');
const before = page.url();
await page.locator('.lesson-bottom-bar .lbb-next').click();
await page.waitForURL((u) => u.toString() !== before); step('нижняя панель: переход к следующему шагу');
// Меню: переход в «Мои курсы»
await page.locator('.mnav-toggle').click();
await page.locator('.mnav-link', { hasText: 'Мои курсы' }).click();
await page.waitForURL('**/me'); step('мобильное меню: переход в «Мои курсы»');
// Квиз
await page.goto(base + '/learn/541c6896-f162-58c2-84d1-072698934edc/4f3908dd-6ed8-5979-af39-31ad7c66b2b8');
await page.locator('.quiz-option-card').first().click();
await page.locator('.quiz-controls button[type=submit]').click();
await page.locator('.learn-verdict p').first().waitFor(); 
step('тест: ответ отправлен, вердикт — «' + (await page.locator('.learn-verdict p').first().innerText()) + '»');
const sw = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
step('горизонтальной прокрутки нет: ' + (sw <= 0));
await page.locator('.mnav-toggle').click();
await page.locator('.mnav-logout').click();
await page.locator('.top-login').waitFor(); step('выход через меню');
console.log(errors.length ? 'JS errors: ' + errors.join(' | ') : 'JS errors: нет');
await browser.close();
