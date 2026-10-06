#!/bin/bash
# SPDX-License-Identifier: GPL-2.0-or-later
set -euo pipefail
cd "$(dirname "$0")"
export MUTTI_TEST_UID="$(id -u)" MUTTI_TEST_GID="$(id -g)"
docker compose -f compose.yaml down
