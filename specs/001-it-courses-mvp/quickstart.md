# Quickstart: проверка MVP

**Цель**: доказать сквозные сценарии из spec.md без полной реализации.

## Предусловия

- Docker + Compose v2
- Порты свободны: 8080 (api), 5433 (db), 3000 (web), 8081 (runner)
- `.env` из `.env.example`: `PAYMENTS_PROVIDER=mock`, `COMMISSION_PCT=15`

## Запуск

```powershell
docker compose up --build
# web: http://localhost:3000
# api: http://localhost:8080/api/v1
# health: http://localhost:8080/healthz -> OK
```

## Сценарий A: покупка учеником

1. Открыть каталог `/` — виден сид-курс "Python с нуля".
2. Открыть `/courses/python-s-nulya` — программа, цена, учитель.
3. "Купить" без входа → редирект на `/login`.
4. Зарегистрироваться `student@test.ru / test1234` → кабинет пуст.
5. Купить (provider=mock) → редирект mock-успех → webhook → статус succeeded.
6. Ожидаемо: курс в "Мои курсы", прогресс 0%, доступна кнопка "Продолжить".

## Сценарий B: учитель + модерация

1. Как админ (`admin@local / admin1234`): "Приглашения" → ввести `t@local` → скопировать ссылку.
2. Открыть ссылку инкогнито → регистрация учителя → кабинет учителя.
3. "Создать курс": 1 модуль, 3 урока (video_text, quiz, code python).
4. "На проверку" → как админ одобрить → курс в каталоге.

## Сценарий C: прохождение и автопроверка

1. Как ученик: пройти video_text → "Отметить пройденным".
2. Квиз: неверный ответ → пояснение; верный → зачет.
3. Код python `print(sum(...))`: неверное → diff ошибок; верное → accepted.
4. Прогресс вырос, "Продолжить" ведет на следующий урок.

## Контрактные проверки

```powershell
# health
curl http://localhost:8080/healthz
# каталог
curl http://localhost:8080/api/v1/courses | head -c 500
# openapi сверка: contracts/openapi.yaml vs /api/v1/openapi.yaml (после implement)
```

## Критерии прохождения

- SC-001..SC-007 из spec.md воспроизводятся вручную.
- Двойной webhook не дублирует доступ (проверка `provider_payment_id` unique).
- Пустой курс не уходит на модерацию.
