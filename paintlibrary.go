package main

import (
	_ "embed"
	"encoding/json"
	"sync"
)

// The built-in paint library: the five ranges every new collection starts with.
// It lives in a JSON file rather than a Go literal purely because there are
// over a thousand entries - a table that size is easier to regenerate and
// easier to read as data than as source.
//
// Names, product codes and range names come from each manufacturer's own
// catalogue; the hex values are the swatch colors published by the community
// paint databases at github.com/Arcturus5404/miniature-paints (MIT),
// miniaturepaintingforge.com and thepaintingledger.com. They are close enough
// to pick a pot out of a rack by eye, which is all the swatch is for - a paint
// on a model never matches its label anyway.
//
// Most Citadel Air paints share a name with a Base or Layer paint, so they
// carry the suffix the pot does: "The Fang - Air". Without it the rack would
// show two identical-looking rows and no way to tell which one you own.
//
// Known gaps, so nobody goes hunting for a bug that isn't one: the Citadel
// spray cans are left out, and so are Two Thin Coats' six effect paints, for
// which no source lists a color. Both can still be added by hand like any
// other paint.
//
//go:embed paintlibrary.json
var paintLibraryJSON []byte

var (
	libraryOnce  sync.Once
	libraryCache []Paint
)

// PaintLibrary returns the built-in paints. The slice is shared, so callers
// must copy anything they intend to modify - which SeedPaints does by value.
func PaintLibrary() []Paint {
	libraryOnce.Do(func() {
		// The file ships inside the binary and is checked by a test, so a
		// parse failure is a build problem rather than something a user can
		// hit; an empty library degrades to "the rack starts empty".
		_ = json.Unmarshal(paintLibraryJSON, &libraryCache)
	})
	return libraryCache
}

// LibraryBrands lists the brands in the built-in library, in the order they're
// introduced to the user.
var LibraryBrands = []string{
	"Warhammer Colour", "Pro Acryl", "Ionic Smart Colors",
	"AK Interactive", "Two Thin Coats",
}

// RenamedBrands maps brands that have since been renamed by their maker onto
// the name the library uses, so an older collection doesn't end up listing the
// same paints twice under two spellings.
var RenamedBrands = map[string]string{
	"Citadel":        "Warhammer Colour",
	"Citadel Colour": "Warhammer Colour",
}
