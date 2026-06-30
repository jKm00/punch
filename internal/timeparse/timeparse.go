// Package timeparse parses CLI date/time input. Dates are day-first European
// (DD.MM.YYYY / DD-MM-YYYY / DD.MM / DD-MM) plus the keywords today/yesterday.
// Times are 24-hour HH:MM. All values are returned in terms of the supplied
// "now" so the package stays pure and testable.
package timeparse

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseDate parses a day-first European date string relative to now and
// returns the calendar date with the time-of-day zeroed (in now's location).
//
// Accepted forms:
//
//	today, yesterday
//	DD.MM.YYYY, DD-MM-YYYY
//	DD.MM, DD-MM       (year defaults to now's year)
//
// 2-digit years and otherwise ambiguous input are rejected.
func ParseDate(s string, now time.Time) (time.Time, error) {
	trimmed := strings.TrimSpace(s)
	switch strings.ToLower(trimmed) {
	case "today":
		return dateOnly(now), nil
	case "yesterday":
		return dateOnly(now.AddDate(0, 0, -1)), nil
	}

	// Determine the separator; both '.' and '-' are accepted but not mixed.
	var sep string
	switch {
	case strings.Contains(trimmed, "."):
		sep = "."
	case strings.Contains(trimmed, "-"):
		sep = "-"
	default:
		return time.Time{}, fmt.Errorf("invalid date %q: expected DD.MM.YYYY, DD.MM, today or yesterday", s)
	}

	parts := strings.Split(trimmed, sep)
	for _, p := range parts {
		if p == "" {
			return time.Time{}, fmt.Errorf("invalid date %q", s)
		}
	}

	var day, month, year int
	switch len(parts) {
	case 2:
		year = now.Year()
	case 3:
		if len(parts[2]) != 4 {
			return time.Time{}, fmt.Errorf("invalid date %q: year must be 4 digits", s)
		}
		y, err := strconv.Atoi(parts[2])
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid date %q: %v", s, err)
		}
		year = y
	default:
		return time.Time{}, fmt.Errorf("invalid date %q: expected DD%sMM or DD%sMM%sYYYY", s, sep, sep, sep)
	}

	d, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q: bad day", s)
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q: bad month", s)
	}
	day, month = d, m

	if month < 1 || month > 12 {
		return time.Time{}, fmt.Errorf("invalid date %q: month out of range", s)
	}
	if day < 1 || day > 31 {
		return time.Time{}, fmt.Errorf("invalid date %q: day out of range", s)
	}

	result := time.Date(year, time.Month(month), day, 0, 0, 0, 0, now.Location())
	// Reject overflow (e.g. 31.02) by checking round-trip.
	if result.Day() != day || int(result.Month()) != month || result.Year() != year {
		return time.Time{}, fmt.Errorf("invalid date %q: no such calendar day", s)
	}
	return result, nil
}

// ParseTime parses a 24-hour HH:MM time-of-day and returns hour, minute.
func ParseTime(s string) (hour, minute int, err error) {
	trimmed := strings.TrimSpace(s)
	parts := strings.Split(trimmed, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid time %q: expected HH:MM", s)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid time %q: bad hour", s)
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid time %q: bad minute", s)
	}
	if h < 0 || h > 23 {
		return 0, 0, fmt.Errorf("invalid time %q: hour out of range", s)
	}
	if m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("invalid time %q: minute out of range", s)
	}
	return h, m, nil
}

// CombineDateTime returns the given date at the supplied hour:minute, in the
// date's location.
func CombineDateTime(date time.Time, hour, minute int) time.Time {
	return time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, date.Location())
}

// daysInMonth returns the maximum day count for a month (1–12). February uses
// 29 so that 29.02 is accepted for a year-agnostic recurring date.
func daysInMonth(month int) int {
	switch month {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		return 29
	default:
		return 0
	}
}

// ParseMonthDay parses a year-agnostic day-first month/day in the forms DD.MM
// or DD-MM, returning the month and day. The month must be 1–12 and the day
// must be valid for that month (February allows up to 29). Empty input is not
// handled here; callers decide how to treat it (e.g. keep a default).
func ParseMonthDay(s string) (month, day int, err error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return 0, 0, fmt.Errorf("empty month/day")
	}

	var sep string
	switch {
	case strings.Contains(trimmed, "."):
		sep = "."
	case strings.Contains(trimmed, "-"):
		sep = "-"
	default:
		return 0, 0, fmt.Errorf("invalid month/day %q: expected DD.MM or DD-MM", s)
	}

	parts := strings.Split(trimmed, sep)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return 0, 0, fmt.Errorf("invalid month/day %q: expected DD%sMM", s, sep)
	}
	d, derr := strconv.Atoi(parts[0])
	if derr != nil {
		return 0, 0, fmt.Errorf("invalid month/day %q: bad day", s)
	}
	m, merr := strconv.Atoi(parts[1])
	if merr != nil {
		return 0, 0, fmt.Errorf("invalid month/day %q: bad month", s)
	}
	if m < 1 || m > 12 {
		return 0, 0, fmt.Errorf("invalid month/day %q: month out of range", s)
	}
	if d < 1 || d > daysInMonth(m) {
		return 0, 0, fmt.Errorf("invalid month/day %q: day out of range for month", s)
	}
	return m, d, nil
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
