package ui

import (
	"image"
	"log/slog"
	"unsafe"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"
	"golang.org/x/sys/windows"
)

var procMonitorFromRect = windows.NewLazySystemDLL("user32.dll").NewProc("MonitorFromRect")

// Popup size in device-independent pixels.
const popupW, popupH = 440, 240

// PopupOptions configures a result popup.
type PopupOptions struct {
	Title string
	// Anchor is the screen region (physical pixels) the result belongs to.
	// The popup opens just below it, or above it when there is no room.
	Anchor image.Rectangle
	// OnTranslate, if set, enables the 翻译 button; it receives the text
	// currently in the box. Runs on the UI thread.
	OnTranslate func(text string)
}

// Popup is a small always-on-top window holding editable result text. It
// closes when it loses focus unless pinned. All methods must run on the UI
// thread and are no-ops once the window is gone.
type Popup struct {
	mw     *walk.MainWindow
	text   *walk.TextEdit
	pin    *walk.CheckBox
	copy   *walk.PushButton
	transl *walk.PushButton

	translEnabled bool
}

// NewPopup creates and shows the popup. UI thread only.
func NewPopup(opts PopupOptions) (*Popup, error) {
	mw, err := walk.NewMainWindowWithCfg(&walk.MainWindowCfg{DisableMaximize: true, DisableMinimize: true})
	if err != nil {
		return nil, err
	}
	p := &Popup{mw: mw}
	ok := false
	defer func() {
		if !ok {
			mw.Dispose()
		}
	}()
	mw.SetExitOnClose(false) // walk would otherwise end the app with this window
	mw.SetPersistent(false)
	win.SetMenu(mw.Handle(), 0) // drop walk's empty menu bar
	// walk's layout ignores an empty toolbar, but the toolbar window still
	// paints over the top of the content.
	mw.ToolBar().SetVisible(false)
	mw.SetIcon(appIcon())
	if err := mw.SetTitle(opts.Title); err != nil {
		return nil, err
	}
	layout := walk.NewVBoxLayout()
	layout.SetMargins(walk.Margins{HNear: 8, VNear: 8, HFar: 8, VFar: 8})
	layout.SetSpacing(6)
	if err := mw.SetLayout(layout); err != nil {
		return nil, err
	}

	if p.text, err = walk.NewTextEditWithStyle(mw, win.WS_VSCROLL); err != nil {
		return nil, err
	}
	p.text.KeyDown().Attach(func(key walk.Key) {
		if key == walk.KeyEscape {
			mw.Close()
		}
	})

	bar, err := walk.NewComposite(mw)
	if err != nil {
		return nil, err
	}
	barLayout := walk.NewHBoxLayout()
	barLayout.SetMargins(walk.Margins{})
	if err := bar.SetLayout(barLayout); err != nil {
		return nil, err
	}
	if p.pin, err = walk.NewCheckBox(bar); err != nil {
		return nil, err
	}
	p.pin.SetText("钉住")
	if _, err := walk.NewHSpacer(bar); err != nil {
		return nil, err
	}
	if p.transl, err = walk.NewPushButton(bar); err != nil {
		return nil, err
	}
	p.transl.SetText("翻译")
	p.transl.SetEnabled(false)
	if opts.OnTranslate != nil {
		p.transl.Clicked().Attach(func() { opts.OnTranslate(p.text.Text()) })
	}
	if p.copy, err = walk.NewPushButton(bar); err != nil {
		return nil, err
	}
	p.copy.SetText("复制")
	p.copy.SetEnabled(false)
	p.copy.Clicked().Attach(func() {
		if err := walk.Clipboard().SetText(p.text.Text()); err != nil {
			slog.Warn("copy to clipboard", "err", err)
			return
		}
		p.copy.SetText("已复制")
	})
	p.translEnabled = opts.OnTranslate != nil

	mw.Deactivating().Attach(func() {
		if !p.pin.Checked() {
			mw.Close()
		}
	})
	mw.Closing().Attach(func(*bool, walk.CloseReason) {
		// Dispose after the close finishes; disposing inside Closing is unsafe.
		walk.App().Synchronize(mw.Dispose)
	})

	// Tool window: no taskbar button. Set while hidden so it takes effect.
	h := mw.Handle()
	ex := win.GetWindowLong(h, win.GWL_EXSTYLE)
	win.SetWindowLong(h, win.GWL_EXSTYLE, ex|win.WS_EX_TOOLWINDOW)
	if err := mw.SetBoundsPixels(placement(opts.Anchor)); err != nil {
		return nil, err
	}
	win.SetWindowPos(h, win.HWND_TOPMOST, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE)
	mw.Show()
	win.SetForegroundWindow(h)
	ok = true
	return p, nil
}

// SetBusy shows a placeholder while work is in progress.
func (p *Popup) SetBusy(msg string) {
	if p.gone() {
		return
	}
	p.text.SetText(msg)
	p.text.SetReadOnly(true)
	p.copy.SetEnabled(false)
	p.transl.SetEnabled(false)
}

// SetText shows a result the user can edit, copy and translate.
func (p *Popup) SetText(text string) {
	if p.gone() {
		return
	}
	p.text.SetReadOnly(false)
	p.text.SetText(crlf(text))
	p.copy.SetText("复制")
	p.copy.SetEnabled(text != "")
	p.transl.SetEnabled(text != "" && p.translEnabled)
	p.text.SetFocus()
}

// SetError shows a failure in place of the result.
func (p *Popup) SetError(msg string) {
	if p.gone() {
		return
	}
	p.text.SetReadOnly(true)
	p.text.SetText(crlf(msg))
	p.copy.SetEnabled(false)
	p.transl.SetEnabled(false)
}

func (p *Popup) gone() bool { return p.mw.IsDisposed() }

// crlf converts line breaks for the Win32 EDIT control.
func crlf(s string) string {
	out := make([]byte, 0, len(s)+16)
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' && (i == 0 || s[i-1] != '\r') {
			out = append(out, '\r')
		}
		out = append(out, s[i])
	}
	return string(out)
}

// placement returns popup bounds in physical pixels: below anchor, or above
// it when the monitor's work area has no room below, clamped to the area.
func placement(anchor image.Rectangle) walk.Rectangle {
	ar := win.RECT{Left: int32(anchor.Min.X), Top: int32(anchor.Min.Y), Right: int32(anchor.Max.X), Bottom: int32(anchor.Max.Y)}
	const monitorDefaultToNearest = 2
	hmon, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&ar)), monitorDefaultToNearest)
	mi := win.MONITORINFO{}
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	win.GetMonitorInfo(win.HMONITOR(hmon), &mi)
	work := image.Rect(int(mi.RcWork.Left), int(mi.RcWork.Top), int(mi.RcWork.Right), int(mi.RcWork.Bottom))
	var dpiX, dpiY uint32 = 96, 96
	win.GetDpiForMonitor(win.HMONITOR(hmon), win.MDT_EFFECTIVE_DPI, &dpiX, &dpiY)
	return placeIn(anchor, work, popupW*int(dpiX)/96, popupH*int(dpiY)/96, 8*int(dpiY)/96)
}

func placeIn(anchor, work image.Rectangle, w, h, gap int) walk.Rectangle {
	x := anchor.Min.X
	y := anchor.Max.Y + gap
	if y+h > work.Max.Y {
		y = anchor.Min.Y - gap - h
	}
	x = min(max(x, work.Min.X), work.Max.X-w)
	y = min(max(y, work.Min.Y), work.Max.Y-h)
	return walk.Rectangle{X: x, Y: y, Width: w, Height: h}
}
