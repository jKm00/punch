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

func TestSeasonRoundTrip(t *testing.T) {
	st := openTemp(t)
	s, err := st.Season()
	if err != nil {
		t.Fatal(err)
	}
	if s != domain.DefaultSeason {
		t.Errorf("default season = %s, want %s", s, domain.DefaultSeason)
	}
	if err := st.SetSeason(domain.Summer); err != nil {
		t.Fatal(err)
	}
	s, _ = st.Season()
	if s != domain.Summer {
		t.Errorf("season = %s, want summer", s)
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
	if got.EffectiveLunch() != 45 {
		t.Errorf("lunch = %d, want 45", got.EffectiveLunch())
	}

	// Default lunch when nil.
	d2date := time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)
	if err := st.UpsertDay(&Day{Date: d2date, ExpectedMinutes: 450}); err != nil {
		t.Fatal(err)
	}
	d2, _ := st.GetDay(d2date)
	if d2.EffectiveLunch() != domain.DefaultLunchMinutes {
		t.Errorf("default lunch = %d, want %d", d2.EffectiveLunch(), domain.DefaultLunchMinutes)
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
