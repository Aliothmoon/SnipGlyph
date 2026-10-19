package ui

import (
	"context"
	"image"
	"log/slog"
	"time"
	"unsafe"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"
	"golang.org/x/sys/windows"

	"SnipGlyph/internal/winclip"
)

var procMonitorFromRect = windows.NewLazySystemDLL("user32.dll").NewProc("MonitorFromRect")

const defaultHint = "Ctrl+Enter 翻译 · Esc 关闭"

// PopupOptions configures a result popup.
type PopupOptions struct {
	Title string
	// Anchor is the screen region (physical pixels) the popup belongs to; it
	// opens centred on that region's monitor.
	Anchor image.Rectangle
	// Engines names the translation engines; with none, translating is off.
	Engines []string
	Engine  int // index of the initially selected engine
	// OnTranslate runs on the UI thread when the user asks for a translation:
	// the 翻译 button, Ctrl+Enter in the source box, or switching engine.
	OnTranslate func(text string, engine int)
	// OnClose runs on the UI thread when the window closes.
	OnClose func()
}

// Popup is an always-on-top window with two cards: editable source text,
// and a translation pane that opens on the first translation. It sizes
// itself to its text and closes when it loses focus unless pinned. All
// methods must run on the UI thread and are no-ops once it is closed.
type Popup struct {
	opts PopupOptions
	mw   *walk.MainWindow

	source  *walk.TextEdit
	copySrc *walk.LinkLabel

	trCard  *walk.Composite // the whole translation pane
	dir     *walk.Label
	engine  *walk.ComboBox
	copyRes *walk.LinkLabel
	result  *walk.TextEdit

	pin      *iconButton
	hint     *walk.Label
	translBt *walk.PushButton

	pinned      bool // stays open when it loses focus
	expanded    bool
	closing     bool
	settingText bool // the source text is being set by code, not typed
}

// NewPopup creates and shows the popup. UI thread only.
func NewPopup(opts PopupOptions) (*Popup, error) {
	mw, err := newWindow(opts.Title, &walk.MainWindowCfg{DisableMaximize: true, DisableMinimize: true, DisableResizing: true})
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
	walk.App().AddPreTranslateHandlerForHWND(h, popupKeys{p})
	mw.Deactivating().Attach(func() {
		// Close later, never from inside the activation change: walk disposes
		// a form while handling WM_CLOSE, destroying it deactivates it, and a
		// second Close re-entering Dispose deadlocks the UI thread.
		walk.App().Synchronize(func() {
			// It may have been activated again in the meantime.
			if !p.closing && !p.gone() && !p.pinned && win.GetForegroundWindow() != h {
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
	frameless(mw) // the header row is the title bar
	// Move onto the target monitor first, so the window adopts its DPI.
	work, dpi := monitorOf(opts.Anchor)
	if err := mw.SetBoundsPixels(centerIn(work, fitMinW*dpi/96, 200*dpi/96)); err != nil {
		return nil, err
	}
	win.SetWindowPos(h, win.HWND_TOPMOST, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE)
	// Size it to the content while shown but cloaked: walk measures only
	// visible children.
	cloak(h, true)
	mw.Show()
	p.fit(work, centreInWork)
	cloak(h, false)
	win.SetForegroundWindow(h)
	p.source.SetFocus()
	ok = true
	return p, nil
}

func (p *Popup) build() error {
	mw := p.mw
	outer := walk.NewVBoxLayout()
	outer.SetMargins(walk.Margins{})
	outer.SetSpacing(0)
	if err := mw.SetLayout(outer); err != nil {
		return err
	}

	// Header: the title bar, drawn by us.
	bar, err := row(mw)
	if err != nil {
		return err
	}
	bar.Layout().SetMargins(walk.Margins{HNear: 12, VNear: 6, HFar: 6, VFar: 2})
	bar.Layout().SetSpacing(2)
	title, err := label(bar, p.opts.Title, colorMuted, nil)
	if err != nil {
		return err
	}
	if _, err := walk.NewHSpacer(bar); err != nil {
		return err
	}
	if p.pin, err = newIconButton(bar, glyphPin, "钉住：点别处不关闭", false, func() { p.setPinned(!p.pinned) }); err != nil {
		return err
	}
	if _, err := newIconButton(bar, glyphClose, "关闭（Esc）", true, func() { p.mw.Close() }); err != nil {
		return err
	}
	dragBy(mw, bar)
	dragBy(mw, title)

	body, err := walk.NewComposite(mw)
	if err != nil {
		return err
	}
	layout := walk.NewVBoxLayout()
	layout.SetMargins(walk.Margins{HNear: 12, VNear: 2, HFar: 12, VFar: 10})
	layout.SetSpacing(10)
	if err := body.SetLayout(layout); err != nil {
		return err
	}

	// Source card.
	srcCard, src, err := card(body)
	if err != nil {
		return err
	}
	head, err := row(src)
	if err != nil {
		return err
	}
	if _, err := label(head, "原文", colorMuted, titleFont()); err != nil {
		return err
	}
	if _, err := walk.NewHSpacer(head); err != nil {
		return err
	}
	if p.copySrc, err = link(head, "复制", func() { p.copyText(p.copySrc, p.source) }); err != nil {
		return err
	}
	if p.source, err = textBox(src); err != nil {
		return err
	}
	// Typing (input translation, fixing OCR text) updates what can be copied
	// or translated, and grows the box with the text.
	p.source.TextChanged().Attach(func() {
		if p.source.ReadOnly() || p.settingText {
			return
		}
		has := p.source.TextLength() > 0
		setLinkText(p.copySrc, "复制", true)
		p.copySrc.SetEnabled(has)
		p.translBt.SetEnabled(has && len(p.opts.Engines) > 0)
		p.refitAnchored(keepTop)
	})

	// Translation card.
	var tr *walk.Composite
	if p.trCard, tr, err = card(body); err != nil {
		return err
	}
	if head, err = row(tr); err != nil {
		return err
	}
	if p.dir, err = label(head, "译文", colorMuted, titleFont()); err != nil {
		return err
	}
	if _, err := walk.NewHSpacer(head); err != nil {
		return err
	}
	if p.engine, err = walk.NewDropDownBox(head); err != nil {
		return err
	}
	if err := p.engine.SetModel(p.opts.Engines); err != nil {
		return err
	}
	if len(p.opts.Engines) > 0 {
		p.engine.SetCurrentIndex(p.opts.Engine)
	} else {
		setShown(p.engine, false) // the pane explains how to add one
	}
	p.engine.SetToolTipText("翻译引擎")
	p.engine.CurrentIndexChanged().Attach(func() {
		if p.expanded && p.Source() != "" {
			p.requestTranslation()
		}
	})
	if p.copyRes, err = link(head, "复制", func() { p.copyText(p.copyRes, p.result) }); err != nil {
		return err
	}
	if p.result, err = textBox(tr); err != nil {
		return err
	}
	p.result.SetReadOnly(true)

	// Both cards share the height.
	layout.SetStretchFactor(srcCard, 1)
	layout.SetStretchFactor(p.trCard, 1)

	// Footer.
	foot, err := row(body)
	if err != nil {
		return err
	}
	if p.hint, err = label(foot, defaultHint, colorMuted, nil); err != nil {
		return err
	}
	if _, err := walk.NewHSpacer(foot); err != nil {
		return err
	}
	if p.translBt, err = walk.NewPushButton(foot); err != nil {
		return err
	}
	p.translBt.SetText("翻译")
	p.translBt.SetMinMaxSize(walk.Size{Width: 88}, walk.Size{})
	p.translBt.Clicked().Attach(p.requestTranslation)
	// The accent ring of a default button marks the main action.
	p.translBt.SendMessage(win.BM_SETSTYLE, win.BS_DEFPUSHBUTTON, 1)

	p.translBt.SetEnabled(false)
	p.copySrc.SetEnabled(false)
	p.copyRes.SetEnabled(false)
	setShown(p.trCard, false) // opens on the first translation
	return nil
}

// setPinned keeps the popup open when it loses focus, or lets it close.
func (p *Popup) setPinned(on bool) {
	p.pinned = on
	if on {
		p.pin.setGlyph(glyphPinned, true)
		p.pin.cw.SetToolTipText("取消钉住")
	} else {
		p.pin.setGlyph(glyphPin, false)
		p.pin.cw.SetToolTipText("钉住：点别处不关闭")
	}
}

// popupKeys handles the popup's keys wherever the focus is: Esc closes it,
// Ctrl+Enter in the source box translates. Ctrl+Enter is caught before
// TranslateMessage, or the edit control would also insert a line break.
type popupKeys struct{ p *Popup }

func (k popupKeys) OnPreTranslate(msg *win.MSG) bool {
	p := k.p
	if p.gone() || msg.Message != win.WM_KEYDOWN {
		return false
	}
	h := p.mw.Handle()
	if msg.HWnd != h && !win.IsChild(h, msg.HWnd) {
		return false
	}
	mods := walk.ModifiersDown()
	switch {
	case msg.WParam == win.VK_RETURN && mods == walk.ModControl && msg.HWnd == p.source.Handle():
		p.requestTranslation()
		return true
	case msg.WParam == win.VK_ESCAPE && mods == 0:
		// An open drop-down list takes Esc to close itself.
		if win.SendMessage(msg.HWnd, win.CB_GETDROPPEDSTATE, 0, 0) != 0 {
			return false
		}
		walk.App().Synchronize(func() {
			if !p.gone() {
				p.mw.Close()
			}
		})
		return true
	}
	return false
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

// copyToClipboard replaces the clipboard content with text and shows how it
// went on ll. The clipboard may be held by another program for a while, so
// the waiting happens off the UI thread. UI thread.
func copyToClipboard(ll *walk.LinkLabel, text string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := winclip.SetText(ctx, text, winclip.WriteOptions{})
		cancel()
		walk.App().Synchronize(func() {
			if ll.IsDisposed() {
				return
			}
			if err != nil {
				slog.Warn("copy to clipboard", "err", err)
				setLinkText(ll, "复制失败，重试", true)
				return
			}
			setLinkText(ll, "已复制", false)
		})
	}()
}

func (p *Popup) copyText(ll *walk.LinkLabel, from *walk.TextEdit) {
	copyToClipboard(ll, lf(from.Text()))
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
	p.source.SetReadOnly(true)
	p.source.SetTextColor(colorMuted)
	p.source.SetText(msg)
	p.copySrc.SetEnabled(false)
	p.translBt.SetEnabled(false)
	p.refit()
}

// SetSource shows text the user can edit, copy and translate.
func (p *Popup) SetSource(text string) {
	if p.gone() {
		return
	}
	p.source.SetReadOnly(false)
	p.source.SetTextColor(colorText)
	p.settingText = true
	p.source.SetText(crlf(text))
	p.settingText = false
	setLinkText(p.copySrc, "复制", true)
	p.copySrc.SetEnabled(text != "")
	p.translBt.SetEnabled(text != "" && len(p.opts.Engines) > 0)
	p.refit()
	p.source.SetFocus()
}

// SelectSource selects all source text, so typing replaces it.
func (p *Popup) SelectSource() {
	if p.gone() {
		return
	}
	p.source.SetTextSelection(0, -1)
}

// SetSourceError shows a failure in place of the source text.
func (p *Popup) SetSourceError(msg string) {
	if p.gone() {
		return
	}
	p.source.SetReadOnly(true)
	p.source.SetTextColor(colorError)
	p.source.SetText(crlf(msg))
	p.copySrc.SetEnabled(false)
	p.translBt.SetEnabled(false)
	p.refit()
}

// SetHint replaces the footer's keyboard hint with guidance.
func (p *Popup) SetHint(text string) {
	if p.gone() {
		return
	}
	p.hint.SetText(text)
}

// ShowTranslating opens the translation pane with a placeholder.
func (p *Popup) ShowTranslating(direction string) {
	if p.gone() {
		return
	}
	p.expand()
	p.dir.SetText(direction)
	p.result.SetTextColor(colorMuted)
	p.result.SetText("翻译中…")
	p.copyRes.SetEnabled(false)
	p.refit()
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
	p.result.SetTextColor(colorText)
	p.result.SetText(crlf(text))
	setLinkText(p.copyRes, "复制", true)
	p.copyRes.SetEnabled(text != "")
	p.refit()
}

// SetTranslationError shows a failure in the translation pane.
func (p *Popup) SetTranslationError(msg string) {
	if p.gone() {
		return
	}
	p.expand()
	p.result.SetTextColor(colorError)
	p.result.SetText(crlf(msg))
	p.copyRes.SetEnabled(false)
	p.refit()
}

func (p *Popup) gone() bool { return p.closing || p.mw.IsDisposed() }

// expand opens the translation pane; the next refit makes room for it.
func (p *Popup) expand() {
	if p.expanded {
		return
	}
	p.expanded = true
	setShown(p.trCard, true)
}

// refit resizes the popup to its current text.
func (p *Popup) refit() { p.refitAnchored(keepCentre) }

func (p *Popup) refitAnchored(how anchoring) {
	h := p.mw.Handle()
	work := workArea(win.MonitorFromWindow(h, win.MONITOR_DEFAULTTONEAREST))
	p.fit(work, how)
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

// monitorOf returns the work area and DPI of the monitor nearest anchor.
func monitorOf(anchor image.Rectangle) (image.Rectangle, int) {
	ar := win.RECT{Left: int32(anchor.Min.X), Top: int32(anchor.Min.Y), Right: int32(anchor.Max.X), Bottom: int32(anchor.Max.Y)}
	hmon, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&ar)), win.MONITOR_DEFAULTTONEAREST)
	var dpiX, dpiY uint32 = 96, 96
	win.GetDpiForMonitor(win.HMONITOR(hmon), win.MDT_EFFECTIVE_DPI, &dpiX, &dpiY)
	return workArea(win.HMONITOR(hmon)), int(dpiY)
}

func workArea(hmon win.HMONITOR) image.Rectangle {
	mi := win.MONITORINFO{}
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	win.GetMonitorInfo(hmon, &mi)
	return image.Rect(int(mi.RcWork.Left), int(mi.RcWork.Top), int(mi.RcWork.Right), int(mi.RcWork.Bottom))
}

// centerIn centres a w x h window in work, shrinking it to fit if needed.
func centerIn(work image.Rectangle, w, h int) walk.Rectangle {
	w, h = min(w, work.Dx()), min(h, work.Dy())
	return walk.Rectangle{
		X:      work.Min.X + (work.Dx()-w)/2,
		Y:      work.Min.Y + (work.Dy()-h)/2,
		Width:  w,
		Height: h,
	}
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
