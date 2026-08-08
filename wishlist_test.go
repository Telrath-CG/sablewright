package main

import (
	"strings"
	"testing"
)

// Only macOS draws the buttons a dialog asks for. Windows and Linux answer with
// their own word for yes, so a confirmation matched on the caller's label alone
// reads as a refusal there and the delete silently never happens.
func TestIsAffirmativeAcceptsEveryPlatformsYes(t *testing.T) {
	yes := []string{"Delete", "delete", "Yes", "yes", "Ok", "OK"}
	for _, c := range yes {
		if !isAffirmative(c, "Delete") {
			t.Errorf("isAffirmative(%q, \"Delete\") = false, want true", c)
		}
	}

	no := []string{"No", "no", "Cancel", "cancel", "", "   ", "Error"}
	for _, c := range no {
		if isAffirmative(c, "Delete") {
			t.Errorf("isAffirmative(%q, \"Delete\") = true, want false", c)
		}
	}
}

// An empty answer is what a dialog that failed to open reports, and it must
// never be read as permission to destroy something.
func TestIsAffirmativeRejectsAnEmptyAnswerEvenWithAnEmptyLabel(t *testing.T) {
	if isAffirmative("", "") {
		t.Error(`isAffirmative("", "") = true; a dialog that never answered must not confirm`)
	}
}

// The wishlist is a shopping list, not the opposite of owned: a pot you have
// but have nearly used up belongs on it too.
// The shopping list's second half is the query only this app can run: a paint
// recorded on a mini with no pot in the rack. Anything already flagged stays
// out of it, or the suggestion list would just repeat the list above it.
func TestWishlistSuggestsPaintsUsedButNotOwned(t *testing.T) {
	s := &Store{data: Data{
		Paints: []Paint{
			{ID: 1, Name: "Borrowed Blue", Brand: "B", Owned: false},
			{ID: 2, Name: "Owned Ochre", Brand: "A", Owned: true},
			{ID: 3, Name: "Wanted White", Brand: "A", Owned: false, Wishlist: true},
			{ID: 4, Name: "Untouched Umber", Brand: "A", Owned: false},
			{ID: 5, Name: "Run Dry Red", Brand: "A", Owned: false, Wishlist: true},
		},
		Models: []Model{{ID: 10, Name: "Marine", PaintIDs: []int{1, 2, 5}}},
	}}

	listed, missing := s.WishlistPaints()

	// Both are brand A with no range, so they fall to name order.
	if names := paintNames(listed); names != "Run Dry Red, Wanted White" {
		t.Errorf("listed = %s, want the two flagged, in rack order", names)
	}
	// 2 is owned, 4 was never used, and 5 is already on the list above.
	if names := paintNames(missing); names != "Borrowed Blue" {
		t.Errorf("missing = %s, want only the used-but-unowned paint", names)
	}
}

func paintNames(ps []Paint) string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	return strings.Join(out, ", ")
}

// Ticking a pot off the shopping list is two changes at once - it is owned
// now, and no longer wanted - and the list would otherwise keep showing it.
func TestSetPaintFlagsTicksOffWithoutTheEditDialog(t *testing.T) {
	s := newTempStore(t)
	s.data = Data{NextID: 100, Paints: []Paint{
		{ID: 1, Name: "Wanted White", Owned: false, Wishlist: true},
	}}

	p, err := s.SetPaintFlags(1, true, false)
	if err != nil {
		t.Fatalf("SetPaintFlags(): %v", err)
	}
	if !p.Owned || p.Wishlist {
		t.Errorf("paint is owned=%v wanted=%v, want owned and no longer wanted",
			p.Owned, p.Wishlist)
	}
}

// A paint is a way into the records that mention it, which is the question
// "used on 3 minis" was raising and refusing to answer.
func TestPaintLinksFindTheMinisAndRecipesThatUseIt(t *testing.T) {
	s := &Store{data: Data{
		Models: []Model{
			{ID: 1, Name: "Squad", Status: "In Progress", Count: 10, Done: 6,
				PaintIDs: []int{7, 8}},
			{ID: 2, Name: "Hero", Status: "Complete", Count: 1, PaintIDs: []int{9}},
		},
		Tips: []Tip{
			{ID: 3, Title: "Rust", Category: "Effects", PaintIDs: []int{7}},
			{ID: 4, Title: "Cloaks", Category: "Blending", PaintIDs: []int{9}},
		},
	}}
	a := &App{store: s}

	got := a.PaintLinks(7)

	if len(got.Minis) != 1 || got.Minis[0].Name != "Squad" {
		t.Fatalf("minis = %v, want just the squad", got.Minis)
	}
	if got.Minis[0].Count != 10 || got.Minis[0].Done != 6 {
		t.Errorf("mini ref = %d of %d, want the batch it belongs to",
			got.Minis[0].Done, got.Minis[0].Count)
	}
	if len(got.Tips) != 1 || got.Tips[0].Title != "Rust" {
		t.Errorf("tips = %v, want just the rust recipe", got.Tips)
	}
	if links := a.PaintLinks(99); len(links.Minis) != 0 || len(links.Tips) != 0 {
		t.Errorf("an unused paint reported %v", links)
	}
}

func TestWishlistIsIndependentOfOwned(t *testing.T) {
	s := newTempStore(t)
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	a := &App{store: s}

	restock, err := a.SavePaint(Paint{Name: "Nearly Out Red", Brand: "Zzz Test",
		Type: "Base", Hex: "#ff0000", Owned: true, Wishlist: true})
	if err != nil {
		t.Fatal(err)
	}
	toBuy, err := a.SavePaint(Paint{Name: "Never Had It Blue", Brand: "Zzz Test",
		Type: "Base", Hex: "#0000ff", Owned: false, Wishlist: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SavePaint(Paint{Name: "Plenty Left Green", Brand: "Zzz Test",
		Type: "Base", Hex: "#00ff00", Owned: true}); err != nil {
		t.Fatal(err)
	}

	page := a.ListPaints(PaintFilter{Brand: "Zzz Test", Stock: "On wishlist"})
	if page.Total != 2 {
		t.Fatalf("wishlist filter matched %d paints, want the 2 flagged", page.Total)
	}
	got := map[int]bool{}
	for _, r := range page.Rows {
		got[r.ID] = true
	}
	if !got[restock.ID] || !got[toBuy.ID] {
		t.Errorf("wishlist = %v, want both the owned-but-low and the never-owned paint", got)
	}

	// the owned filters must not have picked up the wishlist flag
	if owned := a.ListPaints(PaintFilter{Brand: "Zzz Test", Stock: "Owned only"}); owned.Total != 2 {
		t.Errorf("owned-only matched %d, want 2", owned.Total)
	}
	if not := a.ListPaints(PaintFilter{Brand: "Zzz Test", Stock: "Not owned"}); not.Total != 1 {
		t.Errorf("not-owned matched %d, want 1", not.Total)
	}
}

// The counts behind the inventory subtitle and the dashboard tile.
func TestPaintCountsReportsTheWishlist(t *testing.T) {
	s := newTempStore(t)
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	a := &App{store: s}

	if _, _, wish := s.PaintCounts(); wish != 0 {
		t.Fatalf("a fresh rack has %d wishlisted, want 0", wish)
	}

	if _, err := a.SavePaint(Paint{Name: "Wanted", Brand: "Zzz Test",
		Type: "Base", Hex: "#123456", Wishlist: true}); err != nil {
		t.Fatal(err)
	}

	if _, _, wish := s.PaintCounts(); wish != 1 {
		t.Errorf("PaintCounts wishlist = %d, want 1", wish)
	}
	if f := a.Facets(""); f.Wishlist != 1 {
		t.Errorf("Facets.Wishlist = %d, want 1", f.Wishlist)
	}
	if st := s.Stats(); st.PaintsWish != 1 {
		t.Errorf("Stats.PaintsWish = %d, want 1", st.PaintsWish)
	}
}

// A wishlist flag is the user's own data, so putting the built-in ranges back
// must not wipe it off a paint that shares a name with a library one.
func TestRestoringTheLibraryKeepsAWishlistFlag(t *testing.T) {
	s := newTempStore(t)
	if err := s.load(); err != nil {
		t.Fatal(err)
	}

	lib := PaintLibrary()[0]
	target := -1
	for i, p := range s.data.Paints {
		if p.Brand == lib.Brand && p.Name == lib.Name {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatalf("could not find %q %q in the stocked rack", lib.Brand, lib.Name)
	}
	s.data.Paints[target].Wishlist = true
	id := s.data.Paints[target].ID

	if _, err := s.RestoreLibraryPaints(); err != nil {
		t.Fatal(err)
	}

	for _, p := range s.data.Paints {
		if p.ID == id {
			if !p.Wishlist {
				t.Error("restoring the library cleared the wishlist flag")
			}
			return
		}
	}
	t.Error("the wishlisted paint vanished when the library was restored")
}
