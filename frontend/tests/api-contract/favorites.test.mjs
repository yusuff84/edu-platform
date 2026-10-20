// T74: контрактные тесты клиента избранного (P9/T73).
//
// Покрыто: addFavorite (PUT, 204, без тела, URL-кодирование, 401/404),
// removeFavorite (DELETE, 204 без тела, идемпотентность, 401)
// и myFavorites (GET, конверт {items,total} → CourseItem[], порядок, 401).
// Сервер — мини-API из helper.mjs на фиксированном 127.0.0.1:8799.

import { after, before, beforeEach, test } from 'node:test';
import assert from 'node:assert/strict';
import { startMockApi, lastRequest } from './helper.mjs';
import {
  ApiError,
  addFavorite,
  myFavorites,
  removeFavorite,
} from '../../.tmp-api-test/client.js';

let api;

/** Карточка курса по контракту CourseItem (GET /me/favorites → items). */
function courseFixture(overrides = {}) {
  return {
    id: 'c1',
    slug: 'python-basics',
    title: 'Python с нуля',
    description: 'Базовый курс по программированию',
    priceCents: 990000,
    currency: 'RUB',
    avgRating: 4.8,
    ratingsCount: 12,
    category: 'programming',
    coverPath: '/uploads/covers/python.png',
    isFavorite: true,
    ...overrides,
  };
}

before(async () => {
  api = await startMockApi();
});

beforeEach(() => {
  api.requests.length = 0;
});

after(async () => {
  await api.close();
});

/** Проверяет, что промис отклонён ApiError с нужными статусом и текстом. */
async function expectApiError(promise, status, message) {
  await assert.rejects(promise, (err) => {
    assert.ok(err instanceof ApiError, 'ожидалась ApiError');
    assert.equal(err.status, status);
    assert.equal(err.message, message);
    return true;
  });
}

// ---------- addFavorite: PUT ----------

test('addFavorite: PUT /me/favorites/{id}, 204 → undefined, без тела', async () => {
  api.mockReply({ status: 204 });
  const res = await addFavorite('c1');
  assert.equal(res, undefined);

  const req = lastRequest(api);
  assert.equal(req.method, 'PUT');
  assert.equal(req.path, '/api/v1/me/favorites/c1');
  assert.equal(req.rawBody, '', 'у PUT избранного тела быть не должно');
  assert.equal(req.query.toString(), '');
});

test('addFavorite: id курса кодируется целиком (encodeURIComponent)', async () => {
  api.mockReply({ status: 204 });
  await addFavorite('c/1');
  assert.equal(lastRequest(api).path, '/api/v1/me/favorites/c%2F1');
});

test('addFavorite: повторный вызов идемпотентен (204)', async () => {
  api.mockReply({ status: 204 });
  await assert.doesNotReject(addFavorite('c1'));
  await assert.doesNotReject(addFavorite('c1'));
  assert.equal(api.requests.length, 2);
});

test('addFavorite: 401 (гость) → ApiError 401 с текстом сервера', async () => {
  api.mockReply({ status: 401, json: { error: 'Требуется вход в аккаунт' } });
  await expectApiError(addFavorite('c1'), 401, 'Требуется вход в аккаунт');
});

test('addFavorite: 404 (курс не published/не найден) → ApiError 404', async () => {
  api.mockReply({ status: 404, json: { error: 'Курс не найден' } });
  await expectApiError(addFavorite('draft'), 404, 'Курс не найден');
});

// ---------- removeFavorite: DELETE ----------

test('removeFavorite: DELETE /me/favorites/{id}, 204 → undefined, без тела', async () => {
  api.mockReply({ status: 204 });
  const res = await removeFavorite('c1');
  assert.equal(res, undefined);

  const req = lastRequest(api);
  assert.equal(req.method, 'DELETE');
  assert.equal(req.path, '/api/v1/me/favorites/c1');
  assert.equal(req.rawBody, '', 'у DELETE тела быть не должно');
});

test('removeFavorite: id курса кодируется целиком', async () => {
  api.mockReply({ status: 204 });
  await removeFavorite('c/1');
  assert.equal(lastRequest(api).path, '/api/v1/me/favorites/c%2F1');
});

test('removeFavorite: идемпотентность — 204 и при повторном удалении', async () => {
  api.mockReply({ status: 204 });
  await assert.doesNotReject(removeFavorite('c1'));
  await assert.doesNotReject(removeFavorite('c1'));
  assert.equal(api.requests.length, 2);
});

test('removeFavorite: 401 (гость) → ApiError 401', async () => {
  api.mockReply({ status: 401, json: { error: 'Требуется вход в аккаунт' } });
  await expectApiError(removeFavorite('c1'), 401, 'Требуется вход в аккаунт');
});

test('removeFavorite: 401 без envelope → запасной текст клиента', async () => {
  api.mockReply({ status: 401 });
  await expectApiError(removeFavorite('c1'), 401, 'Требуется вход в аккаунт');
});

// ---------- myFavorites: GET ----------

test('myFavorites: GET /me/favorites, конверт {items,total} → массив', async () => {
  const items = [
    courseFixture({ id: 'c2', slug: 'sql-start', title: 'SQL с нуля' }),
    courseFixture(),
  ];
  api.mockReply({ json: { items, total: 2 } });
  const res = await myFavorites();
  // Возвращается именно массив CourseItem[], без конверта и total.
  assert.deepEqual(res, items);
  assert.equal(Array.isArray(res), true);
  assert.equal(res.length, 2);
  assert.equal('total' in res, false);
  assert.deepEqual(
    res.map((c) => c.id),
    ['c2', 'c1'],
    'порядок сервера (добавленные позже первыми) сохраняется',
  );

  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/me/favorites');
  assert.equal(req.query.toString(), '');
  assert.equal(req.rawBody, '');
});

test('myFavorites: пустой список — пустой массив', async () => {
  api.mockReply({ json: { items: [], total: 0 } });
  assert.deepEqual(await myFavorites(), []);
});

test('myFavorites: 401 (гость) → ApiError 401', async () => {
  api.mockReply({ status: 401, json: { error: 'Требуется вход в аккаунт' } });
  await expectApiError(myFavorites(), 401, 'Требуется вход в аккаунт');
});
