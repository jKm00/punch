package app

import (
	"flag"
	"fmt"
	"time"

	"punch/internal/calc"
	"punch/internal/domain"
	"punch/internal/store"
	"punch/internal/timeparse"
)

// resolveDayAndTime parses an optional leading positional DATE arg and an
// optional --at HH:MM. It returns the target calendar date and, if --at was
// given, the literal wall-clock time on that date.
//
// When no DATE is given, the date is today. When --at is given, atTime is
// non-nil and represents date@HH:MM (literal). Otherwise atTime is nil and the
// caller applies the bare now-case (with clock adjustment).
func (a *App) resolveDayAndTime(args []string, atFlag string) (date time.Time, atTime *time.Time, err error) {
	date = a.dateOnly(a.now())
	if len(args) > 0 {
		d, perr := timeparse.ParseDate(args[0], a.now())
		if perr != nil {
			return time.Time{}, nil, perr
		}
		date = a.dateOnly(d)
	}
	if atFlag != "" {
		h, m, perr := timeparse.ParseTime(atFlag)
		if perr != nil {
			return time.Time{}, nil, perr
		}
		t := timeparse.CombineDateTime(date, h, m)
		atTime = &t
	}
	return date, atTime, nil
}

// CmdIn handles `punch in`.
func (a *App) CmdIn(args []string) error {
	fs := flag.NewFlagSet("in", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	at := fs.String("at", "", "literal start time HH:MM (no ±5min adjustment)")
	force := fs.Bool("force", false, "overwrite an existing start / allow future timestamps")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}
	date, atTime, err := a.resolveDayAndTime(fs.Args(), *at)
	if err != nil {
		return err
	}

	var start time.Time
	bareAdjust := false
	if atTime != nil {
		start = *atTime // literal
	} else if len(fs.Args()) > 0 {
		// Explicit DATE without --at: literal now-time-of-day is meaningless;
		// require --at for a past/explicit date.
		return fmt.Errorf("a DATE was given without --at; use `punch in %s --at HH:MM` or `punch set`", fs.Args()[0])
	} else {
		start = a.now().Add(-domain.ClockAdjustMinutes * time.Minute)
		bareAdjust = true
	}

	// The bare in-case subtracts 5 minutes, so it is always in the past; only
	// literal/explicit times are subject to the future check.
	if !bareAdjust && start.After(a.now()) && !*force {
		return fmt.Errorf("start %s is in the future; pass --force to allow", start.Format(displayDateTime))
	}

	day, err := a.Store.GetDay(date)
	if err != nil {
		return err
	}
	season, err := a.Store.Season()
	if err != nil {
		return err
	}

	if day == nil {
		day = &store.Day{
			Date:            date,
			ExpectedMinutes: a.Config.ExpectedMinutesFor(season),
		}
	}
	if day.IsOff {
		return fmt.Errorf("%s is marked OFF; run `punch off %s --clear` first", date.Format(displayDate), date.Format(displayDate))
	}
	if day.Start != nil && !*force {
		return fmt.Errorf("already clocked in at %s on %s; use --force to overwrite or `punch set` to edit",
			day.Start.Format("15:04"), date.Format(displayDate))
	}

	day.Start = &start
	if err := a.Store.UpsertDay(day); err != nil {
		return err
	}
	a.printf("%s clocked in %s at %s\n", a.styler().Green("✓"), date.Format(displayDateWeekday), a.styler().Bold(start.Format("15:04")))
	return nil
}

// CmdOut handles `punch out`.
func (a *App) CmdOut(args []string) error {
	fs := flag.NewFlagSet("out", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	at := fs.String("at", "", "literal end time HH:MM (no ±5min adjustment)")
	force := fs.Bool("force", false, "allow future timestamps")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}
	date, atTime, err := a.resolveDayAndTime(fs.Args(), *at)
	if err != nil {
		return err
	}

	day, err := a.Store.GetDay(date)
	if err != nil {
		return err
	}
	if day == nil || day.Start == nil {
		return fmt.Errorf("no open clock-in for %s; run `punch in` first or use `punch set`", date.Format(displayDate))
	}
	if day.IsOff {
		return fmt.Errorf("%s is marked OFF", date.Format(displayDate))
	}

	var end time.Time
	bareAdjust := false
	if atTime != nil {
		end = *atTime // literal
	} else if len(fs.Args()) > 0 {
		return fmt.Errorf("a DATE was given without --at; use `punch out %s --at HH:MM` or `punch set`", fs.Args()[0])
	} else {
		end = a.now().Add(domain.ClockAdjustMinutes * time.Minute)
		bareAdjust = true
	}

	// The bare out-case adds 5 minutes and is intentionally just past "now",
	// so it is exempt from the future check; literal/explicit times are not.
	if !bareAdjust && end.After(a.now()) && !*force {
		return fmt.Errorf("end %s is in the future; pass --force to allow", end.Format(displayDateTime))
	}
	if end.Before(*day.Start) {
		return fmt.Errorf("end %s is before start %s (overnight shifts are not supported)",
			end.Format("15:04"), day.Start.Format("15:04"))
	}

	day.End = &end
	if err := a.Store.UpsertDay(day); err != nil {
		return err
	}

	worked := calc.WorkedMinutes(*day.Start, end, day.EffectiveLunch(a.Config.DefaultLunchMinutes))
	a.printf("%s clocked out %s at %s — worked %s %s\n",
		a.styler().Green("✓"), date.Format(displayDateWeekday), a.styler().Bold(end.Format("15:04")),
		a.styler().Bold(calc.FormatHM(worked)), a.styler().Dim("("+calc.FormatDecimalHours(worked)+")"))
	if worked > domain.LongDayMinutes {
		a.errorf("%s that is a very long day (%s)\n", a.styler().Yellow("warning:"), calc.FormatHM(worked))
	}
	return nil
}

// CmdSet handles `punch set DATE --start HH:MM --end HH:MM [--lunch DUR] [--expected DUR]`.
func (a *App) CmdSet(args []string) error {
	fs := flag.NewFlagSet("set", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	startStr := fs.String("start", "", "start time HH:MM (required)")
	endStr := fs.String("end", "", "end time HH:MM (required)")
	lunchStr := fs.String("lunch", "", "lunch duration (e.g. 30m, 0m)")
	expectedStr := fs.String("expected", "", "expected work duration (e.g. 7h30m)")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: punch set DATE --start HH:MM --end HH:MM [--lunch DUR] [--expected DUR]")
	}
	if *startStr == "" || *endStr == "" {
		return fmt.Errorf("--start and --end are required")
	}
	date, err := timeparse.ParseDate(fs.Arg(0), a.now())
	if err != nil {
		return err
	}
	date = a.dateOnly(date)

	sh, sm, err := timeparse.ParseTime(*startStr)
	if err != nil {
		return err
	}
	eh, em, err := timeparse.ParseTime(*endStr)
	if err != nil {
		return err
	}
	start := timeparse.CombineDateTime(date, sh, sm)
	end := timeparse.CombineDateTime(date, eh, em)
	if end.Before(start) {
		return fmt.Errorf("end %s is before start %s (overnight shifts are not supported)", *endStr, *startStr)
	}

	season, err := a.Store.Season()
	if err != nil {
		return err
	}

	before, err := a.Store.GetDay(date)
	if err != nil {
		return err
	}

	newDay := &store.Day{
		Date:            date,
		Start:           &start,
		End:             &end,
		ExpectedMinutes: a.Config.ExpectedMinutesFor(season),
	}
	// Preserve previous per-day overrides unless changed.
	if before != nil && !before.IsOff {
		newDay.ExpectedMinutes = before.ExpectedMinutes
		newDay.LunchMinutes = before.LunchMinutes
	}
	if *lunchStr != "" {
		lm, err := timeparse.ParseDuration(*lunchStr)
		if err != nil {
			return err
		}
		newDay.LunchMinutes = &lm
	}
	if *expectedStr != "" {
		em, err := timeparse.ParseDuration(*expectedStr)
		if err != nil {
			return err
		}
		newDay.ExpectedMinutes = em
	}

	if err := a.Store.UpsertDay(newDay); err != nil {
		return err
	}

	a.printf("%s set %s\n", a.styler().Green("✓"), date.Format(displayDateWeekday))
	a.printf("  %s %s\n", a.styler().Dim("before:"), a.describeDay(before))
	a.printf("  %s %s\n", a.styler().Dim("after: "), a.describeDay(newDay))

	worked := calc.WorkedMinutes(start, end, newDay.EffectiveLunch(a.Config.DefaultLunchMinutes))
	if worked > domain.LongDayMinutes {
		a.errorf("%s that is a very long day (%s)\n", a.styler().Yellow("warning:"), calc.FormatHM(worked))
	}
	return nil
}

// CmdOff handles `punch off DATE [--clear]`.
func (a *App) CmdOff(args []string) error {
	fs := flag.NewFlagSet("off", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	clear := fs.Bool("clear", false, "clear the off mark instead of setting it")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: punch off DATE [--clear]")
	}
	date, err := timeparse.ParseDate(fs.Arg(0), a.now())
	if err != nil {
		return err
	}
	date = a.dateOnly(date)

	day, err := a.Store.GetDay(date)
	if err != nil {
		return err
	}

	if *clear {
		if day == nil || !day.IsOff {
			return fmt.Errorf("%s is not marked OFF", date.Format(displayDate))
		}
		// Clearing an off day removes it entirely (it had no work).
		if _, err := a.Store.DeleteDay(date); err != nil {
			return err
		}
		a.printf("%s cleared OFF on %s\n", a.styler().Green("✓"), date.Format(displayDateWeekday))
		return nil
	}

	if day != nil && (day.Start != nil || day.End != nil) {
		return fmt.Errorf("%s already has worked hours; run `punch clear %s` first",
			date.Format(displayDate), date.Format(displayDate))
	}

	off := &store.Day{
		Date:            date,
		ExpectedMinutes: 0,
		IsOff:           true,
	}
	if err := a.Store.UpsertDay(off); err != nil {
		return err
	}
	a.printf("%s marked %s %s\n", a.styler().Green("✓"), date.Format(displayDateWeekday), a.styler().Yellow("OFF"))
	return nil
}

// CmdClear handles `punch clear DATE`.
func (a *App) CmdClear(args []string) error {
	fs := flag.NewFlagSet("clear", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: punch clear DATE")
	}
	date, err := timeparse.ParseDate(fs.Arg(0), a.now())
	if err != nil {
		return err
	}
	date = a.dateOnly(date)
	deleted, err := a.Store.DeleteDay(date)
	if err != nil {
		return err
	}
	if !deleted {
		a.printf("Nothing to clear for %s\n", date.Format(displayDateWeekday))
		return nil
	}
	a.printf("%s cleared %s\n", a.styler().Green("✓"), date.Format(displayDateWeekday))
	return nil
}

func (a *App) describeDay(d *store.Day) string {
	if d == nil {
		return "(none)"
	}
	if d.IsOff {
		return "OFF"
	}
	s := "—"
	e := "—"
	if d.Start != nil {
		s = d.Start.Format("15:04")
	}
	if d.End != nil {
		e = d.End.Format("15:04")
	}
	worked := 0
	if d.Start != nil && d.End != nil {
		worked = calc.WorkedMinutes(*d.Start, *d.End, d.EffectiveLunch(a.Config.DefaultLunchMinutes))
	}
	return fmt.Sprintf("%s–%s lunch %s worked %s expected %s",
		s, e, calc.FormatHM(d.EffectiveLunch(a.Config.DefaultLunchMinutes)),
		calc.FormatHM(worked), calc.FormatHM(d.ExpectedMinutes))
}
