package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A mis-clicked delete used to take the photos with it, and a photo of a mini
// part-painted three months ago cannot be retaken. The record leaves the app
// immediately - nothing reads the trash - but it is recoverable.
func TestDeleteModelKeepsTheMiniAndItsPhotosRecoverable(t *testing.T) {
	s := newTempStore(t)
	if err := os.MkdirAll(s.PhotoDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	shot := filepath.Join(s.PhotoDir(), "m1_1.jpg")
	if err := os.WriteFile(shot, []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.data = Data{NextID: 100, Models: []Model{{
		ID: 1, Name: "Marine", Status: "Complete", Count: 1,
		Photos: []Photo{{ID: 5, File: "m1_1.jpg", Kind: "Final"}},
	}}}

	if err := s.DeleteModel(1); err != nil {
		t.Fatalf("DeleteModel(): %v", err)
	}

	if got := len(s.Models(ModelFilter{})); got != 0 {
		t.Errorf("the list still shows %d minis after the delete", got)
	}
	if _, err := os.Stat(shot); err != nil {
		t.Errorf("the photo was destroyed on delete: %v", err)
	}

	back, err := s.RestoreModel(1)
	if err != nil {
		t.Fatalf("RestoreModel(): %v", err)
	}
	if back.Name != "Marine" || len(back.Photos) != 1 {
		t.Errorf("restored %+v, want the mini with its photo", back)
	}
	if got := len(s.Models(ModelFilter{})); got != 1 {
		t.Errorf("the list shows %d minis after the undo, want 1", got)
	}
	if len(s.data.Trash) != 0 {
		t.Error("the restored mini is still sitting in the trash as well")
	}
}

// The trash is a grace period, not storage. Once it lapses the photos go too,
// or the data folder quietly hoards a collection you meant to be rid of.
func TestPurgeTrashDestroysWhatIsPastItsMonth(t *testing.T) {
	s := newTempStore(t)
	if err := os.MkdirAll(s.PhotoDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(s.PhotoDir(), "old.jpg")
	recent := filepath.Join(s.PhotoDir(), "recent.jpg")
	for _, p := range []string{old, recent} {
		if err := os.WriteFile(p, []byte("jpeg"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s.data = Data{NextID: 100, Trash: []Trashed{
		{Deleted: "2026-06-01", Model: Model{ID: 1, Name: "Long gone",
			Photos: []Photo{{ID: 2, File: "old.jpg"}}}},
		{Deleted: "2026-08-01", Model: Model{ID: 3, Name: "Last week",
			Photos: []Photo{{ID: 4, File: "recent.jpg"}}}},
	}}

	if got := s.PurgeTrash(day("2026-08-08")); got != 1 {
		t.Errorf("purged %d entries, want the one past 30 days", got)
	}

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("the lapsed mini's photo is still on disk")
	}
	if _, err := os.Stat(recent); err != nil {
		t.Error("a mini deleted last week lost its photo early")
	}
	if len(s.data.Trash) != 1 || s.data.Trash[0].Model.Name != "Last week" {
		t.Errorf("trash holds %+v, want just the recent one", s.data.Trash)
	}
}

// The failure this guards against is the one noticed afterwards: a bad
// import, a mass edit, a file that stopped being readable. Manual backups are
// better - they carry the photos - but they have to be remembered.
func TestSnapshotOnStartupKeepsThreeAndDropsTheOldest(t *testing.T) {
	s := newTempStore(t)
	s.data = Data{NextID: 100, Models: []Model{{ID: 1, Name: "Marine", Count: 1}}}
	if err := s.persist(); err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 8, 8, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		if err := s.SnapshotOnStartup(base.Add(time.Duration(i) * time.Hour).
			Format("20060102-150405")); err != nil {
			t.Fatalf("SnapshotOnStartup(): %v", err)
		}
	}

	entries, err := os.ReadDir(s.BackupDir())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != rollingBackups {
		t.Fatalf("kept %d backups (%v), want %d", len(names), names, rollingBackups)
	}
	// The three kept are the three newest, and the names sort that way.
	if !strings.Contains(strings.Join(names, " "), "20260808-130000") {
		t.Errorf("the newest snapshot is missing from %v", names)
	}
	if strings.Contains(strings.Join(names, " "), "20260808-090000") {
		t.Errorf("the oldest snapshot survived in %v", names)
	}
}
