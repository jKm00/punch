# wh — workhour tracker

`wh` is a small personal command-line tool for tracking work hours. Run a
command when you arrive, a command when you leave, then view a weekly summary so
you can log overtime in the company's official app.

It is **not** a daemon: each invocation opens a local SQLite database, does its
work, and exits.

## How it works

- One record per day (start, end, lunch, expected hours, or "off").
- Worked time for a day = `(end − start) − lunch`, never negative.
- Expected hours are **snapshotted per day** from the current season when the
  day is first created, so changing season later does not rewrite history.
- A weekly summary shows totals and a suggested logging range for the official
  app.

## Installation

### Prerequisites

- **Go 1.25** or newer.
- `~/.local/bin` on your `$PATH` (it already is, for this user).

### Build & install

```sh
make install      # builds and installs to ~/.local/bin/wh
```

Other targets:

```sh
make build        # build ./wh in the repo
make test         # run the unit tests
make vet          # go vet ./...
make tidy         # go mod tidy
```

Verify it works:

```sh
wh help
```

The only non-standard-library dependency is the pure-Go SQLite driver
`modernc.org/sqlite` (no CGo, no system SQLite required).

## Database location

The database is stored at:

1. `$WH_DB` if set (highest precedence), else
2. `$XDG_DATA_HOME/wh/wh.db` if `$XDG_DATA_HOME` is set, else
3. `~/.local/share/wh/wh.db`.

The directory is created automatically. The schema is created on first run
(`PRAGMA journal_mode=WAL`, `PRAGMA foreign_keys=ON`). There is no migration
framework.

## Development

When working on the tool, run it from source against a **separate dev
database** so you never touch your real data at `~/.local/share/wh/wh.db`.

### One-time setup

```bash
cp .env.example .env
```

`.env` is gitignored and sets `WH_DB=./dev.db` (a repo-local, gitignored
database). It is loaded **only** by the `make dev*` targets — it is never read
by the installed `wh` binary, so your production database is unaffected.

> Avoid `export WH_DB=...` in your shell: that variable would leak into the
> installed `wh` and make it use your dev database too. The `make dev` targets
> scope `WH_DB` to the dev command only, avoiding this.

### Dev commands

Run any subcommand from source via `make dev ARGS="..."`:

```bash
make dev ARGS="help"
make dev ARGS="in"
make dev ARGS="week last"
make dev ARGS='set 15.02 --start 08:00 --end 16:00'
```

Other dev targets:

| Command            | What it does                                              |
| ------------------ | --------------------------------------------------------- |
| `make dev ARGS=…`  | Run `go run ./cmd/wh …` against the dev database.         |
| `make env`         | Print the effective dev env (`ENV_FILE`, `WH_DB`).        |
| `make dev-db-path` | Print the dev database path `make dev` will use.          |
| `make dev-reset`   | Delete the dev database (and its `-wal`/`-shm` sidecars). |
| `make build`       | Build a local `./wh` binary (gitignored).                 |
| `make test`        | `go test ./...`                                            |
| `make vet`         | `go vet ./...`                                             |
| `make tidy`        | `go mod tidy`                                              |

The source entry point lives at `./cmd/wh`, so a bare `go run .` will not work —
use `make dev` (or `go run ./cmd/wh …`) instead.

If you build a binary for repeated manual dev runs, point it at the dev DB
explicitly so it doesn't use production:

```bash
make build
WH_DB=./dev.db ./wh week
```

## Rules & constants

These are hardcoded:

| Constant                    | Value                          |
| --------------------------- | ------------------------------ |
| Default lunch               | 30m (deducted every clocked day) |
| Expected per day — winter   | 7h30m                          |
| Expected per day — summer   | 7h                             |
| Logging range start — winter | 16:00                         |
| Logging range start — summer | 15:30                         |
| Clock adjustment            | ±5 minutes                     |

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
Output is plain text with no ANSI color, so it is safe to pipe.

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
