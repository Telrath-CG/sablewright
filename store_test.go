package main

import (
	"encoding/json"
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
