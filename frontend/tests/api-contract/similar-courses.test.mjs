// T80: контрактные тесты клиента похожих курсов (T79).
// Проверяют GET /courses/{id}/similar?limit=N: query-параметр limit,
// URL-кодирование id в пути и разбор конверта {items,total} для пустого
// и непустого ответа.

import { after, before, beforeEach, test } from 'node:test';
import assert from 'node:assert/strict';
import { startMockApi, lastRequest } from './helper.mjs';
import { getSimilarCourses } from '../../.tmp-api-test/client.js';

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

/** Карточка в форме CourseItem — как её отдаёт бэкенд в конверте. */
const courseItem = {
  id: 'c2',
  slug: 'sql-basics',
  title: 'SQL Basics',
  description: 'Запросы с нуля',
  priceCents: 199900,
  currency: 'RUB',
  avgRating: 4.5,
  ratingsCount: 7,
  category: 'data',
};

test('getSimilarCourses: GET /courses/{id}/similar с limit=4', async () => {
  api.mockReply({ json: { items: [courseItem], total: 1 } });
  await getSimilarCourses('course-1', 4);
  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/courses/course-1/similar');
  assert.equal(req.query.get('limit'), '4');
  assert.equal(req.query.toString(), 'limit=4', 'лишних параметров быть не должно');
});

test('getSimilarCourses: без limit параметр в query не отправляется', async () => {
  api.mockReply({ json: { items: [], total: 0 } });
  await getSimilarCourses('course-1');
  const req = lastRequest(api);
  assert.equal(req.path, '/api/v1/courses/course-1/similar');
  assert.equal(req.query.toString(), '', 'limit необязателен — query пустой');
});

test('getSimilarCourses: id кодируется в пути', async () => {
  const id = 'курс 1/два?три';
  api.mockReply({ json: { items: [], total: 0 } });
  await getSimilarCourses(id, 4);
  const req = lastRequest(api);
  const encoded = encodeURIComponent(id);
  assert.equal(req.path, `/api/v1/courses/${encoded}/similar`);
  assert.ok(
    req.url.startsWith(`/api/v1/courses/${encoded}/similar?`),
    `в сыром URL ожидался закодированный id, получено: ${req.url}`,
  );
  assert.equal(req.query.get('limit'), '4');
});

test('getSimilarCourses: пустой items → пустой массив', async () => {
  api.mockReply({ json: { items: [], total: 0 } });
  const res = await getSimilarCourses('empty-course', 4);
  assert.ok(Array.isArray(res), 'клиент возвращает массив, а не конверт');
  assert.deepEqual(res, []);
});

test('getSimilarCourses: непустой ответ → CourseItem[] из items', async () => {
  const second = { ...courseItem, id: 'c3', slug: 'sql-advanced', title: 'SQL Advanced' };
  api.mockReply({ json: { items: [courseItem, second], total: 42 } });
  const res = await getSimilarCourses('course-1', 4);
  assert.ok(Array.isArray(res), 'клиент возвращает массив, а не конверт');
  assert.deepEqual(res, [courseItem, second]);
  assert.equal(res.length, 2);
  assert.equal(lastRequest(api).query.get('limit'), '4');
});
