// T90: контрактные тесты глобального журнала промокодов (T89) в API-клиенте.
//
// Файл автономен: api/adminConsole.ts не входит в тестовый бандл client.ts
// (npm run test:build собирает только src/api/client.ts), поэтому здесь он
// достраивается в свой outDir (.tmp-api-test/admin-console) той же командой
// vite, что и test:build. package.json не меняется, outDir лежит в gitignore
// и не пересекается с client.js параллельных тест-файлов.
//
// Покрыто: query string listAdminPromoCodes (включая active=false и пустые
// параметры), PATCH-тело updateAdminPromoCode, явный expiresAt=null, конверт
// {promoCode} и тексты ошибок 400/404/409.

import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { after, before, beforeEach, test } from 'node:test';
import assert from 'node:assert/strict';
import { startMockApi, lastRequest, jsonBody, expectBodyKeys } from './helper.mjs';

const FRONTEND_ROOT = fileURLToPath(new URL('../..', import.meta.url));
const ADMIN_OUT_DIR = '.tmp-api-test/admin-console';

let api;
let adminConsole; // собранный модуль adminConsole.js

/** Собирает api/adminConsole.ts обычной тестовой сборкой vite (--mode test). */
async function buildAdminConsoleModule() {
  const viteBin = fileURLToPath(
    new URL('../../node_modules/vite/bin/vite.js', import.meta.url),
  );
  const run = spawnSync(
    process.execPath,
    [
      viteBin,
      'build',
      '--ssr',
      'src/api/adminConsole.ts',
      '--outDir',
      ADMIN_OUT_DIR,
      '--emptyOutDir',
      '--mode',
      'test',
    ],
    { cwd: FRONTEND_ROOT, encoding: 'utf8' },
  );
  assert.equal(
    run.status,
    0,
    `сборка adminConsole.ts провалилась:\n${run.stdout ?? ''}\n${run.stderr ?? ''}`,
  );
  // Клиент собран с VITE_API_URL из .env.test — тот же адрес, что и у helper.
  return import(
    new URL(`../../${ADMIN_OUT_DIR}/adminConsole.js`, import.meta.url).href
  );
}

before(async () => {
  api = await startMockApi();
  adminConsole = await buildAdminConsoleModule();
});

beforeEach(() => {
  api.requests.length = 0;
});

after(async () => {
  await api.close();
});

/** Элемент списка GET /admin/promo-codes по контракту T89. */
function promoFixture(overrides = {}) {
  return {
    id: 'p1',
    courseId: 'c1',
    courseTitle: 'Python с нуля',
    courseSlug: 'python-from-zero',
    code: 'SALE20',
    discountPct: 20,
    maxUses: 100,
    usedCount: 3,
    expiresAt: null,
    active: true,
    createdAt: '2026-10-24T10:00:00Z',
    ...overrides,
  };
}

// ---------- listAdminPromoCodes ----------

test('listAdminPromoCodes: GET /admin/promo-codes без параметров', async () => {
  const promo = promoFixture();
  api.mockReply({ json: { items: [promo], total: 1, page: 1, limit: 20 } });
  const res = await adminConsole.listAdminPromoCodes();
  assert.equal(res.total, 1);
  assert.equal(res.page, 1);
  assert.equal(res.limit, 20);
  assert.deepEqual(res.items, [promo]);
  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/admin/promo-codes');
  assert.equal(req.query.toString(), '');
  assert.equal(req.rawBody, '');
});

test('listAdminPromoCodes: courseId/q/active=false/page/limit уходят в query', async () => {
  api.mockReply({
    json: { items: [promoFixture({ active: false })], total: 21, page: 2, limit: 50 },
  });
  const res = await adminConsole.listAdminPromoCodes({
    courseId: 'c1',
    q: 'sale',
    active: false,
    page: 2,
    limit: 50,
  });
  assert.equal(res.items[0].active, false);
  const req = lastRequest(api);
  assert.equal(req.path, '/api/v1/admin/promo-codes');
  assert.equal(req.query.get('courseId'), 'c1');
  assert.equal(req.query.get('q'), 'sale');
  // active=false не должен потеряться: buildQuery пропускает только ''.
  assert.equal(req.query.get('active'), 'false');
  assert.equal(req.query.get('page'), '2');
  assert.equal(req.query.get('limit'), '50');
  assert.deepEqual(
    [...req.query.keys()].sort(),
    ['active', 'courseId', 'limit', 'page', 'q'],
  );
});

test('listAdminPromoCodes: active=true и expiresAt=null сохраняются', async () => {
  const promo = promoFixture({ expiresAt: null, active: true });
  api.mockReply({ json: { items: [promo], total: 1, page: 1, limit: 20 } });
  const res = await adminConsole.listAdminPromoCodes({ active: true });
  assert.equal(res.items[0].expiresAt, null);
  assert.equal(res.items[0].active, true);
  const req = lastRequest(api);
  assert.equal(req.query.get('active'), 'true');
  assert.deepEqual([...req.query.keys()], ['active']);
});

test('listAdminPromoCodes: пустой список — items пустой массив', async () => {
  api.mockReply({ json: { items: [], total: 0, page: 1, limit: 20 } });
  const res = await adminConsole.listAdminPromoCodes({ q: 'нет-такого' });
  assert.deepEqual(res.items, []);
  assert.equal(res.total, 0);
});

// ---------- updateAdminPromoCode ----------

test('updateAdminPromoCode: PATCH со всеми полями, ответ {promoCode} распакован', async () => {
  const updated = promoFixture({
    discountPct: 25,
    maxUses: 200,
    expiresAt: '2026-12-31T20:59:59Z',
    active: false,
  });
  api.mockReply({ json: { promoCode: updated } });
  const res = await adminConsole.updateAdminPromoCode('p1', {
    discountPct: 25,
    maxUses: 200,
    expiresAt: '2026-12-31T20:59:59Z',
    active: false,
  });
  assert.deepEqual(res, updated);
  const req = lastRequest(api);
  assert.equal(req.method, 'PATCH');
  assert.equal(req.path, '/api/v1/admin/promo-codes/p1');
  assert.equal(req.headers['content-type'], 'application/json');
  assert.deepEqual(
    expectBodyKeys(jsonBody(req), ['discountPct', 'maxUses', 'expiresAt', 'active']),
    {
      discountPct: 25,
      maxUses: 200,
      expiresAt: '2026-12-31T20:59:59Z',
      active: false,
    },
  );
});

test('updateAdminPromoCode: expiresAt=null отправляется явно (снять срок)', async () => {
  api.mockReply({ json: { promoCode: promoFixture({ expiresAt: null }) } });
  await adminConsole.updateAdminPromoCode('p1', { expiresAt: null });
  const body = expectBodyKeys(jsonBody(lastRequest(api)), ['expiresAt']);
  assert.equal(body.expiresAt, null);
  assert.equal('expiresAt' in body, true);
});

test('updateAdminPromoCode: частичное тело — только переданные поля', async () => {
  api.mockReply({ json: { promoCode: promoFixture({ active: false }) } });
  await adminConsole.updateAdminPromoCode('p1', { active: false });
  assert.deepEqual(expectBodyKeys(jsonBody(lastRequest(api)), ['active']), {
    active: false,
  });
});

test('updateAdminPromoCode: id кодируется в пути', async () => {
  api.mockReply({ json: { promoCode: promoFixture() } });
  await adminConsole.updateAdminPromoCode('p/1', { active: true });
  assert.equal(lastRequest(api).path, '/api/v1/admin/promo-codes/p%2F1');
});

test('updateAdminPromoCode: 400 неизвестного поля — статус и текст сервера', async () => {
  api.mockReply({ status: 400, json: { error: 'Неизвестное поле promo' } });
  await assert.rejects(
    adminConsole.updateAdminPromoCode('p1', { active: true }),
    (err) => {
      assert.ok(err instanceof Error);
      assert.equal(err.status, 400);
      assert.equal(err.message, 'Неизвестное поле promo');
      return true;
    },
  );
});

test('updateAdminPromoCode: 404 — промокод не найден', async () => {
  api.mockReply({ status: 404, json: { error: 'Промокод не найден' } });
  await assert.rejects(adminConsole.updateAdminPromoCode('nope', { active: true }), (err) => {
    assert.equal(err.status, 404);
    assert.equal(err.message, 'Промокод не найден');
    return true;
  });
});

test('updateAdminPromoCode: 409 архивного курса — текст сервера', async () => {
  api.mockReply({
    status: 409,
    json: { error: 'Курс в архиве: промокоды недоступны' },
  });
  await assert.rejects(
    adminConsole.updateAdminPromoCode('p1', { discountPct: 10 }),
    (err) => {
      assert.equal(err.status, 409);
      assert.equal(err.message, 'Курс в архиве: промокоды недоступны');
      return true;
    },
  );
});

test('listAdminPromoCodes: 403 — статус и текст сервера', async () => {
  api.mockReply({ status: 403, json: { error: 'Недостаточно прав' } });
  await assert.rejects(adminConsole.listAdminPromoCodes(), (err) => {
    assert.equal(err.status, 403);
    assert.equal(err.message, 'Недостаточно прав');
    return true;
  });
});
