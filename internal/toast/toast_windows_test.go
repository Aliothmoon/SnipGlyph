package toast

import (
	"image"
	"image/png"
	"os"
	"testing"
	"time"
	"unsafe"

	"github.com/tailscale/win"
	"golang.org/x/sys/windows"
)

var procFindWindowEx = user32.NewProc("FindWindowExW")

// toastWindows lists the visible toast windows.
func toastWindows() []win.HWND {
	cls, _ := windows.UTF16PtrFromString("SnipGlyphToast")
	var out []win.HWND
	var h uintptr
	for {
		h, _, _ = procFindWindowEx.Call(0, h, uintptr(unsafe.Pointer(cls)), 0)
		if h == 0 {
			return out
		}
		if win.IsWindowVisible(win.HWND(h)) {
			out = append(out, win.HWND(h))
		}
	}
}

// TestShowOnScreen shows real toasts; set SNIPGLYPH_UI_TESTS to run it and
// SNAP_OUT to a .png path to get a screenshot of one.
func TestShowOnScreen(t *testing.T) {
	if os.Getenv("SNIPGLYPH_UI_TESTS") == "" {
		t.Skip("set SNIPGLYPH_UI_TESTS=1 to show toasts on screen")
	}
	procSetProcessDpiAwarenessContext := user32.NewProc("SetProcessDpiAwarenessContext")
	procSetProcessDpiAwarenessContext.Call(^uintptr(3))
	anchor := image.Rect(100, 100, 200, 200) // somewhere on the primary monitor

	Show("第一条，应被下一条顶替", anchor)
	time.Sleep(100 * time.Millisecond)
	Show("已复制 3 行文字", anchor)
	time.Sleep(400 * time.Millisecond)
	ws := toastWindows()
	if len(ws) != 1 {
		t.Fatalf("%d toasts on screen, want 1", len(ws))
	}
	if out := os.Getenv("SNAP_OUT"); out != "" {
		var r win.RECT
		win.GetWindowRect(ws[0], &r)
		snap(t, image.Rect(int(r.Left)-40, int(r.Top)-30, int(r.Right)+40, int(r.Bottom)+30), out)
		t.Logf("toast at %+v", r)
	}

	deadline := time.Now().Add(fadeIn + holdFor("已复制 3 行文字") + fadeOut + time.Second)
	for len(toastWindows()) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("toast did not go away")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func snap(t *testing.T, r image.Rectangle, path string) {
	screen := win.GetDC(0)
	defer win.ReleaseDC(0, screen)
	dc := win.CreateCompatibleDC(screen)
	defer win.DeleteDC(dc)
	bih := win.BITMAPINFOHEADER{BiWidth: int32(r.Dx()), BiHeight: -int32(r.Dy()), BiPlanes: 1, BiBitCount: 32}
	bih.BiSize = uint32(unsafe.Sizeof(bih))
	var bits unsafe.Pointer
	bmp := win.CreateDIBSection(dc, &bih, 0, &bits, 0, 0)
	defer win.DeleteObject(win.HGDIOBJ(bmp))
	old := win.SelectObject(dc, win.HGDIOBJ(bmp))
	defer win.SelectObject(dc, old)
	win.BitBlt(dc, 0, 0, int32(r.Dx()), int32(r.Dy()), screen, int32(r.Min.X), int32(r.Min.Y), win.SRCCOPY|win.CAPTUREBLT)
	pix := unsafe.Slice((*byte)(bits), r.Dx()*r.Dy()*4)
	img := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for i := 0; i < len(pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = pix[i+2], pix[i+1], pix[i], 255
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
