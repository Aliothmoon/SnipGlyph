package ui

import (
	"log/slog"
	"sync"
	"unsafe"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"
	"golang.org/x/sys/windows"
)

// Windows 11 light palette.
var (
	colorWindow = walk.RGB(243, 243, 243)
	colorCard   = walk.RGB(255, 255, 255)
	colorBorder = walk.RGB(225, 225, 225)
	colorText   = walk.RGB(26, 26, 26)
	colorMuted  = walk.RGB(112, 112, 112)
	colorError  = walk.RGB(196, 43, 28)
)

const uiFace = "Microsoft YaHei UI"

func newFont(points int, style walk.FontStyle) func() *walk.Font {
	return sync.OnceValue(func() *walk.Font {
		f, err := walk.NewFont(uiFace, points, style)
		if err != nil {
			slog.Warn("font unavailable, using the default", "face", uiFace, "err", err)
			return nil
		}
		return f
	})
}

var (
	uiFont    = newFont(9, 0)  // labels and buttons
	textFont  = newFont(11, 0) // the text being read
	titleFont = newFont(9, walk.FontBold)
)

func setFont(w walk.Window, f *walk.Font) {
	if f != nil {
		w.SetFont(f)
	}
}

// setBackground gives w a solid background. Each call makes a new brush:
// walk disposes a brush once the last window using it is destroyed, so a
// brush cached across popups would be dead by the second one.
func setBackground(w walk.Window, c walk.Color) {
	b, err := walk.NewSolidColorBrush(c)
	if err != nil {
		slog.Warn("create brush", "err", err)
		return
	}
	w.SetBackground(b)
}

// card adds a white panel with a hairline border to parent. Children go in
// inner; outer is what to show, hide or stretch.
func card(parent walk.Container) (outer, inner *walk.Composite, err error) {
	if outer, err = walk.NewComposite(parent); err != nil {
		return nil, nil, err
	}
	// The border is the outer panel showing around the inner one.
	setBackground(outer, colorBorder)
	ol := walk.NewVBoxLayout()
	ol.SetMargins(walk.Margins{HNear: 1, VNear: 1, HFar: 1, VFar: 1})
	ol.SetSpacing(0)
	if err := outer.SetLayout(ol); err != nil {
		return nil, nil, err
	}
	if inner, err = walk.NewComposite(outer); err != nil {
		return nil, nil, err
	}
	setBackground(inner, colorCard)
	il := walk.NewVBoxLayout()
	il.SetMargins(walk.Margins{HNear: 12, VNear: 8, HFar: 8, VFar: 8})
	il.SetSpacing(4)
	return outer, inner, inner.SetLayout(il)
}

// row adds a horizontal strip without margins.
func row(parent walk.Container) (*walk.Composite, error) {
	c, err := walk.NewComposite(parent)
	if err != nil {
		return nil, err
	}
	l := walk.NewHBoxLayout()
	l.SetMargins(walk.Margins{})
	l.SetSpacing(8)
	return c, c.SetLayout(l)
}

func label(parent walk.Container, text string, color walk.Color, font *walk.Font) (*walk.Label, error) {
	l, err := walk.NewLabel(parent)
	if err != nil {
		return nil, err
	}
	setFont(l, font)
	l.SetTextColor(color)
	return l, l.SetText(text)
}

// link adds a hyperlink-style action, lighter than a push button.
func link(parent walk.Container, text string, onClick func()) (*walk.LinkLabel, error) {
	ll, err := walk.NewLinkLabel(parent)
	if err != nil {
		return nil, err
	}
	ll.LinkActivated().Attach(func(*walk.LinkLabelLink) { onClick() })
	return ll, setLinkText(ll, text, true)
}

// setLinkText shows text as a link, or as plain text when active is false
// (for transient states such as "已复制").
func setLinkText(ll *walk.LinkLabel, text string, active bool) error {
	if active {
		text = "<a>" + text + "</a>"
	}
	return ll.SetText(text)
}

// textBox adds a borderless multi-line edit in the reading font. Its scroll
// bar exists but is shown only when the text overflows (see fit).
func textBox(parent walk.Container) (*walk.TextEdit, error) {
	te, err := walk.NewTextEditWithStyle(parent, win.WS_VSCROLL|win.ES_AUTOVSCROLL)
	if err != nil {
		return nil, err
	}
	setFont(te, textFont())
	// Drop the sunken 3D edge: the card already frames the text.
	h := te.Handle()
	ex := win.GetWindowLong(h, win.GWL_EXSTYLE)
	win.SetWindowLong(h, win.GWL_EXSTYLE, ex&^win.WS_EX_CLIENTEDGE)
	win.SetWindowPos(h, 0, 0, 0, 0, 0, win.SWP_FRAMECHANGED|win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOZORDER|win.SWP_NOACTIVATE)
	te.SetTextColor(colorText)
	// Read-only edits paint the dialog grey unless given a background.
	setBackground(te, colorCard)
	return te, nil
}

var procDwmSetWindowAttribute = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")

// tintCaption paints the title bar in the window colour (Windows 11), so
// the frame and the content read as one surface. Older systems ignore it.
func tintCaption(hwnd win.HWND, c walk.Color) {
	const dwmwaCaptionColor = 35
	ref := uint32(c) // walk.Color is already 0x00BBGGRR
	procDwmSetWindowAttribute.Call(uintptr(hwnd), dwmwaCaptionColor, uintptr(unsafe.Pointer(&ref)), 4)
}
