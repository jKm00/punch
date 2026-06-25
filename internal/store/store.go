// Package store wraps the SQLite persistence layer for punch. Each CLI invocation
// opens the DB, does its work, and closes it again.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite"

	"punch/internal/domain"
)

// DateLayout is the ISO date format used for the days.date primary key.
const DateLayout = "2006-01-02"

// timeLayout is the full ISO datetime stored for start/end and logged_at.
const timeLayout = time.RFC3339

// Day is one persisted day record.
type Day struct {
	Date            time.Time // calendar date (00:00, local)
	Start           *time.Time
	End             *time.Time
	LunchMinutes    *int // nil => use the configured default lunch
	ExpectedMinutes int
	IsOff           bool
}

// EffectiveLunch returns the lunch minutes to apply for this day, using
// defaultLunch (the configured default) when the day has no per-day override.
func (d Day) EffectiveLunch(defaultLunch int) int {
	if d.LunchMinutes != nil {
		return *d.LunchMinutes
	}
	return defaultLunch
}

// Store is a handle to the SQLite database.
type Store struct {
	db  *sql.DB
	loc *time.Location
}

// DefaultPath resolves the DB path, honoring PUNCH_DB (then the legacy WH_DB),
// then XDG_DATA_HOME, then ~/.local/share/punch/punch.db.
func DefaultPath() (string, error) {
	if v := os.Getenv("PUNCH_DB"); v != "" {
		return v, nil
	}
	if v := os.Getenv("WH_DB"); v != "" {
		return v, nil
	}
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "punch", "punch.db"), nil
}

// Open opens (creating if needed) the SQLite DB at path and ensures the schema
// exists. loc is the time zone used for parsing/formatting stored times.
func Open(path string, loc *time.Location) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	s := &Store{db: db, loc: loc}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL: %w", err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("set foreign_keys: %w", err)
	}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS days (
	date             TEXT PRIMARY KEY,
	start            TEXT,
	end              TEXT,
	lunch_minutes    INTEGER,
	expected_minutes INTEGER NOT NULL,
	is_off           INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS week_status (
	year_week TEXT PRIMARY KEY,
	logged_at TEXT
);
CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`
	_, err := s.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	// One-time rename: the logging-start settings keys were renamed to
	// end-of-day to match the configured value's true meaning (the typical
	// departure time). Rename in place so existing custom values are preserved.
	// Idempotent: a no-op once renamed, and harmless if the old keys never
	// existed.
	const renameKeys = `
UPDATE OR IGNORE settings SET key = 'winter_end_of_day' WHERE key = 'winter_logging_start';
UPDATE OR IGNORE settings SET key = 'summer_end_of_day' WHERE key = 'summer_logging_start';
`
	if _, err := s.db.Exec(renameKeys); err != nil {
		return fmt.Errorf("migrate (rename keys): %w", err)
	}
	return nil
}

// ---- settings ----

const seasonKey = "season"

// Settings keys for the configurable values written by the setup wizard. Each
// is one row in the generic key/value settings table. When a key is absent the
// corresponding domain constant is used as a fallback (see LoadConfig).
const (
	winterExpectedMinutesKey = "winter_expected_minutes"
	summerExpectedMinutesKey = "summer_expected_minutes"
	winterEndOfDayKey        = "winter_end_of_day"
	summerEndOfDayKey        = "summer_end_of_day"
	defaultLunchMinutesKey   = "default_lunch_minutes"

	// setupCompletedKey is the sentinel written LAST after a successful setup
	// run. Its presence marks setup as complete.
	setupCompletedKey = "setup_completed"
)

// getSetting returns the raw value for a settings key and whether it was found.
func (s *Store) getSetting(key string) (string, bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// setSetting upserts a settings key/value pair.
func (s *Store) setSetting(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO settings(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	return err
}

// SetupCompleted reports whether the setup wizard has been completed (the
// setup_completed sentinel key is present).
func (s *Store) SetupCompleted() (bool, error) {
	_, ok, err := s.getSetting(setupCompletedKey)
	return ok, err
}

// SaveConfig persists every configurable value and, last of all, the
// setup_completed sentinel. Values are written sequentially; the sentinel is
// written only after all others succeed so an interrupted run is not treated as
// complete. season is written via the same key as `punch season`.
func (s *Store) SaveConfig(cfg domain.Config, season domain.Season) error {
	writes := []struct{ key, value string }{
		{winterExpectedMinutesKey, strconv.Itoa(cfg.WinterExpectedMinutes)},
		{summerExpectedMinutesKey, strconv.Itoa(cfg.SummerExpectedMinutes)},
		{winterEndOfDayKey, formatTimeOfDay(cfg.WinterEndOfDay)},
		{summerEndOfDayKey, formatTimeOfDay(cfg.SummerEndOfDay)},
		{defaultLunchMinutesKey, strconv.Itoa(cfg.DefaultLunchMinutes)},
		{seasonKey, string(season)},
	}
	for _, w := range writes {
		if err := s.setSetting(w.key, w.value); err != nil {
			return err
		}
	}
	// setup_completed must be written last.
	return s.setSetting(setupCompletedKey, "true")
}

// LoadConfig resolves the configuration for this invocation: for each key it
// reads the stored value, falling back to the corresponding domain constant
// when absent. Malformed stored values fall back to the constant too, so a bad
// row never breaks the CLI.
func (s *Store) LoadConfig() (domain.Config, error) {
	cfg := domain.DefaultConfig()

	if v, ok, err := s.getSetting(winterExpectedMinutesKey); err != nil {
		return cfg, err
	} else if ok {
		if n, perr := strconv.Atoi(v); perr == nil {
			cfg.WinterExpectedMinutes = n
		}
	}
	if v, ok, err := s.getSetting(summerExpectedMinutesKey); err != nil {
		return cfg, err
	} else if ok {
		if n, perr := strconv.Atoi(v); perr == nil {
			cfg.SummerExpectedMinutes = n
		}
	}
	if v, ok, err := s.getSetting(winterEndOfDayKey); err != nil {
		return cfg, err
	} else if ok {
		if tod, perr := parseTimeOfDay(v); perr == nil {
			cfg.WinterEndOfDay = tod
		}
	}
	if v, ok, err := s.getSetting(summerEndOfDayKey); err != nil {
		return cfg, err
	} else if ok {
		if tod, perr := parseTimeOfDay(v); perr == nil {
			cfg.SummerEndOfDay = tod
		}
	}
	if v, ok, err := s.getSetting(defaultLunchMinutesKey); err != nil {
		return cfg, err
	} else if ok {
		if n, perr := strconv.Atoi(v); perr == nil {
			cfg.DefaultLunchMinutes = n
		}
	}
	return cfg, nil
}

// formatTimeOfDay renders a TimeOfDay as "HH:MM" for storage.
func formatTimeOfDay(t domain.TimeOfDay) string {
	return fmt.Sprintf("%02d:%02d", t.Hour, t.Minute)
}

// parseTimeOfDay parses a stored "HH:MM" value into a TimeOfDay.
func parseTimeOfDay(v string) (domain.TimeOfDay, error) {
	t, err := time.Parse("15:04", v)
	if err != nil {
		return domain.TimeOfDay{}, fmt.Errorf("parse time-of-day %q: %w", v, err)
	}
	return domain.TimeOfDay{Hour: t.Hour(), Minute: t.Minute()}, nil
}

// Season returns the configured season, defaulting when unset.
func (s *Store) Season() (domain.Season, error) {
	v, ok, err := s.getSetting(seasonKey)
	if err != nil {
		return "", err
	}
	if !ok {
		return domain.DefaultSeason, nil
	}
	return domain.Normalize(v), nil
}

// SetSeason persists the season.
func (s *Store) SetSeason(season domain.Season) error {
	return s.setSetting(seasonKey, string(season))
}

// ---- days ----

// GetDay loads a day record by date, or (nil, nil) if not present.
func (s *Store) GetDay(date time.Time) (*Day, error) {
	key := date.Format(DateLayout)
	row := s.db.QueryRow(
		`SELECT date, start, end, lunch_minutes, expected_minutes, is_off FROM days WHERE date = ?`, key)
	return s.scanDay(row)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (s *Store) scanDay(row rowScanner) (*Day, error) {
	var (
		dateStr  string
		startStr sql.NullString
		endStr   sql.NullString
		lunch    sql.NullInt64
		expected int
		isOff    int
	)
	err := row.Scan(&dateStr, &startStr, &endStr, &lunch, &expected, &isOff)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	date, err := time.ParseInLocation(DateLayout, dateStr, s.loc)
	if err != nil {
		return nil, fmt.Errorf("parse stored date %q: %w", dateStr, err)
	}
	d := &Day{
		Date:            date,
		ExpectedMinutes: expected,
		IsOff:           isOff != 0,
	}
	if startStr.Valid {
		t, err := s.parseStoredTime(startStr.String, date)
		if err != nil {
			return nil, err
		}
		d.Start = &t
	}
	if endStr.Valid {
		t, err := s.parseStoredTime(endStr.String, date)
		if err != nil {
			return nil, err
		}
		d.End = &t
	}
	if lunch.Valid {
		v := int(lunch.Int64)
		d.LunchMinutes = &v
	}
	return d, nil
}

// parseStoredTime accepts either a full RFC3339 datetime or a bare HH:MM
// (combined with the day's date) for robustness.
func (s *Store) parseStoredTime(v string, date time.Time) (time.Time, error) {
	if t, err := time.ParseInLocation(timeLayout, v, s.loc); err == nil {
		return t.In(s.loc), nil
	}
	if t, err := time.ParseInLocation("15:04", v, s.loc); err == nil {
		return time.Date(date.Year(), date.Month(), date.Day(), t.Hour(), t.Minute(), 0, 0, s.loc), nil
	}
	return time.Time{}, fmt.Errorf("parse stored time %q", v)
}

// UpsertDay inserts or replaces a day record.
func (s *Store) UpsertDay(d *Day) error {
	key := d.Date.Format(DateLayout)
	var startVal, endVal any
	if d.Start != nil {
		startVal = d.Start.Format(timeLayout)
	}
	if d.End != nil {
		endVal = d.End.Format(timeLayout)
	}
	var lunchVal any
	if d.LunchMinutes != nil {
		lunchVal = *d.LunchMinutes
	}
	_, err := s.db.Exec(
		`INSERT INTO days(date, start, end, lunch_minutes, expected_minutes, is_off)
		 VALUES(?, ?, ?, ?, ?, ?)
		 ON CONFLICT(date) DO UPDATE SET
			start = excluded.start,
			end = excluded.end,
			lunch_minutes = excluded.lunch_minutes,
			expected_minutes = excluded.expected_minutes,
			is_off = excluded.is_off`,
		key, startVal, endVal, lunchVal, d.ExpectedMinutes, boolToInt(d.IsOff))
	return err
}

// DeleteDay removes a day record. Returns true if a row was deleted.
func (s *Store) DeleteDay(date time.Time) (bool, error) {
	key := date.Format(DateLayout)
	res, err := s.db.Exec(`DELETE FROM days WHERE date = ?`, key)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DaysInRange returns all day records with date in [from, to] inclusive,
// ordered by date ascending.
func (s *Store) DaysInRange(from, to time.Time) ([]*Day, error) {
	rows, err := s.db.Query(
		`SELECT date, start, end, lunch_minutes, expected_minutes, is_off
		 FROM days WHERE date >= ? AND date <= ? ORDER BY date ASC`,
		from.Format(DateLayout), to.Format(DateLayout))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Day
	for rows.Next() {
		d, err := s.scanDay(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// AllWorkedDayKeys returns every distinct year-week key that has at least one
// day record with a start time set (i.e. some worked/clocked activity),
// ordered ascending by date.
func (s *Store) EarliestWorkedDate() (*time.Time, error) {
	row := s.db.QueryRow(`SELECT MIN(date) FROM days WHERE start IS NOT NULL`)
	var v sql.NullString
	if err := row.Scan(&v); err != nil {
		return nil, err
	}
	if !v.Valid {
		return nil, nil
	}
	t, err := time.ParseInLocation(DateLayout, v.String, s.loc)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ---- week_status ----

// WeekLoggedAt returns the logged-at timestamp for a year-week, or nil if the
// week is not (yet) logged.
func (s *Store) WeekLoggedAt(yearWeek string) (*time.Time, error) {
	var v sql.NullString
	err := s.db.QueryRow(`SELECT logged_at FROM week_status WHERE year_week = ?`, yearWeek).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !v.Valid {
		return nil, nil
	}
	t, err := time.ParseInLocation(timeLayout, v.String, s.loc)
	if err != nil {
		return nil, err
	}
	tt := t.In(s.loc)
	return &tt, nil
}

// SetWeekLogged records logged_at for a year-week.
func (s *Store) SetWeekLogged(yearWeek string, at time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO week_status(year_week, logged_at) VALUES(?, ?)
		 ON CONFLICT(year_week) DO UPDATE SET logged_at = excluded.logged_at`,
		yearWeek, at.Format(timeLayout))
	return err
}

// ClearWeekLogged removes the logged state for a year-week. It is a no-op if the
// week was not logged.
func (s *Store) ClearWeekLogged(yearWeek string) error {
	_, err := s.db.Exec(`DELETE FROM week_status WHERE year_week = ?`, yearWeek)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
