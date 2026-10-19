# T68 — Фронтенд: сертификаты (публичная страница, «Мои курсы», обучение)

**Волна:** P8  **Ветка:** `task/T68-certificates-ui`  **Worktree:** `wt/t68`

## Суть

Нужна публичная страница сертификата по номеру (можно делиться ссылкой),
бейдж-ссылка в «Моих курсах» и поздравление со ссылкой на сертификат после
завершения курса. API — по разделу «Эндпоинты волны P8» в
`Task/00-api-contract.md` (**заморожен**); бэкенд T67 идёт параллельно.

Работать только в своём worktree `wt/t68` (ветка `task/T68-certificates-ui`).

## Что сделать

1. `frontend/src/api/types.ts`: `CertificatePublic`
   (`number, issuedAt, studentName, courseTitle, courseSlug`), `MyCertificate`
   (`number, issuedAt, courseId, courseTitle, courseSlug`);
   `MyCourse.certificateNumber?: string` (аддитивно с бэкенда).
2. `frontend/src/api/client.ts`: `getCertificate(number)` и
   `myCertificates()` (возвращает массив `items`).
3. `frontend/src/app/router.tsx`: публичный маршрут `/certificates/:number`
   (страница доступна без входа, внутри общего Layout — как другие публичные
   страницы); lazy-импорт по стилю соседних маршрутов.
4. Новая страница `frontend/src/pages/Certificate.tsx`:
   - загрузка по номеру; состояния загрузки/ошибки;
   - карточка: «Сертификат о завершении курса», ФИО (или «Студент», если пусто),
     название курса, дата выдачи, номер;
   - кнопка «Скопировать ссылку» (`navigator.clipboard`, фолбэк — `document.execCommand`),
     после копирования — текст `p8.cert.copied`;
   - ссылка «К курсу» (`/courses/<courseSlug>`), ссылка в каталог;
   - `404` — дружелюбное состояние `p8.cert.notFound/notFoundHint`.
5. `frontend/src/pages/MyCourses.tsx`: если у курса есть `certificateNumber` —
   бейдж/кнопка «Сертификат» со ссылкой `/certificates/<number>`.
6. `frontend/src/pages/Learn.tsx`: при `percent === 100` — баннер «Курс завершён
   — сертификат готов!» со ссылкой на сертификат. Номер берите из
   `myCertificates()` (по `courseId`) либо из `MyCourse.certificateNumber`;
   если сертификата ещё нет — баннер без ссылки.
7. Стили — новый `frontend/src/styles/certificate.css` (импорт в страницах, где
   используются новые классы), на переменных/классах дизайн-системы. Чужие
   css-файлы не править.
8. i18n — только готовые ключи `p8.cert.*`; словарь не править, не хватает —
   в отчёт.
9. Тесты: новый `frontend/tests/api-contract/certificates.test.mjs` по образцу
   существующих (мок-сервер 127.0.0.1:8799): `getCertificate` (200/404),
   `myCertificates` (парсинг `items`).

## Разрешённые файлы

- `frontend/src/app/router.tsx`
- `frontend/src/pages/Certificate.tsx` (новый)
- `frontend/src/pages/MyCourses.tsx`, `frontend/src/pages/Learn.tsx`
- `frontend/src/api/client.ts`, `frontend/src/api/types.ts`
- `frontend/src/styles/certificate.css` (новый)
- `frontend/tests/api-contract/certificates.test.mjs` (новый)
- `frontend/tests/T68-certificates-ui.md` (отчёт)

## Запрещено

- `backend/**`, `Task/**`, `specs/**`, `frontend/src/i18n/ru.json`, чужие
  страницы/компоненты/стили, `qa-*.ps1`, `docker-compose*`.
- Менять существующие маршруты и сигнатуры клиентских функций.
- `git push`, `git merge`, смена ветки, работа вне `wt/t68`.

## Проверка

Из папки `frontend/`:

```
npm run build
npm test
```

Сборка и все тесты (старые + новые) зелёные; вывод приложите к отчёту.

## Отчёт

`frontend/tests/T68-certificates-ui.md`: «Что сделано», «Проверка» (команды и
результаты), «Отклонения от постановки», «Открытые вопросы». Закоммитьте отчёт
и код в ветку `task/T68-certificates-ui`.
