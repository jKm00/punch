// Command punch is a personal work-hour tracker. Each invocation opens the
// SQLite database, performs one subcommand, and exits.
package main

import (
	"fmt"
	"os"
	"time"

	"punch/internal/app"
	"punch/internal/selfupdate"
	"punch/internal/store"
	"punch/internal/ui"
)

// version is the binary version. It is "dev" for local builds and is stamped at
// release time via -ldflags "-X main.version=vX.Y.Z".
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// extractNoColor removes a global `--no-color` token from anywhere in args and
// reports whether it was present. Keeping it global means it works regardless
// of subcommand position.
func extractNoColor(args []string) ([]string, bool) {
	out := make([]string, 0, len(args))
	found := false
	for _, a := range args {
		if a == "--no-color" || a == "-no-color" {
			found = true
			continue
		}
		out = append(out, a)
	}
	return out, found
}

func run(args []string) error {
	args, noColor := extractNoColor(args)

	if len(args) == 0 {
		fmt.Fprint(os.Stdout, app.Usage())
		return nil
	}

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "help", "-h", "--help":
		fmt.Fprint(os.Stdout, app.Usage())
		return nil
	case "version", "--version", "-v":
		fmt.Fprintf(os.Stdout, "punch %s\n", version)
		return nil
	}

	colorOn := ui.ShouldEnable(os.Stdout, noColor)
	styler := ui.New(colorOn)

	// Show any pending "new version" notice (cheap, no network) before running.
	if notice := selfupdate.PendingNotice(version); notice != "" {
		fmt.Fprintln(os.Stderr, styler.Yellow(notice))
	}

	// `upgrade` does not need the database.
	if cmd == "upgrade" {
		return cmdUpgrade(styler)
	}

	path, err := store.DefaultPath()
	if err != nil {
		return err
	}
	loc := time.Local
	st, err := store.Open(path, loc)
	if err != nil {
		return err
	}
	defer st.Close()

	a := &app.App{
		Store: st,
		Now:   time.Now,
		Loc:   loc,
		In:    os.Stdin,
		Out:   os.Stdout,
		Err:   os.Stderr,
		UI:    styler,
	}

	// Load the resolved configuration (stored values, falling back to the
	// domain constants for any unset key) for this invocation.
	cfg, err := st.LoadConfig()
	if err != nil {
		return err
	}
	a.Config = cfg

	runErr := dispatch(a, cmd, rest)

	// Refresh the cached latest-version (at most once/day) without delaying the
	// user: run it in the background and wait only briefly. If it does not
	// finish in time, the process exits and the check simply happens next time.
	backgroundRefresh()

	return runErr
}

func dispatch(a *app.App, cmd string, rest []string) error {
	switch cmd {
	case "in":
		return a.CmdIn(rest)
	case "out":
		return a.CmdOut(rest)
	case "set":
		return a.CmdSet(rest)
	case "off":
		return a.CmdOff(rest)
	case "clear":
		return a.CmdClear(rest)
	case "week":
		return a.CmdWeek(rest)
	case "unlogged":
		return a.CmdUnlogged(rest)
	case "log":
		return a.CmdLog(rest)
	case "setup":
		return a.CmdSetup(rest)
	case "status":
		return a.CmdStatus(rest)
	case "analytics":
		return a.CmdAnalytics(rest)
	default:
		fmt.Fprint(os.Stderr, app.Usage())
		return fmt.Errorf("unknown command %q", cmd)
	}
}

// backgroundRefresh kicks off the daily update check and waits up to a short
// grace period so it usually completes without making the CLI feel slow.
func backgroundRefresh() {
	done := make(chan struct{})
	go func() {
		selfupdate.MaybeRefresh(version)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(1500 * time.Millisecond):
	}
}

// cmdUpgrade runs the self-update flow.
func cmdUpgrade(styler *ui.Styler) error {
	res, err := selfupdate.Upgrade(version, func(msg string) {
		fmt.Fprintln(os.Stderr, styler.Dim(msg))
	})
	if err != nil {
		if err == selfupdate.ErrUpToDate {
			fmt.Fprintf(os.Stdout, "punch is already up to date (%s).\n", version)
			return nil
		}
		return err
	}
	fmt.Fprintf(os.Stdout, "%s upgraded punch %s → %s\n", styler.Green("✓"), res.From, res.To)
	return nil
}
