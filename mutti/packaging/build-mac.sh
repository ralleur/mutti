#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
WEB="${MUTTI_WEB_DIR:-$ROOT/../mutti-web}"
DOTNET="${MUTTI_DOTNET:-dotnet}"
RID="${MUTTI_MAC_RID:-osx-arm64}"
case "$RID" in osx-arm64) ARCH=arm64;; osx-x64) ARCH=x86_64;; *) echo "Unsupported Mac runtime" >&2; exit 1;; esac
export DOTNET_CLI_TELEMETRY_OPTOUT=1 DOTNET_NOLOGO=1
python3 "$ROOT/mutti/packaging/verify-sources.py" "$WEB"
"$DOTNET" publish "$ROOT/Jellyfin.Server/Jellyfin.Server.csproj" -c Release -r "$RID" --self-contained true -o "$ROOT/build/server/$RID"
(cd "$WEB" && npm ci --no-audit --no-fund && npm run build:production)
swift build --package-path "$ROOT/mutti/apps/macos" -c release --arch "$ARCH"
BIN="$(swift build --package-path "$ROOT/mutti/apps/macos" -c release --arch "$ARCH" --show-bin-path)"
python3 "$ROOT/mutti/packaging/fetch-ffmpeg.py" "$RID"
APP="$ROOT/build/macos/$RID/Mutti.app"
# This is a generated product path, never a user data directory.
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$BIN/Mutti" "$APP/Contents/MacOS/Mutti"
cp "$ROOT/mutti/apps/macos/Info.plist" "$APP/Contents/Info.plist"
cp "$ROOT/mutti/design/Mutti.icns" "$APP/Contents/Resources/Mutti.icns"
cp -R "$ROOT/mutti/apps/macos/Localization/"*.lproj "$APP/Contents/Resources/"
cp -R "$ROOT/build/server/$RID" "$APP/Contents/Resources/server"
cp -R "$WEB/dist" "$APP/Contents/Resources/web"
mkdir -p "$APP/Contents/Resources/ffmpeg" "$APP/Contents/Resources/licenses"
cp "$ROOT/build/ffmpeg/$RID/ffmpeg" "$ROOT/build/ffmpeg/$RID/ffprobe" "$APP/Contents/Resources/ffmpeg/"
cp "$ROOT/LICENSE" "$APP/Contents/Resources/licenses/Jellyfin.txt"
cp "$WEB/LICENSE" "$APP/Contents/Resources/licenses/Jellyfin-Web.txt"
cp "$ROOT/mutti/design/assets/Sora-OFL.txt" "$APP/Contents/Resources/licenses/"
cp "$ROOT/mutti/THIRD-PARTY.md" "$APP/Contents/Resources/licenses/"
cp "$ROOT/mutti/components.lock.json" "$APP/Contents/Resources/"
python3 "$ROOT/mutti/packaging/provenance.py" "$WEB" "$APP/Contents/Resources/build-provenance.json"
# Local development signature only. Release signing/notarization is a separate gate.
codesign --force --deep --sign - "$APP"
codesign --verify --deep --strict "$APP"
echo "$APP"
