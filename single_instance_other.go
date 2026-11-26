//go:build !linux

package main

// singleInstanceUsable reports whether Wails' single-instance lock can work.
// On Windows and macOS it needs nothing that might be missing.
func singleInstanceUsable() bool { return true }
