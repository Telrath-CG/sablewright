package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// AllPaints crosses into the frontend as JSON, where renderPaints and
// modelDialog both read .length off the result without guarding. A nil slice
// marshals to `null` rather than `[]`, so returning nil for an empty rack
// throws in both and locks a fresh install out of Paint Inventory and Add
// Mini at once — with no way to add the first paint that would fix it.
func TestAllPaintsEmptyMarshalsToArrayNotNull(t *testing.T) {
	s := &Store{}

	got := s.AllPaints()
	if got == nil {
		t.Fatal("AllPaints() = nil; marshals to JSON null and breaks the paint screens")
	}
	if len(got) != 0 {
		t.Fatalf("AllPaints() on an empty store returned %d paints, want 0", len(got))
	}

	b, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshalling AllPaints(): %v", err)
	}
	if string(b) != "[]" {
		t.Errorf("AllPaints() marshalled to %s, want []", b)
	}
}

// The returned slice is a copy specifically so sorting it can't reorder the
// stored one. That property is easy to lose while changing how it's built.
func TestAllPaintsDoesNotReorderStoredPaints(t *testing.T) {
	s := &Store{data: Data{Paints: []Paint{
		{ID: 1, Name: "Zandri Dust"},
		{ID: 2, Name: "Abaddon Black"},
	}}}

	_ = s.AllPaints()

	if s.data.Paints[0].Name != "Zandri Dust" {
		t.Errorf("AllPaints() reordered the stored slice: first paint is now %q",
			s.data.Paints[0].Name)
	}
}

// A session written up after the fact can predate the start date on record -
// an evening at the desk in June, logged in August. The detail pane shows
// Started directly under the log, so the two sat there contradicting each
// other.
func TestSaveSessionPullsStartedBackToAnEarlierEntry(t *testing.T) {
	s := newTempStore(t)
	s.data = Data{NextID: 100, Models: []Model{{
		ID: 1, Name: "Marine", Started: "2026-07-01",
		Sessions: []Session{{ID: 10, Date: "2026-07-02", Notes: "first"}},
	}}}

	m, err := s.SaveSession(1, Session{Date: "2026-06-15", Notes: "forgotten evening"})
	if err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if m.Started != "2026-06-15" {
		t.Errorf("Started = %q after logging a session dated 2026-06-15, want that date",
			m.Started)
	}
}

// Re-dating an existing entry has to move it too - the same mistake, caught
// on the edit path rather than the add path.
func TestSaveSessionPullsStartedBackWhenAnEntryIsRedated(t *testing.T) {
	s := newTempStore(t)
	s.data = Data{NextID: 100, Models: []Model{{
		ID: 1, Name: "Marine", Started: "2026-07-01",
		Sessions: []Session{{ID: 10, Date: "2026-07-02", Notes: "first"}},
	}}}

	m, err := s.SaveSession(1, Session{ID: 10, Date: "2026-05-20", Notes: "first"})
	if err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if m.Started != "2026-05-20" {
		t.Errorf("Started = %q after re-dating the only entry to 2026-05-20, want that date",
			m.Started)
	}
}

// It's a one-way ratchet. Painting a mini today says nothing about when it was
// started, so a later session must leave the recorded date alone.
func TestSaveSessionLeavesAnEarlierStartedAlone(t *testing.T) {
	s := newTempStore(t)
	s.data = Data{NextID: 100, Models: []Model{{
		ID: 1, Name: "Marine", Started: "2026-07-01",
		Sessions: []Session{{ID: 10, Date: "2026-07-02", Notes: "first"}},
	}}}

	m, err := s.SaveSession(1, Session{Date: "2026-08-20", Notes: "another evening"})
	if err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if m.Started != "2026-07-01" {
		t.Errorf("Started = %q after a later session, want it left at 2026-07-01", m.Started)
	}
}

// The original behaviour this grew out of: with no start date at all, the
// first session logged supplies one.
func TestSaveSessionFillsInAMissingStarted(t *testing.T) {
	s := newTempStore(t)
	s.data = Data{NextID: 100, Models: []Model{{ID: 1, Name: "Marine"}}}

	m, err := s.SaveSession(1, Session{Date: "2026-06-15", Notes: "first sitting"})
	if err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if m.Started != "2026-06-15" {
		t.Errorf("Started = %q on a mini with no start date, want the session's date",
			m.Started)
	}
}

// Counts arrived in version 5. Every entry written before that is one mini,
// and a zero would erase the whole collection from a dashboard that counts
// minis by adding them up.
func TestOlderCollectionsGetACountOfOne(t *testing.T) {
	s := newTempStore(t)
	writeCollection(t, s, 4, nil, []Model{{ID: 1, Name: "Marine", Status: "Complete"}})

	if err := s.load(); err != nil {
		t.Fatalf("load(): %v", err)
	}

	if got := s.data.Models[0].Count; got != 1 {
		t.Errorf("migrated entry has Count %d, want 1", got)
	}
}

// Calling an entry finished is the one moment the app can fill in the rest
// without guessing, and it saves the two bits of bookkeeping most likely to
// be skipped - which is why "Recently finished" was sorting on a blank date.
func TestSaveModelStampsTheFinishWhenAnEntryIsCompleted(t *testing.T) {
	s := newTempStore(t)
	s.data = Data{NextID: 100}

	m, err := s.SaveModel(Model{Name: "Tactical Squad", Status: "Complete", Count: 10, Done: 6})
	if err != nil {
		t.Fatalf("SaveModel(): %v", err)
	}

	if m.Done != 10 {
		t.Errorf("Done = %d on a completed entry of 10, want all of them", m.Done)
	}
	if m.Completed != today() {
		t.Errorf("Completed = %q, want today (%q)", m.Completed, today())
	}
}

// The stamp only ever runs one way. Correcting a status by hand, or starting
// a repaint, must not destroy the record of when the thing was finished.
func TestSaveModelKeepsTheFinishWhenAnEntryGoesBackToTheDesk(t *testing.T) {
	s := newTempStore(t)
	s.data = Data{NextID: 100, Models: []Model{{
		ID: 1, Name: "Marine", Status: "Complete", Count: 1, Done: 1,
		Completed: "2026-06-03",
	}}}

	m, err := s.SaveModel(Model{ID: 1, Name: "Marine", Status: "In Progress", Count: 1, Done: 1,
		Completed: "2026-06-03"})
	if err != nil {
		t.Fatalf("SaveModel(): %v", err)
	}

	if m.Completed != "2026-06-03" {
		t.Errorf("Completed = %q after going back to In Progress, want it kept", m.Completed)
	}
}

// A collection file can be hand-edited, and an entry claiming twelve of ten
// painted would draw a progress bar past its own end.
func TestMinisClampsWhatTheFileClaims(t *testing.T) {
	total, done := Model{Status: "In Progress", Count: 0, Done: 12}.Minis()
	if total != 1 || done != 1 {
		t.Errorf("Minis() on a countless entry = %d of %d, want 1 of 1", done, total)
	}
	if _, done := (Model{Status: "Display", Count: 4}.Minis()); done != 4 {
		t.Errorf("Minis() on a finished entry = %d painted, want all 4", done)
	}
}

// The dashboard describes the collection, so it has to count the collection:
// a squad of ten is ten minis on the shelf, and a part-painted one belongs to
// both bars it straddles rather than to whichever reads better.
func TestStatsCountsMinisRatherThanEntries(t *testing.T) {
	s := &Store{data: Data{Models: []Model{
		{ID: 1, Name: "Squad", Status: "In Progress", Count: 10, Done: 6},
		{ID: 2, Name: "Hero", Status: "Complete", Count: 1},
		{ID: 3, Name: "Boxed", Status: "Backlog", Count: 5},
		{ID: 4, Name: "Shelf", Status: "Display", Count: 2},
	}}}

	got := s.Stats()

	for _, c := range []struct {
		what string
		got  int
		want int
	}{
		{"minis tracked", got.Models, 18},
		{"finished", got.Finished, 9},
		{"in progress", got.InProg, 4},
		{"Complete bar", got.ByStatus["Complete"], 7},
		{"In Progress bar", got.ByStatus["In Progress"], 4},
		{"Backlog bar", got.ByStatus["Backlog"], 5},
		{"Display bar", got.ByStatus["Display"], 2},
	} {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.what, c.got, c.want)
		}
	}
}

// The list draws one shot per mini, and which one it is has to be worth
// looking at: the finished article beats a shot of bare primer, and an
// explicit choice beats both.
func TestCoverPhotoPicksTheShotWorthShowing(t *testing.T) {
	progress := Photo{ID: 1, File: "a.jpg", Kind: "Progress"}
	final := Photo{ID: 2, File: "b.jpg", Kind: "Final"}
	chosen := Photo{ID: 3, File: "c.jpg", Kind: "Progress", Cover: true}

	for _, c := range []struct {
		what   string
		photos []Photo
		want   int
	}{
		{"nothing at all", nil, 0},
		{"only progress shots", []Photo{progress}, 1},
		{"a final among them", []Photo{progress, final}, 2},
		{"an explicit choice", []Photo{progress, final, chosen}, 3},
	} {
		got := Model{Photos: c.photos}.CoverPhoto()
		if c.want == 0 {
			if got != nil {
				t.Errorf("CoverPhoto() with %s = %v, want nil", c.what, got)
			}
			continue
		}
		if got == nil || got.ID != c.want {
			t.Errorf("CoverPhoto() with %s = %v, want photo %d", c.what, got, c.want)
		}
	}
}

// Deleting a photo has to take its thumbnail with it. A generated file with
// nothing left pointing at it is invisible, so it would accumulate in the
// data folder forever.
func TestDeletePhotoRemovesTheThumbnailToo(t *testing.T) {
	s := newTempStore(t)
	if err := os.MkdirAll(s.PhotoDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"m1_1.jpg", "t_m1_1.jpg"} {
		if err := os.WriteFile(filepath.Join(s.PhotoDir(), n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s.data = Data{NextID: 100, Models: []Model{{ID: 1, Name: "Marine", Photos: []Photo{
		{ID: 5, File: "m1_1.jpg", Thumb: "t_m1_1.jpg", Kind: "Progress"},
	}}}}

	if _, err := s.DeletePhoto(1, 5); err != nil {
		t.Fatalf("DeletePhoto(): %v", err)
	}

	if _, err := os.Stat(filepath.Join(s.PhotoDir(), "t_m1_1.jpg")); !os.IsNotExist(err) {
		t.Error("the thumbnail outlived the photo it was made from")
	}
}

// Clicking the star on the photo already chosen is the only way back to
// letting the app pick, so it has to clear rather than re-set.
func TestSetCoverPhotoTogglesTheChoiceOff(t *testing.T) {
	s := newTempStore(t)
	s.data = Data{NextID: 100, Models: []Model{{ID: 1, Name: "Marine", Photos: []Photo{
		{ID: 5, File: "a.jpg", Kind: "Progress"},
		{ID: 6, File: "b.jpg", Kind: "Final"},
	}}}}

	if _, err := s.SetCoverPhoto(1, 5); err != nil {
		t.Fatalf("SetCoverPhoto(): %v", err)
	}
	if got := s.data.Models[0].CoverPhoto(); got == nil || got.ID != 5 {
		t.Fatalf("cover = %v after choosing photo 5", got)
	}

	m, err := s.SetCoverPhoto(1, 5)
	if err != nil {
		t.Fatalf("SetCoverPhoto(): %v", err)
	}
	for _, p := range m.Photos {
		if p.Cover {
			t.Errorf("photo %d is still marked after choosing it twice", p.ID)
		}
	}
	if got := m.CoverPhoto(); got == nil || got.ID != 6 {
		t.Errorf("cover fell back to %v, want the final shot", got)
	}
}

// Game system and faction were recorded on every mini and usable only as
// words in the search box, which matches a mini whose notes happen to mention
// the army it's fighting. Filtering on them is exact.
func TestModelsFilterOnTheFacetsExactly(t *testing.T) {
	s := &Store{data: Data{Models: []Model{
		{ID: 1, Name: "Plague Marines", GameSystem: "40k", Faction: "Death Guard",
			Project: "Tournament"},
		{ID: 2, Name: "Blightlord", GameSystem: "40k", Faction: "Death Guard"},
		{ID: 3, Name: "Skink", GameSystem: "AoS", Faction: "Seraphon",
			Project: "Tournament"},
		{ID: 4, Name: "Notes mention Death Guard", GameSystem: "AoS", Faction: "Nurgle"},
	}}}

	for _, c := range []struct {
		what string
		f    ModelFilter
		want string
	}{
		{"a faction", ModelFilter{Faction: "Death Guard", Sort: "Name"},
			"Blightlord, Plague Marines"},
		{"a system", ModelFilter{System: "AoS", Sort: "Name"},
			"Notes mention Death Guard, Skink"},
		{"a project", ModelFilter{Project: "Tournament", Sort: "Name"},
			"Plague Marines, Skink"},
		{"two at once", ModelFilter{System: "40k", Project: "Tournament", Sort: "Name"},
			"Plague Marines"},
		{"All everywhere", ModelFilter{System: "All", Faction: "All", Project: "All",
			Sort: "Name"},
			"Blightlord, Notes mention Death Guard, Plague Marines, Skink"},
	} {
		if got := modelNames(t, s.Models(c.f)); got != c.want {
			t.Errorf("filtering by %s = %s, want %s", c.what, got, c.want)
		}
	}
}

// The pickers offer what the collection actually contains, so they can never
// point at a value that matches nothing. Blanks are not a choice.
func TestModelFacetsListWhatIsInUse(t *testing.T) {
	s := &Store{data: Data{Models: []Model{
		{ID: 1, GameSystem: "40k", Faction: "Death Guard", Project: "Tournament"},
		{ID: 2, GameSystem: "40k", Faction: "  ", Project: ""},
		{ID: 3, GameSystem: "AoS", Faction: "Seraphon"},
	}}}

	systems, factions, projects := s.ModelFacets()

	if strings.Join(systems, ", ") != "40k, AoS" {
		t.Errorf("systems = %v, want the two in use, deduplicated", systems)
	}
	if strings.Join(factions, ", ") != "Death Guard, Seraphon" {
		t.Errorf("factions = %v, want the blank one left out", factions)
	}
	if strings.Join(projects, ", ") != "Tournament" {
		t.Errorf("projects = %v, want just the one", projects)
	}
}

func modelNames(t *testing.T, ms []Model) string {
	t.Helper()
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Name
	}
	return strings.Join(out, ", ")
}

// The list opens on this ordering and the first row is the one selected, so
// it decides what the detail pane shows on arrival. Ordering by the pipeline
// put a finished mini above one still on the desk, which is backwards: the
// half-painted one is the one about to be edited.
func TestModelsSortByStatusLeadsWithWhatIsOnTheDesk(t *testing.T) {
	s := &Store{data: Data{Models: []Model{
		{ID: 1, Name: "Finished Marine", Status: "Complete"},
		{ID: 2, Name: "Boxed Necron", Status: "Backlog"},
		{ID: 3, Name: "Half-Painted Ork", Status: "In Progress"},
	}}}

	got := modelNames(t, s.Models(ModelFilter{Sort: "Status"}))
	want := "Half-Painted Ork, Boxed Necron, Finished Marine"
	if got != want {
		t.Errorf("Models() sorted by status = %s, want %s", got, want)
	}
}

// Clicking a column header a second time reverses that column. The rows it
// can't tell apart aren't part of what was asked for, so they stay put -
// otherwise every reversal reshuffles the names inside each status too.
func TestModelsDescReversesTheColumnButNotTheTieBreak(t *testing.T) {
	s := &Store{data: Data{Models: []Model{
		{ID: 1, Name: "Zogg", Status: "Complete"},
		{ID: 2, Name: "Mordred", Status: "In Progress"},
		{ID: 3, Name: "Anvil", Status: "Complete"},
	}}}

	got := modelNames(t, s.Models(ModelFilter{Sort: "Status", Desc: true}))
	want := "Anvil, Zogg, Mordred"
	if got != want {
		t.Errorf("Models() sorted by status, reversed = %s, want %s", got, want)
	}
}

// A count is worth reading from the top down, so the paints column starts on
// most-first and only reads the other way once reversed.
func TestModelsSortByPaintsStartsWithTheMostPainted(t *testing.T) {
	s := &Store{data: Data{Models: []Model{
		{ID: 1, Name: "One", Status: "Backlog", PaintIDs: []int{1}},
		{ID: 2, Name: "Three", Status: "Complete", PaintIDs: []int{1, 2, 3}},
		{ID: 3, Name: "None", Status: "In Progress"},
	}}}

	got := modelNames(t, s.Models(ModelFilter{Sort: "Paints"}))
	if want := "Three, One, None"; got != want {
		t.Errorf("Models() sorted by paints = %s, want %s", got, want)
	}
	got = modelNames(t, s.Models(ModelFilter{Sort: "Paints", Desc: true}))
	if want := "None, One, Three"; got != want {
		t.Errorf("Models() sorted by paints, reversed = %s, want %s", got, want)
	}
}

func TestAllPaintsSortsByNameIgnoringCase(t *testing.T) {
	s := &Store{data: Data{Paints: []Paint{
		{ID: 1, Name: "zandri dust"},
		{ID: 2, Name: "Abaddon Black"},
		{ID: 3, Name: "Mephiston Red"},
	}}}

	got := s.AllPaints()
	want := []string{"Abaddon Black", "Mephiston Red", "zandri dust"}
	for i, w := range want {
		if got[i].Name != w {
			t.Errorf("paint %d = %q, want %q", i, got[i].Name, w)
		}
	}
}
