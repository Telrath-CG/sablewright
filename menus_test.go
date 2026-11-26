package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// Dropped paths come back from the page, so each is checked before it is
// copied: a real picture is taken, anything else is refused.
func TestDroppablePicture(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.png")
	f, err := os.Create(real)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if !droppablePicture(real) {
		t.Error("a real PNG was refused")
	}

	fake := filepath.Join(dir, "secrets.png")
	if err := os.WriteFile(fake, []byte("password=hunter2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if droppablePicture(fake) {
		t.Error("a text file named .png was taken")
	}
	if droppablePicture(filepath.Join(dir, "missing.png")) {
		t.Error("a missing file was taken")
	}
	if droppablePicture(dir) {
		t.Error("a folder was taken")
	}
	link := filepath.Join(dir, "link.png")
	if err := os.Symlink(real, link); err == nil && droppablePicture(link) {
		t.Error("a symlink was followed")
	}
}
