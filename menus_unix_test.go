//go:build !windows

package main

import (
	"path/filepath"
	"syscall"
	"testing"
)

// A named pipe would hang the copy for ever, whatever it is called.
func TestDroppablePictureRefusesAPipe(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "pipe.png")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skip("can't make a named pipe here:", err)
	}
	if droppablePicture(fifo) {
		t.Error("a named pipe was taken")
	}
}
