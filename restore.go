package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// photoExts are the formats a new photo can be: the ones every platform's
// webview can draw. AddPhotos offers them in its picker and AddPhoto refuses
// anything else.
var photoExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true, ".webp": true,
}

// photoPattern is photoExts as a file dialog filter. Each is listed in both
// cases because GTK matches patterns case-sensitively, and a camera's
// IMG_0001.JPG would otherwise not be offered on Linux.
const photoPattern = "*.png;*.jpg;*.jpeg;*.gif;*.bmp;*.webp;" +
	"*.PNG;*.JPG;*.JPEG;*.GIF;*.BMP;*.WEBP"

// viewableExts are the picture formats a photo file may be. OpenPhoto hands
// only these to the system opener, which runs a script as readily as it
// shows a picture, and an import restores only these, so a backup can't put
// a shortcut or a script in the photos folder for someone to double-click.
// It is photoExts plus the picture formats older versions took in through an
// "All files" option, so photos already in a collection keep working.
var viewableExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true, ".webp": true,
	".jfif": true, ".heic": true, ".heif": true, ".tif": true, ".tiff": true, ".avif": true,
	".jxl": true, ".dng": true, ".raw": true, ".cr2": true, ".cr3": true, ".nef": true,
	".arw": true, ".orf": true, ".rw2": true, ".psd": true,
}

// isViewableImage reports whether a file is in one of the picture formats.
func isViewableImage(name string) bool {
	return viewableExts[strings.ToLower(filepath.Ext(name))]
}

// safeFileName reports whether a name is a plain file name that stays inside
// whatever folder it is joined onto. Photo names are read from the collection
// file, and a collection can arrive in a backup someone else made, so a name
// is never trusted to be one the app wrote: it must not climb out with "..",
// name another folder, carry a drive or stream (":"), hide with a leading
// dot, or end in a dot or space, which Windows strips - " .." would name the
// folder above. On Windows filepath.IsLocal also refuses device names such
// as CON.
func safeFileName(name string) bool {
	return name != "" && name == filepath.Base(name) && filepath.IsLocal(name) &&
		!strings.ContainsAny(name, `/\:`) && !strings.HasPrefix(name, ".") &&
		!strings.HasSuffix(name, ".") && !strings.HasSuffix(name, " ")
}

// photoPath is where a photo named in the collection lives, or false if the
// name isn't one that can safely be turned into a path.
func (s *Store) photoPath(name string) (string, bool) {
	if !safeFileName(name) {
		return "", false
	}
	return filepath.Join(s.PhotoDir(), name), true
}

// sanitizePhotos drops photo records whose file name isn't safe, and clears a
// thumbnail name that isn't, so that nothing read from a file can point the
// app at a path outside the photos folder. The app only ever writes names
// like m12_1754650000.jpg, so a collection it made loses nothing here.
func sanitizePhotos(d *Data) {
	clean := func(photos []Photo) []Photo {
		kept := photos[:0]
		for _, p := range photos {
			if !safeFileName(p.File) {
				continue
			}
			if p.Thumb != "" && !safeFileName(p.Thumb) {
				p.Thumb = ""
			}
			kept = append(kept, p)
		}
		return kept
	}
	for i := range d.Models {
		d.Models[i].Photos = clean(d.Models[i].Photos)
	}
	for i := range d.Trash {
		d.Trash[i].Model.Photos = clean(d.Trash[i].Model.Photos)
	}
}

// photoFiles lists every file a collection's photos use, thumbnails included,
// trashed minis included.
func photoFiles(d *Data) map[string]bool {
	files := map[string]bool{}
	add := func(photos []Photo) {
		for _, p := range photos {
			files[p.File] = true
			if p.Thumb != "" {
				files[p.Thumb] = true
			}
		}
	}
	for _, m := range d.Models {
		add(m.Photos)
	}
	for _, t := range d.Trash {
		add(t.Model.Photos)
	}
	return files
}

// RestoreFrom replaces the collection and its photos with the contents of a
// backup zip. stamp names the folder the current collection is kept in.
//
// A backup is a file someone handed over, so it is read for what a backup
// holds and nothing more: collection.json, and the picture files directly
// under photos/ that the collection says are its photos. Everything else in
// the zip is ignored, whatever it is called.
//
// Nothing current is touched until the whole backup has been read and
// unpacked, and the current collection and photos are moved into
// backups/before-restore-<stamp> rather than deleted, so a restore of the
// wrong file can itself be undone by hand.
func (s *Store) RestoreFrom(src, stamp string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("could not open that zip: %w", err)
	}
	defer zr.Close()

	var restored *Data
	for _, f := range zr.File {
		if f.Name != "collection.json" || f.FileInfo().IsDir() {
			continue
		}
		b, err := readZipEntry(f)
		if err != nil {
			return fmt.Errorf("could not read that backup: %w", err)
		}
		d, err := decodeData(b)
		if err != nil {
			return fmt.Errorf("that backup's collection could not be read: %w", err)
		}
		restored = &d
	}
	if restored == nil {
		return fmt.Errorf("that zip doesn't look like a Sablewright backup")
	}
	// Saving a newer build's collection here would quietly drop whatever
	// that build added, so it is refused rather than half-restored.
	if restored.Version > dataVersion {
		return fmt.Errorf("that backup was made by a newer version of Sablewright; " +
			"update this one to import it")
	}

	wanted := photoFiles(restored)
	var photos []*zip.File
	for _, f := range zr.File {
		name := photoEntryName(f.Name)
		if name != "" && wanted[name] && isViewableImage(name) && !f.FileInfo().IsDir() {
			photos = append(photos, f)
		}
	}

	// Unpack into a staging folder beside the real one, so the swap below is
	// a rename on the same disk rather than a copy that can fail half way.
	stage, err := os.MkdirTemp(s.dir, ".restore-")
	if err != nil {
		return fmt.Errorf("could not unpack that backup: %w", err)
	}
	defer os.RemoveAll(stage)
	staged := filepath.Join(stage, "photos")
	if err := os.Mkdir(staged, 0o755); err != nil {
		return fmt.Errorf("could not unpack that backup: %w", err)
	}
	for _, f := range photos {
		if err := extractZipEntry(f, filepath.Join(staged, photoEntryName(f.Name))); err != nil {
			return fmt.Errorf("could not unpack %s: %w", f.Name, err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	kept := filepath.Join(s.BackupDir(), "before-restore-"+stamp)
	if err := os.MkdirAll(kept, 0o755); err != nil {
		return fmt.Errorf("could not keep a copy of the current collection: %w", err)
	}
	if raw, err := os.ReadFile(s.path); err == nil {
		if err := os.WriteFile(filepath.Join(kept, "collection.json"), raw, 0o644); err != nil {
			return fmt.Errorf("could not keep a copy of the current collection: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("could not keep a copy of the current collection: %w", err)
	}
	keptPhotos := filepath.Join(kept, "photos")
	hadPhotos := true
	if err := os.Rename(s.PhotoDir(), keptPhotos); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("could not set the current photos aside: %w", err)
		}
		hadPhotos = false
	}
	if err := os.Rename(staged, s.PhotoDir()); err != nil {
		if !hadPhotos {
			_ = os.MkdirAll(s.PhotoDir(), 0o755)
		} else if rerr := os.Rename(keptPhotos, s.PhotoDir()); rerr != nil {
			return fmt.Errorf("could not put the backup's photos in place (%v), and your "+
				"current photos are still in %s: %w", err, keptPhotos, rerr)
		}
		return fmt.Errorf("could not put the backup's photos in place: %w", err)
	}

	previous := s.data
	s.data = *restored
	if s.data.Version < dataVersion {
		s.migrate()
		s.data.Version = dataVersion
	}
	if err := s.persist(); err != nil {
		// Put everything back as it was: the old data in memory, the old
		// photos on disk. The file itself was never replaced.
		s.data = previous
		if rerr := os.Rename(s.PhotoDir(), staged); rerr != nil {
			if hadPhotos {
				return fmt.Errorf("%w; your current photos are in %s", err, keptPhotos)
			}
			return err
		}
		if !hadPhotos {
			_ = os.MkdirAll(s.PhotoDir(), 0o755)
		} else if rerr := os.Rename(keptPhotos, s.PhotoDir()); rerr != nil {
			return fmt.Errorf("%w; your current photos are in %s", err, keptPhotos)
		}
		return err
	}
	return nil
}

// RemoveStaleStaging deletes staging folders an import left behind when the
// app was closed or crashed part way through one. Nothing reads them, and an
// import that got that far never touched the collection. It runs as the
// store opens, before anything can start an import of its own.
func (s *Store) RemoveStaleStaging() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), ".restore-") {
			_ = os.RemoveAll(filepath.Join(s.dir, e.Name()))
		}
	}
}

// photoEntryName returns the file name a zip entry restores to, or "" if the
// entry isn't one that could be. Only files sitting directly in photos/ with
// a safe name qualify; a name that tries to climb out, hide, or nest does not.
func photoEntryName(entry string) string {
	name, ok := strings.CutPrefix(entry, "photos/")
	if !ok || !safeFileName(name) {
		return ""
	}
	return name
}

func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func extractZipEntry(f *zip.File, dst string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
