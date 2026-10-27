// P11: утилитарные тесты навигации админки и возврата после входа.
//
// Компонентной инфраструктуры (jsdom/testing-library) в проекте нет, поэтому
// проверяем чистые helper-функции, которые используют сами страницы/гварды:
//   - AdminEnrollments: чтение/запись courseId в query-параметрах;
//   - AdminCourseDetail: ссылка на записи курса с сохранённым courseId;
//   - RouteGuards: state.from для /login (pathname + search + hash).
//
// Модули грузим через Vite SSR (как соседние тесты api-contract), чтобы
// TSX/CSS обрабатывались настоящим сборщиком, без новой тестовой обвязки.

import { after, before, test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const FRONTEND_ROOT = fileURLToPath(new URL('../..', import.meta.url));

let vite;
let enrollments;
let courseDetail;
let guards;

before(async () => {
  vite = await createServer({
    root: FRONTEND_ROOT,
    configFile: false,
    mode: 'test',
    logLevel: 'silent',
    server: { middlewareMode: true },
    appType: 'custom',
  });
  [enrollments, courseDetail, guards] = await Promise.all([
    vite.ssrLoadModule('/src/pages/AdminEnrollments.tsx'),
    vite.ssrLoadModule('/src/pages/AdminCourseDetail.tsx'),
    vite.ssrLoadModule('/src/components/RouteGuards.tsx'),
  ]);
});

after(async () => {
  if (vite) await vite.close();
});

test('AdminEnrollments: courseId читается из URL', () => {
  assert.equal(
    enrollments.courseIdFromSearchParams(new URLSearchParams('courseId=c1&status=active')),
    'c1',
  );
  assert.equal(enrollments.courseIdFromSearchParams(new URLSearchParams('')), '');
});

test('AdminEnrollments: смена courseId сохраняет остальные query-параметры', () => {
  const original = new URLSearchParams('courseId=c1&status=active&sort=title');
  const next = enrollments.withCourseId(original, 'c2');

  assert.equal(next.get('courseId'), 'c2');
  assert.equal(next.get('status'), 'active');
  assert.equal(next.get('sort'), 'title');
  // Исходный объект не мутируется — его использует React Router.
  assert.equal(original.get('courseId'), 'c1');
});

test('AdminEnrollments: пустой courseId удаляет параметр', () => {
  const next = enrollments.withCourseId(new URLSearchParams('courseId=c1&status=active'), '');
  assert.equal(next.has('courseId'), false);
  assert.equal(next.get('status'), 'active');
});

test('AdminCourseDetail: ссылка на записи сохраняет courseId и кодирует его', () => {
  assert.equal(
    courseDetail.courseEnrollmentsPath('c 1/2'),
    '/admin/enrollments?courseId=c%201%2F2',
  );
});

test('RouteGuards: from для /login содержит pathname, search и hash', () => {
  const state = guards.loginRedirectState({
    pathname: '/admin/courses/c1',
    search: '?tab=students&page=2',
    hash: '#reviews',
  });

  assert.deepEqual(state, {
    from: {
      pathname: '/admin/courses/c1',
      search: '?tab=students&page=2',
      hash: '#reviews',
    },
  });
});
