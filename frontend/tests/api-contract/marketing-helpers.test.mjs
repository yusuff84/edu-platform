// Чистые helper-тесты маркетинга и конструктора (P11).
//
// Лежат в tests/api-contract, потому что npm test запускает только этот каталог.
// HTTP-сервер здесь не нужен: helpers собираются esbuild'ом прямо из исходников
// (esbuild уже стоит как зависимость Vite — новой инфраструктуры не добавляем).

import { after, before, test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { build } from 'esbuild';

let dir;
let marketing;
let marketingWithHook;
let constructor;

/** Собирает TSX-модуль в ESM для импорта из Node. */
async function bundle(entry, outfile, extra = {}) {
  await build({
    entryPoints: [fileURLToPath(new URL(entry, import.meta.url))],
    outfile,
    bundle: true,
    platform: 'node',
    format: 'esm',
    jsx: 'automatic',
    logLevel: 'silent',
    ...extra,
  });
  return import(pathToFileURL(outfile).href);
}

before(async () => {
  dir = await mkdtemp(join(tmpdir(), 'edu-p11-helpers-'));
  marketing = await bundle('../../src/components/marketing.tsx', join(dir, 'marketing.mjs'));
  marketingWithHook = await bundle(
    '../../src/components/marketing.tsx',
    join(dir, 'marketing-hook.mjs'),
    {
      define: {
        'import.meta.env':
          '{"VITE_LEADS_WEBHOOK_URL":"https://hooks.example/leads"}',
      },
    },
  );
  constructor = await bundle('../../src/pages/Constructor.tsx', join(dir, 'constructor.mjs'), {
    // Страница импортирует CSS; для Node он не нужен.
    loader: { '.css': 'empty' },
    define: { 'import.meta.env.VITE_API_URL': '"/api/v1"' },
  });
});

after(async () => {
  if (dir) await rm(dir, { recursive: true, force: true });
});

const LEAD = {
  name: 'Анна',
  contact: '+7 999 123-45-67',
  email: 'anna@example.ru',
  method: 'phone',
  consent: true,
};

// ---------- Валидация заявки ----------

test('validateLead: пустая форма показывает ошибки имени, контакта и согласия', () => {
  const errors = marketing.validateLead({
    name: '',
    contact: '',
    email: '',
    method: 'phone',
    consent: false,
  });
  assert.deepEqual(Object.keys(errors).sort(), ['consent', 'contact', 'name']);
});

test('validateLead: телефон проверяется по количеству цифр, а не по маске', () => {
  assert.ok(
    marketing.validateLead({ ...LEAD, contact: '+7 (999) 123-45-67' }).contact === undefined,
  );
  assert.match(
    marketing.validateLead({ ...LEAD, contact: '123' }).contact,
    /10–15 цифр/,
  );
});

test('validateLead: email необязателен, но неверный адрес не пропускается', () => {
  assert.equal(marketing.validateLead({ ...LEAD, email: '' }).email, undefined);
  assert.match(marketing.validateLead({ ...LEAD, email: 'anna@' }).email, /почты/);
});

test('validateLead: для мессенджера телефонная маска не требуется', () => {
  const errors = marketing.validateLead({
    ...LEAD,
    method: 'telegram',
    contact: '@anna',
  });
  assert.deepEqual(errors, {});
});

// ---------- mailto и текст заявки ----------

test('buildLeadMailto: тема и тело заполнены, адрес — контакт поддержки', () => {
  const href = marketing.buildLeadMailto(LEAD);
  assert.ok(href.startsWith('mailto:hello@edu-platform.ru?subject='), href);

  const url = new URL(href);
  assert.equal(url.searchParams.get('subject'), 'Заявка на консультацию — Анна');
  const body = url.searchParams.get('body');
  assert.match(body, /Имя: Анна/);
  assert.match(body, /Контакт \(По телефону\): \+7 999 123-45-67/);
  assert.match(body, /Email: anna@example\.ru/);
  assert.match(body, /Предпочтительный способ связи: По телефону/);
  assert.match(body, /Согласие на обработку персональных данных: да/);
});

test('buildLeadMailto: способ связи и пустой email попадают в тело корректно', () => {
  const href = marketing.buildLeadMailto({
    ...LEAD,
    method: 'telegram',
    contact: '@anna',
    email: '',
  });
  const body = new URL(href).searchParams.get('body');
  assert.match(body, /Контакт \(В Telegram\): @anna/);
  assert.doesNotMatch(body, /Email:/);
});

test('buildLeadCopyText: к тексту заявки добавлен адрес отправки', () => {
  const text = marketing.buildLeadCopyText(LEAD);
  assert.match(text, /Имя: Анна/);
  assert.match(text, /Отправить на: hello@edu-platform\.ru/);
});

// ---------- Ссылки CTA ----------

test('catalogCategoryHref: ведёт в каталог с фильтром категории', () => {
  assert.equal(marketing.catalogCategoryHref('design'), '/?category=design');
  assert.equal(marketing.catalogCategoryHref('data'), '/?category=data');
});

test('CTA поддержки и мечты не ведут на голый "/"', () => {
  const supportLinks = Object.values(marketing.SUPPORT_TEXT).map((s) => s.to);
  const dreamLinks = marketing.DREAMS.map((d) => d.to);
  for (const href of [...supportLinks, ...dreamLinks]) {
    assert.notEqual(href, '/', 'CTA должен вести на осмысленный маршрут');
  }
  assert.ok(supportLinks.includes(marketing.LEAD_ANCHOR), 'нужен якорь формы заявки');
  assert.ok(
    dreamLinks.some((href) => href.startsWith('/?category=')),
    'нужны ссылки на категории каталога',
  );
  assert.equal(marketing.REFUND_ROUTE, '/#refund');
});

// ---------- Вебхук заявок ----------

test('LEADS_WEBHOOK_URL: без env онлайн-отправка выключена', () => {
  assert.equal(marketing.LEADS_WEBHOOK_URL, '');
});

test('LEADS_WEBHOOK_URL: заданный env включает онлайн-отправку', () => {
  assert.equal(marketingWithHook.LEADS_WEBHOOK_URL, 'https://hooks.example/leads');
});

test('buildLeadPayload: значения обрезаются, состав полей стабилен', () => {
  assert.deepEqual(
    marketing.buildLeadPayload({
      ...LEAD,
      name: '  Анна  ',
      contact: '  +7 (999) 123-45-67  ',
      email: '  anna@example.ru  ',
    }),
    {
      name: 'Анна',
      contact: '+7 (999) 123-45-67',
      email: 'anna@example.ru',
      method: 'phone',
      consent: true,
    },
  );
});

test('sendLead: POST JSON, успех только при 2xx, ошибка повторяема', async () => {
  const calls = [];
  const okFetch = async (url, init) => {
    calls.push({ url, init });
    return { ok: true, status: 204 };
  };
  await marketing.sendLead('https://hooks.example/leads', LEAD, okFetch);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].url, 'https://hooks.example/leads');
  assert.equal(calls[0].init.method, 'POST');
  assert.match(calls[0].init.headers['Content-Type'], /application\/json/);
  assert.deepEqual(JSON.parse(calls[0].init.body), marketing.buildLeadPayload(LEAD));

  let attempts = 0;
  const failingFetch = async () => {
    attempts += 1;
    return { ok: false, status: 500 };
  };
  await assert.rejects(
    () => marketing.sendLead('https://hooks.example/leads', LEAD, failingFetch),
    /500/,
  );
  await assert.rejects(
    () => marketing.sendLead('https://hooks.example/leads', LEAD, failingFetch),
    /500/,
  );
  assert.equal(attempts, 2, 'после ошибки отправку можно повторить');
});

// ---------- Промокод: конец дня по локальному времени ----------

test('promoExpiresAt: 23:59:59 выбранного дня остаётся локальным временем', () => {
  const result = constructor.promoExpiresAt('2026-12-31');
  assert.equal(result, new Date('2026-12-31T23:59:59').toISOString());

  const local = new Date(result);
  assert.equal(local.getFullYear(), 2026);
  assert.equal(local.getMonth(), 11);
  assert.equal(local.getDate(), 31);
  assert.equal(local.getHours(), 23);
  assert.equal(local.getMinutes(), 59);
  assert.equal(local.getSeconds(), 59);
});

test('promoExpiresAt: пустая и некорректная дата → undefined', () => {
  assert.equal(constructor.promoExpiresAt(''), undefined);
  assert.equal(constructor.promoExpiresAt('not-a-date'), undefined);
});
