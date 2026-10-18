// Контрактные тесты транспорта API-клиента: apiFetch, buildQuery и чистые утилиты.
// Запуск: npm test (см. frontend/package.json). Сервер — мини-API из helper.mjs.

import { after, before, beforeEach, test } from 'node:test';
import assert from 'node:assert/strict';
import { startMockApi, lastRequest, jsonBody, TEST_BASE } from './helper.mjs';
import {
  ApiError,
  apiFetch,
  buildQuery,
  formatDate,
  formatMoney,
  formatPrice,
  useApiBase,
} from '../../.tmp-api-test/client.js';

let api;

before(async () => {
  api = await startMockApi();
});

beforeEach(() => {
  // каждый тест начинает с чистого журнала запросов
  api.requests.length = 0;
});

after(async () => {
  await api.close();
});

// ---------- apiFetch: успех ----------

test('apiFetch: GET по умолчанию, JSON разбирается, есть Content-Type: application/json', async () => {
  api.mockReply({ json: { ok: true, nested: { n: 1 } } });
  const res = await apiFetch('/ping');
  assert.deepEqual(res, { ok: true, nested: { n: 1 } });
  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/ping');
  assert.equal(req.query.toString(), '');
  assert.equal(req.headers['content-type'], 'application/json');
});

test('apiFetch: POST передаёт метод, тело и заголовок Content-Type: application/json', async () => {
  api.mockReply({ json: { saved: 1 } });
  const res = await apiFetch('/echo', { method: 'POST', body: JSON.stringify({ a: 1 }) });
  assert.deepEqual(res, { saved: 1 });
  const req = lastRequest(api);
  assert.equal(req.method, 'POST');
  assert.equal(req.path, '/api/v1/echo');
  assert.equal(req.headers['content-type'], 'application/json');
  assert.deepEqual(jsonBody(req), { a: 1 });
});

test('apiFetch: PATCH — тоже Content-Type: application/json', async () => {
  api.mockReply({ json: { patched: true } });
  await apiFetch('/echo', { method: 'PATCH', body: JSON.stringify({ b: 2 }) });
  const req = lastRequest(api);
  assert.equal(req.method, 'PATCH');
  assert.equal(req.headers['content-type'], 'application/json');
  assert.deepEqual(jsonBody(req), { b: 2 });
});

test('apiFetch: 200 с не-JSON телом → undefined (считается пустым ответом)', async () => {
  api.mockReply({ status: 200, text: 'не json', contentType: 'text/plain' });
  const res = await apiFetch('/weird');
  assert.equal(res, undefined);
});

// ---------- apiFetch: ошибки ----------

test('apiFetch: 400 {"error": "…"} → ApiError со статусом и текстом сервера', async () => {
  api.mockReply({ status: 400, json: { error: 'поле title обязательно' } });
  await assert.rejects(apiFetch('/bad'), (err) => {
    assert.ok(err instanceof ApiError, 'ожидалась ApiError');
    assert.ok(err instanceof Error, 'ApiError должен быть наследником Error');
    assert.equal(err.status, 400);
    assert.equal(err.message, 'поле title обязательно');
    return true;
  });
});

test('apiFetch: ошибка с не-JSON телом → запасной текст «Ошибка запроса (500)»', async () => {
  api.mockReply({ status: 500, text: '<html>boom</html>', contentType: 'text/html' });
  await assert.rejects(apiFetch('/boom'), (err) => {
    assert.ok(err instanceof ApiError);
    assert.equal(err.status, 500);
    assert.equal(err.message, 'Ошибка запроса (500)');
    return true;
  });
});

test('apiFetch: ошибка с JSON-массивом вместо envelope → запасной текст', async () => {
  api.mockReply({ status: 400, json: [{ error: 'не envelope' }] });
  await assert.rejects(apiFetch('/bad-array'), (err) => {
    assert.ok(err instanceof ApiError);
    assert.equal(err.status, 400);
    assert.equal(err.message, 'Ошибка запроса (400)');
    return true;
  });
});

test('apiFetch: 401 без текста → «Требуется вход в аккаунт»', async () => {
  api.mockReply({ status: 401, text: '' });
  await assert.rejects(apiFetch('/private'), (err) => {
    assert.ok(err instanceof ApiError);
    assert.equal(err.status, 401);
    assert.equal(err.message, 'Требуется вход в аккаунт');
    return true;
  });
});

test('apiFetch: 401 с текстом сервера → текст сервера, не запасной', async () => {
  api.mockReply({ status: 401, json: { error: 'сессия истекла' } });
  await assert.rejects(apiFetch('/private'), (err) => {
    assert.equal(err.status, 401);
    assert.equal(err.message, 'сессия истекла');
    return true;
  });
});

test('apiFetch: 204 No Content → undefined без попытки парсинга', async () => {
  api.mockReply({ status: 204 });
  const res = await apiFetch('/no-content', { method: 'POST' });
  assert.equal(res, undefined);
  const req = lastRequest(api);
  assert.equal(req.method, 'POST');
  assert.equal(req.path, '/api/v1/no-content');
});

// ---------- buildQuery ----------

test('buildQuery: без параметров и с пустым объектом → пустая строка', () => {
  assert.equal(buildQuery(), '');
  assert.equal(buildQuery({}), '');
});

test('buildQuery: undefined и пустые строки выбрасываются', () => {
  assert.equal(buildQuery({ a: undefined, b: '', c: 'x' }), '?c=x');
});

test('buildQuery: числа приводятся к строке, 0 не выбрасывается', () => {
  assert.equal(buildQuery({ page: 2, limit: 10 }), '?page=2&limit=10');
  assert.equal(buildQuery({ page: 0 }), '?page=0');
});

test('buildQuery: значения URL-кодируются', () => {
  assert.equal(buildQuery({ q: 'react hooks' }), '?q=react+hooks');
  assert.equal(buildQuery({ q: 'a&b=c' }), '?q=a%26b%3Dc');
  assert.equal(buildQuery({ q: 'кот' }), `?q=${encodeURIComponent('кот')}`);
});

test('buildQuery: порядок ключей сохраняется', () => {
  assert.equal(buildQuery({ b: '2', a: '1' }), '?b=2&a=1');
});

// ---------- Чистые утилиты (без сети) ----------

test('formatMoney: 0 — это «0 ₽», копейки и отрицательные суммы не округляются до нуля', () => {
  assert.equal(formatMoney(0), '0 ₽');
  assert.equal(formatMoney(1), '0,01 ₽');
  assert.equal(formatMoney(123456), '1\u00A0234,56 ₽');
  assert.equal(formatMoney(-5000), '-50 ₽');
});

test('formatPrice: 0 и меньше → «Бесплатно», иначе та же сумма, что у formatMoney', () => {
  assert.equal(formatPrice(0), 'Бесплатно');
  assert.equal(formatPrice(-100), 'Бесплатно');
  assert.equal(formatPrice(100), '1 ₽');
  assert.equal(formatPrice(123456), formatMoney(123456));
});

test('formatDate: RFC3339 → «дд.мм.гггг, чч:мм» в локальном времени', () => {
  // создаём момент из локальных частей — тест не зависит от таймзоны машины
  const local = new Date(2024, 0, 5, 12, 30);
  assert.equal(formatDate(local.toISOString()), '05.01.2024, 12:30');
});

test('formatDate: пустое и некорректное значение → «—»', () => {
  assert.equal(formatDate(''), '—');
  assert.equal(formatDate(undefined), '—');
  assert.equal(formatDate(null), '—');
  assert.equal(formatDate('не-дата'), '—');
});

test('useApiBase: клиент собран с адресом из .env.test (VITE_API_URL)', () => {
  assert.equal(useApiBase(), TEST_BASE);
});
