package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// filled builds a test image of a given size, with every pixel distinct
// enough to tell one corner from another after a transform.
func filled(w, h int, at func(x, y int) color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, at(x, y))
		}
	}
	return img
}

func TestDownscaleFitsTheLongEdgeAndKeepsTheShape(t *testing.T) {
	src := filled(1000, 400, func(x, y int) color.RGBA { return color.RGBA{200, 100, 50, 255} })

	got := downscale(src, 480).Bounds()

	if got.Dx() != 480 || got.Dy() != 192 {
		t.Errorf("downscale(1000x400, 480) = %dx%d, want 480x192", got.Dx(), got.Dy())
	}
}

// Blowing a small photo up to fill the thumbnail would produce a blurrier
// file than the original it was made from, for more bytes on disk.
func TestDownscaleLeavesASmallImageAlone(t *testing.T) {
	src := filled(100, 80, func(x, y int) color.RGBA { return color.RGBA{1, 2, 3, 255} })

	if got := downscale(src, 480).Bounds(); got != src.Bounds() {
		t.Errorf("downscale() resized an already-small image to %v", got)
	}
}

// Averaging is the point of the box filter: a checkerboard sampled by picking
// one pixel per cell would come out solid black or solid white depending on
// where the grid happened to land.
func TestDownscaleAveragesRatherThanPicks(t *testing.T) {
	src := filled(400, 400, func(x, y int) color.RGBA {
		if (x+y)%2 == 0 {
			return color.RGBA{0, 0, 0, 255}
		}
		return color.RGBA{255, 255, 255, 255}
	})

	small := downscale(src, 100)

	r, _, _, _ := small.At(50, 50).RGBA()
	if v := r >> 8; v < 96 || v > 160 {
		t.Errorf("a black and white checkerboard averaged to %d, want mid grey", v)
	}
}

// A phone that was held sideways records the fact rather than rotating the
// pixels. Browsers honour that tag and image/jpeg does not, so without this
// the thumbnail would hang sideways beside the upright photo it came from.
func TestOrientTurnsAQuarterTurnClockwise(t *testing.T) {
	red, blue := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}
	src := filled(2, 1, func(x, y int) color.RGBA {
		if x == 0 {
			return red
		}
		return blue
	})

	got := orient(src, 6)

	if b := got.Bounds(); b.Dx() != 1 || b.Dy() != 2 {
		t.Fatalf("orient(2x1, 6) = %dx%d, want 1x2", b.Dx(), b.Dy())
	}
	if r, _, _, _ := got.At(0, 0).RGBA(); r>>8 != 255 {
		t.Error("the left-hand pixel should be on top after a clockwise turn")
	}
	if _, _, b, _ := got.At(0, 1).RGBA(); b>>8 != 255 {
		t.Error("the right-hand pixel should be at the bottom after a clockwise turn")
	}
}

func TestOrientLeavesTheIdentityAndNonsenseAlone(t *testing.T) {
	src := filled(3, 2, func(x, y int) color.RGBA { return color.RGBA{9, 9, 9, 255} })

	for _, o := range []int{0, 1, 9, -3} {
		if got := orient(src, o); got.Bounds() != src.Bounds() {
			t.Errorf("orient(_, %d) resized the image to %v", o, got.Bounds())
		}
	}
}

// exifJPEG builds the smallest JPEG that carries an orientation tag: the
// start marker, one APP1 Exif segment holding a single-entry TIFF directory,
// and the end marker.
func exifJPEG(orientation byte) []byte {
	tiff := []byte{
		'I', 'I', 0x2A, 0x00, // little-endian, the magic 42
		0x08, 0x00, 0x00, 0x00, // the first directory starts at byte 8
		0x01, 0x00, // holding one entry
		0x12, 0x01, // tag 0x0112, Orientation
		0x03, 0x00, // type 3, SHORT
		0x01, 0x00, 0x00, 0x00, // one of them
		orientation, 0x00, 0x00, 0x00, // the value, in the first two bytes
		0x00, 0x00, 0x00, 0x00, // no directory after this one
	}
	payload := append([]byte("Exif\x00\x00"), tiff...)
	size := len(payload) + 2
	out := []byte{0xFF, 0xD8, 0xFF, 0xE1, byte(size >> 8), byte(size)}
	out = append(out, payload...)
	return append(out, 0xFF, 0xD9)
}

func TestExifOrientationReadsTheTag(t *testing.T) {
	if got := exifOrientation(exifJPEG(6)); got != 6 {
		t.Errorf("exifOrientation() = %d, want 6", got)
	}
}

// Most of what reaches this is a PNG, a screenshot, or a camera that writes
// no EXIF at all, and every one of those has to come back as "leave it".
func TestExifOrientationDefaultsToUprightOnAnythingElse(t *testing.T) {
	for _, c := range []struct {
		what string
		b    []byte
	}{
		{"an empty file", nil},
		{"a PNG", []byte("\x89PNG\r\n\x1a\n")},
		{"a JPEG with no EXIF", []byte{0xFF, 0xD8, 0xFF, 0xDA, 0x00, 0x02}},
		{"a truncated segment", []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x40, 'E'}},
		{"a nonsense orientation", exifJPEG(99)},
	} {
		if got := exifOrientation(c.b); got != 1 {
			t.Errorf("exifOrientation(%s) = %d, want 1", c.what, got)
		}
	}
}

func TestMakeThumbWritesASmallerJpegBesideTheOriginal(t *testing.T) {
	dir := t.TempDir()
	src := filled(1200, 900, func(x, y int) color.RGBA {
		return color.RGBA{uint8(x % 256), uint8(y % 256), 128, 255}
	})
	f, err := os.Create(filepath.Join(dir, "shot.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, src); err != nil {
		t.Fatal(err)
	}
	f.Close()

	name, err := makeThumb(dir, "shot.png")
	if err != nil {
		t.Fatalf("makeThumb(): %v", err)
	}

	if name != "t_shot.jpg" {
		t.Errorf("thumbnail named %q, want t_shot.jpg", name)
	}
	tf, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("opening the thumbnail: %v", err)
	}
	defer tf.Close()
	cfg, _, err := image.DecodeConfig(tf)
	if err != nil {
		t.Fatalf("the thumbnail did not decode: %v", err)
	}
	if cfg.Width != thumbMax {
		t.Errorf("thumbnail is %dpx wide, want %d", cfg.Width, thumbMax)
	}
}

// The file picker offers webp and bmp, which the standard library cannot
// decode. That has to leave the photo itself imported and thumbnail-less
// rather than failing the import.
func TestMakeThumbRefusesWhatItCannotDecode(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notreally.webp"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := makeThumb(dir, "notreally.webp"); err == nil {
		t.Error("makeThumb() on an undecodable file returned no error")
	}
}
