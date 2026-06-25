package app

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"punch/internal/domain"
	"punch/internal/store"
)

// fixedNow is the reference "now" used by the test App: a Wednesday at 12:00.
var fixedNow = time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)

// newTestApp builds an App backed by a temp-file store (via a temp dir) with a
// deterministic clock fixed at fixedNow. Out/Err are captured for assertions.
func newTestApp(t *testing.T) (*App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "wh.db"), time.UTC)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	var out, errb bytes.Buffer
	a := &App{
		Store:  st,
		Now:    func() time.Time { return fixedNow },
		Loc:    time.UTC,
		Out:    &out,
		Err:    &errb,
		Config: domain.DefaultConfig(),
	}
	return a, &out, &errb
}

// TestClockAdjustment verifies the ±5min bare-clock adjustment applies to a
// bare `in`/`out`, while a literal --at time is recorded verbatim.
func TestClockAdjustment(t *testing.T) {
	cases := []struct {
		name      string
		atFlag    string // "" => bare now-case
		wantStart string // HH:MM expected on the stored day
	}{
		{name: "bare in subtracts 5min", atFlag: "", wantStart: "11:55"},
		{name: "literal --at is verbatim", atFlag: "10:00", wantStart: "10:00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, _, _ := newTestApp(t)
			var args []string
			if tc.atFlag != "" {
				args = []string{"--at", tc.atFlag}
			}
			if err := a.CmdIn(args); err != nil {
				t.Fatalf("CmdIn: %v", err)
			}
			day, err := a.Store.GetDay(a.dateOnly(fixedNow))
			if err != nil {
				t.Fatal(err)
			}
			if day == nil || day.Start == nil {
				t.Fatal("expected a clocked-in day")
			}
			if got := day.Start.Format("15:04"); got != tc.wantStart {
				t.Errorf("start = %s, want %s", got, tc.wantStart)
			}
		})
	}
}

// TestOutClockAdjustment verifies a bare `out` adds 5min, while a literal --at
// time is recorded verbatim.
func TestOutClockAdjustment(t *testing.T) {
	cases := []struct {
		name    string
		atFlag  string
		wantEnd string
	}{
		{name: "bare out adds 5min", atFlag: "", wantEnd: "12:05"},
		{name: "literal --at is verbatim", atFlag: "11:30", wantEnd: "11:30"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, _, _ := newTestApp(t)
			// Open a clock-in first (literal, well before now).
			if err := a.CmdIn([]string{"--at", "08:00"}); err != nil {
				t.Fatalf("CmdIn: %v", err)
			}
			var args []string
			if tc.atFlag != "" {
				args = []string{"--at", tc.atFlag}
			}
			if err := a.CmdOut(args); err != nil {
				t.Fatalf("CmdOut: %v", err)
			}
			day, err := a.Store.GetDay(a.dateOnly(fixedNow))
			if err != nil {
				t.Fatal(err)
			}
			if day == nil || day.End == nil {
				t.Fatal("expected a clocked-out day")
			}
			if got := day.End.Format("15:04"); got != tc.wantEnd {
				t.Errorf("end = %s, want %s", got, tc.wantEnd)
			}
		})
	}
}

// TestFutureTimestampGuard verifies that a literal future --at is rejected
// unless --force is given.
func TestFutureTimestampGuard(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{name: "future in rejected", args: []string{"--at", "23:00"}, wantErr: true},
		{name: "future in allowed with --force", args: []string{"--at", "23:00", "--force"}, wantErr: false},
		{name: "past in allowed", args: []string{"--at", "08:00"}, wantErr: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, _, _ := newTestApp(t)
			err := a.CmdIn(tc.args)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr && !strings.Contains(err.Error(), "future") {
				t.Errorf("error = %q, want it to mention 'future'", err)
			}
		})
	}
}

// TestOvernightGuard verifies that end < start is rejected on both `out` and
// `set`.
func TestOvernightGuard(t *testing.T) {
	t.Run("out with end before start", func(t *testing.T) {
		a, _, _ := newTestApp(t)
		if err := a.CmdIn([]string{"--at", "10:00"}); err != nil {
			t.Fatalf("CmdIn: %v", err)
		}
		err := a.CmdOut([]string{"--at", "09:00"})
		if err == nil || !strings.Contains(err.Error(), "before start") {
			t.Fatalf("expected overnight error, got %v", err)
		}
	})
	t.Run("set with end before start", func(t *testing.T) {
		a, _, _ := newTestApp(t)
		err := a.CmdSet([]string{"24.06.2026", "--start", "10:00", "--end", "09:00"})
		if err == nil || !strings.Contains(err.Error(), "before start") {
			t.Fatalf("expected overnight error, got %v", err)
		}
	})
}

// TestOffMutualExclusion verifies that marking a day OFF when it already has
// worked hours errors out.
func TestOffMutualExclusion(t *testing.T) {
	a, _, _ := newTestApp(t)
	if err := a.CmdIn([]string{"--at", "08:00"}); err != nil {
		t.Fatalf("CmdIn: %v", err)
	}
	err := a.CmdOff([]string{"24.06.2026"})
	if err == nil || !strings.Contains(err.Error(), "worked hours") {
		t.Fatalf("expected worked-hours error, got %v", err)
	}
}

// TestSeasonSnapshotAtCreation verifies that a day created while season=winter
// keeps expected=450 even after toggling the season to summer.
func TestSeasonSnapshotAtCreation(t *testing.T) {
	a, _, _ := newTestApp(t)
	// Default season is winter; create a day via set.
	if s, err := a.Store.Season(); err != nil || s != domain.Winter {
		t.Fatalf("precondition: season = %v, %v; want winter", s, err)
	}
	if err := a.CmdSet([]string{"24.06.2026", "--start", "08:00", "--end", "16:00"}); err != nil {
		t.Fatalf("CmdSet: %v", err)
	}
	// Toggle to summer.
	if err := a.Store.SetSeason(domain.Summer); err != nil {
		t.Fatal(err)
	}
	day, err := a.Store.GetDay(a.dateOnly(fixedNow))
	if err != nil {
		t.Fatal(err)
	}
	if day == nil {
		t.Fatal("expected a day")
	}
	if day.ExpectedMinutes != domain.WinterExpectedMinutes {
		t.Errorf("expected minutes = %d, want %d (winter snapshot preserved)",
			day.ExpectedMinutes, domain.WinterExpectedMinutes)
	}
}

// TestSetLongDayWarning verifies that a >16h day created via `set` warns on
// stderr but is still allowed (no error).
func TestSetLongDayWarning(t *testing.T) {
	a, _, errb := newTestApp(t)
	// 06:00–23:00 with 0 lunch = 17h worked, beyond LongDayMinutes (16h).
	if err := a.CmdSet([]string{"24.06.2026", "--start", "06:00", "--end", "23:00", "--lunch", "0m"}); err != nil {
		t.Fatalf("CmdSet: %v", err)
	}
	if !strings.Contains(errb.String(), "very long day") {
		t.Errorf("expected long-day warning on stderr, got %q", errb.String())
	}
	// The day must still have been written.
	day, err := a.Store.GetDay(a.dateOnly(fixedNow))
	if err != nil {
		t.Fatal(err)
	}
	if day == nil || day.Start == nil || day.End == nil {
		t.Fatal("expected the long day to be saved despite the warning")
	}
}

// TestLogToggle verifies that `punch log` toggles a week's logged state: first
// invocation logs it, second unlogs it, and the output names the resulting
// state each time. Uses `last` (the previous week) seeded with worked time to
// avoid the current-week and empty-week warnings.
func TestLogToggle(t *testing.T) {
	a, out, _ := newTestApp(t)

	// Seed worked time in the previous week (W25: 15–21 Jun 2026).
	if err := a.CmdSet([]string{"18.06.2026", "--start", "08:00", "--end", "16:00"}); err != nil {
		t.Fatalf("CmdSet: %v", err)
	}
	out.Reset()

	// First toggle: unlogged → logged.
	if err := a.CmdLog([]string{"last"}); err != nil {
		t.Fatalf("CmdLog (log): %v", err)
	}
	if got := out.String(); !strings.Contains(got, "logged") || strings.Contains(got, "unlogged") {
		t.Errorf("first toggle output = %q, want it to say 'logged' (not 'unlogged')", got)
	}
	if at, err := a.Store.WeekLoggedAt("2026-W25"); err != nil || at == nil {
		t.Fatalf("expected week logged after first toggle, at=%v err=%v", at, err)
	}

	out.Reset()

	// Second toggle: logged → unlogged.
	if err := a.CmdLog([]string{"last"}); err != nil {
		t.Fatalf("CmdLog (unlog): %v", err)
	}
	if got := out.String(); !strings.Contains(got, "unlogged") {
		t.Errorf("second toggle output = %q, want it to say 'unlogged'", got)
	}
	if at, err := a.Store.WeekLoggedAt("2026-W25"); err != nil || at != nil {
		t.Fatalf("expected week unlogged after second toggle, at=%v err=%v", at, err)
	}
}
