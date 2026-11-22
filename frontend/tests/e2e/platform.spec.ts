import { test, expect, type APIRequestContext, type Page } from '@playwright/test';

const origin = process.env.E2E_BASE_URL ?? 'http://127.0.0.1:13000';
const password = 'e2e-password123';
const unique = () => `${Date.now()}-${Math.random().toString(36).slice(2)}`;

test('mobile catalog fits the viewport and supports keyboard entry', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/');
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
  await page.keyboard.press('Tab');
  await expect(page.getByRole('link', { name: 'Перейти к содержимому', exact: true })).toBeFocused();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('mobile-catalog.png'), fullPage: true });
});

async function post(api: APIRequestContext, path: string, data: unknown = {}) {
  const response = await api.post('/api/v1' + path, { data, headers: { Origin: origin } });
  expect(response.ok(), await response.text()).toBeTruthy();
  return response.json();
}

async function register(page: Page, email: string, navigate = true) {
  if (navigate) await page.goto('/register');
  await page.locator('#register-email').fill(email);
  await page.locator('#register-password').fill(password);
  await page.getByRole('button', { name: 'Зарегистрироваться', exact: true }).click();
  await expect(page.locator('.user-email')).toHaveText(email);
}

test('lesson markdown displays local images and source links while rejecting unsafe URLs', async ({ page, playwright }) => {
  const teacher = await playwright.request.newContext({ baseURL: origin });
  const admin = await playwright.request.newContext({ baseURL: origin });
  try {
    await post(teacher, '/auth/login', { email: 'teacher@e2e.test', password });
    await post(admin, '/auth/login', { email: 'admin@e2e.test', password });
    const course = await post(teacher, '/teacher/courses', { title: 'Markdown media ' + unique(), description: 'Rich lesson content', priceCents: 0 });
    const module = await post(teacher, `/teacher/courses/${course.id}/modules`, { title: 'Materials' });
    const lesson = await post(teacher, `/teacher/courses/${course.id}/lessons`, {
      moduleId: module.id, title: 'Illustrated lesson', kind: 'video_text',
      bodyMd: '![Local illustration](/uploads/markdown-test.svg)\n\n[Source](https://stepik.org/course/146168)\n\n[Unsafe link](javascript:alert%281%29)\n\n![Unsafe image](data:text/html;base64,abc)\n\n<img src=x onerror=alert(1)>',
    });
    await post(teacher, `/teacher/courses/${course.id}/submit`);
    await post(admin, `/admin/courses/${course.id}/moderate`, { decision: 'approve' });
    await register(page, `markdown-${unique()}@e2e.test`);
    await post(page.request, '/enrollments', { courseId: course.id });
    await page.route('**/uploads/markdown-test.svg', route => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"><rect width="16" height="16" fill="blue"/></svg>' }));
    await page.goto(`/learn/${course.id}/${lesson.id}`);
    const content = page.locator('.md');
    await expect(content.getByRole('img', { name: 'Local illustration' })).toBeVisible();
    await expect.poll(() => content.locator('img').evaluate(img => (img as HTMLImageElement).naturalWidth)).toBe(16);
    await expect(content.getByRole('link', { name: 'Source', exact: true })).toHaveAttribute('href', 'https://stepik.org/course/146168');
    await expect(content.locator('a[href^="javascript:"],img[src^="data:"],img[onerror]')).toHaveCount(0);
    await expect(content).toContainText('<img src=x onerror=alert(1)>');
  } finally { await teacher.dispose(); await admin.dispose(); }
});

test('purchase, lesson, certificate, archived access and refund', async ({ page, playwright }) => {
  const teacher = await playwright.request.newContext({ baseURL: origin });
  const admin = await playwright.request.newContext({ baseURL: origin });
  try {
    await post(teacher, '/auth/login', { email: 'teacher@e2e.test', password });
    await post(admin, '/auth/login', { email: 'admin@e2e.test', password });
    const course = await post(teacher, '/teacher/courses', { title: 'Browser course ' + unique(), description: 'Browser regression', priceCents: 10000 });
    const module = await post(teacher, `/teacher/courses/${course.id}/modules`, { title: 'Module' });
    const lesson = await post(teacher, `/teacher/courses/${course.id}/lessons`, { moduleId: module.id, title: 'Private lesson', kind: 'video_text', bodyMd: 'Purchased private lesson body' });
    await post(teacher, `/teacher/courses/${course.id}/submit`);
    await post(admin, `/admin/courses/${course.id}/moderate`, { decision: 'approve' });
    const publicResponse = await page.request.get(`/api/v1/courses/${course.slug}`);
    expect(await publicResponse.text()).not.toContain('Purchased private lesson body');
    await register(page, `student-${unique()}@e2e.test`);
    await page.goto(`/courses/${course.slug}`);
    await page.getByRole('button', { name: 'Купить', exact: true }).click();
    await expect(page).toHaveURL(/\/pay\/mock\//);
    await page.getByRole('button', { name: 'Оплатить', exact: true }).click();
    await expect(page.locator('#result')).toContainText('processed');
    await page.goto(`/learn/${course.id}/${lesson.id}`);
    await expect(page.getByText('Purchased private lesson body', { exact: true })).toBeVisible();
    await page.getByRole('button', { name: /^Отметить пройденным:/ }).click();
    await page.getByRole('link', { name: 'Открыть сертификат', exact: true }).click();
    await expect(page.locator('.cert-number')).toHaveText(/^EDU-/);
    await post(teacher, `/teacher/courses/${course.id}/archive`);
    await page.goto(`/learn/${course.id}/${lesson.id}`);
    await expect(page.getByText('Purchased private lesson body', { exact: true })).toBeVisible();
    const journal = await admin.get('/api/v1/admin/payments?courseId=' + course.id);
    const { items } = await journal.json();
    await post(admin, `/admin/payments/${items[0].id}/refund`);
    await page.reload();
    await expect(page.getByText('Purchased private lesson body', { exact: true })).toHaveCount(0);
  } finally { await teacher.dispose(); await admin.dispose(); }
});

test('switching accounts clears private profile even when refetch fails', async ({ page }) => {
  await register(page, `account-a-${unique()}@e2e.test`);
  await page.getByRole('link', { name: 'Профиль', exact: true }).click();
  await page.getByLabel('Отображаемое имя').fill('PRIVATE ACCOUNT A');
  await page.getByRole('button', { name: 'Сохранить', exact: true }).click();
  await expect(page.getByText('Профиль сохранён', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Выйти', exact: true }).click();
  await expect(page.locator('.user-email')).toHaveCount(0);
  await page.getByRole('link', { name: 'Начать учиться', exact: true }).click();
  await register(page, `account-b-${unique()}@e2e.test`, false);
  await page.route('**/api/v1/me/profile', route => route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"test outage"}' }));
  await page.getByRole('link', { name: 'Профиль', exact: true }).click();
  await expect(page.getByText('test outage', { exact: true })).toBeVisible();
  expect(await page.locator('input').evaluateAll(nodes => nodes.some(node => node.value === 'PRIVATE ACCOUNT A'))).toBe(false);
});

test('teacher invite link registers a teacher in the browser', async ({ page, playwright }) => {
  const admin = await playwright.request.newContext({ baseURL: origin });
  try {
    await post(admin, '/auth/login', { email: 'admin@e2e.test', password });
    const email = `invited-${unique()}@e2e.test`;
    const invitation = await post(admin, '/admin/invites', { email });
    const link = new URL(invitation.inviteLink);
    expect(link.origin).toBe(origin);
    await page.goto(link.pathname + link.search);
    await page.locator('#register-email').fill(email);
    await page.locator('#register-password').fill(password);
    await page.getByRole('button', { name: 'Зарегистрироваться', exact: true }).click();
    await expect(page).toHaveURL(/\/teach$/);
    const me = await page.request.get('/api/v1/auth/me');
    expect((await me.json()).role).toBe('teacher');
  } finally { await admin.dispose(); }
});

test('teacher creates content and administrator publishes it through the UI', async ({ page, browser }) => {
  const title = 'UI constructor ' + unique();
  await page.goto('/login');
  await page.locator('#login-email').fill('teacher@e2e.test');
  await page.locator('#login-password').fill(password);
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  await expect(page.locator('.user-email')).toHaveText('teacher@e2e.test');
  await page.goto('/teach/new');
  await page.locator('#course-title').fill(title);
  await page.locator('#course-desc').fill('Created and moderated through browser forms');
  await page.locator('#course-price').fill('100');
  await page.getByRole('button', { name: 'Сохранить', exact: true }).click();
  await expect(page).toHaveURL(/\/teach\/[0-9a-f-]{36}$/);
  await page.locator('#module-title').fill('Browser module');
  await page.getByRole('button', { name: 'Добавить модуль', exact: true }).click();
  await expect(page.locator('#lesson-module option').filter({ hasText: 'Browser module' })).toHaveCount(1);
  await page.locator('#lesson-module').selectOption({ label: 'Browser module' });
  await page.locator('#lesson-title').fill('Browser lesson');
  await page.locator('#lesson-kind').selectOption('video_text');
  await page.locator('#lesson-body-md').fill('Lesson created in the UI');
  await page.locator('#lesson-form').getByRole('button', { name: 'Добавить урок', exact: true }).click();
  const submit = page.getByRole('button', { name: 'Отправить на модерацию', exact: true });
  await expect(submit).toBeEnabled();
  page.once('dialog', dialog => dialog.accept());
  const submitted = page.waitForResponse(response => response.url().endsWith('/submit') && response.request().method() === 'POST');
  await submit.click();
  expect((await submitted).ok()).toBeTruthy();
  const adminContext = await browser.newContext({ baseURL: origin });
  try {
    const admin = await adminContext.newPage();
    await admin.goto('/login');
    await admin.locator('#login-email').fill('admin@e2e.test');
    await admin.locator('#login-password').fill(password);
    await admin.getByRole('button', { name: 'Войти', exact: true }).click();
    await expect(admin.locator('.user-email')).toHaveText('admin@e2e.test');
    await admin.goto('/admin/moderation');
    const row = admin.locator('.acn-qcard').filter({ hasText: title });
    await row.getByRole('button', { name: 'Одобрить и опубликовать' }).click();
    // Окно подтверждения: комментарий необязателен.
    await admin.getByRole('dialog').getByRole('button', { name: 'Одобрить и опубликовать' }).click();
    await expect(row).toHaveCount(0);
  } finally { await adminContext.close(); }
});
