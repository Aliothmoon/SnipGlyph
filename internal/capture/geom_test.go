package capture

import (
	"slices"
	"testing"
)

func TestRectFromPointsIncludesBothEnds(t *testing.T) {
	r := rectFromPoints(10, 20, 4, 5)
	if r != (Rect{4, 5, 11, 21}) {
		t.Fatalf("got %+v", r)
	}
	if !r.Contains(10, 20) || !r.Contains(4, 5) {
		t.Fatal("endpoints not contained")
	}
}

func TestIntersectUnion(t *testing.T) {
	a, b := Rect{0, 0, 10, 10}, Rect{5, 5, 20, 20}
	if got := a.Intersect(b); got != (Rect{5, 5, 10, 10}) {
		t.Errorf("Intersect = %+v", got)
	}
	if got := a.Intersect(Rect{10, 0, 20, 10}); !got.Empty() {
		t.Errorf("touching rects intersect: %+v", got)
	}
	if got := a.Union(b); got != (Rect{0, 0, 20, 20}) {
		t.Errorf("Union = %+v", got)
	}
	if got := (Rect{}).Union(b); got != b {
		t.Errorf("empty Union = %+v", got)
	}
}

func TestChainAt(t *testing.T) {
	mon1, mon2 := Rect{0, 0, 1920, 1080}, Rect{1920, 0, 3840, 1080}
	editor := window{
		rect: Rect{100, 100, 900, 700},
		children: []Rect{
			{100, 100, 900, 700}, // client area same as window: deduplicated
			{120, 150, 880, 680}, // text area
			{120, 150, 300, 200}, // a button inside it
		},
	}
	behind := window{rect: Rect{0, 0, 1000, 1000}}
	l := layout{
		windows:  []window{editor, behind, {rect: Rect{0, 0, 3840, 1080}, desktop: true}},
		monitors: []Rect{mon1, mon2},
	}

	tests := []struct {
		name string
		x, y int
		want []Rect
	}{
		{"button", 130, 160, []Rect{{120, 150, 300, 200}, {120, 150, 880, 680}, editor.rect, mon1}},
		{"text area", 500, 500, []Rect{{120, 150, 880, 680}, editor.rect, mon1}},
		{"window behind", 950, 950, []Rect{behind.rect, mon1}},
		{"desktop", 3000, 500, []Rect{mon2}},
	}
	for _, tt := range tests {
		if got := l.chainAt(tt.x, tt.y); !slices.Equal(got, tt.want) {
			t.Errorf("%s: chainAt(%d,%d) = %v, want %v", tt.name, tt.x, tt.y, got, tt.want)
		}
	}
}
