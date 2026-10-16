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

// Popup sizes in device-independent pixels: source only, and with the
// translation pane.
const (
	popupW         = 460
	popupH         = 240
	popupExpandedH = 420
)

// PopupOptions configures a result popup.
type PopupOptions struct {
	Title string
	// Anchor is the screen region (physical pixels) the popup belongs to. It
	// opens just below it, or above when there is no room below.
	Anchor image.Rectangle
	// Translation opens the popup with the translation pane showing.
	Translation bool
	// Engines names the translation engines; with none, translating is off.
	Engines []string
	Engine  int // index of the initially selected engine
	// OnTranslate runs on the UI thread when the user asks for a translation:
	// the 翻译 button, Ctrl+Enter in the source box, or switching engine.
	OnTranslate func(text string, engine int)
	// OnClose runs on the UI thread when the window closes.
	OnClose func()
}

// Popup is a small always-on-top window with editable source text and an
// optional translation pane. It closes when it loses focus unless pinned.
// All methods must run on the UI thread and are no-ops once it is closed.
type Popup struct {
	opts PopupOptions
	mw   *walk.MainWindow

	source   *walk.TextEdit
	trBar    *walk.Composite
	dir      *walk.Label
	engine   *walk.ComboBox
	result   *walk.TextEdit
	pin      *walk.CheckBox
	translBt *walk.PushButton
	copySrc  *walk.PushButton
	copyRes  *walk.PushButton
	expanded bool
	closing  bool
}

// NewPopup creates and shows the popup. UI thread only.
func NewPopup(opts PopupOptions) (*Popup, error) {
	mw, err := walk.NewMainWindowWithCfg(&walk.MainWindowCfg{DisableMaximize: true, DisableMinimize: true})
	if err != nil {
		return nil, err
	}
	p := &Popup{opts: opts, mw: mw}
	ok := false
	defer func() {
		if !ok {
			mw.Dispose()
		}
	}()
	if err := p.build(); err != nil {
		return nil, err
	}

	h := mw.Handle()
	// Ctrl+Enter translates. Catch it before TranslateMessage, or the edit
	// control would also insert a line break.
	walk.App().AddPreTranslateHandlerForHWND(h, ctrlEnter{p})
	mw.Deactivating().Attach(func() {
		// Close later, never from inside the activation change: walk disposes
		// a form while handling WM_CLOSE, destroying it deactivates it, and a
		// second Close re-entering Dispose deadlocks the UI thread.
		walk.App().Synchronize(func() {
			if !p.closing && !p.gone() && !p.pin.Checked() {
				mw.Close()
			}
		})
	})
	mw.Closing().Attach(func(*bool, walk.CloseReason) {
		// walk disposes the form right after this event.
		p.closing = true
		walk.App().DeletePreTranslateHandlerForHWND(h)
		if opts.OnClose != nil {
			opts.OnClose()
		}
	})

	// Tool window: no taskbar button. Set while hidden so it takes effect.
	ex := win.GetWindowLong(h, win.GWL_EXSTYLE)
	win.SetWindowLong(h, win.GWL_EXSTYLE, ex|win.WS_EX_TOOLWINDOW)
	height := popupH
	if opts.Translation {
		height = popupExpandedH
		p.setExpanded()
	}
	if err := mw.SetBoundsPixels(placement(opts.Anchor, height)); err != nil {
		return nil, err
	}
	win.SetWindowPos(h, win.HWND_TOPMOST, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE)
	mw.Show()
	win.SetForegroundWindow(h)
	p.source.SetFocus()
	ok = true
	return p, nil
}

func (p *Popup) build() error {
	mw := p.mw
	mw.SetExitOnClose(false) // walk would otherwise end the app with this window
	mw.SetPersistent(false)
	win.SetMenu(mw.Handle(), 0) // drop walk's empty menu bar
	// walk's layout ignores an empty toolbar, but the toolbar window still
	// paints over the top of the content.
	setShown(mw.ToolBar(), false)
	mw.SetIcon(appIcon())
	if err := mw.SetTitle(p.opts.Title); err != nil {
		return err
	}
	layout := walk.NewVBoxLayout()
	layout.SetMargins(walk.Margins{HNear: 8, VNear: 8, HFar: 8, VFar: 8})
	layout.SetSpacing(6)
	if err := mw.SetLayout(layout); err != nil {
		return err
	}

	var err error
	if p.source, err = walk.NewTextEditWithStyle(mw, win.WS_VSCROLL); err != nil {
		return err
	}
	p.source.KeyDown().Attach(p.onKey)

	if p.trBar, err = hbox(mw); err != nil {
		return err
	}
	if p.dir, err = walk.NewLabel(p.trBar); err != nil {
		return err
	}
	if _, err := walk.NewHSpacer(p.trBar); err != nil {
		return err
	}
	if p.engine, err = walk.NewDropDownBox(p.trBar); err != nil {
		return err
	}
	if err := p.engine.SetModel(p.opts.Engines); err != nil {
		return err
	}
	if len(p.opts.Engines) > 0 {
		p.engine.SetCurrentIndex(p.opts.Engine)
	}
	p.engine.CurrentIndexChanged().Attach(func() {
		if p.expanded && p.Source() != "" {
			p.requestTranslation()
		}
	})

	if p.result, err = walk.NewTextEditWithStyle(mw, win.WS_VSCROLL); err != nil {
		return err
	}
	p.result.SetReadOnly(true)
	p.result.KeyDown().Attach(p.onKey)

	bar, err := hbox(mw)
	if err != nil {
		return err
	}
	if p.pin, err = walk.NewCheckBox(bar); err != nil {
		return err
	}
	p.pin.SetText("钉住")
	if _, err := walk.NewHSpacer(bar); err != nil {
		return err
	}
	if p.translBt, err = button(bar, "翻译", p.requestTranslation); err != nil {
		return err
	}
	if p.copySrc, err = button(bar, "复制", func() { p.copyText(p.copySrc, p.source) }); err != nil {
		return err
	}
	if p.copyRes, err = button(bar, "复制译文", func() { p.copyText(p.copyRes, p.result) }); err != nil {
		return err
	}
	p.translBt.SetToolTipText("Ctrl+Enter")
	p.translBt.SetEnabled(false)
	p.copySrc.SetEnabled(false)
	p.copyRes.SetEnabled(false)
	// Typing into the box (input translation, edits to OCR text) updates
	// what can be copied or translated.
	p.source.TextChanged().Attach(func() {
		if p.source.ReadOnly() {
			return
		}
		has := p.source.TextLength() > 0
		p.copySrc.SetText("复制")
		p.copySrc.SetEnabled(has)
		p.translBt.SetEnabled(has && len(p.opts.Engines) > 0)
	})

	setShown(p.trBar, false)
	setShown(p.result, false)
	setShown(p.copyRes, false)
	return nil
}

func hbox(parent walk.Container) (*walk.Composite, error) {
	c, err := walk.NewComposite(parent)
	if err != nil {
		return nil, err
	}
	l := walk.NewHBoxLayout()
	l.SetMargins(walk.Margins{})
	return c, c.SetLayout(l)
}

func button(parent walk.Container, text string, onClick func()) (*walk.PushButton, error) {
	b, err := walk.NewPushButton(parent)
	if err != nil {
		return nil, err
	}
	b.SetText(text)
	b.Clicked().Attach(onClick)
	return b, nil
}

func (p *Popup) onKey(key walk.Key) {
	if key == walk.KeyEscape {
		p.mw.Close()
	}
}

type ctrlEnter struct{ p *Popup }

func (c ctrlEnter) OnPreTranslate(msg *win.MSG) bool {
	if msg.Message != win.WM_KEYDOWN || msg.WParam != win.VK_RETURN || walk.ModifiersDown() != walk.ModControl {
		return false
	}
	if c.p.gone() || msg.HWnd != c.p.source.Handle() {
		return false
	}
	c.p.requestTranslation()
	return true
}

func (p *Popup) requestTranslation() {
	if len(p.opts.Engines) == 0 || p.opts.OnTranslate == nil || p.source.ReadOnly() {
		return
	}
	text := p.Source()
	if text == "" {
		return
	}
	p.opts.OnTranslate(text, p.engine.CurrentIndex())
}

func (p *Popup) copyText(btn *walk.PushButton, from *walk.TextEdit) {
	if err := walk.Clipboard().SetText(from.Text()); err != nil {
		slog.Warn("copy to clipboard", "err", err)
		return
	}
	btn.SetText("已复制")
}

// Source is the text in the source box, as edited by the user.
func (p *Popup) Source() string {
	if p.gone() {
		return ""
	}
	return lf(p.source.Text())
}

// SetSourceBusy shows a placeholder while the source text is being produced.
func (p *Popup) SetSourceBusy(msg string) {
	if p.gone() {
		return
	}
	p.source.SetText(msg)
	p.source.SetReadOnly(true)
	p.copySrc.SetEnabled(false)
	p.translBt.SetEnabled(false)
}

// SetSource shows text the user can edit, copy and translate.
func (p *Popup) SetSource(text string) {
	if p.gone() {
		return
	}
	p.source.SetReadOnly(false)
	p.source.SetText(crlf(text))
	p.copySrc.SetText("复制")
	p.copySrc.SetEnabled(text != "")
	p.translBt.SetEnabled(text != "" && len(p.opts.Engines) > 0)
	p.source.SetFocus()
}

// SetSourceError shows a failure in place of the source text.
func (p *Popup) SetSourceError(msg string) {
	if p.gone() {
		return
	}
	p.source.SetReadOnly(true)
	p.source.SetText(crlf(msg))
	p.copySrc.SetEnabled(false)
	p.translBt.SetEnabled(false)
}

// SetHint shows a line of guidance above the (empty) translation pane.
func (p *Popup) SetHint(text string) {
	if p.gone() {
		return
	}
	p.dir.SetText(text)
}

// ShowTranslating opens the translation pane with a placeholder.
func (p *Popup) ShowTranslating(direction string) {
	if p.gone() {
		return
	}
	p.expand()
	p.dir.SetText(direction)
	p.result.SetText("翻译中…")
	p.copyRes.SetEnabled(false)
}

// SetTranslation shows (possibly partial) translated text.
func (p *Popup) SetTranslation(direction, text string) {
	if p.gone() {
		return
	}
	p.expand()
	if direction != "" {
		p.dir.SetText(direction)
	}
	p.result.SetText(crlf(text))
	p.copyRes.SetText("复制译文")
	p.copyRes.SetEnabled(text != "")
}

// SetTranslationError shows a failure in the translation pane.
func (p *Popup) SetTranslationError(msg string) {
	if p.gone() {
		return
	}
	p.expand()
	p.result.SetText(crlf(msg))
	p.copyRes.SetEnabled(false)
}

func (p *Popup) gone() bool { return p.closing || p.mw.IsDisposed() }

func (p *Popup) setExpanded() {
	p.expanded = true
	setShown(p.trBar, true)
	setShown(p.result, true)
	setShown(p.copyRes, true)
}

// expand shows the translation pane, growing the window downwards (or
// upwards when the work area ends) the first time.
func (p *Popup) expand() {
	if p.expanded {
		return
	}
	p.setExpanded()
	h := p.mw.Handle()
	b := p.mw.BoundsPixels()
	dpi := int(win.GetDpiForWindow(h))
	b.Height = popupExpandedH * dpi / 96
	mi := win.MONITORINFO{}
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	win.GetMonitorInfo(win.MonitorFromWindow(h, win.MONITOR_DEFAULTTONEAREST), &mi)
	if bottom := int(mi.RcWork.Bottom); b.Y+b.Height > bottom {
		b.Y = max(int(mi.RcWork.Top), bottom-b.Height)
	}
	p.mw.SetBoundsPixels(b)
}

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

// lf undoes crlf.
func lf(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n' {
			continue
		}
		out = append(out, s[i])
	}
	return string(out)
}

// placement returns popup bounds in physical pixels for a popup of height
// h (DIPs): below anchor, or above it when the monitor's work area has no
// room below, clamped to the work area.
func placement(anchor image.Rectangle, h int) walk.Rectangle {
	ar := win.RECT{Left: int32(anchor.Min.X), Top: int32(anchor.Min.Y), Right: int32(anchor.Max.X), Bottom: int32(anchor.Max.Y)}
	hmon, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&ar)), win.MONITOR_DEFAULTTONEAREST)
	mi := win.MONITORINFO{}
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	win.GetMonitorInfo(win.HMONITOR(hmon), &mi)
	work := image.Rect(int(mi.RcWork.Left), int(mi.RcWork.Top), int(mi.RcWork.Right), int(mi.RcWork.Bottom))
	var dpiX, dpiY uint32 = 96, 96
	win.GetDpiForMonitor(win.HMONITOR(hmon), win.MDT_EFFECTIVE_DPI, &dpiX, &dpiY)
	return placeIn(anchor, work, popupW*int(dpiX)/96, h*int(dpiY)/96, 8*int(dpiY)/96)
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

// setShown shows or hides w. Unlike walk's SetVisible it also works before
// the form is first shown: SetVisible compares against IsWindowVisible,
// which reports false for every child of a hidden window, so hiding a child
// early silently does nothing. walk picks up the change through
// WM_WINDOWPOSCHANGED and re-lays out.
func setShown(w walk.Window, shown bool) {
	cmd := int32(win.SW_HIDE)
	if shown {
		cmd = win.SW_SHOWNA
	}
	win.ShowWindow(w.Handle(), cmd)
}
