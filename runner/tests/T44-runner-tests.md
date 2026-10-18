# T44 — отчёт: прогон тестов runner с интерпретаторами

**Волна:** P5 · **Ветка:** `task/T44-runner-tests` · **Worktree:** `wt/t44`
**Код:** `runner/**` (единственное изменение — комментарий в `main_test.go`) ·
**README:** новый раздел «Проверка runner (python3/nodejs)» в «Тесты и QA»

## Команда

Из постановки (корень worktree):

```powershell
docker run --rm -v "${PWD}\runner:/src" -w /src golang:1.24-bookworm sh -c "apt-get update -qq && apt-get install -y -qq --no-install-recommends python3 nodejs >/dev/null && go vet ./... && go test ./... -count=1 -v"
```

**Честное отличие при прогоне:** в текущей сети `deb.debian.org` недоступен
(DNS резолвится, TCP-соединение виснет — `apt-get update` не завершался даже
через 10 минут; с хоста `deb.debian.org` и `ftp.debian.org` тоже недоступны,
при этом `proxy.golang.org`, `mirror.yandex.ru`, `github.com` открываются).
Поэтому прогон выполнен тем же составом команд, но с переключением источников
apt на рабочее зеркало:

```sh
sed -i 's|deb.debian.org|mirror.yandex.ru|g' /etc/apt/sources.list.d/debian.sources
apt-get update -qq && apt-get install -y -qq --no-install-recommends python3 nodejs >/dev/null
cd /src && go vet ./... && go test ./... -count=1 -v
```

Скрипты прогона (вне репозитория, во временном каталоге):
`%TEMP%\opencode\t44-baseline.sh`, `%TEMP%\opencode\t44-run.sh`;
логи — `t44-baseline.log`, `t44-final.log`.

Окружение прогона: образ `golang:1.24-bookworm`, установлены
`python3` 3.11.2 и `node` v18.20.4 из Debian bookworm.

## Было: чистый образ golang:1.24-bookworm (без установки пакетов)

Неожиданное наблюдение: **`python3` в чистом `golang:1.24-bookworm` уже есть**
(`/usr/bin/python3`, тянется как зависимость образа), нет только `nodejs`.
Поэтому SKIP был ровно один — не «python/js-тесты», а только `TestJSSuccess`.

```
== интерпретаторы в чистом образе ==
/usr/bin/python3
node: не найден
== go vet ==
== go test -count=1 -v ==
=== RUN   TestHealth
--- PASS: TestHealth (0.00s)
=== RUN   TestRunRejectsGET
--- PASS: TestRunRejectsGET (0.00s)
=== RUN   TestUnknownLanguage
--- PASS: TestUnknownLanguage (0.00s)
=== RUN   TestEmptyCode
--- PASS: TestEmptyCode (0.00s)
=== RUN   TestPythonSuccess
--- PASS: TestPythonSuccess (0.23s)
=== RUN   TestPythonFailureShowsDiff
--- PASS: TestPythonFailureShowsDiff (0.09s)
=== RUN   TestJSSuccess
    main_test.go:100: интерпретатор node не установлен — тест пропущен
--- SKIP: TestJSSuccess (0.00s)
=== RUN   TestRequestBudgetKillsInfiniteLoop
--- PASS: TestRequestBudgetKillsInfiniteLoop (1.02s)
=== RUN   TestOutputLimit
--- PASS: TestOutputLimit (0.10s)
=== RUN   TestConcurrencyRejected
--- PASS: TestConcurrencyRejected (0.11s)
=== RUN   TestHTMLCSSPass
--- PASS: TestHTMLCSSPass (0.00s)
=== RUN   TestHTMLCSSStyleInsensitivity
--- PASS: TestHTMLCSSStyleInsensitivity (0.00s)
=== RUN   TestHTMLCSSElementNotMentionedInText
--- PASS: TestHTMLCSSElementNotMentionedInText (0.00s)
=== RUN   TestHTMLCSSErrorExplainsLimitation
--- PASS: TestHTMLCSSErrorExplainsLimitation (0.00s)
=== RUN   TestLimitedWriter
--- PASS: TestLimitedWriter (0.00s)
PASS
ok  	runner	1.592s
```

Итог «было»: **14 PASS, 1 SKIP, 0 FAIL**.

## Стало: с python3 и nodejs

Прогон на финальном состоянии ветки: `go vet` — без замечаний,
`go test -count=1 -v` — **15 PASS, 0 SKIP, 0 FAIL**, код возврата docker — 0.

```
== интерпретаторы ==
/usr/bin/python3
Python 3.11.2
/usr/bin/node
v18.20.4
== go vet ==
== go test -count=1 -v ==
=== RUN   TestHealth
--- PASS: TestHealth (0.00s)
=== RUN   TestRunRejectsGET
--- PASS: TestRunRejectsGET (0.00s)
=== RUN   TestUnknownLanguage
--- PASS: TestUnknownLanguage (0.00s)
=== RUN   TestEmptyCode
--- PASS: TestEmptyCode (0.00s)
=== RUN   TestPythonSuccess
--- PASS: TestPythonSuccess (0.04s)
=== RUN   TestPythonFailureShowsDiff
--- PASS: TestPythonFailureShowsDiff (0.05s)
=== RUN   TestJSSuccess
--- PASS: TestJSSuccess (0.43s)
=== RUN   TestRequestBudgetKillsInfiniteLoop
--- PASS: TestRequestBudgetKillsInfiniteLoop (1.01s)
=== RUN   TestOutputLimit
--- PASS: TestOutputLimit (0.10s)
=== RUN   TestConcurrencyRejected
--- PASS: TestConcurrencyRejected (0.06s)
=== RUN   TestHTMLCSSPass
--- PASS: TestHTMLCSSPass (0.00s)
=== RUN   TestHTMLCSSStyleInsensitivity
--- PASS: TestHTMLCSSStyleInsensitivity (0.00s)
=== RUN   TestHTMLCSSElementNotMentionedInText
--- PASS: TestHTMLCSSElementNotMentionedInText (0.00s)
=== RUN   TestHTMLCSSErrorExplainsLimitation
--- PASS: TestHTMLCSSErrorExplainsLimitation (0.00s)
=== RUN   TestLimitedWriter
--- PASS: TestLimitedWriter (0.00s)
PASS
ok  	runner	1.713s
```

### Было → стало по каждому тесту

| # | Тест | Чистый образ | С интерпретаторами |
|---|---|---|---|
| 1 | `TestHealth` | PASS | PASS |
| 2 | `TestRunRejectsGET` | PASS | PASS |
| 3 | `TestUnknownLanguage` | PASS | PASS |
| 4 | `TestEmptyCode` | PASS | PASS |
| 5 | `TestPythonSuccess` | PASS | PASS |
| 6 | `TestPythonFailureShowsDiff` | PASS | PASS |
| 7 | `TestJSSuccess` | **SKIP** (нет `node`) | **PASS** |
| 8 | `TestRequestBudgetKillsInfiniteLoop` | PASS | PASS |
| 9 | `TestOutputLimit` | PASS | PASS |
| 10 | `TestConcurrencyRejected` | PASS | PASS |
| 11 | `TestHTMLCSSPass` | PASS | PASS |
| 12 | `TestHTMLCSSStyleInsensitivity` | PASS | PASS |
| 13 | `TestHTMLCSSElementNotMentionedInText` | PASS | PASS |
| 14 | `TestHTMLCSSErrorExplainsLimitation` | PASS | PASS |
| 15 | `TestLimitedWriter` | PASS | PASS |

Итог: было **14 PASS / 1 SKIP**, стало **15 PASS / 0 SKIP**. Оставшихся
честно невозможных SKIP нет: с установленными `python3` и `nodejs` ни один
тест не пропускается.

## Что изменено

| Файл | Что |
|---|---|
| `runner/main_test.go` | уточнён комментарий: в чистом `golang:1.24-bookworm` `python3` есть, `nodejs` — нет (раньше комментарий говорил, что пропускаются и python, и js) |
| `README.md` | новый короткий раздел «Проверка runner (python3/nodejs)» рядом с «Тесты и QA» |
| `runner/tests/T44-runner-tests.md` | этот отчёт (новый каталог) |

Починок в `runner/main.go` не потребовалось: FAIL в обоих прогонах нет.
Новые тесты не добавлялись: все сценарии из п.3 постановки уже покрыты
(см. таблицу соответствия ниже).

## Соответствие п.3 постановки (покрытие тестами)

| Требование | Тест |
|---|---|
| реальный запуск python (успех) | `TestPythonSuccess` |
| реальный запуск python (провал теста, diff в error_ru) | `TestPythonFailureShowsDiff` |
| реальный запуск js (успех, input через stdin + прогон теста) | `TestJSSuccess` |
| общий бюджет времени на весь запрос | `TestRequestBudgetKillsInfiniteLoop` |
| лимит вывода процесса + пометка об обрезке | `TestOutputLimit`, `TestLimitedWriter` |
| семафор конкурентности (503 без очереди, работа после освобождения) | `TestConcurrencyRejected` |
| статическая проверка htmlcss | `TestHTMLCSSPass`, `TestHTMLCSSStyleInsensitivity`, `TestHTMLCSSElementNotMentionedInText`, `TestHTMLCSSErrorExplainsLimitation` |

## Не проверено

- **Команда из постановки выполнена не дословно**: в текущей сети
  `deb.debian.org` недоступен (см. «Команда»), поэтому apt-источники
  переключены на `mirror.yandex.ru`. Сам `go vet`/`go test` и состав
  устанавливаемых пакетов совпадают. Дословный вариант в README приведён;
  при закрытом Debian-зеркале сработает примечание рядом с командой.
- **Сборка `runner/Dockerfile` не пересобиралась** (правила волны: стенд не
  трогать; к тому же build-этап требует `deb.debian.org`). То, что те же
  тесты гоняются в build-этапе при сборке образа, этим прогоном не
  подтверждается.
- **Другие версии интерпретаторов не проверялись**: результат завязан на
  python 3.11.2 и node v18.20.4 из Debian bookworm; на другом окружении
  (alpine, другие версии) поведение не проверялось. `-race` не гонялся.
- **Реальная параллельность N запросов** не проверялась: `TestConcurrencyRejected`
  занимает слоты семафора вручную. Что 4 «живых» запроса действительно идут
  параллельно, а 5-й получает 503 — не измерено.
- **Лимит вывода проверяется с уменьшенным лимитом** (`rn.maxOutput=1024`):
  реальные 256 КБ stdout+stderr и поведение памяти на них не прогонялись.
- **Потолок на один тест** (`execTimeout=5s`) отдельно от общего бюджета
  не проверяется: в тесте бюджета `execTimeout` выставлен в 30 с, то есть
  срабатывает именно бюджет запроса.
- **htmlcss — по-прежнему только статический анализ** (без браузера, так и
  задумано): вложенность DOM, применение CSS и рендеринг не проверяются.
- Тайминги тестов имеют запас (бюджетный тест допускает до 10 с, «мгновенный»
  503 — до 2 с), но на сильно перегруженном CI возможна нестабильность.
