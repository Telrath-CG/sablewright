#!/usr/bin/env bash
# ===========================================================
#  Sablewright - build for macOS or Linux
#
#  Run:  ./build-mac-or-linux.sh
#
#  The same steps as the release build in .github/workflows/build.yml, so a
#  local build is the app that ships.
# ===========================================================
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$HERE"

WAILS_VERSION="v3.1.0"
GO_MIN_MINOR=26   # Go 1.26 or newer

say()  { printf '  %s\n' "$*"; }
fail() { printf '\n  [X] %s\n\n' "$*"; exit 1; }

echo
echo "  Sablewright - build"
echo "  ============================="
echo

[ -f main.go ] || fail "Run this from the project folder (the one with main.go)."

# ---------------------------------------------------------------- 1. Go
say "[1/4] Checking for Go..."
if ! command -v go >/dev/null 2>&1; then
  echo
  case "$(uname -s)" in
    Darwin) say "Go isn't installed. Either:"
            say "  brew install go"
            say "  ...or download the .pkg from https://go.dev/dl/" ;;
    *)      say "Go isn't installed. Download it from https://go.dev/dl/"
            say "(the golang-go package in most distributions is too old)." ;;
  esac
  echo
  fail "Install Go, then run this script again."
fi
# Distribution packages of Go often lag well behind, and Wails v3 needs 1.26.
GOVER="$(go env GOVERSION)"                      # e.g. go1.26.11
MINOR="$(printf '%s' "$GOVER" | sed -n 's/^go1\.\([0-9]*\).*/\1/p')"
if [ -z "$MINOR" ] || [ "$MINOR" -lt "$GO_MIN_MINOR" ]; then
  fail "Found $GOVER, but Sablewright needs Go 1.$GO_MIN_MINOR or newer. Get it from https://go.dev/dl/"
fi
say "      Found $GOVER"

# ---------------------------------------------- 2. platform build deps
case "$(uname -s)" in
  Darwin)
    say "[2/4] Checking Xcode command line tools..."
    if ! xcode-select -p >/dev/null 2>&1; then
      say "      Not installed - launching the installer."
      say "      Accept the prompt, wait for it to finish, then re-run this."
      xcode-select --install || true
      exit 1
    fi
    say "      Ready."
    ;;
  Linux)
    say "[2/4] Checking GTK 4 and WebKitGTK 6.0 headers..."
    if pkg-config --exists gtk4 webkitgtk-6.0 2>/dev/null; then
      say "      Found."
    else
      echo
      say "Missing the GTK 4 / WebKitGTK 6.0 development headers. These need"
      say "Ubuntu 24.04, Debian 13, or newer. Install them with:"
      say "  sudo apt install build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev"
      say "  # Fedora: sudo dnf install gtk4-devel webkitgtk6.0-devel"
      echo
      fail "Install those, then run this script again."
    fi
    ;;
  *) fail "Unsupported system: $(uname -s). Use the .bat script on Windows." ;;
esac

# ------------------------------------------------ 3. Wails CLI (macOS)
# The app itself builds with plain `go build`. On macOS the Wails CLI is used
# for one thing, the disk image; Linux doesn't need it.
GOBIN="$(go env GOBIN)"
[ -n "$GOBIN" ] || GOBIN="$(go env GOPATH)/bin"
if [ "$(uname -s)" = "Darwin" ]; then
  say "[3/4] Installing the Wails tool (for the disk image)..."
  # Always installed, even over an existing copy: `go install` of a pinned
  # version is quick when it's already there, and an older one left over
  # from a previous build would otherwise be used without a word.
  go install "github.com/wailsapp/wails/v3/cmd/wails3@${WAILS_VERSION}" \
    || fail "Could not install the Wails tool (check your internet connection)."
  say "      Ready."
else
  say "[3/4] (No extra tools needed on Linux.)"
fi

# ---------------------------------------------------------- 4. build
say "[4/4] Fetching packages and building (slow the first time)..."
# download, not tidy: go.mod and go.sum are committed and say exactly what to
# fetch, and a build shouldn't rewrite them.
go mod download || fail "Could not download the Go packages (check your internet connection)."

# The version is read from build/config.yml rather than repeated here, and the
# commit is stamped alongside it so a build can say exactly what it came from.
# Both are optional: without them the app just calls itself a dev build.
VERSION="$(awk '/^info:/{f=1;next} /^[^[:space:]#]/{f=0}
                f && $1=="version:"{v=$2; gsub(/"/,"",v); print v; exit}' build/config.yml)"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || true)"
LDFLAGS="-w -s"
[ -n "$VERSION" ] && LDFLAGS="$LDFLAGS -X main.version=$VERSION"
[ -n "$COMMIT" ]  && LDFLAGS="$LDFLAGS -X main.commit=$COMMIT"

mkdir -p build/bin
echo
if [ "$(uname -s)" = "Darwin" ]; then
  # 12.0 is the oldest macOS Wails v3 supports.
  export MACOSX_DEPLOYMENT_TARGET=12.0
  export CGO_CFLAGS=-mmacosx-version-min=12.0 CGO_LDFLAGS=-mmacosx-version-min=12.0
fi
go build -tags production -trimpath -buildvcs=false -ldflags "$LDFLAGS" -o build/bin/Sablewright . \
  || fail "The build failed - the compiler error is above."
echo

OUT="build/bin/Sablewright"
DMG=""
if [ "$(uname -s)" = "Darwin" ]; then
  # Put the .app together the way the release build does: the binary, the
  # Info.plist with this version, the icon and the license files, then an
  # ad-hoc signature over the whole bundle so it opens as one app rather than
  # being called damaged.
  say "      Assembling Sablewright.app..."
  APP=build/bin/Sablewright.app
  rm -rf "$APP"
  mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
  mv build/bin/Sablewright "$APP/Contents/MacOS/Sablewright"
  cp build/darwin/Info.plist "$APP/Contents/Info.plist"
  if [ -n "$VERSION" ]; then
    plutil -replace CFBundleShortVersionString -string "$VERSION" "$APP/Contents/Info.plist"
    plutil -replace CFBundleVersion -string "$VERSION" "$APP/Contents/Info.plist"
  fi
  cp build/darwin/icons.icns LICENSE THIRD-PARTY-NOTICES.md "$APP/Contents/Resources/"
  codesign --force --deep --sign - \
    --identifier "$(plutil -extract CFBundleIdentifier raw "$APP/Contents/Info.plist")" "$APP" \
    || fail "Could not sign the app bundle."
  OUT="$APP"

  say "      Packaging a disk image..."
  rm -f build/bin/Sablewright.dmg
  if "$GOBIN/wails3" tool package --format dmg --name Sablewright --out build/bin >/dev/null 2>&1 \
     && [ -f build/bin/Sablewright.dmg ]; then
    DMG="build/bin/Sablewright.dmg"
  else
    say "      (No disk image was made - the .app below is fine.)"
  fi
  echo
fi

echo "  ============================================================"
echo "   Done."
echo "   Built: $OUT"
[ -n "$DMG" ] && echo "   Disk image: $DMG"
case "$(uname -s)" in
  Darwin) echo "   Drag the .app into /Applications and you're set."
          echo "   (Built here, it opens without a prompt. A copy given to"
          echo "    someone else isn't signed by Apple, so their first launch"
          echo "    is blocked until they click 'Open Anyway' in"
          echo "    System Settings -> Privacy & Security.)" ;;
  Linux)  echo "   That's a self-contained binary - run it directly." ;;
esac
echo "  ============================================================"
echo
