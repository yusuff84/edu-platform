// T72: контрактные тесты клиента Q&A по урокам (T71/00-api-contract).
// Проверяем пути и query, тела запросов без лишних полей (DisallowUnknownFields),
// распаковку конвертов {question}/{answer} и тексты ошибок 400/403/404.
// Сервер — мини-API из helper.mjs на фиксированном 127.0.0.1:8799.

import { after, before, beforeEach, test } from 'node:test';
import assert from 'node:assert/strict';
import { startMockApi, lastRequest, jsonBody, expectBodyKeys } from './helper.mjs';
import {
  ApiError,
  answerQuestion,
  askQuestion,
  deleteAnswer,
  deleteQuestion,
  editAnswer,
  editQuestion,
  listLessonQuestions,
} from '../../.tmp-api-test/client.js';

let api;

const OLD_ANSWER = {
  id: 'a1',
  questionId: 'q1',
  authorId: 'u2',
  authorName: 'Пётр',
  authorRole: 'student',
  isTeacher: false,
  body: 'Первый ответ',
  createdAt: '2026-10-19T10:00:00Z',
};

const TEACHER_ANSWER = {
  id: 'a2',
  questionId: 'q1',
  authorId: 't1',
  authorName: 'Мария',
  authorRole: 'teacher',
  isTeacher: true,
  body: 'Ответ преподавателя',
  createdAt: '2026-10-19T11:00:00Z',
};

const QUESTION = {
  id: 'q1',
  courseId: 'c1',
  lessonId: 'l1',
  lessonTitle: 'Введение',
  studentId: 'u1',
  studentName: 'Иван',
  body: 'Как запустить пример из урока?',
  createdAt: '2026-10-19T09:00:00Z',
  answers: [OLD_ANSWER, TEACHER_ANSWER],
};

const PAGE = { items: [QUESTION], total: 1, page: 1, limit: 20 };

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

// ---------- listLessonQuestions: GET ----------

test('listLessonQuestions: lessonId и page уходят в query, конверт возвращается целиком', async () => {
  api.mockReply({ json: PAGE });
  const res = await listLessonQuestions('c1', 'l1', 2);
  assert.deepEqual(res, PAGE);
  assert.equal(res.total, 1);
  assert.deepEqual(res.items[0].answers.map((a) => a.id), ['a1', 'a2']);

  const req = lastRequest(api);
  assert.equal(req.method, 'GET');
  assert.equal(req.path, '/api/v1/courses/c1/questions');
  assert.equal(req.query.get('lessonId'), 'l1');
  assert.equal(req.query.get('page'), '2');
  assert.equal(req.query.toString(), 'lessonId=l1&page=2');
});

test('listLessonQuestions: без lessonId/page — все вопросы курса, пустой query', async () => {
  api.mockReply({ json: { items: [], total: 0, page: 1, limit: 20 } });
  const res = await listLessonQuestions('c1');
  assert.deepEqual(res.items, []);
  assert.equal(res.total, 0);

  const req = lastRequest(api);
  assert.equal(req.path, '/api/v1/courses/c1/questions');
  assert.equal(req.query.toString(), '', 'без параметров query пустой');
});

test('listLessonQuestions: id курса кодируется, lessonId — параметром запроса', async () => {
  api.mockReply({ json: PAGE });
  await listLessonQuestions('c/1', 'l/1', 1);
  const req = lastRequest(api);
  assert.equal(req.path, '/api/v1/courses/c%2F1/questions');
  assert.equal(req.query.get('lessonId'), 'l/1');
  assert.equal(req.query.get('page'), '1');
});

test('listLessonQuestions: 404 урока не из курса → ApiError с текстом сервера', async () => {
  api.mockReply({ status: 404, json: { error: 'Урок не найден в этом курсе' } });
  await expectApiError(
    listLessonQuestions('c1', 'other'),
    404,
    'Урок не найден в этом курсе',
  );
});

// ---------- askQuestion: POST ----------

test('askQuestion: POST /courses/{id}/questions, тело ровно {lessonId, body}', async () => {
  const created = { ...QUESTION, answers: [] };
  api.mockReply({ status: 201, json: { question: created } });
  const res = await askQuestion('c1', 'l1', created.body);
  // Конверт { question } разворачивается в LessonQuestion.
  assert.deepEqual(res, created);
  assert.equal(res.answers.length, 0);

  const req = lastRequest(api);
  assert.equal(req.method, 'POST');
  assert.equal(req.path, '/api/v1/courses/c1/questions');
  const body = expectBodyKeys(jsonBody(req), ['lessonId', 'body']);
  assert.deepEqual(body, { lessonId: 'l1', body: created.body });
});

test('askQuestion: id курса кодируется целиком', async () => {
  api.mockReply({ json: { question: QUESTION } });
  await askQuestion('c/1', 'l1', 'Вопрос достаточной длины');
  assert.equal(lastRequest(api).path, '/api/v1/courses/c%2F1/questions');
});

test('askQuestion: 400 (короткий вопрос) → ApiError с текстом сервера', async () => {
  api.mockReply({ status: 400, json: { error: 'Вопрос: от 10 до 2000 символов' } });
  await expectApiError(
    askQuestion('c1', 'l1', 'коротко'),
    400,
    'Вопрос: от 10 до 2000 символов',
  );
});

test('askQuestion: 403 (нет записи) → ApiError с текстом сервера', async () => {
  api.mockReply({
    status: 403,
    json: { error: 'Задавать вопросы могут только записанные на курс' },
  });
  await expectApiError(
    askQuestion('c1', 'l1', 'Вопрос достаточной длины'),
    403,
    'Задавать вопросы могут только записанные на курс',
  );
});

test('askQuestion: 404 (урок не из курса) → ApiError 404', async () => {
  api.mockReply({ status: 404, json: { error: 'Урок не найден в этом курсе' } });
  await expectApiError(
    askQuestion('c1', 'other', 'Вопрос достаточной длины'),
    404,
    'Урок не найден в этом курсе',
  );
});

// ---------- answerQuestion: POST ----------

test('answerQuestion: POST .../questions/{qid}/answers, тело ровно {body}', async () => {
  api.mockReply({ status: 201, json: { answer: TEACHER_ANSWER } });
  const res = await answerQuestion('c1', 'q1', TEACHER_ANSWER.body);
  // Конверт { answer } разворачивается в LessonAnswer.
  assert.deepEqual(res, TEACHER_ANSWER);
  assert.equal(res.isTeacher, true);
  assert.equal(res.authorRole, 'teacher');

  const req = lastRequest(api);
  assert.equal(req.method, 'POST');
  assert.equal(req.path, '/api/v1/courses/c1/questions/q1/answers');
  const body = expectBodyKeys(jsonBody(req), ['body']);
  assert.deepEqual(body, { body: TEACHER_ANSWER.body });
});

test('answerQuestion: id курса и вопроса кодируются целиком', async () => {
  api.mockReply({ json: { answer: TEACHER_ANSWER } });
  await answerQuestion('c/1', 'q/1', 'Ответ');
  assert.equal(
    lastRequest(api).path,
    '/api/v1/courses/c%2F1/questions/q%2F1/answers',
  );
});

test('answerQuestion: 400 (короткий ответ) → ApiError с текстом сервера', async () => {
  api.mockReply({ status: 400, json: { error: 'Ответ: от 3 до 2000 символов' } });
  await expectApiError(
    answerQuestion('c1', 'q1', 'ok'),
    400,
    'Ответ: от 3 до 2000 символов',
  );
});

test('answerQuestion: 403 (нет записи) → ApiError с текстом сервера', async () => {
  api.mockReply({
    status: 403,
    json: { error: 'Отвечать могут только записанные на курс' },
  });
  await expectApiError(
    answerQuestion('c1', 'q1', 'Ответ'),
    403,
    'Отвечать могут только записанные на курс',
  );
});

test('answerQuestion: 404 (вопрос не найден) → ApiError 404', async () => {
  api.mockReply({ status: 404, json: { error: 'Вопрос не найден' } });
  await expectApiError(answerQuestion('c1', 'nope', 'Ответ'), 404, 'Вопрос не найден');
});

// ---------- editQuestion: PATCH ----------

test('editQuestion: PATCH .../questions/{qid}, тело ровно {body}, конверт распакован', async () => {
  const edited = { ...QUESTION, body: '  Новый текст вопроса  ' };
  api.mockReply({ status: 200, json: { question: edited } });
  const res = await editQuestion('c1', 'q1', edited.body);
  assert.equal(res.body, edited.body);
  assert.equal(res.id, 'q1');

  const req = lastRequest(api);
  assert.equal(req.method, 'PATCH');
  assert.equal(req.path, '/api/v1/courses/c1/questions/q1');
  const body = jsonBody(req);
  expectBodyKeys(body, ['body']);
  assert.equal(body.body, '  Новый текст вопроса  ');
});

test('editQuestion: id курса и вопроса кодируются целиком', async () => {
  api.mockReply({ status: 200, json: { question: QUESTION } });
  await editQuestion('c/1', 'q/1', 'Новый текст вопроса');
  assert.equal(
    lastRequest(api).path,
    '/api/v1/courses/c%2F1/questions/q%2F1',
  );
});

test('editQuestion: 403 (не автор) → ApiError «Нет прав на редактирование»', async () => {
  api.mockReply({ status: 403, json: { error: 'Нет прав на редактирование' } });
  await expectApiError(
    editQuestion('c1', 'q1', 'Чужой текст вопроса'),
    403,
    'Нет прав на редактирование',
  );
});

test('editQuestion: 404 (вопрос не найден) → ApiError 404', async () => {
  api.mockReply({ status: 404, json: { error: 'Вопрос не найден' } });
  await expectApiError(editQuestion('c1', 'nope', 'Текст вопроса'), 404, 'Вопрос не найден');
});

// ---------- editAnswer: PATCH ----------

test('editAnswer: PATCH .../answers/{aid}, тело ровно {body}, конверт распакован', async () => {
  const edited = { ...OLD_ANSWER, body: 'Новый текст ответа' };
  api.mockReply({ status: 200, json: { answer: edited } });
  const res = await editAnswer('c1', 'q1', 'a1', edited.body);
  assert.equal(res.body, edited.body);
  assert.equal(res.id, 'a1');

  const req = lastRequest(api);
  assert.equal(req.method, 'PATCH');
  assert.equal(req.path, '/api/v1/courses/c1/questions/q1/answers/a1');
  const body = jsonBody(req);
  expectBodyKeys(body, ['body']);
  assert.equal(body.body, 'Новый текст ответа');
});

test('editAnswer: id курса, вопроса и ответа кодируются целиком', async () => {
  api.mockReply({ status: 200, json: { answer: OLD_ANSWER } });
  await editAnswer('c/1', 'q/1', 'a/1', 'Новый текст ответа');
  assert.equal(
    lastRequest(api).path,
    '/api/v1/courses/c%2F1/questions/q%2F1/answers/a%2F1',
  );
});

test('editAnswer: 403 (не автор) → ApiError «Нет прав на редактирование»', async () => {
  api.mockReply({ status: 403, json: { error: 'Нет прав на редактирование' } });
  await expectApiError(
    editAnswer('c1', 'q1', 'a1', 'Чужой текст ответа'),
    403,
    'Нет прав на редактирование',
  );
});

test('editAnswer: 404 (ответ не найден) → ApiError 404', async () => {
  api.mockReply({ status: 404, json: { error: 'Ответ не найден' } });
  await expectApiError(editAnswer('c1', 'q1', 'nope', 'Текст ответа'), 404, 'Ответ не найден');
});

// ---------- deleteQuestion: DELETE ----------

test('deleteQuestion: DELETE .../questions/{qid}, 204 → undefined, без тела', async () => {
  api.mockReply({ status: 204 });
  const res = await deleteQuestion('c1', 'q1');
  assert.equal(res, undefined);

  const req = lastRequest(api);
  assert.equal(req.method, 'DELETE');
  assert.equal(req.path, '/api/v1/courses/c1/questions/q1');
  assert.equal(req.rawBody, '', 'у DELETE тела быть не должно');
});

test('deleteQuestion: id кодируются целиком', async () => {
  api.mockReply({ status: 204 });
  await deleteQuestion('c/1', 'q/1');
  assert.equal(lastRequest(api).path, '/api/v1/courses/c%2F1/questions/q%2F1');
});

test('deleteQuestion: 403 (не автор) → ApiError «Нет прав на удаление»', async () => {
  api.mockReply({ status: 403, json: { error: 'Нет прав на удаление' } });
  await expectApiError(deleteQuestion('c1', 'q1'), 403, 'Нет прав на удаление');
});

test('deleteQuestion: 404 (вопрос не найден) → ApiError 404', async () => {
  api.mockReply({ status: 404, json: { error: 'Вопрос не найден' } });
  await expectApiError(deleteQuestion('c1', 'nope'), 404, 'Вопрос не найден');
});

// ---------- deleteAnswer: DELETE ----------

test('deleteAnswer: DELETE .../questions/{qid}/answers/{aid}, 204 → undefined', async () => {
  api.mockReply({ status: 204 });
  const res = await deleteAnswer('c1', 'q1', 'a1');
  assert.equal(res, undefined);

  const req = lastRequest(api);
  assert.equal(req.method, 'DELETE');
  assert.equal(req.path, '/api/v1/courses/c1/questions/q1/answers/a1');
  assert.equal(req.rawBody, '');
});

test('deleteAnswer: id курса, вопроса и ответа кодируются целиком', async () => {
  api.mockReply({ status: 204 });
  await deleteAnswer('c/1', 'q/1', 'a/1');
  assert.equal(
    lastRequest(api).path,
    '/api/v1/courses/c%2F1/questions/q%2F1/answers/a%2F1',
  );
});

test('deleteAnswer: 403 (не автор) → ApiError «Нет прав на удаление»', async () => {
  api.mockReply({ status: 403, json: { error: 'Нет прав на удаление' } });
  await expectApiError(deleteAnswer('c1', 'q1', 'a1'), 403, 'Нет прав на удаление');
});

test('deleteAnswer: 404 (ответ не найден) → ApiError 404', async () => {
  api.mockReply({ status: 404, json: { error: 'Ответ не найден' } });
  await expectApiError(deleteAnswer('c1', 'q1', 'nope'), 404, 'Ответ не найден');
});
