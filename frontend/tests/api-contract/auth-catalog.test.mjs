// Контрактные тесты клиента: auth, каталог, отзывы, обучение.
// Тело каждого запроса проверяется точно (DisallowUnknownFields на backend).

import { after, before, beforeEach, test } from 'node:test';
import assert from 'node:assert/strict';
import { startMockApi, lastRequest, jsonBody, expectBodyKeys } from './helper.mjs';
import {
  completeLesson,
  createReview,
  enroll,
  getCourse,
  listCourses,
  listReviews,
  login,
  logout,
  me,
  myCourses,
  register,
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

// ---------- Auth ----------

test('register: POST /auth/register, тело строго email+password', async () => {
  api.mockReply({ json: { id: 'u1', email: 'student@example.com', role: 'student' } });
  const user = await register({ email: 'student@example.com', password: 'secret123' });
  assert.deepEqual(user, { id: 'u1', email: 'student@example.com', role: 'student' });
  const req = lastRequest(api);
  assert.equal(req.method, 'POST');
  assert.equal(req.path, '/api/v1/auth/register');
  assert.equal(req.headers['content-type'], 'application/json');
  expectBodyKeys(jsonBody(req), ['email', 'password']);
  assert.deepEqual(req.json, { email: 'student@example.com', password: 'secret123' });
});

test('register: inviteToken уходит только когда передан', async () => {
  api.mockReply({ json: { id: 'u2', email: 'teacher@example.com', role: 'teacher' } });
  await register({ email: 'teacher@example.com', password: 'pw123456', inviteToken: 'INV-TOKEN-1' });
  const body = expectBodyKeys(jsonBody(lastRequest(api)), ['email', 'password', 'inviteToken']);
  assert.deepEqual(body, {
    email: 'teacher@example.com',
    password: 'pw123456',
    inviteToken: 'INV-TOKEN-1',
  });
});

test('login: POST /auth/login, тело строго email+password', async () => {
  api.mockReply({ json: { id: 'u1', email: 'student@example.com', role: 'student' } });
  const user = await login({ email: 'student@example.com', password: 'secret123' });
  assert.equal(user.id, 'u1');
  assert.equal(user.role, 'student');
  const req = lastRequest(api);
  assert.equal(req.method, 'POST');
  assert.equal(req.path, '/api/v1/auth/login');
  assert.deepEqual(expectBodyKeys(jsonBody(req), ['email', 'password']), {
    email: 'student@example.com',
    password: 'secret123',
  });
});

test('logout: POST /auth/logout без тела, 204 → undefined', async () => {
  api.mockReply({ status: 204 });
  const res = await logout();
  assert.equal(res, undefined);
  const req = lastRequest(api);
  assert.equal(req.method, 'POST');
  assert.equal(req.path, '/api/v1/auth/logout');
  assert.equal(req.rawBody, '', 'тело у logout не отправляется');
});

test('me: GET /auth/me возвращает пользователя как есть', async () => {
  api.mockReply({ json: { id: 'u1', email: 'student@example.com', role: 'student', displayName: 'Иван' } });
  const user = await me();
  assert.deepEqual(user, {
    id: 'u1',
    email: 'student@example.com',
    role: 'student',
    displayName: 'Иван',
  });
  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/auth/me');
});

// ---------- Каталог ----------

test('listCourses: без параметров — GET /courses без query', async () => {
  api.mockReply({ json: { items: [], total: 0, page: 1, limit: 20 } });
  const res = await listCourses();
  assert.deepEqual(res, { items: [], total: 0, page: 1, limit: 20 });
  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/courses');
  assert.equal(req.query.toString(), '');
});

test('listCourses: q/page/limit попадают в query, пустые выбрасываются', async () => {
  api.mockReply({ json: { items: [], total: 0, page: 2, limit: 10 } });
  await listCourses({ q: 'react hooks', page: 2, limit: 10 });
  const req = lastRequest(api);
  assert.equal(req.path, '/api/v1/courses');
  assert.equal(req.query.get('q'), 'react hooks');
  assert.equal(req.query.get('page'), '2');
  assert.equal(req.query.get('limit'), '10');
  assert.equal(req.query.toString(), 'q=react+hooks&page=2&limit=10');

  api.mockReply({ json: { items: [], total: 0, page: 1, limit: 20 } });
  await listCourses({ q: '', page: undefined, limit: undefined });
  assert.equal(lastRequest(api).query.toString(), '', 'пустые параметры не должны попадать в query');
});

test('listCourses: карточки каталога возвращаются в конверте {items,total,page,limit}', async () => {
  const item = {
    id: 'c1',
    slug: 'react-basics',
    title: 'React Basics',
    description: 'Основы',
    priceCents: 150000,
    currency: 'RUB',
    avgRating: 4.5,
    ratingsCount: 2,
  };
  api.mockReply({ json: { items: [item], total: 1, page: 1, limit: 20 } });
  const res = await listCourses({ q: 'react' });
  assert.equal(res.total, 1);
  assert.equal(res.page, 1);
  assert.equal(res.limit, 20);
  assert.deepEqual(res.items, [item]);
});

test('getCourse: GET /courses/{slug}, конверт {course, modules} распаковывается', async () => {
  const course = {
    id: 'c1',
    teacherId: 't1',
    slug: 'react-basics',
    title: 'React Basics',
    description: 'Основы',
    priceCents: 150000,
    currency: 'RUB',
    status: 'published',
    avgRating: 4.5,
    ratingsCount: 2,
    createdAt: '2024-01-01T00:00:00Z',
    updatedAt: '2024-01-02T00:00:00Z',
  };
  const modules = [{ id: 'm1', title: 'Введение', position: 1, lessons: [] }];
  api.mockReply({ json: { course, modules } });
  const res = await getCourse('react-basics');
  assert.equal(lastRequest(api).path, '/api/v1/courses/react-basics');
  // поля course развёрнуты на верхний уровень, модули сохранены как есть
  assert.equal(res.id, 'c1');
  assert.equal(res.slug, 'react-basics');
  assert.equal(res.status, 'published');
  assert.deepEqual(res.modules, modules);
});

test('getCourse: slug URL-кодируется целиком (пробелы, слэш, кириллица)', async () => {
  const slug = 'курс слэш/и пробел';
  api.mockReply({
    json: {
      course: {
        id: 'c2',
        teacherId: 't1',
        slug,
        title: 'Курс',
        description: '',
        priceCents: 0,
        currency: 'RUB',
        status: 'draft',
        avgRating: 0,
        ratingsCount: 0,
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-01T00:00:00Z',
      },
      modules: [],
    },
  });
  await getCourse(slug);
  assert.equal(lastRequest(api).path, `/api/v1/courses/${encodeURIComponent(slug)}`);
});

// ---------- Отзывы ----------

test('listReviews: GET /courses/{id}/reviews, конверт распаковывается в массив', async () => {
  const review = {
    id: 'r1',
    studentId: 's1',
    courseId: 'c1',
    rating: 5,
    body: 'Отличный курс',
    createdAt: '2024-02-01T10:00:00Z',
  };
  api.mockReply({ json: { items: [review], total: 1, avgRating: 5, ratingsCount: 1 } });
  const res = await listReviews('c1');
  assert.equal(lastRequest(api).path, '/api/v1/courses/c1/reviews');
  assert.equal(lastRequest(api).method, 'GET');
  assert.deepEqual(res, [review]);
});

test('createReview: POST /courses/{id}/reviews, тело строго rating+body (ключ body, не text)', async () => {
  api.mockReply({
    json: {
      id: 'r2',
      studentId: 's1',
      courseId: 'c1',
      rating: 4,
      body: 'Хорошо',
      createdAt: '2024-02-02T10:00:00Z',
    },
  });
  const res = await createReview('c1', { rating: 4, body: 'Хорошо' });
  assert.equal(res.id, 'r2');
  assert.equal(res.rating, 4);
  const req = lastRequest(api);
  assert.equal(req.method, 'POST');
  assert.equal(req.path, '/api/v1/courses/c1/reviews');
  assert.deepEqual(expectBodyKeys(jsonBody(req), ['rating', 'body']), { rating: 4, body: 'Хорошо' });
});

// ---------- Обучение ----------

test('enroll: POST /enrollments, provider по умолчанию mock', async () => {
  const result = {
    paymentId: 'p1',
    providerPaymentId: 'mock-1',
    redirectUrl: '',
    amountCents: 0,
    status: 'succeeded',
    enrollmentActive: true,
  };
  api.mockReply({ json: result });
  const res = await enroll('c1');
  assert.deepEqual(res, result);
  const req = lastRequest(api);
  assert.equal(req.method, 'POST');
  assert.equal(req.path, '/api/v1/enrollments');
  assert.deepEqual(expectBodyKeys(jsonBody(req), ['courseId', 'provider']), {
    courseId: 'c1',
    provider: 'mock',
  });
});

test('enroll: явный provider попадает в тело без изменений', async () => {
  api.mockReply({
    json: {
      paymentId: 'p2',
      providerPaymentId: 'stripe-1',
      redirectUrl: 'https://pay.example/1',
      amountCents: 150000,
      status: 'pending',
      enrollmentActive: false,
    },
  });
  await enroll('c9', 'stripe');
  assert.deepEqual(jsonBody(lastRequest(api)), { courseId: 'c9', provider: 'stripe' });
});

test('myCourses: GET /me/courses, конверт распаковывается в массив MyCourse', async () => {
  const myCourse = {
    course: {
      id: 'c1',
      slug: 'react-basics',
      title: 'React Basics',
      description: 'Основы',
      priceCents: 150000,
      currency: 'RUB',
      avgRating: 4.5,
      ratingsCount: 2,
    },
    percent: 40,
    nextLesson: { lessonId: 'l2', title: 'Хуки', kind: 'video_text' },
  };
  api.mockReply({ json: { items: [myCourse], total: 1 } });
  const res = await myCourses();
  assert.equal(lastRequest(api).path, '/api/v1/me/courses');
  assert.deepEqual(res, [myCourse]);
});

test('completeLesson (video_text): POST …/complete без тела вовсе', async () => {
  api.mockReply({ json: { passed: true, completed: true, percent: 100 } });
  const res = await completeLesson('c1', 'l1');
  assert.deepEqual(res, { passed: true, completed: true, percent: 100 });
  const req = lastRequest(api);
  assert.equal(req.method, 'POST');
  assert.equal(req.path, '/api/v1/me/courses/c1/lessons/l1/complete');
  assert.equal(req.rawBody, '', 'для video/live тело не отправляется (пустой объект = 400)');
});

test('completeLesson (quiz): тело строго quizAnswers', async () => {
  api.mockReply({ json: { passed: false, completed: false, correct: 1, total: 2, message: 'Есть ошибки' } });
  const res = await completeLesson('c1', 'l2', { quizAnswers: { q1: 0, q2: 1 } });
  assert.equal(res.passed, false);
  assert.equal(res.total, 2);
  assert.deepEqual(expectBodyKeys(jsonBody(lastRequest(api)), ['quizAnswers']), {
    quizAnswers: { q1: 0, q2: 1 },
  });
});

test('completeLesson (quiz): пустой quizAnswers → тело не отправляется', async () => {
  api.mockReply({ json: { passed: true, completed: true } });
  await completeLesson('c1', 'l2', { quizAnswers: {} });
  assert.equal(lastRequest(api).rawBody, '');
});

test('completeLesson (code): тело code+language', async () => {
  api.mockReply({ json: { passed: true, completed: true, percent: 80 } });
  await completeLesson('c1', 'l3', { code: 'print(1)', language: 'python' });
  assert.deepEqual(expectBodyKeys(jsonBody(lastRequest(api)), ['code', 'language']), {
    code: 'print(1)',
    language: 'python',
  });
});

test('completeLesson (code): без language в теле только code', async () => {
  api.mockReply({ json: { passed: false, completed: false, message: 'Тесты не прошли' } });
  await completeLesson('c1', 'l3', { code: 'return 1' });
  assert.deepEqual(expectBodyKeys(jsonBody(lastRequest(api)), ['code']), { code: 'return 1' });
});
