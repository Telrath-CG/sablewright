package main

import (
	"os"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestWindowStateRoundTripsAndRejectsNonsense(t *testing.T) {
	dir := t.TempDir()
	want := windowState{X: 120, Y: 80, Width: 1300, Height: 820, Maximised: true}
	if err := saveWindowState(dir, want); err != nil {
		t.Fatal(err)
	}
	if got, ok := loadWindowState(dir); !ok || got != want {
		t.Errorf("loaded %+v, %v; want %+v", got, ok, want)
	}
	for _, bad := range []string{`{"width":10,"height":10}`, `not json`, `{"width":99999,"height":800}`} {
		if err := os.WriteFile(windowStatePath(dir), []byte(bad), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, ok := loadWindowState(dir); ok {
			t.Errorf("accepted %s", bad)
		}
	}
	if _, ok := loadWindowState(t.TempDir()); ok {
		t.Error("a missing file counted as a saved window")
	}
}

// A window saved on a monitor that has since been unplugged must not come
// back where it can't be reached.
func TestGrabbableNeedsTheTopEdgeOnAScreen(t *testing.T) {
	laptop := []application.Rect{{X: 0, Y: 25, Width: 1512, Height: 920}}
	both := append(laptop, application.Rect{X: 1512, Y: 0, Width: 2560, Height: 1415})
	onSecond := windowState{X: 2000, Y: 100, Width: 1200, Height: 800}
	if !grabbable(onSecond, both) {
		t.Error("a window on a connected second screen was called unreachable")
	}
	if grabbable(onSecond, laptop) {
		t.Error("a window on an unplugged screen was called reachable")
	}
	if grabbable(windowState{X: 100, Y: 930, Width: 1200, Height: 800}, laptop) {
		t.Error("a window with its title bar below the screen was called reachable")
	}
	if !grabbable(windowState{X: -1000, Y: 100, Width: 1200, Height: 800}, laptop) {
		t.Error("a window hanging off the left with 200px showing was called unreachable")
	}
}

// A window last sized on a bigger monitor than any still connected shrinks to
// fit, never below the minimum.
func TestFitSizeShrinksToTheLargestScreen(t *testing.T) {
	laptop := []application.Rect{{X: 0, Y: 25, Width: 1512, Height: 920}}
	if w, h := fitSize(windowState{Width: 3000, Height: 1800}, laptop); w != 1512 || h != 920 {
		t.Errorf("fitSize = %dx%d, want 1512x920", w, h)
	}
	if w, h := fitSize(windowState{Width: 1200, Height: 800}, laptop); w != 1200 || h != 800 {
		t.Errorf("fitSize = %dx%d, want the saved 1200x800 unchanged", w, h)
	}
	tiny := []application.Rect{{Width: 800, Height: 500}}
	if w, h := fitSize(windowState{Width: 1200, Height: 800}, tiny); w != minWidth || h != minHeight {
		t.Errorf("fitSize = %dx%d, want the minimum %dx%d", w, h, minWidth, minHeight)
	}
}
