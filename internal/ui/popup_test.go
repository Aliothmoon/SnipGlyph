package ui

import (
	"image"
	"testing"

	"github.com/tailscale/walk"
)

func TestCenterIn(t *testing.T) {
	// A second monitor to the right, below a taskbar-less top edge.
	work := image.Rect(1920, 0, 3840, 1040)
	if got, want := centerIn(work, 960, 510), (walk.Rectangle{X: 2400, Y: 265, Width: 960, Height: 510}); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	// Too big for a small screen: shrunk to the work area.
	small := image.Rect(0, 0, 800, 560)
	if got, want := centerIn(small, 960, 870), (walk.Rectangle{X: 0, Y: 0, Width: 800, Height: 560}); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestResizeAround(t *testing.T) {
	work := image.Rect(0, 0, 1920, 1040)
	tests := []struct {
		name string
		b    walk.Rectangle
		w, h int
		want walk.Rectangle
	}{
		{"keeps the centre", walk.Rectangle{X: 480, Y: 265, Width: 960, Height: 510}, 1000, 870, walk.Rectangle{X: 460, Y: 85, Width: 1000, Height: 870}},
		{"shrinks around the centre", walk.Rectangle{X: 480, Y: 265, Width: 960, Height: 510}, 600, 300, walk.Rectangle{X: 660, Y: 370, Width: 600, Height: 300}},
		{"pushed down from the top", walk.Rectangle{X: 0, Y: 10, Width: 960, Height: 200}, 960, 600, walk.Rectangle{X: 0, Y: 0, Width: 960, Height: 600}},
		{"pushed up from the bottom", walk.Rectangle{X: 0, Y: 900, Width: 960, Height: 100}, 960, 600, walk.Rectangle{X: 0, Y: 440, Width: 960, Height: 600}},
		{"clamped to the work area", walk.Rectangle{X: 0, Y: 0, Width: 960, Height: 100}, 3000, 2000, walk.Rectangle{X: 0, Y: 0, Width: 1920, Height: 1040}},
	}
	for _, tt := range tests {
		if got := resizeAround(tt.b, tt.w, tt.h, work); got != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

func TestGrowDown(t *testing.T) {
	work := image.Rect(0, 0, 1920, 1040)
	b := walk.Rectangle{X: 660, Y: 370, Width: 600, Height: 300}
	if got, want := growDown(b, 700, 420, work), (walk.Rectangle{X: 610, Y: 370, Width: 700, Height: 420}); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	// Near the bottom it is pushed up rather than running off screen.
	b.Y = 700
	if got, want := growDown(b, 600, 420, work), (walk.Rectangle{X: 660, Y: 620, Width: 600, Height: 420}); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestCRLF(t *testing.T) {
	if got := crlf("a\nb\r\nc\n"); got != "a\r\nb\r\nc\r\n" {
		t.Fatalf("got %q", got)
	}
}

func TestLF(t *testing.T) {
	if got := lf(crlf("a\nb\n\nc")); got != "a\nb\n\nc" {
		t.Fatalf("got %q", got)
	}
	if got := lf("lone\rcr"); got != "lone\rcr" {
		t.Fatalf("got %q", got)
	}
}
