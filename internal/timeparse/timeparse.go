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

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
