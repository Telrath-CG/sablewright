#!/usr/bin/env bash
# ===========================================================
#  Sablewright - build for macOS or Linux
#
#  Run:  ./build-mac-or-linux.sh
#  (first: chmod +x build-mac-or-linux.sh)
# ===========================================================
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$HERE"

WAILS_VERSION="v2.10.2"

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
    *)      say "Go isn't installed. Either:"
            say "  sudo apt install golang-go     (Debian/Ubuntu)"
            say "  ...or download it from https://go.dev/dl/" ;;
  esac
  echo
  fail "Install Go, then run this script again."
fi
say "      Found $(go version)"

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
    WAILS_TAGS=""
    ;;
  Linux)
    say "[2/4] Checking GTK/WebKit headers..."
    # 4.1 first: it is what current distributions ship, and what the released
    # packages are built against. 4.0 stays as a fallback for older systems
    # that only have it, and needs no build tag.
    if pkg-config --exists webkit2gtk-4.1 2>/dev/null; then
      WAILS_TAGS="-tags webkit2_41"
      say "      Found webkit2gtk-4.1 (using the webkit2_41 build tag)."
    elif pkg-config --exists webkit2gtk-4.0 2>/dev/null; then
      WAILS_TAGS=""
      say "      Found webkit2gtk-4.0."
    else
      echo
      say "Missing the GTK/WebKit development headers. Install them with:"
      say "  sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev"
      say "  # older distros: libwebkit2gtk-4.0-dev"
      echo
      fail "Install those, then run this script again."
    fi
    ;;
  *) fail "Unsupported system: $(uname -s). Use the .bat script on Windows." ;;
esac

# ------------------------------------------------------- 3. Wails CLI
say "[3/4] Installing the Wails build tool..."
GOBIN="$(go env GOBIN)"
[ -n "$GOBIN" ] || GOBIN="$(go env GOPATH)/bin"
export PATH="$GOBIN:$PATH"

if [ -x "$GOBIN/wails" ]; then
  say "      Already installed."
else
  go install "github.com/wailsapp/wails/v2/cmd/wails@${WAILS_VERSION}" \
    || fail "Could not install the Wails tool (check your internet connection)."
fi
[ -x "$GOBIN/wails" ] || fail "The Wails tool isn't at $GOBIN/wails after installing."
say "      Ready."

# ---------------------------------------------------------- 4. build
say "[4/4] Fetching packages and building (slow the first time)..."
go mod tidy || fail "Could not download the Go packages."

# The version is read from wails.json rather than repeated here, and the
# commit is stamped alongside it so a build can say exactly what it came from.
# Both are optional: without them the app just calls itself a dev build.
VERSION="$(sed -n 's/.*"productVersion"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' wails.json | head -1)"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || true)"
LDFLAGS="-w -s"
[ -n "$VERSION" ] && LDFLAGS="$LDFLAGS -X main.version=$VERSION"
[ -n "$COMMIT" ]  && LDFLAGS="$LDFLAGS -X main.commit=$COMMIT"

echo
# shellcheck disable=SC2086
"$GOBIN/wails" build $WAILS_TAGS -ldflags "$LDFLAGS" || fail "The build failed - the compiler error is above."
echo

OUT="$(find build/bin -maxdepth 1 -mindepth 1 ! -name '*.dmg' 2>/dev/null | head -1)"
[ -n "$OUT" ] || fail "The build finished but produced nothing in build/bin."

# ------------------------------------------ macOS disk image (optional extra)
# The releases ship a .dmg, so produce one here too when the tool for it is
# installed. Not a hard requirement: the .app on its own is what you want for
# testing your own build, and the disk image only earns its keep when you are
# handing the build to someone else.
DMG=""
if [ "$(uname -s)" = "Darwin" ] && [ -d build/bin/Sablewright.app ]; then
  if command -v create-dmg >/dev/null 2>&1; then
    say "      Packaging a disk image..."

    # create-dmg images the *contents* of the folder it is given, so the .app
    # needs a staging directory to itself or the bundle gets unwrapped into
    # the image. ditto rather than cp: it keeps the permissions and extended
    # attributes that make the app launchable.
    STAGE="$(mktemp -d)"
    ditto build/bin/Sablewright.app "$STAGE/Sablewright.app"

    # --volname leads the array so it is never empty: macOS ships bash 3.2,
    # where expanding an empty array trips the `set -u` above.
    OPTS=(--volname Sablewright)
    ICNS="$(ls build/bin/Sablewright.app/Contents/Resources/*.icns 2>/dev/null | head -1 || true)"
    [ -n "$ICNS" ] && OPTS+=(--volicon "$ICNS")
    OPTS+=(--window-pos 200 120 --window-size 600 400 --icon-size 110
           --icon Sablewright.app 150 190
           --hide-extension Sablewright.app
           --app-drop-link 450 190)

    rm -f build/bin/Sablewright.dmg
    create-dmg "${OPTS[@]}" build/bin/Sablewright.dmg "$STAGE" >/dev/null 2>&1
    rm -rf "$STAGE"

    if [ -f build/bin/Sablewright.dmg ]; then
      DMG="build/bin/Sablewright.dmg"
    else
      say "      (create-dmg didn't produce one - the .app below is fine.)"
    fi
  else
    say "      Skipping the .dmg - install it with 'brew install create-dmg'."
  fi
  echo
fi

echo "  ============================================================"
echo "   Done."
echo "   Built: $OUT"
[ -n "$DMG" ] && echo "   Disk image: $DMG"
case "$(uname -s)" in
  Darwin) echo "   Drag the .app into /Applications and you're set."
          echo "   (It isn't code-signed, so the first launch is blocked."
          echo "    Open System Settings -> Privacy & Security and click"
          echo "    'Open Anyway' near the bottom.)" ;;
  Linux)  echo "   That's a self-contained binary - run it directly." ;;
esac
echo "  ============================================================"
echo
