# T67 — Бэкенд: сертификаты о завершении курса

**Волна:** P8  **Ветка:** `task/T67-certificates`  **Worktree:** `wt/t67`

## Суть

Пройденный курс никак не подтверждается: нет сертификата и публичной проверки.
Волна P8 вводит сертификаты: выдаются автоматически при 100% прогресса, имеют
номер, доступны в «Моих курсах» и проверяются публичной страницей по номеру.
Раздел «Эндпоинты волны P8 (T63–T70)» в `Task/00-api-contract.md` — источник
правды, **заморожен**.

Работать только в своём worktree `wt/t67` (ветка `task/T67-certificates`).

## Что сделать

1. Миграция `backend/internal/migrate/migrations/010_certificates.sql` — таблица
   `certificates` по контракту (`UNIQUE (user_id, course_id)`, `number UNIQUE`,
   FK `ON DELETE CASCADE`); идемпотентно, в стиле `007_cover.sql`.
2. `repo/models.go`: `Certificate` (id, userId, courseId, number, issuedAt) и
   вид для списка (`number, issuedAt, courseId, courseTitle, courseSlug`) —
   JSON-теги по контракту.
3. StoreAPI (блок методов в конце интерфейса): идемпотентная выдача
   (`UNIQUE (user_id, course_id)`; при конфликте — вернуть существующий),
   получение по номеру (с названием/слагом курса и displayName студента),
   список сертификатов пользователя. Postgres и in-memory; parity.
4. Выдача: в точке, где после завершения урока прогресс ученика по курсу
   впервые становится 100% (все уроки завершены, у курса ≥ 1 урока) — создайте
   сертификат для текущего ученика. Повторное достижение 100% второй сертификат
   не создаёт. Номер: `EDU-<ГГГГ>-<8 hex в верхнем регистре>` (crypto/rand),
   при коллизии — повторная генерация.
5. `GET /me/certificates` (только вошедший) — `{"items":[…]}` по контракту,
   новые первыми. Регистрация рядом с другими `/me`-маршрутами.
6. `GET /me/courses`: в каждый элемент добавьте `"certificateNumber"` (пустая
   строка, если сертификата нет).
7. `GET /certificates/{number}` — **публичный, без авторизации**: поля по
   контракту (`studentName` — displayName, без e-mail), `404 «Сертификат не
   найден»`. Зарегистрируйте маршрут в публичной секции.
8. Тесты `backend/tests/contract/`: выдача после последнего урока (и только
   после него), идемпотентность (повторное завершение), список `/me/certificates`
   (порядок), `certificateNumber` в `/me/courses` (есть/нет), публичный GET без
   авторизации, 404. Parity-тесты новых методов стора в `backend/tests/postgres/`.
9. Короткая приписка в `docs/API.md`.

Существующие тесты должны остаться зелёными. Правки существующих тестов
допустимы только при изменении контракта — обоснуйте в отчёте.

## Разрешённые файлы

- `backend/internal/migrate/migrations/010_certificates.sql` (новый)
- `backend/internal/repo/**`
- `backend/internal/handler/**`
- `backend/tests/**`
- `docs/API.md`
- `backend/tests/T67-certificates.md` (отчёт)

## Запрещено

- `Task/**`, `specs/**`, `frontend/**`, `ru.json`, миграции 001–009,
  `docker-compose*`, `qa-*.ps1`, `.github/**`, `deploy/**`.
- Менять формы существующих ответов (только аддитивные поля).
- `git push`, `git merge`, смена ветки, работа вне `wt/t67`.

## Проверка (только через Docker)

Из корня репозитория:

```
docker run --rm -e HOME=/root -v "${PWD}\backend:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.24-alpine sh -c "go vet ./... && go test ./... -count=1"
```

Postgres-parity: `postgres:16-alpine` на порту 55432 (БД `edu_test`, `edu/edu`),
прогон с `TEST_DATABASE_URL` (хост из контейнера тестов — `host.docker.internal`).
Вывод приложите к отчёту.

## Отчёт

`backend/tests/T67-certificates.md`: «Что сделано», «Проверка» (команды и
результаты), «Отклонения от постановки», «Открытые вопросы». Закоммитьте отчёт
и код в ветку `task/T67-certificates`.
