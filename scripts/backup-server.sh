#!/usr/bin/env bash
# Ежедневная резервная копия production (Linux, Docker): дамп Postgres + архив тома uploads
# + SHA256-манифест. Старые копии старше KEEP_DAYS удаляются.
#
#   sudo scripts/backup-server.sh                 # в /opt/skillex/backups/daily/<дата-время>/
#   sudo KEEP_DAYS=30 DEST=/mnt/backup scripts/backup-server.sh
#
# Расписание (cron, раз в сутки в 03:30): scripts/backup-server.sh --install-cron
# Восстановление — docs/OPERATIONS.md, раздел «Резервная копия и восстановление».
# Копии на том же сервере защищают от ошибок, но не от потери сервера: регулярно
# выгружайте их во внешнее хранилище.
set -euo pipefail

PROJECT="${PROJECT:-skillex}"
DB_CONTAINER="${DB_CONTAINER:-${PROJECT}-db-1}"
UPLOADS_VOLUME="${UPLOADS_VOLUME:-${PROJECT}_uploads}"
DEST="${DEST:-/opt/skillex/backups/daily}"
KEEP_DAYS="${KEEP_DAYS:-14}"

if [ "${1:-}" = "--install-cron" ]; then
  script="$(readlink -f "$0")"
  echo "30 3 * * * root $script >> /var/log/skillex-backup.log 2>&1" > /etc/cron.d/skillex-backup
  chmod 644 /etc/cron.d/skillex-backup
  echo "cron установлен: /etc/cron.d/skillex-backup (ежедневно 03:30, лог /var/log/skillex-backup.log)"
  exit 0
fi

stamp="$(date -u +%Y%m%d-%H%M%S)"
dir="$DEST/$stamp"
mkdir -p "$dir"
chmod 700 "$DEST" "$dir"

# 1. База: custom-format дамп изнутри контейнера (пароль и имя БД — из его окружения).
docker exec "$DB_CONTAINER" sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$dir/database.dump"
[ -s "$dir/database.dump" ] || { echo "пустой дамп базы" >&2; exit 1; }

# 2. Загруженные файлы (обложки, картинки и видео уроков).
docker run --rm -v "$UPLOADS_VOLUME":/uploads:ro -v "$dir":/out alpine:3.20 \
  tar -czf /out/uploads.tar.gz -C /uploads .

# 3. Манифест с контрольными суммами.
(cd "$dir" && sha256sum database.dump uploads.tar.gz > SHA256SUMS)
chmod 600 "$dir"/*

# 4. Ротация.
find "$DEST" -mindepth 1 -maxdepth 1 -type d -mtime +"$KEEP_DAYS" -exec rm -rf {} +

echo "$(date -u +%FT%TZ) резервная копия: $dir ($(du -sh "$dir" | cut -f1))"
