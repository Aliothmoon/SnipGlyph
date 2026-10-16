package ui

import (
	"log/slog"
	"sync"
	"unsafe"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"
)

// Segoe Fluent Icons glyphs, as in Windows 11's own title bars.
const (
	glyphClose  = "\uE8BB"
	glyphPin    = "\uE718"
	glyphPinned = "\uE840"
)

var (
	colorHot       = walk.RGB(233, 233, 233)
	colorPressed   = walk.RGB(222, 222, 222)
	colorDanger    = walk.RGB(196, 43, 28) // Windows 11 close button
	colorDangerHot = walk.RGB(200, 64, 52)
	colorAccent    = walk.RGB(0, 95, 184)
)

var iconFont = sync.OnceValue(func() *walk.Font {
	for _, face := range []string{"Segoe Fluent Icons", "Segoe MDL2 Assets"} {
		if f, err := walk.NewFont(face, 8, 0); err == nil {
			return f
		}
	}
	slog.Warn("no icon font")
	return nil
})

// iconButton is a flat glyph button that lights up under the mouse, red
// for a close button. It takes no keyboard focus.
type iconButton struct {
	cw      *walk.CustomWidget
	glyph   string
	danger  bool // a close button
	on      bool // a toggle that is on: drawn in the accent colour
	hot     bool
	pressed bool
	onClick func()
}

func newIconButton(parent walk.Container, glyph, tip string, danger bool, onClick func()) (*iconButton, error) {
	b := &iconButton{glyph: glyph, danger: danger, onClick: onClick}
	cw, err := walk.NewCustomWidgetPixels(parent, 0, b.paint)
	if err != nil {
		return nil, err
	}
	b.cw = cw
	cw.SetClearsBackground(false)
	cw.SetInvalidatesOnResize(true)
	cw.SetMinMaxSize(walk.Size{Width: 36, Height: 28}, walk.Size{Width: 36, Height: 28})
	cw.SetToolTipText(tip)
	cw.MouseMove().Attach(func(int, int, walk.MouseButton) {
		if !b.hot {
			b.hot = true
			b.trackLeave()
			cw.Invalidate()
		}
	})
	cw.MouseDown().Attach(func(_, _ int, button walk.MouseButton) {
		if button == walk.LeftButton {
			b.pressed = true
			cw.Invalidate()
		}
	})
	cw.MouseUp().Attach(func(x, y int, button walk.MouseButton) {
		if button != walk.LeftButton || !b.pressed {
			return
		}
		b.pressed = false
		s := cw.ClientBoundsPixels()
		inside := x >= 0 && y >= 0 && x < s.Width && y < s.Height
		b.hot = inside // released outside: no leave message comes after a drag
		cw.Invalidate()
		if inside && b.onClick != nil {
			b.onClick()
		}
	})
	hookWindow(cw.Handle(), func(_ win.HWND, msg uint32, _, _ uintptr) (uintptr, bool) {
		if msg == win.WM_MOUSELEAVE {
			b.hot, b.pressed = false, false
			cw.Invalidate()
		}
		return 0, false
	})
	return b, nil
}

// trackLeave asks for WM_MOUSELEAVE, which ends the highlight.
func (b *iconButton) trackLeave() {
	t := win.TRACKMOUSEEVENT{DwFlags: win.TME_LEAVE, HwndTrack: b.cw.Handle()}
	t.CbSize = uint32(unsafe.Sizeof(t))
	win.TrackMouseEvent(&t)
}

// setGlyph changes the icon, and whether it shows as switched on.
func (b *iconButton) setGlyph(glyph string, on bool) {
	b.glyph, b.on = glyph, on
	b.cw.Invalidate()
}

func (b *iconButton) paint(c *walk.Canvas, _ walk.Rectangle) error {
	r := b.cw.ClientBoundsPixels()
	bg, fg := colorWindow, colorText
	switch {
	case b.danger && b.pressed:
		bg, fg = colorDangerHot, walk.RGB(255, 255, 255)
	case b.danger && b.hot:
		bg, fg = colorDanger, walk.RGB(255, 255, 255)
	case b.pressed:
		bg = colorPressed
	case b.hot:
		bg = colorHot
	}
	if b.on {
		fg = colorAccent
	}
	if err := fill(c, colorWindow, r, 0); err != nil {
		return err
	}
	if bg != colorWindow {
		radius := 4 * b.cw.DPI() / 96
		if err := fill(c, bg, r, radius); err != nil {
			return err
		}
	}
	font := iconFont()
	if font == nil {
		font = b.cw.Font()
	}
	return c.DrawTextPixels(b.glyph, font, fg, r, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
}

// fill paints r in colour c, with rounded corners when radius > 0.
func fill(canvas *walk.Canvas, c walk.Color, r walk.Rectangle, radius int) error {
	br, err := walk.NewSolidColorBrush(c)
	if err != nil {
		return err
	}
	defer br.Dispose()
	if radius > 0 {
		return canvas.FillRoundedRectanglePixels(br, r, walk.Size{Width: 2 * radius, Height: 2 * radius})
	}
	return canvas.FillRectanglePixels(br, r)
}
