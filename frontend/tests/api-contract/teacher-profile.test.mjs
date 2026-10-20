// Контрактные тесты клиента: публичный профиль преподавателя (T82).
// GET /teachers/{id} — публичный (200/404); ответ { teacher, courses }
// возвращается как есть, курсы — массив CourseItem.

import { after, before, beforeEach, test } from 'node:test';
import assert from 'node:assert/strict';
import { startMockApi, lastRequest } from './helper.mjs';
import { ApiError, getTeacherProfile } from '../../.tmp-api-test/client.js';

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

const TEACHER = {
  id: 't-11111111-2222-3333-4444-555555555555',
  displayName: 'Анна Смирнова',
  coursesCount: 2,
  studentsCount: 137,
  avgRating: 4.75,
  ratingsCount: 21,
};

const COURSES = [
  {
    id: 'c-1',
    slug: 'python-basics',
    title: 'Python с нуля',
    description: 'Первый курс по программированию на Python.',
    priceCents: 490000,
    currency: 'RUB',
    avgRating: 4.8,
    ratingsCount: 12,
    category: 'programming',
    coverPath: '/uploads/covers/python.png',
  },
  {
    id: 'c-2',
    slug: 'sql-basics',
    title: 'SQL для начинающих',
    description: 'Запросы, JOIN и агрегаты на практике.',
    priceCents: 0,
    currency: 'RUB',
    avgRating: 4.7,
    ratingsCount: 9,
    category: 'data',
    coverPath: null,
  },
];

// ---------- Успешный профиль ----------

test('getTeacherProfile: GET /teachers/{id}, конверт и курсы как в контракте', async () => {
  api.mockReply({ json: { teacher: TEACHER, courses: COURSES } });
  const res = await getTeacherProfile(TEACHER.id);

  // Поля шапки и карточки курсов не теряются при разборе ответа.
  assert.deepEqual(res, { teacher: TEACHER, courses: COURSES });
  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, `/api/v1/teachers/${TEACHER.id}`);
  assert.equal(req.query.toString(), '');
  assert.equal(req.rawBody, '', 'публичный GET отправляется без тела');
});

test('getTeacherProfile: id URL-кодируется целиком', async () => {
  api.mockReply({ json: { teacher: { ...TEACHER, id: 'teacher 1/2' }, courses: [] } });
  const id = 'teacher 1/2';
  await getTeacherProfile(id);
  assert.equal(lastRequest(api).path, `/api/v1/teachers/${encodeURIComponent(id)}`);
});

test('getTeacherProfile: пустой список курсов → [], не undefined', async () => {
  api.mockReply({ json: { teacher: { ...TEACHER, coursesCount: 0 }, courses: [] } });
  const res = await getTeacherProfile(TEACHER.id);
  assert.deepEqual(res.courses, []);
  assert.equal(res.teacher.coursesCount, 0);
});

// ---------- 404 ----------

test('getTeacherProfile: 404 → ApiError(404) с текстом сервера', async () => {
  api.mockReply({ status: 404, json: { error: 'Преподаватель не найден' } });
  await assert.rejects(
    () => getTeacherProfile('00000000-0000-0000-0000-000000000000'),
    (err) => {
      assert.ok(err instanceof ApiError, 'ожидалась ApiError');
      assert.equal(err.status, 404);
      assert.equal(err.message, 'Преподаватель не найден');
      return true;
    },
  );
  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(
    req.path,
    '/api/v1/teachers/00000000-0000-0000-0000-000000000000',
  );
});
