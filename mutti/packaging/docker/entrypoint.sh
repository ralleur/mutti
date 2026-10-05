#!/bin/sh
set -eu
umask 077
exec /mutti/migrate --container --root /config --server /jellyfin/jellyfin \
  --web /jellyfin/jellyfin-web --ffmpeg /usr/lib/jellyfin-ffmpeg/ffmpeg --connect /mutti/connect \
  --listen 0.0.0.0:18594 --origin http://127.0.0.1:18594 \
  --backend http://127.0.0.1:8096 --target-origin http://127.0.0.1:18597 --bind 0.0.0.0 \
  --connect-listen 0.0.0.0:18595 --connect-origin http://127.0.0.1:18595 "$@"
