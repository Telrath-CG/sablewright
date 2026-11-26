package main

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// writeZip builds a zip at path holding the given entries, written in name
// order so a test reads the same way every run.
func writeZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z := zip.NewWriter(f)
	names := make([]string, 0, len(entries))
	for n := range entries {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		w, err := z.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(entries[n])); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
}

// currentCollection gives a store something of its own to lose: one mini, a
// project deadline and a photo.
func currentCollection(t *testing.T) *Store {
	t.Helper()
	s := newTempStore(t)
	if err := os.MkdirAll(s.PhotoDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	s.data = Data{Version: dataVersion, NextID: 50,
		Models:   []Model{{ID: 1, Name: "Current", Count: 1}},
		Projects: []ProjectMeta{{Name: "Current project", Due: "2026-09-01"}},
	}
	if err := s.persist(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.PhotoDir(), "current.jpg"), []byte("current photo"), 0o644); err != nil {
		t.Fatal(err)
	}
	return s
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// A backup is a file someone handed over. Restoring one takes its collection
// and the photos in it, and nothing else: not a script dressed as a photo,
// not a file that climbs out of the data folder, not a folder of its own.
func TestRestoreFromTakesOnlyTheCollectionAndItsPhotos(t *testing.T) {
	s := currentCollection(t)
	// No "projects" key, as in a backup made before projects existed.
	collection := `{"version":5,"nextId":200,"models":[{"id":7,"name":"Restored","count":1,` +
		`"photos":[{"id":8,"file":"m7_1.jpg"}]}],"paints":[],"tips":[],"trash":[]}`
	zipPath := filepath.Join(t.TempDir(), "backup.zip")
	writeZip(t, zipPath, map[string]string{
		"collection.json":           collection,
		"photos/m7_1.jpg":           "restored photo",
		"photos/run.bat":            "echo not a photo",
		"photos/sub/nested.jpg":     "nested",
		"photos/.hidden.jpg":        "hidden",
		"photos/../../climbed.jpg":  "climbed",
		"../climbed.jpg":            "climbed",
		"backups/collection-x.json": "{}",
	})

	if err := s.RestoreFrom(zipPath, "20260808-090000"); err != nil {
		t.Fatalf("RestoreFrom(): %v", err)
	}

	if len(s.data.Models) != 1 || s.data.Models[0].Name != "Restored" {
		t.Errorf("models = %+v, want just the restored mini", s.data.Models)
	}
	if len(s.data.Projects) != 0 {
		t.Errorf("projects = %+v; a key the backup lacked kept the current collection's value", s.data.Projects)
	}
	if got := dirNames(t, s.PhotoDir()); len(got) != 1 || got[0] != "m7_1.jpg" {
		t.Errorf("photos folder holds %v, want only the restored photo", got)
	}
	for _, p := range []string{
		filepath.Join(s.dir, "climbed.jpg"),
		filepath.Join(filepath.Dir(s.dir), "climbed.jpg"),
		filepath.Join(s.dir, "backups", "collection-x.json"),
	} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("restore wrote %s, which the backup has no business putting there", p)
		}
	}
	for _, n := range dirNames(t, s.dir) {
		if strings.HasPrefix(n, ".restore-") {
			t.Errorf("the staging folder %s was left behind", n)
		}
	}

	// The file on disk is the restored collection, not just what is in memory.
	reopened := &Store{dir: s.dir, path: s.path}
	if err := reopened.load(); err != nil {
		t.Fatal(err)
	}
	if len(reopened.data.Models) != 1 || reopened.data.Models[0].Name != "Restored" {
		t.Errorf("reopened models = %+v, want the restored mini", reopened.data.Models)
	}
}

// The collection being replaced is kept, photos and all, so restoring the
// wrong file can itself be undone.
func TestRestoreFromKeepsTheCurrentCollection(t *testing.T) {
	s := currentCollection(t)
	zipPath := filepath.Join(t.TempDir(), "backup.zip")
	writeZip(t, zipPath, map[string]string{
		"collection.json": `{"version":5,"nextId":2,"models":[]}`,
	})

	if err := s.RestoreFrom(zipPath, "20260808-090000"); err != nil {
		t.Fatalf("RestoreFrom(): %v", err)
	}

	kept := filepath.Join(s.BackupDir(), "before-restore-20260808-090000")
	raw, err := os.ReadFile(filepath.Join(kept, "collection.json"))
	if err != nil || !strings.Contains(string(raw), "Current") {
		t.Errorf("the previous collection wasn't kept (%v)", err)
	}
	if b, err := os.ReadFile(filepath.Join(kept, "photos", "current.jpg")); err != nil || string(b) != "current photo" {
		t.Errorf("the previous photo wasn't kept (%v)", err)
	}
	// The kept folder is not one of the rolling copies, so launches never
	// rotate it out.
	if made, _ := s.rollingSnapshots(); len(made) != 0 {
		t.Errorf("rolling copies = %v, want the kept folder left out of them", made)
	}
}

// A zip that isn't a backup, or whose collection can't be read, changes
// nothing at all.
func TestRestoreFromLeavesEverythingAloneOnABadBackup(t *testing.T) {
	for name, entries := range map[string]map[string]string{
		"no collection":         {"photos/a.jpg": "photo"},
		"unreadable collection": {"collection.json": "{not json", "photos/a.jpg": "photo"},
	} {
		t.Run(name, func(t *testing.T) {
			s := currentCollection(t)
			zipPath := filepath.Join(t.TempDir(), "backup.zip")
			writeZip(t, zipPath, entries)

			if err := s.RestoreFrom(zipPath, "20260808-090000"); err == nil {
				t.Fatal("RestoreFrom() succeeded, want an error")
			}
			if len(s.data.Models) != 1 || s.data.Models[0].Name != "Current" {
				t.Errorf("models = %+v, want the current collection untouched", s.data.Models)
			}
			if got := dirNames(t, s.PhotoDir()); len(got) != 1 || got[0] != "current.jpg" {
				t.Errorf("photos folder holds %v, want the current photo untouched", got)
			}
			if _, err := os.Stat(s.BackupDir()); err == nil {
				t.Error("a failed restore still set the current collection aside")
			}
		})
	}
}

func TestPhotoEntryNameTakesOnlyImagesDirectlyInPhotos(t *testing.T) {
	cases := map[string]string{
		"photos/m1_2.jpg":     "m1_2.jpg",
		"photos/t_m1_2.jpg":   "t_m1_2.jpg",
		"photos/SHOT.PNG":     "SHOT.PNG",
		"photos/run.bat":      "",
		"photos/a.jpg.exe":    "",
		"photos/sub/a.jpg":    "",
		"photos/..\\a.jpg":    "",
		"photos/C:a.jpg":      "",
		"photos/.a.jpg":       "",
		"photos/":             "",
		"collection.json":     "",
		"elsewhere/a.jpg":     "",
		"../photos/a.jpg":     "",
		"photos/../a.jpg":     "",
		"photos/a.jpeg":       "a.jpeg",
		"photos/a.webp":       "a.webp",
		"photos/no-extension": "",
	}
	for entry, want := range cases {
		if got := photoEntryName(entry); got != want {
			t.Errorf("photoEntryName(%q) = %q, want %q", entry, got, want)
		}
	}
}

// A collection file that can't be parsed is moved aside, never written over,
// and the app starts on a fresh one with a warning rather than refusing to
// start at all.
func TestLoadSetsAnUnreadableCollectionAsideAndWarns(t *testing.T) {
	s := newTempStore(t)
	if err := os.WriteFile(s.path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := s.load()
	var aside *setAsideError
	if !errors.As(err, &aside) {
		t.Fatalf("load() = %v, want a set-aside warning", err)
	}
	if b, err := os.ReadFile(filepath.Join(s.dir, aside.name)); err != nil || string(b) != "{not json" {
		t.Errorf("the unreadable file wasn't kept as %s (%v)", aside.name, err)
	}
	if len(s.data.Paints) != len(PaintLibrary()) {
		t.Errorf("fresh collection has %d paints, want the library", len(s.data.Paints))
	}
}

// If the unreadable file can't be moved aside, it must not be replaced: it
// is the whole collection, and it may well be fixable by hand.
func TestLoadNeverOverwritesAnUnreadableCollectionItCannotMove(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("folder permissions don't block a rename on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores folder permissions")
	}
	s := newTempStore(t)
	if err := os.WriteFile(s.path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(s.dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(s.dir, 0o755) })

	err := s.load()
	var aside *setAsideError
	if err == nil || errors.As(err, &aside) {
		t.Fatalf("load() = %v, want a hard error", err)
	}
	if b, _ := os.ReadFile(s.path); string(b) != "{not json" {
		t.Errorf("the collection file now holds %q; it was overwritten", b)
	}
}

// Loading reads into a fresh value, so nothing from a collection already in
// memory survives into one whose file doesn't mention it.
func TestLoadDoesNotKeepFieldsTheFileLacks(t *testing.T) {
	s := newTempStore(t)
	s.data = Data{Projects: []ProjectMeta{{Name: "Stale", Due: "2026-09-01"}}}
	if err := os.WriteFile(s.path, []byte(`{"version":5,"nextId":3,"models":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	if len(s.data.Projects) != 0 {
		t.Errorf("projects = %+v, want none: the file has none", s.data.Projects)
	}
}
