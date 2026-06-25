package analytics

import (
	"testing"
	"time"

	"punch/internal/store"
)

func tm(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.ParseInLocation("2006-01-02 15:04", s, time.UTC)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

func dateOf(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.ParseInLocation("2006-01-02", s, time.UTC)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

func lunch(n int) *int { return &n }

func TestComputeEmptyHasNoData(t *testing.T) {
	s := Compute(2025, nil, nil)
	if s.HasData {
		t.Error("expected HasData=false for no days")
	}
	if s.DaysWorked != 0 || s.TotalWorked != 0 {
		t.Error("expected zero totals")
	}
}

func TestComputeBasicTotalsAndAverages(t *testing.T) {
	// Two fully-clocked days, one off, one open.
	mon := dateOf(t, "2025-01-06") // ISO week 2 Monday
	tue := dateOf(t, "2025-01-07")
	wed := dateOf(t, "2025-01-08")
	thu := dateOf(t, "2025-01-09")

	mkClocked := func(date time.Time, start, end string, expected int) *store.Day {
		s := tm(t, date.Format("2006-01-02")+" "+start)
		e := tm(t, date.Format("2006-01-02")+" "+end)
		return &store.Day{Date: date, Start: &s, End: &e, LunchMinutes: lunch(30), ExpectedMinutes: expected}
	}

	days := []*store.Day{
		mkClocked(mon, "08:00", "16:00", 450), // gross 480 - 30 = 450 worked, expected 450 -> bal 0
		mkClocked(tue, "09:00", "16:00", 450), // gross 420 - 30 = 390 worked, expected 450 -> bal -60
		{Date: wed, IsOff: true},
		{Date: thu, Start: ptr(tm(t, "2025-01-09 08:00"))}, // open (no end)
	}

	s := Compute(2025, days, map[WeekKey]bool{})

	if !s.HasData {
		t.Fatal("expected HasData=true")
	}
	if s.DaysWorked != 2 {
		t.Errorf("DaysWorked = %d, want 2", s.DaysWorked)
	}
	if s.DaysOff != 1 {
		t.Errorf("DaysOff = %d, want 1", s.DaysOff)
	}
	if s.DaysOpen != 1 {
		t.Errorf("DaysOpen = %d, want 1", s.DaysOpen)
	}
	if s.TotalWorked != 840 { // 450 + 390
		t.Errorf("TotalWorked = %d, want 840", s.TotalWorked)
	}
	if s.TotalExpected != 900 {
		t.Errorf("TotalExpected = %d, want 900", s.TotalExpected)
	}
	if s.Balance != -60 {
		t.Errorf("Balance = %d, want -60", s.Balance)
	}
	if s.TotalLunch != 60 {
		t.Errorf("TotalLunch = %d, want 60", s.TotalLunch)
	}
	if s.AvgWorkedPerDay != 420 { // 840/2
		t.Errorf("AvgWorkedPerDay = %d, want 420", s.AvgWorkedPerDay)
	}
	if s.AvgBalancePerDay != -30 { // -60/2
		t.Errorf("AvgBalancePerDay = %d, want -30", s.AvgBalancePerDay)
	}
	// Avg arrival = (480 + 540)/2 = 510 = 08:30; departure both 16:00 = 960.
	if s.AvgArrival != 510 {
		t.Errorf("AvgArrival = %d, want 510", s.AvgArrival)
	}
	if s.AvgDeparture != 960 {
		t.Errorf("AvgDeparture = %d, want 960", s.AvgDeparture)
	}
}

func TestComputeExtremes(t *testing.T) {
	d1 := dateOf(t, "2025-03-03")
	d2 := dateOf(t, "2025-03-04")
	mk := func(date time.Time, start, end string) *store.Day {
		s := tm(t, date.Format("2006-01-02")+" "+start)
		e := tm(t, date.Format("2006-01-02")+" "+end)
		return &store.Day{Date: date, Start: &s, End: &e, LunchMinutes: lunch(0), ExpectedMinutes: 450}
	}
	days := []*store.Day{
		mk(d1, "07:00", "17:00"), // 600 worked, start 420, end 1020
		mk(d2, "09:30", "15:00"), // 330 worked, start 570, end 900
	}
	s := Compute(2025, days, nil)

	if !s.Longest.Valid || s.Longest.Minutes != 600 || !s.Longest.Date.Equal(d1) {
		t.Errorf("Longest wrong: %+v", s.Longest)
	}
	if !s.Shortest.Valid || s.Shortest.Minutes != 330 || !s.Shortest.Date.Equal(d2) {
		t.Errorf("Shortest wrong: %+v", s.Shortest)
	}
	if !s.EarliestStart.Valid || s.EarliestStart.Clock != 420 || !s.EarliestStart.Date.Equal(d1) {
		t.Errorf("EarliestStart wrong: %+v", s.EarliestStart)
	}
	if !s.LatestFinish.Valid || s.LatestFinish.Clock != 1020 || !s.LatestFinish.Date.Equal(d1) {
		t.Errorf("LatestFinish wrong: %+v", s.LatestFinish)
	}
}

func TestComputeWeekdayAndMonthDistribution(t *testing.T) {
	// 2025-01-06 is a Monday; 2025-02-07 is a Friday.
	mon := dateOf(t, "2025-01-06")
	fri := dateOf(t, "2025-02-07")
	mk := func(date time.Time, start, end string) *store.Day {
		s := tm(t, date.Format("2006-01-02")+" "+start)
		e := tm(t, date.Format("2006-01-02")+" "+end)
		return &store.Day{Date: date, Start: &s, End: &e, LunchMinutes: lunch(0), ExpectedMinutes: 450}
	}
	days := []*store.Day{
		mk(mon, "08:00", "16:00"), // 480, Monday (index 0), January (0)
		mk(fri, "08:00", "12:00"), // 240, Friday (index 4), February (1)
	}
	s := Compute(2025, days, nil)

	if s.ByWeekday[0] != 480 {
		t.Errorf("Monday total = %d, want 480", s.ByWeekday[0])
	}
	if s.ByWeekday[4] != 240 {
		t.Errorf("Friday total = %d, want 240", s.ByWeekday[4])
	}
	if s.ByMonth[0] != 480 {
		t.Errorf("January total = %d, want 480", s.ByMonth[0])
	}
	if s.ByMonth[1] != 240 {
		t.Errorf("February total = %d, want 240", s.ByMonth[1])
	}
}

func TestComputeWeekLoggedCoverage(t *testing.T) {
	// Two days in different ISO weeks; one week logged, one not.
	w2 := dateOf(t, "2025-01-06") // ISO 2025-W02
	w3 := dateOf(t, "2025-01-13") // ISO 2025-W03
	mk := func(date time.Time) *store.Day {
		s := tm(t, date.Format("2006-01-02")+" 08:00")
		e := tm(t, date.Format("2006-01-02")+" 16:00")
		return &store.Day{Date: date, Start: &s, End: &e, ExpectedMinutes: 450}
	}
	y2, wk2 := w2.ISOWeek()
	logged := map[WeekKey]bool{{Year: y2, Week: wk2}: true}

	s := Compute(2025, []*store.Day{mk(w2), mk(w3)}, logged)

	if s.WeeksActive != 2 {
		t.Errorf("WeeksActive = %d, want 2", s.WeeksActive)
	}
	if s.WeeksLogged != 1 {
		t.Errorf("WeeksLogged = %d, want 1", s.WeeksLogged)
	}
	if s.WeeksUnlogged != 1 {
		t.Errorf("WeeksUnlogged = %d, want 1", s.WeeksUnlogged)
	}
}

func ptr(t time.Time) *time.Time { return &t }
