package main

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// TimerState is the painting timer as the page holds it. The timer lives in
// the page - it is not saved anywhere - and the page sends it here whenever it
// starts, pauses, resumes or stops, so that the app can warn before quitting
// with time unlogged, and show the clock in the macOS menu bar.
type TimerState struct {
	Mini      string `json:"mini"`      // the mini it is running for; "" when there is no timer
	Running   bool   `json:"running"`   // false while paused
	ElapsedMs int64  `json:"elapsedMs"` // on the clock when this was sent
	// Seq numbers the reports. Each arrives on a request of its own, so two
	// sent in quick succession - a double-click on Pause - can arrive in
	// either order; an older one is ignored.
	Seq int64 `json:"seq"`
}

// timerTracker is the app's copy of the timer, and the menu-bar item that
// shows it.
//
// Two locks, because the menu-bar calls below wait for the main thread, and
// the main thread reads the timer when asked whether the app may quit: mu
// guards only the state and is never held across such a call; trayMu guards
// the menu-bar item and is never taken on the main thread.
type timerTracker struct {
	mu    sync.Mutex
	state TimerState
	at    time.Time // when state arrived, for working out the time since

	trayMu sync.Mutex
	tray   *application.SystemTray
	menu   *application.Menu
	header *application.MenuItem
	toggle *application.MenuItem
	ticker *time.Ticker
	done   chan struct{}
}

// active reports whether there is a timer, running or paused.
func (t *timerTracker) active() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state.Mini != ""
}

// snapshot is the timer now: its state and the time on its clock.
func (t *timerTracker) snapshot() (TimerState, time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state, t.elapsedLocked()
}

func (t *timerTracker) elapsedLocked() time.Duration {
	d := time.Duration(t.state.ElapsedMs) * time.Millisecond
	if t.state.Running {
		d += time.Since(t.at)
	}
	return d
}

// hoursMinutes is a duration as the menu bar shows it: 0:05, 1:30.
func hoursMinutes(d time.Duration) string {
	m := int(d / time.Minute)
	return fmt.Sprintf("%d:%02d", m/60, m%60)
}

// TimerChanged is how the page reports the timer. It is called with an empty
// Mini when the timer stops.
func (a *App) TimerChanged(s TimerState) {
	t := &a.timer
	t.mu.Lock()
	if s.Seq != 0 && s.Seq < t.state.Seq {
		t.mu.Unlock()
		return
	}
	t.state, t.at = s, time.Now()
	t.mu.Unlock()
	if runtime.GOOS == "darwin" {
		a.updateMenuBarTimer()
	}
}

// mayQuit answers whether the app can quit now. With a timer going and the
// question not yet answered, it says no and asks - after returning, because
// Wails calls this on the main thread, where a dialog waiting for its answer
// would wait for ever.
func (a *App) mayQuit() bool {
	if a.quitConfirmed.Load() || !a.timer.active() {
		return true
	}
	go a.askToQuit()
	return false
}

// askToQuit asks whether to quit and lose the timer's time, and quits if the
// answer is yes. Only one question is ever on screen.
func (a *App) askToQuit() {
	if !a.askingToQuit.CompareAndSwap(false, true) {
		return
	}
	defer a.askingToQuit.Store(false)
	a.bringForward()
	s, elapsed := a.timer.snapshot()
	how := "running"
	if !s.Running {
		how = "paused"
	}
	ok, _ := a.Confirm("A painting timer is "+how,
		fmt.Sprintf("%s on %s hasn't been logged yet, and will be lost if you quit.\n\nQuit anyway?",
			hoursMinutes(elapsed), s.Mini),
		"Quit")
	if ok {
		a.quitConfirmed.Store(true)
		a.app.Quit()
	}
}

// bringForward shows the window and puts it in front.
func (a *App) bringForward() {
	if w := a.win.Load(); w != nil {
		w.UnMinimise()
		w.Show()
		w.Focus()
	}
}

// updateMenuBarTimer shows the timer as an item in the macOS menu bar while
// there is one, and takes it away when there isn't. The item's menu pauses,
// resumes and stops the timer the same way the sidebar does, by asking the
// page to.
//
// macOS only: on Windows a tray item's text is only a tooltip, and on Linux
// most desktops don't show it at all. Called off the main thread.
func (a *App) updateMenuBarTimer() {
	t := &a.timer
	// The state is read once trayMu is held, so two reports handled at once
	// redraw the menu bar in the order they took it, each with the newest
	// state - never an older one landing last.
	t.trayMu.Lock()
	defer t.trayMu.Unlock()
	state, label := t.stateAndLabel()
	if state.Mini == "" {
		if t.tray != nil {
			t.tray.Hide()
		}
		t.stopTicking()
		return
	}
	if t.tray == nil {
		t.menu = application.NewMenu()
		t.header = t.menu.Add("").SetEnabled(false)
		t.toggle = t.menu.Add("Pause").OnClick(func(*application.Context) {
			a.app.Event.Emit("timer", "toggle")
		})
		t.menu.Add("Stop and log…").OnClick(func(*application.Context) {
			a.bringForward()
			a.app.Event.Emit("timer", "stop")
		})
		t.menu.AddSeparator()
		t.menu.Add("Show Sablewright").OnClick(func(*application.Context) { a.bringForward() })
		t.tray = a.app.SystemTray.New()
		t.tray.SetMenu(t.menu)
	}
	toggle := "Pause"
	if !state.Running {
		toggle = "Resume"
	}
	// A menu item's label is set straight on the native item, so on the
	// main thread; Update then redraws the menu, and does its own hop there.
	application.InvokeSync(func() {
		t.header.SetLabel(state.Mini)
		t.toggle.SetLabel(toggle)
	})
	t.menu.Update()
	t.tray.SetLabel(label)
	t.tray.SetTooltip("Painting " + state.Mini)
	t.tray.Show()
	if state.Running {
		t.startTicking()
	} else {
		t.stopTicking()
	}
}

// stateAndLabel is the timer's state and its menu-bar text: the time, with a
// pause sign while paused.
func (t *timerTracker) stateAndLabel() (TimerState, string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	sign := "◷ "
	if !t.state.Running {
		sign = "⏸ "
	}
	return t.state, sign + hoursMinutes(t.elapsedLocked())
}

// startTicking keeps the menu-bar clock current while the timer runs. It only
// shows minutes, so it is redrawn every few seconds rather than every second.
// Called with trayMu held.
func (t *timerTracker) startTicking() {
	if t.ticker != nil {
		return
	}
	t.ticker = time.NewTicker(5 * time.Second)
	t.done = make(chan struct{})
	tick, done := t.ticker.C, t.done
	go func() {
		for {
			select {
			case <-tick:
				// The label is read under trayMu, and only written while this
				// ticker is still the current one, so a tick can't overwrite
				// the pause sign a pause has just put up.
				t.trayMu.Lock()
				if t.done == done && t.tray != nil {
					if state, label := t.stateAndLabel(); state.Mini != "" {
						t.tray.SetLabel(label)
					}
				}
				t.trayMu.Unlock()
			case <-done:
				return
			}
		}
	}()
}

// stopTicking stops the menu-bar clock. Called with trayMu held.
func (t *timerTracker) stopTicking() {
	if t.ticker == nil {
		return
	}
	t.ticker.Stop()
	close(t.done)
	t.ticker, t.done = nil, nil
}
