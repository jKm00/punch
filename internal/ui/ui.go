// Package ui provides terminal styling for wh: ANSI colors, text attributes,
// and box-drawing helpers. All styling is gated behind an Enabled flag so the
// same code path produces clean, plain text when output is piped/redirected,
// when NO_COLOR is set, or when --no-color is passed.
package ui

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

// ANSI SGR codes used by wh.
const (
	codeReset  = "\x1b[0m"
	codeBold   = "\x1b[1m"
	codeDim    = "\x1b[2m"
	codeRed    = "\x1b[31m"
	codeGreen  = "\x1b[32m"
	codeYellow = "\x1b[33m"
	codeCyan   = "\x1b[36m"
)

// Styler renders styled strings. When Enabled is false every styling method
// returns its input unchanged, so output is plain and pipeable.
type Styler struct {
	Enabled bool
}

// New returns a Styler with the given enabled state.
func New(enabled bool) *Styler { return &Styler{Enabled: enabled} }

// ShouldEnable decides whether color should be on for the given output file.
// Color is enabled only when: the NO_COLOR env var is unset, noColorFlag is
// false, and out is a character device (an interactive terminal). This keeps
// redirected/piped output plain.
func ShouldEnable(out *os.File, noColorFlag bool) bool {
	if noColorFlag {
		return false
	}
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	if out == nil {
		return false
	}
	info, err := out.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func (s *Styler) wrap(code, text string) string {
	if !s.Enabled || text == "" {
		return text
	}
	return code + text + codeReset
}

// Bold renders text in bold.
func (s *Styler) Bold(text string) string { return s.wrap(codeBold, text) }

// Dim renders text dimmed.
func (s *Styler) Dim(text string) string { return s.wrap(codeDim, text) }

// Red renders text in red.
func (s *Styler) Red(text string) string { return s.wrap(codeRed, text) }

// Green renders text in green.
func (s *Styler) Green(text string) string { return s.wrap(codeGreen, text) }

// Yellow renders text in yellow.
func (s *Styler) Yellow(text string) string { return s.wrap(codeYellow, text) }

// Cyan renders text in cyan.
func (s *Styler) Cyan(text string) string { return s.wrap(codeCyan, text) }

// BoldColor combines bold with one of the color helpers.
func (s *Styler) BoldColor(color func(string) string, text string) string {
	return s.Bold(color(text))
}

// Balance colors a signed value string: green when positive, red when
// negative, dim when zero.
func (s *Styler) Balance(minutes int, text string) string {
	switch {
	case minutes > 0:
		return s.Green(text)
	case minutes < 0:
		return s.Red(text)
	default:
		return s.Dim(text)
	}
}

// visibleWidth returns the display width of text, ignoring ANSI escape
// sequences so box layout stays aligned whether or not color is enabled.
func visibleWidth(text string) int {
	w := 0
	inEsc := false
	for i := 0; i < len(text); {
		if text[i] == '\x1b' {
			inEsc = true
			i++
			continue
		}
		if inEsc {
			// ANSI SGR sequences end with 'm'.
			if text[i] == 'm' {
				inEsc = false
			}
			i++
			continue
		}
		_, size := utf8.DecodeRuneInString(text[i:])
		w++
		i += size
	}
	return w
}

// Box-drawing runes.
const (
	tl = "┌"
	tr = "┐"
	bl = "└"
	br = "┘"
	h  = "─"
	v  = "│"
	lt = "├"
	rt = "┤"
)

// Box renders a titled box around the given content lines. The box auto-sizes
// to the widest line (or the title). When styling is disabled the borders are
// still drawn (plain ASCII-art is useful even without color); only color
// escapes are suppressed via the individual cell strings.
//
// title may be empty. content lines may contain ANSI escapes; width is measured
// against their visible width.
func (s *Styler) Box(title string, lines []string) string {
	inner := 0
	if title != "" {
		inner = visibleWidth(title)
	}
	for _, ln := range lines {
		if w := visibleWidth(ln); w > inner {
			inner = w
		}
	}
	// 1 space of padding on each side.
	pad := 1
	width := inner + pad*2

	border := s.Dim(v)
	var b strings.Builder

	// Top border, optionally with an inset title.
	if title != "" {
		t := s.Bold(title)
		// "┌─ title " + fill + "┐"
		used := 2 + 1 + visibleWidth(title) + 1 // "┌─" + space + title + space
		fill := width - (used - 1)              // remaining ─ before ┐ (account for the one ─ already drawn)
		if fill < 0 {
			fill = 0
		}
		b.WriteString(s.Dim(tl + h))
		b.WriteString(" " + t + " ")
		b.WriteString(s.Dim(strings.Repeat(h, fill) + tr))
		b.WriteByte('\n')
	} else {
		b.WriteString(s.Dim(tl + strings.Repeat(h, width) + tr))
		b.WriteByte('\n')
	}

	for _, ln := range lines {
		gap := width - pad*2 - visibleWidth(ln)
		if gap < 0 {
			gap = 0
		}
		b.WriteString(border)
		b.WriteString(strings.Repeat(" ", pad))
		b.WriteString(ln)
		b.WriteString(strings.Repeat(" ", gap+pad))
		b.WriteString(border)
		b.WriteByte('\n')
	}

	b.WriteString(s.Dim(bl + strings.Repeat(h, width) + br))
	b.WriteByte('\n')
	return b.String()
}

// Rule renders a horizontal separator the given visible width.
func (s *Styler) Rule(width int) string {
	return s.Dim(strings.Repeat(h, width))
}

// PadRight pads text on the right to a visible width of n (ANSI-aware).
func PadRight(text string, n int) string {
	w := visibleWidth(text)
	if w >= n {
		return text
	}
	return text + strings.Repeat(" ", n-w)
}

// PadLeft pads text on the left to a visible width of n (ANSI-aware).
func PadLeft(text string, n int) string {
	w := visibleWidth(text)
	if w >= n {
		return text
	}
	return strings.Repeat(" ", n-w) + text
}

// VisibleWidth is the exported display width (ANSI-aware).
func VisibleWidth(text string) int { return visibleWidth(text) }

// Sprintf is a convenience wrapper kept for symmetry with fmt usage in callers.
func Sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }
