// T86: контрактные тесты клиента глобальной модерации Q&A (T85).
// Проверяем query GET /admin/questions, форму ответа с веткой answers и
// DELETE вопроса/ответа через существующие пути P9 (204, без тела).
// Сервер — мини-API из helper.mjs на фиксированном 127.0.0.1:8799.
//
// test:build собирает только client.ts, поэтому adminConsole.ts собираем здесь
// же Vite-ом в подкаталог .tmp-api-test (каталог в .gitignore) и импортируем
// готовый ESM: Node не умеет исполнять .ts напрямую.

import { after, before, beforeEach, test } from 'node:test';
import assert from 'node:assert/strict';
import { rm } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { build } from 'vite';
import { startMockApi, lastRequest } from './helper.mjs';

const FRONTEND_ROOT = fileURLToPath(new URL('../..', import.meta.url));
const OUT_DIR = path.join(FRONTEND_ROOT, '.tmp-api-test', 'admin-console');

let api;
let adminConsole;

const ANSWER = {
  id: 'a1',
  authorId: 'u2',
  authorEmail: 'teacher@example.test',
  authorName: 'Мария',
  authorRole: 'teacher',
  body: 'Ответ преподавателя',
  createdAt: '2026-10-24T10:05:00Z',
};

const QUESTION = {
  id: 'q1',
  courseId: 'c1',
  courseTitle: 'Основы Python',
  courseSlug: 'python-basics',
  lessonId: 'l1',
  lessonTitle: 'Введение',
  studentId: 'u1',
  studentEmail: 'student@example.test',
  studentName: 'Иван',
  body: 'Как запустить пример из урока?',
  createdAt: '2026-10-24T10:00:00Z',
  answers: [ANSWER],
};

const PAGE = { items: [QUESTION], total: 1, page: 1, limit: 20 };

before(async () => {
  // Отдельная сборка модуля-компаньона: конфиг проекта не нужен, режим test
  // подхватывает frontend/.env.test с адресом мини-API.
  await build({
    configFile: false,
    root: FRONTEND_ROOT,
    mode: 'test',
    logLevel: 'silent',
    build: {
      ssr: 'src/api/adminConsole.ts',
      outDir: OUT_DIR,
      emptyOutDir: true,
      rollupOptions: {
        output: { entryFileNames: 'adminConsole.mjs' },
      },
    },
  });
  adminConsole = await import(pathToFileURL(path.join(OUT_DIR, 'adminConsole.mjs')).href);
  api = await startMockApi();
});

beforeEach(() => {
  api.requests.length = 0;
});

after(async () => {
  if (api) await api.close();
  await rm(OUT_DIR, { recursive: true, force: true });
});

/** Проверяет, что промис отклонён ApiError с нужными статусом и текстом. */
async function expectApiError(promise, status, message) {
  await assert.rejects(promise, (err) => {
    assert.ok(err instanceof Error, 'ожидалась Error');
    assert.equal(err.status, status);
    assert.equal(err.message, message);
    return true;
  });
}

// ---------- listAdminQuestions: GET ----------

test('listAdminQuestions: courseId/lessonId/q/page/limit уходят в query', async () => {
  api.mockReply({ json: PAGE });
  const res = await adminConsole.listAdminQuestions({
    courseId: 'c1',
    lessonId: 'l1',
    q: 'Python',
    page: 2,
    limit: 50,
  });
  // Конверт возвращается целиком, включая ветку answers.
  assert.deepEqual(res, PAGE);
  assert.equal(res.items[0].answers[0].authorRole, 'teacher');
  assert.equal(res.items[0].courseSlug, 'python-basics');

  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/admin/questions');
  assert.equal(req.query.get('courseId'), 'c1');
  assert.equal(req.query.get('lessonId'), 'l1');
  assert.equal(req.query.get('q'), 'Python');
  assert.equal(req.query.get('page'), '2');
  assert.equal(req.query.get('limit'), '50');
  assert.equal(
    req.query.toString(),
    'courseId=c1&lessonId=l1&q=Python&page=2&limit=50',
  );
});

test('listAdminQuestions: без аргументов — пустой query, items может быть пустым', async () => {
  api.mockReply({ json: { items: [], total: 0, page: 1, limit: 20 } });
  const res = await adminConsole.listAdminQuestions();
  assert.deepEqual(res.items, []);
  assert.equal(res.total, 0);

  const req = lastRequest(api);
  assert.equal(req.path, '/api/v1/admin/questions');
  assert.equal(req.query.toString(), '', 'пустые фильтры buildQuery пропускает');
});

test('listAdminQuestions: пустые строки фильтров не попадают в query', async () => {
  api.mockReply({ json: PAGE });
  await adminConsole.listAdminQuestions({
    courseId: '',
    lessonId: '',
    q: '',
    page: 1,
    limit: 20,
  });

  const req = lastRequest(api);
  assert.equal(req.query.toString(), 'page=1&limit=20');
});

test('listAdminQuestions: q и UUID кодируются URLSearchParams', async () => {
  api.mockReply({ json: PAGE });
  await adminConsole.listAdminQuestions({ courseId: 'c/1', q: 'вопрос & ответ' });

  const req = lastRequest(api);
  assert.equal(req.query.get('courseId'), 'c/1');
  assert.equal(req.query.get('q'), 'вопрос & ответ');
  assert.match(req.query.toString(), /courseId=c%2F1/);
});

test('listAdminQuestions: 400 неверного UUID → ApiError с текстом сервера', async () => {
  api.mockReply({ status: 400, json: { error: 'Некорректный courseId' } });
  await expectApiError(
    adminConsole.listAdminQuestions({ courseId: 'not-a-uuid' }),
    400,
    'Некорректный courseId',
  );
});

// ---------- deleteAdminQuestion: DELETE ----------

test('deleteAdminQuestion: DELETE /courses/{courseId}/questions/{questionId}, 204', async () => {
  api.mockReply({ status: 204 });
  const res = await adminConsole.deleteAdminQuestion('c1', 'q1');
  assert.equal(res, undefined);

  const req = lastRequest(api);
  assert.equal(req.method, 'DELETE');
  assert.equal(req.path, '/api/v1/courses/c1/questions/q1');
  assert.equal(req.rawBody, '', 'у DELETE тела быть не должно');
});

test('deleteAdminQuestion: id курса и вопроса кодируются целиком', async () => {
  api.mockReply({ status: 204 });
  await adminConsole.deleteAdminQuestion('c/1', 'q/1');
  assert.equal(lastRequest(api).path, '/api/v1/courses/c%2F1/questions/q%2F1');
});

test('deleteAdminQuestion: 404 вопроса → ApiError 404', async () => {
  api.mockReply({ status: 404, json: { error: 'Вопрос не найден' } });
  await expectApiError(adminConsole.deleteAdminQuestion('c1', 'nope'), 404, 'Вопрос не найден');
});

// ---------- deleteAdminAnswer: DELETE ----------

test('deleteAdminAnswer: DELETE .../questions/{qid}/answers/{aid}, 204', async () => {
  api.mockReply({ status: 204 });
  const res = await adminConsole.deleteAdminAnswer('c1', 'q1', 'a1');
  assert.equal(res, undefined);

  const req = lastRequest(api);
  assert.equal(req.method, 'DELETE');
  assert.equal(req.path, '/api/v1/courses/c1/questions/q1/answers/a1');
  assert.equal(req.rawBody, '');
});

test('deleteAdminAnswer: id курса, вопроса и ответа кодируются целиком', async () => {
  api.mockReply({ status: 204 });
  await adminConsole.deleteAdminAnswer('c/1', 'q/1', 'a/1');
  assert.equal(
    lastRequest(api).path,
    '/api/v1/courses/c%2F1/questions/q%2F1/answers/a%2F1',
  );
});

test('deleteAdminAnswer: 404 ответа → ApiError 404', async () => {
  api.mockReply({ status: 404, json: { error: 'Ответ не найден' } });
  await expectApiError(
    adminConsole.deleteAdminAnswer('c1', 'q1', 'nope'),
    404,
    'Ответ не найден',
  );
});
