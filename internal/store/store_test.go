package store

import (
	"path/filepath"
	"testing"
	"time"

	"punch/internal/domain"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "wh.db"), time.UTC)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestNewConfigFieldsRoundTrip(t *testing.T) {
	st := openTemp(t)

	// Absent keys must resolve to the domain defaults.
	cfg, err := st.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	def := domain.DefaultConfig()
	if cfg.SeasonsEnabled != def.SeasonsEnabled {
		t.Errorf("default SeasonsEnabled = %v, want %v", cfg.SeasonsEnabled, def.SeasonsEnabled)
	}
	if cfg.SummerStart != def.SummerStart {
		t.Errorf("default SummerStart = %+v, want %+v", cfg.SummerStart, def.SummerStart)
	}
	if cfg.SummerEnd != def.SummerEnd {
		t.Errorf("default SummerEnd = %+v, want %+v", cfg.SummerEnd, def.SummerEnd)
	}

	// Save custom values and read them back unchanged.
	want := domain.DefaultConfig()
	want.SeasonsEnabled = false
	want.SummerStart = domain.MonthDay{Month: 6, Day: 1}
	want.SummerEnd = domain.MonthDay{Month: 9, Day: 15}
	if err := st.SaveConfig(want); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	got, err := st.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.SeasonsEnabled != want.SeasonsEnabled {
		t.Errorf("SeasonsEnabled = %v, want %v", got.SeasonsEnabled, want.SeasonsEnabled)
	}
	if got.SummerStart != want.SummerStart {
		t.Errorf("SummerStart = %+v, want %+v", got.SummerStart, want.SummerStart)
	}
	if got.SummerEnd != want.SummerEnd {
		t.Errorf("SummerEnd = %+v, want %+v", got.SummerEnd, want.SummerEnd)
	}
}

// TestStaleSeasonRowDeletedOnOpen verifies the one-time cleanup migration:
// a stale `season` settings row left over from the old manual toggle is removed
// on the next Open.
func TestStaleSeasonRowDeletedOnOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wh.db")

	st, err := Open(path, time.UTC)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := st.setSetting("season", "summer"); err != nil {
		t.Fatal(err)
	}
	st.Close()

	st2, err := Open(path, time.UTC)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	t.Cleanup(func() { st2.Close() })

	if _, ok, _ := st2.getSetting("season"); ok {
		t.Error("stale season row should have been deleted on Open")
	}
}

func TestDayRoundTrip(t *testing.T) {
	st := openTemp(t)
	date := time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)
	start := time.Date(2026, 6, 24, 8, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 24, 16, 0, 0, 0, time.UTC)
	lunch := 45
	d := &Day{
		Date:            date,
		Start:           &start,
		End:             &end,
		LunchMinutes:    &lunch,
		ExpectedMinutes: domain.WinterExpectedMinutes,
	}
	if err := st.UpsertDay(d); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetDay(date)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("GetDay returned nil")
	}
	if got.Start == nil || !got.Start.Equal(start) {
		t.Errorf("start = %v, want %v", got.Start, start)
	}
	if got.End == nil || !got.End.Equal(end) {
		t.Errorf("end = %v, want %v", got.End, end)
	}
	if got.EffectiveLunch(domain.DefaultLunchMinutes) != 45 {
		t.Errorf("lunch = %d, want 45", got.EffectiveLunch(domain.DefaultLunchMinutes))
	}

	// Default lunch when nil.
	d2date := time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)
	if err := st.UpsertDay(&Day{Date: d2date, ExpectedMinutes: 450}); err != nil {
		t.Fatal(err)
	}
	d2, _ := st.GetDay(d2date)
	if d2.EffectiveLunch(domain.DefaultLunchMinutes) != domain.DefaultLunchMinutes {
		t.Errorf("default lunch = %d, want %d", d2.EffectiveLunch(domain.DefaultLunchMinutes), domain.DefaultLunchMinutes)
	}

	// Delete.
	deleted, err := st.DeleteDay(date)
	if err != nil || !deleted {
		t.Fatalf("DeleteDay = %v, %v", deleted, err)
	}
	got, _ = st.GetDay(date)
	if got != nil {
		t.Errorf("day still present after delete")
	}
}

func TestWeekStatus(t *testing.T) {
	st := openTemp(t)
	at, err := st.WeekLoggedAt("2026-W26")
	if err != nil {
		t.Fatal(err)
	}
	if at != nil {
		t.Errorf("expected unlogged week")
	}
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	if err := st.SetWeekLogged("2026-W26", now); err != nil {
		t.Fatal(err)
	}
	at, _ = st.WeekLoggedAt("2026-W26")
	if at == nil || !at.Equal(now) {
		t.Errorf("logged at = %v, want %v", at, now)
	}

	// Clearing removes the logged state.
	if err := st.ClearWeekLogged("2026-W26"); err != nil {
		t.Fatal(err)
	}
	at, _ = st.WeekLoggedAt("2026-W26")
	if at != nil {
		t.Errorf("expected unlogged week after clear, got %v", at)
	}
	// Clearing an already-unlogged week is a no-op.
	if err := st.ClearWeekLogged("2026-W26"); err != nil {
		t.Fatalf("clear no-op: %v", err)
	}
}

func TestEarliestWorkedDate(t *testing.T) {
	st := openTemp(t)
	got, err := st.EarliestWorkedDate()
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil earliest, got %v", got)
	}
	// Off day (no start) should not count.
	st.UpsertDay(&Day{Date: time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC), ExpectedMinutes: 0, IsOff: true})
	start := time.Date(2026, 6, 22, 8, 0, 0, 0, time.UTC)
	st.UpsertDay(&Day{Date: time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC), Start: &start, ExpectedMinutes: 450})
	got, _ = st.EarliestWorkedDate()
	if got == nil || got.Format(DateLayout) != "2026-06-22" {
		t.Errorf("earliest = %v, want 2026-06-22", got)
	}
}

// TestLegacyLoggingStartKeysMigrated verifies the one-time key rename: values
// stored under the legacy winter_logging_start / summer_logging_start keys are
// migrated to winter_end_of_day / summer_end_of_day on the next Open, so a
// user's custom values are preserved (not silently reset to defaults).
func TestLegacyLoggingStartKeysMigrated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wh.db")

	// First open: seed values under the legacy keys, simulating an older DB.
	st, err := Open(path, time.UTC)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := st.setSetting("winter_logging_start", "17:15"); err != nil {
		t.Fatal(err)
	}
	if err := st.setSetting("summer_logging_start", "14:45"); err != nil {
		t.Fatal(err)
	}
	st.Close()

	// Re-open: migrate() should rename the keys in place.
	st2, err := Open(path, time.UTC)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	t.Cleanup(func() { st2.Close() })

	// The values must now resolve under the new keys.
	cfg, err := st2.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WinterEndOfDay != (domain.TimeOfDay{Hour: 17, Minute: 15}) {
		t.Errorf("winter end of day = %+v, want 17:15 (migrated from legacy key)", cfg.WinterEndOfDay)
	}
	if cfg.SummerEndOfDay != (domain.TimeOfDay{Hour: 14, Minute: 45}) {
		t.Errorf("summer end of day = %+v, want 14:45 (migrated from legacy key)", cfg.SummerEndOfDay)
	}

	// The legacy keys must no longer exist.
	if _, ok, _ := st2.getSetting("winter_logging_start"); ok {
		t.Error("legacy winter_logging_start key should have been renamed away")
	}
	if _, ok, _ := st2.getSetting("summer_logging_start"); ok {
		t.Error("legacy summer_logging_start key should have been renamed away")
	}
}
