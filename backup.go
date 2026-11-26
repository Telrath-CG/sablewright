package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// WriteBackup zips the collection and every photo into dst.
//
// The zip is written beside dst under a .partial name and renamed into place
// only once it is complete, so a backup that fails part way - a full disk, a
// photo that can't be read - leaves nothing behind that looks like a backup
// and isn't one.
func (s *Store) WriteBackup(dst string) (err error) {
	tmp := dst + ".partial"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("could not write the backup: %w", err)
	}
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(tmp)
		}
	}()

	z := zip.NewWriter(f)
	if err = addFileToZip(z, s.path, "collection.json"); err != nil {
		return err
	}
	entries, _ := os.ReadDir(s.PhotoDir())
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if err = addFileToZip(z, filepath.Join(s.PhotoDir(), e.Name()), "photos/"+e.Name()); err != nil {
			return err
		}
	}
	if err = z.Close(); err != nil {
		return fmt.Errorf("could not finish the backup: %w", err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("could not finish the backup: %w", err)
	}
	if err = os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("could not finish the backup: %w", err)
	}
	return nil
}

func addFileToZip(z *zip.Writer, src, name string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("could not read %s: %w", filepath.Base(src), err)
	}
	defer in.Close()
	w, err := z.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, in)
	return err
}
