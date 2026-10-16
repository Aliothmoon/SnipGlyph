package ui

import (
	"image"
	"strings"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"
	"golang.org/x/sys/windows"
)

// Sizing limits for fitting a popup to its text, in DIPs or fractions of
// the monitor's work area.
const (
	fitMinW         = 400
	fitMaxW         = 760
	fitSourceMaxH   = 0.35
	fitResultMaxH   = 0.40
	fitMinLines     = 2
	fitMinLinesNone = 3 // an empty box, waiting for input
	// Horizontal space around a text box inside the client area: window
	// margins 12+12, card border 1+1, card padding 12+8.
	fitCardChromeX = 46
)

// anchoring says what stays put when a popup is refitted.
type anchoring int

const (
	centreInWork anchoring = iota // first fit: centre in the work area
	keepCentre                    // new content: grow in place
	keepTop                       // typing: grow downwards, the caret stays put
)

// fit sizes the popup to its text: each text box as tall as its wrapped
// text within limits (then it scrolls), the window as wide as the longest
// line within limits and as tall as the layout needs.
func (p *Popup) fit(work image.Rectangle, how anchoring) {
	h := p.mw.Handle()
	dpi := int(win.GetDpiForWindow(h))
	px := func(dip int) int { return dip * dpi / 96 }

	var wr, cr win.RECT
	win.GetWindowRect(h, &wr)
	win.GetClientRect(h, &cr)
	frameX := int(wr.Right-wr.Left) - int(cr.Right-cr.Left)
	frameY := int(wr.Bottom-wr.Top) - int(cr.Bottom-cr.Top)
	// Text boxes only show a scroll bar when their text does not fit, and
	// then their height is capped anyway, so no room is kept for one.
	margins := p.source.SendMessage(win.EM_GETMARGINS, 0, 0)
	editChromeX := int(margins&0xFFFF) + int(margins>>16&0xFFFF) + px(4)
	chromeX := frameX + px(fitCardChromeX) + editChromeX

	m := newMeasurer(win.HFONT(p.source.SendMessage(win.WM_GETFONT, 0, 0)))
	defer m.close()
	boxes := []*walk.TextEdit{p.source}
	if p.expanded {
		boxes = append(boxes, p.result)
	}
	textW := 0
	for _, b := range boxes {
		textW = max(textW, m.longestLine(b.Text()))
	}

	minW := px(fitMinW)
	if li, ok := walk.CreateLayoutItemsForContainer(p.mw).(walk.MinSizeForSizer); ok {
		minW = max(minW, li.MinSizeForSize(walk.Size{Width: minW}).Width+frameX)
	}
	w := min(max(textW+chromeX, minW), px(fitMaxW), work.Dx())
	area := w - chromeX

	p.setBoxHeight(p.source, m, area, int(fitSourceMaxH*float64(work.Dy())))
	if p.expanded {
		p.setBoxHeight(p.result, m, area, int(fitResultMaxH*float64(work.Dy())))
	}

	height := 0
	if li, ok := walk.CreateLayoutItemsForContainer(p.mw).(walk.MinSizeForSizer); ok {
		height = li.MinSizeForSize(walk.Size{Width: w}).Height + frameY
	}
	height = min(height, work.Dy())
	b := p.mw.BoundsPixels()
	switch how {
	case centreInWork:
		b = centerIn(work, w, height)
	case keepCentre:
		b = resizeAround(b, w, height, work)
	case keepTop:
		b = growDown(b, w, height, work)
	}
	if b != p.mw.BoundsPixels() {
		p.mw.SetBoundsPixels(b)
	}
}

// setBoxHeight fixes box's height to its text wrapped at width, between a
// few lines and maxH, with a scroll bar only when the text does not fit.
func (p *Popup) setBoxHeight(box *walk.TextEdit, m *measurer, width, maxH int) {
	lines := fitMinLines
	if box.TextLength() == 0 {
		lines = fitMinLinesNone
	}
	// The EDIT control keeps a pixel above and below its formatting rect.
	const pad = 4
	textH := m.height(box.Text(), width) + pad
	h := max(textH, lines*m.lineH+pad)
	h = min(h, max(maxH, fitMinLines*m.lineH+pad))
	box.SetMinMaxSizePixels(walk.Size{Height: h}, walk.Size{Height: h})
	showVScroll(box, textH > h)
}

var procShowScrollBar = windows.NewLazySystemDLL("user32.dll").NewProc("ShowScrollBar")

func showVScroll(box *walk.TextEdit, show bool) {
	var b uintptr
	if show {
		b = 1
	}
	procShowScrollBar.Call(uintptr(box.Handle()), win.SB_VERT, b)
}

// resizeAround gives b the size w x h around its current centre, then moves
// it back inside work if it spills out.
func resizeAround(b walk.Rectangle, w, h int, work image.Rectangle) walk.Rectangle {
	w, h = min(w, work.Dx()), min(h, work.Dy())
	b.X += (b.Width - w) / 2
	b.Y += (b.Height - h) / 2
	b.Width, b.Height = w, h
	b.X = min(max(b.X, work.Min.X), work.Max.X-w)
	b.Y = min(max(b.Y, work.Min.Y), work.Max.Y-h)
	return b
}

// growDown gives b the size w x h keeping its top edge and horizontal
// centre, then moves it back inside work if it spills out.
func growDown(b walk.Rectangle, w, h int, work image.Rectangle) walk.Rectangle {
	w, h = min(w, work.Dx()), min(h, work.Dy())
	b.X += (b.Width - w) / 2
	b.Width, b.Height = w, h
	b.X = min(max(b.X, work.Min.X), work.Max.X-w)
	b.Y = min(max(b.Y, work.Min.Y), work.Max.Y-h)
	return b
}

// measurer lays out text the way the EDIT control will, with its font.
type measurer struct {
	dc    win.HDC
	old   win.HGDIOBJ
	lineH int
}

func newMeasurer(font win.HFONT) *measurer {
	m := &measurer{dc: win.CreateCompatibleDC(0)}
	m.old = win.SelectObject(m.dc, win.HGDIOBJ(font))
	var tm win.TEXTMETRIC
	win.GetTextMetrics(m.dc, &tm)
	m.lineH = int(tm.TmHeight)
	return m
}

func (m *measurer) close() {
	win.SelectObject(m.dc, m.old)
	win.DeleteDC(m.dc)
}

func (m *measurer) calc(text string, flags uint32, r *win.RECT) {
	if text == "" {
		return
	}
	t, err := windows.UTF16FromString(text)
	if err != nil { // embedded NUL
		return
	}
	win.DrawTextEx(m.dc, &t[0], int32(len(t)-1), r, flags|win.DT_CALCRECT|win.DT_NOPREFIX|win.DT_EXPANDTABS, nil)
}

// longestLine is the width of the widest line, unwrapped.
func (m *measurer) longestLine(text string) int {
	w := 0
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		var r win.RECT
		m.calc(line, win.DT_SINGLELINE, &r)
		w = max(w, int(r.Right-r.Left))
	}
	return w
}

// height is the height of text wrapped at width.
func (m *measurer) height(text string, width int) int {
	r := win.RECT{Right: int32(width)}
	m.calc(text, win.DT_WORDBREAK|win.DT_EDITCONTROL, &r)
	return int(r.Bottom - r.Top)
}
