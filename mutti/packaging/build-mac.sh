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
go run "$ROOT/mutti/packaging/fetch-intro-skipper.go" "$ROOT/mutti/components.lock.json" "$ROOT/build/intro-skipper"
(cd "$ROOT/mutti/connect" && CGO_ENABLED=0 GOOS=darwin GOARCH="$(test "$ARCH" = arm64 && echo arm64 || echo amd64)" go build -trimpath -o "$ROOT/build/connect/$RID/mutti-connect" ./cmd/mutti-connect)
(cd "$ROOT/mutti/migrate" && CGO_ENABLED=0 GOOS=darwin GOARCH="$(test "$ARCH" = arm64 && echo arm64 || echo amd64)" go build -trimpath -o "$ROOT/build/migrate/$RID/mutti-migrate" ./cmd/mutti-migrate)
"$DOTNET" build "$ROOT/mutti/export/Mutti.Export.csproj" -c Release -p:JellyfinDir="$ROOT/build/server/$RID" -o "$ROOT/build/export"
APP="$ROOT/build/macos/$RID/Mutti.app"
# This is a generated product path, never a user data directory.
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$BIN/Mutti" "$APP/Contents/MacOS/Mutti"
cp "$ROOT/mutti/apps/macos/Info.plist" "$APP/Contents/Info.plist"
cp "$ROOT/mutti/design/Mutti.icns" "$APP/Contents/Resources/Mutti.icns"
cp -R "$ROOT/mutti/apps/macos/Localization/"*.lproj "$APP/Contents/Resources/"
mkdir -p "$APP/Contents/Resources/connect"
cp "$ROOT/build/connect/$RID/mutti-connect" "$APP/Contents/Resources/connect/"
mkdir -p "$APP/Contents/Resources/migrate"
cp "$ROOT/build/migrate/$RID/mutti-migrate" "$APP/Contents/Resources/migrate/"
cp -R "$ROOT/build/intro-skipper" "$APP/Contents/Resources/intro-skipper"
mkdir -p "$APP/Contents/Resources/export"
cp "$ROOT/build/export/Mutti.Export.dll" "$ROOT/mutti/export/README.md" "$APP/Contents/Resources/export/"
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
