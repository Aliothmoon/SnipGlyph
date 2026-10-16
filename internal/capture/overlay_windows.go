package capture

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/tailscale/win"
	"golang.org/x/sys/windows"
)

var (
	// ErrCanceled means the user dismissed the overlay (Esc, right click,
	// switching away) or the context was cancelled.
	ErrCanceled = errors.New("capture canceled")
	// ErrBusy means another selection is already on screen.
	ErrBusy = errors.New("capture already in progress")
)

// Shot is the selected region.
type Shot struct {
	Bounds Rect   // in screen coordinates (physical pixels)
	Pix    []byte // BGRA, top-down, Bounds.Dx()*Bounds.Dy()*4 bytes
}

var busy sync.Mutex

// Select freezes the screen and blocks until the user picks a region or
// cancels. It runs the overlay on its own OS thread, so it may be called
// from any goroutine except the UI thread of a message loop it would stall.
func Select(ctx context.Context) (Shot, error) {
	if !busy.TryLock() {
		return Shot{}, ErrBusy
	}
	defer busy.Unlock()
	type result struct {
		s   Shot
		err error
	}
	ch := make(chan result, 1)
	go func() {
		// Never unlocked: the thread dies with the goroutine, taking its DPI
		// awareness override with it.
		runtime.LockOSThread()
		s, err := run(ctx)
		ch <- result{s, err}
	}()
	r := <-ch
	return r.s, r.err
}

var (
	user32                           = windows.NewLazySystemDLL("user32.dll")
	procSetThreadDpiAwarenessContext = user32.NewProc("SetThreadDpiAwarenessContext")
	procTranslateMessage             = user32.NewProc("TranslateMessage")
)

const (
	dpiAwarenessPerMonitorV2 = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
	clickSlop                = 3           // px the mouse may move and still count as a click
	borderColor              = 0x00FF901E  // COLORREF (BGR) of #1E90FF
	labelBg                  = 0x00202020
	labelFg                  = 0x00FFFFFF
)

var (
	classOnce   sync.Once
	className16 *uint16
	classErr    error
	wndProcCB   = syscall.NewCallback(wndProc)
	current     *overlay // the overlay on screen; set only on its thread
)

type overlay struct {
	hwnd win.HWND
	f    *frame
	lay  layout
	vs   Rect // virtual screen in screen coordinates

	dpi    int
	border int
	font   win.HFONT

	pressed, dragging bool
	pressX, pressY    int
	chain             []Rect // hover candidates, smallest first
	level             int    // index into chain, changed by the wheel
	sel               Rect   // highlighted rectangle, client coordinates

	done     bool
	canceled bool
	result   Rect
}

func run(ctx context.Context) (Shot, error) {
	// Physical pixels everywhere, whatever the process manifest says.
	procSetThreadDpiAwarenessContext.Call(dpiAwarenessPerMonitorV2)

	vs := Rect{
		Left: int(win.GetSystemMetrics(win.SM_XVIRTUALSCREEN)),
		Top:  int(win.GetSystemMetrics(win.SM_YVIRTUALSCREEN)),
	}
	vs.Right = vs.Left + int(win.GetSystemMetrics(win.SM_CXVIRTUALSCREEN))
	vs.Bottom = vs.Top + int(win.GetSystemMetrics(win.SM_CYVIRTUALSCREEN))

	f, err := grab(vs)
	if err != nil {
		return Shot{}, err
	}
	defer f.close()
	o := &overlay{f: f, lay: collectLayout(vs), vs: vs}

	if err := registerClass(); err != nil {
		return Shot{}, err
	}
	current = o
	defer func() { current = nil }()
	o.hwnd = win.CreateWindowEx(win.WS_EX_TOPMOST|win.WS_EX_TOOLWINDOW, className16, nil, win.WS_POPUP,
		int32(vs.Left), int32(vs.Top), int32(vs.Dx()), int32(vs.Dy()), 0, 0, win.GetModuleHandle(nil), nil)
	if o.hwnd == 0 {
		return Shot{}, fmt.Errorf("CreateWindowEx: %w", windows.GetLastError())
	}
	o.dpi = int(win.GetDpiForWindow(o.hwnd))
	o.border = max(1, 2*o.dpi/96)
	o.font = newFont(o.dpi)
	defer win.DeleteObject(win.HGDIOBJ(o.font))

	var pt win.POINT
	win.GetCursorPos(&pt)
	o.hover(int(pt.X)-vs.Left, int(pt.Y)-vs.Top)

	win.ShowWindow(o.hwnd, win.SW_SHOW)
	win.SetForegroundWindow(o.hwnd)
	stop := context.AfterFunc(ctx, func() { win.PostMessage(o.hwnd, win.WM_CLOSE, 0, 0) })
	defer stop()

	var msg win.MSG
	for win.GetMessage(&msg, 0, 0, 0) > 0 {
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		win.DispatchMessage(&msg)
	}

	if err := ctx.Err(); err != nil {
		return Shot{}, err
	}
	if o.canceled || o.result.Empty() {
		return Shot{}, ErrCanceled
	}
	return Shot{Bounds: o.result.Add(vs.Left, vs.Top), Pix: f.crop(o.result)}, nil
}

func registerClass() error {
	classOnce.Do(func() {
		className16, _ = windows.UTF16PtrFromString("SnipGlyphCapture")
		wc := win.WNDCLASSEX{
			LpfnWndProc:   wndProcCB,
			HInstance:     win.GetModuleHandle(nil),
			HCursor:       win.LoadCursor(0, win.MAKEINTRESOURCE(win.IDC_CROSS)),
			LpszClassName: className16,
		}
		wc.CbSize = uint32(unsafe.Sizeof(wc))
		if win.RegisterClassEx(&wc) == 0 {
			classErr = fmt.Errorf("RegisterClassEx: %w", windows.GetLastError())
		}
	})
	return classErr
}

func newFont(dpi int) win.HFONT {
	lf := win.LOGFONT{
		LfHeight:  -int32(13 * dpi / 96),
		LfWeight:  win.FW_NORMAL,
		LfCharSet: win.DEFAULT_CHARSET,
		LfQuality: win.CLEARTYPE_QUALITY,
	}
	copy(lf.LfFaceName[:], windows.StringToUTF16("Segoe UI"))
	return win.CreateFontIndirect(&lf)
}

func wndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	o := current
	if o == nil || o.hwnd != hwnd {
		return win.DefWindowProc(hwnd, msg, wParam, lParam)
	}
	x, y := int(int16(lParam&0xFFFF)), int(int16(lParam>>16&0xFFFF))
	switch msg {
	case win.WM_PAINT:
		o.paint()
		return 0
	case win.WM_ERASEBKGND:
		return 1
	case win.WM_MOUSEMOVE:
		o.mouseMove(x, y)
		return 0
	case win.WM_LBUTTONDOWN:
		o.pressed, o.dragging = true, false
		o.pressX, o.pressY = x, y
		win.SetCapture(hwnd)
		return 0
	case win.WM_LBUTTONUP:
		if !o.pressed {
			return 0
		}
		o.pressed = false
		win.ReleaseCapture()
		if !o.sel.Empty() {
			o.finish(o.sel)
		}
		return 0
	case win.WM_RBUTTONUP:
		o.cancel()
		return 0
	case win.WM_KEYDOWN:
		if wParam == win.VK_ESCAPE {
			o.cancel()
		}
		return 0
	case win.WM_MOUSEWHEEL:
		if !o.dragging {
			delta := int16(wParam >> 16 & 0xFFFF)
			step := 1
			if delta < 0 {
				step = -1
			}
			o.setSel(o.pick(o.level + step))
		}
		return 0
	case win.WM_ACTIVATE:
		if wParam&0xFFFF == win.WA_INACTIVE {
			o.cancel() // the user switched away: never leave a stale overlay
		}
		return 0
	case win.WM_DPICHANGED:
		return 0 // keep covering the whole virtual screen
	case win.WM_CLOSE:
		o.cancel()
		return 0
	case win.WM_DESTROY:
		win.PostQuitMessage(0)
		return 0
	}
	return win.DefWindowProc(hwnd, msg, wParam, lParam)
}

func (o *overlay) mouseMove(x, y int) {
	x = min(max(x, 0), o.f.w-1)
	y = min(max(y, 0), o.f.h-1)
	if o.pressed && !o.dragging && (abs(x-o.pressX) > clickSlop || abs(y-o.pressY) > clickSlop) {
		o.dragging = true
	}
	if o.dragging {
		o.setSel(rectFromPoints(o.pressX, o.pressY, x, y))
		return
	}
	o.hover(x, y)
}

// hover selects the smallest target under (x, y), or keeps the wheel level
// while the cursor stays over the same smallest target.
func (o *overlay) hover(x, y int) {
	chain := o.lay.chainAt(x, y)
	if len(chain) == 0 || len(o.chain) == 0 || chain[0] != o.chain[0] {
		o.level = 0
	}
	o.chain = chain
	o.setSel(o.pick(o.level))
}

func (o *overlay) pick(level int) Rect {
	if len(o.chain) == 0 {
		return Rect{}
	}
	o.level = min(max(level, 0), len(o.chain)-1)
	return o.chain[o.level]
}

func (o *overlay) setSel(r Rect) {
	if r == o.sel {
		return
	}
	o.invalidate(o.sel)
	o.sel = r
	o.invalidate(o.sel)
}

func (o *overlay) invalidate(sel Rect) {
	if sel.Empty() {
		return
	}
	d := sel.Outset(o.border).Union(o.labelRect(sel))
	wr := win.RECT{Left: int32(d.Left), Top: int32(d.Top), Right: int32(d.Right), Bottom: int32(d.Bottom)}
	win.InvalidateRect(o.hwnd, &wr, false)
}

func (o *overlay) finish(r Rect) {
	if o.done {
		return
	}
	o.done, o.result = true, r
	win.DestroyWindow(o.hwnd)
}

func (o *overlay) cancel() {
	if o.done {
		return
	}
	o.done, o.canceled = true, true
	win.DestroyWindow(o.hwnd)
}

func (o *overlay) label(sel Rect) string {
	return fmt.Sprintf("%d × %d", sel.Dx(), sel.Dy())
}

// labelRect places the size label just above the selection's top-left
// corner, or inside it when there is no room above.
func (o *overlay) labelRect(sel Rect) Rect {
	text := windows.StringToUTF16(o.label(sel))
	old := win.SelectObject(o.f.orig.dc, win.HGDIOBJ(o.font))
	var m win.RECT
	win.DrawTextEx(o.f.orig.dc, &text[0], int32(len(text)-1), &m, win.DT_CALCRECT|win.DT_SINGLELINE|win.DT_NOPREFIX, nil)
	win.SelectObject(o.f.orig.dc, old)
	padX, padY, gap := 6*o.dpi/96, 2*o.dpi/96, 4*o.dpi/96
	w, h := int(m.Right-m.Left)+2*padX, int(m.Bottom-m.Top)+2*padY
	top := sel.Top - o.border - gap - h
	if top < 0 {
		top = sel.Top + gap
	}
	left := min(sel.Left, o.f.w-w)
	return Rect{left, top, left + w, top + h}
}

func (o *overlay) paint() {
	var ps win.PAINTSTRUCT
	hdc := win.BeginPaint(o.hwnd, &ps)
	defer win.EndPaint(o.hwnd, &ps)
	r := Rect{int(ps.RcPaint.Left), int(ps.RcPaint.Top), int(ps.RcPaint.Right), int(ps.RcPaint.Bottom)}.
		Intersect(Rect{0, 0, o.f.w, o.f.h})
	if r.Empty() {
		return
	}

	// Compose the dirty area off-screen, then blit it once: no flicker.
	mem := win.CreateCompatibleDC(hdc)
	defer win.DeleteDC(mem)
	bmp := win.CreateCompatibleBitmap(hdc, int32(r.Dx()), int32(r.Dy()))
	defer win.DeleteObject(win.HGDIOBJ(bmp))
	old := win.SelectObject(mem, win.HGDIOBJ(bmp))
	defer win.SelectObject(mem, old)

	win.BitBlt(mem, 0, 0, int32(r.Dx()), int32(r.Dy()), o.f.dim.dc, int32(r.Left), int32(r.Top), win.SRCCOPY)
	if !o.sel.Empty() {
		if s := o.sel.Intersect(r); !s.Empty() {
			win.BitBlt(mem, int32(s.Left-r.Left), int32(s.Top-r.Top), int32(s.Dx()), int32(s.Dy()),
				o.f.orig.dc, int32(s.Left), int32(s.Top), win.SRCCOPY)
		}
		o.drawBorder(mem, o.sel.Add(-r.Left, -r.Top))
		o.drawLabel(mem, o.labelRect(o.sel).Add(-r.Left, -r.Top), o.label(o.sel))
	}
	win.BitBlt(hdc, int32(r.Left), int32(r.Top), int32(r.Dx()), int32(r.Dy()), mem, 0, 0, win.SRCCOPY)
}

// drawBorder frames s from the outside, so the selected pixels stay visible.
func (o *overlay) drawBorder(dc win.HDC, s Rect) {
	t := o.border
	brush := solidBrush(borderColor)
	defer win.DeleteObject(win.HGDIOBJ(brush))
	for _, e := range []Rect{
		{s.Left - t, s.Top - t, s.Right + t, s.Top},
		{s.Left - t, s.Bottom, s.Right + t, s.Bottom + t},
		{s.Left - t, s.Top, s.Left, s.Bottom},
		{s.Right, s.Top, s.Right + t, s.Bottom},
	} {
		fill(dc, e, brush)
	}
}

func (o *overlay) drawLabel(dc win.HDC, r Rect, text string) {
	brush := solidBrush(labelBg)
	defer win.DeleteObject(win.HGDIOBJ(brush))
	fill(dc, r, brush)
	old := win.SelectObject(dc, win.HGDIOBJ(o.font))
	defer win.SelectObject(dc, old)
	win.SetBkMode(dc, win.TRANSPARENT)
	win.SetTextColor(dc, labelFg)
	t := windows.StringToUTF16(text)
	wr := win.RECT{Left: int32(r.Left), Top: int32(r.Top), Right: int32(r.Right), Bottom: int32(r.Bottom)}
	win.DrawTextEx(dc, &t[0], int32(len(t)-1), &wr, win.DT_CENTER|win.DT_VCENTER|win.DT_SINGLELINE|win.DT_NOPREFIX, nil)
}

func solidBrush(c win.COLORREF) win.HBRUSH {
	return win.CreateBrushIndirect(&win.LOGBRUSH{LbStyle: win.BS_SOLID, LbColor: c})
}

func fill(dc win.HDC, r Rect, b win.HBRUSH) {
	wr := win.RECT{Left: int32(r.Left), Top: int32(r.Top), Right: int32(r.Right), Bottom: int32(r.Bottom)}
	win.FillRect(dc, &wr, b)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
