package ui

import (
	"strings"
	"testing"
	"time"
)

func TestShortTime(t *testing.T) {
	now := time.Date(2026, 10, 17, 15, 0, 0, 0, time.Local)
	for _, tc := range []struct {
		t    time.Time
		want string
	}{
		{time.Date(2026, 10, 17, 9, 5, 0, 0, time.Local), "09:05"},
		{time.Date(2026, 10, 16, 23, 59, 0, 0, time.Local), "10-16 23:59"},
		{time.Date(2025, 10, 17, 9, 5, 0, 0, time.Local), "2025-10-17"},
	} {
		if got := shortTime(tc.t, now); got != tc.want {
			t.Errorf("shortTime(%v) = %q, want %q", tc.t, got, tc.want)
		}
	}
}

func TestPreview(t *testing.T) {
	if got := preview("  line one\r\n\tline two  "); got != "line one line two" {
		t.Errorf("preview = %q", got)
	}
	long := strings.Repeat("字", 200)
	if got := preview(long); got != strings.Repeat("字", 120)+"…" {
		t.Errorf("long preview has %d runes", len([]rune(got)))
	}
}
