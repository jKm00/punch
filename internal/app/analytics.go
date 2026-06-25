package app

import (
	"flag"
	"fmt"
	"time"

	"punch/internal/analytics"
	"punch/internal/calc"
	"punch/internal/isoweek"
	"punch/internal/ui"
)

// CmdAnalytics handles `punch analytics [YEAR]`. With no year it uses the current
// year. It prints a dashboard of computed metrics for that calendar year.
func (a *App) CmdAnalytics(args []string) error {
	fs := flag.NewFlagSet("analytics", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}

	year := a.now().Year()
	if fs.NArg() > 0 {
		var y int
		if _, err := fmt.Sscanf(fs.Arg(0), "%d", &y); err != nil || y < 1970 || y > 9999 {
			return fmt.Errorf("invalid year %q: use a 4-digit year like 2025", fs.Arg(0))
		}
		year = y
	}

	// Load every day in the calendar year.
	from := time.Date(year, time.January, 1, 0, 0, 0, 0, a.Loc)
	to := time.Date(year, time.December, 31, 0, 0, 0, 0, a.Loc)
	days, err := a.Store.DaysInRange(from, to)
	if err != nil {
		return err
	}

	// Build the logged-week map for weeks that have any activity this year.
	logged := map[analytics.WeekKey]bool{}
	for _, d := range days {
		if d.IsOff || d.Start == nil {
			continue
		}
		y, w := d.Date.ISOWeek()
		key := analytics.WeekKey{Year: y, Week: w}
		if _, seen := logged[key]; seen {
			continue
		}
		at, err := a.Store.WeekLoggedAt(isoweek.Key(y, w))
		if err != nil {
			return err
		}
		logged[key] = at != nil
	}

	sum := analytics.Compute(year, days, logged)

	if !sum.HasData {
		a.printf("No data for %d.\n", year)
		return nil
	}

	a.printAnalytics(sum)
	return nil
}

func (a *App) printAnalytics(s *analytics.Summary) {
	st := a.styler()

	// --- Overview box ---
	kv := func(label, value string) string {
		return st.Dim(ui.PadRight(label, 20)) + value
	}
	balColor := st.Balance(s.Balance, calc.FormatHM(s.Balance)) + st.Dim(" ("+calc.FormatDecimalHours(s.Balance)+")")

	overview := []string{
		kv("Days worked", fmt.Sprintf("%d", s.DaysWorked)) +
			st.Dim(fmt.Sprintf("   off %d   open %d", s.DaysOff, s.DaysOpen)),
		kv("Total worked", calc.FormatHM(s.TotalWorked)+st.Dim(" ("+calc.FormatDecimalHours(s.TotalWorked)+")")),
		kv("Total expected", calc.FormatHM(s.TotalExpected)+st.Dim(" ("+calc.FormatDecimalHours(s.TotalExpected)+")")),
		kv("Balance", balColor),
		kv("Time at lunch", calc.FormatHM(s.TotalLunch)),
	}
	a.printf("%s", st.Box(fmt.Sprintf("Analytics %d", s.Year), overview))

	// --- Averages / rhythm box ---
	averages := []string{
		kv("Avg day", calc.FormatHM(s.AvgWorkedPerDay)),
		kv("Avg balance/day", st.Balance(s.AvgBalancePerDay, calc.FormatHM(s.AvgBalancePerDay))),
		kv("Avg arrival", calc.FormatClockMinutes(s.AvgArrival)),
		kv("Avg departure", calc.FormatClockMinutes(s.AvgDeparture)),
	}
	a.printf("%s", st.Box("Rhythm", averages))

	// --- Extremes box ---
	ext := func(label string, e analytics.Extreme, clock bool) string {
		if !e.Valid {
			return kv(label, st.Dim("—"))
		}
		var val string
		if clock {
			val = calc.FormatClockMinutes(e.Clock)
		} else {
			val = calc.FormatHM(e.Minutes)
		}
		return kv(label, val+st.Dim("  "+e.Date.Format(displayDateWeekday)))
	}
	extremes := []string{
		ext("Longest day", s.Longest, false),
		ext("Shortest day", s.Shortest, false),
		ext("Earliest start", s.EarliestStart, true),
		ext("Latest finish", s.LatestFinish, true),
	}
	a.printf("%s", st.Box("Extremes", extremes))

	// --- Weeks box ---
	weeks := []string{
		kv("Active weeks", fmt.Sprintf("%d", s.WeeksActive)),
		kv("Logged", st.Green(fmt.Sprintf("%d", s.WeeksLogged))+st.Dim(fmt.Sprintf(" of %d", s.WeeksActive))),
		kv("Unlogged", st.Balance(-s.WeeksUnlogged, fmt.Sprintf("%d", s.WeeksUnlogged))),
	}
	a.printf("%s", st.Box("Weeks", weeks))

	// --- Weekday distribution ---
	weekdayNames := [7]string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	a.printf("%s", st.Box("Worked by weekday", barRows(st, weekdayNames[:], s.ByWeekday[:])))

	// --- Month distribution ---
	monthNames := [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	a.printf("%s", st.Box("Worked by month", barRows(st, monthNames[:], s.ByMonth[:])))
}

// barRows renders aligned "label  ████   value" rows scaled to the max value.
func barRows(st *ui.Styler, labels []string, values []int) []string {
	const barWidth = 24
	max := 0
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	rows := make([]string, 0, len(labels))
	for i, label := range labels {
		v := values[i]
		frac := 0.0
		if max > 0 {
			frac = float64(v) / float64(max)
		}
		bar := st.Bar(frac, barWidth, st.CyanFn())
		val := calc.FormatHM(v)
		if v == 0 {
			val = st.Dim("0m")
		}
		rows = append(rows, ui.PadRight(label, 4)+" "+bar+" "+ui.PadLeft(val, 7))
	}
	return rows
}
