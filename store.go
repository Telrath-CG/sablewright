package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Data types
//
// Storage is a single JSON file rather than SQLite. For a personal collection
// (hundreds of records, not millions) this is more than fast enough, and it
// keeps the app dependency-free: no cgo, no C compiler, no native database
// library to build per platform. The file is human-readable and trivial to
// back up, and every write is atomic (temp file + rename) so a crash or a
// full disk can't leave a half-written database behind.
// ---------------------------------------------------------------------------

type Photo struct {
	ID   int    `json:"id"`
	File string `json:"file"` // filename inside the photos folder
	// Thumb is the generated small copy, or empty for a photo in a format
	// the decoder could not read. Everywhere a thumbnail is drawn falls back
	// to File, so an empty one costs memory rather than a broken image.
	Thumb string `json:"thumb"`
	// Kind is "Product", "Progress" or "Final". The first is the maker's own
	// photograph of a painted example - a reference to work towards, not a
	// record of work done - which is why it is the one kind an export leaves
	// out. Nothing validates this: it is written by the caller and only ever
	// compared against, so an unknown value reads as a progress shot rather
	// than breaking anything.
	Kind  string `json:"kind"`
	Added string `json:"added"` // YYYY-MM-DD
	// Cover marks the one photo that stands for the mini in the list. At most
	// one per model; with none set, CoverPhoto picks the best candidate.
	Cover bool `json:"cover"`
}

// Session is one sitting at the desk: what got done, and how long it took.
// Kept separate from the mini's general Notes so there's a real history of
// how a model came together rather than one ever-growing blob of text.
type Session struct {
	ID      int    `json:"id"`
	Date    string `json:"date"`    // YYYY-MM-DD
	Minutes int    `json:"minutes"` // optional, 0 = not recorded
	Notes   string `json:"notes"`
}

// Model is one entry in the collection, which is not the same thing as one
// miniature. A squad is painted as a single job - same paints, same evening,
// one line in the log - so it is one entry carrying the number of minis in it
// rather than ten near-identical records. Count says how many, Done says how
// many of them are finished, and Status describes the rest. A character who
// deserves his own photos and his own log simply gets his own entry, which is
// how a painter thinks of him anyway.
type Model struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	GameSystem string `json:"gameSystem"`
	Faction    string `json:"faction"`
	Status     string `json:"status"`
	// Project groups entries into a thing being worked towards - an army, a
	// tournament list, a boxed game. Free text with autocomplete, exactly as
	// Brand works on a paint: the list of projects is whatever is in use, so
	// there is no separate record to create first and none left behind.
	Project   string    `json:"project"`
	Count     int       `json:"count"` // minis in this entry; at least 1
	Done      int       `json:"done"`  // how many of them are finished
	Favorite  bool      `json:"favorite"`
	Notes     string    `json:"notes"`
	Started   string    `json:"started"`
	Completed string    `json:"completed"`
	Created   string    `json:"created"`
	PaintIDs  []int     `json:"paintIds"`
	Photos    []Photo   `json:"photos"`
	Sessions  []Session `json:"sessions"`
}

// finished is true of the statuses that mean the painting is over. Display is
// a finished mini that has somewhere to stand, not a stage on the way there.
func finished(status string) bool { return status == "Complete" || status == "Display" }

// reference is true of a photo kind that belongs to the manufacturer rather
// than to the painter. Kept as a function beside finished() so the string
// lives in one place: the cover order and the two exporters all ask this.
func reference(kind string) bool { return kind == "Product" }

// Minis reports how many miniatures this entry stands for and how many of
// them are painted, with both clamped into sense. A file written by an older
// build has no count at all, and one edited by hand can say anything, so
// every reader goes through here rather than trusting the two fields.
func (m Model) Minis() (total, done int) {
	total = m.Count
	if total < 1 {
		total = 1
	}
	done = m.Done
	if finished(m.Status) {
		done = total
	}
	if done < 0 {
		done = 0
	}
	if done > total {
		done = total
	}
	return total, done
}

// CoverPhoto is the shot that stands for this mini in the list, or nil if it
// has none. An explicit choice always wins. Failing that the default follows
// what the mini is: a finished one is represented by the finished article,
// since that is what you would point at, while one still on the desk is
// represented by the maker's reference shot, since a row is being picked out
// of a list by eye and a studio photograph is far more recognisable than a
// half-painted one - and most of a backlog has no photograph at all. Within a
// kind the newest wins, being the one closest to how it looks now.
func (m Model) CoverPhoto() *Photo {
	var product, final, progress *Photo
	for i := range m.Photos {
		p := &m.Photos[i]
		if p.Cover {
			return p
		}
		switch {
		case reference(p.Kind):
			product = p
		case p.Kind == "Final":
			final = p
		default:
			progress = p
		}
	}
	order := []*Photo{product, final, progress}
	if finished(m.Status) {
		order = []*Photo{final, product, progress}
	}
	for _, p := range order {
		if p != nil {
			return p
		}
	}
	return nil
}

// TotalMinutes adds up recorded session time for this mini.
func (m Model) TotalMinutes() int {
	n := 0
	for _, s := range m.Sessions {
		n += s.Minutes
	}
	return n
}

type Paint struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Brand string `json:"brand"`
	Range string `json:"range"` // the maker's own line: "Layer", "Wave 2", "3rd Gen Air"
	Code  string `json:"code"`  // the maker's catalogue number, where they use one
	Type  string `json:"type"`
	Hex   string `json:"hex"`
	Owned bool   `json:"owned"`
	// Wishlist marks a paint to buy. It's deliberately independent of Owned:
	// a pot you have but have nearly used up belongs on the shopping list too.
	Wishlist bool   `json:"wishlist"`
	Notes    string `json:"notes"`
}

type Tip struct {
	ID       int      `json:"id"`
	Title    string   `json:"title"`
	Category string   `json:"category"`
	Body     string   `json:"body"`
	Tags     []string `json:"tags"`
	// PaintIDs are the paints this recipe calls for. A recipe written as
	// prose is a recipe you can't search by paint, and "what did I use
	// Nuln Oil for" is a question worth being able to ask of the rack.
	PaintIDs []int  `json:"paintIds"`
	Created  string `json:"created"`
}

// MiniRef and TipRef are enough of a mini or a note to list it and click
// through to it, without dragging its photos, sessions and body text across
// to draw one line.
type MiniRef struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Count  int    `json:"count"`
	Done   int    `json:"done"`
}

type TipRef struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Category string `json:"category"`
}

// Trashed is a deleted mini, kept for a while in case the delete was a
// mistake. Its photos stay where they are on disk until it is purged: they
// are the part of a mini that cannot be typed back in.
type Trashed struct {
	Model   Model  `json:"model"`
	Deleted string `json:"deleted"` // YYYY-MM-DD
}

// ProjectMeta is what a project carries beyond the minis in it. The minis
// themselves are the project: it exists because entries name it, exactly as a
// brand exists because paints name it. This is only the deadline and the
// notes, which have nowhere else to live - and a project with neither has no
// row here at all.
type ProjectMeta struct {
	Name  string `json:"name"` // matches Model.Project exactly
	Due   string `json:"due"`  // YYYY-MM-DD, optional
	Notes string `json:"notes"`
}

type Data struct {
	Version  int           `json:"version"`
	NextID   int           `json:"nextId"`
	Models   []Model       `json:"models"`
	Paints   []Paint       `json:"paints"`
	Tips     []Tip         `json:"tips"`
	Trash    []Trashed     `json:"trash"`
	Projects []ProjectMeta `json:"projects"`
}

// Store owns the data file and guards it with a mutex.
type Store struct {
	mu   sync.RWMutex
	dir  string
	path string
	data Data
}

var Statuses = []string{"Backlog", "Assembled", "Primed", "In Progress", "Complete", "Display"}

// dataVersion is the shape of the file on disk. Bumping it runs migrate() once
// against any collection saved by an older build.
//
//	1  the original
//	2  paints carry a range and a code, and every collection is stocked with
//	   the built-in paint library
//	3  the library gained the Citadel Air range
//	4  the library gained Kimera Kolors, Pure and Signature Blends alike
//	5  a model entry carries how many minis it is, and how many are finished
const dataVersion = 5

func today() string { return time.Now().Format("2006-01-02") }

// DataDir returns the per-user folder where everything is kept.
func DataDir() string { return appDir("Sablewright", "sablewright") }

// appDir builds the platform-conventional data path. name is used on Windows
// and macOS, which prefer title case; unixName is used on Linux, which doesn't.
func appDir(name, unixName string) string {
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("APPDATA")
		if base == "" {
			base, _ = os.UserHomeDir()
		}
		return filepath.Join(base, name)
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", name)
	default:
		if x := os.Getenv("XDG_DATA_HOME"); x != "" {
			return filepath.Join(x, unixName)
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", unixName)
	}
}

func NewStore() (*Store, error) {
	dir := DataDir()
	if err := os.MkdirAll(filepath.Join(dir, "photos"), 0o755); err != nil {
		return nil, fmt.Errorf("could not create the data folder: %w", err)
	}
	s := &Store{dir: dir, path: filepath.Join(dir, "collection.json")}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) PhotoDir() string { return filepath.Join(s.dir, "photos") }
func (s *Store) Dir() string      { return s.dir }

func (s *Store) load() error {
	b, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		s.data = Data{Version: dataVersion, NextID: 1}
		s.addLibraryPaints()
		return s.persist()
	}
	if err != nil {
		return fmt.Errorf("could not read your collection: %w", err)
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		// Don't destroy the file we failed to parse - move it aside so the
		// user still has it, and carry on with an empty collection.
		bad := s.path + ".corrupt-" + time.Now().Format("20060102-150405")
		_ = os.Rename(s.path, bad)
		s.data = Data{Version: dataVersion, NextID: 1}
		s.addLibraryPaints()
		_ = s.persist()
		return fmt.Errorf("your collection file could not be read and was set aside as %s; starting fresh", filepath.Base(bad))
	}
	if s.data.NextID < 1 {
		s.data.NextID = 1
	}
	if s.data.Version < dataVersion {
		s.migrate()
		s.data.Version = dataVersion
		return s.persist()
	}
	return nil
}

// rollingBackups is how many automatic copies of the collection are kept.
// Three is enough to get past a bad session without noticing it at the time,
// and small enough that the folder never needs explaining.
const rollingBackups = 3

// BackupDir is where the automatic copies live, beside the collection they
// are copies of, so that the Backup zip and a copied data folder both catch
// them without being told to.
func (s *Store) BackupDir() string { return filepath.Join(s.dir, "backups") }

// SnapshotOnStartup copies the collection file aside and prunes the old
// copies. Manual backups exist and are better - they carry the photos - but
// they are a thing you have to remember, and the failure this guards against
// is the one you only notice afterwards: a bad import, a mass edit, a file
// that stopped being readable. Only the JSON is copied, because that is the
// part that is small, changes constantly, and cannot be recreated.
func (s *Store) SnapshotOnStartup(stamp string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return err // nothing saved yet, which is nothing to lose
	}
	if err := os.MkdirAll(s.BackupDir(), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(
		filepath.Join(s.BackupDir(), "collection-"+stamp+".json"), raw, 0o644); err != nil {
		return err
	}

	entries, err := os.ReadDir(s.BackupDir())
	if err != nil {
		return err
	}
	var made []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "collection-") &&
			strings.HasSuffix(e.Name(), ".json") {
			made = append(made, e.Name())
		}
	}
	// The names carry a sortable timestamp, so the oldest sort first.
	sort.Strings(made)
	for i := 0; i < len(made)-rollingBackups; i++ {
		_ = os.Remove(filepath.Join(s.BackupDir(), made[i]))
	}
	return nil
}

// migrate brings a collection saved by an older build up to dataVersion. It
// runs with the file already loaded and before anything else can touch it, so
// like persist() it does no locking of its own.
func (s *Store) migrate() {
	// Games Workshop renamed Citadel Colour to Warhammer Colour, and the
	// library is stocked under the new name. Without this an older rack would
	// list Abaddon Black twice, once per spelling.
	for i, p := range s.data.Paints {
		if to, ok := RenamedBrands[p.Brand]; ok {
			s.data.Paints[i].Brand = to
		}
	}
	// Every entry written before counts existed is one mini. A zero here
	// would otherwise erase the whole collection from the dashboard, which
	// counts minis by adding these up.
	for i, m := range s.data.Models {
		if m.Count < 1 {
			s.data.Models[i].Count = 1
		}
	}
	s.addLibraryPaints()
}

// addLibraryPaints stocks the rack from the built-in library, skipping any
// paint already there under the same brand and name so it can be re-run
// without producing duplicates. Library paints arrive unowned: the rack is a
// catalogue to tick off, not a claim that you own thirteen hundred pots.
//
// It also fills in the range and code on paints that predate those fields, so
// a migrated collection filters the same way as a fresh one.
func (s *Store) addLibraryPaints() int {
	type ref struct{ brand, name string }
	have := map[ref]int{}
	for i, p := range s.data.Paints {
		have[ref{strings.ToLower(p.Brand), strings.ToLower(p.Name)}] = i
	}
	added := 0
	for _, lp := range PaintLibrary() {
		k := ref{strings.ToLower(lp.Brand), strings.ToLower(lp.Name)}
		if i, ok := have[k]; ok {
			if s.data.Paints[i].Range == "" {
				s.data.Paints[i].Range = lp.Range
			}
			if s.data.Paints[i].Code == "" {
				s.data.Paints[i].Code = lp.Code
			}
			continue
		}
		lp.ID = s.nextID()
		s.data.Paints = append(s.data.Paints, lp)
		added++
	}
	return added
}

// persist writes atomically: full write to a temp file, fsync, then rename.
// A rename is atomic on all three platforms, so the real file is never a
// half-written mess.
func (s *Store) persist() error {
	b, err := json.MarshalIndent(&s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("could not save: %w", err)
	}
	if _, err = f.Write(b); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("could not save: %w", err)
	}
	if err = f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("could not save: %w", err)
	}
	if err = f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("could not save: %w", err)
	}
	if err = os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("could not save: %w", err)
	}
	return nil
}

func (s *Store) nextID() int {
	id := s.data.NextID
	s.data.NextID++
	return id
}

// ---------------------------------------------------------------------------
// Models
// ---------------------------------------------------------------------------

// workbenchOrder ranks statuses by how likely that mini is to be picked up
// next, which is not the pipeline order Statuses lists them in. A list sorted
// by status is a list of what to work on, so what's on the desk comes first,
// then whatever is closest to being ready for it, and what's finished sinks.
var workbenchOrder = []string{"In Progress", "Primed", "Assembled", "Backlog", "Complete", "Display"}

// workbenchRank places a status in that order. An unknown status sorts last,
// which is where a value from a newer build or a hand-edited file belongs.
func workbenchRank(status string) int {
	for i, v := range workbenchOrder {
		if v == status {
			return i
		}
	}
	return len(workbenchOrder)
}

// ModelFilter is everything the models list asks for in one go: what to keep,
// and what order to put it in. Sort names the primary key and Desc reverses
// it; anything unrecognised falls back to status, which the list opens on.
type ModelFilter struct {
	Search string `json:"search"`
	Status string `json:"status"`
	// System, Faction and Project each match exactly, and an empty one - or
	// the "All" the pickers show - matches everything.
	System  string `json:"system"`
	Faction string `json:"faction"`
	Project string `json:"project"`
	Sort    string `json:"sort"`
	Desc    bool   `json:"desc"`
}

// matches is true when a facet filter is unset or the value is exactly it.
// The pickers offer what's in use, so "All" is the only wildcard needed.
func matches(want, have string) bool {
	return want == "" || want == "All" || want == have
}

// Models returns the collection filtered and ordered for the list pane.
func (s *Store) Models(f ModelFilter) []Model {
	s.mu.RLock()
	defer s.mu.RUnlock()
	search := strings.ToLower(strings.TrimSpace(f.Search))
	sortBy, desc := f.Sort, f.Desc
	out := make([]Model, 0, len(s.data.Models))
	for _, m := range s.data.Models {
		if !matches(f.Status, m.Status) || !matches(f.System, m.GameSystem) ||
			!matches(f.Faction, m.Faction) || !matches(f.Project, m.Project) {
			continue
		}
		if search != "" {
			hay := strings.ToLower(strings.Join([]string{
				m.Name, m.GameSystem, m.Faction, m.Project, m.Notes}, " "))
			if !strings.Contains(hay, search) {
				continue
			}
		}
		// The list draws "x10" and a 6-of-10 bar straight off these, so hand
		// them over already clamped rather than making the frontend guess.
		m.Count, m.Done = m.Minis()
		out = append(out, m)
	}
	byName := func(a, b Model) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	}
	// Each ordering runs one way naturally: names climb A→Z, but counts, dates
	// and flags are only worth reading from the top down. desc flips whichever
	// of those the list is currently on.
	primary := func(a, b Model) int {
		switch sortBy {
		case "Name":
			return byName(a, b)
		case "Paints":
			return len(b.PaintIDs) - len(a.PaintIDs)
		case "Recent":
			return b.ID - a.ID
		case "Favourites":
			return btoi(b.Favorite) - btoi(a.Favorite)
		default: // Status
			return workbenchRank(a.Status) - workbenchRank(b.Status)
		}
	}
	// Ties break on name and then on id, and neither follows desc: reversing
	// the order of the column you asked for shouldn't shuffle the rows that
	// column can't tell apart.
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if c := primary(a, b); c != 0 {
			if desc {
				return c > 0
			}
			return c < 0
		}
		if c := byName(a, b); c != 0 {
			return c < 0
		}
		return a.ID < b.ID
	})
	return out
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ModelFacets reports the game systems, factions and projects in use, each
// sorted and deduplicated. Free-text fields with no separate record behind
// them, exactly like a paint's brand: the list of projects is whatever the
// collection says it is, so nothing has to be created before it can be
// typed and nothing is left behind when the last entry using it changes.
func (s *Store) ModelFacets() (systems, factions, projects []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := [3]map[string]bool{{}, {}, {}}
	out := [3][]string{{}, {}, {}}
	for _, m := range s.data.Models {
		for i, v := range []string{m.GameSystem, m.Faction, m.Project} {
			if v = strings.TrimSpace(v); v != "" && !seen[i][v] {
				seen[i][v] = true
				out[i] = append(out[i], v)
			}
		}
	}
	for i := range out {
		sort.SliceStable(out[i], func(a, b int) bool {
			return strings.ToLower(out[i][a]) < strings.ToLower(out[i][b])
		})
	}
	return out[0], out[1], out[2]
}

func (s *Store) ModelByID(id int) (Model, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, m := range s.data.Models {
		if m.ID == id {
			m.Count, m.Done = m.Minis()
			return m, true
		}
	}
	return Model{}, false
}

func (s *Store) SaveModel(m Model) (Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m.PaintIDs == nil {
		m.PaintIDs = []int{}
	}
	if m.Photos == nil {
		m.Photos = []Photo{}
	}
	if m.Sessions == nil {
		m.Sessions = []Session{}
	}
	m.Count, m.Done = m.Minis()
	// Calling an entry finished is the one moment the app can fill the rest
	// in without guessing: every mini in it is painted, and it was painted by
	// today at the latest. Minis() has already pulled Done up to Count.
	//
	// Neither is ever undone. Moving an entry back off Complete - a repaint,
	// a status fixed after a misclick - keeps the date it was finished on and
	// the count that was painted, because the alternative is an app that
	// quietly destroys a record every time a dropdown is corrected. Nothing
	// reads a finish date on an unfinished entry, so a stale one is harmless
	// where a lost one is not.
	if finished(m.Status) && m.Completed == "" {
		m.Completed = today()
	}
	if m.ID == 0 {
		m.ID = s.nextID()
		m.Created = today()
		s.data.Models = append(s.data.Models, m)
	} else {
		found := false
		for i, ex := range s.data.Models {
			if ex.ID == m.ID {
				// photos and sessions are managed by their own calls; never
				// let a stale copy from the frontend clobber them
				m.Photos = ex.Photos
				m.Sessions = ex.Sessions
				m.Created = ex.Created
				s.data.Models[i] = m
				found = true
				break
			}
		}
		if !found {
			return Model{}, fmt.Errorf("that mini no longer exists")
		}
	}
	return m, s.persist()
}

// trashDays is how long a deleted mini is kept before its photos go with it.
// Long enough to notice the mistake and open the app again; short enough that
// the data folder isn't quietly hoarding a collection you meant to be rid of.
const trashDays = 30

// DeleteModel moves a mini to the trash rather than destroying it. The photos
// are the reason: notes and a status can be typed again, and a shot of a mini
// part-painted three months ago cannot be retaken. Nothing reads the trash,
// so a deleted mini is gone from the app the moment this returns - it is just
// recoverable for a month afterwards, or immediately with Undo.
func (s *Store) DeleteModel(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, m := range s.data.Models {
		if m.ID == id {
			s.data.Trash = append(s.data.Trash, Trashed{Model: m, Deleted: today()})
			s.data.Models = append(s.data.Models[:i], s.data.Models[i+1:]...)
			return s.persist()
		}
	}
	return nil
}

// RestoreModel puts a trashed mini back. Its id comes back with it, so the
// paints, the log and the photos all still point at the same record.
func (s *Store) RestoreModel(id int) (Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, t := range s.data.Trash {
		if t.Model.ID != id {
			continue
		}
		s.data.Trash = append(s.data.Trash[:i], s.data.Trash[i+1:]...)
		s.data.Models = append(s.data.Models, t.Model)
		return t.Model, s.persist()
	}
	return Model{}, fmt.Errorf("that mini is no longer in the trash")
}

// PurgeTrash destroys what has sat in the trash past its month, photos and
// all, and reports how many went. Called on the way in rather than on a
// timer: the app is only ever open for as long as someone is using it.
func (s *Store) PurgeTrash(now time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := now.AddDate(0, 0, -trashDays).Format("2006-01-02")
	kept := make([]Trashed, 0, len(s.data.Trash))
	gone := 0
	for _, t := range s.data.Trash {
		if t.Deleted > cutoff {
			kept = append(kept, t)
			continue
		}
		for _, p := range t.Model.Photos {
			s.removePhotoFiles(p)
		}
		gone++
	}
	if gone == 0 {
		return 0
	}
	s.data.Trash = kept
	_ = s.persist()
	return gone
}

func (s *Store) AddPhoto(modelID int, srcPath, kind string) (Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	for i, m := range s.data.Models {
		if m.ID == modelID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return Model{}, fmt.Errorf("that mini no longer exists")
	}
	ext := strings.ToLower(filepath.Ext(srcPath))
	if ext == "" {
		ext = ".jpg"
	}
	name := fmt.Sprintf("m%d_%d%s", modelID, time.Now().UnixNano(), ext)
	dst := filepath.Join(s.PhotoDir(), name)
	in, err := os.ReadFile(srcPath)
	if err != nil {
		return Model{}, fmt.Errorf("could not read that image: %w", err)
	}
	if err := os.WriteFile(dst, in, 0o644); err != nil {
		return Model{}, fmt.Errorf("could not save that image: %w", err)
	}
	// A photo the decoder can't read is still a photo. It keeps its full-size
	// copy and simply has no thumbnail, which every drawing site allows for.
	thumb, _ := makeThumb(s.PhotoDir(), name)
	s.data.Models[idx].Photos = append(s.data.Models[idx].Photos, Photo{
		ID: s.nextID(), File: name, Thumb: thumb, Kind: kind, Added: today(),
	})
	return s.data.Models[idx], s.persist()
}

func (s *Store) DeletePhoto(modelID, photoID int) (Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, m := range s.data.Models {
		if m.ID != modelID {
			continue
		}
		for j, p := range m.Photos {
			if p.ID == photoID {
				s.removePhotoFiles(p)
				s.data.Models[i].Photos = append(m.Photos[:j], m.Photos[j+1:]...)
				return s.data.Models[i], s.persist()
			}
		}
		return m, nil
	}
	return Model{}, fmt.Errorf("that mini no longer exists")
}

// removePhotoFiles deletes a photo and the thumbnail made from it. A photo
// that never got one leaves nothing behind to miss.
func (s *Store) removePhotoFiles(p Photo) {
	_ = os.Remove(filepath.Join(s.PhotoDir(), p.File))
	if p.Thumb != "" {
		_ = os.Remove(filepath.Join(s.PhotoDir(), p.Thumb))
	}
}

// SetCoverPhoto marks one photo as the mini's cover. Choosing the one already
// marked clears it, which hands the choice back to CoverPhoto rather than
// leaving no way to undo a click.
func (s *Store) SetCoverPhoto(modelID, photoID int) (Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, m := range s.data.Models {
		if m.ID != modelID {
			continue
		}
		was := false
		for j := range m.Photos {
			if m.Photos[j].ID == photoID {
				was = m.Photos[j].Cover
			}
			s.data.Models[i].Photos[j].Cover = false
		}
		if !was {
			for j := range m.Photos {
				if m.Photos[j].ID == photoID {
					s.data.Models[i].Photos[j].Cover = true
				}
			}
		}
		return s.data.Models[i], s.persist()
	}
	return Model{}, fmt.Errorf("that mini no longer exists")
}

// BackfillThumbs generates the thumbnails for photos that were imported
// before thumbnails existed, and returns how many it made.
//
// The decoding happens outside the lock and the file is written once at the
// end, so a collection with hundreds of photos can be caught up in the
// background without the UI waiting on any of it. Until a photo's thumbnail
// lands, every screen draws the original, so this is invisible apart from
// the memory it eventually saves.
func (s *Store) BackfillThumbs() int {
	type job struct {
		modelID, photoID int
		file             string
	}
	s.mu.RLock()
	var jobs []job
	for _, m := range s.data.Models {
		for _, p := range m.Photos {
			if p.Thumb == "" {
				jobs = append(jobs, job{m.ID, p.ID, p.File})
			}
		}
	}
	s.mu.RUnlock()

	made := 0
	for _, j := range jobs {
		name, err := makeThumb(s.PhotoDir(), j.file)
		if err != nil {
			continue // a format the decoder doesn't know, or a missing file
		}
		s.mu.Lock()
		for i, m := range s.data.Models {
			if m.ID != j.modelID {
				continue
			}
			for k := range m.Photos {
				if m.Photos[k].ID == j.photoID {
					s.data.Models[i].Photos[k].Thumb = name
					made++
				}
			}
		}
		s.mu.Unlock()
	}
	if made > 0 {
		s.mu.Lock()
		_ = s.persist()
		s.mu.Unlock()
	}
	return made
}

// ---------------------------------------------------------------------------
// Projects
// ---------------------------------------------------------------------------

// Project is one project rolled up: its minis counted, its hours added, and
// what to pick up next. Nothing here is stored except the deadline and the
// notes - the rest is the collection, read through the name.
type Project struct {
	Name     string         `json:"name"`
	Due      string         `json:"due"`
	Notes    string         `json:"notes"`
	Entries  int            `json:"entries"`
	Minis    int            `json:"minis"`
	Done     int            `json:"done"`
	Minutes  int            `json:"minutes"`
	Sessions int            `json:"sessions"`
	ByStatus map[string]int `json:"byStatus"`
	// Next is what to pick up, in the order the models list would put it:
	// what's on the desk first, then whatever is closest to ready for it.
	Next []MiniRef `json:"next"`
	// DaysLeft is meaningful only when Due is set. Negative is overdue, and
	// it is counted here so that every screen agrees on what "today" means.
	DaysLeft int `json:"daysLeft"`
}

// Projects rolls up every project in use, soonest deadline first.
func (s *Store) Projects(now time.Time) []Project {
	s.mu.RLock()
	defer s.mu.RUnlock()

	meta := map[string]ProjectMeta{}
	byName := map[string]*Project{}
	order := []string{}
	take := func(name string) *Project {
		if p, ok := byName[name]; ok {
			return p
		}
		p := &Project{Name: name, ByStatus: map[string]int{}, Next: []MiniRef{}}
		byName[name] = p
		order = append(order, name)
		return p
	}
	// A project keeps its card once it has a deadline, even with nothing
	// filed under it yet - that is a plan, and losing it would be surprising.
	for _, m := range s.data.Projects {
		meta[m.Name] = m
		take(m.Name)
	}

	for _, m := range s.data.Models {
		name := strings.TrimSpace(m.Project)
		if name == "" {
			continue
		}
		p := take(name)
		total, done := m.Minis()
		p.Entries++
		p.Minis += total
		p.Done += done
		p.ByStatus[m.Status] += total
		for _, e := range m.Sessions {
			p.Sessions++
			p.Minutes += e.Minutes
		}
		if !finished(m.Status) {
			p.Next = append(p.Next, MiniRef{ID: m.ID, Name: m.Name,
				Status: m.Status, Count: total, Done: done})
		}
	}

	out := make([]Project, 0, len(order))
	todayStr := now.Format("2006-01-02")
	for _, name := range order {
		p := byName[name]
		if md, ok := meta[name]; ok {
			p.Due, p.Notes = md.Due, md.Notes
			if md.Due != "" {
				p.DaysLeft = daysBetween(todayStr, md.Due)
			}
		}
		sort.SliceStable(p.Next, func(i, j int) bool {
			ri, rj := workbenchRank(p.Next[i].Status), workbenchRank(p.Next[j].Status)
			if ri != rj {
				return ri < rj
			}
			return strings.ToLower(p.Next[i].Name) < strings.ToLower(p.Next[j].Name)
		})
		if len(p.Next) > 4 {
			p.Next = p.Next[:4]
		}
		out = append(out, *p)
	}
	// A deadline is the whole reason to write one down, so dated projects
	// come first in the order they fall due; the rest follow by name.
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.Due == "") != (b.Due == "") {
			return a.Due != ""
		}
		if a.Due != b.Due {
			return a.Due < b.Due
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return out
}

// daysBetween counts whole days from one YYYY-MM-DD to another, and returns 0
// for anything it can't parse rather than a wild number.
func daysBetween(from, to string) int {
	a, err1 := time.Parse("2006-01-02", from)
	b, err2 := time.Parse("2006-01-02", to)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(b.Sub(a).Hours() / 24)
}

// SaveProject records a deadline and notes against a project name. With
// neither, the row is dropped: the project still exists for as long as minis
// name it, and an empty record would only be clutter in the file.
func (s *Store) SaveProject(p ProjectMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return fmt.Errorf("a project needs a name")
	}
	empty := strings.TrimSpace(p.Due) == "" && strings.TrimSpace(p.Notes) == ""
	for i, ex := range s.data.Projects {
		if ex.Name == p.Name {
			if empty {
				s.data.Projects = append(s.data.Projects[:i], s.data.Projects[i+1:]...)
			} else {
				s.data.Projects[i] = p
			}
			return s.persist()
		}
	}
	if empty {
		return nil
	}
	s.data.Projects = append(s.data.Projects, p)
	return s.persist()
}

// RenameProject re-tags every mini filed under a project, and carries the
// deadline and notes across with them. This is the answer to the one real
// weakness of grouping on free text: without it, fixing a typo in a project
// name means editing every mini that carries it.
//
// Renaming to nothing ungroups them, which is how a project is dissolved
// without touching the minis themselves.
func (s *Store) RenameProject(from, to string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if from == "" {
		return 0, fmt.Errorf("which project?")
	}
	moved := 0
	for i, m := range s.data.Models {
		if strings.TrimSpace(m.Project) == from {
			s.data.Models[i].Project = to
			moved++
		}
	}
	kept := s.data.Projects[:0]
	for _, md := range s.data.Projects {
		switch {
		case md.Name != from:
			kept = append(kept, md)
		case to != "":
			md.Name = to
			kept = append(kept, md)
		}
	}
	s.data.Projects = kept
	return moved, s.persist()
}

// ---------------------------------------------------------------------------
// Time
// ---------------------------------------------------------------------------

// TimeBucket is one month of the log.
type TimeBucket struct {
	Key      string `json:"key"`   // 2026-08, for matching
	Label    string `json:"label"` // Aug, for the axis
	Year     int    `json:"year"`
	Minutes  int    `json:"minutes"`
	Sessions int    `json:"sessions"`
}

// MiniTime is what one entry cost. PerMini divides by the size of the batch,
// which is the only reason counts are worth carrying: three hours on a squad
// of ten is eighteen minutes a mini, and that is the number that tells you
// whether the next squad is an evening or a fortnight.
type MiniTime struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Minutes int    `json:"minutes"`
	Count   int    `json:"count"`
	PerMini int    `json:"perMini"`
}

// TimeReport is the painting log added up. Everything here is derived from
// the sessions; nothing new is recorded to produce it.
type TimeReport struct {
	Months    []TimeBucket `json:"months"` // the last twelve, oldest first
	Total     int          `json:"total"`
	ThisMonth int          `json:"thisMonth"`
	Last30    int          `json:"last30"`
	Sessions  int          `json:"sessions"`
	// Average session length, and the average time a finished mini took.
	PerSession int        `json:"perSession"`
	PerMini    int        `json:"perMini"`
	Busiest    []MiniTime `json:"busiest"`
	// Days is how many separate days were spent at the desk, which is a
	// truer measure of a habit than the number of sessions.
	Days int `json:"days"`
}

// TimeReport adds up the log as of the given day. The day is passed in rather
// than read from the clock so the buckets are testable, and so a report
// generated at 23:59 and read at 00:01 doesn't disagree with itself.
func (s *Store) TimeReport(now time.Time) TimeReport {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Twelve buckets ending on the current month, so the chart always spans
	// a year even where the year has holes in it.
	rep := TimeReport{Months: []TimeBucket{}, Busiest: []MiniTime{}}
	index := map[string]int{}
	first := now.AddDate(0, -11, 0)
	for i := 0; i < 12; i++ {
		m := first.AddDate(0, i, 0)
		index[m.Format("2006-01")] = i
		rep.Months = append(rep.Months, TimeBucket{
			Key: m.Format("2006-01"), Label: m.Format("Jan"), Year: m.Year(),
		})
	}
	thisMonth := now.Format("2006-01")
	cutoff := now.AddDate(0, 0, -30).Format("2006-01-02")
	days := map[string]bool{}

	var timedMinutes, timedMinis int
	for _, m := range s.data.Models {
		mins := 0
		for _, e := range m.Sessions {
			rep.Sessions++
			rep.Total += e.Minutes
			mins += e.Minutes
			if e.Date != "" {
				days[e.Date] = true
			}
			if len(e.Date) >= 7 {
				if i, ok := index[e.Date[:7]]; ok {
					rep.Months[i].Minutes += e.Minutes
					rep.Months[i].Sessions++
				}
				if e.Date[:7] == thisMonth {
					rep.ThisMonth += e.Minutes
				}
			}
			if e.Date >= cutoff {
				rep.Last30 += e.Minutes
			}
		}
		if mins == 0 {
			continue
		}
		total, _ := m.Minis()
		timedMinutes += mins
		timedMinis += total
		rep.Busiest = append(rep.Busiest, MiniTime{
			ID: m.ID, Name: m.Name, Minutes: mins, Count: total,
			PerMini: mins / total,
		})
	}

	rep.Days = len(days)
	if rep.Sessions > 0 {
		rep.PerSession = rep.Total / rep.Sessions
	}
	// Averaged over the minis that have time against them, not the whole
	// collection: a shelf of untouched boxes would otherwise drag the figure
	// towards zero and make every estimate from it useless.
	if timedMinis > 0 {
		rep.PerMini = timedMinutes / timedMinis
	}
	sort.SliceStable(rep.Busiest, func(i, j int) bool {
		if rep.Busiest[i].Minutes != rep.Busiest[j].Minutes {
			return rep.Busiest[i].Minutes > rep.Busiest[j].Minutes
		}
		return strings.ToLower(rep.Busiest[i].Name) < strings.ToLower(rep.Busiest[j].Name)
	})
	if len(rep.Busiest) > 8 {
		rep.Busiest = rep.Busiest[:8]
	}
	return rep
}

// ---------------------------------------------------------------------------
// Painting sessions
// ---------------------------------------------------------------------------

// SaveSession adds a new log entry, or updates one in place if it has an ID.
// Entries are kept newest-first.
func (s *Store) SaveSession(modelID int, sess Session) (Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, m := range s.data.Models {
		if m.ID != modelID {
			continue
		}
		if sess.Date == "" {
			sess.Date = today()
		}
		if sess.Minutes < 0 {
			sess.Minutes = 0
		}
		if sess.ID == 0 {
			sess.ID = s.nextID()
			s.data.Models[i].Sessions = append(s.data.Models[i].Sessions, sess)
		} else {
			found := false
			for j, ex := range m.Sessions {
				if ex.ID == sess.ID {
					s.data.Models[i].Sessions[j] = sess
					found = true
					break
				}
			}
			if !found {
				return Model{}, fmt.Errorf("that log entry no longer exists")
			}
		}
		sort.SliceStable(s.data.Models[i].Sessions, func(a, b int) bool {
			sa, sb := s.data.Models[i].Sessions[a], s.data.Models[i].Sessions[b]
			if sa.Date != sb.Date {
				return sa.Date > sb.Date
			}
			return sa.ID > sb.ID
		})
		// The oldest session doubles as the start date - you can't have been
		// painting a mini before you started it. That fills the date in when
		// nothing was set, and pulls it back when an entry is logged or
		// re-dated earlier than the date on record, which is what happens
		// when you remember a session after the fact.
		//
		// It only ever moves earlier. Logging a session today says nothing
		// about when the mini was started, and deleting the oldest entry
		// isn't evidence the start date was wrong either, so neither pushes
		// it forward.
		//
		// The sort above leaves the list newest-first, so the oldest entry is
		// the last one. Dates are ISO, so they compare as strings.
		if n := len(s.data.Models[i].Sessions); n > 0 {
			oldest := s.data.Models[i].Sessions[n-1].Date
			if s.data.Models[i].Started == "" || oldest < s.data.Models[i].Started {
				s.data.Models[i].Started = oldest
			}
		}
		return s.data.Models[i], s.persist()
	}
	return Model{}, fmt.Errorf("that mini no longer exists")
}

func (s *Store) DeleteSession(modelID, sessionID int) (Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, m := range s.data.Models {
		if m.ID != modelID {
			continue
		}
		for j, e := range m.Sessions {
			if e.ID == sessionID {
				s.data.Models[i].Sessions = append(m.Sessions[:j], m.Sessions[j+1:]...)
				return s.data.Models[i], s.persist()
			}
		}
		return m, nil
	}
	return Model{}, fmt.Errorf("that mini no longer exists")
}

// ---------------------------------------------------------------------------
// Paints
// ---------------------------------------------------------------------------

// Paints filters the rack. stock is the stock-status filter and covers both
// what's on the shelf and what's on the shopping list, since the screen offers
// them from one dropdown.
func (s *Store) Paints(search, ptype, brand, rng, stock string) []Paint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	search = strings.ToLower(strings.TrimSpace(search))
	out := make([]Paint, 0, len(s.data.Paints))
	for _, p := range s.data.Paints {
		if ptype != "" && ptype != "All types" && p.Type != ptype {
			continue
		}
		if brand != "" && brand != "All brands" && p.Brand != brand {
			continue
		}
		if rng != "" && rng != "All ranges" && p.Range != rng {
			continue
		}
		if stock == "Owned only" && !p.Owned {
			continue
		}
		if stock == "Not owned" && p.Owned {
			continue
		}
		if stock == "On wishlist" && !p.Wishlist {
			continue
		}
		// the code is worth searching: it's how AK and Ionic pots are labelled
		if search != "" && !strings.Contains(
			strings.ToLower(p.Name+" "+p.Brand+" "+p.Range+" "+p.Code+" "+p.Notes), search) {
			continue
		}
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !strings.EqualFold(out[i].Brand, out[j].Brand) {
			return strings.ToLower(out[i].Brand) < strings.ToLower(out[j].Brand)
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// PaintCounts returns how many paints are in the rack, how many are owned, and
// how many are on the wishlist.
func (s *Store) PaintCounts() (total, owned, wishlist int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.data.Paints {
		if p.Owned {
			owned++
		}
		if p.Wishlist {
			wishlist++
		}
	}
	return len(s.data.Paints), owned, wishlist
}

// Ranges lists the product lines on offer, narrowed to one brand when given.
func (s *Store) Ranges(brand string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]bool{}
	out := []string{}
	for _, p := range s.data.Paints {
		if brand != "" && brand != "All brands" && p.Brand != brand {
			continue
		}
		if p.Range != "" && !seen[p.Range] {
			seen[p.Range] = true
			out = append(out, p.Range)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Store) AllPaints() []Paint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// A copy, so sorting can't reorder the stored slice. Built with make
	// rather than appending to a nil slice: an empty rack would otherwise
	// return nil, which marshals to JSON null instead of [], and the paint
	// screens read .length off the result.
	out := append(make([]Paint, 0, len(s.data.Paints)), s.data.Paints...)
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func (s *Store) Brands() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]bool{}
	out := []string{}
	for _, p := range s.data.Paints {
		if p.Brand != "" && !seen[p.Brand] {
			seen[p.Brand] = true
			out = append(out, p.Brand)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Store) SavePaint(p Paint) (Paint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ID == 0 {
		p.ID = s.nextID()
		s.data.Paints = append(s.data.Paints, p)
	} else {
		found := false
		for i, ex := range s.data.Paints {
			if ex.ID == p.ID {
				s.data.Paints[i] = p
				found = true
				break
			}
		}
		if !found {
			return Paint{}, fmt.Errorf("that paint no longer exists")
		}
	}
	return p, s.persist()
}

func (s *Store) DeletePaint(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.data.Paints {
		if p.ID == id {
			s.data.Paints = append(s.data.Paints[:i], s.data.Paints[i+1:]...)
			break
		}
	}
	// also unlink it from every mini, so nothing dangles
	for i := range s.data.Models {
		kept := s.data.Models[i].PaintIDs[:0]
		for _, pid := range s.data.Models[i].PaintIDs {
			if pid != id {
				kept = append(kept, pid)
			}
		}
		s.data.Models[i].PaintIDs = kept
	}
	return s.persist()
}

func (s *Store) PaintUsage(id int) int {
	n := 0
	for _, m := range s.data.Models {
		for _, pid := range m.PaintIDs {
			if pid == id {
				n++
				break
			}
		}
	}
	return n
}

// UsesPaint reports whether a list of paint ids contains one.
func usesPaint(ids []int, id int) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// ModelsUsingPaint lists the minis a paint is recorded on, newest first.
// "Used on 3 minis" is a dead number on its own - which three is the question
// actually being asked, and the answer is already in the collection.
func (s *Store) ModelsUsingPaint(id int) []MiniRef {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []MiniRef{}
	for _, m := range s.data.Models {
		if !usesPaint(m.PaintIDs, id) {
			continue
		}
		total, done := m.Minis()
		out = append(out, MiniRef{ID: m.ID, Name: m.Name, Status: m.Status,
			Count: total, Done: done})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// TipsUsingPaint lists the recipes that call for a paint.
func (s *Store) TipsUsingPaint(id int) []TipRef {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []TipRef{}
	for _, t := range s.data.Tips {
		if usesPaint(t.PaintIDs, id) {
			out = append(out, TipRef{ID: t.ID, Title: t.Title, Category: t.Category})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// SetPaintFlags ticks owned or wishlist without sending the whole paint back
// and forth. The shopping list works one checkbox at a time, and a round trip
// through the edit dialog for each would make short work of a long list into
// long work of a short one.
func (s *Store) SetPaintFlags(id int, owned, wishlist bool) (Paint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.data.Paints {
		if p.ID == id {
			s.data.Paints[i].Owned = owned
			s.data.Paints[i].Wishlist = wishlist
			return s.data.Paints[i], s.persist()
		}
	}
	return Paint{}, fmt.Errorf("that paint no longer exists")
}

// WishlistPaints returns the rack's shopping list and, separately, the paints
// the collection says are in use but that aren't owned.
//
// That second list is the one thing only this app can work out: a paint
// recorded on a mini and not in the rack is one you used at a club, borrowed,
// or ran dry, and it belongs on the list before you notice it missing at the
// desk. Anything already on the list is left out of it.
func (s *Store) WishlistPaints() (listed, missing []Paint) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	used := map[int]bool{}
	for _, m := range s.data.Models {
		for _, id := range m.PaintIDs {
			used[id] = true
		}
	}
	listed, missing = []Paint{}, []Paint{}
	for _, p := range s.data.Paints {
		switch {
		case p.Wishlist:
			listed = append(listed, p)
		case used[p.ID] && !p.Owned:
			missing = append(missing, p)
		}
	}
	byRack := func(a, b Paint) bool {
		if !strings.EqualFold(a.Brand, b.Brand) {
			return strings.ToLower(a.Brand) < strings.ToLower(b.Brand)
		}
		if !strings.EqualFold(a.Range, b.Range) {
			return strings.ToLower(a.Range) < strings.ToLower(b.Range)
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	}
	sort.SliceStable(listed, func(i, j int) bool { return byRack(listed[i], listed[j]) })
	sort.SliceStable(missing, func(i, j int) bool { return byRack(missing[i], missing[j]) })
	return listed, missing
}

// RestoreLibraryPaints puts back any built-in paints that have been deleted.
// Nothing already in the rack is touched.
func (s *Store) RestoreLibraryPaints() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	added := s.addLibraryPaints()
	if added == 0 {
		return 0, nil
	}
	return added, s.persist()
}

// ---------------------------------------------------------------------------
// Tips
// ---------------------------------------------------------------------------

func (s *Store) TipList(search, category string) []Tip {
	s.mu.RLock()
	defer s.mu.RUnlock()
	search = strings.ToLower(strings.TrimSpace(search))
	out := make([]Tip, 0, len(s.data.Tips))
	for _, t := range s.data.Tips {
		if category != "" && category != "All" && t.Category != category {
			continue
		}
		if search != "" {
			hay := strings.ToLower(t.Title + " " + t.Body + " " + strings.Join(t.Tags, " "))
			if !strings.Contains(hay, search) {
				continue
			}
		}
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

func (s *Store) TipByID(id int) (Tip, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.data.Tips {
		if t.ID == id {
			return t, true
		}
	}
	return Tip{}, false
}

func (s *Store) SaveTip(t Tip) (Tip, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.Tags == nil {
		t.Tags = []string{}
	}
	if t.PaintIDs == nil {
		t.PaintIDs = []int{}
	}
	if t.ID == 0 {
		t.ID = s.nextID()
		t.Created = today()
		s.data.Tips = append(s.data.Tips, t)
	} else {
		found := false
		for i, ex := range s.data.Tips {
			if ex.ID == t.ID {
				t.Created = ex.Created
				s.data.Tips[i] = t
				found = true
				break
			}
		}
		if !found {
			return Tip{}, fmt.Errorf("that note no longer exists")
		}
	}
	return t, s.persist()
}

func (s *Store) DeleteTip(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, t := range s.data.Tips {
		if t.ID == id {
			s.data.Tips = append(s.data.Tips[:i], s.data.Tips[i+1:]...)
			return s.persist()
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Stats
// ---------------------------------------------------------------------------

// LogEntry is a session with its mini's name attached, for the dashboard.
type LogEntry struct {
	ModelID   int    `json:"modelId"`
	ModelName string `json:"modelName"`
	Date      string `json:"date"`
	Minutes   int    `json:"minutes"`
	Notes     string `json:"notes"`
}

type Stats struct {
	Models     int            `json:"models"`
	PaintsOwn  int            `json:"paintsOwned"`
	PaintsWish int            `json:"paintsWishlist"`
	Tips       int            `json:"tips"`
	InProg     int            `json:"inProgress"`
	Finished   int            `json:"finished"`
	Sessions   int            `json:"sessions"`
	Minutes    int            `json:"minutes"`
	ByStatus   map[string]int `json:"byStatus"`
	Backlog    []Model        `json:"backlog"`
	Recent     []Model        `json:"recent"`
	RecentLogs []LogEntry     `json:"recentLogs"`
}

func (s *Store) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := Stats{ByStatus: map[string]int{}, Backlog: []Model{}, Recent: []Model{},
		RecentLogs: []LogEntry{}}
	for _, v := range Statuses {
		st.ByStatus[v] = 0
	}
	for _, m := range s.data.Models {
		// Counted in minis rather than in entries: a squad of ten is ten
		// minis on the shelf, and saying "1" for it made every number on this
		// screen an undercount of the collection it claims to describe.
		//
		// A part-painted batch is split between the two bars it actually
		// straddles - six finished marines under Complete, four still under
		// In Progress - so the chart and the finished card can never disagree
		// about the same six marines. A finished entry is never split; it
		// counts whole under its own status, so Display keeps its bar.
		total, done := m.Minis()
		st.Models += total
		st.Finished += done
		if finished(m.Status) {
			st.ByStatus[m.Status] += total
		} else {
			st.ByStatus["Complete"] += done
			st.ByStatus[m.Status] += total - done
			if m.Status == "In Progress" {
				st.InProg += total - done
			}
		}
		for _, e := range m.Sessions {
			st.Sessions++
			st.Minutes += e.Minutes
			st.RecentLogs = append(st.RecentLogs, LogEntry{
				ModelID: m.ID, ModelName: m.Name, Date: e.Date,
				Minutes: e.Minutes, Notes: e.Notes,
			})
		}
	}
	sort.SliceStable(st.RecentLogs, func(i, j int) bool {
		return st.RecentLogs[i].Date > st.RecentLogs[j].Date
	})
	if len(st.RecentLogs) > 6 {
		st.RecentLogs = st.RecentLogs[:6]
	}
	for _, p := range s.data.Paints {
		if p.Owned {
			st.PaintsOwn++
		}
		if p.Wishlist {
			st.PaintsWish++
		}
	}
	st.Tips = len(s.data.Tips)

	rank := map[string]int{"Backlog": 0, "Assembled": 1, "Primed": 2}
	for _, m := range s.data.Models {
		if _, ok := rank[m.Status]; ok {
			st.Backlog = append(st.Backlog, m)
		}
		if m.Status == "Complete" || m.Status == "Display" {
			st.Recent = append(st.Recent, m)
		}
	}
	sort.SliceStable(st.Backlog, func(i, j int) bool {
		if rank[st.Backlog[i].Status] != rank[st.Backlog[j].Status] {
			return rank[st.Backlog[i].Status] < rank[st.Backlog[j].Status]
		}
		return strings.ToLower(st.Backlog[i].Name) < strings.ToLower(st.Backlog[j].Name)
	})
	sort.SliceStable(st.Recent, func(i, j int) bool {
		if st.Recent[i].Completed != st.Recent[j].Completed {
			return st.Recent[i].Completed > st.Recent[j].Completed
		}
		return st.Recent[i].ID > st.Recent[j].ID
	})
	if len(st.Backlog) > 8 {
		st.Backlog = st.Backlog[:8]
	}
	if len(st.Recent) > 8 {
		st.Recent = st.Recent[:8]
	}
	return st
}
