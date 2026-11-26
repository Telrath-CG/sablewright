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

// fitSize shrinks a saved size to fit the largest of the screens, so a
// window last sized on a monitor that has since been unplugged can't open
// with its edges out of reach.
func fitSize(ws windowState, areas []application.Rect) (int, int) {
	maxW, maxH := 0, 0
	for _, a := range areas {
		maxW, maxH = max(maxW, a.Width), max(maxH, a.Height)
	}
	w, h := ws.Width, ws.Height
	if maxW > 0 {
		w = max(minWidth, min(w, maxW))
	}
	if maxH > 0 {
		h = max(minHeight, min(h, maxH))
	}
	return w, h
}

// screenAreas waits briefly for Wails to learn the screens - on macOS and
// Linux it does so on the main thread as the app starts, possibly after this
// is asked - and returns their work areas, or nothing if they never arrive.
func screenAreas(app *application.App) []application.Rect {
	for wait := 0; wait < 40; wait++ {
		if screens := app.Screen.GetAll(); len(screens) > 0 {
			areas := make([]application.Rect, 0, len(screens))
			for _, s := range screens {
				areas = append(areas, s.WorkArea)
			}
			return areas
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

// rememberWindow restores the saved size and position once the app has
// started, if they still fit the screens, and saves them again whenever the
// window is moved or resized. Where the desktop decides window positions
// itself - Wayland - only the size comes back.
func rememberWindow(app *application.App, win *application.WebviewWindow, dir string) {
	saved, haveSaved := loadWindowState(dir)

	// Saved a moment after the last move or resize rather than on every one
	// - a drag sends dozens. last is the most recent size and position that
	// wasn't maximised, which is what a maximised window goes back to.
	var mu sync.Mutex
	var pending *time.Timer
	last := saved

	if haveSaved {
		app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
			// Off the thread that delivered the event: moving the window
			// waits for the main thread.
			go func() {
				areas := screenAreas(app)
				if areas == nil {
					return
				}
				w, h := fitSize(saved, areas)
				fits := w == saved.Width && h == saved.Height
				if fits && grabbable(saved, areas) {
					win.SetBounds(application.Rect{X: saved.X, Y: saved.Y, Width: w, Height: h})
				} else {
					win.SetSize(w, h)
					win.Center()
				}
				if saved.Maximised {
					win.Maximise()
				}
			}()
		})
	}

	save := func(*application.WindowEvent) {
		mu.Lock()
		defer mu.Unlock()
		if pending != nil {
			pending.Stop()
		}
		pending = time.AfterFunc(400*time.Millisecond, func() {
			// Full screen is a mode, not a size to come back at.
			if win.IsFullscreen() {
				return
			}
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
