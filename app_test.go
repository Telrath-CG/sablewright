package main

import "testing"

// The rack holds the whole built-in library, so the inventory screen asks for a
// capped page. If the cap stopped being applied the app would still look right
// while shipping the entire catalogue across the bridge on every keystroke.
func TestListPaintsCapsTheRowsButReportsTheRealTotal(t *testing.T) {
	s := newTempStore(t)
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	a := &App{store: s}

	page := a.ListPaints(PaintFilter{Limit: 25})
	if len(page.Rows) != 25 {
		t.Errorf("asked for 25 rows, got %d", len(page.Rows))
	}
	if page.Total != len(PaintLibrary()) {
		t.Errorf("Total = %d, want the full match count %d", page.Total, len(PaintLibrary()))
	}

	if all := a.ListPaints(PaintFilter{}); len(all.Rows) != all.Total {
		t.Errorf("no limit returned %d of %d rows, want all of them", len(all.Rows), all.Total)
	}
}

// Rows is read with .map on the frontend, so it has to marshal as [] and not
// null when nothing matches.
func TestListPaintsReturnsAnEmptyPageNotNull(t *testing.T) {
	a := &App{store: &Store{}}
	page := a.ListPaints(PaintFilter{Search: "no such paint"})
	if page.Rows == nil {
		t.Error("Rows is nil; it marshals to null and breaks the paint table")
	}
	if page.Total != 0 {
		t.Errorf("Total = %d on an empty store, want 0", page.Total)
	}
}

func TestFacetsDropsABrandThatNoLongerExists(t *testing.T) {
	s := newTempStore(t)
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	a := &App{store: s}

	f := a.Facets("Two Thin Coats")
	if f.Brand != "Two Thin Coats" || len(f.Ranges) == 0 {
		t.Fatalf("Facets(%q) = brand %q, ranges %v", "Two Thin Coats", f.Brand, f.Ranges)
	}
	if f.Total != len(PaintLibrary()) || f.Owned != 0 {
		t.Errorf("counts = %d total / %d owned, want %d and 0",
			f.Total, f.Owned, len(PaintLibrary()))
	}

	// a brand the user has deleted every paint of must not stay selected, or
	// the screen filters on something that can never match
	gone := a.Facets("Vallejo")
	if gone.Brand != "" {
		t.Errorf("Facets kept unknown brand %q", gone.Brand)
	}
	if len(gone.Ranges) != len(a.Facets("").Ranges) {
		t.Error("a dropped brand should fall back to every range, not none")
	}
}
