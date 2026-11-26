package main

import (
	"embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

// Stamped in by the linker at build time - see the -ldflags in
// .github/workflows/build.yml and both build scripts. The number itself comes
// from wails.json, which Wails already uses for the Windows file properties,
// so there is one place to bump rather than several that drift apart. These
// defaults are what an unstamped `go build` leaves behind.
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
const appID = "net.dynamicsyntax.sablewright"

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

	wails := application.New(application.Options{
		Name:        "Sablewright",
		Description: "Track your miniature painting projects.",
		Icon:        icon,
		Services:    []application.Service{application.NewService(app)},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: photoRoutes(photoHandler{dir: filepath.Join(DataDir(), "photos")}),
		},
		// A second launch stops inside New and hands over to this one, so
		// it never gets as far as opening the collection below.
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:               appID,
			OnSecondInstanceLaunch: app.onSecondInstance,
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

	store, warning, err := NewStore()
	if err != nil {
		// A store that won't open is fatal, but the message must reach the
		// user - with no console attached, printing alone would vanish.
		fatal("Sablewright could not start.\n\n" + err.Error())
		return
	}
	app.store = store
	app.loadErr = warning

	app.win = wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Sablewright",
		Width:            1180,
		Height:           760,
		MinWidth:         940,
		MinHeight:        600,
		BackgroundColour: application.NewRGB(238, 241, 245),
		Linux: application.LinuxWindow{
			Icon:             icon,
			WebviewGpuPolicy: application.WebviewGpuPolicyOnDemand,
		},
	})

	if err := wails.Run(); err != nil {
		fatal("Sablewright hit a problem and had to stop.\n\n" + err.Error())
	}
}

// fatal reports a startup failure. It writes a log next to the data folder and
// tries to show it, so a double-clicked app never just disappears silently.
func fatal(msg string) {
	logPath := filepath.Join(DataDir(), "error_log.txt")
	_ = os.MkdirAll(DataDir(), 0o755)
	if f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintf(f, "%s\n%s\n\n", strings.Repeat("=", 60), msg)
		f.Close()
	}
	fmt.Fprintln(os.Stderr, msg)
	_ = openPath(logPath)
}
