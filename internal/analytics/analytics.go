// Package analytics computes yearly work-hour metrics from day records. All
// functions here are pure: they take loaded data and return computed results,
// so they are straightforward to unit test independently of the store.
package analytics

import (
	"time"

	"punch/internal/calc"
	"punch/internal/store"
)

// Extreme captures a metric value paired with the date it occurred.
type Extreme struct {
	Minutes int       // worked minutes (for longest/shortest)
	Clock   int       // minutes-since-midnight (for earliest/latest)
	Date    time.Time // the day it occurred
	Valid   bool      // false when no qualifying day exists
}

// Summary holds all computed yearly metrics.
type Summary struct {
	Year int

	DaysWorked int // days with both start and end
	DaysOff    int // off days
	DaysOpen   int // clocked in but not out

	TotalWorked   int // net worked minutes across worked days
	TotalExpected int // expected minutes across worked days
	Balance       int // TotalWorked - TotalExpected
	TotalLunch    int // lunch minutes deducted across worked days

	AvgWorkedPerDay  int // TotalWorked / DaysWorked
	AvgBalancePerDay int // Balance / DaysWorked
	AvgArrival       int // mean start clock (minutes since midnight)
	AvgDeparture     int // mean end clock (minutes since midnight)

	Longest       Extreme
	Shortest      Extreme
	EarliestStart Extreme
	LatestFinish  Extreme

	WeeksActive   int // distinct ISO weeks with worked time
	WeeksLogged   int // of those, how many are logged
	WeeksUnlogged int // active weeks not yet logged

	// ByWeekday[0]=Monday .. [6]=Sunday: total worked minutes.
	ByWeekday [7]int
	// ByMonth[0]=Jan .. [11]=Dec: total worked minutes.
	ByMonth [12]int

	HasData bool // false when no worked days at all
}

// weekKey is a comparable ISO year+week pair.
type weekKey struct{ year, week int }

// WeekKey is the public ISO year+week pair callers use to report logged weeks,
// so the app layer (which owns week_status access) can build the loggedWeeks
// map without importing internal week types.
type WeekKey struct {
	Year int
	Week int
}

// Compute builds a Summary for the given year from the day records (which the
// caller has already filtered/loaded for that year). loggedWeeks reports, for
// an ISO year-week, whether it has been logged; lookups for weeks not present
// default to false (unlogged).
func Compute(year int, days []*store.Day, loggedWeeks map[WeekKey]bool) *Summary {
	s := &Summary{Year: year}

	var (
		arrivalSum, departureSum int
		clockedCount             int
	)
	active := map[weekKey]bool{}

	for _, d := range days {
		switch {
		case d.IsOff:
			s.DaysOff++
			continue
		case d.Start != nil && d.End == nil:
			s.DaysOpen++
			continue
		case d.Start == nil || d.End == nil:
			continue
		}

		// A fully clocked day.
		worked := calc.WorkedMinutes(*d.Start, *d.End, d.EffectiveLunch())
		s.DaysWorked++
		s.TotalWorked += worked
		s.TotalExpected += d.ExpectedMinutes
		s.TotalLunch += d.EffectiveLunch()
		s.HasData = true

		// Weekday / month distribution (Go: Sunday=0..Saturday=6; remap Monday=0).
		wd := (int(d.Date.Weekday()) + 6) % 7
		s.ByWeekday[wd] += worked
		s.ByMonth[int(d.Date.Month())-1] += worked

		// Arrival / departure clocks.
		startClock := d.Start.Hour()*60 + d.Start.Minute()
		endClock := d.End.Hour()*60 + d.End.Minute()
		arrivalSum += startClock
		departureSum += endClock
		clockedCount++

		// Extremes.
		if !s.Longest.Valid || worked > s.Longest.Minutes {
			s.Longest = Extreme{Minutes: worked, Date: d.Date, Valid: true}
		}
		if !s.Shortest.Valid || worked < s.Shortest.Minutes {
			s.Shortest = Extreme{Minutes: worked, Date: d.Date, Valid: true}
		}
		if !s.EarliestStart.Valid || startClock < s.EarliestStart.Clock {
			s.EarliestStart = Extreme{Clock: startClock, Date: d.Date, Valid: true}
		}
		if !s.LatestFinish.Valid || endClock > s.LatestFinish.Clock {
			s.LatestFinish = Extreme{Clock: endClock, Date: d.Date, Valid: true}
		}

		y, w := d.Date.ISOWeek()
		active[weekKey{y, w}] = true
	}

	s.Balance = s.TotalWorked - s.TotalExpected
	if s.DaysWorked > 0 {
		s.AvgWorkedPerDay = s.TotalWorked / s.DaysWorked
		s.AvgBalancePerDay = s.Balance / s.DaysWorked
	}
	if clockedCount > 0 {
		s.AvgArrival = arrivalSum / clockedCount
		s.AvgDeparture = departureSum / clockedCount
	}

	s.WeeksActive = len(active)
	for k := range active {
		if loggedWeeks[WeekKey{Year: k.year, Week: k.week}] {
			s.WeeksLogged++
		}
	}
	s.WeeksUnlogged = s.WeeksActive - s.WeeksLogged

	return s
}
