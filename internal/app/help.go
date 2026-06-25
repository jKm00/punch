package app

const usageText = `wh — workhour tracker

USAGE
  wh <command> [args] [--no-color]

COMMANDS
  in    [DATE] [--at HH:MM] [--force]   Clock in. Bare now-case records (now - 5min).
  out   [DATE] [--at HH:MM] [--force]   Clock out. Bare now-case records (now + 5min).
  set   DATE --start HH:MM --end HH:MM [--lunch DUR] [--expected DUR]
                                        Backfill/overwrite a whole day (prints before→after).
  off   DATE [--clear]                  Mark a day off (or clear it with --clear).
  clear DATE                            Delete a day's record entirely.
  week  [N|last] [--year YYYY]          Week summary (bare = current week).
  unlogged                              List past unlogged weeks that have worked time.
  log   [N|last] [--year YYYY]          Mark a week logged (bare = current week).
  season [summer|winter]                Print or set the season.
  status                                Show clock-in state and time so far today.
  analytics [YEAR]                      Yearly dashboard (default: current year).
  help                                  This help.

DATE FORMATS (day-first European)
  today, yesterday, DD.MM.YYYY, DD-MM-YYYY, DD.MM, DD-MM (year defaults to current)
  Times are 24-hour HH:MM. Durations: 30m, 7h, 7h30m, or HH:MM.

RULES & CONSTANTS
  Lunch default 30m (deducted every clocked day).
  Expected/day: winter 7h30m, summer 7h.
  Logging range starts: winter 16:00, summer 15:30.
  Clock adjustment ±5min on bare in/out. --at and explicit times are literal.
  No overnight shifts. Future times require --force.

OUTPUT
  Color and boxes are shown only on an interactive terminal. Output is plain
  text when piped/redirected, when NO_COLOR is set, or with --no-color.
`

// Usage returns the help text.
func Usage() string { return usageText }

// CmdHelp prints usage.
func (a *App) CmdHelp() {
	a.printf("%s", usageText)
}
