package toast

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestOpacityTimeline(t *testing.T) {
	hold := 2 * time.Second
	tests := []struct {
		t    time.Duration
		want float64
		ok   bool
	}{
		{0, 0, true},
		{fadeIn / 2, 0.5, true},
		{fadeIn + time.Second, 1, true},
		{fadeIn + hold + fadeOut/2, 0.5, true},
		{fadeIn + hold + fadeOut, 0, false},
	}
	for _, tt := range tests {
		a, ok := opacity(tt.t, hold)
		if ok != tt.ok || (ok && (a < tt.want-0.01 || a > tt.want+0.01)) {
			t.Errorf("opacity(%v) = %v, %v; want %v, %v", tt.t, a, ok, tt.want, tt.ok)
		}
	}
}

func TestHoldFor(t *testing.T) {
	if got := holdFor("已复制 3 行文字"); got != minHold {
		t.Errorf("short text: %v", got)
	}
	if got := holdFor(string(make([]rune, 500))); got != maxHold {
		t.Errorf("long text: %v", got)
	}
}

func TestCoverage(t *testing.T) {
	const w, h, r = 100, 40, 20
	if c := coverage(50, 20, w, h, r); c != 1 {
		t.Errorf("centre = %v", c)
	}
	if c := coverage(0, 0, w, h, r); c != 0 {
		t.Errorf("rounded-off corner = %v", c)
	}
	if c := coverage(50, 0, w, h, r); c != 1 {
		t.Errorf("straight top edge = %v", c)
	}
	// The arc crosses pixel (5, 5) at 45° (20 - 20/√2 ≈ 5.86): partly covered.
	if c := coverage(5, 5, w, h, r); c <= 0 || c >= 1 {
		t.Errorf("arc pixel = %v, want partial", c)
	}
}

func TestClip(t *testing.T) {
	if got := clip("  short  "); got != "short" {
		t.Errorf("clip(short) = %q", got)
	}
	lines := clip(strings.Repeat("line\n", 10))
	if strings.Count(lines, "\n") != maxLines-1 || !strings.HasSuffix(lines, "…") {
		t.Errorf("clip(10 lines) = %q", lines)
	}
	long := clip(strings.Repeat("字", 500))
	if n := utf8.RuneCountInString(long); n != maxRunes+1 || !strings.HasSuffix(long, "…") {
		t.Errorf("clip(500 runes) has %d runes", n)
	}
}
