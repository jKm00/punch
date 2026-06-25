package app

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"punch/internal/calc"
	"punch/internal/domain"
	"punch/internal/timeparse"
)

// errSetupAborted is returned when the wizard hits EOF (or a read error) before
// every question has been answered. Callers must not run the original command
// when setup is aborted, and nothing is persisted.
var errSetupAborted = errors.New("setup aborted")

// CmdSetup runs the configuration wizard explicitly (`punch setup`). With
// --curr it instead prints the currently-effective configuration and exits
// without prompting or writing anything. The wizard always offers the hardcoded
// recommended defaults (domain.DefaultConfig / domain.DefaultSeason) for each
// prompt, regardless of any custom values currently stored. The current season
// is read separately only for the --curr listing.
func (a *App) CmdSetup(args []string) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	curr := fs.Bool("curr", false, "print the current configuration and exit (no prompts)")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}

	if *curr {
		season, err := a.Store.Season()
		if err != nil {
			return err
		}
		a.printConfig(a.Config, season)
		return nil
	}

	cfg, season, err := a.runWizard(domain.DefaultConfig(), domain.DefaultSeason)
	if err != nil {
		return err
	}
	if err := a.Store.SaveConfig(cfg, season); err != nil {
		return err
	}
	a.Config = cfg
	a.printf("%s Setup complete.\n", a.styler().Green("✓"))
	return nil
}

// printConfig writes the currently-effective configuration as a plain,
// read-only listing. No prompts, no writes.
func (a *App) printConfig(cfg domain.Config, season domain.Season) {
	s := a.styler()
	a.printf("%s\n", s.Bold("punch configuration"))
	a.printf("  %-28s %s\n", "Current season", s.Bold(string(season)))
	a.printf("  %-28s %s\n", "Winter expected/day", calc.FormatHM(cfg.WinterExpectedMinutes))
	a.printf("  %-28s %s\n", "Summer expected/day", calc.FormatHM(cfg.SummerExpectedMinutes))
	a.printf("  %-28s %s\n", "Winter logging start", calc.FormatClock(cfg.WinterLoggingStart.Hour, cfg.WinterLoggingStart.Minute))
	a.printf("  %-28s %s\n", "Summer logging start", calc.FormatClock(cfg.SummerLoggingStart.Hour, cfg.SummerLoggingStart.Minute))
	a.printf("  %-28s %s\n", "Default lunch", calc.FormatHM(cfg.DefaultLunchMinutes))
}

// runWizard prompts for every configurable value using defaults as the
// currently-effective values. It collects all answers in memory and returns the
// resolved Config and season without persisting anything. On EOF/read error
// mid-wizard it returns errSetupAborted (wrapped) so the caller can abort
// without writing partial state.
func (a *App) runWizard(defaults domain.Config, defaultSeason domain.Season) (domain.Config, domain.Season, error) {
	sc := bufio.NewScanner(a.In)
	// Allow long lines just in case; default is plenty but be safe.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	s := a.styler()
	a.printf("%s\n", s.Bold("punch setup"))
	a.printf("%s\n\n", s.Dim("Press Enter to accept the [default] shown for each value."))

	cfg := defaults

	winterExpected, err := a.promptDuration(sc, "Winter expected hours per day", defaults.WinterExpectedMinutes)
	if err != nil {
		return domain.Config{}, "", err
	}
	cfg.WinterExpectedMinutes = winterExpected

	summerExpected, err := a.promptDuration(sc, "Summer expected hours per day", defaults.SummerExpectedMinutes)
	if err != nil {
		return domain.Config{}, "", err
	}
	cfg.SummerExpectedMinutes = summerExpected

	winterStart, err := a.promptTimeOfDay(sc, "Winter logging start time", defaults.WinterLoggingStart)
	if err != nil {
		return domain.Config{}, "", err
	}
	cfg.WinterLoggingStart = winterStart

	summerStart, err := a.promptTimeOfDay(sc, "Summer logging start time", defaults.SummerLoggingStart)
	if err != nil {
		return domain.Config{}, "", err
	}
	cfg.SummerLoggingStart = summerStart

	lunch, err := a.promptDuration(sc, "Default lunch break in minutes", defaults.DefaultLunchMinutes)
	if err != nil {
		return domain.Config{}, "", err
	}
	cfg.DefaultLunchMinutes = lunch

	season, err := a.promptSeason(sc, "Which season are you in right now?", defaultSeason)
	if err != nil {
		return domain.Config{}, "", err
	}

	return cfg, season, nil
}

// readLine reads a single trimmed line from the scanner. The boolean is false
// when the input is exhausted (EOF) or a read error occurs.
func readLine(sc *bufio.Scanner) (string, bool) {
	if !sc.Scan() {
		return "", false
	}
	return strings.TrimSpace(sc.Text()), true
}

// promptDuration asks for a duration, showing the default formatted as e.g.
// "7h30m". It accepts any timeparse.ParseDuration form ("30m", "7h30m",
// "HH:MM"). Empty input keeps the default. Invalid input re-prompts the same
// field. EOF aborts.
func (a *App) promptDuration(sc *bufio.Scanner, label string, def int) (int, error) {
	for {
		a.printf("%s [%s]: ", label, calc.FormatHM(def))
		line, ok := readLine(sc)
		if !ok {
			return 0, fmt.Errorf("%w: unexpected end of input", errSetupAborted)
		}
		if line == "" {
			return def, nil
		}
		v, err := timeparse.ParseDuration(line)
		if err != nil {
			a.errorf("  %s %v\n", a.styler().Red("invalid:"), err)
			continue
		}
		return v, nil
	}
}

// promptTimeOfDay asks for an HH:MM time-of-day, showing the default. Empty
// input keeps the default. Invalid input re-prompts. EOF aborts.
func (a *App) promptTimeOfDay(sc *bufio.Scanner, label string, def domain.TimeOfDay) (domain.TimeOfDay, error) {
	for {
		a.printf("%s [%s]: ", label, calc.FormatClock(def.Hour, def.Minute))
		line, ok := readLine(sc)
		if !ok {
			return domain.TimeOfDay{}, fmt.Errorf("%w: unexpected end of input", errSetupAborted)
		}
		if line == "" {
			return def, nil
		}
		h, m, err := timeparse.ParseTime(line)
		if err != nil {
			a.errorf("  %s %v\n", a.styler().Red("invalid:"), err)
			continue
		}
		return domain.TimeOfDay{Hour: h, Minute: m}, nil
	}
}

// promptSeason asks for the current season, showing the default. Empty input
// keeps the default. Unknown seasons re-prompt (no silent defaulting). EOF
// aborts.
func (a *App) promptSeason(sc *bufio.Scanner, label string, def domain.Season) (domain.Season, error) {
	for {
		a.printf("%s [%s] (summer/winter): ", label, string(def))
		line, ok := readLine(sc)
		if !ok {
			return "", fmt.Errorf("%w: unexpected end of input", errSetupAborted)
		}
		if line == "" {
			return def, nil
		}
		switch domain.Season(strings.ToLower(line)) {
		case domain.Summer:
			return domain.Summer, nil
		case domain.Winter:
			return domain.Winter, nil
		default:
			a.errorf("  %s unknown season %q: use `summer` or `winter`\n", a.styler().Red("invalid:"), line)
		}
	}
}

// IsInteractive reports whether in refers to an interactive terminal (a
// character device). It mirrors the os.ModeCharDevice check used for output in
// internal/ui. A non-*os.File reader (e.g. a strings.Reader in tests) is treated
// as non-interactive, so piped/scripted input never triggers the wizard.
func IsInteractive(in io.Reader) bool {
	f, ok := in.(*os.File)
	if !ok || f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
