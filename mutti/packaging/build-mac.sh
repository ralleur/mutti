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
# Reproducible hub: qualification binds the executable digest, so it must not
# change with unrelated commits (no VCS stamp; provenance is recorded below).
(cd "$ROOT/mutti/hub" && CGO_ENABLED=0 GOOS=darwin GOARCH="$(test "$ARCH" = arm64 && echo arm64 || echo amd64)" go build -trimpath -buildvcs=false -o "$ROOT/build/hub/$RID/mutti-hub" ./cmd/mutti-hub)
python3 "$ROOT/mutti/packaging/fetch-ollama.py" "$RID"
(cd "$ROOT/mutti/migrate" && CGO_ENABLED=0 GOOS=darwin GOARCH="$(test "$ARCH" = arm64 && echo arm64 || echo amd64)" go build -trimpath -o "$ROOT/build/migrate/$RID/mutti-migrate" ./cmd/mutti-migrate)
"$DOTNET" build "$ROOT/mutti/export/Mutti.Export.csproj" -c Release -p:JellyfinDir="$ROOT/build/server/$RID" -o "$ROOT/build/export"
APP="$ROOT/build/macos/$RID/Mutti.app"
# This is a generated product path, never a user data directory.
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$BIN/Mutti" "$APP/Contents/MacOS/Mutti"
cp "$ROOT/mutti/apps/macos/Info.plist" "$APP/Contents/Info.plist"
cp "$ROOT/mutti/design/Mutti.icns" "$APP/Contents/Resources/Mutti.icns"
cp "$ROOT/mutti/design/assets/wordmark-light.png" "$ROOT/mutti/design/assets/Sora-Bold.ttf" "$APP/Contents/Resources/"
cp -R "$ROOT/mutti/apps/macos/Localization/"*.lproj "$APP/Contents/Resources/"
mkdir -p "$APP/Contents/Resources/connect"
cp "$ROOT/build/connect/$RID/mutti-connect" "$APP/Contents/Resources/connect/"
mkdir -p "$APP/Contents/Resources/hub"
cp "$ROOT/build/hub/$RID/mutti-hub" "$APP/Contents/Resources/hub/"
# Reviewed, signed qualification evidence for exactly this hub, engine and
# Mac class. Without it the app keeps every AI task locked.
if [ -f "$ROOT/mutti/packaging/qualification/records.json" ]; then
  mkdir -p "$APP/Contents/Resources/hub/qualification"
  cp "$ROOT/mutti/packaging/qualification/records.json" "$APP/Contents/Resources/hub/qualification/"
fi
cp -R "$ROOT/build/ollama/$RID" "$APP/Contents/Resources/ai-engine"
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
cp "$ROOT/mutti/packaging/licenses/Ollama-MIT.txt" "$ROOT/mutti/packaging/licenses/llama.cpp-MIT.txt" "$APP/Contents/Resources/licenses/"
if [ "$ARCH" = arm64 ]; then cp "$ROOT/mutti/packaging/licenses/MLX-MIT.txt" "$ROOT/mutti/packaging/licenses/mlx-c-MIT.txt" "$APP/Contents/Resources/licenses/"; fi
cp "$ROOT/mutti/THIRD-PARTY.md" "$APP/Contents/Resources/licenses/"
# The portable FFmpeg archive carries no licence text; the GPL build needs it next to the binaries.
cp "$ROOT/build/ffmpeg/$RID/licenses/COPYING.GPLv3" "$APP/Contents/Resources/licenses/FFmpeg-COPYING.GPLv3.txt"
cp "$ROOT/build/ffmpeg/$RID/licenses/LICENSE.md" "$APP/Contents/Resources/licenses/FFmpeg-LICENSE.md"
cp "$ROOT/build/intro-skipper/LICENSE" "$APP/Contents/Resources/licenses/IntroSkipper-LICENSE.txt"
cp "$ROOT/mutti/connect/LICENSE.md" "$APP/Contents/Resources/licenses/MuttiConnect-MPL-2.0.md"
cp "$ROOT/mutti/licenses/GPL-3.0.txt" "$APP/Contents/Resources/licenses/Mutti-Package-GPL-3.0.txt"
# A self-contained publish does not copy the runtime pack's notices; collect them explicitly.
python3 "$ROOT/mutti/packaging/collect-dotnet-notices.py" "$RID" "$ROOT/build/server/$RID" "$APP/Contents/Resources/licenses"
cp "$ROOT/mutti/components.lock.json" "$APP/Contents/Resources/"
python3 "$ROOT/mutti/packaging/provenance.py" "$WEB" "$APP/Contents/Resources/build-provenance.json"
# License texts of bundled .NET, Go and npm packages, from the exact inputs.
python3 "$ROOT/mutti/packaging/notices.py" --resources "$APP/Contents/Resources" --web "$WEB"
# Component list of everything in Resources; mutti-migrate checks it on every
# start. It must describe the files as shipped, so it is written after the
# last change to Resources: before the ad-hoc signature (which leaves Resources
# unchanged) or, for a release, after every nested Mach-O file was signed and
# before the bundle seal. A release signs the list with the key outside the
# repository (MUTTI_COMPONENT_KEY[_ID]).
write_components() {
  (cd "$ROOT/mutti/migrate" && go run ./cmd/mutti-migrate components write "$APP/Contents/Resources")
  if [ -n "${MUTTI_COMPONENT_KEY:-}" ]; then
    (cd "$ROOT/mutti/hub" && go run ./cmd/mutti-release sign-components --key "$MUTTI_COMPONENT_KEY" --id "${MUTTI_COMPONENT_KEY_ID:?}" --resources "$APP/Contents/Resources")
  fi
}
# Local development signature by default. Release signing is a separate gate: with
# MUTTI_SIGN_IDENTITY set (a "Developer ID Application" identity in an unlocked keychain)
# the bundle is signed inside-out with the hardened runtime and secure timestamps, which
# notarization requires. --deep is not used there because it would sign nested code with
# the outer entitlements instead of per-executable ones.
# Note: release signing rewrites the hub and AI engine binaries, whose digests
# the AI qualification binds; a release is qualified on the signed package.
if [ -z "${MUTTI_SIGN_IDENTITY:-}" ]; then
  write_components
  codesign --force --deep --sign - "$APP"
  codesign --verify --deep --strict "$APP"
else
  RESOURCES="$APP/Contents/Resources"
  RUNTIME_ENTITLEMENTS="$ROOT/mutti/packaging/macos/runtime.entitlements"
  APP_ENTITLEMENTS="$ROOT/mutti/apps/macos/Mutti.entitlements"
  sign_hardened() { codesign --force --options runtime --timestamp --sign "$MUTTI_SIGN_IDENTITY" "$@"; }
  # 1. Every Mach-O file under Contents/Resources first (.dylib by name, everything else by
  #    magic), so a native library dropped into any payload directory is signed too. Only the
  #    .NET host gets the JIT entitlements; ffmpeg, ffprobe, mutti-connect and mutti-migrate
  #    run with the plain hardened runtime. Managed .dll files and web assets are not Mach-O.
  while IFS= read -r -d '' candidate; do
    case "$candidate" in
      *.dylib) ;;
      *) case "$(file -b "$candidate")" in Mach-O*) ;; *) continue;; esac ;;
    esac
    if [ "$candidate" = "$RESOURCES/server/jellyfin" ]; then
      sign_hardened --entitlements "$RUNTIME_ENTITLEMENTS" "$candidate"
    else
      sign_hardened "$candidate"
    fi
  done < <(find "$RESOURCES" -type f -print0)
  # 2. The component list over the signed files, then the bundle last: this signs the main
  #    executable Contents/MacOS/Mutti with the shell's entitlements and seals Contents/Resources
  #    (including the signatures and the list written above).
  write_components
  sign_hardened --entitlements "$APP_ENTITLEMENTS" "$APP"
  # spctl --assess only passes after notarization; the release workflow staples the ticket.
  codesign --verify --deep --strict "$APP"
fi
# Source/license inventory with open release points (report only).
python3 "$ROOT/mutti/packaging/inventory.py" --resources "$APP/Contents/Resources" --web "$WEB" \
  --json "$ROOT/build/macos/$RID/license-inventory.json" --md "$ROOT/build/macos/$RID/license-inventory.md"
echo "$APP"
