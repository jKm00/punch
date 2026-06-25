package app

import (
	"flag"
	"fmt"
	"time"

	"wh/internal/calc"
	"wh/internal/domain"
	"wh/internal/isoweek"
	"wh/internal/store"
)

// resolveWeek parses the `[N|last]` positional and `--year` flag against the
// current time, returning the ISO year and week to operate on.
func (a *App) resolveWeek(arg string, yearFlag int) (year, week int, err error) {
	now := a.now()
	curYear, curWeek := isoweek.Of(now)
	switch arg {
	case "", "current":
		return curYear, curWeek, nil
	case "last":
		// Monday of current week minus one day lands in the previous ISO week.
		mon := isoweek.Monday(curYear, curWeek, a.Loc)
		prev := mon.AddDate(0, 0, -1)
		y, w := isoweek.Of(prev)
		return y, w, nil
	default:
		var n int
		if _, e := fmt.Sscanf(arg, "%d", &n); e != nil {
			return 0, 0, fmt.Errorf("invalid week %q: use a number, `last`, or omit for current", arg)
		}
		y := curYear
		if yearFlag != 0 {
			y = yearFlag
		}
		if n < 1 || n > 53 {
			return 0, 0, fmt.Errorf("invalid week number %d", n)
		}
		return y, n, nil
	}
}

// weekData aggregates a week's day records and totals.
type weekData struct {
	year, week       int
	monday, sunday   time.Time
	days             []*store.Day
	totalWorked      int
	totalExpected    int
	lastWorkedSeason domain.Season
	hasLastWorkedDay bool
}

func (a *App) loadWeek(year, week int) (*weekData, error) {
	monday, sunday := isoweek.Range(year, week, a.Loc)
	days, err := a.Store.DaysInRange(monday, sunday)
	if err != nil {
		return nil, err
	}
	wd := &weekData{year: year, week: week, monday: monday, sunday: sunday, days: days}

	season, err := a.Store.Season()
	if err != nil {
		return nil, err
	}
	wd.lastWorkedSeason = season // fallback

	var lastWorked time.Time
	for _, d := range days {
		if d.IsOff {
			continue
		}
		wd.totalExpected += d.ExpectedMinutes
		if d.Start != nil && d.End != nil {
			wd.totalWorked += calc.WorkedMinutes(*d.Start, *d.End, d.EffectiveLunch())
			if !wd.hasLastWorkedDay || d.Date.After(lastWorked) {
				lastWorked = d.Date
				wd.hasLastWorkedDay = true
			}
		}
	}
	// Season of the most-recent-worked-day: we snapshot expected minutes, so
	// infer season from the day's expected value when possible.
	if wd.hasLastWorkedDay {
		for _, d := range days {
			if d.Date.Equal(lastWorked) {
				wd.lastWorkedSeason = seasonFromExpected(d.ExpectedMinutes, season)
			}
		}
	}
	return wd, nil
}

// seasonFromExpected infers the season from a snapshotted expected-minutes
// value, falling back to the current season for non-standard values.
func seasonFromExpected(expected int, fallback domain.Season) domain.Season {
	switch expected {
	case domain.SummerExpectedMinutes:
		return domain.Summer
	case domain.WinterExpectedMinutes:
		return domain.Winter
	default:
		return fallback
	}
}

// CmdWeek handles `wh week [N|last] [--year YYYY]`.
func (a *App) CmdWeek(args []string) error {
	fs := flag.NewFlagSet("week", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	yearFlag := fs.Int("year", 0, "ISO year for a numeric week")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}
	arg := ""
	if fs.NArg() > 0 {
		arg = fs.Arg(0)
	}
	year, week, err := a.resolveWeek(arg, *yearFlag)
	if err != nil {
		return err
	}
	wd, err := a.loadWeek(year, week)
	if err != nil {
		return err
	}
	return a.printWeek(wd)
}

func (a *App) printWeek(wd *weekData) error {
	a.printf("Week %d (%s – %s)\n",
		wd.week,
		wd.monday.Format("Mon 2006-01-02"),
		wd.sunday.Format("Mon 2006-01-02"))

	// Index days by date for the Mon..Sun walk.
	byDate := map[string]*store.Day{}
	for _, d := range wd.days {
		byDate[d.Date.Format(store.DateLayout)] = d
	}

	a.printf("%-4s %-10s %-13s %-9s %-9s %s\n", "", "Date", "Start–End", "Worked", "Expected", "Balance")
	for i := 0; i < 7; i++ {
		day := wd.monday.AddDate(0, 0, i)
		key := day.Format(store.DateLayout)
		rec := byDate[key]
		wd3 := day.Format("Mon")
		dateStr := day.Format("2006-01-02")

		if rec == nil {
			a.printf("%-4s %-10s %-13s %-9s %-9s %s\n", wd3, dateStr, "—", "—", "—", "—")
			continue
		}
		if rec.IsOff {
			a.printf("%-4s %-10s %-13s %-9s %-9s %s\n", wd3, dateStr, "OFF", "—", "0m", "—")
			continue
		}
		if rec.Start != nil && rec.End == nil {
			a.printf("%-4s %-10s %-13s %-9s %-9s %s\n", wd3, dateStr,
				rec.Start.Format("15:04")+"–open", "open",
				calc.FormatHM(rec.ExpectedMinutes), "—")
			continue
		}
		if rec.Start != nil && rec.End != nil {
			worked := calc.WorkedMinutes(*rec.Start, *rec.End, rec.EffectiveLunch())
			bal := worked - rec.ExpectedMinutes
			a.printf("%-4s %-10s %-13s %-9s %-9s %s\n", wd3, dateStr,
				rec.Start.Format("15:04")+"–"+rec.End.Format("15:04"),
				calc.FormatHM(worked),
				calc.FormatHM(rec.ExpectedMinutes),
				calc.FormatHM(bal))
			continue
		}
		// start nil but record exists (e.g. expected only)
		a.printf("%-4s %-10s %-13s %-9s %-9s %s\n", wd3, dateStr, "—", "—",
			calc.FormatHM(rec.ExpectedMinutes), "—")
	}

	balance := wd.totalWorked - wd.totalExpected
	a.printf("\n")
	a.printf("Worked:   %s (%s)\n", calc.FormatHM(wd.totalWorked), calc.FormatDecimalHours(wd.totalWorked))
	a.printf("Expected: %s (%s)\n", calc.FormatHM(wd.totalExpected), calc.FormatDecimalHours(wd.totalExpected))
	a.printf("Balance:  %s (%s)\n", calc.FormatHM(balance), calc.FormatDecimalHours(balance))

	if balance > 0 {
		sh, sm, eh, em := calc.LoggingEnd(wd.lastWorkedSeason, balance)
		a.printf("Log:      %s–%s  (extra %s, %s season)\n",
			calc.FormatClock(sh, sm), calc.FormatClock(eh, em),
			calc.FormatHM(balance), wd.lastWorkedSeason)
	}

	loggedAt, err := a.Store.WeekLoggedAt(isoweek.Key(wd.year, wd.week))
	if err != nil {
		return err
	}
	if loggedAt != nil {
		a.printf("Status:   logged at %s\n", loggedAt.Format("2006-01-02 15:04"))
	} else {
		a.printf("Status:   not logged\n")
	}
	return nil
}

// CmdUnlogged handles `wh unlogged`.
func (a *App) CmdUnlogged(args []string) error {
	fs := flag.NewFlagSet("unlogged", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}

	earliest, err := a.Store.EarliestWorkedDate()
	if err != nil {
		return err
	}
	if earliest == nil {
		a.printf("No worked days recorded yet.\n")
		return nil
	}

	now := a.now()
	curYear, curWeek := isoweek.Of(now)
	curMonday := isoweek.Monday(curYear, curWeek, a.Loc)

	// Walk week by week from the earliest worked week up to (but excluding)
	// the current in-progress week.
	startYear, startWeek := isoweek.Of(*earliest)
	cursor := isoweek.Monday(startYear, startWeek, a.Loc)

	type pending struct {
		year, week     int
		monday, sunday time.Time
		extra          int
	}
	var pendings []pending

	for cursor.Before(curMonday) {
		y, w := isoweek.Of(cursor)
		wd, err := a.loadWeek(y, w)
		if err != nil {
			return err
		}
		if wd.totalWorked > 0 {
			loggedAt, err := a.Store.WeekLoggedAt(isoweek.Key(y, w))
			if err != nil {
				return err
			}
			if loggedAt == nil {
				pendings = append(pendings, pending{
					year: y, week: w,
					monday: wd.monday, sunday: wd.sunday,
					extra: wd.totalWorked - wd.totalExpected,
				})
			}
		}
		cursor = cursor.AddDate(0, 0, 7)
	}

	if len(pendings) == 0 {
		a.printf("No unlogged past weeks. 🎉\n")
		return nil
	}

	a.printf("%-9s %-27s %s\n", "Week", "Range", "Pending extra")
	for _, p := range pendings {
		a.printf("%-9s %-27s %s\n",
			isoweek.Key(p.year, p.week),
			fmt.Sprintf("%s – %s", p.monday.Format("2006-01-02"), p.sunday.Format("2006-01-02")),
			calc.FormatHM(p.extra))
	}
	return nil
}

// CmdLog handles `wh log [N|last] [--year YYYY]`.
func (a *App) CmdLog(args []string) error {
	fs := flag.NewFlagSet("log", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	yearFlag := fs.Int("year", 0, "ISO year for a numeric week")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}
	arg := ""
	if fs.NArg() > 0 {
		arg = fs.Arg(0)
	}
	year, week, err := a.resolveWeek(arg, *yearFlag)
	if err != nil {
		return err
	}

	wd, err := a.loadWeek(year, week)
	if err != nil {
		return err
	}

	curYear, curWeek := isoweek.Of(a.now())
	if year == curYear && week == curWeek {
		a.errorf("warning: marking the current (in-progress) week as logged\n")
	}
	if wd.totalWorked == 0 {
		a.errorf("warning: week %s has no worked time\n", isoweek.Key(year, week))
	}

	at := a.now()
	if err := a.Store.SetWeekLogged(isoweek.Key(year, week), at); err != nil {
		return err
	}
	a.printf("Marked %s logged at %s\n", isoweek.Key(year, week), at.Format("2006-01-02 15:04"))
	return nil
}

// CmdSeason handles `wh season [summer|winter]`.
func (a *App) CmdSeason(args []string) error {
	fs := flag.NewFlagSet("season", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		s, err := a.Store.Season()
		if err != nil {
			return err
		}
		a.printf("%s\n", s)
		return nil
	}
	arg := fs.Arg(0)
	switch domain.Season(arg) {
	case domain.Summer, domain.Winter:
		if err := a.Store.SetSeason(domain.Season(arg)); err != nil {
			return err
		}
		a.printf("Season set to %s (expected %s/day, logging starts %s)\n",
			arg, calc.FormatHM(domain.ExpectedMinutesFor(domain.Season(arg))),
			logStartClock(domain.Season(arg)))
		return nil
	default:
		return fmt.Errorf("invalid season %q: use `summer` or `winter`", arg)
	}
}

func logStartClock(s domain.Season) string {
	h, m := domain.LoggingStartFor(s)
	return calc.FormatClock(h, m)
}

// CmdStatus handles `wh status`.
func (a *App) CmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}
	now := a.now()
	today := a.dateOnly(now)
	day, err := a.Store.GetDay(today)
	if err != nil {
		return err
	}
	season, err := a.Store.Season()
	if err != nil {
		return err
	}
	a.printf("Today:  %s (season: %s)\n", today.Format("Mon 2006-01-02"), season)

	if day == nil {
		a.printf("Status: not clocked in\n")
		return nil
	}
	if day.IsOff {
		a.printf("Status: OFF\n")
		return nil
	}
	if day.Start != nil && day.End == nil {
		elapsed := int(now.Sub(*day.Start).Minutes())
		netSoFar := elapsed - day.EffectiveLunch()
		if netSoFar < 0 {
			netSoFar = 0
		}
		a.printf("Status: clocked IN since %s — elapsed %s, net of lunch %s\n",
			day.Start.Format("15:04"), calc.FormatHM(elapsed), calc.FormatHM(netSoFar))
		return nil
	}
	if day.Start != nil && day.End != nil {
		worked := calc.WorkedMinutes(*day.Start, *day.End, day.EffectiveLunch())
		a.printf("Status: clocked OUT — %s–%s, worked %s (%s)\n",
			day.Start.Format("15:04"), day.End.Format("15:04"),
			calc.FormatHM(worked), calc.FormatDecimalHours(worked))
		return nil
	}
	a.printf("Status: not clocked in\n")
	return nil
}
