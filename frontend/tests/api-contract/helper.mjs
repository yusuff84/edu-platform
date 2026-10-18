// Помощник контрактных тестов API-клиента (T37).
//
// Поднимает мини-HTTP-сервер на ФИКСИРОВАННОМ адресе 127.0.0.1:8799 — ровно том,
// куда смотрит собранный клиент (VITE_API_URL из frontend/.env.test). Порт зашит
// в бандл на сборке (npm run test:build), поэтому менять его нельзя.
//
// node:test запускает файлы тестов параллельно в отдельных процессах, а порт один —
// startMockApi сериализует файлы сам: если порт занят (EADDRINUSE), привязка
// повторяется до таймаута, по истечении которого тест падает с понятным сообщением.

import http from 'node:http';
import assert from 'node:assert/strict';

export const TEST_HOST = '127.0.0.1';
export const TEST_PORT = 8799;
/** База, вшитая в собранный клиент (см. frontend/.env.test). */
export const TEST_BASE = `http://${TEST_HOST}:${TEST_PORT}/api/v1`;

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

/**
 * Привязывает сервер к порту, повторяя попытки при EADDRINUSE.
 * Пока порт занят другим файлом тестов — ждём; если занят посторонним
 * процессом дольше timeoutMs — падаем с явным сообщением.
 */
async function listenWithRetry(server, port, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    try {
      await new Promise((resolve, reject) => {
        const onError = (err) => {
          server.removeListener('listening', onListening);
          reject(err);
        };
        const onListening = () => {
          server.removeListener('error', onError);
          resolve();
        };
        server.once('error', onError);
        server.once('listening', onListening);
        server.listen(port, TEST_HOST);
      });
      return;
    } catch (err) {
      if (err?.code !== 'EADDRINUSE') throw err;
      if (Date.now() >= deadline) {
        throw new Error(
          `Порт ${TEST_HOST}:${port} занят другим процессом — тестовый мини-сервер поднять нельзя. ` +
            'Порт фиксирован в frontend/.env.test (VITE_API_URL), поэтому освободите его и повторите npm test.',
        );
      }
      await sleep(150);
    }
  }
}

/**
 * Запускает мини-API и возвращает объект-стенд:
 *  - requests: все принятые запросы {method, url, path, query, headers, rawBody, json};
 *  - mockReply(spec): задаёт ответ для всех последующих запросов
 *    ({status?, json?} или {status?, text?, contentType?});
 *  - close(): останавливает сервер.
 */
export async function startMockApi({ timeoutMs = 60_000 } = {}) {
  /** @type {Array<{method?: string, url?: string, path: string, query: URLSearchParams, headers: import('node:http').IncomingHttpHeaders, rawBody: string, json: unknown}>} */
  const requests = [];
  let reply = { status: 200, json: {} };

  const server = http.createServer((req, res) => {
    const chunks = [];
    req.on('data', (chunk) => chunks.push(chunk));
    req.on('end', () => {
      const rawBody = Buffer.concat(chunks).toString('utf8');
      const url = new URL(req.url ?? '/', `http://${TEST_HOST}:${TEST_PORT}`);
      let json;
      try {
        json = rawBody ? JSON.parse(rawBody) : undefined;
      } catch {
        json = undefined; // не-JSON тело — json остаётся undefined, сырой текст в rawBody
      }
      requests.push({
        method: req.method,
        url: req.url,
        path: url.pathname,
        query: url.searchParams,
        headers: req.headers,
        rawBody,
        json,
      });

      res.statusCode = reply.status ?? 200;
      if (reply.json !== undefined) {
        res.setHeader('Content-Type', 'application/json');
        res.end(JSON.stringify(reply.json));
      } else if (reply.text !== undefined) {
        if (reply.contentType) res.setHeader('Content-Type', reply.contentType);
        res.end(reply.text);
      } else {
        res.end();
      }
    });
  });

  await listenWithRetry(server, TEST_PORT, timeoutMs);

  return {
    requests,
    mockReply(spec) {
      reply = spec;
    },
    close() {
      return new Promise((resolve, reject) => {
        server.close((err) => (err ? reject(err) : resolve()));
      });
    },
  };
}

/** Последний принятый запрос; падает с понятным сообщением, если запросов не было. */
export function lastRequest(api, message = 'клиент не отправил ни одного запроса') {
  assert.ok(api.requests.length > 0, message);
  return api.requests[api.requests.length - 1];
}

/** JSON-тело запроса (объект). Падает, если тело не отправлено или не парсится. */
export function jsonBody(req, message = 'ожидалось JSON-тело запроса') {
  assert.ok(req.json !== undefined && typeof req.json === 'object' && req.json !== null, message);
  return req.json;
}

/**
 * Проверяет, что в теле ровно ожидаемый набор полей — контракт с DisallowUnknownFields():
 * ни одного лишнего ключа.
 */
export function expectBodyKeys(body, keys, message) {
  assert.deepEqual(
    Object.keys(body).sort(),
    [...keys].sort(),
    message ?? `в теле ожидались ровно поля: ${keys.join(', ')}`,
  );
  return body;
}
