//go:build linux

package main

import "github.com/godbus/dbus/v5"

// singleInstanceUsable reports whether Wails' single-instance lock can work.
// On Linux the lock is a name claimed on the session D-Bus, and where there is
// no session bus - some minimal desktops, a bare X session, a remote one -
// Wails treats the failure to claim it as fatal and the app never opens. It
// runs without the lock there instead, as it always has.
func singleInstanceUsable() bool {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
