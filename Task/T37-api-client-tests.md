# T37 — Контрактные тесты API-клиента без новых зависимостей (frontend)

**Волна:** P4 · **Ветка:** `task/T37-api-client-tests` · **Worktree:** `wt/t37`

## Зачем

У фронтенда нет ни одного автотеста: 29 функций клиента (`frontend/src/api/client.ts`) собирают
URL, query и тела запросов «на глаз», а бэкенд с `DisallowUnknownFields()` отвечает `400` на любое
лишнее поле. Нужен дешёвый способ проверять контракт клиента в CI, **без установки новых пакетов**
(правило проекта: новых зависимостей нет).

## Как (подход уже проверен координатором — не изобретать заново)

1. `vite build --ssr src/api/client.ts` собирает клиент в обычный ESM-модуль Node
   (проверено: 29 экспортов, запросы уходят по адресу, вшитому из `VITE_API_URL`).
2. Файл `.env.test` (коммитим): `VITE_API_URL=http://127.0.0.1:8799/api/v1`.
3. `package.json`:
   - `"test:build": "vite build --ssr src/api/client.ts --outDir .tmp-api-test --emptyOutDir --mode test"`
   - `"test": "npm run test:build && node --test tests/api-contract"`
4. Тесты — `frontend/tests/api-contract/*.test.mjs` (чистый JS, `node:test` + `node:assert/strict`).
   Внутри: поднять `http.createServer` на **127.0.0.1:8799**, писать в массив все пришедшие
   `{method, url, headers, body}`, отвечать заранее заготовленным JSON; импортировать
   `../../.tmp-api-test/client.js` и вызывать функции. Порт фиксирован (он вшит в сборку при
   `VITE_API_URL`) — при занятом порте тест обязан падать с понятным сообщением.
5. `.tmp-api-test/` добавить в корневой `.gitignore` (папка сборки теста, в репозиторий не попадает).

## Что покрыть (не меньше 25 проверок)

- транспорт: `apiFetch` → ошибка `400 {"error":"…"}` превращается в `ApiError` с `status` и
  этим текстом; не-JSON тело ошибки → текст по умолчанию; `401` без текста → текст про сессию;
  `204` → `undefined`; успешный JSON разбирается; заголовок `Content-Type: application/json` на
  POST/PATCH; `method` по умолчанию `GET`;
- `buildQuery`: `undefined`/пустая строка выбрасываются, числа приводятся, значение URL-кодируется,
  пустой объект → `''`;
- auth: `register`, `login`, `logout`, `me` — методы, пути, тела (только разрешённые поля);
- каталог: `listCourses` (q/page/limit), `getCourse(slug)` (slug кодируется),
  `listReviews(courseId)`, `createReview` (тело);
- обучение: `enroll`, `myCourses`, `completeLesson` (payload для теста/видео), `formatDate`/`formatMoney`
  (чистые функции — можно юнит-тестом без сервера);
- преподавание: `myTeaching`, `createCourse`, `createModule`, `createLesson`, `submitCourse`,
  `createInvite`;
- админка: `reviewQueue`, `moderateCourse`, `getAdminSettings`, `updateAdminSettings`,
  `paymentJournal` — проверяй **наличие** ожидаемых параметров (`page`, `limit`, `status`, `courseId`)
  и не требуй отсутствия других: T38 добавит `q`/`from`/`to` в тот же вызов, тесты не должны из-за
  этого падать.

Правило: ассерты — по конкретным ожидаемым значениям (метод + путь + ключевые query + тело),
никаких «просто не пусто». В одном файле на группу (transport / auth+catalog / teaching+admin),
помощники — в `frontend/tests/api-contract/helper.mjs`.

## Ограничения

- Править только: `frontend/tests/api-contract/**` (новое), `frontend/.env.test` (новое),
  `frontend/package.json` (только `scripts`), корневой `.gitignore` (только строка `.tmp-api-test/`),
  отчёт `frontend/tests/T37-api-client.md`.
- `frontend/src/**` — не менять вообще: клиент тестируется как есть (если тест находит баг — описать
  в отчёте, но не править; координатор распределит).
- `frontend/src/i18n/ru.json`, `Task/**`, `specs/**` — не трогать. Новых пакетов не добавлять.

## Проверка

```powershell
cd C:\Users\96352\all_project\eduPlatform\edu-platform\frontend
npm test        # должно быть: build ок + все тесты pass, exit 0
npx tsc --noEmit
npx vite build  # обычная сборка не должна пострадать от .env.test
```

Все три — `exit 0`. В отчёте — вывод `npm test` целиком (с числом тестов).

## Отчёт

`frontend/tests/T37-api-client.md`: список файлов тестов, покрытие по функциям (таблица
«функция → что проверено»), вывод `npm test`, честный список непокрытого (например, `getTeacherCourse`
из `api/teacherStudents.ts` — он живёт вне `client.ts` и в этом тесте не собирается).
