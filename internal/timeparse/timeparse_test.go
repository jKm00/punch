package timeparse

import (
	"testing"
	"time"
)

func refNow() time.Time {
	// Wednesday 2026-06-24 10:00 local.
	return time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
}

func TestParseDate(t *testing.T) {
	now := refNow()
	tests := []struct {
		in      string
		wantY   int
		wantM   time.Month
		wantD   int
		wantErr bool
	}{
		{in: "today", wantY: 2026, wantM: 6, wantD: 24},
		{in: "yesterday", wantY: 2026, wantM: 6, wantD: 23},
		{in: "15.02.2025", wantY: 2025, wantM: 2, wantD: 15},
		{in: "15-02-2025", wantY: 2025, wantM: 2, wantD: 15},
		{in: "15.02", wantY: 2026, wantM: 2, wantD: 15},
		{in: "15-02", wantY: 2026, wantM: 2, wantD: 15},
		{in: "01.01.2000", wantY: 2000, wantM: 1, wantD: 1},
		// rejections
		{in: "15.02.25", wantErr: true},   // 2-digit year
		{in: "2026-06-24", wantErr: true}, // ISO/year-first ambiguous
		{in: "31.02.2026", wantErr: true}, // no such day
		{in: "00.01.2026", wantErr: true}, // day 0
		{in: "15.13.2026", wantErr: true}, // month 13
		{in: "tomorrow", wantErr: true},
		{in: "", wantErr: true},
		{in: "15/02/2026", wantErr: true}, // slash not supported
	}
	for _, tc := range tests {
		got, err := ParseDate(tc.in, now)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseDate(%q): expected error, got %v", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseDate(%q): unexpected error %v", tc.in, err)
			continue
		}
		if got.Year() != tc.wantY || got.Month() != tc.wantM || got.Day() != tc.wantD {
			t.Errorf("ParseDate(%q) = %v, want %d-%02d-%02d", tc.in, got, tc.wantY, tc.wantM, tc.wantD)
		}
		if got.Hour() != 0 || got.Minute() != 0 {
			t.Errorf("ParseDate(%q) has non-zero time component: %v", tc.in, got)
		}
	}
}

func TestParseTime(t *testing.T) {
	tests := []struct {
		in      string
		h, m    int
		wantErr bool
	}{
		{in: "08:30", h: 8, m: 30},
		{in: "00:00", h: 0, m: 0},
		{in: "23:59", h: 23, m: 59},
		{in: "24:00", wantErr: true},
		{in: "08:60", wantErr: true},
		{in: "8", wantErr: true},
		{in: "08-30", wantErr: true},
	}
	for _, tc := range tests {
		h, m, err := ParseTime(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseTime(%q): expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseTime(%q): unexpected error %v", tc.in, err)
			continue
		}
		if h != tc.h || m != tc.m {
			t.Errorf("ParseTime(%q) = %d:%d, want %d:%d", tc.in, h, m, tc.h, tc.m)
		}
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{in: "30m", want: 30},
		{in: "7h", want: 420},
		{in: "7h30m", want: 450},
		{in: "7:30", want: 450},
		{in: "0m", want: 0},
		{in: "-5m", wantErr: true},
		{in: "30s", wantErr: true}, // sub-minute precision
		{in: "", wantErr: true},
		{in: "abc", wantErr: true},
	}
	for _, tc := range tests {
		got, err := ParseDuration(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseDuration(%q): expected error, got %d", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseDuration(%q): unexpected error %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseDuration(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
