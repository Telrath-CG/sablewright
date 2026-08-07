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
	ID    int    `json:"id"`
	File  string `json:"file"`  // filename inside the photos folder
	Kind  string `json:"kind"`  // "Progress" or "Final"
	Added string `json:"added"` // YYYY-MM-DD
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

type Model struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	GameSystem string    `json:"gameSystem"`
	Faction    string    `json:"faction"`
	Status     string    `json:"status"`
	Favorite   bool      `json:"favorite"`
	Notes      string    `json:"notes"`
	Started    string    `json:"started"`
	Completed  string    `json:"completed"`
	Created    string    `json:"created"`
	PaintIDs   []int     `json:"paintIds"`
	Photos     []Photo   `json:"photos"`
	Sessions   []Session `json:"sessions"`
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
	Type  string `json:"type"`
	Hex   string `json:"hex"`
	Owned bool   `json:"owned"`
	Notes string `json:"notes"`
}

type Tip struct {
	ID       int      `json:"id"`
	Title    string   `json:"title"`
	Category string   `json:"category"`
	Body     string   `json:"body"`
	Tags     []string `json:"tags"`
	Created  string   `json:"created"`
}

type Data struct {
	Version int     `json:"version"`
	NextID  int     `json:"nextId"`
	Models  []Model `json:"models"`
	Paints  []Paint `json:"paints"`
	Tips    []Tip   `json:"tips"`
}

// Store owns the data file and guards it with a mutex.
type Store struct {
	mu   sync.RWMutex
	dir  string
	path string
	data Data
}

var Statuses = []string{"Backlog", "Assembled", "Primed", "In Progress", "Complete", "Display"}

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
		s.data = Data{Version: 1, NextID: 1}
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
		s.data = Data{Version: 1, NextID: 1}
		_ = s.persist()
		return fmt.Errorf("your collection file could not be read and was set aside as %s; starting fresh", filepath.Base(bad))
	}
	if s.data.NextID < 1 {
		s.data.NextID = 1
	}
	return nil
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

func (s *Store) Models(search, status, sortBy string) []Model {
	s.mu.RLock()
	defer s.mu.RUnlock()
	search = strings.ToLower(strings.TrimSpace(search))
	out := make([]Model, 0, len(s.data.Models))
	for _, m := range s.data.Models {
		if status != "" && status != "All" && m.Status != status {
			continue
		}
		if search != "" {
			hay := strings.ToLower(m.Name + " " + m.GameSystem + " " + m.Faction + " " + m.Notes)
			if !strings.Contains(hay, search) {
				continue
			}
		}
		out = append(out, m)
	}
	statusRank := func(st string) int {
		for i, v := range Statuses {
			if v == st {
				return i
			}
		}
		return len(Statuses)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch sortBy {
		case "Name":
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		case "Status":
			if statusRank(a.Status) != statusRank(b.Status) {
				return statusRank(a.Status) < statusRank(b.Status)
			}
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		case "Favourites":
			if a.Favorite != b.Favorite {
				return a.Favorite
			}
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		default: // Recent
			return a.ID > b.ID
		}
	})
	return out
}

func (s *Store) ModelByID(id int) (Model, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, m := range s.data.Models {
		if m.ID == id {
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

func (s *Store) DeleteModel(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, m := range s.data.Models {
		if m.ID == id {
			for _, p := range m.Photos {
				_ = os.Remove(filepath.Join(s.PhotoDir(), p.File))
			}
			s.data.Models = append(s.data.Models[:i], s.data.Models[i+1:]...)
			return s.persist()
		}
	}
	return nil
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
	s.data.Models[idx].Photos = append(s.data.Models[idx].Photos, Photo{
		ID: s.nextID(), File: name, Kind: kind, Added: today(),
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
				_ = os.Remove(filepath.Join(s.PhotoDir(), p.File))
				s.data.Models[i].Photos = append(m.Photos[:j], m.Photos[j+1:]...)
				return s.data.Models[i], s.persist()
			}
		}
		return m, nil
	}
	return Model{}, fmt.Errorf("that mini no longer exists")
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
		// first logged session doubles as the start date if none was set
		if s.data.Models[i].Started == "" {
			oldest := sess.Date
			for _, e := range s.data.Models[i].Sessions {
				if e.Date < oldest {
					oldest = e.Date
				}
			}
			s.data.Models[i].Started = oldest
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

func (s *Store) Paints(search, ptype, brand, owned string) []Paint {
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
		if owned == "Owned only" && !p.Owned {
			continue
		}
		if owned == "Wishlist only" && p.Owned {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(p.Name+" "+p.Brand+" "+p.Notes), search) {
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

func (s *Store) AddStarterPaints() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing := map[string]bool{}
	for _, p := range s.data.Paints {
		existing[strings.ToLower(p.Name)] = true
	}
	added := 0
	for _, sp := range StarterPaints {
		if existing[strings.ToLower(sp.Name)] {
			continue
		}
		sp.ID = s.nextID()
		sp.Owned = true
		s.data.Paints = append(s.data.Paints, sp)
		added++
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

func (s *Store) SaveTip(t Tip) (Tip, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.Tags == nil {
		t.Tags = []string{}
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
			return Tip{}, fmt.Errorf("that tip no longer exists")
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
		st.Models++
		st.ByStatus[m.Status]++
		switch m.Status {
		case "In Progress":
			st.InProg++
		case "Complete", "Display":
			st.Finished++
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
