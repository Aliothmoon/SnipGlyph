package ui

import (
	"image"
	"log/slog"
	"unsafe"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"
	"golang.org/x/sys/windows"
)

// newWindow creates a hidden top-level window in the app's style, without
// walk's empty menu bar and toolbar. Closing it does not end the app.
func newWindow(title string, cfg *walk.MainWindowCfg) (*walk.MainWindow, error) {
	mw, err := walk.NewMainWindowWithCfg(cfg)
	if err != nil {
		return nil, err
	}
	mw.SetExitOnClose(false) // walk would otherwise end the app with this window
	mw.SetPersistent(false)
	win.SetMenu(mw.Handle(), 0) // drop walk's empty menu bar
	// walk's layout ignores an empty toolbar, but the toolbar window still
	// paints over the top of the content.
	setShown(mw.ToolBar(), false)
	mw.SetIcon(appIcon())
	setFont(mw, uiFont()) // inherited by every child without its own font
	setBackground(mw, colorWindow)
	tintCaption(mw.Handle(), colorWindow)
	if err := mw.SetTitle(title); err != nil {
		mw.Dispose()
		return nil, err
	}
	return mw, nil
}

// cursorMonitor returns the work area and DPI of the monitor under the
// mouse pointer.
func cursorMonitor() (image.Rectangle, int) {
	var pt win.POINT
	win.GetCursorPos(&pt)
	return monitorOf(image.Rect(int(pt.X), int(pt.Y), int(pt.X)+1, int(pt.Y)+1))
}

// showFitted shows mw with a client area width DIPs wide and as tall as
// its layout needs, centred on the monitor under the mouse pointer. walk
// lays out only visible children, so the window is measured while shown
// but cloaked, then revealed in place.
func showFitted(mw *walk.MainWindow, width int) error {
	work, dpi := cursorMonitor()
	w := width * dpi / 96
	// Move onto the monitor first, so the window adopts its DPI.
	if err := mw.SetBoundsPixels(centerIn(work, w, w)); err != nil {
		return err
	}
	h := mw.Handle()
	cloak(h, true)
	defer cloak(h, false)
	mw.Show()
	frameX, frameY := frameSize(h)
	height := w
	if li, ok := walk.CreateLayoutItemsForContainer(mw).(walk.MinSizeForSizer); ok {
		height = li.MinSizeForSize(walk.Size{Width: w}).Height
	}
	return mw.SetBoundsPixels(centerIn(work, w+frameX, height+frameY))
}

// refitHeight grows or shrinks mw to its layout's height at the current
// width, keeping the top edge where it is.
func refitHeight(mw *walk.MainWindow) {
	h := mw.Handle()
	li, ok := walk.CreateLayoutItemsForContainer(mw).(walk.MinSizeForSizer)
	if !ok {
		return
	}
	var cr win.RECT
	win.GetClientRect(h, &cr)
	_, frameY := frameSize(h)
	height := li.MinSizeForSize(walk.Size{Width: int(cr.Right - cr.Left)}).Height + frameY
	work := workArea(win.MonitorFromWindow(h, win.MONITOR_DEFAULTTONEAREST))
	mw.SetBoundsPixels(growDown(mw.BoundsPixels(), mw.BoundsPixels().Width, height, work))
}

// frameSize is how much larger the window is than its client area.
func frameSize(h win.HWND) (x, y int) {
	var wr, cr win.RECT
	win.GetWindowRect(h, &wr)
	win.GetClientRect(h, &cr)
	return int(wr.Right-wr.Left) - int(cr.Right-cr.Left), int(wr.Bottom-wr.Top) - int(cr.Bottom-cr.Top)
}

// bringToFront shows mw and makes it the active window.
func bringToFront(mw *walk.MainWindow) {
	h := mw.Handle()
	if win.IsIconic(h) {
		win.ShowWindow(h, win.SW_RESTORE)
	}
	win.SetForegroundWindow(h)
}

// openURL opens a web page in the default browser.
func openURL(url string) {
	verb, _ := windows.UTF16PtrFromString("open")
	u, err := windows.UTF16PtrFromString(url)
	if err == nil {
		err = windows.ShellExecute(0, verb, u, nil, nil, windows.SW_SHOWNORMAL)
	}
	if err != nil {
		slog.Warn("open url", "url", url, "err", err)
	}
}

// setCueBanner shows a grey hint in an empty line edit, even while it has
// the focus (which a search box has from the start).
func setCueBanner(le *walk.LineEdit, text string) {
	cue, _ := windows.UTF16PtrFromString(text)
	le.SendMessage(emSetCueBanner, 1, uintptr(unsafe.Pointer(cue)))
}
