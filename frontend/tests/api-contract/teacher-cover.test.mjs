// T57: контрактные тесты multipart-клиента обложки курса (api/teacherCover.ts,
// контракт T54: POST/DELETE /teacher/courses/{id}/cover).
//
// Файл автономен: api/teacherCover.ts не входит в тестовый бандл client.ts
// (npm run test:build собирает только src/api/client.ts), поэтому здесь он
// достраивается в свой outDir (.tmp-api-test/cover) той же командой vite,
// что и test:build. package.json при этом не трогается; outDir вынесен
// в подкаталог .tmp-api-test (в gitignore) и содержит только свои файлы —
// параллельные тест-файлы, импортирующие .tmp-api-test/client.js, не задеваются.

import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { after, before, beforeEach, test } from 'node:test';
import assert from 'node:assert/strict';
import { lastRequest, startMockApi, TEST_BASE } from './helper.mjs';

const FRONTEND_ROOT = fileURLToPath(new URL('../..', import.meta.url));
const COVER_OUT_DIR = '.tmp-api-test/cover';

let api;
let cover; // собранный модуль teacherCover.js

/** Собирает api/teacherCover.ts обычной тестовой сборкой vite (--mode test). */
async function buildCoverModule() {
  const viteBin = fileURLToPath(
    new URL('../../node_modules/vite/bin/vite.js', import.meta.url),
  );
  const run = spawnSync(
    process.execPath,
    [
      viteBin,
      'build',
      '--ssr',
      'src/api/teacherCover.ts',
      '--outDir',
      COVER_OUT_DIR,
      '--emptyOutDir',
      '--mode',
      'test',
    ],
    { cwd: FRONTEND_ROOT, encoding: 'utf8' },
  );
  assert.equal(
    run.status,
    0,
    `сборка teacherCover.ts провалилась:\n${run.stdout ?? ''}\n${run.stderr ?? ''}`,
  );
  // Клиент собран с VITE_API_URL из .env.test — тот же адрес, что и у helper.
  return import(new URL(`../../${COVER_OUT_DIR}/teacherCover.js`, import.meta.url).href);
}

/**
 * Файл-заглушка как будто выбран из input[type=file]: имя, тип, байты.
 * Содержимое — ASCII-совместимые байты: helper.mjs читает тело как utf8-строку,
 * произвольная бинарь-сигнатура (0x89…) через такое обратное декодирование
 * не сравнивается (см. отчёт T57: «не покрыто»).
 */
function pngFile(name = 'cover.png') {
  const text = 'SIGN-P7-UPLOAD-CHECK';
  const bytes = new Uint8Array(text.length);
  for (let i = 0; i < text.length; i++) bytes[i] = text.charCodeAt(i);
  return new File([bytes], name, { type: 'image/png' });
}

before(async () => {
  api = await startMockApi();
  cover = await buildCoverModule();
});

beforeEach(() => {
  api.requests.length = 0;
});

after(async () => {
  await api.close();
});

// ---------- uploadCover: multipart ----------

test('uploadCover: POST /teacher/courses/{id}/cover, поле file в FormData', async () => {
  api.mockReply({ status: 201, json: { coverPath: '/uploads/covers/cover-1.png' } });
  const file = pngFile();
  const res = await cover.uploadCover('c1', file);
  assert.deepEqual(res, { coverPath: '/uploads/covers/cover-1.png' });
  const req = lastRequest(api);
  assert.equal(req.method, 'POST');
  assert.equal(req.path, '/api/v1/teacher/courses/c1/cover');
  assert.equal(req.query.toString(), '');
  // Ручного Content-Type быть не должно: границу проставляет браузер/undici,
  // приложение/json сломало бы парсинг формы на бэкенде.
  assert.ok(
    (req.headers['content-type'] ?? '').startsWith('multipart/form-data; boundary='),
    `content-type=${req.headers['content-type']}`,
  );
  // Поле формы называется file, файл под своим именем и с image/png.
  assert.ok(req.rawBody.includes('name="file"'), 'нет поля file в multipart-теле');
  assert.ok(req.rawBody.includes('filename="cover.png"'), 'имя файла потерялось');
  assert.ok(req.rawBody.includes('Content-Type: image/png'), 'тип файла потерялся');
});

test('uploadCover: содержимое файла приезжает в multipart-теле', async () => {
  api.mockReply({ status: 201, json: { coverPath: '/uploads/covers/cover-2.png' } });
  const file = pngFile('cover-2.png');
  await cover.uploadCover('c1', file);
  assert.ok(
    lastRequest(api).rawBody.includes('SIGN-P7-UPLOAD-CHECK'),
    'содержимое файла потерялось',
  );
});

test('uploadCover: id курса кодируется целиком (encodeURIComponent)', async () => {
  api.mockReply({ status: 201, json: { coverPath: '/uploads/covers/x.png' } });
  await cover.uploadCover('c/1', pngFile());
  assert.equal(lastRequest(api).path, '/api/v1/teacher/courses/c%2F1/cover');
});

test('uploadCover: 400 {"error": …} → ошибка со статусом и текстом сервера', async () => {
  api.mockReply({
    status: 400,
    json: { error: 'Обложка: допустимы JPEG, PNG или WebP не больше 5 МБ' },
  });
  await assert.rejects(cover.uploadCover('c1', pngFile()), (err) => {
    assert.ok(err instanceof Error);
    assert.equal(err.status, 400);
    assert.equal(err.message, 'Обложка: допустимы JPEG, PNG или WebP не больше 5 МБ');
    return true;
  });
});

test('uploadCover: 409 архивного курса → текст сервера, ошибка со статусом', async () => {
  api.mockReply({ status: 409, json: { error: 'Курс в архиве: обложка заморожена' } });
  await assert.rejects(cover.uploadCover('c1', pngFile()), (err) => {
    assert.ok(err instanceof Error);
    assert.equal(err.status, 409);
    assert.equal(err.message, 'Курс в архиве: обложка заморожена');
    return true;
  });
});

test('uploadCover: ошибка с не-JSON телом → запасной текст со статусом', async () => {
  api.mockReply({ status: 500, text: 'boom', contentType: 'text/plain' });
  await assert.rejects(cover.uploadCover('c1', pngFile()), (err) => {
    assert.ok(err instanceof Error);
    assert.equal(err.status, 500);
    assert.equal(err.message, 'Ошибка запроса (500)');
    return true;
  });
});

test('uploadCover: 401 без текста → «Требуется вход в аккаунт»', async () => {
  api.mockReply({ status: 401 });
  await assert.rejects(cover.uploadCover('c1', pngFile()), (err) => {
    assert.equal(err.status, 401);
    assert.equal(err.message, 'Требуется вход в аккаунт');
    return true;
  });
});

// ---------- removeCover: DELETE ----------

test('removeCover: DELETE /teacher/courses/{id}/cover, 204 → undefined', async () => {
  api.mockReply({ status: 204 });
  const res = await cover.removeCover('c1');
  assert.equal(res, undefined);
  const req = lastRequest(api);
  assert.equal(req.method, 'DELETE');
  assert.equal(req.path, '/api/v1/teacher/courses/c1/cover');
  assert.equal(req.rawBody, '');
  assert.equal(req.headers['content-type'], undefined, 'у DELETE тела быть не должно');
});

test('removeCover: обложки не было → тот же 204 (контракт T54)', async () => {
  api.mockReply({ status: 204 });
  await assert.doesNotReject(cover.removeCover('c1'));
});

test('removeCover: 409 архивного курса → ошибка со статусом и текстом', async () => {
  api.mockReply({ status: 409, json: { error: 'Курс в архиве: обложка заморожена' } });
  await assert.rejects(cover.removeCover('c1'), (err) => {
    assert.ok(err instanceof Error);
    assert.equal(err.status, 409);
    assert.equal(err.message, 'Курс в архиве: обложка заморожена');
    return true;
  });
});

test('removeCover: 404 без курса → ошибка со статусом и текстом сервера', async () => {
  api.mockReply({ status: 404, json: { error: 'Курс не найден' } });
  await assert.rejects(cover.removeCover('nope'), (err) => {
    assert.equal(err.status, 404);
    assert.equal(err.message, 'Курс не найден');
    return true;
  });
});

// ---------- coverUrl: coverPath → URL для <img> ----------

test('coverUrl: пустое и nullish — undefined (плитка как раньше)', () => {
  assert.equal(cover.coverUrl(undefined), undefined);
  assert.equal(cover.coverUrl(null), undefined);
  assert.equal(cover.coverUrl(''), undefined);
});

test('coverUrl: подставляет origin тестовой базы из .env.test', () => {
  const origin = new URL(TEST_BASE).origin; // http://127.0.0.1:8799
  assert.equal(cover.coverUrl('/uploads/covers/a.png'), `${origin}/uploads/covers/a.png`);
  assert.equal(cover.coverUrl('/uploads/covers/a.png').startsWith(TEST_BASE), false,
    'путь не должен склеиваться с /api/v1');
});

// ---------- константы контракта ----------

test('константы T54: accept из трёх типов, лимит 5 МБ', () => {
  assert.equal(cover.COVER_ACCEPT, 'image/jpeg,image/png,image/webp');
  assert.deepEqual([...cover.COVER_TYPES], ['image/jpeg', 'image/png', 'image/webp']);
  assert.equal(cover.COVER_MAX_BYTES, 5 * 1024 * 1024);
});
