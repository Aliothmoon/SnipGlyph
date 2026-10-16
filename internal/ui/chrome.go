package ui

import (
	"unsafe"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"
	"golang.org/x/sys/windows"
)

var (
	comctl32                 = windows.NewLazySystemDLL("comctl32.dll")
	procSetWindowSubclass    = comctl32.NewProc("SetWindowSubclass")
	procRemoveWindowSubclass = comctl32.NewProc("RemoveWindowSubclass")
	procDefSubclassProc      = comctl32.NewProc("DefSubclassProc")
)

// msgHook sees a window's messages before walk does. It returns the result
// and true to handle a message, or false to pass it on.
type msgHook func(hwnd win.HWND, msg uint32, wp, lp uintptr) (uintptr, bool)

// hooks is touched only on the UI thread, which owns the hooked windows.
var hooks = map[win.HWND]msgHook{}

// subclassCB is shared by every hooked window: callbacks are a limited
// resource and are never freed.
var subclassCB uintptr

func init() { subclassCB = windows.NewCallback(subclassProc) }

func subclassProc(hwnd win.HWND, msg uint32, wp, lp, id, ref uintptr) uintptr {
	if msg == win.WM_NCDESTROY {
		delete(hooks, hwnd)
		procRemoveWindowSubclass.Call(uintptr(hwnd), subclassCB, 1)
	} else if f := hooks[hwnd]; f != nil {
		if r, ok := f(hwnd, msg, wp, lp); ok {
			return r
		}
	}
	r, _, _ := procDefSubclassProc.Call(uintptr(hwnd), uintptr(msg), wp, lp)
	return r
}

// hookWindow routes hwnd's messages through f first. UI thread.
func hookWindow(hwnd win.HWND, f msgHook) {
	hooks[hwnd] = f
	procSetWindowSubclass.Call(uintptr(hwnd), subclassCB, 1, 0)
}

// frameless removes the title bar and border, so the content draws its own
// header, while keeping the Windows 11 shadow and rounded corners: the
// window keeps its caption style for DWM, but its client area covers the
// whole window.
func frameless(mw *walk.MainWindow) {
	h := mw.Handle()
	hookWindow(h, func(hwnd win.HWND, msg uint32, wp, lp uintptr) (uintptr, bool) {
		switch msg {
		case win.WM_NCCALCSIZE:
			if wp != 0 {
				return 0, true // client area = window area
			}
		case win.WM_NCACTIVATE:
			// Skip repainting the (now invisible) frame on activation, which
			// can flash a title bar for a frame.
			r, _, _ := procDefSubclassProc.Call(uintptr(hwnd), uintptr(msg), wp, ^uintptr(0))
			return r, true
		}
		return 0, false
	})
	// DWM draws the shadow only for windows with some frame; one pixel of
	// extended frame keeps it. The content paints over that pixel.
	win.DwmExtendFrameIntoClientArea(h, &win.MARGINS{TopHeight: 1})
	const dwmwaWindowCornerPreference, dwmwcpRound = 33, 2
	pref := uint32(dwmwcpRound)
	procDwmSetWindowAttribute.Call(uintptr(h), dwmwaWindowCornerPreference, uintptr(unsafe.Pointer(&pref)), 4)
	win.SetWindowPos(h, 0, 0, 0, 0, 0, win.SWP_FRAMECHANGED|win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOZORDER|win.SWP_NOACTIVATE)
}

// dragBy lets the user move the window by dragging w, like a title bar.
func dragBy(form walk.Form, w walk.Window) {
	w.MouseDown().Attach(func(x, y int, button walk.MouseButton) {
		if button != walk.LeftButton {
			return
		}
		win.ReleaseCapture()
		win.SendMessage(form.Handle(), win.WM_NCLBUTTONDOWN, win.HTCAPTION, 0)
	})
}

// cloak hides a shown window from the screen without hiding it from
// layout: walk only lays out visible children, so a form is measured
// while shown but cloaked, then revealed at its final size and place.
func cloak(h win.HWND, on bool) {
	const dwmwaCloak = 13
	var v int32
	if on {
		v = 1
	}
	procDwmSetWindowAttribute.Call(uintptr(h), dwmwaCloak, uintptr(unsafe.Pointer(&v)), 4)
}
