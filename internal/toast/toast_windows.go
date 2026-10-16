package toast

import (
	"errors"
	"fmt"
	"image"
	"log/slog"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/tailscale/win"
	"golang.org/x/sys/windows"
)

var (
	user32                           = windows.NewLazySystemDLL("user32.dll")
	procUpdateLayeredWindow          = user32.NewProc("UpdateLayeredWindow")
	procMonitorFromRect              = user32.NewProc("MonitorFromRect")
	procSetThreadDpiAwarenessContext = user32.NewProc("SetThreadDpiAwarenessContext")
	procDefWindowProc                = user32.NewProc("DefWindowProcW")
)

// Look, in device-independent pixels and BGR colours.
const (
	fontPx     = 15
	padX, padY = 20, 11
	maxTextW   = 460
	bottomGap  = 72 // above the bottom of the work area
	background = 0x00262626
	foreground = 0x00FFFFFF
	bgAlpha    = 0.92
)

var (
	mu      sync.Mutex
	dismiss chan struct{} // closes to take the toast on screen down
)

// Show displays text on the monitor nearest anchor (screen coordinates,
// physical pixels) and returns at once. A newer toast replaces an older one.
// Safe to call from any goroutine.
func Show(text string, anchor image.Rectangle) {
	mu.Lock()
	if dismiss != nil {
		close(dismiss)
	}
	d := make(chan struct{})
	dismiss = d
	mu.Unlock()

	go func() {
		// Never unlocked: the thread dies with the goroutine.
		runtime.LockOSThread()
		if err := run(text, anchor, d); err != nil {
			slog.Warn("toast", "err", err)
		}
	}()
}

var (
	classOnce sync.Once
	className *uint16
	classErr  error
)

func registerClass() error {
	classOnce.Do(func() {
		className, _ = windows.UTF16PtrFromString("SnipGlyphToast")
		wc := win.WNDCLASSEX{
			LpfnWndProc:   procDefWindowProc.Addr(), // no input, nothing to handle
			HInstance:     win.GetModuleHandle(nil),
			LpszClassName: className,
		}
		wc.CbSize = uint32(unsafe.Sizeof(wc))
		if win.RegisterClassEx(&wc) == 0 {
			classErr = fmt.Errorf("RegisterClassEx: %w", windows.GetLastError())
		}
	})
	return classErr
}

func run(text string, anchor image.Rectangle, dismissed <-chan struct{}) error {
	procSetThreadDpiAwarenessContext.Call(^uintptr(3)) // per-monitor v2
	if err := registerClass(); err != nil {
		return err
	}

	ar := win.RECT{Left: int32(anchor.Min.X), Top: int32(anchor.Min.Y), Right: int32(anchor.Max.X), Bottom: int32(anchor.Max.Y)}
	hmon, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&ar)), win.MONITOR_DEFAULTTONEAREST)
	mi := win.MONITORINFO{}
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	win.GetMonitorInfo(win.HMONITOR(hmon), &mi)
	var dpiX, dpiY uint32 = 96, 96
	win.GetDpiForMonitor(win.HMONITOR(hmon), win.MDT_EFFECTIVE_DPI, &dpiX, &dpiY)
	px := func(dip int) int { return dip * int(dpiY) / 96 }

	bmp, err := render(text, px)
	if err != nil {
		return err
	}
	defer bmp.close()

	work := mi.RcWork
	x := int(work.Left) + (int(work.Right-work.Left)-bmp.w)/2
	y := int(work.Bottom) - px(bottomGap) - bmp.h
	hwnd := win.CreateWindowEx(
		win.WS_EX_LAYERED|win.WS_EX_TRANSPARENT|win.WS_EX_TOPMOST|win.WS_EX_TOOLWINDOW|win.WS_EX_NOACTIVATE,
		className, nil, win.WS_POPUP, int32(x), int32(y), int32(bmp.w), int32(bmp.h), 0, 0, win.GetModuleHandle(nil), nil)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowEx: %w", windows.GetLastError())
	}
	defer win.DestroyWindow(hwnd)

	hold := holdFor(text)
	start := time.Now()
	shown := false
	for {
		select {
		case <-dismissed:
			return nil
		default:
		}
		a, ok := opacity(time.Since(start), hold)
		if !ok {
			return nil
		}
		if err := bmp.present(hwnd, x, y, byte(a*255)); err != nil {
			return err
		}
		if !shown {
			win.ShowWindow(hwnd, win.SW_SHOWNOACTIVATE)
			shown = true
		}
		pump()
		time.Sleep(15 * time.Millisecond)
	}
}

// pump keeps the window responsive to the system while it animates.
func pump() {
	var msg win.MSG
	for win.PeekMessage(&msg, 0, 0, 0, win.PM_REMOVE) {
		win.DispatchMessage(&msg)
	}
}

// bitmap is the toast's pixels: premultiplied BGRA in a DIB selected into a
// memory DC, ready for UpdateLayeredWindow.
type bitmap struct {
	w, h int
	dc   win.HDC
	bmp  win.HBITMAP
	old  win.HGDIOBJ
}

func render(text string, px func(int) int) (*bitmap, error) {
	dc := win.CreateCompatibleDC(0)
	if dc == 0 {
		return nil, errors.New("CreateCompatibleDC failed")
	}
	lf := win.LOGFONT{
		LfHeight:  -int32(px(fontPx)),
		LfWeight:  win.FW_NORMAL,
		LfCharSet: win.DEFAULT_CHARSET,
		// Greyscale: ClearType's colour fringes would show once we take the
		// alpha channel over.
		LfQuality: win.ANTIALIASED_QUALITY,
	}
	copy(lf.LfFaceName[:], windows.StringToUTF16("Microsoft YaHei UI"))
	font := win.CreateFontIndirect(&lf)
	defer win.DeleteObject(win.HGDIOBJ(font))
	oldFont := win.SelectObject(dc, win.HGDIOBJ(font))
	defer win.SelectObject(dc, oldFont)

	t := windows.StringToUTF16(text)
	const flags = win.DT_WORDBREAK | win.DT_NOPREFIX | win.DT_CENTER
	tr := win.RECT{Right: int32(px(maxTextW))}
	win.DrawTextEx(dc, &t[0], int32(len(t)-1), &tr, flags|win.DT_CALCRECT, nil)
	b := &bitmap{w: int(tr.Right) + 2*px(padX), h: int(tr.Bottom) + 2*px(padY), dc: dc}

	bih := win.BITMAPINFOHEADER{BiWidth: int32(b.w), BiHeight: -int32(b.h), BiPlanes: 1, BiBitCount: 32, BiCompression: win.BI_RGB}
	bih.BiSize = uint32(unsafe.Sizeof(bih))
	var bits unsafe.Pointer
	if b.bmp = win.CreateDIBSection(dc, &bih, win.DIB_RGB_COLORS, &bits, 0, 0); b.bmp == 0 || bits == nil {
		win.DeleteDC(dc)
		return nil, errors.New("CreateDIBSection failed")
	}
	b.old = win.SelectObject(dc, win.HGDIOBJ(b.bmp))
	pix := unsafe.Slice((*byte)(bits), b.w*b.h*4)

	// Opaque background, then the text on top of it.
	for i := 0; i < len(pix); i += 4 {
		pix[i], pix[i+1], pix[i+2] = background>>16&0xFF, background>>8&0xFF, background&0xFF
	}
	win.SetBkMode(dc, win.TRANSPARENT)
	win.SetTextColor(dc, foreground)
	textRect := win.RECT{Left: int32(px(padX)), Top: int32(px(padY)), Right: int32(b.w - px(padX)), Bottom: int32(b.h - px(padY))}
	win.DrawTextEx(dc, &t[0], int32(len(t)-1), &textRect, flags, nil)

	// GDI leaves alpha undefined: derive it from the rounded shape and
	// premultiply, as UpdateLayeredWindow expects.
	radius := float64(min(b.h/2, px(14)))
	for y := 0; y < b.h; y++ {
		for x := 0; x < b.w; x++ {
			a := coverage(x, y, b.w, b.h, radius) * bgAlpha
			i := (y*b.w + x) * 4
			pix[i] = byte(float64(pix[i]) * a)
			pix[i+1] = byte(float64(pix[i+1]) * a)
			pix[i+2] = byte(float64(pix[i+2]) * a)
			pix[i+3] = byte(a * 255)
		}
	}
	return b, nil
}

func (b *bitmap) present(hwnd win.HWND, x, y int, alpha byte) error {
	pos := win.POINT{X: int32(x), Y: int32(y)}
	size := win.SIZE{CX: int32(b.w), CY: int32(b.h)}
	var src win.POINT
	blend := win.BLENDFUNCTION{BlendOp: 0, SourceConstantAlpha: alpha, AlphaFormat: win.AC_SRC_ALPHA}
	const ulwAlpha = 2
	ok, _, err := procUpdateLayeredWindow.Call(uintptr(hwnd), 0,
		uintptr(unsafe.Pointer(&pos)), uintptr(unsafe.Pointer(&size)), uintptr(b.dc),
		uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
	if ok == 0 {
		return fmt.Errorf("UpdateLayeredWindow: %w", err)
	}
	return nil
}

func (b *bitmap) close() {
	win.SelectObject(b.dc, b.old)
	win.DeleteObject(win.HGDIOBJ(b.bmp))
	win.DeleteDC(b.dc)
}
