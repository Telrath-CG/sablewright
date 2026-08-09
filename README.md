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
| Debian, Ubuntu | `sablewright_<version>_amd64.deb` |
| Fedora, RHEL, openSUSE | `sablewright-<version>-1.x86_64.rpm` |
| Arch | `sablewright-<version>-1-x86_64.pkg.tar.zst` |
| Other Linux | `Sablewright-Linux`, a bare binary |

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
sudo apt install ./sablewright_1.2.0_amd64.deb
sudo dnf install ./sablewright-1.2.0-1.x86_64.rpm
sudo pacman -U sablewright-1.2.0-1-x86_64.pkg.tar.zst
```

All three depend on GTK 3 and WebKit2GTK 4.1, which current distributions ship
as standard, and the package manager will say so plainly if either is absent.

The bare binary is the fallback for distributions none of those cover, and for
running the app without installing it. It declares no dependencies, so a
missing library surfaces as a linker error rather than a readable message —
prefer a package wherever one fits. It also arrives without the executable
bit: `chmod +x Sablewright-Linux`.

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
optional number of minutes: *"Nuln Oil recess shading done, started edge
highlights."* Entries show newest-first as a timeline on the mini's detail
panel, and the total time is tallied per mini and across the collection. If a
mini has no start date, the earliest log entry becomes it.

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

**Export** — *Export* on a mini writes it out as a file to show someone: an
`.html` page with the photos embedded, which opens in any browser and prints
to PDF, or an `.md` note for a forum or club post with the photos copied into
a folder beside it. The save dialog's file type decides which. This is not a
backup — it's the finished article, its paints, and the log of how it got
there.

**Technique Notes** — recipes and methods, searchable by title, body or tag.
A note can carry the paints it calls for, picked from the rack the same way a
mini's are, so the recipe shows real swatches and the paint knows which
recipes use it.

**Photos** — added through a native file picker and *copied* into the app's own
folder, so moving or deleting the originals later doesn't break anything.
Click a photo to open it full size.

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
move to a new computer.

On top of that the app copies `collection.json` into a `backups` folder every
time it starts, keeping the last three. That's for the failure you only notice
afterwards — a bad import, a mass edit, a file that stopped being readable.
Only the database is copied, since the photos are large and don't change;
a manual backup is still the one that carries everything.

**Deleting a mini** puts it in the trash for 30 days rather than destroying
it, and the toast offers *Undo* on the spot. Notes and a status can be typed
again; a photo of a mini part-painted three months ago can't be retaken. It
leaves the app immediately either way — nothing reads the trash — and after
30 days it and its photos go for good.

**Keyboard** — on the Models screen, ↑ and ↓ walk the list and Enter opens
the highlighted mini for editing.

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
sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
```

WebKit2GTK comes in a 4.0 and a 4.1 flavor and they are not interchangeable:
building against 4.1 needs `-tags webkit2_41`. Releases target 4.1, because
4.0 is gone from Ubuntu 24.04, Debian 13 and Arch, and a package built against
it would be uninstallable on most current systems.
`build-mac-or-linux.sh` detects which of the two you have and picks the tag to
match. CI still runs on `ubuntu-22.04` — for its older glibc, which is what
lets one binary run on old distributions as well as new — but installs the 4.1
headers there rather than the 4.0 that image defaults to.

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

The macOS release is an Apple Silicon build wrapped in a disk image. Wails v2
has no packaging target for that, so CI hands the finished bundle to
[create-dmg](https://github.com/create-dmg/create-dmg):

```
brew install create-dmg
wails build -platform darwin/arm64 -ldflags "-w -s"
```

`build-mac-or-linux.sh` runs the same build and produces the `.dmg` alongside
the `.app` whenever create-dmg is installed, and just the `.app` when it isn't.

The Linux packages come from [nfpm](https://github.com/goreleaser/nfpm), which
wraps the finished binary — one config in `build/linux/nfpm.yaml` covering all
three formats, with no root and no containers involved:

```
go install github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.43.0
export VERSION=1.2.0
nfpm pkg -f build/linux/nfpm.yaml --packager deb       --target build/bin/
nfpm pkg -f build/linux/nfpm.yaml --packager rpm       --target build/bin/
nfpm pkg -f build/linux/nfpm.yaml --packager archlinux --target build/bin/
```

The dependency lists differ per format in ways worth knowing about. The `.deb`
spells out `libgtk-3-0 | libgtk-3-0t64`, because Ubuntu 24.04's 64-bit `time_t`
transition renamed that package. The `.rpm` requires sonames rather than
package names, since Fedora and openSUSE disagree on what to call WebKit while
providing the same soname. Only the `.deb` and `.rpm` carry a glibc floor;
Arch is rolling, so its glibc is never older than the build's.

### Version numbers

`productVersion` in `wails.json` is the only place the version is written
down. Wails already reads it there for the Windows file properties; the build
scripts read the same field and hand it to the linker along with the short
commit hash:

```
-ldflags "-w -s -X main.version=1.2.0 -X main.commit=73ec59c"
```

The sidebar shows whatever was stamped in, which is why it reports the commit
as well as the number: every merge to `main` replaces the same rolling
release, so two downloads a week apart both call themselves the same version
and only the hash says which one a bug report is about. A build with nothing
stamped in says `dev` — that's `wails dev` and a plain `go build`. Releasing a
new version means editing `wails.json` and nothing else.

## License

Sablewright is licensed under the GNU General Public License version 3 — see
[LICENSE](LICENSE) for the full text. It is free to use, study, modify and
redistribute; a derivative has to ship its own source under the same terms.

The released binaries carry third-party work under its own terms. The built-in
paint library takes its hex swatch values from `miniature-paints`, which is
MIT, and the binary statically links Wails and seventeen other Go modules
under MIT, BSD 2-Clause, BSD 3-Clause and Apache 2.0. Every one of those
notices is reproduced in [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md),
which the Linux packages install to `/usr/share/doc/sablewright/`.

All of them are compatible with GPL-3.0. Apache 2.0 is the one that would not
have been under GPL-2.0, which is a reason not to go backwards.

Paint, range and game names are the trademarks of their respective owners.
Sablewright is not affiliated with, endorsed by, or sponsored by any of them.
