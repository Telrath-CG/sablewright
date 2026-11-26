//go:build windows

package main

// confirmCancelLabel is the label for Confirm's safe button on Windows.
//
// Wails draws the given labels with a task dialog, but falls back to a fixed
// Yes/No message box in two cases: when the .exe lacks the common-controls
// manifest, and when the task dialog itself fails to open. The fallback
// pre-selects the safe button only when it is labelled exactly "No" - with
// any other label, Enter presses Yes and the delete goes ahead. The second
// case can't be predicted before the dialog is shown, so the safe button is
// always "No" on Windows, as it was in 1.x.
func confirmCancelLabel() string { return "No" }
