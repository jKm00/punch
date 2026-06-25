package app

import (
	"flag"
	"fmt"
	"time"

	"punch/internal/calc"
	"punch/internal/domain"
	"punch/internal/isoweek"
	"punch/internal/store"
	"punch/internal/ui"
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
			wd.totalWorked += calc.WorkedMinutes(*d.Start, *d.End, d.EffectiveLunch(a.Config.DefaultLunchMinutes))
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
				wd.lastWorkedSeason = a.seasonFromExpected(d.ExpectedMinutes, season)
			}
		}
	}
	return wd, nil
}

// seasonFromExpected infers the season from a snapshotted expected-minutes
// value, falling back to the current season for non-standard values.
func (a *App) seasonFromExpected(expected int, fallback domain.Season) domain.Season {
	switch expected {
	case a.Config.SummerExpectedMinutes:
		return domain.Summer
	case a.Config.WinterExpectedMinutes:
		return domain.Winter
	default:
		return fallback
	}
}

// CmdWeek handles `punch week [N|last] [--year YYYY]`.
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
	s := a.styler()

	// Index days by date for the Mon..Sun walk.
	byDate := map[string]*store.Day{}
	for _, d := range wd.days {
		byDate[d.Date.Format(store.DateLayout)] = d
	}

	// Column widths (visible). Day, Date, Start–End, Worked, Expected, Balance.
	const (
		wDay  = 3
		wDate = 10
		wSE   = 13
		wWrk  = 8
		wExp  = 8
		wBal  = 9
	)

	row := func(c1, c2, c3, c4, c5, c6 string) string {
		return ui.PadRight(c1, wDay) + "  " +
			ui.PadRight(c2, wDate) + "  " +
			ui.PadRight(c3, wSE) + "  " +
			ui.PadRight(c4, wWrk) + "  " +
			ui.PadRight(c5, wExp) + "  " +
			ui.PadRight(c6, wBal)
	}

	var lines []string
	lines = append(lines, s.Bold(row("", "Date", "Start–End", "Worked", "Expected", "Balance")))

	for i := 0; i < 7; i++ {
		day := wd.monday.AddDate(0, 0, i)
		key := day.Format(store.DateLayout)
		rec := byDate[key]
		wd3 := day.Format("Mon")
		dateStr := day.Format(displayDate)
		dash := s.Dim("—")

		switch {
		case rec == nil:
			lines = append(lines, s.Dim(row(wd3, dateStr, "—", "—", "—", "—")))
		case rec.IsOff:
			lines = append(lines, row(wd3, dateStr, s.Yellow("OFF"), dash, s.Dim("0m"), dash))
		case rec.Start != nil && rec.End == nil:
			lines = append(lines, row(wd3, dateStr,
				rec.Start.Format("15:04")+s.Dim("–open"), s.Cyan("open"),
				calc.FormatHM(rec.ExpectedMinutes), dash))
		case rec.Start != nil && rec.End != nil:
			worked := calc.WorkedMinutes(*rec.Start, *rec.End, rec.EffectiveLunch(a.Config.DefaultLunchMinutes))
			bal := worked - rec.ExpectedMinutes
			lines = append(lines, row(wd3, dateStr,
				rec.Start.Format("15:04")+"–"+rec.End.Format("15:04"),
				calc.FormatHM(worked),
				calc.FormatHM(rec.ExpectedMinutes),
				s.Balance(bal, calc.FormatHM(bal))))
		default:
			lines = append(lines, row(wd3, dateStr, dash, dash,
				calc.FormatHM(rec.ExpectedMinutes), dash))
		}
	}

	// Totals block, separated by a rule.
	balance := wd.totalWorked - wd.totalExpected
	tableWidth := ui.VisibleWidth(row("", "", "", "", "", ""))
	lines = append(lines, s.Rule(tableWidth))
	lines = append(lines,
		s.Bold("Worked  ")+ui.PadRight(calc.FormatHM(wd.totalWorked), 9)+s.Dim("("+calc.FormatDecimalHours(wd.totalWorked)+")"))
	lines = append(lines,
		s.Bold("Expected")+" "+ui.PadRight(calc.FormatHM(wd.totalExpected), 9)+s.Dim("("+calc.FormatDecimalHours(wd.totalExpected)+")"))
	lines = append(lines,
		s.Bold("Balance ")+" "+ui.PadRight(s.Balance(balance, calc.FormatHM(balance)), 9)+s.Dim("("+calc.FormatDecimalHours(balance)+")"))

	if balance > 0 {
		sh, sm, eh, em := calc.LoggingEnd(a.Config, wd.lastWorkedSeason, balance)
		lines = append(lines, s.Green(fmt.Sprintf("Log     %s–%s",
			calc.FormatClock(sh, sm), calc.FormatClock(eh, em)))+
			s.Dim(fmt.Sprintf("  (extra %s, %s season)", calc.FormatHM(balance), wd.lastWorkedSeason)))
	}

	loggedAt, err := a.Store.WeekLoggedAt(isoweek.Key(wd.year, wd.week))
	if err != nil {
		return err
	}
	if loggedAt != nil {
		lines = append(lines, s.Green("✓ logged")+s.Dim(" at "+loggedAt.Format(displayDateTime)))
	} else {
		lines = append(lines, s.Yellow("• not logged"))
	}

	title := fmt.Sprintf("Week %d  %s – %s",
		wd.week,
		wd.monday.Format(displayDateWeekday),
		wd.sunday.Format(displayDateWeekday))
	a.printf("%s", s.Box(title, lines))
	return nil
}

// CmdUnlogged handles `punch unlogged`.
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
		a.printf("%s\n", a.styler().Green("No unlogged past weeks."))
		return nil
	}

	s := a.styler()
	const (
		wWeek  = 9
		wRange = 25
	)
	rowU := func(c1, c2, c3 string) string {
		return ui.PadRight(c1, wWeek) + "  " + ui.PadRight(c2, wRange) + "  " + c3
	}
	var lines []string
	lines = append(lines, s.Bold(rowU("Week", "Range", "Pending extra")))
	for _, p := range pendings {
		rng := fmt.Sprintf("%s – %s", p.monday.Format(displayDate), p.sunday.Format(displayDate))
		lines = append(lines, rowU(
			s.Yellow(isoweek.Key(p.year, p.week)),
			rng,
			s.Balance(p.extra, calc.FormatHM(p.extra))))
	}
	a.printf("%s", s.Box("Unlogged weeks", lines))
	return nil
}

// CmdLog handles `punch log [N|last] [--year YYYY]`. It toggles the logged
// state of the target week: an unlogged week becomes logged, and a logged week
// becomes unlogged. The output states the resulting status explicitly.
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

	key := isoweek.Key(year, week)
	loggedAt, err := a.Store.WeekLoggedAt(key)
	if err != nil {
		return err
	}

	// Already logged → toggle off (unlog).
	if loggedAt != nil {
		if err := a.Store.ClearWeekLogged(key); err != nil {
			return err
		}
		a.printf("%s %s is now %s\n",
			a.styler().Yellow("○"), key, a.styler().Bold("unlogged"))
		return nil
	}

	// Not logged → toggle on (log). Warnings only apply when marking logged.
	curYear, curWeek := isoweek.Of(a.now())
	if year == curYear && week == curWeek {
		a.errorf("%s marking the current (in-progress) week as logged\n", a.styler().Yellow("warning:"))
	}
	if wd.totalWorked == 0 {
		a.errorf("%s week %s has no worked time\n", a.styler().Yellow("warning:"), key)
	}

	at := a.now()
	if err := a.Store.SetWeekLogged(key, at); err != nil {
		return err
	}
	a.printf("%s %s is now %s (at %s)\n",
		a.styler().Green("✓"), key, a.styler().Bold("logged"), at.Format(displayDateTime))
	return nil
}

// CmdSeason handles `punch season [summer|winter]`.
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
		a.printf("%s\n", a.styler().Bold(string(s)))
		return nil
	}
	arg := fs.Arg(0)
	switch domain.Season(arg) {
	case domain.Summer, domain.Winter:
		if err := a.Store.SetSeason(domain.Season(arg)); err != nil {
			return err
		}
		a.printf("%s season set to %s %s\n",
			a.styler().Green("✓"), a.styler().Bold(arg),
			a.styler().Dim(fmt.Sprintf("(expected %s/day, logging starts %s)",
				calc.FormatHM(a.Config.ExpectedMinutesFor(domain.Season(arg))),
				a.logStartClock(domain.Season(arg)))))
		return nil
	default:
		return fmt.Errorf("invalid season %q: use `summer` or `winter`", arg)
	}
}

func (a *App) logStartClock(s domain.Season) string {
	h, m := a.Config.LoggingStartFor(s)
	return calc.FormatClock(h, m)
}

// CmdStatus handles `punch status`.
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

	s := a.styler()
	var lines []string
	lines = append(lines, s.Dim("Date   ")+today.Format(displayDateWeekday)+s.Dim("  season ")+string(season))

	switch {
	case day == nil:
		lines = append(lines, s.Dim("Status ")+s.Yellow("not clocked in"))
	case day.IsOff:
		lines = append(lines, s.Dim("Status ")+s.Yellow("OFF"))
	case day.Start != nil && day.End == nil:
		elapsed := int(now.Sub(*day.Start).Minutes())
		netSoFar := elapsed - day.EffectiveLunch(a.Config.DefaultLunchMinutes)
		if netSoFar < 0 {
			netSoFar = 0
		}
		lines = append(lines, s.Dim("Status ")+s.Cyan("clocked IN")+s.Dim(" since ")+day.Start.Format("15:04"))
		lines = append(lines, s.Dim("Elapsed ")+calc.FormatHM(elapsed)+s.Dim("   net of lunch ")+calc.FormatHM(netSoFar))
	case day.Start != nil && day.End != nil:
		worked := calc.WorkedMinutes(*day.Start, *day.End, day.EffectiveLunch(a.Config.DefaultLunchMinutes))
		lines = append(lines, s.Dim("Status ")+s.Green("clocked OUT")+s.Dim(" ")+day.Start.Format("15:04")+"–"+day.End.Format("15:04"))
		lines = append(lines, s.Dim("Worked ")+calc.FormatHM(worked)+s.Dim(" ("+calc.FormatDecimalHours(worked)+")"))
	default:
		lines = append(lines, s.Dim("Status ")+s.Yellow("not clocked in"))
	}

	a.printf("%s", s.Box("Status", lines))
	return nil
}
