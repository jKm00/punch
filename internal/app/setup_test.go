package app

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"punch/internal/calc"
	"punch/internal/domain"
	"punch/internal/store"
)

// newSetupStore opens a fresh temp store for setup tests.
func newSetupStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "wh.db"), time.UTC)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// setupApp builds a test App over the given store with the given stdin content.
// Config is loaded from the store so the wizard's defaults reflect resolved
// values.
func setupApp(t *testing.T, st *store.Store, stdin string) (*App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	cfg, err := st.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	var out, errb bytes.Buffer
	a := &App{
		Store:  st,
		Now:    func() time.Time { return fixedNow },
		Loc:    time.UTC,
		In:     strings.NewReader(stdin),
		Out:    &out,
		Err:    &errb,
		Config: cfg,
	}
	return a, &out, &errb
}

// TestSetupHappyPath feeds explicit answers (seasons enabled) and asserts they
// are persisted and setup_completed is set.
func TestSetupHappyPath(t *testing.T) {
	stdin := strings.Join([]string{
		"y",     // seasons enabled
		"8h",    // winter expected
		"7h15m", // summer expected
		"16:30", // winter end of day
		"15:00", // summer end of day
		"01.06", // summer period start
		"15.09", // summer period end
		"45m",   // default lunch
	}, "\n") + "\n"

	st := newSetupStore(t)
	a, _, _ := setupApp(t, st, stdin)
	if err := a.CmdSetup(nil); err != nil {
		t.Fatalf("CmdSetup: %v", err)
	}

	done, err := st.SetupCompleted()
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("setup_completed should be true")
	}

	cfg, err := st.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SeasonsEnabled {
		t.Errorf("SeasonsEnabled = false, want true")
	}
	if cfg.WinterExpectedMinutes != 8*60 {
		t.Errorf("winter expected = %d, want %d", cfg.WinterExpectedMinutes, 8*60)
	}
	if cfg.SummerExpectedMinutes != 7*60+15 {
		t.Errorf("summer expected = %d, want %d", cfg.SummerExpectedMinutes, 7*60+15)
	}
	if cfg.WinterEndOfDay != (domain.TimeOfDay{Hour: 16, Minute: 30}) {
		t.Errorf("winter end of day = %+v, want 16:30", cfg.WinterEndOfDay)
	}
	if cfg.SummerEndOfDay != (domain.TimeOfDay{Hour: 15, Minute: 0}) {
		t.Errorf("summer end of day = %+v, want 15:00", cfg.SummerEndOfDay)
	}
	if cfg.SummerStart != (domain.MonthDay{Month: 6, Day: 1}) {
		t.Errorf("summer start = %+v, want 01.06", cfg.SummerStart)
	}
	if cfg.SummerEnd != (domain.MonthDay{Month: 9, Day: 15}) {
		t.Errorf("summer end = %+v, want 15.09", cfg.SummerEnd)
	}
	if cfg.DefaultLunchMinutes != 45 {
		t.Errorf("default lunch = %d, want 45", cfg.DefaultLunchMinutes)
	}

	// The in-memory Config should also be refreshed.
	if a.Config.DefaultLunchMinutes != 45 {
		t.Errorf("a.Config.DefaultLunchMinutes = %d, want 45", a.Config.DefaultLunchMinutes)
	}
}

// TestSetupDisabledPath feeds the seasons-disabled flow and asserts the summer
// prompts are skipped, the single schedule is stored in the winter slot, and
// SeasonsEnabled is persisted as false.
func TestSetupDisabledPath(t *testing.T) {
	stdin := strings.Join([]string{
		"n",     // seasons disabled
		"8h",    // expected hours per day
		"17:00", // typical end of day
		"45m",   // default lunch
	}, "\n") + "\n"

	st := newSetupStore(t)
	a, out, _ := setupApp(t, st, stdin)
	if err := a.CmdSetup(nil); err != nil {
		t.Fatalf("CmdSetup: %v", err)
	}

	prompts := out.String()
	// Generic labels are shown; season-specific prompts are not.
	if !strings.Contains(prompts, "Expected hours per day") {
		t.Errorf("expected generic 'Expected hours per day' prompt, got:\n%s", prompts)
	}
	if !strings.Contains(prompts, "Typical end of day") {
		t.Errorf("expected generic 'Typical end of day' prompt, got:\n%s", prompts)
	}
	if strings.Contains(prompts, "Summer expected") || strings.Contains(prompts, "Summer period") {
		t.Errorf("summer prompts should be skipped when seasons disabled, got:\n%s", prompts)
	}

	cfg, err := st.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SeasonsEnabled {
		t.Errorf("SeasonsEnabled = true, want false")
	}
	if cfg.WinterExpectedMinutes != 8*60 {
		t.Errorf("expected hours = %d, want %d (stored in winter slot)", cfg.WinterExpectedMinutes, 8*60)
	}
	if cfg.WinterEndOfDay != (domain.TimeOfDay{Hour: 17, Minute: 0}) {
		t.Errorf("end of day = %+v, want 17:00 (stored in winter slot)", cfg.WinterEndOfDay)
	}
	if cfg.DefaultLunchMinutes != 45 {
		t.Errorf("default lunch = %d, want 45", cfg.DefaultLunchMinutes)
	}
	// Summer fields and interval untouched (defaults).
	def := domain.DefaultConfig()
	if cfg.SummerStart != def.SummerStart || cfg.SummerEnd != def.SummerEnd {
		t.Errorf("summer interval = %+v–%+v, want defaults %+v–%+v",
			cfg.SummerStart, cfg.SummerEnd, def.SummerStart, def.SummerEnd)
	}
}

// TestSetupEnterForDefault feeds empty lines and asserts the domain defaults
// are persisted (the enabled path: bool + 7 values = 8 empty answers).
func TestSetupEnterForDefault(t *testing.T) {
	st := newSetupStore(t)
	a, _, _ := setupApp(t, st, "\n\n\n\n\n\n\n\n") // eight empty answers
	if err := a.CmdSetup(nil); err != nil {
		t.Fatalf("CmdSetup: %v", err)
	}

	cfg, err := st.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	def := domain.DefaultConfig()
	if cfg != def {
		t.Errorf("config = %+v, want defaults %+v", cfg, def)
	}
	done, err := st.SetupCompleted()
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("setup_completed should be true")
	}
}

// TestSetupInvalidThenValid feeds a bad line then a good one for the first
// duration field and asserts the wizard re-prompts and stores the good value.
func TestSetupInvalidThenValid(t *testing.T) {
	stdin := strings.Join([]string{
		"y",              // seasons enabled
		"not-a-duration", // invalid winter expected -> re-prompt
		"8h",             // valid winter expected
		"",               // summer expected default
		"",               // winter end default
		"",               // summer end default
		"",               // summer start default
		"",               // summer end-date default
		"",               // lunch default
	}, "\n") + "\n"

	st := newSetupStore(t)
	a, _, errb := setupApp(t, st, stdin)
	if err := a.CmdSetup(nil); err != nil {
		t.Fatalf("CmdSetup: %v", err)
	}
	if !strings.Contains(errb.String(), "invalid:") {
		t.Errorf("expected an 'invalid:' re-prompt message on stderr, got %q", errb.String())
	}
	cfg, err := st.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WinterExpectedMinutes != 8*60 {
		t.Errorf("winter expected = %d, want %d (the valid retry)", cfg.WinterExpectedMinutes, 8*60)
	}
}

// TestSetupEOFAborts feeds truncated input and asserts an error is returned and
// nothing was persisted.
func TestSetupEOFAborts(t *testing.T) {
	st := newSetupStore(t)
	a, _, _ := setupApp(t, st, "y\n8h\n") // bool + first answer, then EOF
	err := a.CmdSetup(nil)
	if err == nil {
		t.Fatal("expected an error on truncated input")
	}
	if !errors.Is(err, errSetupAborted) {
		t.Errorf("error = %v, want it to wrap errSetupAborted", err)
	}

	// Nothing should have been written: setup not completed, and config still
	// resolves to defaults.
	done, derr := st.SetupCompleted()
	if derr != nil {
		t.Fatal(derr)
	}
	if done {
		t.Fatal("setup_completed must NOT be set after an aborted wizard")
	}
	cfg, cerr := st.LoadConfig()
	if cerr != nil {
		t.Fatal(cerr)
	}
	if cfg != domain.DefaultConfig() {
		t.Errorf("config = %+v, want untouched defaults", cfg)
	}
}

// TestSetupRerunUsesHardcodedDefaults pre-seeds custom config, then re-runs the
// wizard with all-empty input and asserts the shown defaults are the hardcoded
// recommended values (NOT the stored custom values), and that accepting them
// overwrites the stored config with the hardcoded defaults.
func TestSetupRerunUsesHardcodedDefaults(t *testing.T) {
	st := newSetupStore(t)

	seed := domain.Config{
		WinterExpectedMinutes: 8 * 60,
		SummerExpectedMinutes: 6*60 + 30,
		WinterEndOfDay:        domain.TimeOfDay{Hour: 17, Minute: 15},
		SummerEndOfDay:        domain.TimeOfDay{Hour: 14, Minute: 45},
		DefaultLunchMinutes:   60,
		SeasonsEnabled:        true,
		SummerStart:           domain.MonthDay{Month: 6, Day: 1},
		SummerEnd:             domain.MonthDay{Month: 9, Day: 15},
	}
	if err := st.SaveConfig(seed); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	// Re-run with all-empty answers (enabled path: bool + 7 values); defaults
	// must be the hardcoded recommended values, regardless of the stored config.
	a, out, _ := setupApp(t, st, "\n\n\n\n\n\n\n\n")
	if err := a.CmdSetup(nil); err != nil {
		t.Fatalf("CmdSetup re-run: %v", err)
	}
	def := domain.DefaultConfig()
	wantPrompts := []string{
		"[" + calc.FormatHM(def.WinterExpectedMinutes) + "]",
		"[" + calc.FormatHM(def.SummerExpectedMinutes) + "]",
		"[" + calc.FormatClock(def.WinterEndOfDay.Hour, def.WinterEndOfDay.Minute) + "]",
		"[" + calc.FormatClock(def.SummerEndOfDay.Hour, def.SummerEndOfDay.Minute) + "]",
		"[" + formatMonthDayEU(def.SummerStart) + "]",
		"[" + formatMonthDayEU(def.SummerEnd) + "]",
		"[" + calc.FormatHM(def.DefaultLunchMinutes) + "]",
	}
	prompts := out.String()
	for _, want := range wantPrompts {
		if !strings.Contains(prompts, want) {
			t.Errorf("re-run prompts missing hardcoded default %q\nprompts:\n%s", want, prompts)
		}
	}
	// The custom stored values must NOT appear as defaults.
	for _, notWant := range []string{"[8h]", "[6h30m]", "[17:15]", "[14:45]", "[1h]", "[01.06]", "[15.09]"} {
		if strings.Contains(prompts, notWant) {
			t.Errorf("re-run prompts should not show stored value %q\nprompts:\n%s", notWant, prompts)
		}
	}
	// Accepting the hardcoded defaults must overwrite the stored config.
	cfg, err := st.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg != def {
		t.Errorf("config after re-run = %+v, want hardcoded defaults %+v", cfg, def)
	}
}

// TestIsInteractive verifies the TTY helper treats non-file readers as
// non-interactive (so piped/scripted stdin never triggers the wizard).
func TestIsInteractive(t *testing.T) {
	if IsInteractive(strings.NewReader("hello")) {
		t.Error("strings.Reader should not be interactive")
	}
	if IsInteractive(nil) {
		t.Error("nil reader should not be interactive")
	}
}

// TestSetupCurrPrintsConfigWithoutWriting verifies `punch setup --curr` prints
// the currently-effective values and does not prompt or persist anything.
func TestSetupCurrPrintsConfig(t *testing.T) {
	st := newSetupStore(t)

	seed := domain.Config{
		WinterExpectedMinutes: 8 * 60,
		SummerExpectedMinutes: 6*60 + 30,
		WinterEndOfDay:        domain.TimeOfDay{Hour: 17, Minute: 15},
		SummerEndOfDay:        domain.TimeOfDay{Hour: 14, Minute: 45},
		DefaultLunchMinutes:   60,
		SeasonsEnabled:        true,
		SummerStart:           domain.MonthDay{Month: 5, Day: 15},
		SummerEnd:             domain.MonthDay{Month: 8, Day: 31},
	}
	if err := st.SaveConfig(seed); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	// Empty stdin: if --curr prompted at all, the wizard would hit EOF and error.
	a, out, _ := setupApp(t, st, "")
	if err := a.CmdSetup([]string{"--curr"}); err != nil {
		t.Fatalf("CmdSetup --curr: %v", err)
	}

	printed := out.String()
	// fixedNow (24.06) falls in the summer period, so the derived current
	// season is summer.
	for _, want := range []string{"8h", "6h30m", "17:15", "14:45", "1h", "summer", "15.05", "31.08"} {
		if !strings.Contains(printed, want) {
			t.Errorf("--curr output missing %q\noutput:\n%s", want, printed)
		}
	}

	// Nothing should have changed in the store.
	cfg, err := st.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg != seed {
		t.Errorf("config after --curr = %+v, want unchanged seed %+v", cfg, seed)
	}
}
