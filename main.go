package main

import (
	"embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

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

func main() {
	app := NewApp()

	store, err := NewStore()
	if err != nil {
		// A store that won't open is fatal, but the message must reach the
		// user - with no console attached, printing alone would vanish.
		fatal("Sablewright could not start.\n\n" + err.Error())
		return
	}
	app.store = store

	err = wails.Run(&options.App{
		Title:     "Sablewright",
		Width:     1180,
		Height:    760,
		MinWidth:  940,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: photoHandler{dir: store.PhotoDir()},
		},
		BackgroundColour: &options.RGBA{R: 238, G: 241, B: 245, A: 1},
		OnStartup:        app.startup,
		Bind:             []interface{}{app},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
		// An opaque window matters here: some Linux compositors give the
		// webview an RGBA visual by default, which makes the whole UI look
		// washed out because it blends with whatever is behind it.
		Linux: &linux.Options{
			Icon:                icon,
			WindowIsTranslucent: false,
			WebviewGpuPolicy:    linux.WebviewGpuPolicyOnDemand,
			ProgramName:         "Sablewright",
		},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarDefault(),
			About: &mac.AboutInfo{
				Title:   "Sablewright",
				Message: "Track your miniature painting projects.",
				Icon:    icon,
			},
		},
	})
	if err != nil {
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
