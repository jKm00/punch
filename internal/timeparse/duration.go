package timeparse

import (
	"fmt"
	"strings"
	"time"
)

// ParseDuration parses a duration into minutes. It accepts Go-style durations
// such as "30m", "7h", "7h30m", and also "HH:MM" (e.g. "7:30"). Negative and
// sub-minute precision are rejected.
func ParseDuration(s string) (int, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return 0, fmt.Errorf("empty duration")
	}
	// HH:MM form.
	if strings.Contains(trimmed, ":") {
		h, m, err := ParseTime(trimmed)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: %v", s, err)
		}
		return h*60 + m, nil
	}
	d, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: use forms like 30m, 7h, 7h30m or HH:MM", s)
	}
	if d < 0 {
		return 0, fmt.Errorf("invalid duration %q: must not be negative", s)
	}
	mins := d.Minutes()
	if mins != float64(int(mins)) {
		return 0, fmt.Errorf("invalid duration %q: sub-minute precision not allowed", s)
	}
	return int(mins), nil
}
