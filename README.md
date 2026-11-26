# Sablewright — Wails + Go

A small desktop app for tracking miniature painting: what stage each mini is at,
which paints you used, a dated log of every painting session, technique recipes,
and progress photos.

<img width="2203" height="1387" alt="dashboard" src="https://github.com/user-attachments/assets/64fc59d9-124e-4b37-b73e-3133b83f19a4" />


---

## Getting it

The [latest
release](https://github.com/Telrath-CG/sablewright/releases/latest) is the one
to take. Every version carries the same set of downloads:

| Platform | Download |
| --- | --- |
| Windows | `Sablewright-Windows.exe`, or `Sablewright-Windows-installer.exe` to install it properly |
| macOS | `Sablewright-macOS.dmg` — **Apple Silicon only** |
| Debian, Ubuntu | `sablewright_<version>-<build>_amd64.deb` |
| Fedora, RHEL, openSUSE | `sablewright-<version>-<build>.x86_64.rpm` |
| Arch | `sablewright-<version>-<build>-x86_64.pkg.tar.zst` |
| Other Linux | `Sablewright-Linux`, a bare binary |

The `<build>` in the Linux package names is the number of the build that made
them. It goes up with every build, so a newer download installs over an older
one, including a release over a `latest` build of the same version.

**Windows** — the bare `.exe` runs on Microsoft's WebView2, which Windows 11
always has and most Windows 10 PCs have picked up through updates. The
installer adds WebView2 if it's missing, so take that one if you're unsure.

Every merge to `main` also refreshes the [`latest`
prerelease](https://github.com/Telrath-CG/sablewright/releases/tag/latest) with
a build of whatever is on `main` right now. It holds the same set of downloads
and is replaced wholesale by the next merge, so take it only to try something
that hasn't been released yet.

**macOS** supports Apple Silicon (M1 and later) and nothing else. No Intel
build is produced, and the one inside the disk image will not run on an Intel
Mac. The app isn't signed or notarized either, so Gatekeeper blocks the first
launch: open the disk image, drag Sablewright into the Applications folder
beside it, and start it once. macOS refuses and offers only *Done* — go to
*System Settings → Privacy & Security*, scroll to the bottom, and click *Open
Anyway*. (Older instructions say to right-click and choose *Open* instead;
macOS 15 removed that shortcut for unsigned apps.)

**Linux** is better served by the packages than the bare binary where one
fits — they install to `/usr/bin` and add a menu entry, rather than leaving a
file to keep track of:

```
sudo apt install ./sablewright_2.0.0-<build>_amd64.deb
sudo dnf install ./sablewright-2.0.0-<build>.x86_64.rpm
sudo pacman -U sablewright-2.0.0-<build>-x86_64.pkg.tar.zst
```

All three depend on GTK 4 and WebKitGTK 6.0, which Ubuntu 24.04, Debian 13 and
newer releases ship as standard, and the package manager will say so plainly
if either is absent. Older releases such as Ubuntu 22.04 and Debian 12 don't
have them; stay on Sablewright 1.2.x there.

The bare binary is the fallback for distributions none of those cover, and for
running the app without installing it. It declares no dependencies, so a
missing library surfaces as a linker error rather than a readable message —
prefer a package wherever one fits. It also arrives without the executable
bit: `chmod +x Sablewright-Linux`.

### Upgrading from 1.x

- **Your collection comes with you.** It stays in the same data folder, and
  2.0 opens it as it is.
- **Quit 1.x before starting 2.0.** 1.x doesn't stop a second copy from
  running, and 2.0 can't see a 1.x that is still open, so one left open would
  keep saving over 2.0's changes.
- **On Windows**, if you installed 1.x with its installer, the 2.0 installer
  replaces it in place. A 1.x `.exe` kept somewhere else is simply left where
  it is.
- **On Linux**, 2.0 needs Ubuntu 24.04, Debian 13 or newer. Stay on 1.2.x on
  older releases.
- **The light/dark choice** may reset once, since the window's storage moved.
- **Going back** to 1.2.x works, but 1.x doesn't know about the trash for
  deleted photos: it drops those entries the next time it saves, after which
  photos deleted in 2.0 can't be put back.

---

## Using it

**Dashboard** — totals, a status breakdown, your backlog, what you finished
recently, and your latest painting sessions.

**Models** — click a mini to see it on the right. `+ Add Mini` to add one,
double-click to edit. Each mini holds a status, the paints used, technique
notes, photos, and a painting log.

**Projects** — a project is whatever your minis say they belong to. Put a
project name on a mini — an army, a tournament list, a boxed game — and it
appears here with everything else that shares it: how many of its minis are
painted, the hours logged against it, what to pick up next, and a deadline if
you set one. Projects sort by deadline, soonest first. Nothing has to be
created before it can be typed, and nothing is left behind when the last mini
using a name is renamed. *Edit* sets the deadline and notes, renames the
project across every mini at once, or ungroups them without deleting anything.

**Painting log** — the point of this is to record what actually happened at the
desk. Open a mini, go to the *Painting log* tab, and add a dated entry with an
optional time — `90`, `1h 30m` and `1:30` all work: *"Nuln Oil recess shading
done, started edge highlights."* Entries show newest-first as a timeline on the
mini's detail panel, and the total time is tallied per mini and across the
collection. If a mini has no start date, the earliest log entry becomes it. An
entry written but not yet added is saved along with the mini, and closing the
dialog with one still in the form asks first.

**Paint Inventory** — the rack comes stocked with 1,419 paints across six
ranges, so you tick off what you own rather than typing it all in:

| Range | Paints | Covering |
| --- | --- | --- |
| Warhammer Colour (formerly Citadel) | 372 | Base, Layer, Shade, Contrast, Dry, Technical, Glaze, Air |
| AK Interactive 3rd Gen | 486 | Standard, AFV, Air, Figures, Metallic, Primer, Ink, Intense, Pastel, Auxiliary |
| Ionic Smart Colors | 180 | the full range |
| Two Thin Coats | 180 | waves 1–3 |
| Pro Acryl | 131 | Pro Acryl, Signature Series, primers, washes |
| Kimera Kolors | 70 | the Pure line (Base Set, Colors of Nature, Shifted Primaries) and all six artist Signature Blends |

Each carries its real color, its maker's range and catalogue code, and how many
minis it's on. Citadel Air paints mostly share a name with a Base or Layer
paint, so they're listed the way the pot is labelled — *The Fang - Air*. Kimera
pots carry no catalogue number, so the Pure pigments are listed by the pigment
index printed on the bottle instead — *PW6*, *PBk7* — while the Signature
Blends, being mixes, carry no code at all. Kimera's swatches are approximated
rather than taken from a database, so they're the roughest in the set. Filter
by brand, range, type or ownership, or search across names,
ranges and product codes. Everything is editable and nothing is fixed: the brand
and range fields are free text with autocomplete, so any paint from any maker can
be added, and deleting one is permanent unless you ask for the built-in set back.

Paints start out unowned — tick "I own this paint" as you buy them, and the
dashboard counts what's actually on your desk. Opening a paint shows where it
turns up — every mini it's recorded on and every recipe that calls for it —
and each one clicks through to that record.

**Wishlist** — the shopping list, grouped by brand the way a shop is, with
*Copy list* to take it out of the house. Underneath it sits the list only your
own collection can produce: paints recorded on a mini that you don't own,
because you borrowed them, used them at a club, or ran the pot dry. *Got it*
ticks a paint off, adding it to the rack and taking it off the list in one go.

**Time at the Desk** — the painting log added up: this month, the last thirty
days, all time, and how many separate days you've actually sat down. A bar per
month for the last year, the average session, and the average time a *mini*
takes — which is where the batch counts earn their keep, since three hours
across a squad of ten is eighteen minutes a mini, and that's the figure that
tells you whether the next squad is an evening or a fortnight. Everything here
is worked out from sessions you've already logged; nothing extra is recorded.

**Export** — *Export* on a mini writes it out as a file to show someone, in
one of two formats picked before the save dialog opens:

- **Web page** — a single `.html` file with the photos embedded in it. It
  opens in any browser, and a **PDF** is that page printed from there
  (Ctrl/Cmd-P → *Save as PDF*); the page carries a print layout for it. There
  is no separate PDF export.
- **Markdown note** — an `.md` file for a forum or club post, with the photos
  copied into a folder beside it.

This is not a backup — it's the finished article, its paints, and the log of
how it got there.

**Technique Notes** — recipes and methods, searchable by title, body or tag.
A note can carry the paints it calls for, picked from the rack the same way a
mini's are, so the recipe shows real swatches and the paint knows which
recipes use it.

**Photos** — added through a native file picker and *copied* into the app's own
folder, so moving or deleting the originals later doesn't break anything.
Click a photo to open it full size. PNG, JPEG, GIF, BMP and WebP are accepted —
the formats every platform can draw.

A thumbnail is generated on import and is what the app actually draws, so a
mini with a dozen progress shots costs a fraction of the memory it used to;
the original is never touched and is still what opens when you click.

Photos come in three kinds. *Progress* and *Final* are your own work. A
*reference* is the maker's product image — the painted example off the box or
the shop page — added the same way, from a file you've saved yourself. Its job
is to be recognisable: a backlog is mostly minis you haven't photographed,
and a row with the studio paint job on it is far easier to pick out than
another blank square. It's also what you're working towards, so it's there to
look at while you paint.

The star on a photo makes it that mini's cover — the shot shown beside it in
the list — and that choice always wins. Choose nothing and the cover follows
where the mini has got to: the reference stands in while it's still on the
desk, and the newest final photo takes over once it's finished, falling back
to the newest progress shot when there's no final.

Reference shots are left out of exports. An export is a record of your own
work to hand to someone else, and the manufacturer's photograph isn't that.

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
move to a new computer. Importing moves your current collection and photos
into `backups/before-restore-<date>` first, so importing the wrong file can be
undone by hand. Only the database and the picture files it names as photos
are read from a backup; anything else in the zip is ignored.

On top of that the app copies `collection.json` into a `backups` folder every
time it starts, keeping the last three distinct copies — a launch that changed
nothing makes no new one. If the collection is ever found unreadable and set
aside, the copies from before that are kept too, for as long as the set-aside
file is in the folder. That's for the failure you only notice afterwards — a
bad import, a mass edit, a file that stopped being readable. Only the database
is copied, since the photos are large and don't change; a manual backup is
still the one that carries everything.

Only one copy of Sablewright runs at a time. Launching it again brings the open
window forward, since two copies would each save over the other's changes.

**Deleting a mini** puts it in the trash for 30 days rather than destroying
it, and the toast offers *Undo* on the spot. Notes and a status can be typed
again; a photo of a mini part-painted three months ago can't be retaken. It
leaves the app immediately either way — nothing reads the trash — and after
30 days it and its photos go for good. Deleting a single photo or a log entry
offers the same *Undo*, and a deleted photo's file is kept for the same 30
days.

**Keyboard** — Tab reaches the sidebar, and Enter or Space opens a screen. On
the Models screen, ↑ and ↓ walk the list and Enter opens the highlighted mini
for editing.

### Why JSON and not SQLite

A personal collection is hundreds of records, not millions, so a JSON file is
comfortably fast and brings a real benefit: no cgo, no C compiler, and no native
database library to build per platform. That's a large part of why the Windows
build needs nothing but Go. Every save is atomic — written to a temp file and
renamed — so an interrupted write can't corrupt your collection, and if the file
is ever unreadable it's moved aside rather than deleted, and the app says so
when it opens. If it can't be moved aside, the app stops rather than write over
it.

---

## Building from source

### Prerequisites

Every platform needs [Go 1.26 or newer](https://go.dev/dl/). Distribution
packages of Go are often older than that, so take it from go.dev if yours is.

The app is built with plain `go build`. The [Wails](https://v3.wails.io) CLI is
only a tool here — for the Windows resources and installer, and the macOS disk
image — and is pinned to the same version as the library:

```
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0
```

**There is no Node toolchain to install.** The frontend is plain HTML, CSS and
JavaScript committed under `frontend/dist`, so there is nothing to install or
bundle.

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

**Linux** — GTK 4 and WebKitGTK 6.0 development headers, plus a C compiler.
These first shipped in Ubuntu 24.04 and Debian 13; older releases can't build
or run Sablewright 2. On Debian and Ubuntu:

```
sudo apt install build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev
```

On Fedora: `sudo dnf install gtk4-devel webkitgtk6.0-devel`.

**Windows** — Go and the Wails CLI; no C compiler. Building the *installer*
also needs [NSIS](https://nsis.sourceforge.io) on `PATH`:

```
winget install NSIS.NSIS
```

### Building

The scripts build the app the same way the release build in
`.github/workflows/build.yml` does: `build-mac-or-linux.sh` on macOS and Linux,
which on macOS also makes the signed `.app` and the disk image, and
`Build Windows app.bat` on Windows, which makes the `.exe`. The Windows
installer and the Linux packages are left to CI, and to the commands below.

The app itself, on any platform:

```
go build -tags production -trimpath -o build/bin/Sablewright .
```

`go run .` runs it straight from the source for a quick look; there is no live
reload, so restart it after a change.

**Windows** needs its icon, file properties and manifest compiled into the
`.exe` first. The manifest matters: it is what lets the confirm dialogs show
their own button labels rather than a fixed Yes and No.

```
wails3 generate syso -arch amd64 -icon build/windows/icon.ico -manifest build/windows/wails.exe.manifest -info build/windows/info.json -out wails_windows_amd64.syso
go build -tags production -trimpath -ldflags "-H windowsgui" -o build/bin/Sablewright.exe .
```

`build/windows/info.json` holds `0.0.0` where the version goes; CI and the
build script put the real one into a copy before this step, and a build
straight from the file reports 0.0.0 in its file properties.

The installer bundles Microsoft's WebView2 bootstrapper, so it also works on a
PC without the WebView2 runtime; the bare `.exe` needs the runtime already
there, which Windows 11 always has:

```
wails3 generate webview2bootstrapper -dir build/windows/nsis
makensis -DARG_WAILS_AMD64_BINARY=%CD%\build\bin\Sablewright.exe -DINFO_PRODUCTVERSION=2.0.0 build\windows\nsis\project.nsi
```

**macOS** releases are an Apple Silicon `.app` in a disk image. The bundle is
put together from `build/darwin` — the binary, `Info.plist` and the icon — and
ad-hoc signed; `build-mac-or-linux.sh` shows the steps. The disk image is then:

```
wails3 tool package --format dmg --name Sablewright --out build/bin --volume-icon build/darwin/icons.icns
```

The **Linux** packages come from [nfpm](https://github.com/goreleaser/nfpm),
which wraps the finished binary — one config in `build/linux/nfpm.yaml`
covering all three formats, with no root and no containers involved:

```
go install github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.43.0
export VERSION=2.0.0 RELEASE=1
nfpm pkg -f build/linux/nfpm.yaml --packager deb       --target build/bin/
nfpm pkg -f build/linux/nfpm.yaml --packager rpm       --target build/bin/
nfpm pkg -f build/linux/nfpm.yaml --packager archlinux --target build/bin/
```

The `.rpm` requires sonames rather than package names, since the RPM
distributions disagree on what to call WebKit while providing the same
soname. Only the `.deb` and `.rpm` carry a glibc floor (2.39, from the Ubuntu
24.04 build); Arch is rolling, so its glibc is never older than the build's.
CI installs the `.deb` on Ubuntu 24.04 after building it, which proves the
Debian and Ubuntu package names; the `.rpm` and Arch names follow those
distributions' own packaging but aren't installed anywhere by CI.

### Version numbers

`info.version` in `build/config.yml` is the only place the version is written
down. CI and the build scripts read it there, hand it to the linker along with
the short commit hash, and stamp it into the macOS `Info.plist`, the Windows
file properties and installer, and the Linux packages:

```
-ldflags "-w -s -X main.version=2.0.0 -X main.commit=73ec59c"
```

The sidebar shows whatever was stamped in, which is why it reports the commit
as well as the number: every merge to `main` replaces the same rolling
release, so two downloads a week apart both call themselves the same version
and only the hash says which one a bug report is about. A build with nothing
stamped in says `dev` — that's `go run .` and a plain `go build`. Releasing a
new version means editing `build/config.yml` and nothing else; a pushed tag
that doesn't match it fails the build.

## License

Sablewright is licensed under the GNU General Public License version 3 — see
[LICENSE](LICENSE) for the full text. It is free to use, study, modify and
redistribute; a derivative has to ship its own source under the same terms.

Paint, range and game names are the trademarks of their respective owners.
Sablewright is not affiliated with, endorsed by, or sponsored by any of them.
