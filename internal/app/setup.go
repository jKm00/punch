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
	"punch/internal/ui"
)

// errSetupAborted is returned when the wizard hits EOF (or a read error) before
// every question has been answered. Callers must not run the original command
// when setup is aborted, and nothing is persisted.
var errSetupAborted = errors.New("setup aborted")

// CmdSetup runs the configuration wizard explicitly (`punch setup`). With
// --curr it instead prints the currently-effective configuration and exits
// without prompting or writing anything. The wizard always offers the hardcoded
// recommended defaults (domain.DefaultConfig) for each prompt, regardless of
// any custom values currently stored.
func (a *App) CmdSetup(args []string) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	curr := fs.Bool("curr", false, "print the current configuration and exit (no prompts)")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}

	if *curr {
		a.printConfig(a.Config)
		return nil
	}

	cfg, err := a.runWizard(domain.DefaultConfig())
	if err != nil {
		return err
	}
	if err := a.Store.SaveConfig(cfg); err != nil {
		return err
	}
	a.Config = cfg
	a.printf("%s Setup complete.\n", a.styler().Green("✓"))
	return nil
}

// printConfig writes the currently-effective configuration as a read-only
// titled box, matching the style of `punch week`/`punch status`. No prompts, no
// writes. The layout adapts to whether seasons are enabled: when disabled the
// season-specific rows and the current-season line are hidden in favour of
// generic labels.
func (a *App) printConfig(cfg domain.Config) {
	s := a.styler()

	// label renders a dim, fixed-width label so values align in a column.
	const labelWidth = 14
	row := func(label, value string) string {
		return s.Dim(ui.PadRight(label, labelWidth)) + value
	}

	var lines []string
	if !cfg.SeasonsEnabled {
		lines = append(lines,
			row("Seasons", s.Yellow("disabled")),
			row("Expected", calc.FormatHM(cfg.WinterExpectedMinutes)),
			row("End of day", calc.FormatClock(cfg.WinterEndOfDay.Hour, cfg.WinterEndOfDay.Minute)),
			row("Lunch", calc.FormatHM(cfg.DefaultLunchMinutes)),
		)
		a.printf("%s", s.Box("Configuration", lines))
		return
	}

	lines = append(lines,
		row("Seasons", s.Green("enabled")),
		row("Summer", fmt.Sprintf("%s – %s", formatMonthDayEU(cfg.SummerStart), formatMonthDayEU(cfg.SummerEnd))+
			s.Dim("  (current: ")+string(a.Config.SeasonFor(a.dateOnly(a.now())))+s.Dim(")")),
		row("Expected", calc.FormatHM(cfg.WinterExpectedMinutes)+s.Dim(" winter")+"   "+
			calc.FormatHM(cfg.SummerExpectedMinutes)+s.Dim(" summer")),
		row("End of day", calc.FormatClock(cfg.WinterEndOfDay.Hour, cfg.WinterEndOfDay.Minute)+s.Dim(" winter")+"   "+
			calc.FormatClock(cfg.SummerEndOfDay.Hour, cfg.SummerEndOfDay.Minute)+s.Dim(" summer")),
		row("Lunch", calc.FormatHM(cfg.DefaultLunchMinutes)),
	)
	a.printf("%s", s.Box("Configuration", lines))
}

// formatMonthDayEU renders a MonthDay in European DD.MM form for display.
func formatMonthDayEU(md domain.MonthDay) string {
	return fmt.Sprintf("%02d.%02d", md.Day, md.Month)
}

// runWizard prompts for every configurable value using defaults as the
// currently-effective values. It collects all answers in memory and returns the
// resolved Config without persisting anything. The first prompt decides whether
// separate summer/winter schedules apply; when they do not, the season-specific
// prompts are skipped and the single schedule is stored in the winter slot. On
// EOF/read error mid-wizard it returns errSetupAborted (wrapped) so the caller
// can abort without writing partial state.
func (a *App) runWizard(defaults domain.Config) (domain.Config, error) {
	sc := bufio.NewScanner(a.In)
	// Allow long lines just in case; default is plenty but be safe.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	s := a.styler()
	a.printf("%s\n", s.Bold("punch setup"))
	a.printf("%s\n\n", s.Dim("Press Enter to accept the [default] shown for each value."))

	cfg := defaults

	enabled, err := a.promptBool(sc, "Does your company have separate summer/winter schedules?", defaults.SeasonsEnabled)
	if err != nil {
		return domain.Config{}, err
	}
	cfg.SeasonsEnabled = enabled

	if !enabled {
		expected, err := a.promptDuration(sc, "Expected hours per day", defaults.WinterExpectedMinutes)
		if err != nil {
			return domain.Config{}, err
		}
		cfg.WinterExpectedMinutes = expected

		endOfDay, err := a.promptTimeOfDay(sc, "Typical end of day", defaults.WinterEndOfDay)
		if err != nil {
			return domain.Config{}, err
		}
		cfg.WinterEndOfDay = endOfDay

		lunch, err := a.promptDuration(sc, "Default lunch break in minutes", defaults.DefaultLunchMinutes)
		if err != nil {
			return domain.Config{}, err
		}
		cfg.DefaultLunchMinutes = lunch

		// Summer fields and the interval are left at their defaults.
		return cfg, nil
	}

	winterExpected, err := a.promptDuration(sc, "Winter expected hours per day", defaults.WinterExpectedMinutes)
	if err != nil {
		return domain.Config{}, err
	}
	cfg.WinterExpectedMinutes = winterExpected

	summerExpected, err := a.promptDuration(sc, "Summer expected hours per day", defaults.SummerExpectedMinutes)
	if err != nil {
		return domain.Config{}, err
	}
	cfg.SummerExpectedMinutes = summerExpected

	winterEnd, err := a.promptTimeOfDay(sc, "Typical end of day (winter)", defaults.WinterEndOfDay)
	if err != nil {
		return domain.Config{}, err
	}
	cfg.WinterEndOfDay = winterEnd

	summerEnd, err := a.promptTimeOfDay(sc, "Typical end of day (summer)", defaults.SummerEndOfDay)
	if err != nil {
		return domain.Config{}, err
	}
	cfg.SummerEndOfDay = summerEnd

	summerStart, err := a.promptMonthDay(sc, "Summer period start", defaults.SummerStart)
	if err != nil {
		return domain.Config{}, err
	}
	cfg.SummerStart = summerStart

	summerEndDate, err := a.promptMonthDay(sc, "Summer period end", defaults.SummerEnd)
	if err != nil {
		return domain.Config{}, err
	}
	cfg.SummerEnd = summerEndDate

	lunch, err := a.promptDuration(sc, "Default lunch break in minutes", defaults.DefaultLunchMinutes)
	if err != nil {
		return domain.Config{}, err
	}
	cfg.DefaultLunchMinutes = lunch

	return cfg, nil
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

// promptBool asks a yes/no question, showing the default as (Y/n) or (y/N).
// Empty input keeps the default. Unrecognised input re-prompts (no silent
// defaulting). EOF aborts.
func (a *App) promptBool(sc *bufio.Scanner, label string, def bool) (bool, error) {
	hint := "(Y/n)"
	if !def {
		hint = "(y/N)"
	}
	for {
		a.printf("%s %s: ", label, hint)
		line, ok := readLine(sc)
		if !ok {
			return false, fmt.Errorf("%w: unexpected end of input", errSetupAborted)
		}
		if line == "" {
			return def, nil
		}
		switch strings.ToLower(line) {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			a.errorf("  %s answer yes or no\n", a.styler().Red("invalid:"))
		}
	}
}

// promptMonthDay asks for a recurring month/day, showing the default in
// European DD.MM form. It accepts DD.MM or DD-MM. Empty input keeps the
// default. Invalid input re-prompts. EOF aborts.
func (a *App) promptMonthDay(sc *bufio.Scanner, label string, def domain.MonthDay) (domain.MonthDay, error) {
	for {
		a.printf("%s [%s]: ", label, formatMonthDayEU(def))
		line, ok := readLine(sc)
		if !ok {
			return domain.MonthDay{}, fmt.Errorf("%w: unexpected end of input", errSetupAborted)
		}
		if line == "" {
			return def, nil
		}
		m, d, err := timeparse.ParseMonthDay(line)
		if err != nil {
			a.errorf("  %s %v\n", a.styler().Red("invalid:"), err)
			continue
		}
		return domain.MonthDay{Month: m, Day: d}, nil
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
