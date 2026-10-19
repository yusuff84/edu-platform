# T57 — Фронтенд: обложка курса в конструкторе, каталоге и на странице

**Волна:** P7 · **Ветка:** `task/T57-course-cover-ui` · **Статус:** готово, `npm run build` и `npm test` — зелёные (см. «Проверка»).

Реализация под замороженный контракт T54 из `Task/00-api-contract.md` (раздел
«Эндпоинты волны P7»): `POST/DELETE /teacher/courses/{id}/cover` и аддитивное
`coverPath` в ответах. Бэкенд (T54) идёт параллельно — в этой ветке хендлеров
cover ещё нет, фронт написан строго под контракт.

## Файлы

| Файл | Что сделано |
| --- | --- |
| `frontend/src/api/teacherCover.ts` **(НОВЫЙ)** | `uploadCover(courseId, file)` → POST multipart `/teacher/courses/{id}/cover` (поле `file`, без ручного Content-Type) → `{coverPath}`; `removeCover(courseId)` → DELETE → `204/void`; `coverUrl(path)` — обёртка для `<img src>`; константы `COVER_ACCEPT`/`COVER_TYPES`/`COVER_MAX_BYTES`. Ошибки — `ApiError`. |
| `frontend/src/api/types.ts` | аддитивно: `CourseItem.coverPath?: string \| null`; существующее `Course.coverPath` расширено `string \| null` (DELETE обнуляет `cover_path`, контракт говорит `string \| null`). `CourseDetail` наследует. |
| `frontend/src/pages/Constructor.tsx` | блок «Обложка» в карточке курса (только при `course.id`): превью `<img alt={t('p7.cover.alt')}>`, кнопки загрузить/заменить/убрать (inline-подтверждение без `confirm()`), hidden `input[type=file]` с `accept`, клиентские проверки ≤5 МБ и типа → `p7.cover.typeError` без запроса; тосты `p7.cover.saved/removed`; refetch `['teach-course']` + `['teaching']`; `archived` — все кнопки disabled. |
| `frontend/src/pages/Course.tsx` | вся страница (все три ветки рендера, включая loading/404/error) обёрнута в `<main className="page">`; при `course.coverPath` — обложка-хиро в шапке панели курса и настоящий кадр в вилке покупки вместо клише-градиента `cover-*`; без обложки — прежний вид без изменений. |
| `frontend/src/components/CourseCard.tsx` | плитка каталога: при `course.coverPath` поверх градиента — мини-превью `.tile-cover-img` (`object-fit: cover`, `loading="lazy"`, `onError` → картинка скрывается и остаётся прежний градиент, «битой пикчю» не видно). |
| `frontend/src/pages/Catalog.tsx` | **правок не потребовалось** — плитки каталога целиком рендерятся `CourseCard`, cover-логики на уровне страницы нет (см. «Отклонения»). |
| `frontend/src/styles/constructor.css` | `tc-cover-img`, `tc-cover-input`, `tc-cover-actions`, `tc-cover-confirm`. |
| `frontend/src/styles/catalog.css` | `cover-hero`, `cover-buy` (публичная страница), `tile-cover-img` (плитка). |
| `frontend/tests/api-contract/teacher-cover.test.mjs` **(НОВЫЙ)** | 14 юнит-тестов multipart-клиента: поле `file` в FormData, multipart content-type с границей (никакого `application/json`), сохранность содержимого файла, `encodeURIComponent` в пути, 400/409/401/404/500 → ошибка со статусом/текстом, 204 → `void`, `coverUrl`, константы T54. |

## Транспорт multipart (почему fetch вручную, а не apiFetch)

`apiFetch` в `api/client.ts` всегда ставит заголовок `Content-Type: application/json`
— для multipart он недопустим (бэкенд не увидит границы формы). `client.ts`
править нельзя (зона T01/T16), поэтому в `teacherCover.ts` транспорт собран
вручную, повторяя механику `apiFetch`:

- тот же `BASE` (`VITE_API_URL` из окружения, иначе `/api/v1`);
- `credentials: 'include'` (cookie-сессии);
- `Content-Type` **не ставится** — границу проставляет браузер/undici вместе
  с `FormData`;
- 204 → `undefined`, ошибки → `ApiError(status, message)` с текстом из
  envelope `{"error": "…"}`, запасные тексты «Требуется вход в аккаунт» /
  «Ошибка запроса (N)» — как в `apiFetch`.

Про CSRF-заголовок: в текущем `api/client.ts` он **не прикрепляется вообще**
(проверено: единственная механика — cookie через `credentials:'include'`,
Origin проставляет браузер; `X-CSRF-Token` нигде в фронтенде нет, включая
`crudFetch` из Constructor.tsx и все модули-компаньоны). «Тем же механизмом,
что и apiFetch» — значит, никаких самодельных заголовков; сессия cookie
уезжает с запросом автоматически. Если бэкенд каким-то образом потребует
заголовок — путь доступа повторить не из чего, нужны сведения от T54.

## Ключи i18n

Использованы **только замороженные** ключи:

- `p7.cover.label | hint | upload | replace | remove | saved | removed | typeError | alt`
  — все существуют в `ru.json` (строки 547–555);
- существующие: `teach.saving`, `common.confirm`, `common.cancel`.

Новых ключей не нужно — нехватки не обнаружено.

## Тесты: как устроен отдельный бандл для teacherCover

`npm test` собирает через `vite build --ssr src/api/client.ts` только клиент —
`teacherCover.ts` в него не попадает, а править `package.json` нельзя
(не в списке разрешённых файлов). Поэтому `teacher-cover.test.mjs`
автономен: в `before` он той же командой (`--mode test`, тот же `vite` из
`node_modules`) строит `src/api/teacherCover.ts` в `.tmp-api-test/cover/`
(подкаталог уже gitignored-каталога `.tmp-api-test`; `--emptyOutDir` опустошает
только его, параллельные тест-файлы, импортирующие `.tmp-api-test/client.js`,
не задеваются) и динамически импортирует собранный модуль. Мини-сервер —
общий `helper.mjs` (порт 8799, сериализация через retry, как в T37).

Замечание: `ApiError` не импортируется из двух бандлов и не сравнивается через
`instanceof` — у каждого бандла своя копия класса, поэтому в тестах
ошибки проверяются структурно (`instanceof Error` + `status` + `message`).

## Покрытие по функциям

| Функция | Что проверено |
| --- | --- |
| `uploadCover` | POST-путь `/api/v1/teacher/courses/{id}/cover`; поле формы **`file`** + filename + `Content-Type: image/png` в теле; content-type запроса — `multipart/form-data; boundary=…` (не `application/json`); содержимое файла в теле; `encodeURIComponent` для id; 201 → `{coverPath}`; 400/409 → текст и статус сервера; 500 с не-JSON → «Ошибка запроса (500)»; 401 без текста → «Требуется вход в аккаунт» |
| `removeCover` | DELETE-путь; тело пустое, без Content-Type; 204 → `undefined`; обложки не было → тот же 204; 409/404 → `ApiError` со статусом/текстом |
| `coverUrl` | `null/undefined/''` → `undefined` (плитка остаётся прежней); для абсолютного `VITE_API_URL` — путь приклеивается к origin тестовой базы, не к `/api/v1` |
| константы | `COVER_ACCEPT` = `image/jpeg,image/png,image/webp`; `COVER_TYPES` — те же три типа; `COVER_MAX_BYTES` = 5 МБ (совпадает с сервером) |

Плюс неизменные 61 тест T37 — ни один не сломан.

## Не покрыто (честно)

- Байт-в-байт передача бинарного содержимого (сигнатура PNG): helper.mjs
  читает тело запроса как utf8-строку, произвольная бинарная сигнатура
  (0x89 0x50…) через обратное `Buffer.from( … , 'utf8')` не восстанавливается.
  Проверено содержимое файла на ASCII-совместимых байтах — оно доезжает как есть.
- `credentials: 'include'` не проверяется (действует замечание T37: Node-fetch
  не проявляет его в наблюдаемых заголовках без cookie-jar).
- UI-страницы (Constructor/Course/Catalog/CourseCard) — приёмка живьём за
  координатором; программные проверки реактивности (refetch, disabled в
  архиве, typeError до запроса) напрямую не тестируются.

## UI-поведение (кратко)

- **Конструктор** (`/teach/:id`, блок «Обложка» в карточке курса):
  - нет обложки → кнопка «Загрузить обложку»; есть → «Заменить обложку» + «Убрать обложку»;
  - «Убрать обложку» → inline-подтверждение (вопрос + Подтвердить/Отмена), без `window.confirm`;
  - превью `<img>` показывается сразу после `coverPath` (refetch целиком);
  - выбор неподдерживаемого типа/`>`5 МБ блокируется на клиенте — `p7.cover.typeError`
    без запроса (перевыбор файла работает: `input.value` сбрасывается);
  - успех/неуспех — общий тост страницы (`saved`/`removed`/текст `ApiError`);
  - `status: archived` → все cover-кнопки disabled (бэкенд и так вернул бы 409).
- **Публичная страница** (`/courses/:slug`): герой-обложка в шапке + реальный кадр
  в вилке покупки; всё в `<main className="page">`; без обложки страница выглядит по-старому.
- **Каталог**: плитка с обложкой показывает кадр поверх градиента; без обложки — прежний вид.

## Проверка

| Команда | Результат |
| --- | --- |
| `npx tsc --noEmit` | exit 0 |
| `npm run build` | exit 0; `tsc --noEmit && vite build`, 139 модулей, `✓ built in 3m 37s` |
| `npm test` | exit 0: `test:build` ок (client.js 7.46 kB), `node --test` → **75/75 pass, 0 fail** (T37 = 61 + T57 = 14) |

## Отклонения от плана (сознательные)

1. `Catalog.tsx` не изменён: плитки каталога рендерит `CourseCard`, страница
   не имеет собственной cover-логики; мини-превью добавлено там, где рисуется
   плитка (единственное место). Список разрешённых файлов соблюдён.
2. `Course.coverPath` уже существовал в `types.ts` (`string` без null) — вместо
   добавления второго поля тип расширен до контрактного `string | null`.
3. Тестовый файл сам строит бандл `teacherCover` (без правки `package.json`,
   см. выше) — медленнее на ~1 с внутри `npm test`.
4. В `Course.tsx` обложка продублирована в панели-шапке и в вилке покупки:
   заменяется только «клише-градиент», при отсутствии обложки разметка прежняя.

## Найденные расхождения/риски (координатору, на правку не тяну)

1. **Раздача `/uploads/*` не дотягивается до фронтового origin.** Бэкенд хранит
   и раздаёт файлы по `/uploads/*` (handler/middleware.go), а в ответах API
   отдаёт путь `/uploads/covers/<имя>`. В prod-цепочке nginx
   (frontend/nginx.conf) проксирует только `/api/` — `/uploads/*` web-контейнером
   не проксируется, и `<img src="/uploads/…">` на порту web получит 404. Точно
   так же и в dev: `vite.config.ts` проксирует только `/api`. Чтобы обложки
   реально показывались, нужно добавить `location /uploads/ { proxy_pass … }`
   в nginx.conf и (для dev) прокси `/uploads` в vite.config.ts — оба файла вне
   зоны T57. Клиент (`coverUrl`) честно отдаёт путь как есть и умеет
   приклеивать origin при абсолютном `VITE_API_URL`.
2. Контракт говорит `coverPath: string | null`, а текущий backend-код T54 в
   ветке (repo/models.go) сериализует `coverPath,omitempty` в строке (реально
   придёт отсутствие поля либо пустая строка после COALESCE). Типы приняты как
   `string | null` — UI работает с истинностью значения, оба представления
   покрыты.
