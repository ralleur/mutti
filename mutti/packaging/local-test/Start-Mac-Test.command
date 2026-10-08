#!/bin/bash
# SPDX-License-Identifier: GPL-2.0-or-later
# Runs only the binaries beside this script, with separate private test data.
set -euo pipefail
umask 077
PACKAGE="$(cd "$(dirname "$0")" && pwd)"
RESOURCES="$PACKAGE/Mutti.app/Contents/Resources"
for PORT in 31593 31594 31595 31596 31600; do
  if lsof -nP -iTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "Testport $PORT ist belegt. Beende zuerst den früheren isolierten Test."
    exit 1
  fi
done
mkdir -p "$PACKAGE/Testdaten" "$PACKAGE/Testmedien"
echo 'Isolierter Mutti-Test. Die vorhandene Mutti-Installation wird nicht verwendet.'
echo 'Einrichtung: http://127.0.0.1:31594'
echo 'Verwaltung nach der Einrichtung: http://127.0.0.1:31596/web/#/mutti'
echo "Medienordner für die neue Testbibliothek: $PACKAGE/Testmedien"
echo 'Fotos, Dokumente und lokale KI: in der Verwaltung unter „Module“ einrichten.'
echo 'Beenden: Ctrl-C in diesem Fenster. Die Testdaten bleiben für den nächsten Start erhalten.'
"$RESOURCES/connect/mutti-connect" --mode broker --listen 127.0.0.1:31600 --stun-listen 127.0.0.1:0 >"$PACKAGE/Testdaten/broker.log" 2>&1 &
BROKER_PID=$!
trap 'kill "$BROKER_PID" 2>/dev/null || true; wait "$BROKER_PID" 2>/dev/null || true' EXIT
MUTTI_SIGNAL_URL=http://127.0.0.1:31600 MUTTI_STUN_URL= \
  "$RESOURCES/migrate/mutti-migrate" --root "$PACKAGE/Testdaten/server" \
  --server "$RESOURCES/server/jellyfin" --web "$RESOURCES/web" \
  --ffmpeg "$RESOURCES/ffmpeg/ffmpeg" --intro-skipper "$RESOURCES/intro-skipper" \
  --connect "$RESOURCES/connect/mutti-connect" \
  --hub "$RESOURCES/hub/mutti-hub" --hub-listen 127.0.0.1:31593 --ollama "$RESOURCES/ai-engine/ollama" \
  --listen 127.0.0.1:31594 --origin http://127.0.0.1:31594 \
  --backend http://127.0.0.1:31596 --target-origin http://127.0.0.1:31596 \
  --connect-listen 127.0.0.1:31595 --connect-origin http://127.0.0.1:31595
