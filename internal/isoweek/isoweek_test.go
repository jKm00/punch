package isoweek

import (
	"testing"
	"time"
)

func TestOf(t *testing.T) {
	tests := []struct {
		date     time.Time
		wantYear int
		wantWeek int
	}{
		// 2026-06-24 is a Wednesday in ISO week 26.
		{time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC), 2026, 26},
		// 2026-01-01 is a Thursday -> ISO week 1 of 2026.
		{time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), 2026, 1},
		// 2021-01-01 is a Friday -> ISO week 53 of 2020.
		{time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC), 2020, 53},
		// 2024-12-30 is a Monday -> ISO week 1 of 2025.
		{time.Date(2024, 12, 30, 0, 0, 0, 0, time.UTC), 2025, 1},
	}
	for _, tc := range tests {
		y, w := Of(tc.date)
		if y != tc.wantYear || w != tc.wantWeek {
			t.Errorf("Of(%s) = %d-W%02d, want %d-W%02d", tc.date.Format("2006-01-02"), y, w, tc.wantYear, tc.wantWeek)
		}
	}
}

func TestRange(t *testing.T) {
	// ISO week 26 of 2026 runs Mon 2026-06-22 .. Sun 2026-06-28.
	mon, sun := Range(2026, 26, time.UTC)
	if mon.Format("2006-01-02") != "2026-06-22" {
		t.Errorf("Range monday = %s, want 2026-06-22", mon.Format("2006-01-02"))
	}
	if sun.Format("2006-01-02") != "2026-06-28" {
		t.Errorf("Range sunday = %s, want 2026-06-28", sun.Format("2006-01-02"))
	}
	if mon.Weekday() != time.Monday {
		t.Errorf("monday weekday = %s", mon.Weekday())
	}
	if sun.Weekday() != time.Sunday {
		t.Errorf("sunday weekday = %s", sun.Weekday())
	}
}

func TestMondayRoundTrip(t *testing.T) {
	// The Monday of a week should map back to the same ISO year/week.
	for _, tc := range []struct{ y, w int }{{2026, 1}, {2026, 26}, {2025, 1}, {2020, 53}} {
		mon := Monday(tc.y, tc.w, time.UTC)
		y, w := Of(mon)
		if y != tc.y || w != tc.w {
			t.Errorf("Monday(%d,%d) -> Of = %d-W%02d", tc.y, tc.w, y, w)
		}
	}
}

func TestKey(t *testing.T) {
	if got := Key(2026, 26); got != "2026-W26" {
		t.Errorf("Key(2026,26) = %q, want 2026-W26", got)
	}
	if got := Key(2026, 1); got != "2026-W01" {
		t.Errorf("Key(2026,1) = %q, want 2026-W01", got)
	}
}
