// Package domain holds the hardcoded constants and core value types for wh.
package domain

// Season represents the expected-hours regime in effect.
type Season string

const (
	Winter Season = "winter"
	Summer Season = "summer"
)

// DefaultSeason is used when no season has been configured.
const DefaultSeason = Winter

// Hardcoded constants (all expressed in minutes unless noted).
const (
	// DefaultLunchMinutes is deducted on every clocked day unless overridden.
	DefaultLunchMinutes = 30

	// Expected work minutes per day, by season.
	WinterExpectedMinutes = 7*60 + 30 // 7h30m = 450
	SummerExpectedMinutes = 7 * 60    // 7h    = 420

	// ClockAdjustMinutes is applied to bare `wh in` (-5) and `wh out` (+5).
	ClockAdjustMinutes = 5

	// LongDayMinutes: days worked beyond this trigger a warning (but are allowed).
	LongDayMinutes = 16 * 60
)

// ExpectedMinutesFor returns the expected work minutes for a normal (non-off)
// day in the given season.
func ExpectedMinutesFor(s Season) int {
	if s == Summer {
		return SummerExpectedMinutes
	}
	return WinterExpectedMinutes
}

// LoggingStartFor returns the season's logging start time as an hour and
// minute (the earliest wall-clock time at which logging is expected).
func LoggingStartFor(s Season) (hour, minute int) {
	if s == Summer {
		return 15, 30
	}
	return 16, 0
}

// Normalize coerces an arbitrary season string to a known Season, defaulting
// to the configured default for unknown/empty values.
func Normalize(s string) Season {
	switch Season(s) {
	case Summer:
		return Summer
	case Winter:
		return Winter
	default:
		return DefaultSeason
	}
}
