# Sablewright — Wails + Go

A small desktop app for tracking miniature painting: what stage each mini is at,
which paints you used, a dated log of every painting session, technique recipes,
and progress photos.

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
