// T84: контрактные тесты API-клиента глобальной модерации отзывов
// (GET /admin/reviews, DELETE /admin/reviews/{id} — Task/00-api-contract.md, T83).
//
// Стандартный test:build собирает только src/api/client.ts, а модуль
// adminReviews.ts — отдельный файл волны P10. Поэтому тест сам загружает его
// через SSR-загрузчик Vite (createServer + ssrLoadModule): TypeScript и
// import.meta.env обрабатываются так же, как при обычной сборке, а .env.test
// подхватывается режимом mode:'test' (адрес мини-API 127.0.0.1:8799).
// Это единственное отличие от остальных тестов tests/api-contract.

import { after, before, beforeEach, test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import { startMockApi, lastRequest } from './helper.mjs';

const FRONTEND_ROOT = fileURLToPath(new URL('../..', import.meta.url));

let api;
let listAdminReviews;
let deleteAdminReview;
let ApiError;

const REVIEW = {
  id: 'r1',
  courseId: 'c1',
  courseTitle: 'Python для начинающих',
  courseSlug: 'python-dlya-nachinayushchih',
  studentId: 'u1',
  studentEmail: 'student@example.test',
  studentName: 'Иван',
  rating: 5,
  body: 'Понятные объяснения и много практики',
  createdAt: '2026-10-24T10:00:00Z',
  teacherReply: '',
  teacherReplyAt: null,
};

const PAGE = { items: [REVIEW], total: 1, page: 1, limit: 20 };

before(async () => {
  const vite = await createServer({
    root: FRONTEND_ROOT,
    configFile: false,
    mode: 'test',
    logLevel: 'silent',
    server: { middlewareMode: true },
    appType: 'custom',
  });
  try {
    const reviewsModule = await vite.ssrLoadModule('/src/api/adminReviews.ts');
    listAdminReviews = reviewsModule.listAdminReviews;
    deleteAdminReview = reviewsModule.deleteAdminReview;
    const clientModule = await vite.ssrLoadModule('/src/api/client.ts');
    ApiError = clientModule.ApiError;
  } finally {
    await vite.close();
  }
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

// ---------- listAdminReviews ----------

test('listAdminReviews: без параметров — GET /admin/reviews с пустым query', async () => {
  api.mockReply({ json: PAGE });
  const res = await listAdminReviews();
  assert.deepEqual(res, PAGE);
  assert.equal(res.items[0].courseSlug, 'python-dlya-nachinayushchih');
  assert.equal(res.items[0].teacherReplyAt, null);

  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/admin/reviews');
  assert.equal(req.query.toString(), '', 'без фильтров query пустой');
});

test('listAdminReviews: все фильтры уходят в query, ответ — конверт пагинации', async () => {
  const filtered = { ...PAGE, total: 3, page: 2, limit: 50 };
  api.mockReply({ json: filtered });
  const res = await listAdminReviews({
    courseId: 'c1',
    rating: 4,
    q: 'Иван',
    from: '2026-01-01',
    to: '2026-12-31',
    sort: 'rating',
    dir: 'asc',
    page: 2,
    limit: 50,
  });
  assert.deepEqual(res, filtered);
  assert.equal(res.total, 3);
  assert.equal(res.page, 2);
  assert.equal(res.limit, 50);

  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/admin/reviews');
  assert.deepEqual(Object.fromEntries(req.query), {
    courseId: 'c1',
    rating: '4',
    q: 'Иван',
    from: '2026-01-01',
    to: '2026-12-31',
    sort: 'rating',
    dir: 'asc',
    page: '2',
    limit: '50',
  });
});

test('listAdminReviews: пустые значения не отправляются (buildQuery)', async () => {
  api.mockReply({ json: PAGE });
  await listAdminReviews({ q: '', courseId: '', from: '', to: '', page: 1 });
  const req = lastRequest(api);
  assert.deepEqual(Object.fromEntries(req.query), { page: '1' });
});

test('listAdminReviews: отзыв без текста и без ответа разбирается без потерь', async () => {
  const emptyBody = {
    ...PAGE,
    items: [{ ...REVIEW, body: '', teacherReply: '', teacherReplyAt: null }],
  };
  api.mockReply({ json: emptyBody });
  const res = await listAdminReviews({ rating: 5 });
  assert.equal(res.items[0].body, '');
  assert.equal(res.items[0].teacherReply, '');
  assert.equal(res.items[0].teacherReplyAt, null);
  assert.equal(lastRequest(api).query.get('rating'), '5');
});

test('listAdminReviews: пустая выборка — items массив, total 0', async () => {
  const empty = { items: [], total: 0, page: 1, limit: 20 };
  api.mockReply({ json: empty });
  const res = await listAdminReviews({ courseId: 'c1', page: 1, limit: 20 });
  assert.deepEqual(res, empty);
  assert.ok(Array.isArray(res.items));
});

test('listAdminReviews: 400 при недопустимой оценке → ApiError с текстом сервера', async () => {
  api.mockReply({
    status: 400,
    json: { error: 'Оценка должна быть целым числом от 1 до 5' },
  });
  await expectApiError(
    listAdminReviews({ rating: 9 }),
    400,
    'Оценка должна быть целым числом от 1 до 5',
  );
});

test('listAdminReviews: 403 не-админу → ApiError 403', async () => {
  api.mockReply({ status: 403, json: { error: 'Недостаточно прав' } });
  await expectApiError(listAdminReviews(), 403, 'Недостаточно прав');
});

// ---------- deleteAdminReview ----------

test('deleteAdminReview: DELETE /admin/reviews/{id}, 204 → undefined, без тела', async () => {
  api.mockReply({ status: 204 });
  const res = await deleteAdminReview('r1');
  assert.equal(res, undefined);

  const req = lastRequest(api);
  assert.equal(req.method, 'DELETE');
  assert.equal(req.path, '/api/v1/admin/reviews/r1');
  assert.equal(req.rawBody, '', 'у DELETE тела быть не должно');
});

test('deleteAdminReview: id отзыва кодируется целиком (encodeURIComponent)', async () => {
  api.mockReply({ status: 204 });
  await deleteAdminReview('r/1');
  assert.equal(lastRequest(api).path, '/api/v1/admin/reviews/r%2F1');
});

test('deleteAdminReview: 404 (отзыв уже удалён) → ApiError 404', async () => {
  api.mockReply({ status: 404, json: { error: 'Отзыв не найден' } });
  await expectApiError(deleteAdminReview('nope'), 404, 'Отзыв не найден');
});
