#!/bin/bash
# SPDX-License-Identifier: GPL-2.0-or-later
set -euo pipefail
umask 077
cd "$(dirname "$0")"
for PORT in 18594 18595 18597 18599; do
  if command -v lsof >/dev/null && lsof -nP -iTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "Testport $PORT ist belegt. Beende zuerst die Mutti-Mac-App oder den früheren Docker-Test."
    exit 1
  fi
done
mkdir -p Testdaten Testcache Testmedien
export MUTTI_TEST_UID="$(id -u)" MUTTI_TEST_GID="$(id -g)"
docker load -i mutti-image.tar.gz
docker compose -f compose.yaml up -d
echo 'Einrichtung: http://127.0.0.1:18594'
echo 'Verwaltung: http://127.0.0.1:18597/web/#/mutti'
echo 'In der Einrichtung /media als Testbibliothek wählen.'
echo 'Beenden: Stop-Docker-Test.command. Testdaten und Testmedien bleiben erhalten.'
