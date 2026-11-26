package main

import "github.com/wailsapp/wails/v3/pkg/application"

// The native dialogs, in one place. Each platform reports the same outcome in
// its own way, and this is where those differences are smoothed over so the
// rest of the app can treat a dialog as a question with an answer.

// fileFilter is one entry in a file dialog's type list: a label and a
// semicolon-separated list of patterns, such as "*.png;*.jpg".
type fileFilter struct{ name, pattern string }

// dialogCancelled reports whether a file dialog's error only means it was
// closed without choosing anything. Windows reports that as an error, where
// macOS and Linux return an empty result; left as an error, every cancelled
// dialog on Windows would show up in the app as a failure.
//
// Wails keeps that error in an internal package, so it can only be recognised
// by its text.
func dialogCancelled(err error) bool {
	return err != nil && err.Error() == "cancelled by user"
}

// attach shows a dialog over the main window, which on macOS makes it a sheet
// on that window rather than a free-floating panel.
func (a *App) attach() application.Window {
	if a.win == nil {
		return nil
	}
	return a.win
}

// openFiles asks for one or more existing files. No files and no error means
// the dialog was cancelled.
func (a *App) openFiles(title string, filters ...fileFilter) ([]string, error) {
	d := a.app.Dialog.OpenFile().SetTitle(title).CanChooseFiles(true).CanChooseDirectories(false)
	if w := a.attach(); w != nil {
		d.AttachToWindow(w)
	}
	for _, f := range filters {
		d.AddFilter(f.name, f.pattern)
	}
	paths, err := d.PromptForMultipleSelection()
	if dialogCancelled(err) {
		return nil, nil
	}
	return paths, err
}

// openFile asks for one existing file. An empty path and no error means the
// dialog was cancelled.
func (a *App) openFile(title string, filters ...fileFilter) (string, error) {
	d := a.app.Dialog.OpenFile().SetTitle(title).CanChooseFiles(true).CanChooseDirectories(false)
	if w := a.attach(); w != nil {
		d.AttachToWindow(w)
	}
	for _, f := range filters {
		d.AddFilter(f.name, f.pattern)
	}
	path, err := d.PromptForSingleSelection()
	if dialogCancelled(err) {
		return "", nil
	}
	return path, err
}

// saveFile asks where to save a file, suggesting a name. An empty path and no
// error means the dialog was cancelled.
//
// The path comes back exactly as the dialog gave it: which filter was chosen
// is not reported, and macOS ignores the filters entirely, so callers that
// care about the extension set it themselves (see withExtension).
func (a *App) saveFile(title, filename string, filters ...fileFilter) (string, error) {
	// SetOptions rather than the builder methods, because the save dialog's
	// builder has no title setter and Windows and Linux show only the title.
	// SetOptions replaces every field, so the filters and window go in too.
	opts := &application.SaveFileDialogOptions{
		Title:                title,
		Message:              title,
		Filename:             filename,
		CanCreateDirectories: true,
		Window:               a.attach(),
	}
	for _, f := range filters {
		opts.Filters = append(opts.Filters, application.FileFilter{DisplayName: f.name, Pattern: f.pattern})
	}
	d := a.app.Dialog.SaveFile()
	d.SetOptions(opts)
	path, err := d.PromptForSingleSelection()
	if dialogCancelled(err) {
		return "", nil
	}
	return path, err
}

// Confirm shows a native question with two buttons, the action and Cancel,
// and reports whether the action was chosen. It is how deletes and other
// things that can't be undone are checked.
//
// Cancel is both the default button and the cancel button, so Enter means no
// everywhere, and so does Escape or closing the dialog wherever the platform
// offers them. Setting a cancel button also matters for a second reason: on
// Linux, closing a dialog that has none answers nothing at all, and this would
// wait for ever.
func (a *App) Confirm(title, message, okLabel string) (bool, error) {
	if okLabel == "" {
		okLabel = "OK"
	}
	// Buffered and never blocking, so a platform that reports a click twice
	// can't leave a callback stuck on a send nobody receives.
	answer := make(chan bool, 1)
	reply := func(yes bool) func() {
		return func() {
			select {
			case answer <- yes:
			default:
			}
		}
	}
	d := a.app.Dialog.Question().SetTitle(title).SetMessage(message)
	if w := a.attach(); w != nil {
		d.AttachToWindow(w)
	}
	d.AddButton(okLabel).OnClick(reply(true))
	d.AddButton(confirmCancelLabel()).OnClick(reply(false)).SetAsDefault().SetAsCancel()
	// Show returns at once on macOS and Linux and only once answered on
	// Windows; the channel waits the same way on all three.
	d.Show()
	return <-answer, nil
}

// Info shows a native message with a single button.
func (a *App) Info(title, message string) {
	d := a.app.Dialog.Info().SetTitle(title).SetMessage(message)
	if w := a.attach(); w != nil {
		d.AttachToWindow(w)
	}
	d.Show()
}
