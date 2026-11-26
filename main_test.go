package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// Wails v3 has no fallback for paths its assets don't cover, so /photos/ is
// routed by hand: photos from the data folder, everything else to the app's
// own files.
func TestPhotoRoutesSendsPhotosToTheDataFolder(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "m1_2.png"), []byte("png bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	assets := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("asset " + r.URL.Path))
	})
	h := photoRoutes(photoHandler{dir: dir})(assets)

	for path, want := range map[string]string{
		"/photos/m1_2.png":  "png bytes",
		"/index.html":       "asset /index.html",
		"/wails/runtime.js": "asset /wails/runtime.js",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if got := rec.Body.String(); got != want {
			t.Errorf("GET %s = %q, want %q", path, got, want)
		}
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/photos/../collection.json", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("a path climbing out of photos/ got %d, want 404", rec.Code)
	}
}

// Windows reports a cancelled file dialog as an error; it has to read as a
// cancel, or every closed dialog shows up as a failure.
func TestDialogCancelledRecognisesOnlyACancel(t *testing.T) {
	if !dialogCancelled(errors.New("cancelled by user")) {
		t.Error("the Windows cancel wasn't recognised")
	}
	for _, err := range []error{nil, errors.New("access denied")} {
		if dialogCancelled(err) {
			t.Errorf("dialogCancelled(%v) = true, want false", err)
		}
	}
}
