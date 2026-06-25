// Package calc holds pure work-hour calculations: net worked minutes, weekly
// balances, and the official-app logging-range end time.
package calc

import (
	"fmt"
	"time"

	"punch/internal/domain"
)

// WorkedMinutes returns net worked minutes for a clocked day:
// (end - start) - lunch, clamped to >= 0. Lunch is in minutes.
//
// start and end are wall-clock times on the same calendar day; the caller is
// responsible for rejecting overnight shifts (end < start).
func WorkedMinutes(start, end time.Time, lunchMinutes int) int {
	gross := int(end.Sub(start).Minutes())
	net := gross - lunchMinutes
	if net < 0 {
		return 0
	}
	return net
}

// LogRange computes the suggested weekly logging range to enter in the
// company's work-hour app: it anchors at the season's typical end-of-day
// (departure) time (resolved from cfg) and extends by the weekly extra
// (overtime) in minutes, returning the start and end hour/minute. Extra is
// assumed > 0; the result may roll past midnight in which case hour is taken
// modulo 24.
func LogRange(cfg domain.Config, s domain.Season, extraMinutes int) (startHour, startMin, endHour, endMin int) {
	startHour, startMin = cfg.EndOfDayFor(s)
	total := startHour*60 + startMin + extraMinutes
	endHour = (total / 60) % 24
	endMin = total % 60
	return startHour, startMin, endHour, endMin
}

// FormatHM renders a (possibly negative) minute count as e.g. "7h30m",
// "-45m", "0m".
func FormatHM(minutes int) string {
	neg := minutes < 0
	if neg {
		minutes = -minutes
	}
	h := minutes / 60
	m := minutes % 60
	var s string
	switch {
	case h > 0 && m > 0:
		s = fmt.Sprintf("%dh%02dm", h, m)
	case h > 0:
		s = fmt.Sprintf("%dh", h)
	default:
		s = fmt.Sprintf("%dm", m)
	}
	if neg {
		return "-" + s
	}
	return s
}

// FormatDecimalHours renders minutes as decimal hours, e.g. "7.5h", "-0.75h".
func FormatDecimalHours(minutes int) string {
	return fmt.Sprintf("%.2fh", float64(minutes)/60.0)
}

// FormatClock renders hour/minute as "HH:MM".
func FormatClock(hour, minute int) string {
	return fmt.Sprintf("%02d:%02d", hour, minute)
}

// FormatClockMinutes renders minutes-since-midnight as "HH:MM".
func FormatClockMinutes(minutes int) string {
	if minutes < 0 {
		minutes = 0
	}
	return fmt.Sprintf("%02d:%02d", (minutes/60)%24, minutes%60)
}
