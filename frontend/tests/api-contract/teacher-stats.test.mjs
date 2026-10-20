// T78: контрактные тесты клиента аналитики преподавателя (P9/T77).
// Эндпоинты: GET /teacher/stats и GET /teacher/stats/courses?limit=…
// Формы ответов — строго по разделу «Волна P9» Task/00-api-contract.md.

import { after, before, beforeEach, test } from 'node:test';
import assert from 'node:assert/strict';
import { lastRequest, startMockApi } from './helper.mjs';
import { teacherCourseStats, teacherOverview } from '../../.tmp-api-test/client.js';

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

/** Полная сводка по контракту T77. */
function overviewFixture(overrides = {}) {
  return {
    courses: { total: 4, published: 2, draft: 1, review: 1, archived: 0 },
    students: { total: 15, unique: 12 },
    revenue: {
      paidTotalCents: 4500000,
      platformFeeCents: 450000,
      teacherCents: 4050000,
    },
    completion: { completed: 3, avgPercent: 62.5 },
    ratings: { avgRating: 4.6, ratingsCount: 8 },
    last30d: { enrollments: 5, revenueCents: 1200000 },
    ...overrides,
  };
}

/** Строка разбивки по курсам. */
function courseStatFixture(overrides = {}) {
  return {
    courseId: 'c1',
    title: 'React Basics',
    status: 'published',
    students: 7,
    revenueCents: 1500000,
    avgRating: 4.5,
    ratingsCount: 4,
    avgPercent: 48.5,
    ...overrides,
  };
}

// ---------- teacherOverview ----------

test('teacherOverview: GET /teacher/stats, конверт возвращается целиком', async () => {
  const fixture = overviewFixture();
  api.mockReply({ json: fixture });

  const res = await teacherOverview();

  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/teacher/stats');
  assert.equal(req.query.toString(), '', 'у сводки не должно быть query-параметров');
  assert.equal(req.rawBody, '', 'GET не отправляет тело');
  assert.deepEqual(res, fixture);
});

test('teacherOverview: нулевые значения не подменяются и не теряются', async () => {
  const fixture = overviewFixture({
    courses: { total: 0, published: 0, draft: 0, review: 0, archived: 0 },
    students: { total: 0, unique: 0 },
    revenue: { paidTotalCents: 0, platformFeeCents: 0, teacherCents: 0 },
    completion: { completed: 0, avgPercent: 0 },
    ratings: { avgRating: 0, ratingsCount: 0 },
    last30d: { enrollments: 0, revenueCents: 0 },
  });
  api.mockReply({ json: fixture });

  const res = await teacherOverview();

  assert.deepEqual(res, fixture);
  assert.equal(res.ratings.avgRating, 0);
  assert.equal(res.last30d.revenueCents, 0);
});

test('teacherOverview: 403 → ApiError со статусом и текстом сервера', async () => {
  api.mockReply({
    status: 403,
    json: { error: 'Недостаточно прав для этой страницы' },
  });

  await assert.rejects(teacherOverview(), (err) => {
    assert.equal(err.status, 403);
    assert.equal(err.message, 'Недостаточно прав для этой страницы');
    return true;
  });
});

test('teacherOverview: 401 без текста → «Требуется вход в аккаунт»', async () => {
  api.mockReply({ status: 401 });

  await assert.rejects(teacherOverview(), (err) => {
    assert.equal(err.status, 401);
    assert.equal(err.message, 'Требуется вход в аккаунт');
    return true;
  });
});

// ---------- teacherCourseStats ----------

test('teacherCourseStats: без limit запрос идёт без query, конверт цел', async () => {
  const fixture = { items: [courseStatFixture()], total: 1 };
  api.mockReply({ json: fixture });

  const res = await teacherCourseStats();

  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/teacher/stats/courses');
  assert.equal(req.query.toString(), '', 'без limit параметр не отправляется');
  assert.equal(req.rawBody, '');
  assert.deepEqual(res, fixture);
});

test('teacherCourseStats: limit уходит query-параметром', async () => {
  api.mockReply({ json: { items: [], total: 0 } });

  await teacherCourseStats(7);

  const req = lastRequest(api);
  assert.equal(req.path, '/api/v1/teacher/stats/courses');
  assert.equal(req.query.get('limit'), '7');
  assert.equal(req.query.toString(), 'limit=7');
});

test('teacherCourseStats: максимальный limit=200 передаётся как есть', async () => {
  api.mockReply({ json: { items: [], total: 0 } });

  await teacherCourseStats(200);

  assert.equal(lastRequest(api).query.get('limit'), '200');
});

test('teacherCourseStats: пустая выдача и все поля строки сохраняются', async () => {
  const rows = [
    courseStatFixture(),
    courseStatFixture({
      courseId: 'c2',
      title: 'Draft course',
      status: 'draft',
      students: 0,
      revenueCents: 0,
      avgRating: 0,
      ratingsCount: 0,
      avgPercent: 0,
    }),
  ];
  api.mockReply({ json: { items: rows, total: 2 } });

  const res = await teacherCourseStats(50);

  assert.equal(res.total, 2);
  assert.deepEqual(res.items, rows);
  assert.equal(lastRequest(api).query.get('limit'), '50');
});

test('teacherCourseStats: 403 → ApiError 403 с текстом сервера', async () => {
  api.mockReply({ status: 403, json: { error: 'Доступ только для преподавателей' } });

  await assert.rejects(teacherCourseStats(50), (err) => {
    assert.equal(err.status, 403);
    assert.equal(err.message, 'Доступ только для преподавателей');
    return true;
  });
});

test('teacherCourseStats: 403 без текста → запасной текст со статусом', async () => {
  api.mockReply({ status: 403 });

  await assert.rejects(teacherCourseStats(), (err) => {
    assert.equal(err.status, 403);
    assert.equal(err.message, 'Ошибка запроса (403)');
    return true;
  });
});
