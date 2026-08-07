package main

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the object bound to the frontend. Every exported method here is
// callable from JavaScript as window.go.main.App.MethodName(...) and returns
// a Promise.
type App struct {
	ctx   context.Context
	store *Store
	// loadErr is a non-fatal problem from startup (e.g. an unreadable data
	// file that we moved aside) which we surface once the UI is up.
	loadErr string
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

type ModelFilter struct {
	Search string `json:"search"`
	Status string `json:"status"`
	Sort   string `json:"sort"`
}

func (a *App) ListModels(f ModelFilter) []Model { return a.store.Models(f.Search, f.Status, f.Sort) }

func (a *App) GetModel(id int) *Model {
	if m, ok := a.store.ModelByID(id); ok {
		return &m
	}
	return nil
}

type PaintFilter struct {
	Search string `json:"search"`
	Type   string `json:"type"`
	Brand  string `json:"brand"`
	Owned  string `json:"owned"`
}

// PaintRow is a paint plus how many minis it's used on, so the table can show
// that without the frontend doing N extra round trips.
type PaintRow struct {
	Paint
	UsedOn int `json:"usedOn"`
}

func (a *App) ListPaints(f PaintFilter) []PaintRow {
	ps := a.store.Paints(f.Search, f.Type, f.Brand, f.Owned)
	out := make([]PaintRow, 0, len(ps))
	for _, p := range ps {
		out = append(out, PaintRow{Paint: p, UsedOn: a.store.PaintUsage(p.ID)})
	}
	return out
}

func (a *App) AllPaints() []Paint   { return a.store.AllPaints() }
func (a *App) Brands() []string     { return a.store.Brands() }
func (a *App) Statuses() []string   { return Statuses }
func (a *App) GetStats() Stats      { return a.store.Stats() }
func (a *App) DataFolder() string   { return a.store.Dir() }
func (a *App) StartupError() string { e := a.loadErr; a.loadErr = ""; return e }

type TipFilter struct {
	Search   string `json:"search"`
	Category string `json:"category"`
}

func (a *App) ListTips(f TipFilter) []Tip { return a.store.TipList(f.Search, f.Category) }

// ---------------------------------------------------------------------------
// Writes
// ---------------------------------------------------------------------------

func (a *App) SaveModel(m Model) (Model, error) {
	if strings.TrimSpace(m.Name) == "" {
		return Model{}, fmt.Errorf("please give this mini a name")
	}
	return a.store.SaveModel(m)
}

func (a *App) DeleteModel(id int) error { return a.store.DeleteModel(id) }

// SaveSession adds or updates one entry in a mini's painting log.
func (a *App) SaveSession(modelID int, s Session) (*Model, error) {
	if strings.TrimSpace(s.Notes) == "" {
		return nil, fmt.Errorf("write a line about what you did this session")
	}
	m, err := a.store.SaveSession(modelID, s)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (a *App) DeleteSession(modelID, sessionID int) (*Model, error) {
	m, err := a.store.DeleteSession(modelID, sessionID)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (a *App) SavePaint(p Paint) (Paint, error) {
	if strings.TrimSpace(p.Name) == "" {
		return Paint{}, fmt.Errorf("please give the paint a name")
	}
	if !isHexColour(p.Hex) {
		return Paint{}, fmt.Errorf("colour should look like #3f7ac2")
	}
	return a.store.SavePaint(p)
}

func (a *App) DeletePaint(id int) error { return a.store.DeletePaint(id) }

func (a *App) SaveTip(t Tip) (Tip, error) {
	if strings.TrimSpace(t.Title) == "" {
		return Tip{}, fmt.Errorf("please give the tip a title")
	}
	clean := make([]string, 0, len(t.Tags))
	for _, tag := range t.Tags {
		tag = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(tag), "#"))
		if tag != "" {
			clean = append(clean, tag)
		}
	}
	t.Tags = clean
	return a.store.SaveTip(t)
}

func (a *App) DeleteTip(id int) error { return a.store.DeleteTip(id) }

func (a *App) AddStarterPaints() (int, error) { return a.store.AddStarterPaints() }

func isHexColour(s string) bool {
	if len(s) != 7 && len(s) != 4 {
		return false
	}
	if s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Photos
// ---------------------------------------------------------------------------

// AddPhotos opens a native file picker and copies the chosen images into the
// app's own photo folder, so moving or deleting the originals is harmless.
func (a *App) AddPhotos(modelID int, kind string) (*Model, error) {
	paths, err := wruntime.OpenMultipleFilesDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Choose photo" + map[string]string{"Final": " (final)"}[kind],
		Filters: []wruntime.FileFilter{
			{DisplayName: "Images (*.png;*.jpg;*.jpeg;*.gif;*.bmp;*.webp)",
				Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.bmp;*.webp"},
			{DisplayName: "All files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return nil, err
	}
	var m Model
	for _, p := range paths {
		m, err = a.store.AddPhoto(modelID, p, kind)
		if err != nil {
			return nil, err
		}
	}
	if len(paths) == 0 {
		cur, _ := a.store.ModelByID(modelID)
		return &cur, nil
	}
	return &m, nil
}

func (a *App) DeletePhoto(modelID, photoID int) (*Model, error) {
	m, err := a.store.DeletePhoto(modelID, photoID)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// OpenPhoto shows a photo full size in the system's default image viewer.
func (a *App) OpenPhoto(file string) error {
	return openPath(filepath.Join(a.store.PhotoDir(), filepath.Base(file)))
}

func (a *App) OpenDataFolder() error { return openPath(a.store.Dir()) }

func openPath(p string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", p).Start()
	case "darwin":
		return exec.Command("open", p).Start()
	default:
		return exec.Command("xdg-open", p).Start()
	}
}

// ---------------------------------------------------------------------------
// Backup / restore
// ---------------------------------------------------------------------------

func (a *App) Backup() (string, error) {
	dst, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
		Title:           "Save backup",
		DefaultFilename: "mini-painting-backup-" + time.Now().Format("2006-01-02") + ".zip",
		Filters:         []wruntime.FileFilter{{DisplayName: "Zip archive (*.zip)", Pattern: "*.zip"}},
	})
	if err != nil || dst == "" {
		return "", err
	}
	f, err := os.Create(dst)
	if err != nil {
		return "", fmt.Errorf("could not write the backup: %w", err)
	}
	defer f.Close()
	z := zip.NewWriter(f)
	if err := addFileToZip(z, a.store.path, "collection.json"); err != nil {
		z.Close()
		return "", err
	}
	entries, _ := os.ReadDir(a.store.PhotoDir())
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if err := addFileToZip(z, filepath.Join(a.store.PhotoDir(), e.Name()), "photos/"+e.Name()); err != nil {
			z.Close()
			return "", err
		}
	}
	if err := z.Close(); err != nil {
		return "", fmt.Errorf("could not finish the backup: %w", err)
	}
	return dst, nil
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

// Restore replaces the current collection with the contents of a backup zip.
func (a *App) Restore() (bool, error) {
	src, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:   "Choose a backup zip",
		Filters: []wruntime.FileFilter{{DisplayName: "Zip archive (*.zip)", Pattern: "*.zip"}},
	})
	if err != nil || src == "" {
		return false, err
	}
	choice, err := wruntime.MessageDialog(a.ctx, wruntime.MessageDialogOptions{
		Type:          wruntime.QuestionDialog,
		Title:         "Import backup",
		Message:       "This replaces everything currently in the app with the contents of that backup.\n\nContinue?",
		Buttons:       []string{"Import", "Cancel"},
		DefaultButton: "Cancel",
		CancelButton:  "Cancel",
	})
	if err != nil {
		return false, err
	}
	if choice != "Import" {
		return false, nil
	}

	zr, err := zip.OpenReader(src)
	if err != nil {
		return false, fmt.Errorf("could not open that zip: %w", err)
	}
	defer zr.Close()
	hasData := false
	for _, f := range zr.File {
		if f.Name == "collection.json" {
			hasData = true
		}
	}
	if !hasData {
		return false, fmt.Errorf("that zip doesn't look like a Sablewright backup")
	}

	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// guard against zip-slip: never write outside the data folder
		name := filepath.Clean(filepath.FromSlash(f.Name))
		if strings.HasPrefix(name, "..") || filepath.IsAbs(name) {
			continue
		}
		dst := filepath.Join(a.store.dir, name)
		if !strings.HasPrefix(dst, filepath.Clean(a.store.dir)+string(os.PathSeparator)) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return false, err
		}
		rc, err := f.Open()
		if err != nil {
			return false, err
		}
		out, err := os.Create(dst)
		if err != nil {
			rc.Close()
			return false, err
		}
		_, cErr := io.Copy(out, rc)
		rc.Close()
		out.Close()
		if cErr != nil {
			return false, cErr
		}
	}
	if err := a.store.load(); err != nil {
		return false, err
	}
	return true, nil
}

// Confirm shows a native yes/no dialog (used for deletes).
func (a *App) Confirm(title, message, okLabel string) (bool, error) {
	if okLabel == "" {
		okLabel = "OK"
	}
	choice, err := wruntime.MessageDialog(a.ctx, wruntime.MessageDialogOptions{
		Type:          wruntime.QuestionDialog,
		Title:         title,
		Message:       message,
		Buttons:       []string{okLabel, "Cancel"},
		DefaultButton: "Cancel",
		CancelButton:  "Cancel",
	})
	return choice == okLabel, err
}

func (a *App) Info(title, message string) {
	_, _ = wruntime.MessageDialog(a.ctx, wruntime.MessageDialogOptions{
		Type: wruntime.InfoDialog, Title: title, Message: message,
	})
}
