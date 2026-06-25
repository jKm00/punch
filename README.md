# wh — workhour tracker

`wh` is a command-line tool for fast work-hour logging: run a command when you
arrive, another when you leave, then check a weekly summary to log overtime in
the company's official app.

## Installation

Requires **Go 1.25+** and `~/.local/bin` on your `$PATH`.

```sh
make install      # builds and installs to ~/.local/bin/wh
wh help           # verify
```

## Development

Copy the env template, then run from source via `make dev`. `.env` sets
`WH_DB=./dev.db` so dev runs use a separate, gitignored database.

```sh
cp .env.example .env

make dev ARGS="in"
make dev ARGS="week last"
make dev ARGS='set 15.02 --start 08:00 --end 16:00'

make dev-reset    # delete the dev database
make test         # run tests
```

## Rules & constants

These are hardcoded:

| Constant                     | Value                            |
| ---------------------------- | -------------------------------- |
| Default lunch                | 30m (deducted every clocked day) |
| Expected per day — winter    | 7h30m                            |
| Expected per day — summer    | 7h                               |
| Logging range start — winter | 16:00                            |
| Logging range start — summer | 15:30                            |
| Clock adjustment             | ±5 minutes                       |

- **Clock adjustment:** a bare `wh in` records `now − 5min`; a bare `wh out`
  records `now + 5min`. When you pass `--at HH:MM` (or any explicit
  date/time), the time is taken **literally** with no adjustment.
- **Off days** have expected = 0 and no worked time. Marking a day off that
  already has worked hours is an error — run `wh clear` first.
- **No overnight shifts:** an end before the start is rejected.
- **Future timestamps** are rejected unless `--force` (the small ±5min bare
  adjustment is exempt).
- **Very long days** (worked > ~16h) warn but are allowed.
- **Default season** is `winter` until you set it.

## Date & time input formats

Dates are **day-first European**:

- `DD.MM.YYYY` or `DD-MM-YYYY` (e.g. `15.02.2026`)
- `DD.MM` or `DD-MM` (year defaults to the current year, e.g. `15.02`)
- keywords `today` and `yesterday`

Two-digit years and otherwise ambiguous input are rejected.

Times are 24-hour `HH:MM` (e.g. `08:30`).

Durations (for `--lunch` / `--expected`) accept Go-style durations or `HH:MM`:
`30m`, `7h`, `7h30m`, `7:30`, `0m`.

## Command reference

```
wh in    [DATE] [--at HH:MM] [--force]    Clock in (start).
wh out   [DATE] [--at HH:MM] [--force]    Clock out (end).
wh set   DATE --start HH:MM --end HH:MM [--lunch DUR] [--expected DUR]
wh off   DATE [--clear]                   Mark a day off (or clear it).
wh clear DATE                             Delete a day's record entirely.
wh week  [N|last] [--year YYYY]           Week summary.
wh unlogged                               List past unlogged weeks with worked time.
wh log   [N|last] [--year YYYY]           Mark a week logged.
wh season [summer|winter]                 Print or set the season.
wh status                                 Show clock-in state and time so far today.
wh help                                   Usage.
```

### `wh in` — clock in

Opens the day's start time.

```sh
wh in                       # now − 5min, today
wh in --at 08:30            # today at 08:30 (literal)
wh in 15.02 --at 08:30      # 15 Feb this year at 08:30 (literal)
wh in --force               # overwrite an existing start
```

A bare `DATE` without `--at` is rejected (a past day needs an explicit time —
use `--at` or `wh set`). Clocking in when already clocked in errors unless
`--force`.

### `wh out` — clock out

Closes the day's end time.

```sh
wh out                      # now + 5min, today
wh out --at 16:15           # today at 16:15 (literal)
wh out 15.02 --at 16:15     # 15 Feb at 16:15 (literal)
```

Errors if there is no open clock-in for that day.

### `wh set` — backfill / overwrite a day

```sh
wh set 15.02.2026 --start 08:00 --end 16:00
wh set 15.02 --start 08:00 --end 17:00 --lunch 45m
wh set today --start 09:00 --end 16:30 --expected 7h
```

Overwrites the whole day silently but prints a `before → after` diff.

### `wh off` — mark a day off

```sh
wh off 24.12.2026           # mark off
wh off 24.12.2026 --clear   # remove the off mark
```

If the day already has worked hours, run `wh clear` first.

### `wh clear` — delete a day

```sh
wh clear 15.02.2026
```

### `wh week` — week summary

```sh
wh week                     # current week
wh week last                # previous week
wh week 26                  # week 26 of the current ISO year
wh week 26 --year 2026      # week 26 of 2026
```

ISO-8601 weeks (Monday–Sunday). Shows a per-day breakdown, totals (worked,
expected, balance), the week's logged status, and — when the balance is
positive — a suggested logging range, e.g.:

```
Week 26 (Mon 2026-06-22 – Sun 2026-06-28)
     Date       Start–End     Worked    Expected  Balance
Mon  2026-06-22 08:00–16:00   7h30m     7h        30m
...
Worked:   15h30m (15.50h)
Expected: 14h (14.00h)
Balance:  1h30m (1.50h)
Log:      15:30–17:00  (extra 1h30m, summer season)
Status:   not logged
```

The logging range uses the season of the most-recently-worked day in that week.

### `wh unlogged` — pending weeks

```sh
wh unlogged
```

Lists past weeks (excluding the current in-progress week) that have worked time
and have not been logged, oldest-first, with the week number, date range, and
pending extra.

### `wh log` — mark a week logged

```sh
wh log                      # current week
wh log last                 # previous week
wh log 26 --year 2026       # specific week
```

Records a "logged at" timestamp. Warns (but still proceeds) for empty or
current-week logs.

### `wh season` — print or set the season

```sh
wh season                   # print current season
wh season summer            # set season to summer
wh season winter            # set season to winter
```

Only affects days created **after** the change (expected hours are snapshotted
per day).

### `wh status` — current state

```sh
wh status
```

Shows today's date and season, whether you are currently clocked in, and how
much time has accrued today.
