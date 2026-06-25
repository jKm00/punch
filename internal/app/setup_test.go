package app

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// TestSetupHappyPath feeds explicit answers and asserts they are persisted and
// setup_completed is set.
func TestSetupHappyPath(t *testing.T) {
	stdin := strings.Join([]string{
		"8h",     // winter expected
		"7h15m",  // summer expected
		"16:30",  // winter logging start
		"15:00",  // summer logging start
		"45m",    // default lunch
		"summer", // season
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
	if cfg.WinterExpectedMinutes != 8*60 {
		t.Errorf("winter expected = %d, want %d", cfg.WinterExpectedMinutes, 8*60)
	}
	if cfg.SummerExpectedMinutes != 7*60+15 {
		t.Errorf("summer expected = %d, want %d", cfg.SummerExpectedMinutes, 7*60+15)
	}
	if cfg.WinterLoggingStart != (domain.TimeOfDay{Hour: 16, Minute: 30}) {
		t.Errorf("winter logging start = %+v, want 16:30", cfg.WinterLoggingStart)
	}
	if cfg.SummerLoggingStart != (domain.TimeOfDay{Hour: 15, Minute: 0}) {
		t.Errorf("summer logging start = %+v, want 15:00", cfg.SummerLoggingStart)
	}
	if cfg.DefaultLunchMinutes != 45 {
		t.Errorf("default lunch = %d, want 45", cfg.DefaultLunchMinutes)
	}

	season, err := st.Season()
	if err != nil {
		t.Fatal(err)
	}
	if season != domain.Summer {
		t.Errorf("season = %s, want summer", season)
	}

	// The in-memory Config should also be refreshed.
	if a.Config.DefaultLunchMinutes != 45 {
		t.Errorf("a.Config.DefaultLunchMinutes = %d, want 45", a.Config.DefaultLunchMinutes)
	}
}

// TestSetupEnterForDefault feeds empty lines and asserts the domain defaults
// are persisted.
func TestSetupEnterForDefault(t *testing.T) {
	st := newSetupStore(t)
	a, _, _ := setupApp(t, st, "\n\n\n\n\n\n") // six empty answers
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
	season, err := st.Season()
	if err != nil {
		t.Fatal(err)
	}
	if season != domain.DefaultSeason {
		t.Errorf("season = %s, want %s", season, domain.DefaultSeason)
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
// field and asserts the wizard re-prompts and stores the good value.
func TestSetupInvalidThenValid(t *testing.T) {
	stdin := strings.Join([]string{
		"not-a-duration", // invalid winter expected -> re-prompt
		"8h",             // valid winter expected
		"",               // summer default
		"",               // winter start default
		"",               // summer start default
		"",               // lunch default
		"",               // season default
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
	a, _, _ := setupApp(t, st, "8h\n") // only the first answer, then EOF
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

// TestSetupRerunShowsStoredDefaults pre-seeds config, then re-runs the wizard
// with all-empty input and asserts the shown defaults reflect the stored values
// and that accepting defaults keeps them.
func TestSetupRerunShowsStoredDefaults(t *testing.T) {
	st := newSetupStore(t)

	seed := domain.Config{
		WinterExpectedMinutes: 8 * 60,
		SummerExpectedMinutes: 6*60 + 30,
		WinterLoggingStart:    domain.TimeOfDay{Hour: 17, Minute: 15},
		SummerLoggingStart:    domain.TimeOfDay{Hour: 14, Minute: 45},
		DefaultLunchMinutes:   60,
	}
	if err := st.SaveConfig(seed, domain.Summer); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	// Re-run with all-empty answers; defaults must reflect the stored values.
	a, out, _ := setupApp(t, st, "\n\n\n\n\n\n")
	if err := a.CmdSetup(nil); err != nil {
		t.Fatalf("CmdSetup re-run: %v", err)
	}
	prompts := out.String()
	for _, want := range []string{"[8h]", "[6h30m]", "[17:15]", "[14:45]", "[1h]", "[summer]"} {
		if !strings.Contains(prompts, want) {
			t.Errorf("re-run prompts missing default %q\nprompts:\n%s", want, prompts)
		}
	}
	// Accepting defaults must keep the stored values.
	cfg, err := st.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg != seed {
		t.Errorf("config after re-run = %+v, want unchanged seed %+v", cfg, seed)
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
		WinterLoggingStart:    domain.TimeOfDay{Hour: 17, Minute: 15},
		SummerLoggingStart:    domain.TimeOfDay{Hour: 14, Minute: 45},
		DefaultLunchMinutes:   60,
	}
	if err := st.SaveConfig(seed, domain.Summer); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	// Empty stdin: if --curr prompted at all, the wizard would hit EOF and error.
	a, out, _ := setupApp(t, st, "")
	if err := a.CmdSetup([]string{"--curr"}); err != nil {
		t.Fatalf("CmdSetup --curr: %v", err)
	}

	printed := out.String()
	for _, want := range []string{"8h", "6h30m", "17:15", "14:45", "1h", "summer"} {
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
