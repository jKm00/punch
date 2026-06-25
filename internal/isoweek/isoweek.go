// Package isoweek provides ISO-8601 week helpers (weeks start Monday and a
// week belongs to the year that contains its Thursday).
package isoweek

import (
	"fmt"
	"time"
)

// Key formats an ISO year-week key like "2026-W26".
func Key(year, week int) string {
	return fmt.Sprintf("%04d-W%02d", year, week)
}

// Of returns the ISO year and week number for the given time.
func Of(t time.Time) (year, week int) {
	return t.ISOWeek()
}

// KeyOf returns the ISO year-week key for the given time.
func KeyOf(t time.Time) string {
	y, w := t.ISOWeek()
	return Key(y, w)
}

// Monday returns the Monday (00:00, in loc) of the given ISO year/week.
func Monday(year, week int, loc *time.Location) time.Time {
	// Jan 4th is always in ISO week 1.
	jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, loc)
	// Find the Monday of week 1.
	isoWeekday := int(jan4.Weekday())
	if isoWeekday == 0 { // Sunday -> 7
		isoWeekday = 7
	}
	week1Monday := jan4.AddDate(0, 0, -(isoWeekday - 1))
	return week1Monday.AddDate(0, 0, (week-1)*7)
}

// Range returns the Monday and Sunday (both at 00:00) for the given ISO
// year/week.
func Range(year, week int, loc *time.Location) (monday, sunday time.Time) {
	monday = Monday(year, week, loc)
	sunday = monday.AddDate(0, 0, 6)
	return monday, sunday
}
