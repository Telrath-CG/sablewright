package main

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

// Stamped in by the linker at build time - see the -ldflags in
// .github/workflows/build.yml and both build scripts. The number itself comes
// from info.version in build/config.yml, which the build also stamps into each
// platform's packaging, so there is one place to bump rather than several that
// drift apart. These defaults are what an unstamped `go build` leaves behind.
var (
	version = "dev"
	commit  = ""
)

// photoHandler serves the user's photos to the webview from the data folder.
// Embedding them as base64 would balloon memory; this keeps them on disk and
// streams them like any other static file.
type photoHandler struct{ dir string }

func (h photoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/photos/")
	if name == "" || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	p := filepath.Join(h.dir, filepath.Base(name))
	f, err := os.Open(p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	switch strings.ToLower(filepath.Ext(p)) {
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".gif":
		w.Header().Set("Content-Type", "image/gif")
	case ".webp":
		w.Header().Set("Content-Type", "image/webp")
	case ".bmp":
		w.Header().Set("Content-Type", "image/bmp")
	default:
		w.Header().Set("Content-Type", "image/jpeg")
	}
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = io.Copy(w, f)
}

// appID names Sablewright to the operating system: the single-instance lock,
// the GTK application on Linux, and the bundle identifier in build/config.yml.
const appID = "io.github.telrath-cg.sablewright"

// photoRoutes sends /photos/ to the photo handler and everything else on to
// the app's own files. Wails v3 has no fallback handler for paths its assets
// don't cover, so the photos are routed explicitly, ahead of the assets.
func photoRoutes(photos http.Handler) application.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/photos/") {
				photos.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func main() {
	app := NewApp()

	// A second launch stops inside New and hands over to this one, so it
	// never gets as far as opening the collection below. Where the lock
	// can't work at all (Linux without a session bus) the app runs without
	// it, as v2 did, rather than not at all.
	var single *application.SingleInstanceOptions
	if singleInstanceUsable() {
		single = &application.SingleInstanceOptions{
			UniqueID:               appID,
			OnSecondInstanceLaunch: app.onSecondInstance,
		}
	}

	wails := application.New(application.Options{
		Name: "Sablewright",
		// Quitting with a painting timer going asks first: the timer is
		// held only in memory, and its time would be lost.
		ShouldQuit:  app.mayQuit,
		Description: "Track your miniature painting projects.",
		Icon:        icon,
		Services:    []application.Service{application.NewService(app)},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: photoRoutes(photoHandler{dir: filepath.Join(DataDir(), "photos")}),
		},
		SingleInstance: single,
		// Wails sends its errors here, and its logger writes nowhere in a
		// release build, so without this a window that couldn't be created
		// would just never appear. A fatal error - after which Wails exits -
		// is shown the way a collection that won't open is: a log beside the
		// data, opened. So is any error before the page has loaded, which is
		// the window failing to come up: on Windows, WebView2 failing to
		// start is reported as a plain error and followed by an exit. Errors
		// once the app is running are only logged.
		ErrorHandler: func(err error) {
			var fe *application.FatalError
			if errors.As(err, &fe) {
				fatal("Sablewright hit a problem and had to stop.\n\n" + fe.Unwrap().Error())
				return
			}
			if !app.pageReady.Load() {
				fatal("Sablewright could not open its window.\n\n" + err.Error())
				return
			}
			logError(err.Error())
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		// ProgramName is what the window manager groups the window under, so
		// it has to match StartupWMClass in build/linux/sablewright.desktop.
		Linux: application.LinuxOptions{
			ProgramName:   "Sablewright",
			ApplicationID: appID,
		},
	})
	app.app = wails

	// macOS gets the menus every Mac app has - the app menu, Edit for copy
	// and paste in text fields, and Window - plus File and View with
	// Sablewright's own commands. Left to itself Wails adds a View menu that
	// reloads the page, losing a running timer, and a Help item that replaces
	// this window with the Wails website. Windows and Linux keep their
	// windows as they are, with the same shortcuts as key bindings.
	var keys map[string]func(application.Window)
	if runtime.GOOS == "darwin" {
		wails.Menu.Set(app.appMenu())
	} else {
		keys = app.keyBindings()
	}
	app.contextMenus()

	// Opens at the size it was last left at; rememberWindow moves it back to
	// where it was once the screens are known.
	width, height := 1180, 760
	if saved, ok := loadWindowState(DataDir()); ok {
		width, height = saved.Width, saved.Height
	}

	// Nothing is drawn until Run, by which time the store is in place.
	win := wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Sablewright",
		Width:            width,
		Height:           height,
		MinWidth:         minWidth,
		MinHeight:        minHeight,
		BackgroundColour: application.NewRGB(238, 241, 245),
		KeyBindings:      keys,
		// Pictures dropped on a mini's photos are added to it. Only the
		// elements marked data-file-drop-target take a drop.
		EnableFileDrop: true,
	})
	win.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		app.pageReady.Store(true)
	})
	win.OnWindowEvent(events.Common.WindowFilesDropped, app.onFilesDropped)
	// Closing the window with a timer going asks first, as quitting does.
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if !app.mayQuit() {
			e.Cancel()
		}
	})
	rememberWindow(wails, win, DataDir())
	app.win.Store(win)

	store, warning, err := NewStore()
	if err != nil {
		// A store that won't open is fatal, but the message must reach the
		// user - with no console attached, printing alone would vanish.
		fatal("Sablewright could not start.\n\n" + err.Error())
		return
	}
	app.store = store
	app.loadErr = warning

	if err := wails.Run(); err != nil {
		fatal("Sablewright hit a problem and had to stop.\n\n" + err.Error())
	}
}

// errorLogPath is where problems are written down, beside the data folder.
func errorLogPath() string { return filepath.Join(DataDir(), "error_log.txt") }

// logError appends a problem to the error log without showing it.
func logError(msg string) {
	_ = os.MkdirAll(DataDir(), 0o755)
	if f, err := os.OpenFile(errorLogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintf(f, "%s\n%s %s\n\n", strings.Repeat("=", 60), time.Now().Format(time.RFC3339), msg)
		f.Close()
	}
	fmt.Fprintln(os.Stderr, msg)
}

// showLog opens the error log once, however many failures arrive on the way
// down.
var showLog sync.Once

// fatal reports a failure the app can't run past. It writes it to the log
// and opens that, so a double-clicked app never just disappears silently.
func fatal(msg string) {
	logError(msg)
	showLog.Do(func() { _ = openPath(errorLogPath()) })
}
