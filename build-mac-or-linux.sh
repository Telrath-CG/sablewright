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
    # Ubuntu 24.04+ only ships WebKit 4.1, which needs an extra build tag
    if pkg-config --exists webkit2gtk-4.0 2>/dev/null; then
      WAILS_TAGS=""
      say "      Found webkit2gtk-4.0."
    elif pkg-config --exists webkit2gtk-4.1 2>/dev/null; then
      WAILS_TAGS="-tags webkit2_41"
      say "      Found webkit2gtk-4.1 (using the webkit2_41 build tag)."
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
echo
# shellcheck disable=SC2086
"$GOBIN/wails" build $WAILS_TAGS -ldflags "-w -s" || fail "The build failed - the compiler error is above."
echo

OUT="$(find build/bin -maxdepth 1 -mindepth 1 ! -name '*.zip' 2>/dev/null | head -1)"
[ -n "$OUT" ] || fail "The build finished but produced nothing in build/bin."

echo "  ============================================================"
echo "   Done."
echo "   Built: $OUT"
case "$(uname -s)" in
  Darwin) echo "   Drag the .app into /Applications and you're set."
          echo "   (It isn't code-signed, so the first launch needs"
          echo "    right-click -> Open to get past Gatekeeper.)" ;;
  Linux)  echo "   That's a self-contained binary - run it directly." ;;
esac
echo "  ============================================================"
echo
