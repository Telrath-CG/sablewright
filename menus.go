package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Native menus, shortcuts and right-click menus all ask the page to do
// things rather than doing them here: each sends the page a named action,
// and the page runs the same function its own button runs. One code path per
// action, whichever way it was asked for.

// menuAction is that request: what to do, and to which record.
type menuAction struct {
	Action string `json:"action"`
	ID     string `json:"id,omitempty"`
}

// send asks the page to perform an action.
func (a *App) send(action, id string) {
	a.app.Event.Emit("menu", menuAction{Action: action, ID: id})
}

// act is send as a menu item's click handler.
func (a *App) act(action string) func(*application.Context) {
	return func(*application.Context) { a.send(action, "") }
}

// screens are the sidebar's screens in order, for the View menu and the
// number shortcuts.
var screens = []struct{ key, label string }{
	{"dashboard", "Dashboard"},
	{"models", "Models"},
	{"projects", "Projects"},
	{"paints", "Paint Inventory"},
	{"wishlist", "Wishlist"},
	{"tips", "Technique Notes"},
	{"time", "Time at the Desk"},
}

// appMenu is the macOS menu bar: the menus every Mac app has, and File and
// View with Sablewright's own commands and their shortcuts.
func (a *App) appMenu() *application.Menu {
	m := application.NewMenu()
	m.AddRole(application.AppMenu)

	file := m.AddSubmenu("File")
	file.Add("New Mini").SetAccelerator("CmdOrCtrl+N").OnClick(a.act("new-mini"))
	file.Add("Export Mini…").SetAccelerator("CmdOrCtrl+E").OnClick(a.act("export"))
	file.AddSeparator()
	file.Add("Back Up…").OnClick(a.act("backup"))
	file.Add("Import Backup…").OnClick(a.act("import"))
	file.Add("Open Data Folder").OnClick(a.act("data-folder"))

	m.AddRole(application.EditMenu)

	view := m.AddSubmenu("View")
	view.Add("Find").SetAccelerator("CmdOrCtrl+F").OnClick(a.act("find"))
	view.AddSeparator()
	for i, s := range screens {
		view.Add(s.label).SetAccelerator(fmt.Sprintf("CmdOrCtrl+%d", i+1)).OnClick(a.act("screen:" + s.key))
	}

	m.AddRole(application.WindowMenu)
	return m
}

// keyBindings are the same shortcuts for Windows and Linux, which keep the
// window as it is - no menu bar - and take them as key bindings instead.
func (a *App) keyBindings() map[string]func(application.Window) {
	bind := func(action string) func(application.Window) {
		return func(application.Window) { a.send(action, "") }
	}
	keys := map[string]func(application.Window){
		"CmdOrCtrl+N": bind("new-mini"),
		"CmdOrCtrl+E": bind("export"),
		"CmdOrCtrl+F": bind("find"),
	}
	for i, s := range screens {
		keys[fmt.Sprintf("CmdOrCtrl+%d", i+1)] = bind("screen:" + s.key)
	}
	return keys
}

// contextMenus registers the right-click menus. The page marks an element
// with the --custom-contextmenu style naming one of these, and the record's
// id in --custom-contextmenu-data, which comes back here with the click.
func (a *App) contextMenus() {
	withID := func(action string) func(*application.Context) {
		return func(ctx *application.Context) { a.send(action, strings.TrimSpace(ctx.ContextMenuData())) }
	}

	mini := a.app.ContextMenu.New()
	mini.Add("Edit…").OnClick(withID("mini-edit"))
	mini.Add("Export…").OnClick(withID("mini-export"))
	mini.AddSeparator()
	mini.Add("Delete…").OnClick(withID("mini-delete"))
	a.app.ContextMenu.Add("mini", mini)

	photo := a.app.ContextMenu.New()
	photo.Add("Open Full Size").OnClick(withID("photo-open"))
	photo.Add("Use as Cover").OnClick(withID("photo-cover"))
	photo.AddSeparator()
	photo.Add("Delete").OnClick(withID("photo-delete"))
	a.app.ContextMenu.Add("photo", photo)

	paint := a.app.ContextMenu.New()
	paint.Add("Edit…").OnClick(withID("paint-edit"))
	paint.Add("Mark Owned").OnClick(withID("paint-owned"))
	paint.Add("Add to Wishlist").OnClick(withID("paint-want"))
	paint.AddSeparator()
	paint.Add("Delete…").OnClick(withID("paint-delete"))
	a.app.ContextMenu.Add("paint", paint)
}

// photosAdded tells the page what a drop added.
type photosAdded struct {
	ID      int    `json:"id"`      // the mini they were added to
	Added   int    `json:"added"`   // how many were kept
	Skipped int    `json:"skipped"` // how many weren't: not pictures, or unreadable
	Error   string `json:"error,omitempty"`
}

// onFilesDropped adds pictures dropped onto a mini's photos. The page marks
// the drop zone with the mini's id in data-mini (and optionally the kind of
// photo in data-kind); the paths never pass through the page, which keeps the
// same rule as the file picker: only the app decides what it opens.
func (a *App) onFilesDropped(e *application.WindowEvent) {
	ctx := e.Context()
	files := ctx.DroppedFiles()
	target := ctx.DropTargetDetails()
	if target == nil || len(files) == 0 {
		return
	}
	id, err := strconv.Atoi(target.Attributes["data-mini"])
	if err != nil || id <= 0 {
		return
	}
	kind := target.Attributes["data-kind"]
	if kind != "Product" && kind != "Final" {
		kind = "Progress"
	}
	// Copying and thumbnailing take a moment per photo: not on the thread
	// that delivered the drop.
	go func() {
		res := photosAdded{ID: id}
		for _, f := range files {
			if !photoExts[strings.ToLower(filepath.Ext(f))] {
				res.Skipped++
				continue
			}
			if _, err := a.store.AddPhoto(id, f, kind); err != nil {
				res.Skipped++
				res.Error = err.Error()
				continue
			}
			res.Added++
		}
		a.app.Event.Emit("photos-added", res)
	}()
}
