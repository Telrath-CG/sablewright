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
	// Both of these are housekeeping the user never asked for, so neither is
	// allowed to hold the window shut. A collection with hundreds of photos
	// would take a noticeable while to catch its thumbnails up, and until
	// each one lands the UI simply draws the original.
	go func() {
		now := time.Now()
		if err := a.store.SnapshotOnStartup(now.Format("20060102-150405")); err != nil {
			// A missing or unreadable file is what the snapshot is for, not
			// something to interrupt the user over on the way in.
			_ = err
		}
		a.store.PurgeTrash(now)
		a.store.BackfillThumbs()
	}()
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

func (a *App) ListModels(f ModelFilter) []Model { return a.store.Models(f) }

// ModelFacets is what the models filter bar offers: the values actually in
// use, so the pickers can never point at something that matches nothing.
// A facet with fewer than two values is left out of the bar entirely - a
// collection all of one game system has no use for a game system filter.
type ModelFacets struct {
	Systems  []string `json:"systems"`
	Factions []string `json:"factions"`
	Projects []string `json:"projects"`
}

func (a *App) ModelFacets() ModelFacets {
	systems, factions, projects := a.store.ModelFacets()
	return ModelFacets{Systems: systems, Factions: factions, Projects: projects}
}

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
	Range  string `json:"range"`
	// Stock is the stock-status filter: "Owned only", "Not owned",
	// "On wishlist", or anything else for no filtering.
	Stock string `json:"stock"`
	Limit int    `json:"limit"` // 0 = everything
}

// PaintRow is a paint plus how many minis it's used on, so the table can show
// that without the frontend doing N extra round trips.
type PaintRow struct {
	Paint
	UsedOn int `json:"usedOn"`
}

// PaintPage is one screenful of the rack. Every collection is stocked with
// well over a thousand paints and the table only draws a few hundred of them,
// so sending the rest across on every keystroke is pure waste - Total is there
// to say how many matched regardless of what came back.
type PaintPage struct {
	Rows  []PaintRow `json:"rows"`
	Total int        `json:"total"`
}

func (a *App) ListPaints(f PaintFilter) PaintPage {
	ps := a.store.Paints(f.Search, f.Type, f.Brand, f.Range, f.Stock)
	page := PaintPage{Total: len(ps), Rows: []PaintRow{}}
	if f.Limit > 0 && len(ps) > f.Limit {
		ps = ps[:f.Limit]
	}
	for _, p := range ps {
		page.Rows = append(page.Rows, PaintRow{Paint: p, UsedOn: a.store.PaintUsage(p.ID)})
	}
	return page
}

// PaintFacets is everything the inventory screen needs to draw its filter bar
// and its counts, so it doesn't have to pull the whole rack down to work them
// out for itself.
type PaintFacets struct {
	Brands   []string `json:"brands"`
	Ranges   []string `json:"ranges"`
	Brand    string   `json:"brand"` // the requested brand, or "" if it's gone
	Total    int      `json:"total"`
	Owned    int      `json:"owned"`
	Wishlist int      `json:"wishlist"`
}

// Facets reports the brands on offer and, for the brand asked about, its
// ranges. A brand that no longer exists - the last paint of it was deleted,
// say - comes back as an empty Brand so the screen can fall back to showing
// everything instead of filtering on something that matches nothing.
func (a *App) Facets(brand string) PaintFacets {
	f := PaintFacets{Brands: a.store.Brands()}
	for _, b := range f.Brands {
		if b == brand {
			f.Brand = brand
			break
		}
	}
	f.Ranges = a.store.Ranges(f.Brand)
	f.Total, f.Owned, f.Wishlist = a.store.PaintCounts()
	return f
}

// PaintLinks is everywhere one paint turns up in the collection. Both lists
// come back in one call because the dialog draws them together, and both are
// short - a paint is on a handful of minis, not hundreds.
type PaintLinks struct {
	Minis []MiniRef `json:"minis"`
	Tips  []TipRef  `json:"tips"`
}

func (a *App) PaintLinks(paintID int) PaintLinks {
	return PaintLinks{
		Minis: a.store.ModelsUsingPaint(paintID),
		Tips:  a.store.TipsUsingPaint(paintID),
	}
}

// WishlistPage is the shopping list: what has been marked to buy, and what
// the collection implies should be on it.
type WishlistPage struct {
	Rows    []PaintRow `json:"rows"`
	Missing []PaintRow `json:"missing"`
}

func (a *App) Wishlist() WishlistPage {
	listed, missing := a.store.WishlistPaints()
	page := WishlistPage{Rows: []PaintRow{}, Missing: []PaintRow{}}
	for _, p := range listed {
		page.Rows = append(page.Rows, PaintRow{Paint: p, UsedOn: a.store.PaintUsage(p.ID)})
	}
	for _, p := range missing {
		page.Missing = append(page.Missing, PaintRow{Paint: p, UsedOn: a.store.PaintUsage(p.ID)})
	}
	return page
}

// SetPaintFlags ticks owned or wanted from the shopping list, where opening
// the edit dialog per pot would turn a quick pass into a chore.
func (a *App) SetPaintFlags(id int, owned, wishlist bool) (Paint, error) {
	return a.store.SetPaintFlags(id, owned, wishlist)
}

// CopyText puts the shopping list on the clipboard, which is how it reaches
// a phone, a notes app, or whatever gets carried into the shop.
func (a *App) CopyText(s string) error {
	return wruntime.ClipboardSetText(a.ctx, s)
}

// TimeReport adds up the painting log. The clock is read here rather than in
// the store so the buckets it produces can be tested against a fixed day.
func (a *App) TimeReport() TimeReport { return a.store.TimeReport(time.Now()) }

// Projects rolls up what's filed under each project name, for the same
// reason and in the same way.
func (a *App) Projects() []Project { return a.store.Projects(time.Now()) }

func (a *App) SaveProject(p ProjectMeta) error { return a.store.SaveProject(p) }

// RenameProject re-tags every mini filed under the old name and returns how
// many moved. An empty new name ungroups them.
func (a *App) RenameProject(from, to string) (int, error) {
	return a.store.RenameProject(from, to)
}

func (a *App) AllPaints() []Paint   { return a.store.AllPaints() }
func (a *App) Brands() []string     { return a.store.Brands() }
func (a *App) Statuses() []string   { return Statuses }
func (a *App) GetStats() Stats      { return a.store.Stats() }
func (a *App) DataFolder() string   { return a.store.Dir() }
func (a *App) StartupError() string { e := a.loadErr; a.loadErr = ""; return e }

// Version identifies the running build for the sidebar. The commit earns its
// place next to the number: every merge to main replaces the same rolling
// release, so two downloads a week apart both call themselves v1.1.0 and only
// the hash says which one a bug report is about.
func (a *App) Version() string {
	s := version
	if s != "dev" {
		s = "v" + s
	}
	if commit != "" {
		s += " (" + commit + ")"
	}
	return s
}

type TipFilter struct {
	Search   string `json:"search"`
	Category string `json:"category"`
}

func (a *App) ListTips(f TipFilter) []Tip { return a.store.TipList(f.Search, f.Category) }

// GetTip is how a paint opens the recipe that calls for it, without the
// screen having to pull down every note to find one.
func (a *App) GetTip(id int) *Tip {
	if t, ok := a.store.TipByID(id); ok {
		return &t
	}
	return nil
}

// ---------------------------------------------------------------------------
// Writes
// ---------------------------------------------------------------------------

func (a *App) SaveModel(m Model) (Model, error) {
	if strings.TrimSpace(m.Name) == "" {
		return Model{}, fmt.Errorf("please give this mini a name")
	}
	return a.store.SaveModel(m)
}

// DeleteModel moves a mini to the trash, where UndoDeleteModel can fetch it
// back for the next month. The screen offers that as an Undo on the toast.
func (a *App) DeleteModel(id int) error { return a.store.DeleteModel(id) }

func (a *App) UndoDeleteModel(id int) (*Model, error) {
	m, err := a.store.RestoreModel(id)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

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
	if !isHexColor(p.Hex) {
		return Paint{}, fmt.Errorf("color should look like #3f7ac2")
	}
	return a.store.SavePaint(p)
}

func (a *App) DeletePaint(id int) error { return a.store.DeletePaint(id) }

func (a *App) SaveTip(t Tip) (Tip, error) {
	if strings.TrimSpace(t.Title) == "" {
		return Tip{}, fmt.Errorf("please give the note a title")
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

// RestoreLibraryPaints puts back built-in paints the user has deleted.
func (a *App) RestoreLibraryPaints() (int, error) { return a.store.RestoreLibraryPaints() }

func isHexColor(s string) bool {
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
		Title: "Choose photo" + map[string]string{
			"Final":   " (final)",
			"Product": " (product image)",
		}[kind],
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

// SetCoverPhoto picks which shot represents this mini in the list.
func (a *App) SetCoverPhoto(modelID, photoID int) (*Model, error) {
	m, err := a.store.SetCoverPhoto(modelID, photoID)
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
	ok, err := a.Confirm("Import backup",
		"This replaces everything currently in the app with the contents of that backup.\n\nContinue?",
		"Import")
	if err != nil {
		return false, err
	}
	if !ok {
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

// Confirm shows a native yes/no dialog (used for deletes and other actions
// that can't be undone).
//
// Only macOS draws the buttons it's handed. Windows shows a fixed MB_YESNO
// pair and Linux a GTK_BUTTONS_YES_NO one, and both answer with the button
// they actually drew rather than the label asked for - so comparing against
// okLabel alone reads every confirmation on those two as a refusal, and the
// action silently never happens. Hence isAffirmative rather than ==.
func (a *App) Confirm(title, message, okLabel string) (bool, error) {
	if okLabel == "" {
		okLabel = "OK"
	}
	// The safe button has to be named "No" off macOS: the Windows backend
	// pre-selects it only when DefaultButton reads exactly that, and without
	// it the destructive button is the one Enter presses.
	cancelLabel := "Cancel"
	if runtime.GOOS != "darwin" {
		cancelLabel = "No"
	}
	choice, err := wruntime.MessageDialog(a.ctx, wruntime.MessageDialogOptions{
		Type:          wruntime.QuestionDialog,
		Title:         title,
		Message:       message,
		Buttons:       []string{okLabel, cancelLabel},
		DefaultButton: cancelLabel,
		CancelButton:  cancelLabel,
	})
	if err != nil {
		return false, err
	}
	return isAffirmative(choice, okLabel), nil
}

// isAffirmative reports whether a dialog answer means "go ahead". macOS hands
// back the label it was given; the other two hand back their own word for yes,
// and GTK says "OK" where Windows says "Ok".
func isAffirmative(choice, okLabel string) bool {
	choice = strings.TrimSpace(choice)
	if choice == "" {
		return false
	}
	switch strings.ToLower(choice) {
	case "yes", "ok":
		return true
	}
	return strings.EqualFold(choice, strings.TrimSpace(okLabel))
}

func (a *App) Info(title, message string) {
	_, _ = wruntime.MessageDialog(a.ctx, wruntime.MessageDialogOptions{
		Type: wruntime.InfoDialog, Title: title, Message: message,
	})
}
