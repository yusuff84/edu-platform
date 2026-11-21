import { defineConfig, devices } from '@playwright/test';

const baseURL = process.env.E2E_BASE_URL ?? 'http://127.0.0.1:13000';
export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: false,
  workers: 1,
  timeout: 45_000,
  use: { baseURL, trace: 'retain-on-failure' },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: process.env.E2E_BASE_URL ? undefined : [
    {
      command: 'node tests/e2e/mail-server.mjs',
      url: 'http://127.0.0.1:13025/healthz', reuseExistingServer: false,
    },
    {
      command: 'go run ./cmd/api', cwd: '../backend',
      url: 'http://127.0.0.1:18082/healthz', reuseExistingServer: false,
      env: {
        APP_ENV: 'test', DATABASE_URL: '', PORT: '18082',
        SESSION_SECRET: 'browser-e2e-local-secret', PAYMENTS_PROVIDER: 'mock',
        ADMIN_EMAIL: 'admin@e2e.test', ADMIN_PASSWORD: 'e2e-password123',
        TEACHER_EMAIL: 'teacher@e2e.test', TEACHER_PASSWORD: 'e2e-password123',
        SMTP_ADDR: '127.0.0.1:11025', SMTP_FROM: 'support@e2e.test', SMTP_TLS: 'none', SMTP_USERNAME: '', SMTP_PASSWORD: '', YANDEX_CLIENT_ID: '', YANDEX_CLIENT_SECRET: '',
        MOCK_WEBHOOK_SECRET: '', ALLOWED_ORIGINS: baseURL,
        RATE_LIMIT_AUTH_PER_MIN: '0', RATE_LIMIT_GLOBAL_PER_MIN: '0',
        PAYMENTS_RETURN_URL: baseURL + '/me', PUBLIC_APP_URL: baseURL,
      },
    },
    {
      command: 'npm run dev -- --host 127.0.0.1 --port 13000 --strictPort',
      url: baseURL, reuseExistingServer: false,
      env: { VITE_API_URL: '/api/v1', VITE_API_PROXY: 'http://127.0.0.1:18082' },
    },
  ],
});
