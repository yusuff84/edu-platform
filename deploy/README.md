# Развёртывание на IP-сервере

Целевой адрес: `http://132.243.195.86`. Домен и TLS не подключаются по явному запросу владельца. Сервисы и база размещаются в отдельном Compose-проекте `skillex`. PostgreSQL, API и runner не публикуют порты в интернет; web доступен nginx хоста через `127.0.0.1:13080`.

## Состав релиза

`compose.server.yml` задаёт ограничения памяти для сервера с 1 ГБ RAM. Образы `skillex-api`, `skillex-web`, `skillex-runner` фиксируются тегом релиза. База PostgreSQL 16 и том uploads сохраняются при пересоздании контейнеров. Runner использует bubblewrap и отдельную внутреннюю сеть.

`scripts/prepare-server-release.py --output-root D:/CodexDeploy/skillex` создаёт снимок БД и uploads с приостановкой API, архив исходников, образы и SHA256 manifest. Пароли генерируются случайно, сохраняются только в приватном каталоге релиза, не попадают в исходники или Git.

Для текущего переноса использован релиз `20261118-5b628dcc`. Из-за заполнения диска C: выполнено восстановление локального Docker; его отдельная резервная копия находится в `D:/CodexDeploy/skillex/docker-data-before-repair.vhdx`. Полный custom-format дамп БД получен до сбоя. Все 62 uploads восстановлены из копии хранилища; размеры и SHA256 всех 60 медиафайлов курса сравнены с исходной выгрузкой. Локальный Docker и оба ранее работавших Compose-стенда восстановлены.

`scripts/package-offline-release.py` упаковывает этот снимок, проверенные uploads, исходники, собранные Linux-бинарники и Docker-образы. `Dockerfile.*-prebuilt` позволяют собрать образы без компиляции Go/TypeScript на малом сервере, если архив образов отсутствует. `scripts/upload-server-release.py` запрашивает SSH-пароль в терминале, проверяет известный ключ сервера, передаёт архив по SFTP и запускает установщик. SSH-пароль не записывается в файлы.

## Размещение на сервере

- `/opt/skillex/releases/20261118-5b628dcc` — исходники и Compose.
- `/opt/skillex/.env` — production-настройки, права 600.
- `/opt/skillex/current` — ссылка на активный релиз.
- `/opt/skillex/transfers/20261118-5b628dcc` — снимки и контрольные суммы, доступ только root.
- `/opt/skillex/backups/hamlion-before-20261118-5b628dcc` — дамп старой БД, роли, uploads, Redis, nginx и конфигурация контейнеров.
- `/root/skillex-deploy-20261118-5b628dcc.log` — журнал установки.

`install-server.sh` проверяет SHA256, сохраняет прежнее приложение, останавливает его без удаления контейнеров/томов, восстанавливает БД только в пустую новую базу и переносит uploads. Тестовые учётки с доменами `local`, `e2e.test`, `example.test` сохраняются в БД, но блокируются. Создаётся отдельный администратор `admin@skillex.space` с новым случайным паролем. Данные для входа лежат в приватном `admin-access.txt` каталога релиза на D: и в каталоге transfers сервера.

В production выбран явный режим `PAYMENTS_PROVIDER=disabled`: платные покупки возвращают 503 без создания платежа и доступа, бесплатный курс открывается без оплаты. Mock в production по-прежнему запрещён. Для включения платных покупок нужны реальные настройки Stripe/YooKassa. SMTP и Яндекс OAuth не настроены: реальные письма и OAuth требуют соответствующих реквизитов. Сессионные cookie для HTTP имеют HttpOnly и SameSite=Lax; при последующем подключении HTTPS сервер выставляет Secure по протоколу запроса.

## Управление

```sh
docker compose -p skillex --env-file /opt/skillex/.env \
  -f /opt/skillex/current/deploy/compose.server.yml ps
docker compose -p skillex --env-file /opt/skillex/.env \
  -f /opt/skillex/current/deploy/compose.server.yml logs --tail 100 api runner
```

Существующие контейнеры и тома hamlion сохранены. Для возврата сначала остановить проект skillex, включить сохранённую конфигурацию nginx для hamlion и запустить `hamlion-postgres`, `hamlion-redis`, `hamlion-web`, `hamlion-worker`. Не удалять тома и не применять `down -v` при обновлении.
