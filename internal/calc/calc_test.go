package calc

import (
	"testing"
	"time"

	"punch/internal/domain"
)

func at(h, m int) time.Time {
	return time.Date(2026, 6, 24, h, m, 0, 0, time.UTC)
}

func TestWorkedMinutes(t *testing.T) {
	tests := []struct {
		name        string
		start, end  time.Time
		lunch, want int
	}{
		{"full day", at(8, 0), at(16, 0), 30, 8*60 - 30}, // 7h30m
		{"no lunch", at(9, 0), at(17, 0), 0, 8 * 60},     // 8h
		{"clamp to zero", at(12, 0), at(12, 10), 30, 0},  // lunch > gross
		{"exactly lunch", at(12, 0), at(12, 30), 30, 0},  // net 0
		{"one minute over lunch", at(12, 0), at(12, 31), 30, 1},
	}
	for _, tc := range tests {
		got := WorkedMinutes(tc.start, tc.end, tc.lunch)
		if got != tc.want {
			t.Errorf("%s: WorkedMinutes = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestLoggingEnd(t *testing.T) {
	tests := []struct {
		season         domain.Season
		extra          int
		wantSH, wantSM int
		wantEH, wantEM int
	}{
		// winter starts 16:00, extra 1h15m -> 17:15
		{domain.Winter, 75, 16, 0, 17, 15},
		// summer starts 15:30, extra 30m -> 16:00
		{domain.Summer, 30, 15, 30, 16, 0},
		// winter starts 16:00, extra 0 -> 16:00
		{domain.Winter, 0, 16, 0, 16, 0},
		// summer starts 15:30, extra 2h45m -> 18:15
		{domain.Summer, 165, 15, 30, 18, 15},
	}
	for _, tc := range tests {
		sh, sm, eh, em := LoggingEnd(tc.season, tc.extra)
		if sh != tc.wantSH || sm != tc.wantSM || eh != tc.wantEH || em != tc.wantEM {
			t.Errorf("LoggingEnd(%s,%d) = %02d:%02d-%02d:%02d, want %02d:%02d-%02d:%02d",
				tc.season, tc.extra, sh, sm, eh, em, tc.wantSH, tc.wantSM, tc.wantEH, tc.wantEM)
		}
	}
}

func TestFormatHM(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{450, "7h30m"},
		{420, "7h"},
		{45, "45m"},
		{0, "0m"},
		{-45, "-45m"},
		{-75, "-1h15m"},
	}
	for _, tc := range tests {
		if got := FormatHM(tc.in); got != tc.want {
			t.Errorf("FormatHM(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatDecimalHours(t *testing.T) {
	if got := FormatDecimalHours(450); got != "7.50h" {
		t.Errorf("FormatDecimalHours(450) = %q, want 7.50h", got)
	}
}
