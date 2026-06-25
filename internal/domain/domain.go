// Package domain holds the hardcoded constants and core value types for punch.
package domain

// Season represents the expected-hours regime in effect.
type Season string

const (
	Winter Season = "winter"
	Summer Season = "summer"
)

// DefaultSeason is used when no season has been configured.
const DefaultSeason = Winter

// Hardcoded constants (all expressed in minutes unless noted). These serve as
// the fallback defaults when the corresponding configuration value has not been
// set via the setup wizard (`punch setup`).
const (
	// DefaultLunchMinutes is deducted on every clocked day unless overridden.
	DefaultLunchMinutes = 30

	// Expected work minutes per day, by season.
	WinterExpectedMinutes = 7*60 + 30 // 7h30m = 450
	SummerExpectedMinutes = 7 * 60    // 7h    = 420

	// ClockAdjustMinutes is applied to bare `punch in` (-5) and `punch out` (+5).
	ClockAdjustMinutes = 5

	// LongDayMinutes: days worked beyond this trigger a warning (but are allowed).
	LongDayMinutes = 16 * 60
)

// Default logging-start times by season (the earliest wall-clock time at which
// logging is expected). These are fallbacks when not configured.
const (
	WinterLoggingStartHour   = 16
	WinterLoggingStartMinute = 0
	SummerLoggingStartHour   = 15
	SummerLoggingStartMinute = 30
)

// TimeOfDay is a wall-clock hour:minute, used for configurable logging-start
// times.
type TimeOfDay struct {
	Hour   int
	Minute int
}

// Config holds the resolved configuration values for one CLI invocation. Each
// field is loaded from persisted settings, falling back to the package
// constants above when a value is absent. The store owns loading; domain and
// calc stay pure and receive a Config as a parameter.
type Config struct {
	WinterExpectedMinutes int
	SummerExpectedMinutes int
	WinterLoggingStart    TimeOfDay
	SummerLoggingStart    TimeOfDay
	DefaultLunchMinutes   int
}

// DefaultConfig returns a Config populated entirely from the package constants.
// It is the value a fresh install resolves to before any setup is performed.
func DefaultConfig() Config {
	return Config{
		WinterExpectedMinutes: WinterExpectedMinutes,
		SummerExpectedMinutes: SummerExpectedMinutes,
		WinterLoggingStart:    TimeOfDay{Hour: WinterLoggingStartHour, Minute: WinterLoggingStartMinute},
		SummerLoggingStart:    TimeOfDay{Hour: SummerLoggingStartHour, Minute: SummerLoggingStartMinute},
		DefaultLunchMinutes:   DefaultLunchMinutes,
	}
}

// ExpectedMinutesFor returns the expected work minutes for a normal (non-off)
// day in the given season.
func (cfg Config) ExpectedMinutesFor(s Season) int {
	if s == Summer {
		return cfg.SummerExpectedMinutes
	}
	return cfg.WinterExpectedMinutes
}

// LoggingStartFor returns the season's logging start time as an hour and
// minute (the earliest wall-clock time at which logging is expected).
func (cfg Config) LoggingStartFor(s Season) (hour, minute int) {
	if s == Summer {
		return cfg.SummerLoggingStart.Hour, cfg.SummerLoggingStart.Minute
	}
	return cfg.WinterLoggingStart.Hour, cfg.WinterLoggingStart.Minute
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
