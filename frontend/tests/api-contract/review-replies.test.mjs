// T70: контрактные тесты клиента отзывов — конверт с разбивкой оценок,
// фильтр по rating, PUT/DELETE ответа преподавателя (контракт T69/00-api-contract).
// Сервер — мини-API из helper.mjs на фиксированном 127.0.0.1:8799.

import { after, before, beforeEach, test } from 'node:test';
import assert from 'node:assert/strict';
import { startMockApi, lastRequest, jsonBody, expectBodyKeys } from './helper.mjs';
import {
  ApiError,
  deleteReviewReply,
  getCourseReviews,
  replyToReview,
} from '../../.tmp-api-test/client.js';

let api;

const REVIEW = {
  id: 'r1',
  studentId: 'u1',
  courseId: 'c1',
  rating: 5,
  body: 'Отличный курс',
  createdAt: '2026-10-19T12:00:00Z',
  teacherReply: '',
  teacherReplyAt: null,
};

const PAGE = {
  items: [REVIEW],
  total: 1,
  page: 1,
  limit: 20,
  avgRating: 4.5,
  ratingsCount: 3,
  breakdown: { 1: 0, 2: 1, 3: 0, 4: 1, 5: 1 },
};

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

// ---------- getCourseReviews: конверт и фильтр ----------

test('getCourseReviews: без параметров — GET /courses/{id}/reviews, конверт целиком', async () => {
  api.mockReply({ json: PAGE });
  const res = await getCourseReviews('c1');
  // Клиент возвращает конверт как есть (в отличие от listReviews).
  assert.deepEqual(res, PAGE);
  assert.deepEqual(res.breakdown, { 1: 0, 2: 1, 3: 0, 4: 1, 5: 1 });
  assert.equal(res.avgRating, 4.5);
  assert.equal(res.ratingsCount, 3);

  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/courses/c1/reviews');
  assert.equal(req.query.toString(), '', 'без фильтра query пустой');
});

test('getCourseReviews: rating попадает в query, items/total — по фильтру', async () => {
  const filtered = {
    ...PAGE,
    items: [{ ...REVIEW, rating: 4 }],
    total: 1,
  };
  api.mockReply({ json: filtered });
  const res = await getCourseReviews('c1', { rating: 4 });
  assert.deepEqual(res, filtered);

  const req = lastRequest(api);
  assert.equal(req.path, '/api/v1/courses/c1/reviews');
  assert.equal(req.query.get('rating'), '4');
  assert.equal(req.query.toString(), 'rating=4');
});

test('getCourseReviews: id курса кодируется целиком (encodeURIComponent)', async () => {
  api.mockReply({ json: PAGE });
  await getCourseReviews('c/1', { rating: 5 });
  const req = lastRequest(api);
  assert.equal(req.path, '/api/v1/courses/c%2F1/reviews');
  assert.equal(req.query.get('rating'), '5');
});

test('getCourseReviews: breakdown с нулями и пустой items разбираются без потерь', async () => {
  const empty = {
    items: [],
    total: 0,
    page: 1,
    limit: 20,
    avgRating: 0,
    ratingsCount: 0,
    breakdown: { 1: 0, 2: 0, 3: 0, 4: 0, 5: 0 },
  };
  api.mockReply({ json: empty });
  const res = await getCourseReviews('c1', { rating: 3 });
  assert.deepEqual(res, empty);
  assert.equal(res.total, 0);
  assert.equal(res.breakdown['3'], 0);
});

test('getCourseReviews: 400 при недопустимой оценке → ApiError с текстом сервера', async () => {
  api.mockReply({ status: 400, json: { error: 'Оценка должна быть числом от 1 до 5' } });
  await expectApiError(
    getCourseReviews('c1', { rating: 9 }),
    400,
    'Оценка должна быть числом от 1 до 5',
  );
});

// ---------- replyToReview: PUT ----------

test('replyToReview: PUT /teacher/courses/{id}/reviews/{rid}/reply, тело ровно {body}', async () => {
  const replied = {
    ...REVIEW,
    teacherReply: 'Спасибо за отзыв!',
    teacherReplyAt: '2026-10-20T09:30:00Z',
  };
  api.mockReply({ json: { review: replied } });
  const res = await replyToReview('c1', 'r1', 'Спасибо за отзыв!');
  // Конверт { review } разворачивается в Review.
  assert.deepEqual(res, replied);
  assert.equal(res.teacherReply, 'Спасибо за отзыв!');
  assert.equal(res.teacherReplyAt, '2026-10-20T09:30:00Z');

  const req = lastRequest(api);
  assert.equal(req.method, 'PUT');
  assert.equal(req.path, '/api/v1/teacher/courses/c1/reviews/r1/reply');
  const body = expectBodyKeys(jsonBody(req), ['body']);
  assert.deepEqual(body, { body: 'Спасибо за отзыв!' });
});

test('replyToReview: id курса и отзыва кодируются целиком', async () => {
  api.mockReply({ json: { review: REVIEW } });
  await replyToReview('c/1', 'r/1', 'Ответ');
  assert.equal(
    lastRequest(api).path,
    '/api/v1/teacher/courses/c%2F1/reviews/r%2F1/reply',
  );
});

test('replyToReview: 400 (валидация длины) → ApiError 400 с текстом сервера', async () => {
  api.mockReply({ status: 400, json: { error: 'Ответ: от 3 до 2000 символов' } });
  await expectApiError(
    replyToReview('c1', 'r1', 'ok'),
    400,
    'Ответ: от 3 до 2000 символов',
  );
});

test('replyToReview: 403 (чужой курс) → ApiError 403', async () => {
  api.mockReply({ status: 403, json: { error: 'Нет прав на этот курс' } });
  await expectApiError(replyToReview('c1', 'r1', 'Ответ'), 403, 'Нет прав на этот курс');
});

test('replyToReview: 404 (отзыв не найден) → ApiError 404', async () => {
  api.mockReply({ status: 404, json: { error: 'Отзыв не найден' } });
  await expectApiError(replyToReview('c1', 'nope', 'Ответ'), 404, 'Отзыв не найден');
});

// ---------- deleteReviewReply: DELETE ----------

test('deleteReviewReply: DELETE .../reply, 204 → undefined, без тела', async () => {
  api.mockReply({ status: 204 });
  const res = await deleteReviewReply('c1', 'r1');
  assert.equal(res, undefined);

  const req = lastRequest(api);
  assert.equal(req.method, 'DELETE');
  assert.equal(req.path, '/api/v1/teacher/courses/c1/reviews/r1/reply');
  assert.equal(req.rawBody, '', 'у DELETE тела быть не должно');
});

test('deleteReviewReply: ответа не было — тот же 204 (идемпотентно)', async () => {
  api.mockReply({ status: 204 });
  await assert.doesNotReject(deleteReviewReply('c1', 'r1'));
});

test('deleteReviewReply: 403 (чужой курс) → ApiError 403', async () => {
  api.mockReply({ status: 403, json: { error: 'Нет прав на этот курс' } });
  await expectApiError(
    deleteReviewReply('c1', 'r1'),
    403,
    'Нет прав на этот курс',
  );
});

test('deleteReviewReply: 404 (отзыв не найден) → ApiError 404', async () => {
  api.mockReply({ status: 404, json: { error: 'Отзыв не найден' } });
  await expectApiError(deleteReviewReply('c1', 'nope'), 404, 'Отзыв не найден');
});
