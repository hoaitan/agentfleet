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
		{"multi-day", 25*time.Hour + time.Minute + 2*time.Second, "25:01:02"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatElapsed(tc.in); got != tc.want {
				t.Errorf("formatElapsed(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
