package main

import (
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The report is asked for a day rather than reading the clock, so the twelve
// buckets it produces are the same every run.
func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestTimeReportBucketsTheLastTwelveMonths(t *testing.T) {
	s := &Store{data: Data{Models: []Model{{
		ID: 1, Name: "Marine", Count: 1, Sessions: []Session{
			{ID: 1, Date: "2026-08-02", Minutes: 90, Notes: "this month"},
			{ID: 2, Date: "2026-07-30", Minutes: 60, Notes: "last month"},
			{ID: 3, Date: "2025-09-15", Minutes: 45, Notes: "on the far edge"},
			{ID: 4, Date: "2025-06-01", Minutes: 300, Notes: "off the end"},
		},
	}}}}

	r := s.TimeReport(day("2026-08-08"))

	if len(r.Months) != 12 {
		t.Fatalf("report has %d months, want 12", len(r.Months))
	}
	if first, last := r.Months[0].Key, r.Months[11].Key; first != "2025-09" || last != "2026-08" {
		t.Errorf("months run %s..%s, want 2025-09..2026-08", first, last)
	}
	if got := r.Months[11].Minutes; got != 90 {
		t.Errorf("this month = %d minutes, want 90", got)
	}
	if got := r.Months[0].Minutes; got != 45 {
		t.Errorf("the oldest bucket = %d minutes, want the 45 that fall in it", got)
	}
	// Everything is counted in the totals, including what falls off the chart.
	if r.Total != 495 {
		t.Errorf("total = %d, want every session including the one off the chart", r.Total)
	}
	if r.ThisMonth != 90 {
		t.Errorf("this month = %d, want 90", r.ThisMonth)
	}
}

// The busiest list is where the batch counts pay off: three hours across a
// squad of ten is eighteen minutes a mini, and that is the number that says
// whether the next squad is an evening or a fortnight.
func TestTimeReportDividesBatchTimeByTheBatch(t *testing.T) {
	s := &Store{data: Data{Models: []Model{
		{ID: 1, Name: "Squad", Status: "In Progress", Count: 10,
			Sessions: []Session{{ID: 1, Date: "2026-08-01", Minutes: 180, Notes: "x"}}},
		{ID: 2, Name: "Hero", Status: "Complete", Count: 1,
			Sessions: []Session{{ID: 2, Date: "2026-08-01", Minutes: 120, Notes: "y"}}},
		{ID: 3, Name: "Untouched", Status: "Backlog", Count: 40},
	}}}

	r := s.TimeReport(day("2026-08-08"))

	if len(r.Busiest) != 2 {
		t.Fatalf("busiest lists %d entries, want the two with time on them", len(r.Busiest))
	}
	if r.Busiest[0].Name != "Squad" || r.Busiest[0].PerMini != 18 {
		t.Errorf("busiest[0] = %s at %d a mini, want Squad at 18",
			r.Busiest[0].Name, r.Busiest[0].PerMini)
	}
	// 300 minutes across the 11 minis that have time against them. The forty
	// untouched ones in the backlog are not part of the question.
	if r.PerMini != 27 {
		t.Errorf("average a mini = %d, want 27 (300 over 11)", r.PerMini)
	}
	if r.PerSession != 150 {
		t.Errorf("average session = %d, want 150", r.PerSession)
	}
}

// Two sessions on one evening are one evening at the desk. Counting days
// rather than entries is what makes the figure mean anything as a habit.
func TestTimeReportCountsDaysNotEntries(t *testing.T) {
	s := &Store{data: Data{Models: []Model{{
		ID: 1, Name: "Marine", Count: 1, Sessions: []Session{
			{ID: 1, Date: "2026-08-02", Minutes: 30, Notes: "before dinner"},
			{ID: 2, Date: "2026-08-02", Minutes: 45, Notes: "after dinner"},
			{ID: 3, Date: "2026-08-03", Minutes: 20, Notes: "next day"},
		},
	}}}}

	if got := s.TimeReport(day("2026-08-08")).Days; got != 2 {
		t.Errorf("days at the desk = %d, want 2", got)
	}
}

func TestExportWritesASelfContainedPage(t *testing.T) {
	s := newTempStore(t)
	s.data = Data{NextID: 100,
		Paints: []Paint{{ID: 7, Name: "Nuln Oil", Brand: "Warhammer Colour", Hex: "#101418"}},
		Models: []Model{{
			ID: 1, Name: "Plague Marines", GameSystem: "40k", Faction: "Death Guard",
			Status: "In Progress", Count: 10, Done: 6, PaintIDs: []int{7},
			Notes:    "Rust first, <script>alert(1)</script> then grime",
			Sessions: []Session{{ID: 2, Date: "2026-08-01", Minutes: 95, Notes: "Shading"}},
		}}}
	a := &App{store: s}
	path := filepath.Join(t.TempDir(), "out.html")

	m, _ := s.ModelByID(1)
	if err := a.exportHTML(m, a.paintNames(m.PaintIDs), path); err != nil {
		t.Fatalf("exportHTML(): %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	page := string(b)
	for _, want := range []string{
		"Plague Marines", "Death Guard", "10 minis, 6 painted",
		"Nuln Oil", "1h 35m", "Shading",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the exported page is missing %q", want)
		}
	}
	// The export is a file handed to someone else, so a mini named after a
	// tag must not arrive as one.
	if strings.Contains(page, "<script>alert(1)</script>") {
		t.Error("notes were written into the page unescaped")
	}
}

func TestExportMarkdownCopiesThePhotosBeside(t *testing.T) {
	s := newTempStore(t)
	if err := os.MkdirAll(s.PhotoDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.PhotoDir(), "m1_1.jpg"), []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.data = Data{NextID: 100, Models: []Model{{
		ID: 1, Name: "Hero", Status: "Complete", Count: 1,
		Photos: []Photo{{ID: 3, File: "m1_1.jpg", Kind: "Final"}},
	}}}
	a := &App{store: s}
	path := filepath.Join(t.TempDir(), "hero.md")

	m, _ := s.ModelByID(1)
	if err := a.exportMarkdown(m, nil, path); err != nil {
		t.Fatalf("exportMarkdown(): %v", err)
	}

	beside := filepath.Join(filepath.Dir(path), "hero-photos", "m1_1.jpg")
	if _, err := os.Stat(beside); err != nil {
		t.Errorf("the photo was not copied beside the note: %v", err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "![Final](hero-photos/m1_1.jpg)") {
		t.Errorf("the note does not link the photo it copied:\n%s", b)
	}
}

// The format is chosen in the app, so the name the save dialog comes back
// with has to be made to match it: the file is handed to someone else, and
// one labelled .md with a web page inside is worse than either format.
func TestExportNamesTheFileForItsFormat(t *testing.T) {
	cases := []struct{ path, ext, want string }{
		{"hero", ".html", "hero.html"},
		{"hero.html", ".html", "hero.html"},
		{"hero.HTML", ".html", "hero.HTML"}, // already a page; leave the name typed
		{"hero.htm", ".html", "hero.htm"},
		{"hero", ".md", "hero.md"},
		{"hero.md", ".md", "hero.md"},
		{"hero.markdown", ".md", "hero.markdown"},
		{"hero.html", ".md", "hero.html.md"}, // the other format's name means nothing here
		{"Sgt. Weathers", ".md", "Sgt. Weathers.md"},
	}
	for _, c := range cases {
		if got := withExtension(c.path, c.ext); got != c.want {
			t.Errorf("withExtension(%q, %q) = %q, want %q", c.path, c.ext, got, c.want)
		}
	}
}

// writePhoto puts a real, decodable image in the store's photo folder. The
// HTML exporter re-encodes what it embeds, so a file of made-up bytes would
// be silently dropped and the test would pass for the wrong reason.
func writePhoto(t *testing.T, s *Store, name string) {
	t.Helper()
	if err := os.MkdirAll(s.PhotoDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(s.PhotoDir(), name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img := filled(8, 8, func(x, y int) color.RGBA { return color.RGBA{uint8(x * 8), uint8(y * 8), 40, 255} })
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// An export is handed to someone else as a record of the painter's own work.
// The maker's product photograph is not that, so neither exporter carries it
// however the mini is otherwise set up.
func TestExportLeavesTheProductShotOut(t *testing.T) {
	s := newTempStore(t)
	writePhoto(t, s, "m1_final.png")
	writePhoto(t, s, "m1_product.png")
	s.data = Data{NextID: 100, Models: []Model{{
		ID: 1, Name: "Hero", Status: "Complete", Count: 1,
		Photos: []Photo{
			{ID: 3, File: "m1_final.png", Kind: "Final"},
			{ID: 4, File: "m1_product.png", Kind: "Product"},
		},
	}}}
	a := &App{store: s}
	m, _ := s.ModelByID(1)

	htmlPath := filepath.Join(t.TempDir(), "hero.html")
	if err := a.exportHTML(m, nil, htmlPath); err != nil {
		t.Fatalf("exportHTML(): %v", err)
	}
	page, _ := os.ReadFile(htmlPath)
	// The kind is written as the caption under each shot, so counting the
	// captions is the same as counting what got embedded.
	if strings.Contains(string(page), "Product") {
		t.Error("the exported page carries the product shot")
	}
	if !strings.Contains(string(page), "Final") {
		t.Error("the exported page dropped the painter's own photo")
	}

	mdPath := filepath.Join(t.TempDir(), "hero.md")
	if err := a.exportMarkdown(m, nil, mdPath); err != nil {
		t.Fatalf("exportMarkdown(): %v", err)
	}
	note, _ := os.ReadFile(mdPath)
	if strings.Contains(string(note), "m1_product.png") {
		t.Error("the exported note links the product shot")
	}
	beside := filepath.Join(filepath.Dir(mdPath), "hero-photos")
	if _, err := os.Stat(filepath.Join(beside, "m1_product.png")); err == nil {
		t.Error("the product shot was copied beside the note")
	}
	if _, err := os.Stat(filepath.Join(beside, "m1_final.png")); err != nil {
		t.Errorf("the painter's own photo was not copied beside the note: %v", err)
	}
}

// A mini whose only image is the product shot has nothing to export. The
// filtering happens before the folder is made, so it should not be left with
// an empty photos folder and a heading over nothing.
func TestExportMakesNoPhotoFolderForAProductShotAlone(t *testing.T) {
	s := newTempStore(t)
	writePhoto(t, s, "m1_product.png")
	s.data = Data{NextID: 100, Models: []Model{{
		ID: 1, Name: "Hero", Status: "Backlog", Count: 1,
		Photos: []Photo{{ID: 4, File: "m1_product.png", Kind: "Product"}},
	}}}
	a := &App{store: s}
	m, _ := s.ModelByID(1)

	path := filepath.Join(t.TempDir(), "hero.md")
	if err := a.exportMarkdown(m, nil, path); err != nil {
		t.Fatalf("exportMarkdown(): %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "hero-photos")); err == nil {
		t.Error("an empty photos folder was written beside the note")
	}
	note, _ := os.ReadFile(path)
	if strings.Contains(string(note), "## Photos") {
		t.Error("the note has a Photos heading with nothing under it")
	}
}

func TestSafeFilenameKeepsANameUsableOnEveryPlatform(t *testing.T) {
	for _, c := range [][2]string{
		{"Sgt. Aurelius", "Sgt. Aurelius"},
		{`Marine: "Bob" <2/3>`, "Marine- -Bob- -2-3-"},
		{"   ", "mini"},
		{"trailing dots...", "trailing dots"},
	} {
		if got := safeFilename(c[0]); got != c[1] {
			t.Errorf("safeFilename(%q) = %q, want %q", c[0], got, c[1])
		}
	}
}
