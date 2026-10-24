// T94: контрактные тесты API-клиента активных сессий пользователя (P10/T93).
//
// adminConsole.ts не входит в SSR-сборку client.ts (test:build собирает только
// client.ts), поэтому модуль загружается через Vite SSR. Это тот же
// TypeScript-исходник и тот же import.meta.env.VITE_API_URL из .env.test, что
// и в приложении; package.json и client.ts при этом не меняются.

import { after, before, beforeEach, test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import { startMockApi, lastRequest } from './helper.mjs';

let api;
let vite;
let adminConsole;

before(async () => {
  api = await startMockApi();
  vite = await createServer({
    configFile: false,
    root: fileURLToPath(new URL('../..', import.meta.url)),
    mode: 'test',
    server: { middlewareMode: true },
    appType: 'custom',
    logLevel: 'silent',
  });
  adminConsole = await vite.ssrLoadModule('/src/api/adminConsole.ts');
});

beforeEach(() => {
  api.requests.length = 0;
});

after(async () => {
  if (vite) await vite.close();
  if (api) await api.close();
});

/** Сессия по контракту T93: id, createdAt, expiresAt. */
function sessionFixture(overrides = {}) {
  return {
    id: 'sess-1',
    createdAt: '2026-10-20T10:00:00Z',
    expiresAt: '2026-11-20T10:00:00Z',
    ...overrides,
  };
}

// ---------- listAdminUserSessions ----------

test('listAdminUserSessions: GET /admin/users/{id}/sessions, {items} как есть', async () => {
  const items = [
    sessionFixture(),
    sessionFixture({ id: 'sess-2', createdAt: '2026-10-19T10:00:00Z' }),
  ];
  api.mockReply({ json: { items } });

  const res = await adminConsole.listAdminUserSessions('u1');

  assert.deepEqual(res, { items });
  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/admin/users/u1/sessions');
  assert.equal(req.query.toString(), '');
  assert.equal(req.rawBody, '');
});

test('listAdminUserSessions: пустой список — {items: []}', async () => {
  api.mockReply({ json: { items: [] } });

  const res = await adminConsole.listAdminUserSessions('u1');

  assert.deepEqual(res, { items: [] });
  assert.equal(lastRequest(api).path, '/api/v1/admin/users/u1/sessions');
});

test('listAdminUserSessions: id пользователя кодируется в пути', async () => {
  api.mockReply({ json: { items: [] } });

  await adminConsole.listAdminUserSessions('u 1/2');

  assert.equal(lastRequest(api).path, '/api/v1/admin/users/u%201%2F2/sessions');
});

test('listAdminUserSessions: 404 пользователя — ApiError со статусом и текстом', async () => {
  api.mockReply({ status: 404, json: { error: 'Пользователь не найден' } });

  await assert.rejects(adminConsole.listAdminUserSessions('nope'), (err) => {
    assert.equal(err.status, 404);
    assert.equal(err.message, 'Пользователь не найден');
    return true;
  });
});

test('listAdminUserSessions: 401 и 403 сохраняют статус сервера', async () => {
  api.mockReply({ status: 401, json: { error: 'Требуется вход в аккаунт' } });
  await assert.rejects(adminConsole.listAdminUserSessions('u1'), (err) => err.status === 401);

  api.requests.length = 0;
  api.mockReply({ status: 403, json: { error: 'Недостаточно прав' } });
  await assert.rejects(adminConsole.listAdminUserSessions('u1'), (err) => err.status === 403);
});

// ---------- revokeAdminUserSessions ----------

test('revokeAdminUserSessions: DELETE /admin/users/{id}/sessions, 204 → undefined', async () => {
  api.mockReply({ status: 204 });

  const res = await adminConsole.revokeAdminUserSessions('u1');

  assert.equal(res, undefined);
  const req = lastRequest(api);
  assert.equal(req.method, 'DELETE');
  assert.equal(req.path, '/api/v1/admin/users/u1/sessions');
  // apiFetch всегда ставит Content-Type: application/json, но тела у DELETE нет.
  assert.equal(req.rawBody, '');
});

test('revokeAdminUserSessions: id пользователя кодируется в пути', async () => {
  api.mockReply({ status: 204 });

  await adminConsole.revokeAdminUserSessions('u 1/2');

  assert.equal(lastRequest(api).path, '/api/v1/admin/users/u%201%2F2/sessions');
});

test('revokeAdminUserSessions: 404 пользователя — ApiError со статусом и текстом', async () => {
  api.mockReply({ status: 404, json: { error: 'Пользователь не найден' } });

  await assert.rejects(adminConsole.revokeAdminUserSessions('nope'), (err) => {
    assert.equal(err.status, 404);
    assert.equal(err.message, 'Пользователь не найден');
    return true;
  });
});

// ---------- Типы ответа ----------

test('AdminSession: в ответе ровно id, createdAt, expiresAt', async () => {
  const session = sessionFixture();
  api.mockReply({ json: { items: [session] } });

  const res = await adminConsole.listAdminUserSessions('u1');

  assert.deepEqual(Object.keys(res.items[0]).sort(), ['createdAt', 'expiresAt', 'id']);
  assert.equal(typeof res.items[0].id, 'string');
  assert.equal(typeof res.items[0].createdAt, 'string');
  assert.equal(typeof res.items[0].expiresAt, 'string');
});
