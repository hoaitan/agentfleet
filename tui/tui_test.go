package tui

import (
	"testing"
	"time"
)

func TestFormatElapsed(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want string
	}{
		{"zero", 0, "00:00"},
		{"seconds", 5 * time.Second, "00:05"},
		{"sub-second rounds", 5*time.Second + 400*time.Millisecond, "00:05"},
		{"minutes", 90 * time.Second, "01:30"},
		{"just under an hour", 59*time.Minute + 59*time.Second, "59:59"},
		{"exactly one hour", time.Hour, "1:00:00"},
		{"hours minutes seconds", time.Hour + 15*time.Minute + 30*time.Second, "1:15:30"},
		{"just under a day", 23*time.Hour + 59*time.Minute + 59*time.Second, "23:59:59"},
		{"exactly one day", 24 * time.Hour, "1d0h"},
		{"day drops seconds", 25*time.Hour + time.Minute + 2*time.Second, "1d1h"},
		{"multi-day", 2*24*time.Hour + 3*time.Hour + 15*time.Minute, "2d3h"},
		{"two weeks", 14 * 24 * time.Hour, "14d0h"},
		{"a year", 365*24*time.Hour + 23*time.Hour, "365d23h"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatElapsed(tc.in); got != tc.want {
				t.Errorf("formatElapsed(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestFormatElapsedFitsColumn pins the invariant that sizes the card's elapsed
// column: lipgloss wraps rather than truncates when content exceeds Width, so
// an over-long value would corrupt the card layout.
func TestFormatElapsedFitsColumn(t *testing.T) {
	const columnWidth = 8
	for _, d := range []time.Duration{
		0,
		59*time.Minute + 59*time.Second,
		23*time.Hour + 59*time.Minute + 59*time.Second,
		365 * 24 * time.Hour,
		999*24*time.Hour + 23*time.Hour,
	} {
		if got := formatElapsed(d); len(got) > columnWidth {
			t.Errorf("formatElapsed(%v) = %q is %d chars, exceeds the %d-wide column", d, got, len(got), columnWidth)
		}
	}
}
