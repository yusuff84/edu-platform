// T64: контрактные тесты промокодов (P8/T63) в API-клиенте.
//
// Покрыто: createPromoCode/listPromoCodes/deactivatePromoCode,
// validatePromoCode и обратная совместимость enroll(courseId, provider).
// Тела запросов проверяются точно: backend отвечает 400 на любое лишнее поле
// (DisallowUnknownFields), а необязательные maxUses/expiresAt не должны
// уезжать пустыми.

import { after, before, beforeEach, test } from 'node:test';
import assert from 'node:assert/strict';
import { startMockApi, lastRequest, jsonBody, expectBodyKeys } from './helper.mjs';
import {
  createPromoCode,
  deactivatePromoCode,
  enroll,
  listPromoCodes,
  validatePromoCode,
} from '../../.tmp-api-test/client.js';

let api;

before(async () => {
  api = await startMockApi();
});

beforeEach(() => {
  api.requests.length = 0;
});

after(async () => {
  await api.close();
});

/** Модель PromoCode по контракту P8 (T63). */
function promoFixture(overrides = {}) {
  return {
    id: 'p1',
    courseId: 'c1',
    code: 'SALE20',
    discountPct: 20,
    maxUses: 100,
    usedCount: 0,
    expiresAt: '2026-12-31T23:59:59Z',
    active: true,
    createdAt: '2026-10-19T12:00:00Z',
    ...overrides,
  };
}

/** Результат POST /enrollments (существующая форма T01). */
function enrollmentFixture(overrides = {}) {
  return {
    paymentId: 'pay1',
    providerPaymentId: 'mock-pay1',
    redirectUrl: 'https://pay.example/1',
    amountCents: 8000,
    status: 'pending',
    enrollmentActive: false,
    ...overrides,
  };
}

// ---------- createPromoCode ----------

test('createPromoCode: POST /teacher/courses/{id}/promo-codes, конверт {promoCode} распакован', async () => {
  const promo = promoFixture();
  api.mockReply({ status: 201, json: { promoCode: promo } });
  const res = await createPromoCode('c1', {
    code: 'SALE20',
    discountPct: 20,
    maxUses: 100,
    expiresAt: '2026-12-31T23:59:59Z',
  });
  assert.deepEqual(res, promo);
  const req = lastRequest(api);
  assert.equal(req.method, 'POST');
  assert.equal(req.path, '/api/v1/teacher/courses/c1/promo-codes');
  assert.equal(req.headers['content-type'], 'application/json');
  assert.deepEqual(
    expectBodyKeys(jsonBody(req), ['code', 'discountPct', 'maxUses', 'expiresAt']),
    {
      code: 'SALE20',
      discountPct: 20,
      maxUses: 100,
      expiresAt: '2026-12-31T23:59:59Z',
    },
  );
});

test('createPromoCode: без maxUses/expiresAt в тело уходят только code+discountPct', async () => {
  api.mockReply({
    status: 201,
    json: { promoCode: promoFixture({ maxUses: 0, expiresAt: null }) },
  });
  await createPromoCode('c1', { code: 'NOEXP', discountPct: 15 });
  assert.deepEqual(expectBodyKeys(jsonBody(lastRequest(api)), ['code', 'discountPct']), {
    code: 'NOEXP',
    discountPct: 15,
  });
});

test('createPromoCode: expiresAt=null не отправляется (RFC3339 в теле невалиден)', async () => {
  api.mockReply({
    status: 201,
    json: { promoCode: promoFixture({ maxUses: 0, expiresAt: null }) },
  });
  await createPromoCode('c1', { code: 'NOEXP', discountPct: 15, expiresAt: null });
  assert.equal('expiresAt' in jsonBody(lastRequest(api)), false);
});

test('createPromoCode: 400 дубликата → статус и текст сервера', async () => {
  api.mockReply({ status: 400, json: { error: 'Такой промокод уже существует на этом курсе' } });
  await assert.rejects(
    createPromoCode('c1', { code: 'SALE20', discountPct: 20 }),
    (err) => {
      assert.ok(err instanceof Error);
      assert.equal(err.status, 400);
      assert.equal(err.message, 'Такой промокод уже существует на этом курсе');
      return true;
    },
  );
});

test('createPromoCode: 409 архивного курса → текст сервера', async () => {
  api.mockReply({ status: 409, json: { error: 'Курс в архиве: промокоды недоступны' } });
  await assert.rejects(
    createPromoCode('c1', { code: 'SALE20', discountPct: 20 }),
    (err) => {
      assert.equal(err.status, 409);
      assert.equal(err.message, 'Курс в архиве: промокоды недоступны');
      return true;
    },
  );
});

// ---------- listPromoCodes ----------

test('listPromoCodes: GET /teacher/courses/{id}/promo-codes, {items} → массив', async () => {
  const items = [promoFixture(), promoFixture({ id: 'p2', code: 'OLD10', active: false })];
  api.mockReply({ json: { items } });
  const res = await listPromoCodes('c1');
  assert.deepEqual(res, items);
  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/teacher/courses/c1/promo-codes');
  assert.equal(req.query.toString(), '');
  assert.equal(req.rawBody, '');
});

test('listPromoCodes: пустой список — пустой массив', async () => {
  api.mockReply({ json: { items: [] } });
  assert.deepEqual(await listPromoCodes('c1'), []);
});

// ---------- deactivatePromoCode ----------

test('deactivatePromoCode: DELETE .../promo-codes/{promoId}, 204 → undefined', async () => {
  api.mockReply({ status: 204 });
  const res = await deactivatePromoCode('c1', 'p1');
  assert.equal(res, undefined);
  const req = lastRequest(api);
  assert.equal(req.method, 'DELETE');
  assert.equal(req.path, '/api/v1/teacher/courses/c1/promo-codes/p1');
  // apiFetch всегда ставит Content-Type: application/json (общий транспорт),
  // но тела у DELETE нет — backend его не читает.
  assert.equal(req.rawBody, '');
});

test('deactivatePromoCode: повторный вызов идемпотентен (204)', async () => {
  api.mockReply({ status: 204 });
  await assert.doesNotReject(deactivatePromoCode('c1', 'p1'));
  await assert.doesNotReject(deactivatePromoCode('c1', 'p1'));
});

test('deactivatePromoCode: 404 → ошибка со статусом и текстом', async () => {
  api.mockReply({ status: 404, json: { error: 'Промокод не найден' } });
  await assert.rejects(deactivatePromoCode('c1', 'nope'), (err) => {
    assert.equal(err.status, 404);
    assert.equal(err.message, 'Промокод не найден');
    return true;
  });
});

// ---------- validatePromoCode ----------

test('validatePromoCode: POST /courses/{id}/promo/validate, тело строго {code}', async () => {
  const validation = {
    code: 'SALE20',
    discountPct: 20,
    priceCents: 10000,
    finalPriceCents: 8000,
  };
  api.mockReply({ json: validation });
  const res = await validatePromoCode('c1', 'SALE20');
  assert.deepEqual(res, validation);
  const req = lastRequest(api);
  assert.equal(req.method, 'POST');
  assert.equal(req.path, '/api/v1/courses/c1/promo/validate');
  assert.deepEqual(expectBodyKeys(jsonBody(req), ['code']), { code: 'SALE20' });
});

test('validatePromoCode: 400 «Промокод истёк» → текст сервера как есть', async () => {
  api.mockReply({ status: 400, json: { error: 'Промокод истёк' } });
  await assert.rejects(validatePromoCode('c1', 'OLD10'), (err) => {
    assert.equal(err.status, 400);
    assert.equal(err.message, 'Промокод истёк');
    return true;
  });
});

test('validatePromoCode: 400 «Курс бесплатный — промокод не нужен»', async () => {
  api.mockReply({ status: 400, json: { error: 'Курс бесплатный — промокод не нужен' } });
  await assert.rejects(validatePromoCode('free', 'SALE20'), (err) => {
    assert.equal(err.status, 400);
    assert.equal(err.message, 'Курс бесплатный — промокод не нужен');
    return true;
  });
});

// ---------- enroll: promoCode вторым аргументом, provider не передаём ----------

test('enroll: (courseId, promoCode) — в теле ровно courseId+promoCode, без provider', async () => {
  const result = enrollmentFixture();
  api.mockReply({ json: result });
  const res = await enroll('c1', 'SALE20');
  assert.deepEqual(res, result);
  const req = lastRequest(api);
  assert.equal(req.method, 'POST');
  assert.equal(req.path, '/api/v1/enrollments');
  assert.deepEqual(expectBodyKeys(jsonBody(req), ['courseId', 'promoCode']), {
    courseId: 'c1',
    promoCode: 'SALE20',
  });
  assert.equal('provider' in jsonBody(req), false);
});

test('enroll: без промокода в теле только courseId', async () => {
  api.mockReply({ json: enrollmentFixture() });
  await enroll('c1');
  assert.deepEqual(jsonBody(lastRequest(api)), { courseId: 'c1' });
});

test('enroll: promoCode передаётся вторым параметром', async () => {
  const result = enrollmentFixture({ amountCents: 8000, promoCode: 'SALE20', discountCents: 2000 });
  api.mockReply({ json: result });
  const res = await enroll('c1', 'SALE20');
  assert.deepEqual(res, result);
  assert.deepEqual(
    expectBodyKeys(jsonBody(lastRequest(api)), ['courseId', 'promoCode']),
    { courseId: 'c1', promoCode: 'SALE20' },
  );
});

test('enroll: пробелы вокруг промокода обрезаются', async () => {
  api.mockReply({ json: enrollmentFixture() });
  await enroll('c1', '  SALE20  ');
  assert.equal(jsonBody(lastRequest(api)).promoCode, 'SALE20');
});

test('enroll: пустой promoCode не отправляется (бесплатный курс не ломаем)', async () => {
  api.mockReply({ json: enrollmentFixture({ amountCents: 0, enrollmentActive: true }) });
  await enroll('c1', '');
  assert.equal('promoCode' in jsonBody(lastRequest(api)), false);

  api.requests.length = 0;
  api.mockReply({ json: enrollmentFixture({ amountCents: 0, enrollmentActive: true }) });
  await enroll('free', '   ');
  assert.deepEqual(jsonBody(lastRequest(api)), { courseId: 'free' });
});

test('enroll: 400 по промокоду → ошибка со статусом и текстом сервера', async () => {
  api.mockReply({ status: 400, json: { error: 'Лимит использований промокода исчерпан' } });
  await assert.rejects(enroll('c1', 'USEDUP'), (err) => {
    assert.equal(err.status, 400);
    assert.equal(err.message, 'Лимит использований промокода исчерпан');
    return true;
  });
});
