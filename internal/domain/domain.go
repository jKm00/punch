// Package domain holds the hardcoded constants and core value types for punch.
package domain

import "time"

// Season represents the expected-hours regime in effect.
type Season string

const (
	Winter Season = "winter"
	Summer Season = "summer"
)

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

// Default typical end-of-day (departure) times by season — the wall-clock time
// you normally leave work. Used as the anchor for the suggested weekly logging
// range. These are fallbacks when not configured.
const (
	WinterEndOfDayHour   = 16
	WinterEndOfDayMinute = 0
	SummerEndOfDayHour   = 15
	SummerEndOfDayMinute = 30
)

// Default summer-period boundaries (inclusive), expressed as recurring
// month/day pairs. These anchor the date-driven season derivation when seasons
// are enabled. Defaults: 15.05 – 31.08.
const (
	SummerStartMonth = 5
	SummerStartDay   = 15
	SummerEndMonth   = 8
	SummerEndDay     = 31
)

// TimeOfDay is a wall-clock hour:minute, used for the configurable typical
// end-of-day times.
type TimeOfDay struct {
	Hour   int
	Minute int
}

// MonthDay is a year-agnostic recurring calendar point (month + day). It is
// used to express the start and end of the summer period without binding to a
// specific year.
type MonthDay struct {
	Month int
	Day   int
}

// ordinal collapses a MonthDay into a comparable month*100+day value.
func (md MonthDay) ordinal() int { return md.Month*100 + md.Day }

// Config holds the resolved configuration values for one CLI invocation. Each
// field is loaded from persisted settings, falling back to the package
// constants above when a value is absent. The store owns loading; domain and
// calc stay pure and receive a Config as a parameter.
type Config struct {
	WinterExpectedMinutes int
	SummerExpectedMinutes int
	WinterEndOfDay        TimeOfDay
	SummerEndOfDay        TimeOfDay
	DefaultLunchMinutes   int

	// SeasonsEnabled controls whether separate summer/winter schedules apply.
	// When false the winter slot is used year-round and SeasonFor always
	// returns Winter.
	SeasonsEnabled bool
	// SummerStart and SummerEnd bound the summer period (inclusive) as
	// recurring month/day pairs.
	SummerStart MonthDay
	SummerEnd   MonthDay
}

// DefaultConfig returns a Config populated entirely from the package constants.
// It is the value a fresh install resolves to before any setup is performed.
func DefaultConfig() Config {
	return Config{
		WinterExpectedMinutes: WinterExpectedMinutes,
		SummerExpectedMinutes: SummerExpectedMinutes,
		WinterEndOfDay:        TimeOfDay{Hour: WinterEndOfDayHour, Minute: WinterEndOfDayMinute},
		SummerEndOfDay:        TimeOfDay{Hour: SummerEndOfDayHour, Minute: SummerEndOfDayMinute},
		DefaultLunchMinutes:   DefaultLunchMinutes,
		SeasonsEnabled:        true,
		SummerStart:           MonthDay{Month: SummerStartMonth, Day: SummerStartDay},
		SummerEnd:             MonthDay{Month: SummerEndMonth, Day: SummerEndDay},
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

// EndOfDayFor returns the season's typical end-of-day (departure) time as an
// hour and minute — the anchor for the suggested weekly logging range.
func (cfg Config) EndOfDayFor(s Season) (hour, minute int) {
	if s == Summer {
		return cfg.SummerEndOfDay.Hour, cfg.SummerEndOfDay.Minute
	}
	return cfg.WinterEndOfDay.Hour, cfg.WinterEndOfDay.Minute
}

// SeasonFor derives the season in effect on the given date. When seasons are
// disabled the winter (year-round) slot is always returned. Otherwise the date
// is compared against the configured summer period (inclusive boundaries),
// handling intervals that wrap across the new year.
func (cfg Config) SeasonFor(date time.Time) Season {
	if !cfg.SeasonsEnabled {
		return Winter
	}
	curOrd := int(date.Month())*100 + date.Day()
	startOrd := cfg.SummerStart.ordinal()
	endOrd := cfg.SummerEnd.ordinal()
	if startOrd <= endOrd {
		if startOrd <= curOrd && curOrd <= endOrd {
			return Summer
		}
		return Winter
	}
	// Wrap-around interval (e.g. 01.11 – 28.02).
	if curOrd >= startOrd || curOrd <= endOrd {
		return Summer
	}
	return Winter
}
