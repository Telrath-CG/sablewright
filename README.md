# Sablewright — Wails + Go

A small desktop app for tracking miniature painting: what stage each mini is at,
which paints you used, a dated log of every painting session, technique recipes,
and progress photos.

---

## Getting it

Every merge to `main` publishes a fresh build to the [`latest`
release](https://github.com/Telrath-CG/sablewright/releases/tag/latest), so
that tag is always the current build of `main`:

| Platform | Download |
| --- | --- |
| Windows | `Sablewright-Windows.exe`, or `Sablewright-Windows-installer.exe` to install it properly |
| macOS | `Sablewright-macOS.zip` |
| Linux | `Sablewright-Linux` |

Two things the downloads need on first run. On macOS the app isn't signed or
notarised, so Gatekeeper refuses it on a double-click — right-click the app and
choose *Open*, which offers to run it anyway. On Linux the binary arrives
without the executable bit: `chmod +x Sablewright-Linux`.

---

## Using it

**Dashboard** — totals, a status breakdown, your backlog, what you finished
recently, and your latest painting sessions.

**Models** — click a mini to see it on the right. `+ Add Mini` to add one,
double-click to edit. Each mini holds a status, the paints used, technique
notes, photos, and a painting log.

**Painting log** — the point of this is to record what actually happened at the
desk. Open a mini, go to the *Painting log* tab, and add a dated entry with an
optional number of minutes: *"Nuln Oil recess shading done, started edge
highlights."* Entries show newest-first as a timeline on the mini's detail
panel, and the total time is tallied per mini and across the collection. If a
mini has no start date, the earliest log entry becomes it.

**Paint Inventory** — every paint with its real color, brand and type, and how
many minis it's on. The brand field is free text with autocomplete: type any
brand and it's remembered and offered next time. Untick "I own this paint" to
keep it as a wishlist item. An empty rack offers a starter set of ~36 common
paints.

**Technique Tips** — recipes and methods, searchable by title, body or tag.

**Photos** — added through a native file picker and *copied* into the app's own
folder, so moving or deleting the originals later doesn't break anything.
Click a photo to open it full size.

---

## Your data

Everything lives in one folder:

| OS | Location |
| --- | --- |
| Windows | `%APPDATA%\Sablewright` |
| macOS | `~/Library/Application Support/Sablewright` |
| Linux | `~/.local/share/sablewright` |

It holds `collection.json` and a `photos` folder. *Data folder* in the sidebar
opens it. Nothing is uploaded anywhere; the app makes no network connections.

**Back it up** with *Backup data* in the sidebar — one zip containing the
database and every photo. *Import backup* restores one, which is also how you
move to a new computer.

### Why JSON and not SQLite

A personal collection is hundreds of records, not millions, so a JSON file is
comfortably fast and brings a real benefit: no cgo, no C compiler, and no native
database library to build per platform. That's a large part of why the Windows
build needs nothing but Go. Every save is atomic — written to a temp file and
renamed — so an interrupted write can't corrupt your collection, and if the file
is ever unreadable it's moved aside rather than deleted.

---

## Building from source

### Prerequisites

Every platform needs [Go 1.25 or newer](https://go.dev/dl/) and the Wails CLI:

```
go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2
```

**There is no Node toolchain to install.** Wails projects normally want npm,
but this frontend is plain HTML, CSS and JavaScript committed under
`frontend/dist`, so the frontend build hooks in `wails.json` are empty and
there is nothing to install or bundle.

Past that, each platform needs its own native pieces.

**macOS** — the Xcode Command Line Tools, which supply the compiler and system
headers Wails links against:

```
xcode-select --install
```

[Homebrew](https://brew.sh) is the least painful way to get Go itself:

```
brew install go
```

**Linux** — GTK 3 and WebKit2GTK development headers, plus a C compiler. On
Debian and Ubuntu:

```
sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.0-dev
```

Ubuntu 24.04 and newer ship WebKit2GTK **4.1** rather than 4.0. There you want
`libwebkit2gtk-4.1-dev` instead, and the build needs `-tags webkit2_41`. CI
pins `ubuntu-22.04` precisely to stay on the 4.0 side of that split.

**Windows** — nothing beyond Go. The WebView2 runtime ships with Windows 11 and
is embedded into the binary at build time regardless. Building the *installer*
additionally needs [NSIS](https://nsis.sourceforge.io) on `PATH`:

```
winget install NSIS.NSIS
```

### Building

```
wails dev        # run with live reload
wails build      # binary into build/bin
```

The Windows installer is a separate invocation:

```
wails build -platform windows/amd64 -webview2 embed -nsis
```
