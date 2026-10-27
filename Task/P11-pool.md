# P11 — пул задач финальной полировки

Ветки созданы от `main` (`df9ed31`). Каждая задача — отдельный worktree и PR.
Агенты не трогают `Task/**`, `specs/**`, `frontend/src/i18n/ru.json` и чужие файлы.

| ID | Ветка | Владелец файлов | Задача |
|---|---|---|---|
| P11-1 | `fix/p11-payments` | `backend/internal/handler/{enroll,mock_payment,admin_payments,middleware}.go`, `backend/internal/payments/{provider,mock,yookassa}.go`, `backend/internal/config/config.go`, `backend/internal/repo/{store,postgres_store,store_iface}.go`, миграции, тесты | Платёжная безопасность: cross-provider webhook, запрет клиентского `provider`/mock вне dev, легальные переходы, проверка ошибок записи, blocked `optionalUser` |
| P11-2 | `fix/p11-content` | `backend/internal/handler/courses.go`, `backend/internal/handler/reviews.go`, новые тесты | Гейтинг контента: гость видит `bodyMd/videoUrl/liveUrl` только у `isPreview`; активная запись — контент без ответов/скрытых тестов; owner/admin — всё. Отзывы только опубликованных курсов |
| P11-3 | `fix/p11-admin` | `frontend/src/pages/Admin*.tsx`, `frontend/src/components/RouteGuards.tsx`, `frontend/src/pages/{Auth,Favorites,Profile}.tsx` | Админ-навигация: `courseId` из URL и сохранение фильтра, `coverUrl`, лимит 100, `from` с search/hash, 401 в избранном, aria-ошибки |
| P11-4 | `fix/p11-shell` | `frontend/src/styles.css`, `frontend/src/app/router.tsx`, `frontend/src/pages/{Learn,Course}.tsx` | Контраст WCAG AA, гонка вердикта Learn, мёртвые ссылки/футер, админ-ссылки только админу, lazy Course, осмысленные outcomes |
| P11-5 | `fix/p11-marketing` | `frontend/src/components/marketing.tsx`, `frontend/src/pages/{Catalog,Constructor}.tsx` | Честная лид-форма без ложного успеха, ссылки направлений, статистика каталога, локальная дата промокода, доступные табы |
| P11-6 | `fix/p11-docs` | `README.md`, `docs/API.md`, `.env.example` | Актуализация документации: Yandex OAuth, teacher seed, env, QA-скрипты, ограничения |
| P11-7 | `fix/p11-startup` | `backend/cmd/api/main.go` | Безопасный старт: не поднимать прод на in-memory при недоступной БД; не создавать `teacher@local/teacher1` без явных `TEACHER_*`, не сбрасывать существующий пароль |

## Волна 2 (после мержа P11-1/P11-2)

| ID | Ветка | Владелец файлов | Задача |
|---|---|---|---|
| P11-8 | `fix/p11-quiz-multi` | `backend/internal/repo/models.go`, `backend/internal/service/checker.go`, `backend/internal/handler/courses.go`, `frontend/src/components/LessonView.tsx`, `frontend/src/pages/Constructor.tsx`, тесты | FR-010: множественный выбор в квизе с обратной совместимостью (`correct` + `multi`/`correctIndexes`) |
| P11-9 | `fix/p11-tests-gaps` | новые файлы `backend/tests/contract/*.go`, `backend/internal/handler/*_test.go` | Недостающие проверки: failed-платёж, просроченный invite, code accepted/failed/retry |

## Проверки после интеграции

- `npm ci && npm run build` — PASS.
- `npm test` — все тесты PASS.
- Go `go vet ./... && go test ./... -count=1` в Docker.
- DOM-проверки на live: гейтинг урока, админ-фильтры, контраст, лид-форма.
