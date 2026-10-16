package capture

import (
	"syscall"
	"unsafe"

	"github.com/tailscale/win"
	"golang.org/x/sys/windows"
)

var procEnumDisplayMonitors = windows.NewLazySystemDLL("user32.dll").NewProc("EnumDisplayMonitors")

// Enumeration state. Only the capture thread enumerates, one capture at a
// time (see Select), so plain package variables are enough.
var (
	enumOrigin   Rect // virtual screen, to convert to client coordinates
	enumWindows  []window
	enumChildren []Rect
	enumMonitors []Rect

	topCallback     = syscall.NewCallback(onTopWindow)
	childCallback   = syscall.NewCallback(onChildWindow)
	monitorCallback = syscall.NewCallback(onMonitor)
)

// collectLayout snapshots the windows, their controls and the monitors,
// translated so that vs's top-left corner is (0, 0).
func collectLayout(vs Rect) layout {
	enumOrigin = vs
	enumWindows, enumMonitors = nil, nil
	_ = windows.EnumWindows(topCallback, nil)
	procEnumDisplayMonitors.Call(0, 0, monitorCallback, 0)
	l := layout{windows: enumWindows, monitors: enumMonitors}
	enumWindows, enumChildren, enumMonitors = nil, nil, nil
	return l
}

func toClient(r win.RECT) Rect {
	return Rect{int(r.Left), int(r.Top), int(r.Right), int(r.Bottom)}.
		Add(-enumOrigin.Left, -enumOrigin.Top).
		Intersect(Rect{0, 0, enumOrigin.Dx(), enumOrigin.Dy()})
}

func onTopWindow(hwnd win.HWND, _ uintptr) uintptr {
	if !win.IsWindowVisible(hwnd) || win.IsIconic(hwnd) {
		return 1
	}
	// Click-through overlays (game bars, screen annotators) are not targets.
	if uint32(win.GetWindowLong(hwnd, win.GWL_EXSTYLE))&win.WS_EX_TRANSPARENT != 0 {
		return 1
	}
	// Cloaked: UWP apps in the background, windows on other virtual desktops.
	var cloaked uint32
	if win.DwmGetWindowAttribute(hwnd, win.DWMWA_CLOAKED, unsafe.Pointer(&cloaked), 4) == 0 && cloaked != 0 {
		return 1
	}
	var wr win.RECT
	// The extended frame excludes the invisible resize borders and shadow.
	if win.DwmGetWindowAttribute(hwnd, win.DWMWA_EXTENDED_FRAME_BOUNDS, unsafe.Pointer(&wr), uint32(unsafe.Sizeof(wr))) != 0 {
		win.GetWindowRect(hwnd, &wr)
	}
	r := toClient(wr)
	if r.Dx() < 4 || r.Dy() < 4 {
		return 1
	}
	w := window{rect: r}
	switch className(hwnd) {
	case "Progman", "WorkerW":
		w.desktop = true
	default:
		enumChildren = nil
		win.EnumChildWindows(hwnd, childCallback, 0)
		for _, c := range enumChildren {
			if c = c.Intersect(r); c.Dx() >= 4 && c.Dy() >= 4 {
				w.children = append(w.children, c)
			}
		}
	}
	enumWindows = append(enumWindows, w)
	return 1
}

func onChildWindow(hwnd win.HWND, _ uintptr) uintptr {
	if win.IsWindowVisible(hwnd) {
		var wr win.RECT
		if win.GetWindowRect(hwnd, &wr) {
			enumChildren = append(enumChildren, toClient(wr))
		}
	}
	return 1
}

func onMonitor(_ uintptr, _ uintptr, r *win.RECT, _ uintptr) uintptr {
	if m := toClient(*r); !m.Empty() {
		enumMonitors = append(enumMonitors, m)
	}
	return 1
}

func className(hwnd win.HWND) string {
	var buf [64]uint16
	n, _ := win.GetClassName(hwnd, &buf[0], len(buf))
	return windows.UTF16ToString(buf[:n])
}
