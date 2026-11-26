package main

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// App is the service bound to the frontend. Every exported method here is
// callable from JavaScript as wails.Call.ByName("main.App.MethodName", ...)
// and returns a Promise; app.js wraps that as App().MethodName(...).
type App struct {
	app   *application.App
	win   *application.WebviewWindow
	store *Store
	// loadErr is a non-fatal problem from startup (e.g. an unreadable data
	// file that we moved aside) which we surface once the UI is up.
	loadErr string
}

func NewApp() *App { return &App{} }

// ServiceStartup runs once as the app starts, before the window's page loads.
// Wails calls it by name; it is not one of the methods the page can call.
func (a *App) ServiceStartup(context.Context, application.ServiceOptions) error {
	// All of this is housekeeping the user never asked for, so none of it is
	// allowed to hold the window shut. A collection with hundreds of photos
	// would take a noticeable while to catch its thumbnails up, and until
	// each one lands the UI simply draws the original.
	go func() {
		now := time.Now()
		// A missing or unreadable file is what the snapshot is for, not
		// something to interrupt the user over on the way in.
		_ = a.store.SnapshotOnStartup(now.Format("20060102-150405"))
		a.store.PurgeTrash(now)
		a.store.BackfillThumbs()
	}()
	return nil
}

// onSecondInstance brings the open window forward when Sablewright is
// launched again. Only one copy may run: each holds the whole collection in
// memory and saves all of it, so a second copy would silently undo the
// first one's changes the next time it saved.
func (a *App) onSecondInstance(application.SecondInstanceData) {
	if a.win == nil {
		return
	}
	a.win.UnMinimise()
	a.win.Show()
	a.win.Focus()
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
	used := a.store.PaintUsageCounts()
	for _, p := range ps {
		page.Rows = append(page.Rows, PaintRow{Paint: p, UsedOn: used[p.ID]})
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
	used := a.store.PaintUsageCounts()
	for _, p := range listed {
		page.Rows = append(page.Rows, PaintRow{Paint: p, UsedOn: used[p.ID]})
	}
	for _, p := range missing {
		page.Missing = append(page.Missing, PaintRow{Paint: p, UsedOn: used[p.ID]})
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
	if !a.app.Clipboard.SetText(s) {
		return fmt.Errorf("could not copy to the clipboard")
	}
	return nil
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
	paths, err := a.openFiles("Choose photo"+map[string]string{
		"Final":   " (final)",
		"Product": " (product image)",
	}[kind], fileFilter{"Images", photoPattern})
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

// UndoDeletePhoto puts a deleted photo back. The screen offers it as an Undo
// on the toast, as it does for a deleted mini.
func (a *App) UndoDeletePhoto(modelID, photoID int) (*Model, error) {
	m, err := a.store.UndoDeletePhoto(modelID, photoID)
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
// Only image files are handed to the system: the name comes from the page,
// and the system opener would just as happily run a script.
func (a *App) OpenPhoto(file string) error {
	path, ok := a.store.photoPath(file)
	if !ok || !isViewableImage(file) {
		return fmt.Errorf("that isn't a photo")
	}
	return openPath(path)
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
	dst, err := a.saveFile("Save backup",
		"mini-painting-backup-"+time.Now().Format("2006-01-02")+".zip",
		fileFilter{"Zip archive (*.zip)", "*.zip"})
	if err != nil || dst == "" {
		return "", err
	}
	if err := a.store.WriteBackup(dst); err != nil {
		return "", err
	}
	return dst, nil
}

// Restore replaces the current collection with the contents of a backup zip.
func (a *App) Restore() (bool, error) {
	src, err := a.openFile("Choose a backup zip", fileFilter{"Zip archive (*.zip)", "*.zip"})
	if err != nil || src == "" {
		return false, err
	}
	ok, err := a.Confirm("Import backup",
		"This replaces everything currently in the app with the contents of that backup.\n\n"+
			"Your current collection and photos are moved to the backups folder first.\n\nContinue?",
		"Import")
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	if err := a.store.RestoreFrom(src, time.Now().Format("20060102-150405")); err != nil {
		return false, err
	}
	return true, nil
}
