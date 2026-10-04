#!/bin/sh
set -eu
umask 077
mkdir -p "$JELLYFIN_CONFIG_DIR" "$JELLYFIN_DATA_DIR" "$JELLYFIN_LOG_DIR" "$JELLYFIN_CACHE_DIR"
if [ ! -f "$JELLYFIN_CONFIG_DIR/system.xml" ]; then
  printf '%s\n' '<ServerConfiguration><ServerName>Mutti</ServerName></ServerConfiguration>' > "$JELLYFIN_CONFIG_DIR/system.xml"
fi
if [ ! -f "$JELLYFIN_CONFIG_DIR/network.xml" ]; then
  printf '%s\n' '<NetworkConfiguration><EnableRemoteAccess>false</EnableRemoteAccess><AutoDiscovery>false</AutoDiscovery><EnableIPv6>false</EnableIPv6></NetworkConfiguration>' > "$JELLYFIN_CONFIG_DIR/network.xml"
fi
exec /mutti/connect --listen 0.0.0.0:18595 --admin-origin http://127.0.0.1:18595 --state /config/connect --target http://127.0.0.1:8096 --target-host 127.0.0.1:18597 -- /jellyfin/jellyfin --webdir /jellyfin/jellyfin-web --ffmpeg /usr/lib/jellyfin-ffmpeg/ffmpeg --package-name mutti-preview "$@"
