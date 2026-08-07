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
