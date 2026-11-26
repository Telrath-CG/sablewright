package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// The window's size and position, kept between launches. Wails doesn't
// remember them, so a window dragged to a second monitor or made larger came
// back centred at its first size every time.

const (
	minWidth  = 940
	minHeight = 600
)

// windowState is where the window was. It lives in window.json beside the
// collection rather than in it: it describes this computer's screens, not
// the collection, and has no business in a backup.
type windowState struct {
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximised bool `json:"maximised"`
}

func windowStatePath(dir string) string { return filepath.Join(dir, "window.json") }

// loadWindowState reads the saved window, or reports there is none worth
// using: a missing or unreadable file, or a size outside what makes sense.
func loadWindowState(dir string) (windowState, bool) {
	b, err := os.ReadFile(windowStatePath(dir))
	if err != nil {
		return windowState{}, false
	}
	var ws windowState
	if json.Unmarshal(b, &ws) != nil ||
		ws.Width < minWidth || ws.Height < minHeight || ws.Width > 20000 || ws.Height > 20000 {
		return windowState{}, false
	}
	return ws, true
}

// saveWindowState writes the window's state, replacing the file in one step
// so a quit half way through can't leave it unreadable.
func saveWindowState(dir string, ws windowState) error {
	b, err := json.MarshalIndent(ws, "", "  ")
	if err != nil {
		return err
	}
	tmp := windowStatePath(dir) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, windowStatePath(dir))
}

// grabbable reports whether a window at ws would show enough of its top edge
// on one of the screens' work areas to be seen and dragged: a monitor that
// has since been unplugged, or a resolution since lowered, would otherwise
// put it somewhere it can't be reached.
func grabbable(ws windowState, areas []application.Rect) bool {
	const need = 100 // pixels of the top edge that must be on a screen
	for _, a := range areas {
		left, right := max(ws.X, a.X), min(ws.X+ws.Width, a.X+a.Width)
		if right-left >= need && ws.Y >= a.Y && ws.Y < a.Y+a.Height-40 {
			return true
		}
	}
	return false
}

// rememberWindow restores the saved size and position once the app has
// started, if they still fit the screens, and saves them again whenever the
// window is moved or resized.
func rememberWindow(app *application.App, win *application.WebviewWindow, dir string) {
	if saved, ok := loadWindowState(dir); ok {
		app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
			// Off the thread that delivered the event: moving the window
			// waits for the main thread.
			go func() {
				var areas []application.Rect
				for _, s := range app.Screen.GetAll() {
					areas = append(areas, s.WorkArea)
				}
				if grabbable(saved, areas) {
					win.SetBounds(application.Rect{X: saved.X, Y: saved.Y, Width: saved.Width, Height: saved.Height})
				} else {
					win.SetSize(saved.Width, saved.Height)
				}
				if saved.Maximised {
					win.Maximise()
				}
			}()
		})
	}

	// Saved a moment after the last move or resize rather than on every one
	// - a drag sends dozens.
	var mu sync.Mutex
	var pending *time.Timer
	var last windowState
	save := func(*application.WindowEvent) {
		mu.Lock()
		defer mu.Unlock()
		if pending != nil {
			pending.Stop()
		}
		pending = time.AfterFunc(400*time.Millisecond, func() {
			ws := windowState{Maximised: win.IsMaximised()}
			mu.Lock()
			// A maximised window keeps the size it will go back to.
			if ws.Maximised {
				ws.X, ws.Y, ws.Width, ws.Height = last.X, last.Y, last.Width, last.Height
			} else {
				b := win.Bounds()
				ws.X, ws.Y, ws.Width, ws.Height = b.X, b.Y, b.Width, b.Height
				last = ws
			}
			mu.Unlock()
			if ws.Width >= minWidth && ws.Height >= minHeight {
				_ = saveWindowState(dir, ws)
			}
		})
	}
	win.OnWindowEvent(events.Common.WindowDidMove, save)
	win.OnWindowEvent(events.Common.WindowDidResize, save)
}
