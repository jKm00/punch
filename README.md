# punch — workhour tracker

`punch` is a command-line tool for fast work-hour logging: run a command when you
arrive, another when you leave, then check a weekly summary to log overtime in
the company's official app.

## Installation

### Install script (recommended)

**Step 1 — install.** Run this one-liner — it detects your OS/arch, downloads
the latest release, verifies the checksum, and installs `punch` to
`~/.local/bin` (it warns you if `~/.local/bin` isn't on your `$PATH` and tells
you how to fix it):

```sh
curl -fsSL https://raw.githubusercontent.com/jKm00/punch/main/install.sh | bash
```

**Step 2 — verify** the install:

```sh
punch version
```

**Step 3 — configure** punch (expected hours, logging times, lunch, season):

```sh
punch setup
```

Then run `punch help` any time for usage. The setup wizard also runs
automatically the first time you use `punch` on an interactive terminal, so you
can skip Step 3 and just start logging if the defaults suit you.

### Build from source

Requires **Go 1.25+**:

```sh
make install           # builds and installs to ~/.local/bin/punch
punch version          # verify
punch setup            # configure (expected hours, logging times, lunch, season)
punch help             # usage
```

## Upgrading

`punch` checks for new releases once a day (in the background) and prints a
notice when one is available:

```
A new version of punch is available (v1.0.0 → v1.1.0). Run `punch upgrade` to update.
```

To update in place:

```sh
punch upgrade          # downloads the latest release, verifies it, replaces the binary
```

Notes:

- Disable the background check with `PUNCH_NO_UPDATE_CHECK=1`.
- Builds installed from source report version `dev` and are never auto-nagged
  or self-replaced — use `make install` to update them.

## Uninstall

Remove the binary:

```sh
rm -f ~/.local/bin/punch     # if you installed via the script or from source
# or, from a cloned repo:
make uninstall
```

Your logged hours stay at `~/.local/share/punch/punch.db`. To remove them too:

```sh
rm -rf ~/.local/share/punch
```

## Development

Copy the env template, then run from source via `make dev`. `.env` sets
`PUNCH_DB=./dev.db` so dev runs use a separate, gitignored database.

```sh
cp .env.example .env

make dev ARGS="in"
make dev ARGS="week last"
make dev ARGS='set 15.02 --start 08:00 --end 16:00'

make dev-reset    # delete the dev database
make test         # run tests
```

### Releasing

Releases are cut by pushing a semver tag. A GitHub Actions workflow
(`.github/workflows/release.yml`) builds binaries for macOS and Linux
(`amd64`/`arm64`), packages each as `punch_<version>_<os>_<arch>.tar.gz`,
generates a `SHA256SUMS` file, and attaches them — along with `install.sh` —
to a GitHub Release. The version is stamped into the binary via
`-ldflags -X main.version`.

```sh
git tag v1.0.0
git push origin v1.0.0
```

Users then get the upgrade notice and can run `punch upgrade`.

## Configuration

`punch` ships with sensible defaults but lets you tailor the values that matter
most. A short setup wizard runs **automatically on first use** (the first time
you run any database-backed command on an interactive terminal) and is
**re-runnable any time** with:

```sh
punch setup
```

The wizard configures:

- **Winter / summer expected hours per day** (defaults `7h30m` / `7h`)
- **Winter / summer logging start time** (defaults `16:00` / `15:30`)
- **Default lunch break** (default `30m`, deducted on every clocked day)
- **Current season** (`summer` or `winter`)

For every prompt the **recommended default** is shown in brackets — press
**Enter** to accept it. Values are stored in the same SQLite database as your
hours, so they persist across runs. The wizard always offers the built-in
recommended defaults (not your stored values), so re-running it and accepting
every prompt resets your configuration back to the recommendations.

Notes:

- Each value falls back to its built-in default if you never run the wizard,
  so existing setups keep working unchanged.
- When stdin is **not** an interactive terminal (piped or scripted), the
  first-run wizard is skipped silently and the defaults are used — `punch` stays
  scriptable.
- `punch season` still works on its own and writes the same season setting as
  the wizard.

## Rules & constants

These defaults apply until you change the configurable ones via `punch setup`
(see [Configuration](#configuration)). The clock adjustment and the very-long-day
threshold are not configurable.

| Constant                     | Value                            | Configurable |
| ---------------------------- | -------------------------------- | ------------ |
| Default lunch                | 30m (deducted every clocked day) | yes          |
| Expected per day — winter    | 7h30m                            | yes          |
| Expected per day — summer    | 7h                               | yes          |
| Logging range start — winter | 16:00                            | yes          |
| Logging range start — summer | 15:30                            | yes          |
| Clock adjustment             | ±5 minutes                       | no           |

- **Clock adjustment:** a bare `punch in` records `now − 5min`; a bare `punch out`
  records `now + 5min`. When you pass `--at HH:MM` (or any explicit
  date/time), the time is taken **literally** with no adjustment.
- **Off days** have expected = 0 and no worked time. Marking a day off that
  already has worked hours is an error — run `punch clear` first.
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
punch in    [DATE] [--at HH:MM] [--force]    Clock in (start).
punch out   [DATE] [--at HH:MM] [--force]    Clock out (end).
punch set   DATE --start HH:MM --end HH:MM [--lunch DUR] [--expected DUR]
punch off   DATE [--clear]                   Mark a day off (or clear it).
punch clear DATE                             Delete a day's record entirely.
punch week  [N|last] [--year YYYY]           Week summary.
punch unlogged                               List past unlogged weeks with worked time.
punch log   [N|last] [--year YYYY]           Toggle a week's logged state.
punch season [summer|winter]                 Print or set the season.
punch setup [--curr]                         Configure punch; --curr prints current config.
punch status                                 Show clock-in state and time so far today.
punch analytics [YEAR]                       Yearly dashboard (default: current year).
punch version                                Print the installed version.
punch upgrade                                Download and install the latest version.
punch help                                   Usage.
```

### `punch in` — clock in

Opens the day's start time.

```sh
punch in                       # now − 5min, today
punch in --at 08:30            # today at 08:30 (literal)
punch in 15.02 --at 08:30      # 15 Feb this year at 08:30 (literal)
punch in --force               # overwrite an existing start
```

A bare `DATE` without `--at` is rejected (a past day needs an explicit time —
use `--at` or `punch set`). Clocking in when already clocked in errors unless
`--force`.

### `punch out` — clock out

Closes the day's end time.

```sh
punch out                      # now + 5min, today
punch out --at 16:15           # today at 16:15 (literal)
punch out 15.02 --at 16:15     # 15 Feb at 16:15 (literal)
```

Errors if there is no open clock-in for that day.

### `punch set` — backfill / overwrite a day

```sh
punch set 15.02.2026 --start 08:00 --end 16:00
punch set 15.02 --start 08:00 --end 17:00 --lunch 45m
punch set today --start 09:00 --end 16:30 --expected 7h
```

Overwrites the whole day silently but prints a `before → after` diff.

### `punch off` — mark a day off

```sh
punch off 24.12.2026           # mark off
punch off 24.12.2026 --clear   # remove the off mark
```

If the day already has worked hours, run `punch clear` first.

### `punch clear` — delete a day

```sh
punch clear 15.02.2026
```

### `punch week` — week summary

```sh
punch week                     # current week
punch week last                # previous week
punch week 26                  # week 26 of the current ISO year
punch week 26 --year 2026      # week 26 of 2026
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

### `punch unlogged` — pending weeks

```sh
punch unlogged
```

Lists past weeks (excluding the current in-progress week) that have worked time
and have not been logged, oldest-first, with the week number, date range, and
pending extra.

### `punch log` — toggle a week's logged state

```sh
punch log                      # toggle current week
punch log last                 # toggle previous week
punch log 26 --year 2026       # toggle a specific week
```

Toggles the week's logged state: an unlogged week is marked logged (recording a
"logged at" timestamp), and a logged week is unlogged. The output states the
resulting status. When marking a week logged, it warns (but still proceeds) for
empty or current-week logs.

### `punch season` — print or set the season

```sh
punch season                   # print current season
punch season summer            # set season to summer
punch season winter            # set season to winter
```

Only affects days created **after** the change (expected hours are snapshotted
per day).

### `punch setup` — configure punch

```sh
punch setup                    # re-run the configuration wizard
punch setup --curr             # print the current configuration (no prompts)
```

Walks through the configurable values (expected hours per season, logging start
times, default lunch, and the current season), showing the **recommended
default** as the `[default]` for each prompt — press Enter to accept it. The
wizard always offers the built-in recommended defaults, even if you have custom
values stored, so re-running it lets you reset toward the recommendations. The
wizard also runs automatically the first time you use `punch` on an interactive
terminal. Use `--curr` to print the currently-effective configuration without
prompting or changing anything. See [Configuration](#configuration) for details.

### `punch status` — current state

```sh
punch status
```

Shows today's date and season, whether you are currently clocked in, and how
much time has accrued today.

### `punch analytics` — yearly dashboard

```sh
punch analytics                # current year
punch analytics 2025           # a specific year
```

Prints a dashboard for the calendar year: totals (days worked, worked vs
expected, balance, time at lunch), rhythm (average day, average balance,
average arrival/departure), extremes (longest/shortest day, earliest start,
latest finish — each with the date), week coverage (active/logged/unlogged),
and worked-hours bar charts by weekday and by month. Prints a short notice if
the year has no data.

### `punch version` / `punch upgrade`

```sh
punch version              # print the installed version
punch upgrade              # update to the latest release
```

See [Upgrading](#upgrading) for details on the update check.
