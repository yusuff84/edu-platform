# T41 — Slug: транслитерация кириллицы (backend)

**Волна:** P5 · **Ветка:** `task/T41-slug-translit` · **Worktree:** `wt/t41`
**Контракт:** `Task/00-api-contract.md`, раздел «Эндпоинты волны P5» → «Slug: транслитерация кириллицы (T41)». Контракт заморожен — менять его нельзя.

## Зачем

Открытая находка T20: `slugRe` пропускает только латиницу — кириллический `slug`
отклоняется `400`, а авто-slug (`course-<hex8>`) нечитаем в URL каталога.
Закрываем: кириллицу транслитерируем на входе, авто-slug строим из названия курса.

## Что сделать

1. Новый файл `backend/internal/handler/slug.go`:
   - `translitSlug(s string) string` — нижний регистр; таблица кириллицы:
     `а→a б→b в→v г→g д→d е→e ё→e ж→zh з→z и→i й→y к→k л→l м→m н→n о→o п→p
     р→r с→s т→t у→u ф→f х→h ц→c ч→ch ш→sh щ→sch ъ→(пусто) ы→y ь→(пусто)
     э→e ю→yu я→ya`; всё не из `[a-z0-9]` → `-`; повторы `-` схлопнуть;
     `-` по краям обрезать; обрезка до 120 символов;
   - `slugFromTitle(title string) string` — `translitSlug(title)`; если короче
     3 символов — пустая строка (решение о fallback принимает вызывающий).
2. `backend/internal/handler/teacher.go` — `makeSlug`:
   - новая сигнатура `makeSlug(custom, title string) (string, bool)`
     (единственный вызов — `TeacherCreateCourse`; обновить его);
   - `custom` непуст → `translitSlug` → проверка `slugRe` → занятость
     (тексты ошибок в хендлере прежние);
   - `custom` пуст → `slugFromTitle(title)`; занятость: суффиксы `-2`…`-9`
     (укладываться в лимит 120), затем случайный `course-<hex8>` — как раньше;
     пустая/короткая основа — сразу `course-<hex8>`.
3. `backend/internal/handler/teacher_edit.go` (PATCH): slug прогонять через
   `translitSlug` вместо `strings.ToLower(strings.TrimSpace(...))`;
   проверки и сообщения («Некорректный slug …», «Такой slug уже занят») — прежние.
4. Новый `backend/internal/handler/slug_test.go` — таблицей:
   `«Мой Курс» → «moy-kurs»`, `«Ёжик и Яблоки» → «ezhik-i-yabloki»`,
   `«Тест №1!» → «test-1»`, `«---» → «»`, строка длиннее 120 — обрезана;
   плюс проверка уникализации `-2` при занятости (через тестовый store,
   как в соседних тестах `internal/handler`).
5. Прогнать все тесты; если существующие тесты опирались на старый вид авто-slug
   (`course-<hex8>`) — обновить ожидания и перечислить правки в отчёте
   (вероятные места: `backend/internal/handler/*_test.go`, `backend/tests/contract/**`).

## Правила волны

- Работать только в своём worktree; коммит — в свою ветку `task/T41-slug-translit`.
- Править только: `backend/internal/handler/slug.go` (новый), `slug_test.go` (новый),
  `teacher.go`, `teacher_edit.go`, тесты, опиравшиеся на старый вид slug
  (только ожидания), отчёт `backend/tests/T41-slug-translit.md`.
- `frontend/**`, `Task/**`, `specs/**`, `backend/internal/repo/**` — не трогать.
- Код и комментарии — по-русски; новых внешних зависимостей нет.

## Проверка (Docker, из корня worktree; Go на хост не ставить)

```powershell
docker run --rm -v "${PWD}\backend:/src" -v p5-gomod:/go/pkg/mod -w /src -e GOFLAGS=-mod=mod golang:1.24-alpine sh -c "go build ./... && go vet ./... && go test ./... -count=1"
```

`exit 0` обязателен. Если стенд пересобран — ручная проверка в отчёт честно:
`POST /teacher/courses` с `slug: "Мой курс"` → `201`, `slug: "moy-kurs"`.

## Отчёт

`backend/tests/T41-slug-translit.md`: что изменено, таблица примеров, какие тесты
обновлены, честный список непроверенного.
