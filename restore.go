package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// imageExts are the photo formats the app takes in. AddPhotos offers these in
// its picker, and a restore or an open refuses anything else.
var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true, ".webp": true,
}

// isImageFile reports whether a file name carries one of the photo formats.
func isImageFile(name string) bool {
	return imageExts[strings.ToLower(filepath.Ext(name))]
}

// RestoreFrom replaces the collection and its photos with the contents of a
// backup zip. stamp names the folder the current collection is kept in.
//
// A backup is a file someone handed over, so it is read for what a backup
// holds and nothing more: collection.json, and the image files directly under
// photos/. Everything else in the zip is ignored, whatever it is called.
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
	var photos []*zip.File
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Name == "collection.json" {
			b, err := readZipEntry(f)
			if err != nil {
				return fmt.Errorf("could not read that backup: %w", err)
			}
			d, err := decodeData(b)
			if err != nil {
				return fmt.Errorf("that backup's collection could not be read: %w", err)
			}
			restored = &d
			continue
		}
		if photoEntryName(f.Name) != "" {
			photos = append(photos, f)
		}
	}
	if restored == nil {
		return fmt.Errorf("that zip doesn't look like a Sablewright backup")
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
	if err := os.Rename(s.PhotoDir(), keptPhotos); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("could not set the current photos aside: %w", err)
	}
	if err := os.Rename(staged, s.PhotoDir()); err != nil {
		_ = os.Rename(keptPhotos, s.PhotoDir())
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
		_ = os.Rename(s.PhotoDir(), staged)
		_ = os.Rename(keptPhotos, s.PhotoDir())
		return err
	}
	return nil
}

// photoEntryName returns the file name a zip entry restores to, or "" if the
// entry isn't one to restore. Only image files sitting directly in photos/
// qualify; a name that tries to climb out, hide, or nest is ignored.
func photoEntryName(entry string) string {
	name, ok := strings.CutPrefix(entry, "photos/")
	if !ok || name == "" || strings.ContainsAny(name, `/\:`) ||
		strings.HasPrefix(name, ".") || !isImageFile(name) {
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
