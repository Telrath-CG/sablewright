//go:build !windows

package main

// confirmCancelLabel is the label for Confirm's safe button. macOS and Linux
// always draw the labels they are given.
func confirmCancelLabel() string { return "Cancel" }
