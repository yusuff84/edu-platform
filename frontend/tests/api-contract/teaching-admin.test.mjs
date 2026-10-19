// Контрактные тесты клиента: преподавание (создание курса/модулей/уроков) и админка.
// Тела запросов проверяются точно — backend отвечает 400 на любое лишнее поле.

import { after, before, beforeEach, test } from 'node:test';
import assert from 'node:assert/strict';
import { startMockApi, lastRequest, jsonBody, expectBodyKeys } from './helper.mjs';
import {
  createCourse,
  createInvite,
  createLesson,
  createModule,
  getAdminSettings,
  getPublicSettings,
  moderateCourse,
  myTeaching,
  paymentJournal,
  reviewQueue,
  submitCourse,
  updateAdminSettings,
  updateAdminSettingsPromo,
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

/** Полный Course-объект для фикстур ответов. */
function courseFixture(overrides = {}) {
  return {
    id: 'c1',
    teacherId: 't1',
    slug: 'react-basics',
    title: 'React Basics',
    description: 'Основы React',
    priceCents: 150000,
    currency: 'RUB',
    status: 'draft',
    avgRating: 0,
    ratingsCount: 0,
    createdAt: '2024-01-01T00:00:00Z',
    updatedAt: '2024-01-01T00:00:00Z',
    ...overrides,
  };
}

// ---------- Преподавание ----------

test('myTeaching: GET /teacher/courses, конверт {items,total} → массив', async () => {
  const course = courseFixture();
  api.mockReply({ json: { items: [course], total: 1 } });
  const res = await myTeaching();
  assert.equal(lastRequest(api).path, '/api/v1/teacher/courses');
  assert.equal(lastRequest(api).method, 'GET');
  assert.deepEqual(res, [course]);
});

test('createCourse: POST /teacher/courses, тело строго из 3 полей без slug/currency', async () => {
  api.mockReply({ json: courseFixture({ title: 'Курс', description: 'Описание' }) });
  const res = await createCourse({ title: 'Курс', description: 'Описание', priceCents: 150000 });
  assert.equal(res.id, 'c1');
  const req = lastRequest(api);
  assert.equal(req.method, 'POST');
  assert.equal(req.path, '/api/v1/teacher/courses');
  assert.deepEqual(expectBodyKeys(jsonBody(req), ['title', 'description', 'priceCents']), {
    title: 'Курс',
    description: 'Описание',
    priceCents: 150000,
  });
});

test('createCourse: slug и currency добавляются, только когда переданы', async () => {
  api.mockReply({ json: courseFixture({ slug: 'my-slug', currency: 'RUB' }) });
  await createCourse({
    title: 'Курс',
    description: 'Описание',
    priceCents: 0,
    slug: 'my-slug',
    currency: 'RUB',
  });
  assert.deepEqual(
    expectBodyKeys(jsonBody(lastRequest(api)), ['title', 'description', 'priceCents', 'slug', 'currency']),
    { title: 'Курс', description: 'Описание', priceCents: 0, slug: 'my-slug', currency: 'RUB' },
  );
});

test('createModule: POST /teacher/courses/{id}/modules, тело строго {title}', async () => {
  api.mockReply({ json: { id: 'm1', title: 'Введение', position: 1, lessons: [] } });
  const mod = await createModule('c1', { title: 'Введение' });
  assert.deepEqual(mod, { id: 'm1', title: 'Введение', position: 1, lessons: [] });
  const req = lastRequest(api);
  assert.equal(req.method, 'POST');
  assert.equal(req.path, '/api/v1/teacher/courses/c1/modules');
  assert.deepEqual(expectBodyKeys(jsonBody(req), ['title']), { title: 'Введение' });
});

test('createLesson (video_text): в теле только поля своего kind', async () => {
  api.mockReply({ json: { id: 'l1', title: 'Урок', kind: 'video_text', position: 1 } });
  const lesson = await createLesson('c1', {
    moduleId: 'm1',
    title: 'Урок',
    kind: 'video_text',
    bodyMd: '# Текст урока',
    videoUrl: 'https://video.example/1',
  });
  assert.equal(lesson.id, 'l1');
  const req = lastRequest(api);
  assert.equal(req.method, 'POST');
  assert.equal(req.path, '/api/v1/teacher/courses/c1/lessons');
  assert.deepEqual(
    expectBodyKeys(jsonBody(req), ['moduleId', 'title', 'kind', 'bodyMd', 'videoUrl']),
    {
      moduleId: 'm1',
      title: 'Урок',
      kind: 'video_text',
      bodyMd: '# Текст урока',
      videoUrl: 'https://video.example/1',
    },
  );
});

test('createLesson (video_text без bodyMd/videoUrl): только обязательные поля', async () => {
  api.mockReply({ json: { id: 'l2', title: 'Пустой урок', kind: 'video_text', position: 2 } });
  await createLesson('c1', { moduleId: 'm1', title: 'Пустой урок', kind: 'video_text' });
  assert.deepEqual(expectBodyKeys(jsonBody(lastRequest(api)), ['moduleId', 'title', 'kind']), {
    moduleId: 'm1',
    title: 'Пустой урок',
    kind: 'video_text',
  });
});

test('createLesson (live): liveUrl и startsAt, без чужих полей', async () => {
  api.mockReply({ json: { id: 'l3', title: 'Вебинар', kind: 'live', position: 3 } });
  await createLesson('c1', {
    moduleId: 'm2',
    title: 'Вебинар',
    kind: 'live',
    liveUrl: 'https://live.example/room/1',
    startsAt: '2024-03-01T10:00:00Z',
  });
  assert.deepEqual(
    expectBodyKeys(jsonBody(lastRequest(api)), ['moduleId', 'title', 'kind', 'liveUrl', 'startsAt']),
    {
      moduleId: 'm2',
      title: 'Вебинар',
      kind: 'live',
      liveUrl: 'https://live.example/room/1',
      startsAt: '2024-03-01T10:00:00Z',
    },
  );
});

test('createLesson (live без liveUrl/startsAt): только обязательные поля', async () => {
  api.mockReply({ json: { id: 'l4', title: 'Вебинар-заглушка', kind: 'live', position: 4 } });
  await createLesson('c1', { moduleId: 'm2', title: 'Вебинар-заглушка', kind: 'live' });
  assert.deepEqual(expectBodyKeys(jsonBody(lastRequest(api)), ['moduleId', 'title', 'kind']), {
    moduleId: 'm2',
    title: 'Вебинар-заглушка',
    kind: 'live',
  });
});

test('createLesson (quiz): вложенный quiz = {questions}, вопросы как есть', async () => {
  api.mockReply({ json: { id: 'l5', title: 'Квиз', kind: 'quiz', position: 5 } });
  const questions = [
    { id: 'q1', text: 'Сколько будет 2+2?', options: ['3', '4'], correct: 1 },
    { text: 'Столица Франции?', options: ['Париж', 'Лион'], correct: 0, explanation: 'Очевидно' },
  ];
  await createLesson('c1', {
    moduleId: 'm1',
    title: 'Квиз',
    kind: 'quiz',
    quiz: { questions },
  });
  assert.deepEqual(
    expectBodyKeys(jsonBody(lastRequest(api)), ['moduleId', 'title', 'kind', 'quiz']),
    { moduleId: 'm1', title: 'Квиз', kind: 'quiz', quiz: { questions } },
  );
});

test('createLesson (code): вложенный codeTask с template', async () => {
  api.mockReply({ json: { id: 'l6', title: 'Задача', kind: 'code', position: 6 } });
  const tests = [{ name: 'Тест 1', input: '1', expected: '1' }];
  await createLesson('c1', {
    moduleId: 'm1',
    title: 'Задача',
    kind: 'code',
    codeTask: { language: 'python', template: 'def solve():\n    pass', tests },
  });
  assert.deepEqual(
    expectBodyKeys(jsonBody(lastRequest(api)), ['moduleId', 'title', 'kind', 'codeTask']),
    {
      moduleId: 'm1',
      title: 'Задача',
      kind: 'code',
      codeTask: { language: 'python', template: 'def solve():\n    pass', tests },
    },
  );
});

test('createLesson (code без template): в codeTask только language+tests', async () => {
  api.mockReply({ json: { id: 'l7', title: 'Задача 2', kind: 'code', position: 7 } });
  const tests = [{ name: 'Тест', input: '2', expected: '4', hidden: true }];
  await createLesson('c1', {
    moduleId: 'm1',
    title: 'Задача 2',
    kind: 'code',
    codeTask: { language: 'go', tests },
  });
  assert.deepEqual(
    expectBodyKeys(jsonBody(lastRequest(api)), ['moduleId', 'title', 'kind', 'codeTask']),
    { moduleId: 'm1', title: 'Задача 2', kind: 'code', codeTask: { language: 'go', tests } },
  );
});

test('submitCourse: POST /teacher/courses/{id}/submit без тела', async () => {
  api.mockReply({ json: courseFixture({ status: 'review' }) });
  const res = await submitCourse('c1');
  assert.equal(res.status, 'review');
  const req = lastRequest(api);
  assert.equal(req.method, 'POST');
  assert.equal(req.path, '/api/v1/teacher/courses/c1/submit');
  assert.equal(req.rawBody, '');
});

// ---------- Админка ----------

test('createInvite: POST /admin/invites, тело строго {email}', async () => {
  api.mockReply({
    json: {
      id: 'i1',
      email: 'teacher@example.com',
      token: 'TOKEN-SECRET',
      tokenPrefix: 'TOK',
      expiresAt: '2024-04-01T00:00:00Z',
    },
  });
  const invite = await createInvite('teacher@example.com');
  assert.equal(invite.token, 'TOKEN-SECRET');
  const req = lastRequest(api);
  assert.equal(req.method, 'POST');
  assert.equal(req.path, '/api/v1/admin/invites');
  assert.deepEqual(expectBodyKeys(jsonBody(req), ['email']), { email: 'teacher@example.com' });
});

test('reviewQueue: GET /admin/courses/review, конверт {items,total} → массив', async () => {
  const course = courseFixture({ status: 'review' });
  api.mockReply({ json: { items: [course], total: 1 } });
  const res = await reviewQueue();
  assert.equal(lastRequest(api).path, '/api/v1/admin/courses/review');
  assert.equal(lastRequest(api).method, 'GET');
  assert.deepEqual(res, [course]);
});

test('moderateCourse: POST /admin/courses/{id}/moderate, только {decision} без comment', async () => {
  api.mockReply({ json: courseFixture({ status: 'published' }) });
  const res = await moderateCourse('c1', { decision: 'approve' });
  assert.equal(res.status, 'published');
  const req = lastRequest(api);
  assert.equal(req.method, 'POST');
  assert.equal(req.path, '/api/v1/admin/courses/c1/moderate');
  assert.deepEqual(expectBodyKeys(jsonBody(req), ['decision']), { decision: 'approve' });
});

test('moderateCourse: reject передаёт decision+comment', async () => {
  api.mockReply({ json: courseFixture({ status: 'draft', moderationComment: 'Мало содержания' }) });
  const res = await moderateCourse('c1', { decision: 'reject', comment: 'Мало содержания' });
  assert.equal(res.moderationComment, 'Мало содержания');
  assert.deepEqual(expectBodyKeys(jsonBody(lastRequest(api)), ['decision', 'comment']), {
    decision: 'reject',
    comment: 'Мало содержания',
  });
});

test('getAdminSettings: GET /admin/settings', async () => {
  api.mockReply({ json: { commissionPct: 10 } });
  const settings = await getAdminSettings();
  assert.deepEqual(settings, { commissionPct: 10 });
  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/admin/settings');
});

test('updateAdminSettings: PUT /admin/settings, тело строго {commissionPct}', async () => {
  api.mockReply({ json: { commissionPct: 12.5 } });
  const settings = await updateAdminSettings(12.5);
  assert.deepEqual(settings, { commissionPct: 12.5 });
  const req = lastRequest(api);
  assert.equal(req.method, 'PUT');
  assert.equal(req.path, '/api/v1/admin/settings');
  assert.deepEqual(expectBodyKeys(jsonBody(req), ['commissionPct']), { commissionPct: 12.5 });
});

test('updateAdminSettingsPromo: PUT /admin/settings, тело строго {promoBarEnabled}', async () => {
  api.mockReply({ json: { commissionPct: 10, promoBarEnabled: false } });
  const settings = await updateAdminSettingsPromo(false);
  assert.deepEqual(settings, { commissionPct: 10, promoBarEnabled: false });
  const req = lastRequest(api);
  assert.equal(req.method, 'PUT');
  assert.equal(req.path, '/api/v1/admin/settings');
  assert.deepEqual(expectBodyKeys(jsonBody(req), ['promoBarEnabled']), { promoBarEnabled: false });
});

test('getPublicSettings: GET /settings/public без авторизации', async () => {
  api.mockReply({ json: { promoBarEnabled: true } });
  const settings = await getPublicSettings();
  assert.deepEqual(settings, { promoBarEnabled: true });
  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/settings/public');
});

test('paymentJournal: без параметров — GET /admin/payments без query', async () => {
  api.mockReply({
    json: { items: [], total: 0, page: 1, limit: 20, totals: { amountCents: 0, platformFeeCents: 0, teacherAmountCents: 0 } },
  });
  const page = await paymentJournal();
  assert.equal(page.total, 0);
  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/admin/payments');
  assert.equal(req.query.toString(), '');
});

// ВНИМАНИЕ (forward-compatibility): здесь проверяется НАЛИЧИЕ и значения ожидаемых
// параметров. Отсутствие других параметров не проверяется — задача T38 добавит в этот
// же вызов q/from/to, и этот тест не должен из-за этого падать.
test('paymentJournal: page/limit/status/courseId попадают в query', async () => {
  api.mockReply({
    json: {
      items: [
        {
          id: 'p1',
          studentId: 's1',
          studentEmail: 'student@example.com',
          studentName: 'Иван',
          courseId: 'c1',
          courseTitle: 'React Basics',
          amountCents: 150000,
          platformFeeCents: 15000,
          teacherAmountCents: 135000,
          currency: 'RUB',
          provider: 'mock',
          providerPaymentId: 'mock-1',
          status: 'succeeded',
          createdAt: '2024-02-01T10:00:00Z',
          processedAt: '2024-02-01T10:00:05Z',
        },
      ],
      total: 1,
      page: 3,
      limit: 50,
      totals: { amountCents: 150000, platformFeeCents: 15000, teacherAmountCents: 135000 },
    },
  });
  const page = await paymentJournal({ page: 3, limit: 50, status: 'succeeded', courseId: 'c1' });
  assert.equal(page.total, 1);
  assert.equal(page.totals.teacherAmountCents, 135000);
  assert.equal(page.items[0].studentEmail, 'student@example.com');
  const req = lastRequest(api);
  assert.equal(req.path, '/api/v1/admin/payments');
  assert.equal(req.query.get('page'), '3');
  assert.equal(req.query.get('limit'), '50');
  assert.equal(req.query.get('status'), 'succeeded');
  assert.equal(req.query.get('courseId'), 'c1');
});

test('paymentJournal: частичные параметры — пустые не отправляются (совместимо с T38)', async () => {
  api.mockReply({
    json: { items: [], total: 0, page: 1, limit: 20, totals: { amountCents: 0, platformFeeCents: 0, teacherAmountCents: 0 } },
  });
  await paymentJournal({ status: 'refunded' });
  const req = lastRequest(api);
  assert.equal(req.query.get('status'), 'refunded');
  assert.equal(req.query.get('page'), null, 'undefined не должен попадать в query');
  assert.equal(req.query.get('courseId'), null, 'undefined не должен попадать в query');
});
