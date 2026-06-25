// Command wh is a personal work-hour tracker. Each invocation opens the SQLite
// database, performs one subcommand, and exits.
package main

import (
	"fmt"
	"os"
	"time"

	"wh/internal/app"
	"wh/internal/store"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stdout, app.Usage())
		return nil
	}

	cmd := args[0]
	rest := args[1:]

	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		fmt.Fprint(os.Stdout, app.Usage())
		return nil
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
		Out:   os.Stdout,
		Err:   os.Stderr,
	}

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
	case "season":
		return a.CmdSeason(rest)
	case "status":
		return a.CmdStatus(rest)
	default:
		fmt.Fprint(os.Stderr, app.Usage())
		return fmt.Errorf("unknown command %q", cmd)
	}
}
