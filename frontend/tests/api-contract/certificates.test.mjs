// Контрактные тесты клиента: сертификаты (T68).
// GET /certificates/{number} — публичный (200/404), GET /me/certificates —
// конверт { items } распаковывается в массив.

import { after, before, beforeEach, test } from 'node:test';
import assert from 'node:assert/strict';
import { startMockApi, lastRequest } from './helper.mjs';
import {
  ApiError,
  getCertificate,
  myCertificates,
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

const CERT = {
  number: 'EDU-2026-1A2B3C4D',
  issuedAt: '2026-10-19T12:00:00Z',
  studentName: 'Иван Петров',
  courseTitle: 'React Basics',
  courseSlug: 'react-basics',
};

// ---------- Публичный сертификат ----------

test('getCertificate: GET /certificates/{number}, поля как в контракте', async () => {
  api.mockReply({ json: CERT });
  const res = await getCertificate(CERT.number);
  assert.deepEqual(res, CERT);
  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/certificates/EDU-2026-1A2B3C4D');
  assert.equal(req.query.toString(), '');
  assert.equal(req.rawBody, '', 'публичный GET отправляется без тела');
});

test('getCertificate: номер URL-кодируется целиком', async () => {
  api.mockReply({ json: { ...CERT, number: 'EDU 2026/1' } });
  const number = 'EDU 2026/1';
  await getCertificate(number);
  assert.equal(lastRequest(api).path, `/api/v1/certificates/${encodeURIComponent(number)}`);
});

test('getCertificate: 404 → ApiError(404) с текстом сервера', async () => {
  api.mockReply({ status: 404, json: { error: 'Сертификат не найден' } });
  await assert.rejects(
    () => getCertificate('EDU-2026-00000000'),
    (err) => {
      assert.ok(err instanceof ApiError, 'ожидалась ApiError');
      assert.equal(err.status, 404);
      assert.equal(err.message, 'Сертификат не найден');
      return true;
    },
  );
  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/certificates/EDU-2026-00000000');
});

// ---------- Мои сертификаты ----------

test('myCertificates: GET /me/certificates, items распаковываются в массив', async () => {
  const items = [
    {
      number: 'EDU-2026-1A2B3C4D',
      issuedAt: '2026-10-19T12:00:00Z',
      courseId: 'c1',
      courseTitle: 'React Basics',
      courseSlug: 'react-basics',
    },
    {
      number: 'EDU-2025-9F8E7D6C',
      issuedAt: '2025-12-01T09:30:00Z',
      courseId: 'c2',
      courseTitle: 'SQL для начинающих',
      courseSlug: 'sql-basics',
    },
  ];
  api.mockReply({ json: { items } });
  const res = await myCertificates();
  assert.deepEqual(res, items);
  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/me/certificates');
  assert.equal(req.query.toString(), '');
});

test('myCertificates: пустой items → пустой массив, не undefined', async () => {
  api.mockReply({ json: { items: [] } });
  const res = await myCertificates();
  assert.deepEqual(res, []);
});
