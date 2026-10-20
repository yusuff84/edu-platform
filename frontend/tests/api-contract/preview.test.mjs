// T76: контрактные тесты превью-уроков (P9/T75) в API-клиенте.
//
// Покрыто: getLessonPreview (200, кодирование slug/lessonId, 404),
// передача isPreview при создании урока (createLesson) и при PATCH
// (updateLesson). Тела проверяются точно: backend включает
// DisallowUnknownFields(), а отсутствие isPreview при создании означает false.

import { after, before, beforeEach, test } from 'node:test';
import assert from 'node:assert/strict';
import { startMockApi, lastRequest, jsonBody, expectBodyKeys } from './helper.mjs';
import {
  createLesson,
  getLessonPreview,
  updateLesson,
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

/** Публичный вид урока из GET /courses/{slug}/preview/{lessonId}. */
function lessonFixture(overrides = {}) {
  return {
    id: 'l1',
    moduleId: 'm1',
    title: 'Введение в Python',
    kind: 'video_text',
    position: 1,
    bodyMd: '# Первый урок',
    videoUrl: 'https://video.example/intro',
    isPreview: true,
    ...overrides,
  };
}

/** Конверт ответа превью: { course, lesson }. */
function previewFixture(overrides = {}) {
  return {
    course: { id: 'c1', slug: 'python-basics', title: 'Python с нуля' },
    lesson: lessonFixture(),
    ...overrides,
  };
}

// ---------- getLessonPreview ----------

test('getLessonPreview: GET /courses/{slug}/preview/{lessonId}, конверт как есть', async () => {
  const preview = previewFixture();
  api.mockReply({ json: preview });
  const res = await getLessonPreview('python-basics', 'l1');
  assert.deepEqual(res, preview);
  assert.equal(res.lesson.isPreview, true);

  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/courses/python-basics/preview/l1');
  assert.equal(req.query.toString(), '');
  assert.equal(req.rawBody, '');
});

test('getLessonPreview: slug и lessonId кодируются в pathname', async () => {
  const slug = 'курс 2026/весна';
  const lessonId = 'l 1/2';
  api.mockReply({ json: previewFixture() });
  await getLessonPreview(slug, lessonId);

  const req = lastRequest(api);
  assert.equal(
    req.path,
    `/api/v1/courses/${encodeURIComponent(slug)}/preview/${encodeURIComponent(lessonId)}`,
  );
  // Слэш внутри параметра не должен превращаться в новый сегмент пути.
  assert.ok(req.path.endsWith('/preview/l%201%2F2'));
  assert.ok(req.path.includes('%D0%BA%D1%83%D1%80%D1%81'));
});

test('getLessonPreview: 404 «Урок не найден» → статус и текст сервера', async () => {
  api.mockReply({ status: 404, json: { error: 'Урок не найден' } });
  await assert.rejects(getLessonPreview('python-basics', 'l9'), (err) => {
    assert.ok(err instanceof Error);
    assert.equal(err.status, 404);
    assert.equal(err.message, 'Урок не найден');
    return true;
  });
});

test('getLessonPreview: 404 «Курс не найден» → статус и текст сервера', async () => {
  api.mockReply({ status: 404, json: { error: 'Курс не найден' } });
  await assert.rejects(getLessonPreview('no-course', 'l1'), (err) => {
    assert.equal(err.status, 404);
    assert.equal(err.message, 'Курс не найден');
    return true;
  });
});

// ---------- createLesson: isPreview ----------

test('createLesson: isPreview=true уходит в тело создания', async () => {
  api.mockReply({ status: 201, json: lessonFixture() });
  const res = await createLesson('c1', {
    moduleId: 'm1',
    title: 'Введение в Python',
    kind: 'video_text',
    bodyMd: '# Первый урок',
    videoUrl: 'https://video.example/intro',
    isPreview: true,
  });
  assert.equal(res.isPreview, true);

  const req = lastRequest(api);
  assert.equal(req.method, 'POST');
  assert.equal(req.path, '/api/v1/teacher/courses/c1/lessons');
  assert.deepEqual(
    expectBodyKeys(jsonBody(req), [
      'moduleId',
      'title',
      'kind',
      'bodyMd',
      'videoUrl',
      'isPreview',
    ]),
    {
      moduleId: 'm1',
      title: 'Введение в Python',
      kind: 'video_text',
      bodyMd: '# Первый урок',
      videoUrl: 'https://video.example/intro',
      isPreview: true,
    },
  );
});

test('createLesson: isPreview=false передаётся явно', async () => {
  api.mockReply({ status: 201, json: lessonFixture({ isPreview: false }) });
  await createLesson('c1', {
    moduleId: 'm1',
    title: 'Закрытый урок',
    kind: 'video_text',
    isPreview: false,
  });
  const body = jsonBody(lastRequest(api));
  assert.equal(body.isPreview, false);
});

test('createLesson: без isPreview поле не отправляется (обратная совместимость)', async () => {
  api.mockReply({ status: 201, json: lessonFixture({ isPreview: false }) });
  await createLesson('c1', { moduleId: 'm1', title: 'Урок', kind: 'video_text' });
  assert.deepEqual(expectBodyKeys(jsonBody(lastRequest(api)), ['moduleId', 'title', 'kind']), {
    moduleId: 'm1',
    title: 'Урок',
    kind: 'video_text',
  });
  assert.equal('isPreview' in jsonBody(lastRequest(api)), false);
});

// ---------- updateLesson (PATCH): isPreview ----------

test('updateLesson: PATCH .../lessons/{id} с isPreview=false', async () => {
  api.mockReply({ json: lessonFixture({ isPreview: false }) });
  const res = await updateLesson('c1', 'l1', {
    moduleId: 'm1',
    title: 'Введение в Python',
    kind: 'video_text',
    bodyMd: '# Первый урок',
    videoUrl: '',
    isPreview: false,
  });
  assert.equal(res.isPreview, false);

  const req = lastRequest(api);
  assert.equal(req.method, 'PATCH');
  assert.equal(req.path, '/api/v1/teacher/courses/c1/lessons/l1');
  assert.deepEqual(
    expectBodyKeys(jsonBody(req), [
      'moduleId',
      'title',
      'kind',
      'bodyMd',
      'videoUrl',
      'isPreview',
    ]),
    {
      moduleId: 'm1',
      title: 'Введение в Python',
      kind: 'video_text',
      bodyMd: '# Первый урок',
      videoUrl: '',
      isPreview: false,
    },
  );
});

test('updateLesson: PATCH с isPreview=true и другим kind', async () => {
  api.mockReply({ json: lessonFixture({ kind: 'live', isPreview: true }) });
  await updateLesson('c1', 'l2', {
    moduleId: 'm2',
    title: 'Живой эфир',
    kind: 'live',
    liveUrl: 'https://live.example/room',
    startsAt: '',
    isPreview: true,
  });

  const req = lastRequest(api);
  assert.equal(req.method, 'PATCH');
  assert.equal(req.path, '/api/v1/teacher/courses/c1/lessons/l2');
  assert.deepEqual(
    expectBodyKeys(jsonBody(req), [
      'moduleId',
      'title',
      'kind',
      'liveUrl',
      'startsAt',
      'isPreview',
    ]),
    {
      moduleId: 'm2',
      title: 'Живой эфир',
      kind: 'live',
      liveUrl: 'https://live.example/room',
      startsAt: '',
      isPreview: true,
    },
  );
});

test('updateLesson: 404 урока → статус и текст сервера', async () => {
  api.mockReply({ status: 404, json: { error: 'Урок не найден' } });
  await assert.rejects(
    updateLesson('c1', 'nope', { title: 'X', kind: 'video_text', isPreview: true }),
    (err) => {
      assert.equal(err.status, 404);
      assert.equal(err.message, 'Урок не найден');
      return true;
    },
  );
});
