package ui

import (
	"unsafe"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"

	"SnipGlyph/internal/cliphist"
)

const glyphAdd = "\uE710"

// tabStrip shows the clipboard tabs as a row of names, the current one
// underlined, and a + button that adds a tab.
type tabStrip struct {
	cw    *walk.CustomWidget
	tabs  []cliphist.Tab
	cur   int
	hot   int              // index under the mouse; len(tabs) is the + button
	rects []walk.Rectangle // per tab, then the + button; set by paint

	onSelect      func(i int)
	onAdd         func()
	onRightButton func(i int) // i < 0 off any tab; before the context menu
}

func newTabStrip(parent walk.Container) (*tabStrip, error) {
	t := &tabStrip{hot: -1}
	cw, err := walk.NewCustomWidgetPixels(parent, 0, t.paint)
	if err != nil {
		return nil, err
	}
	t.cw = cw
	cw.SetClearsBackground(false)
	cw.SetInvalidatesOnResize(true)
	cw.SetPaintMode(walk.PaintBuffered)
	cw.SetMinMaxSize(walk.Size{Height: 32}, walk.Size{Height: 32})
	cw.MouseMove().Attach(func(x, y int, _ walk.MouseButton) {
		if i := t.at(x, y); i != t.hot {
			if t.hot == -1 {
				tr := win.TRACKMOUSEEVENT{DwFlags: win.TME_LEAVE, HwndTrack: cw.Handle()}
				tr.CbSize = uint32(unsafe.Sizeof(tr))
				win.TrackMouseEvent(&tr)
			}
			t.hot = i
			cw.Invalidate()
		}
	})
	cw.MouseDown().Attach(func(x, y int, button walk.MouseButton) {
		i := t.at(x, y)
		switch {
		case button == walk.RightButton:
			if t.onRightButton != nil {
				t.onRightButton(min(i, len(t.tabs)-1))
			}
		case i == len(t.tabs):
			t.onAdd()
		case i >= 0:
			t.onSelect(i)
		}
	})
	hookWindow(cw.Handle(), func(_ win.HWND, msg uint32, _, _ uintptr) (uintptr, bool) {
		if msg == win.WM_MOUSELEAVE {
			t.hot = -1
			cw.Invalidate()
		}
		return 0, false
	})
	return t, nil
}

func (t *tabStrip) setTabs(tabs []cliphist.Tab, cur int) {
	t.tabs, t.cur = tabs, cur
	t.cw.Invalidate()
}

func (t *tabStrip) at(x, y int) int {
	for i, r := range t.rects {
		if x >= r.X && x < r.X+r.Width && y >= r.Y && y < r.Y+r.Height {
			return i
		}
	}
	return -1
}

func (t *tabStrip) paint(c *walk.Canvas, _ walk.Rectangle) error {
	r := t.cw.ClientBoundsPixels()
	if err := fill(c, colorWindow, r, 0); err != nil {
		return err
	}
	dpi := t.cw.DPI()
	px := func(dip int) int { return dip * dpi / 96 }
	font := uiFont()
	hf := win.CreateFontIndirect(font.LOGFONTForDPI(dpi))
	m := newMeasurer(hf)
	defer func() {
		m.close()
		win.DeleteObject(win.HGDIOBJ(hf))
	}()

	t.rects = t.rects[:0]
	x := 0
	maxW := px(140)
	for i, tab := range t.tabs {
		w := min(m.longestLine(tab.Name), maxW) + 2*px(10)
		tr := walk.Rectangle{X: x, Y: 0, Width: w, Height: r.Height}
		t.rects = append(t.rects, tr)
		color := colorMuted
		if i == t.cur || i == t.hot {
			color = colorText
		}
		if i == t.hot && i != t.cur {
			fill(c, colorHot, walk.Rectangle{X: tr.X, Y: px(3), Width: tr.Width, Height: r.Height - px(6)}, px(4))
		}
		text := walk.Rectangle{X: tr.X + px(10), Y: 0, Width: tr.Width - 2*px(10), Height: r.Height}
		c.DrawTextPixels(tab.Name, font, color, text, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis|walk.TextNoPrefix)
		if i == t.cur {
			fill(c, colorAccent, walk.Rectangle{X: tr.X + px(10), Y: r.Height - px(3), Width: tr.Width - 2*px(10), Height: px(3)}, px(1))
		}
		x += w + px(2)
	}
	add := walk.Rectangle{X: x, Y: px(3), Width: px(28), Height: r.Height - px(6)}
	t.rects = append(t.rects, add)
	if t.hot == len(t.tabs) {
		fill(c, colorHot, add, px(4))
	}
	if f := iconFont(); f != nil {
		c.DrawTextPixels(glyphAdd, f, colorMuted, add, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
	}
	return nil
}
