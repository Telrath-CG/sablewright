package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The library is embedded data, so nothing at runtime validates it. If a
// regenerated file lands malformed, these are what catch it.
func TestPaintLibraryIsWellFormed(t *testing.T) {
	lib := PaintLibrary()
	if len(lib) < 1000 {
		t.Fatalf("library has %d paints; expected the full set of ranges", len(lib))
	}

	hex := regexp.MustCompile(`^#[0-9a-f]{6}$`)
	type ref struct{ brand, name, rng string }
	seen := map[ref]bool{}
	for _, p := range lib {
		switch {
		case p.Name == "":
			t.Errorf("paint with no name: %+v", p)
		case p.Brand == "":
			t.Errorf("%q has no brand", p.Name)
		case !hex.MatchString(p.Hex):
			t.Errorf("%s %q has hex %q, want lowercase #rrggbb", p.Brand, p.Name, p.Hex)
		case p.Type == "":
			t.Errorf("%s %q has no type", p.Brand, p.Name)
		case p.ID != 0:
			t.Errorf("%s %q ships with ID %d; IDs are assigned on seeding", p.Brand, p.Name, p.ID)
		case p.Owned:
			t.Errorf("%s %q ships as owned; the rack is a catalogue to tick off",
				p.Brand, p.Name)
		}
		// Citadel really does sell one Dawnstone as a Layer and another as a
		// Dry, so the range is part of a paint's identity - but a repeat of all
		// three fields means the same pot got listed twice.
		k := ref{p.Brand, p.Name, p.Range}
		if seen[k] {
			t.Errorf("duplicate entry: %s %q (%s)", p.Brand, p.Name, p.Range)
		}
		seen[k] = true
	}
}

// Citadel Air paints mostly share a name with a Base or Layer paint, and are
// only told apart by the suffix. Drop it and the rack shows two identical rows.
func TestCitadelAirPaintsAreSuffixed(t *testing.T) {
	air, core := map[string]bool{}, map[string]bool{}
	for _, p := range PaintLibrary() {
		if p.Brand != "Warhammer Colour" {
			continue
		}
		if p.Range == "Air" {
			air[p.Name] = true
		} else {
			core[p.Name] = true
		}
	}
	if len(air) == 0 {
		t.Fatal("the Citadel Air range is missing from the library")
	}
	for name := range air {
		if core[name] {
			t.Errorf("%q exists in both the Air range and a paint range; the Air "+
				"entry needs its suffix to be tellable apart", name)
		}
	}
	// the paint the convention was chosen for
	if !air["The Fang - Air"] || !core["The Fang"] {
		t.Error(`expected both "The Fang" and "The Fang - Air" in the library`)
	}
	// ...but the thinner already says Air, and isn't a color
	if air["Air Caste Thinner - Air"] {
		t.Error(`"Air Caste Thinner" was given a redundant Air suffix`)
	}
}

// What each brand has to clear. These guard against a regenerated file quietly
// dropping most of a range, so they sit under what actually ships - a maker
// adding a few pots shouldn't fail the build.
//
// Kimera is why this is a table rather than the flat 100 it used to be. Even
// with all nine of its ranges it lands around 70, so a three-figure minimum
// would have meant the brand could never be listed at all.
var brandFloor = map[string]int{
	"Warhammer Colour":   350,
	"AK Interactive":     450,
	"Ionic Smart Colors": 170,
	"Two Thin Coats":     170,
	"Pro Acryl":          120,
	"Kimera Kolors":      65,
}

func TestPaintLibraryCoversEveryBrand(t *testing.T) {
	count := map[string]int{}
	for _, p := range PaintLibrary() {
		count[p.Brand]++
	}

	listed := map[string]bool{}
	for _, b := range LibraryBrands {
		listed[b] = true
		floor, ok := brandFloor[b]
		if !ok {
			t.Errorf("%s is in LibraryBrands with no floor in brandFloor; give it one "+
				"so a range going missing is still caught", b)
			continue
		}
		if count[b] < floor {
			t.Errorf("%s has %d paints in the library, want at least %d", b, count[b], floor)
		}
	}

	// A brand in the file but not in LibraryBrands clears every check above by
	// never being looked at, which is how a whole range could rot unnoticed.
	for b := range count {
		if !listed[b] {
			t.Errorf("%s is in the library but missing from LibraryBrands", b)
		}
	}
}

// newTempStore builds a store over an empty temp folder, the way NewStore does
// but without touching the real data directory.
func newTempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	return &Store{dir: dir, path: filepath.Join(dir, "collection.json")}
}

func TestFreshInstallIsStockedWithTheLibrary(t *testing.T) {
	s := newTempStore(t)
	if err := s.load(); err != nil {
		t.Fatalf("load() on a fresh install: %v", err)
	}

	if got, want := len(s.data.Paints), len(PaintLibrary()); got != want {
		t.Errorf("fresh install has %d paints, want the whole library (%d)", got, want)
	}
	if s.data.Version != dataVersion {
		t.Errorf("fresh install saved version %d, want %d", s.data.Version, dataVersion)
	}
	ids := map[int]bool{}
	for _, p := range s.data.Paints {
		if p.ID == 0 {
			t.Fatalf("%q was seeded without an ID", p.Name)
		}
		if ids[p.ID] {
			t.Fatalf("id %d handed out twice", p.ID)
		}
		ids[p.ID] = true
	}

	// and it has to survive the round trip to disk
	reopened := &Store{dir: s.dir, path: s.path}
	if err := reopened.load(); err != nil {
		t.Fatalf("reopening: %v", err)
	}
	if len(reopened.data.Paints) != len(s.data.Paints) {
		t.Errorf("reopened with %d paints, saved %d",
			len(reopened.data.Paints), len(s.data.Paints))
	}
}

// writeCollection lays down a collection file saved by an older build.
func writeCollection(t *testing.T, s *Store, version int, paints []Paint, models []Model) {
	t.Helper()
	b, err := json.Marshal(Data{Version: version, NextID: 100, Paints: paints, Models: models})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeV1 lays down a collection file in the pre-library format.
func writeV1(t *testing.T, s *Store, paints []Paint, models []Model) {
	t.Helper()
	writeCollection(t, s, 1, paints, models)
}

// Bumping dataVersion is how a range added to the library reaches collections
// that already exist - there's no separate top-up step to forget.
func TestVersionBumpTopsUpTheRack(t *testing.T) {
	var stocked []Paint
	id := 1
	for _, p := range PaintLibrary() {
		if p.Range == "Air" && p.Brand == "Warhammer Colour" {
			continue // the range this collection predates
		}
		p.ID = id
		id++
		stocked = append(stocked, p)
	}

	s := newTempStore(t)
	writeCollection(t, s, dataVersion-1, stocked, nil)
	if err := s.load(); err != nil {
		t.Fatalf("load(): %v", err)
	}

	if len(s.data.Paints) != len(PaintLibrary()) {
		t.Errorf("after the bump the rack holds %d paints, want the full library (%d)",
			len(s.data.Paints), len(PaintLibrary()))
	}
	if s.data.Version != dataVersion {
		t.Errorf("version is still %d after migrating", s.data.Version)
	}
	air := 0
	for _, p := range s.data.Paints {
		if p.Brand == "Warhammer Colour" && p.Range == "Air" {
			air++
		}
	}
	if air == 0 {
		t.Error("the Air range never arrived")
	}
}

// An existing collection has to gain the library without losing anything it
// already had - least of all a paint ID, which minis reference by number.
func TestMigrationKeepsExistingPaintsAndTheirIDs(t *testing.T) {
	s := newTempStore(t)
	writeV1(t, s,
		[]Paint{
			{ID: 7, Name: "Abaddon Black", Brand: "Citadel", Type: "Base",
				Hex: "#231f20", Owned: true, Notes: "nearly out"},
			{ID: 8, Name: "Mixed Skin Tone", Brand: "Homebrew", Type: "Layer",
				Hex: "#c08a70", Owned: true},
		},
		[]Model{{ID: 9, Name: "Test Marine", PaintIDs: []int{7, 8}}})

	if err := s.load(); err != nil {
		t.Fatalf("load(): %v", err)
	}

	var black, home *Paint
	for i, p := range s.data.Paints {
		switch p.ID {
		case 7:
			black = &s.data.Paints[i]
		case 8:
			home = &s.data.Paints[i]
		}
	}
	if black == nil || home == nil {
		t.Fatal("migration dropped one of the user's own paints")
	}
	if !black.Owned || black.Notes != "nearly out" {
		t.Errorf("migration clobbered the user's edits: %+v", *black)
	}
	if home.Brand != "Homebrew" {
		t.Errorf("a brand outside the library was rewritten to %q", home.Brand)
	}
	if s.data.Models[0].PaintIDs[0] != 7 {
		t.Error("migration disturbed a mini's paint links")
	}
	// nothing may be handed an ID the user's paints already hold
	for _, p := range s.data.Paints {
		if p.ID == 7 && p.Name != "Abaddon Black" {
			t.Errorf("id 7 was reused by %q", p.Name)
		}
	}
}

// The old starter set filed Citadel paints under "Citadel"; the library uses
// the range's current name. Without the rename the rack lists both.
func TestMigrationRenamesCitadelAndDoesNotDuplicateIt(t *testing.T) {
	s := newTempStore(t)
	writeV1(t, s, []Paint{
		{ID: 7, Name: "Abaddon Black", Brand: "Citadel", Type: "Base", Hex: "#231f20", Owned: true},
	}, nil)

	if err := s.load(); err != nil {
		t.Fatalf("load(): %v", err)
	}

	n := 0
	for _, p := range s.data.Paints {
		if strings.EqualFold(p.Name, "Abaddon Black") {
			n++
			if p.Brand != "Warhammer Colour" {
				t.Errorf("Abaddon Black is filed under %q, want Warhammer Colour", p.Brand)
			}
		}
	}
	if n != 1 {
		t.Errorf("found %d copies of Abaddon Black after migrating, want 1", n)
	}
	// the range should have been filled in from the library
	for _, p := range s.data.Paints {
		if p.ID == 7 && p.Range != "Base" {
			t.Errorf("migrated paint has range %q, want it backfilled to Base", p.Range)
		}
	}
}

// Restoring the library is offered as a repair, so running it when nothing is
// missing must be a no-op rather than a second copy of everything.
func TestRestoreLibraryPaintsIsIdempotent(t *testing.T) {
	s := newTempStore(t)
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	before := len(s.data.Paints)

	added, err := s.RestoreLibraryPaints()
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 || len(s.data.Paints) != before {
		t.Errorf("restoring a complete rack added %d paints (%d -> %d), want none",
			added, before, len(s.data.Paints))
	}

	// delete one, and it should come back - just that one
	s.data.Paints = s.data.Paints[1:]
	added, err = s.RestoreLibraryPaints()
	if err != nil {
		t.Fatal(err)
	}
	if added != 1 || len(s.data.Paints) != before {
		t.Errorf("restoring after one deletion added %d, ending at %d; want 1 and %d",
			added, len(s.data.Paints), before)
	}
}

func TestPaintsFilterByRange(t *testing.T) {
	s := newTempStore(t)
	if err := s.load(); err != nil {
		t.Fatal(err)
	}

	got := s.Paints("", "", "Warhammer Colour", "Contrast", "")
	if len(got) == 0 {
		t.Fatal("no Warhammer Colour Contrast paints came back")
	}
	for _, p := range got {
		if p.Range != "Contrast" || p.Brand != "Warhammer Colour" {
			t.Fatalf("range filter let through %s %q (%s)", p.Brand, p.Name, p.Range)
		}
	}

	if all := s.Paints("", "", "", "", ""); len(all) <= len(got) {
		t.Errorf("unfiltered list (%d) should be larger than one range (%d)", len(all), len(got))
	}
	// a code is how an AK pot is labelled, so search has to find one
	if hits := s.Paints("AK11001", "", "", "", ""); len(hits) != 1 {
		t.Errorf("searching a product code returned %d paints, want 1", len(hits))
	}
}

func TestRangesFollowTheBrand(t *testing.T) {
	s := newTempStore(t)
	if err := s.load(); err != nil {
		t.Fatal(err)
	}

	ttc := s.Ranges("Two Thin Coats")
	want := []string{"Wave 1", "Wave 2", "Wave 3"}
	if len(ttc) != len(want) {
		t.Fatalf("Two Thin Coats ranges = %v, want %v", ttc, want)
	}
	for i, w := range want {
		if ttc[i] != w {
			t.Errorf("range %d = %q, want %q", i, ttc[i], w)
		}
	}
	if len(s.Ranges("")) <= len(ttc) {
		t.Error("the unfiltered range list should span every brand")
	}
}
