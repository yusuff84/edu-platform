# Task — план доработок edu-platform

Папка `Task/` — **единый источник правды** по доработкам репозитория.
Каждая задача = один Markdown-файл = один агент = один git-worktree = один ключ сессии.

## Как это работает

1. Читаешь `Task/00-api-contract.md` — это **зафиксированный контракт API**.
   Бэкенд почти не меняется, поэтому фронтенд пишется строго под него.
2. Берёшь файл задачи (например `T03-learn-experience.md`).
3. Агент работает **только в своём worktree** и **только со своими файлами**
   (см. раздел «Владение файлами» в каждой задаче). Это гарантирует
   слияние без конфликтов.
4. Проверяешь критерии приёмки из задачи.
5. Коммит в ветку задачи → merge в `integration/p0-p1-p2`.

## Правила для агентов (обязательно)

- **Не трогать чужие файлы.** Список «Владение файлами» — жёсткая граница.
  Нужно что-то ещё — описать в отчёте, а не править.
- **CSS изолировать по файлам**: `frontend/src/styles/<задача>.css`,
  импортировать из своего компонента. Общий `styles.css` не трогать.
- **Код и комментарии — по-русски** (конституция проекта, `.specify/memory/constitution.md`).
- **Не менять `specs/`** — это артефакты SDD-процесса.
- Перед коммитом прогнать проверки из раздела «Проверка» задачи.
- Коммит — один, с префиксом ключа задачи: `T03: <что сделано>`.

## Волны

| Волна | Задачи | Зависимости | Почему вместе |
|-------|--------|-------------|----------------|
| 0 | — | — | Контракт зафиксирован в `00-api-contract.md` |
| 1 | T01, T02, T07, T09, T10, T11 | — | Независимы, файлы не пересекаются |
| 2 | T03, T04, T05, T06, T08 | T01, T02 | Фронтенд-страницы пишутся поверх нового клиента и словаря |
| 3 | T12, T13, T14 | T03…T11 | Тесты, сквозная проводка, документация |

## Задачи

| Ключ | Название | Волна | Слой | Статус |
|------|----------|-------|------|--------|
| [T01](T01-api-client-contract.md) | Контракт API-клиента фронтенда | 1 | frontend | ☑ |
| [T02](T02-i18n-contract.md) | Словарь и типизированные i18n-ключи | 1 | frontend | ☑ |
| [T03](T03-learn-experience.md) | Экран обучения и просмотр урока | 2 | frontend | ☑ |
| [T04](T04-catalog-reviews.md) | Каталог, страница курса, отзывы | 2 | frontend | ☑ |
| [T05](T05-routing-guards.md) | Роутинг, гварды ролей, 404, error boundary | 2 | frontend | ☑ |
| [T06](T06-constructor-editor.md) | Конструктор курса: редактирование | 2 | frontend | ☑ |
| [T07](T07-admin-us6-backend.md) | US6: журнал платежей и комиссия (Go) | 1 | backend | ☑ |
| [T08](T08-admin-us6-frontend.md) | US6: админ-панель (React) | 2 | frontend | ☑ |
| [T09](T09-payments-providers.md) | Stripe, YooKassa, подпись вебхуков | 1 | backend | ☑ |
| [T10](T10-auth-security.md) | Yandex OAuth, cookie, rate-limit, CSRF | 1 | backend | ☑ |
| [T11](T11-runner-sandbox.md) | Песочница и лимиты runner | 1 | runner | ☑ |
| [T12](T12-contract-tests.md) | Недостающие контрактные тесты + qa-скрипт | 3 | backend | ☑ |
| [T13](T13-integration-wiring.md) | Сквозная проводка маршрутов и compose | 3 | full | ☑ |
| [T14](T14-docs-cleanup.md) | Документация, env, чистка долгов | 3 | docs | ☑ |
| [T15](T15-auth-flow-redirect.md) | Переход после входа: `Auth.tsx` и `state.from` | 2 | frontend | ☑ |

### Волна A — исправления и оптимизация (параллельно)

| Задача | Что | Слой | Статус |
|---|---|---|---|
| [T16](T16-backend-bugs.md) | 7 бэкенд-багов: `startsAt`, прогресс, email в журнале, возвраты, **демо-учётки** | backend | ☑ |
| [T17](T17-frontend-bugs.md) | `inviteToken` из ссылки, редирект с `/login`, `formatPrice(0)` | frontend | ☑ |
| [T18](T18-frontend-perf.md) | Code splitting, дедуп CSS, мемоизации — **с цифрами до/после** | frontend | ☑ |

### Волна B — после T16

| Задача | Что | Слой | Статус |
|---|---|---|---|
| [T19](T19-constructor-crud.md) | CRUD конструктора: правка, удаление, порядок | full | ☑ |

### Волна C — финал

| Задача | Что | Слой | Статус |
|---|---|---|---|
| [T20](T20-tests-coverage.md) | Тесты, сверка контракта, сквозной прогон 49/49 | full | ☑ |

## Карта владения файлами (не пересекается между задачами)

```
frontend/src/api/**                         → T01
frontend/src/i18n/ru.json                   → T02
frontend/src/pages/Learn.tsx                → T03
frontend/src/components/LessonView.tsx      → T03
frontend/src/styles/learn.css               → T03
frontend/src/pages/{Catalog,Course}.tsx     → T04
frontend/src/components/{CourseCard,Reviews}.tsx → T04
frontend/src/styles/catalog.css             → T04
frontend/src/app/router.tsx                 → T05
frontend/src/pages/{Cabinets,MyCourses,NotFound}.tsx → T05
frontend/src/components/ErrorBoundary.tsx   → T05
frontend/src/styles/shell.css               → T05
frontend/src/pages/Constructor.tsx          → T06
frontend/src/styles/constructor.css         → T06
frontend/src/pages/Admin.tsx                → T08
frontend/src/styles/admin.css               → T08
frontend/src/pages/Auth.tsx                 → T15, T17
backend/internal/repo/**                    → T07, T16, T19

### Волна A/B/C

backend/internal/handler/{teacher,progress,admin_payments}.go → T16
backend/cmd/api/main.go                     → T16
backend/internal/repo/{store_iface,store,postgres_store,models}.go → T16 → T19
backend/internal/handler/{teacher_edit,teacher_crud}.go → T19
frontend/src/api/{client,types}.ts          → T17
frontend/src/app/router.tsx                 → T18
frontend/src/styles.css                     → T18
frontend/src/components/marketing.tsx       → T18
frontend/src/pages/Constructor.tsx          → T19
backend/tests/contract/**, qa-abc.ps1       → T20
docker-compose.yml                          → T13 (закрыто)
specs/**                                     → запрещено всем
backend/internal/handler/admin_payments.go  → T07  (новый)
backend/internal/service/billing.go         → T07
backend/internal/payments/**                → T09
backend/internal/handler/enroll.go          → T09
backend/internal/auth/**                    → T10
backend/internal/handler/auth.go            → T10
backend/internal/handler/middleware.go      → T10
backend/internal/config/config.go           → T10
backend/internal/handler/main.go*           → T10
runner/**                                   → T11
backend/tests/contract/**                   → T12
qa-abc.ps1                                 → T12
backend/internal/handler/admin.go           → T13
backend/internal/handler/teacher.go         → T13
docker-compose.yml                          → T13
README.md, .env.example, .env               → T14
*                                         → никто, см. волну 3
```

`frontend/src/main.tsx`, `frontend/src/styles.css`, `frontend/src/i18n/t.ts`
не меняет никто, кроме явного указания в задаче.

## Что было до этого

Спецификация `specs/001-it-courses-mvp/tasks.md` помечает все 52 задачи как `[x]`,
но фактически есть критические расхождения — 5 страниц фронтенда падают с
`TypeError`, 2 эндпоинта всегда отвечают `400`, вердикт по уроку всегда «не зачтено»,
US6 (журнал платежей и комиссия) не реализован. Подробности — в
`specs/001-it-courses-mvp/converge-report.md` и в `Task/00-api-contract.md`.
