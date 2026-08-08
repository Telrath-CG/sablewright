package main

// Thumbnails.
//
// Photos are copied in at whatever size the camera produced, which is several
// megabytes and several thousand pixels each. Every one of them was then
// decoded at full size to paint a 90px tile, so a mini with a dozen progress
// shots cost tens of megabytes of decoded bitmap to draw a strip of stamps.
//
// A thumbnail is generated once, on import, and stored beside the original.
// The original is never touched: clicking a photo still opens the full file in
// the system viewer, and a backup still holds the real image.
//
// Scaling is done here rather than with a library. It is one downscale of one
// image on a path that already touches the disk, and the arithmetic below is
// shorter than the dependency would be - which matters for a project whose
// Windows build needs nothing but Go.

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
)

// thumbMax is the long edge of a generated thumbnail. The largest a thumbnail
// is ever drawn is the detail pane's photo strip on a wide window; 480 leaves
// room for that and for a HiDPI screen drawing it at twice the CSS size.
const thumbMax = 480

// thumbQuality trades a little fidelity for a file a fraction of the size.
// These are never the image you look at closely - that's the original.
const thumbQuality = 82

// thumbName is what a photo's thumbnail is filed under. The prefix keeps it
// beside its original in the same folder, so a backup catches both and a
// restore needs no extra step, while still being obvious in a directory
// listing. The extension changes because the thumbnail is always a JPEG.
func thumbName(file string) string {
	return "t_" + strings.TrimSuffix(file, filepath.Ext(file)) + ".jpg"
}

// makeThumb writes a downscaled copy of srcPath into the same folder and
// returns the filename it used.
//
// A failure here is not fatal to anything. The formats the file picker offers
// include webp and bmp, which the standard library cannot decode, and a photo
// with no thumbnail simply falls back to the original everywhere it's drawn.
func makeThumb(dir, file string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		return "", err
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("could not read %s as an image: %w", file, err)
	}
	// Downscale first and rotate second: orientation is a pure permutation of
	// pixels, so doing it to the finished thumbnail is a few hundred thousand
	// moves rather than a few million.
	small := orient(downscale(img, thumbMax), exifOrientation(raw))

	name := thumbName(file)
	out, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	defer out.Close()
	if err := jpeg.Encode(out, small, &jpeg.Options{Quality: thumbQuality}); err != nil {
		return "", err
	}
	return name, nil
}

// sampleCap bounds how many source pixels are averaged into one output pixel.
//
// A true box filter reads every source pixel, which on a 12 megapixel phone
// photo is twelve million interface calls to produce a picture 480 pixels
// wide. Sampling a grid of at most 4x4 within each source cell is
// indistinguishable at this size and finishes in a fraction of the time; it
// is supersampling rather than averaging, and it is still averaging enough
// pixels to avoid the shimmer that picking a single one would produce.
const sampleCap = 4

// downscale shrinks an image so its long edge is at most max, averaging the
// source pixels that fall into each output pixel. An image already smaller
// than max is returned untouched - upscaling a photo to fill a thumbnail
// would only make a blurrier file than the original.
func downscale(src image.Image, max int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || (w <= max && h <= max) {
		return src
	}
	nw, nh := w, h
	if w >= h {
		nw, nh = max, h*max/w
	} else {
		nw, nh = w*max/h, max
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		y0, y1 := b.Min.Y+y*h/nh, b.Min.Y+(y+1)*h/nh
		if y1 <= y0 {
			y1 = y0 + 1
		}
		ystep := (y1 - y0 + sampleCap - 1) / sampleCap
		for x := 0; x < nw; x++ {
			x0, x1 := b.Min.X+x*w/nw, b.Min.X+(x+1)*w/nw
			if x1 <= x0 {
				x1 = x0 + 1
			}
			xstep := (x1 - x0 + sampleCap - 1) / sampleCap

			var r, g, bl, a, n uint64
			for yy := y0; yy < y1; yy += ystep {
				for xx := x0; xx < x1; xx += xstep {
					cr, cg, cb, ca := src.At(xx, yy).RGBA()
					r, g, bl, a, n = r+uint64(cr), g+uint64(cg), bl+uint64(cb), a+uint64(ca), n+1
				}
			}
			if n == 0 {
				continue
			}
			// RGBA() returns 16-bit values, so each average is shifted back
			// down to the 8 bits the destination image stores.
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8(r / n >> 8), G: uint8(g / n >> 8),
				B: uint8(bl / n >> 8), A: uint8(a / n >> 8),
			})
		}
	}
	return dst
}

// orient applies an EXIF orientation, which is how a phone records "I was
// held sideways" without rewriting the pixels. Browsers honour the tag and
// image/jpeg does not, so skipping this would hang a sideways thumbnail next
// to the upright photo it was made from.
func orient(img image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	nw, nh := w, h
	if o >= 5 { // the four that transpose the axes
		nw, nh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			sx, sy := x, y
			switch o {
			case 2: // mirrored
				sx = w - 1 - x
			case 3: // upside down
				sx, sy = w-1-x, h-1-y
			case 4: // mirrored, upside down
				sy = h - 1 - y
			case 5: // transposed
				sx, sy = y, x
			case 6: // a quarter turn clockwise
				sx, sy = y, h-1-x
			case 7: // transversed
				sx, sy = w-1-y, h-1-x
			case 8: // a quarter turn anticlockwise
				sx, sy = w-1-y, x
			}
			dst.Set(x, y, img.At(b.Min.X+sx, b.Min.Y+sy))
		}
	}
	return dst
}

// exifOrientation reads the orientation tag out of a JPEG, returning 1 (the
// identity) for anything it cannot find or does not understand - a PNG, a
// camera that writes no EXIF, a truncated file.
//
// This walks the segments by hand rather than pulling in an EXIF library,
// because one tag is wanted out of the whole standard and it lives at a fixed
// place in the first directory.
func exifOrientation(b []byte) int {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 { // not a JPEG
		return 1
	}
	for i := 2; i+4 <= len(b); {
		if b[i] != 0xFF {
			return 1 // out of step with the segment structure
		}
		marker := b[i+1]
		if marker == 0xD8 || (marker >= 0xD0 && marker <= 0xD9) {
			i += 2
			continue
		}
		if marker == 0xDA { // start of scan: the image data, no more headers
			return 1
		}
		size := int(b[i+2])<<8 | int(b[i+3])
		if size < 2 || i+2+size > len(b) {
			return 1
		}
		seg := b[i+4 : i+2+size]
		if marker == 0xE1 && len(seg) > 6 && string(seg[:6]) == "Exif\x00\x00" {
			return tiffOrientation(seg[6:])
		}
		i += 2 + size
	}
	return 1
}

// tiffOrientation reads tag 0x0112 out of the first directory of the TIFF
// block an Exif segment carries.
func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	// The block declares its own byte order, and both are in the wild.
	var big bool
	switch {
	case t[0] == 'M' && t[1] == 'M':
		big = true
	case t[0] == 'I' && t[1] == 'I':
		big = false
	default:
		return 1
	}
	u16 := func(p []byte) int {
		if big {
			return int(p[0])<<8 | int(p[1])
		}
		return int(p[1])<<8 | int(p[0])
	}
	u32 := func(p []byte) int {
		if big {
			return int(p[0])<<24 | int(p[1])<<16 | int(p[2])<<8 | int(p[3])
		}
		return int(p[3])<<24 | int(p[2])<<16 | int(p[1])<<8 | int(p[0])
	}
	off := u32(t[4:8])
	if off < 8 || off+2 > len(t) {
		return 1
	}
	count := u16(t[off : off+2])
	for i := 0; i < count; i++ {
		e := off + 2 + i*12
		if e+12 > len(t) {
			return 1
		}
		if u16(t[e:e+2]) != 0x0112 {
			continue
		}
		// A SHORT sits in the first two bytes of the four-byte value field,
		// which is where the padding goes on a big-endian block too.
		v := u16(t[e+8 : e+10])
		if v >= 1 && v <= 8 {
			return v
		}
		return 1
	}
	return 1
}
