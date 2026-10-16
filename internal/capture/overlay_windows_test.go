package capture

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
	"unsafe"

	"github.com/tailscale/win"
	"golang.org/x/sys/windows"
)

// These tests flash the overlay on the real screen, so they only run when
// asked to. Mouse input is posted to the overlay window itself, never
// injected system-wide, so nothing else on the desktop can be clicked.
func requireUITests(t *testing.T) {
	if os.Getenv("SNIPGLYPH_UI_TESTS") == "" {
		t.Skip("set SNIPGLYPH_UI_TESTS=1 to run overlay tests")
	}
}

var procFindWindow = windows.NewLazySystemDLL("user32.dll").NewProc("FindWindowW")

// The app's manifest makes it per-monitor DPI aware; the test binary has no
// manifest, and Windows would rescale the coordinates we post.
func TestMain(m *testing.M) {
	user32.NewProc("SetProcessDpiAwarenessContext").Call(dpiAwarenessPerMonitorV2)
	os.Exit(m.Run())
}

func waitOverlay(t *testing.T) win.HWND {
	t.Helper()
	cls, _ := windows.UTF16PtrFromString("SnipGlyphCapture")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		h, _, _ := procFindWindow.Call(uintptr(unsafePointer(cls)), 0)
		if h != 0 && win.IsWindowVisible(win.HWND(h)) {
			return win.HWND(h)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("overlay never appeared")
	return 0
}

func post(hwnd win.HWND, msg uint32, x, y int) {
	win.PostMessage(hwnd, msg, 0, uintptr(uint16(x))|uintptr(uint16(y))<<16)
}

type selectResult struct {
	s   Shot
	err error
}

func startSelect(ctx context.Context) <-chan selectResult {
	ch := make(chan selectResult, 1)
	go func() {
		s, err := Select(ctx)
		ch <- selectResult{s, err}
	}()
	return ch
}

func TestSelectDrag(t *testing.T) {
	requireUITests(t)
	began := time.Now()
	ch := startSelect(context.Background())
	hwnd := waitOverlay(t)
	t.Logf("overlay up after %v", time.Since(began))

	if _, err := Select(context.Background()); !errors.Is(err, ErrBusy) {
		t.Errorf("second Select: err = %v, want ErrBusy", err)
	}

	post(hwnd, win.WM_MOUSEMOVE, 100, 100)
	post(hwnd, win.WM_LBUTTONDOWN, 100, 100)
	post(hwnd, win.WM_MOUSEMOVE, 200, 150)
	post(hwnd, win.WM_MOUSEMOVE, 300, 250)
	post(hwnd, win.WM_LBUTTONUP, 300, 250)

	r := <-ch
	if r.err != nil {
		t.Fatal(r.err)
	}
	vx, vy := int(win.GetSystemMetrics(win.SM_XVIRTUALSCREEN)), int(win.GetSystemMetrics(win.SM_YVIRTUALSCREEN))
	want := Rect{100, 100, 301, 251}.Add(vx, vy)
	if r.s.Bounds != want {
		t.Fatalf("Bounds = %+v, want %+v", r.s.Bounds, want)
	}
	if len(r.s.Pix) != want.Dx()*want.Dy()*4 {
		t.Fatalf("got %d bytes, want %d", len(r.s.Pix), want.Dx()*want.Dy()*4)
	}
	if r.s.Pix[3] != 0xFF {
		t.Error("alpha not opaque")
	}
}

func TestSelectClickPicksTargetUnderCursor(t *testing.T) {
	requireUITests(t)
	ch := startSelect(context.Background())
	hwnd := waitOverlay(t)
	post(hwnd, win.WM_MOUSEMOVE, 400, 300)
	post(hwnd, win.WM_LBUTTONDOWN, 400, 300)
	post(hwnd, win.WM_LBUTTONUP, 401, 301) // within the click slop
	r := <-ch
	if r.err != nil {
		t.Fatal(r.err)
	}
	vx, vy := int(win.GetSystemMetrics(win.SM_XVIRTUALSCREEN)), int(win.GetSystemMetrics(win.SM_YVIRTUALSCREEN))
	if !r.s.Bounds.Contains(400+vx, 300+vy) {
		t.Fatalf("Bounds %+v do not contain the click", r.s.Bounds)
	}
	t.Logf("clicked target: %+v", r.s.Bounds)
}

func TestSelectCancel(t *testing.T) {
	requireUITests(t)

	ch := startSelect(context.Background())
	hwnd := waitOverlay(t)
	win.PostMessage(hwnd, win.WM_RBUTTONUP, 0, 0)
	if r := <-ch; !errors.Is(r.err, ErrCanceled) {
		t.Fatalf("right click: err = %v, want ErrCanceled", r.err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch = startSelect(ctx)
	waitOverlay(t)
	cancel()
	if r := <-ch; !errors.Is(r.err, context.Canceled) {
		t.Fatalf("context cancel: err = %v, want context.Canceled", r.err)
	}
}

func unsafePointer(p *uint16) unsafe.Pointer { return unsafe.Pointer(p) }
