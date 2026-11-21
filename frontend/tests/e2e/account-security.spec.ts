import { test, expect, type APIRequestContext, type Page } from '@playwright/test';

const origin = process.env.E2E_BASE_URL ?? 'http://127.0.0.1:13000';
const mailOrigin = process.env.E2E_MAIL_URL ?? 'http://127.0.0.1:13025';
const password = 'initial-password123';
const emailAddress = () => `security-${Date.now()}-${Math.random().toString(36).slice(2)}@e2e.test`;

async function register(page: Page, email: string) {
  await page.goto('/register');
  await page.locator('#register-email').fill(email);
  await page.locator('#register-password').fill(password);
  await page.getByRole('button', { name: 'Зарегистрироваться', exact: true }).click();
  await expect(page.locator('.user-email')).toHaveText(email);
}

async function emailLink(request: APIRequestContext, email: string, path: string): Promise<string> {
  let link = '';
  await expect.poll(async () => {
    const response = await request.get(`${mailOrigin}/messages?to=${encodeURIComponent(email)}`);
    const { items } = await response.json() as { items: { raw: string }[] };
    for (const item of items) {
      for (const candidate of item.raw.match(/https?:\/\/[^\s<>]+/g) ?? []) {
        const url = new URL(candidate);
        if (url.pathname === path) { expect(url.origin).toBe(origin); link = url.pathname + url.hash; }
      }
    }
    return Boolean(link);
  }, { timeout: 10_000 }).toBe(true);
  return link;
}

test('registration email needs explicit confirmation and cannot be reused', async ({ page, request }) => {
  const email = emailAddress();
  await register(page, email);
  const link = await emailLink(request, email, '/verify-email');
  await page.goto(link);
  await expect(page.getByRole('heading', { name: 'Подтверждение email' })).toBeVisible();
  expect((await (await page.request.get('/api/v1/me/security')).json()).emailVerified).toBe(false);
  await expect(page).toHaveURL(origin + '/verify-email');
  await page.getByRole('button', { name: 'Подтвердить email', exact: true }).click();
  await expect(page.getByRole('status')).toContainText('Email подтверждён.');
  await page.getByRole('link', { name: 'Перейти в профиль', exact: true }).click();
  await expect(page.getByText('Подтверждён', { exact: true })).toBeVisible();
  await page.goto(link);
  await page.getByRole('button', { name: 'Подтвердить email', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('уже использована');
});

test('password recovery sends email, changes the password and logs out all devices', async ({ page, request, playwright }) => {
  const email = emailAddress();
  await register(page, email);
  const other = await playwright.request.newContext({ baseURL: origin });
  try {
    const login = await other.post('/api/v1/auth/login', { data: { email, password }, headers: { Origin: origin } });
    expect(login.ok()).toBe(true);
    await page.goto('/forgot-password');
    await page.getByLabel('Email', { exact: true }).fill(email);
    await page.getByRole('button', { name: 'Отправить ссылку', exact: true }).click();
    await expect(page.getByRole('status')).toContainText('Если аккаунт');
    const link = await emailLink(request, email, '/reset-password');
    await page.goto(link);
    await expect(page.getByRole('heading', { name: 'Новый пароль', exact: true })).toBeVisible();
    await page.getByLabel('Новый пароль', { exact: true }).fill('changed-password123');
    await page.getByLabel('Повторите пароль').fill('not-matching-password');
    await page.getByRole('button', { name: 'Сохранить новый пароль' }).click();
    await expect(page.getByRole('alert')).toContainText('Пароли не совпадают');
    await page.getByLabel('Повторите пароль').fill('changed-password123');
    await page.getByRole('button', { name: 'Сохранить новый пароль' }).click();
    await expect(page.getByRole('status')).toContainText('Пароль изменён');
    expect((await other.get('/api/v1/auth/me')).status()).toBe(401);
    expect((await page.request.get('/api/v1/auth/me')).status()).toBe(401);
    const old = await other.post('/api/v1/auth/login', { data: { email, password }, headers: { Origin: origin } });
    expect(old.status()).toBe(401);
    await page.locator('#content').getByRole('link', { name: 'Войти', exact: true }).click();
    await page.locator('#login-email').fill(email);
    await page.locator('#login-password').fill('changed-password123');
    await page.getByRole('button', { name: 'Войти', exact: true }).click();
    await expect(page.locator('.user-email')).toHaveText(email);
  } finally { await other.dispose(); }
});

test('profile revokes another device, then the current session', async ({ page, playwright }) => {
	await page.setViewportSize({ width: 390, height: 844 });
  const email = emailAddress();
  await register(page, email);
  const other = await playwright.request.newContext({ baseURL: origin });
  try {
    expect((await other.post('/api/v1/auth/login', { data: { email, password }, headers: { Origin: origin } })).ok()).toBe(true);
    await page.goto('/profile');
    await expect(page.locator('.account-session')).toHaveCount(2);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await expect(page.getByText('Текущая сессия', { exact: true })).toHaveCount(1);
    await page.getByRole('button', { name: 'Завершить сессию', exact: true }).click();
    await expect(page.locator('.account-session')).toHaveCount(1);
    expect((await other.get('/api/v1/auth/me')).status()).toBe(401);
    expect((await other.post('/api/v1/auth/login', { data: { email, password }, headers: { Origin: origin } })).ok()).toBe(true);
    await page.reload();
    await page.getByRole('button', { name: 'Завершить другие сессии', exact: true }).click();
    await expect(page.locator('.account-session')).toHaveCount(1);
    expect((await other.get('/api/v1/auth/me')).status()).toBe(401);
    await page.getByRole('button', { name: 'Выйти с этого устройства', exact: true }).click();
    await expect(page.locator('#login-email')).toBeVisible();
    expect((await page.request.get('/api/v1/auth/me')).status()).toBe(401);
  } finally { await other.dispose(); }
});
