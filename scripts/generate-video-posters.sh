#!/usr/bin/env bash
# Генерирует превью-кадры для видео уроков: рядом с каждым <имя>.mp4 в томе
# uploads кладёт <имя>.poster.jpg (кадр ~15% длительности, не раньше 3 с —
# первые секунды часто чёрные). Фронтенд берёт превью по этому соглашению
# (frontend/src/app/media.ts), а если файла нет — показывает прежнюю заглушку.
#
# Идемпотентно: существующие превью не перезаписываются (FORCE=1 — перегенерировать).
# Запуск на сервере (ffmpeg на хосте не нужен — работает во временном контейнере):
#   sudo scripts/generate-video-posters.sh            # том skillex_uploads
#   sudo VOLUME=other_uploads scripts/generate-video-posters.sh
set -euo pipefail

VOLUME="${VOLUME:-skillex_uploads}"
FORCE="${FORCE:-0}"

docker volume inspect "$VOLUME" >/dev/null

docker run --rm -e FORCE="$FORCE" -v "$VOLUME":/uploads alpine:3.20 sh -euc '
  apk add --no-cache ffmpeg >/dev/null
  made=0; skipped=0; failed=0
  for v in $(find /uploads -type f \( -iname "*.mp4" -o -iname "*.webm" -o -iname "*.mov" \)); do
    poster="${v%.*}.poster.jpg"
    if [ -s "$poster" ] && [ "$FORCE" != "1" ]; then skipped=$((skipped+1)); continue; fi
    dur=$(ffprobe -v error -show_entries format=duration -of default=nw=1:nk=1 "$v" 2>/dev/null | cut -d. -f1)
    dur=${dur:-0}
    at=$((dur * 15 / 100)); [ "$at" -lt 3 ] && at=3; [ "$dur" -gt 0 ] && [ "$at" -ge "$dur" ] && at=0
    if ffmpeg -nostdin -v error -y -ss "$at" -i "$v" -frames:v 1 -vf "scale=640:-2" -q:v 4 "$poster.tmp.jpg" \
       && [ -s "$poster.tmp.jpg" ]; then
      mv "$poster.tmp.jpg" "$poster"
      # тот же владелец, что у видео: API в контейнере работает не от root
      chown "$(stat -c %u:%g "$v")" "$poster"
      made=$((made+1))
    else
      rm -f "$poster.tmp.jpg"; failed=$((failed+1)); echo "не удалось: $v" >&2
    fi
  done
  echo "превью: создано $made, уже были $skipped, ошибок $failed"
'
