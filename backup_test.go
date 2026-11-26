package main

import (
	"archive/zip"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A finished backup holds the collection and every photo, under the name
// asked for and nothing else beside it.
func TestWriteBackupHoldsTheCollectionAndPhotos(t *testing.T) {
	s := currentCollection(t)
	dst := filepath.Join(t.TempDir(), "backup.zip")
	if err := s.WriteBackup(dst); err != nil {
		t.Fatalf("WriteBackup(): %v", err)
	}
	zr, err := zip.OpenReader(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	if got := strings.Join(names, ","); got != "collection.json,photos/current.jpg" {
		t.Errorf("backup holds %s", got)
	}
	if _, err := os.Stat(dst + ".partial"); !os.IsNotExist(err) {
		t.Error("the .partial file was left beside the finished backup")
	}
}

// A backup that fails part way leaves nothing that looks like a backup.
func TestWriteBackupLeavesNothingWhenItFails(t *testing.T) {
	s := currentCollection(t)
	if err := os.Remove(s.path); err != nil { // nothing to back up
		t.Fatal(err)
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "backup.zip")
	if err := s.WriteBackup(dst); err == nil {
		t.Fatal("WriteBackup() succeeded without a collection to back up")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("a failed backup left %v behind", entries)
	}
}

// JPEG has no transparency, and a cut-out PNG used to come out as a black
// square. It is laid over white instead.
func TestThumbnailOfATransparentPNGIsWhiteNotBlack(t *testing.T) {
	dir := t.TempDir()
	img := image.NewNRGBA(image.Rect(0, 0, 20, 20)) // fully transparent
	img.Set(10, 10, color.NRGBA{R: 200, A: 255})
	f, err := os.Create(filepath.Join(dir, "cutout.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	name, err := makeThumb(dir, "cutout.png")
	if err != nil {
		t.Fatalf("makeThumb(): %v", err)
	}
	tf, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer tf.Close()
	thumb, err := jpeg.Decode(tf)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := thumb.At(0, 0).RGBA()
	if r>>8 < 240 || g>>8 < 240 || b>>8 < 240 {
		t.Errorf("a transparent corner came out as rgb(%d,%d,%d), want white", r>>8, g>>8, b>>8)
	}
}

// A Markdown link ends at the first space, so an export saved under a name
// with spaces needs its image paths encoded or every image breaks.
func TestMarkdownExportEncodesSpacesInImagePaths(t *testing.T) {
	s := currentCollection(t)
	s.data.Models[0].Photos = []Photo{{ID: 9, File: "current.jpg", Kind: "Progress"}}
	a := &App{store: s}
	out := filepath.Join(t.TempDir(), "Death Guard.md")
	if err := a.exportMarkdown(s.data.Models[0], nil, out); err != nil {
		t.Fatalf("exportMarkdown(): %v", err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "](Death%20Guard-photos/current.jpg)") {
		t.Errorf("image link not encoded:\n%s", b)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(out), "Death Guard-photos", "current.jpg")); err != nil {
		t.Errorf("the photo wasn't copied beside the note: %v", err)
	}
}

// Photos added at the same moment must each get a file of their own: two
// records sharing one file means deleting one destroys the other.
func TestAddPhotoGivesEveryPhotoItsOwnFile(t *testing.T) {
	s := currentCollection(t)
	src := filepath.Join(t.TempDir(), "shot.jpg")
	if err := os.WriteFile(src, []byte("not really a jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	const n = 32
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			_, err := s.AddPhoto(1, src, "Progress")
			errs <- err
		}()
	}
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("AddPhoto(): %v", err)
		}
	}
	files := map[string]bool{}
	for _, p := range s.data.Models[0].Photos {
		if files[p.File] {
			t.Errorf("two photos share %s", p.File)
		}
		files[p.File] = true
	}
	if len(files) != n {
		t.Errorf("%d distinct files for %d photos", len(files), n)
	}
}
