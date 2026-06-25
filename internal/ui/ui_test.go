package ui

import (
	"strings"
	"testing"
)

func TestStylerDisabledIsPlain(t *testing.T) {
	s := New(false)
	cases := []string{s.Bold("x"), s.Dim("x"), s.Red("x"), s.Green("x"), s.Yellow("x"), s.Cyan("x")}
	for _, got := range cases {
		if got != "x" {
			t.Errorf("disabled styler should return input unchanged, got %q", got)
		}
		if strings.Contains(got, "\x1b") {
			t.Errorf("disabled styler emitted an escape: %q", got)
		}
	}
}

func TestStylerEnabledWraps(t *testing.T) {
	s := New(true)
	got := s.Green("x")
	if !strings.HasPrefix(got, "\x1b[32m") || !strings.HasSuffix(got, "\x1b[0m") {
		t.Errorf("enabled green should wrap with SGR codes, got %q", got)
	}
	// Empty strings are never wrapped (avoids stray escapes around nothing).
	if s.Green("") != "" {
		t.Errorf("empty input should not be wrapped, got %q", s.Green(""))
	}
}

func TestBalanceColors(t *testing.T) {
	s := New(true)
	if !strings.Contains(s.Balance(5, "5m"), "\x1b[32m") {
		t.Error("positive balance should be green")
	}
	if !strings.Contains(s.Balance(-5, "-5m"), "\x1b[31m") {
		t.Error("negative balance should be red")
	}
	if !strings.Contains(s.Balance(0, "0m"), "\x1b[2m") {
		t.Error("zero balance should be dim")
	}

	// When disabled, balance never adds escapes regardless of sign.
	d := New(false)
	for _, v := range []int{-1, 0, 1} {
		if strings.Contains(d.Balance(v, "x"), "\x1b") {
			t.Errorf("disabled balance emitted escape for %d", v)
		}
	}
}

func TestVisibleWidthIgnoresEscapes(t *testing.T) {
	s := New(true)
	colored := s.Green("hello") // has escapes
	if VisibleWidth(colored) != 5 {
		t.Errorf("visible width should ignore escapes, got %d for %q", VisibleWidth(colored), colored)
	}
	// Multibyte runes count as one column each.
	if VisibleWidth("héllo–x") != 7 {
		t.Errorf("multibyte width wrong: %d", VisibleWidth("héllo–x"))
	}
}

func TestPadRightAnsiAware(t *testing.T) {
	s := New(true)
	colored := s.Green("ab") // visible width 2
	out := PadRight(colored, 5)
	if VisibleWidth(out) != 5 {
		t.Errorf("PadRight should pad to visible width 5, got %d", VisibleWidth(out))
	}
	if !strings.HasSuffix(out, "   ") {
		t.Errorf("PadRight should append spaces, got %q", out)
	}
	// Already wide enough: unchanged.
	if PadRight("abcdef", 3) != "abcdef" {
		t.Error("PadRight should not truncate")
	}
}

func TestPadLeftAnsiAware(t *testing.T) {
	out := PadLeft("ab", 5)
	if out != "   ab" {
		t.Errorf("PadLeft wrong: %q", out)
	}
}

func TestBoxBordersAlignToWidestLine(t *testing.T) {
	s := New(false) // disabled: borders drawn, no color escapes
	lines := []string{"short", "a much longer line"}
	out := s.Box("Title", lines)
	rows := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(rows) != 4 { // top + 2 content + bottom
		t.Fatalf("expected 4 rows, got %d: %q", len(rows), out)
	}
	// Every row must have identical visible width.
	want := VisibleWidth(rows[0])
	for i, r := range rows {
		if VisibleWidth(r) != want {
			t.Errorf("row %d width %d != %d (%q)", i, VisibleWidth(r), want, r)
		}
	}
	// Box must contain the corners.
	if !strings.Contains(out, tl) || !strings.Contains(out, br) {
		t.Error("box missing corner runes")
	}
}

func TestBoxWithColoredContentStaysAligned(t *testing.T) {
	s := New(true)
	lines := []string{s.Green("ok"), s.Red("longer text")}
	out := s.Box("", lines)
	rows := strings.Split(strings.TrimRight(out, "\n"), "\n")
	want := VisibleWidth(rows[0])
	for i, r := range rows {
		if VisibleWidth(r) != want {
			t.Errorf("colored row %d visible width %d != %d", i, VisibleWidth(r), want)
		}
	}
}

func TestBarWidthAndClamp(t *testing.T) {
	s := New(false) // no color, easier to measure
	full := s.Bar(1.0, 10, nil)
	if VisibleWidth(full) != 10 {
		t.Errorf("full bar width = %d, want 10", VisibleWidth(full))
	}
	if strings.TrimRight(full, "█") != "" {
		t.Errorf("full bar should be all full blocks, got %q", full)
	}
	empty := s.Bar(0, 10, nil)
	if VisibleWidth(empty) != 10 {
		t.Errorf("empty bar width = %d, want 10", VisibleWidth(empty))
	}
	if strings.TrimSpace(empty) != "" {
		t.Errorf("empty bar should be blank, got %q", empty)
	}
	if VisibleWidth(s.Bar(5, 8, nil)) != 8 {
		t.Error("over-1 fraction should clamp to full width")
	}
	if VisibleWidth(s.Bar(-1, 8, nil)) != 8 {
		t.Error("negative fraction should clamp to empty width")
	}
	if s.Bar(0.5, 0, nil) != "" {
		t.Error("zero width should yield empty string")
	}
}

func TestBarColored(t *testing.T) {
	s := New(true)
	out := s.Bar(0.5, 10, s.CyanFn())
	if !strings.Contains(out, "\x1b[36m") {
		t.Errorf("colored bar should contain cyan code, got %q", out)
	}
	if VisibleWidth(out) != 10 {
		t.Errorf("colored bar visible width = %d, want 10", VisibleWidth(out))
	}
}
