package ui

import (
	"strings"
	"testing"
	"time"

	"SnipGlyph/internal/i18n"
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

func TestAgoLocalized(t *testing.T) {
	orig := i18n.Active()
	t.Cleanup(func() { i18n.Set(orig) })

	now := time.Date(2026, 4, 15, 14, 30, 0, 0, time.Local)
	yest := time.Date(2026, 4, 14, 9, 5, 0, 0, time.Local)

	i18n.Set(i18n.ZhCN)
	if got := ago(now.Add(-20*time.Second), now); got != "刚刚" {
		t.Errorf("zh just now = %q", got)
	}
	if got := ago(now.Add(-5*time.Minute), now); got != "5 分钟前" {
		t.Errorf("zh 5m = %q", got)
	}
	if got := ago(yest, now); got != "昨天 09:05" {
		t.Errorf("zh yesterday = %q", got)
	}

	i18n.Set(i18n.En)
	if got := ago(now.Add(-20*time.Second), now); got != "Just now" {
		t.Errorf("en just now = %q", got)
	}
	if got := ago(now.Add(-5*time.Minute), now); got != "5m ago" {
		t.Errorf("en 5m = %q", got)
	}
	if got := ago(yest, now); got != "Yest. 09:05" {
		t.Errorf("en yesterday = %q", got)
	}
}
