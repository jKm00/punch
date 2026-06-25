// Package app implements the punch subcommands on top of the store. It contains
// the command dispatch and all the user-facing behavior.
package app

import (
	"fmt"
	"io"
	"time"

	"punch/internal/store"
	"punch/internal/ui"
)

// App carries the dependencies shared by all command handlers.
type App struct {
	Store *store.Store
	Now   func() time.Time
	Loc   *time.Location
	Out   io.Writer
	Err   io.Writer
	UI    *ui.Styler
}

// styler returns the App's styler, defaulting to a disabled (plain) styler when
// none was provided (e.g. in tests).
func (a *App) styler() *ui.Styler {
	if a.UI == nil {
		return ui.New(false)
	}
	return a.UI
}

// now returns the current time in the app's location.
func (a *App) now() time.Time { return a.Now().In(a.Loc) }

// dateOnly truncates t to midnight in the app's location.
func (a *App) dateOnly(t time.Time) time.Time {
	t = t.In(a.Loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, a.Loc)
}

func (a *App) printf(format string, args ...any) {
	fmt.Fprintf(a.Out, format, args...)
}

func (a *App) errorf(format string, args ...any) {
	fmt.Fprintf(a.Err, format, args...)
}

// reorderArgs moves any leading positional arguments (those appearing before
// the first flag token) to the end, so the stdlib flag package — which stops
// parsing at the first non-flag argument — still sees the flags. punch's grammar
// only ever places positionals first (e.g. `punch set DATE --start ...`), so this
// is safe and unambiguous.
func reorderArgs(args []string) []string {
	var leading []string
	i := 0
	for i < len(args) {
		if len(args[i]) > 0 && args[i][0] == '-' {
			break
		}
		leading = append(leading, args[i])
		i++
	}
	if len(leading) == 0 {
		return args
	}
	out := make([]string, 0, len(args))
	out = append(out, args[i:]...)
	out = append(out, leading...)
	return out
}
