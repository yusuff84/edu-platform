// T66: контрактные тесты фильтров каталога.
// Проверяют query-параметры GET /courses (T65), парсинг GET /categories
// и сохранение конверта {items,total,page,limit}.

import { after, before, beforeEach, test } from 'node:test';
import assert from 'node:assert/strict';
import { startMockApi, lastRequest } from './helper.mjs';
import { getCategories, listCourses } from '../../.tmp-api-test/client.js';

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

// ---------- GET /courses: новые фильтры ----------

test('listCourses: category/price/minRating/sort уходят в query вместе с q/page/limit', async () => {
  api.mockReply({ json: { items: [], total: 0, page: 2, limit: 10 } });
  await listCourses({
    q: 'react',
    category: 'programming',
    price: 'free',
    minRating: '4.5',
    sort: 'rating',
    page: 2,
    limit: 10,
  });
  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/courses');
  assert.equal(req.query.get('q'), 'react');
  assert.equal(req.query.get('category'), 'programming');
  assert.equal(req.query.get('price'), 'free');
  assert.equal(req.query.get('minRating'), '4.5');
  assert.equal(req.query.get('sort'), 'rating');
  assert.equal(req.query.get('page'), '2');
  assert.equal(req.query.get('limit'), '10');
  assert.equal(
    req.query.toString(),
    'q=react&category=programming&price=free&minRating=4.5&sort=rating&page=2&limit=10',
    'в query не должно быть лишних параметров',
  );
});

test('listCourses: каждый новый параметр работает отдельно', async () => {
  api.mockReply({ json: { items: [], total: 0, page: 1, limit: 20 } });
  const cases = [
    ['category', 'design'],
    ['price', 'paid'],
    ['minRating', '3.5'],
    ['minRating', '4'],
    ['sort', 'new'],
    ['sort', 'price_asc'],
    ['sort', 'price_desc'],
  ];
  for (const [key, value] of cases) {
    await listCourses({ [key]: value });
    const req = lastRequest(api);
    assert.equal(req.query.get(key), value, `параметр ${key}=${value} должен уйти в query`);
    assert.equal(req.query.toString(), `${key}=${value}`, 'должен уйти только этот параметр');
  }
});

test('listCourses: пустые новые параметры не отправляются (обратная совместимость)', async () => {
  api.mockReply({ json: { items: [], total: 0, page: 1, limit: 20 } });
  await listCourses({
    q: '',
    category: '',
    price: '',
    minRating: '',
    sort: '',
    page: undefined,
    limit: undefined,
  });
  const req = lastRequest(api);
  assert.equal(req.path, '/api/v1/courses');
  assert.equal(req.query.toString(), '', 'пустые значения не должны попадать в query');
});

test('listCourses: конверт {items,total,page,limit} и category карточки сохраняются', async () => {
  const item = {
    id: 'c1',
    slug: 'sql-basics',
    title: 'SQL Basics',
    description: 'Запросы с нуля',
    priceCents: 0,
    currency: 'RUB',
    avgRating: 4.5,
    ratingsCount: 7,
    category: 'data',
  };
  api.mockReply({ json: { items: [item], total: 42, page: 1, limit: 20 } });
  const res = await listCourses({ category: 'data', sort: 'popular', limit: 20 });
  assert.equal(res.total, 42);
  assert.equal(res.page, 1);
  assert.equal(res.limit, 20);
  assert.deepEqual(res.items, [item]);
  assert.equal(res.items[0].category, 'data');
  assert.equal(lastRequest(api).query.get('category'), 'data');
});

// ---------- GET /categories ----------

test('getCategories: GET /categories, конверт {items} распаковывается в CategoryStat[]', async () => {
  const items = [
    { slug: 'programming', coursesCount: 3 },
    { slug: 'design', coursesCount: 1 },
    { slug: 'data', coursesCount: 0 },
    { slug: 'marketing', coursesCount: 2 },
    { slug: 'ai', coursesCount: 0 },
    { slug: 'other', coursesCount: 4 },
  ];
  api.mockReply({ json: { items } });
  const res = await getCategories();
  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/categories');
  assert.equal(req.query.toString(), '', 'у /categories нет параметров');
  assert.ok(Array.isArray(res), 'клиент возвращает массив, а не конверт');
  assert.deepEqual(res, items);
});

test('getCategories: пустой список → пустой массив', async () => {
  api.mockReply({ json: { items: [] } });
  const res = await getCategories();
  assert.deepEqual(res, []);
});
