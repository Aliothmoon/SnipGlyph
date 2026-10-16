package ui

import (
	"image"
	"testing"

	"github.com/tailscale/walk"
)

func TestPlaceIn(t *testing.T) {
	work := image.Rect(0, 0, 1920, 1040)
	const w, h, gap = 440, 240, 8
	tests := []struct {
		name   string
		anchor image.Rectangle
		want   walk.Rectangle
	}{
		{"below", image.Rect(100, 100, 500, 300), walk.Rectangle{X: 100, Y: 308, Width: w, Height: h}},
		{"above when no room below", image.Rect(100, 700, 500, 900), walk.Rectangle{X: 100, Y: 452, Width: w, Height: h}},
		{"clamped right", image.Rect(1800, 100, 1900, 200), walk.Rectangle{X: 1480, Y: 208, Width: w, Height: h}},
		{"clamped when no room either way", image.Rect(0, 100, 1920, 1000), walk.Rectangle{X: 0, Y: 0, Width: w, Height: h}},
	}
	for _, tt := range tests {
		if got := placeIn(tt.anchor, work, w, h, gap); got != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
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
