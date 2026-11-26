//go:build windows

package main

import "github.com/wailsapp/wails/v3/pkg/w32"

// confirmCancelLabel is the label for Confirm's safe button on Windows.
//
// Wails draws a confirm with the button labels it is given only when version 6
// of the common controls is available, which takes the manifest compiled into
// the .exe. Without it - a plain `go build` that skipped the .syso - Wails
// falls back to a fixed Yes/No box and pre-selects No only when the cancel
// button is labelled exactly that. Every release build carries the manifest;
// this keeps a build without one from making Delete the button Enter presses.
func confirmCancelLabel() string {
	if w32.TaskDialogAvailable() {
		return "Cancel"
	}
	return "No"
}
