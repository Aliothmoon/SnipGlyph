// Package capture freezes the screen and lets the user pick a region of it,
// by dragging a rectangle or by clicking the window or control under the
// cursor.
package capture

import (
	"slices"
)

// Rect is a half-open pixel rectangle: Right and Bottom are exclusive.
type Rect struct {
	Left, Top, Right, Bottom int
}

// rectFromPoints spans two points, including both.
func rectFromPoints(ax, ay, bx, by int) Rect {
	return Rect{min(ax, bx), min(ay, by), max(ax, bx) + 1, max(ay, by) + 1}
}

func (r Rect) Dx() int     { return r.Right - r.Left }
func (r Rect) Dy() int     { return r.Bottom - r.Top }
func (r Rect) Empty() bool { return r.Dx() <= 0 || r.Dy() <= 0 }
func (r Rect) area() int {
	if r.Empty() {
		return 0
	}
	return r.Dx() * r.Dy()
}

func (r Rect) Contains(x, y int) bool {
	return x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom
}

func (r Rect) Intersect(o Rect) Rect {
	i := Rect{max(r.Left, o.Left), max(r.Top, o.Top), min(r.Right, o.Right), min(r.Bottom, o.Bottom)}
	if i.Empty() {
		return Rect{}
	}
	return i
}

func (r Rect) Union(o Rect) Rect {
	switch {
	case r.Empty():
		return o
	case o.Empty():
		return r
	}
	return Rect{min(r.Left, o.Left), min(r.Top, o.Top), max(r.Right, o.Right), max(r.Bottom, o.Bottom)}
}

// Outset grows r by n on every side.
func (r Rect) Outset(n int) Rect {
	return Rect{r.Left - n, r.Top - n, r.Right + n, r.Bottom + n}
}

func (r Rect) Add(dx, dy int) Rect {
	return Rect{r.Left + dx, r.Top + dy, r.Right + dx, r.Bottom + dy}
}

// window is a top-level window as seen when the screen was frozen.
type window struct {
	rect     Rect
	children []Rect // descendant windows, already clipped to rect
	desktop  bool   // the shell's desktop: pick the monitor instead
}

// layout is everything clickable, in overlay client coordinates.
type layout struct {
	windows  []window // z-order, topmost first
	monitors []Rect
}

// chainAt lists the rectangles under (x, y) from smallest to largest: the
// controls of the topmost window there, the window itself, its monitor.
func (l layout) chainAt(x, y int) []Rect {
	var out []Rect
	for _, w := range l.windows {
		if !w.rect.Contains(x, y) {
			continue
		}
		if !w.desktop {
			for _, c := range w.children {
				if c.Contains(x, y) {
					out = append(out, c)
				}
			}
			out = append(out, w.rect)
		}
		break
	}
	for _, m := range l.monitors {
		if m.Contains(x, y) {
			out = append(out, m)
			break
		}
	}
	slices.SortStableFunc(out, func(a, b Rect) int { return a.area() - b.area() })
	return slices.Compact(out)
}
