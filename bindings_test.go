package main

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"testing"
)

// Every call the page makes to Go goes through App().Name. A method renamed or
// removed on the Go side compiles fine and leaves whatever button calls it
// dead, so the page's calls are checked against *App here instead.
func TestEveryBackendCallInTheFrontendExists(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("frontend", "dist", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(`App\(\)\.([A-Za-z_]\w*)`).FindAllStringSubmatch(string(src), -1) {
		seen[m[1]] = true
	}
	// A sanity floor: if the call style changed, matching nothing would pass.
	if len(seen) < 20 {
		t.Fatalf("found only %d backend calls in app.js; has the way it calls Go changed?", len(seen))
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	typ := reflect.TypeOf(&App{})
	for _, n := range names {
		if _, ok := typ.MethodByName(n); !ok {
			t.Errorf("app.js calls App().%s, which *App doesn't have", n)
		}
	}
}
