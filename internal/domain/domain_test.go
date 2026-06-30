package domain

import (
	"testing"
	"time"
)

func date(month, day int) time.Time {
	return time.Date(2026, time.Month(month), day, 12, 0, 0, 0, time.UTC)
}

func TestSeasonFor(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		date time.Time
		want Season
	}{
		{
			name: "boundary start is summer (inclusive)",
			cfg:  DefaultConfig(),
			date: date(5, 15),
			want: Summer,
		},
		{
			name: "boundary end is summer (inclusive)",
			cfg:  DefaultConfig(),
			date: date(8, 31),
			want: Summer,
		},
		{
			name: "mid-summer",
			cfg:  DefaultConfig(),
			date: date(7, 1),
			want: Summer,
		},
		{
			name: "just before start is winter",
			cfg:  DefaultConfig(),
			date: date(5, 14),
			want: Winter,
		},
		{
			name: "just after end is winter",
			cfg:  DefaultConfig(),
			date: date(9, 1),
			want: Winter,
		},
		{
			name: "deep winter",
			cfg:  DefaultConfig(),
			date: date(1, 15),
			want: Winter,
		},
		{
			name: "wrap-around interval inside (after start)",
			cfg: Config{
				SeasonsEnabled: true,
				SummerStart:    MonthDay{Month: 11, Day: 1},
				SummerEnd:      MonthDay{Month: 2, Day: 28},
			},
			date: date(12, 25),
			want: Summer,
		},
		{
			name: "wrap-around interval inside (before end)",
			cfg: Config{
				SeasonsEnabled: true,
				SummerStart:    MonthDay{Month: 11, Day: 1},
				SummerEnd:      MonthDay{Month: 2, Day: 28},
			},
			date: date(1, 15),
			want: Summer,
		},
		{
			name: "wrap-around interval outside",
			cfg: Config{
				SeasonsEnabled: true,
				SummerStart:    MonthDay{Month: 11, Day: 1},
				SummerEnd:      MonthDay{Month: 2, Day: 28},
			},
			date: date(7, 1),
			want: Winter,
		},
		{
			name: "wrap-around boundaries inclusive (start)",
			cfg: Config{
				SeasonsEnabled: true,
				SummerStart:    MonthDay{Month: 11, Day: 1},
				SummerEnd:      MonthDay{Month: 2, Day: 28},
			},
			date: date(11, 1),
			want: Summer,
		},
		{
			name: "wrap-around boundaries inclusive (end)",
			cfg: Config{
				SeasonsEnabled: true,
				SummerStart:    MonthDay{Month: 11, Day: 1},
				SummerEnd:      MonthDay{Month: 2, Day: 28},
			},
			date: date(2, 28),
			want: Summer,
		},
		{
			name: "seasons disabled always winter (summer date)",
			cfg: Config{
				SeasonsEnabled: false,
				SummerStart:    MonthDay{Month: 5, Day: 15},
				SummerEnd:      MonthDay{Month: 8, Day: 31},
			},
			date: date(7, 1),
			want: Winter,
		},
		{
			name: "seasons disabled always winter (winter date)",
			cfg: Config{
				SeasonsEnabled: false,
				SummerStart:    MonthDay{Month: 5, Day: 15},
				SummerEnd:      MonthDay{Month: 8, Day: 31},
			},
			date: date(1, 1),
			want: Winter,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.SeasonFor(tc.date); got != tc.want {
				t.Errorf("SeasonFor(%s) = %s, want %s", tc.date.Format("02.01"), got, tc.want)
			}
		})
	}
}
