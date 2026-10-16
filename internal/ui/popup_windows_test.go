package ui

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"
	"golang.org/x/sys/windows"
)

// The walk message loop must own the main goroutine, so the tests run on
// another goroutine and marshal UI calls with Do. Window tests flash real
// popups and only run with SNIPGLYPH_UI_TESTS set.
func TestMain(m *testing.M) {
	if os.Getenv("SNIPGLYPH_UI_TESTS") == "" {
		os.Exit(m.Run())
	}
	if err := actLikeTheApp(); err != nil {
		panic(err)
	}
	app, err := walk.InitApp()
	if err != nil {
		panic(err)
	}
	code := 0
	go func() {
		code = m.Run()
		app.Exit(code)
	}()
	app.Run()
	os.Exit(code)
}

func requireUITests(t *testing.T) {
	if os.Getenv("SNIPGLYPH_UI_TESTS") == "" {
		t.Skip("set SNIPGLYPH_UI_TESTS=1 to run popup tests")
	}
}

const (
	emGetRect             = 0x00B2
	emGetFirstVisibleLine = 0x00CE
)

func TestPopupLayoutAndFit(t *testing.T) {
	requireUITests(t)
	// Twice: walk disposes a brush when its last window goes, and a brush
	// shared across popups once left every popup after the first unpainted.
	for round := range 2 {
		popupLayoutAndFit(t, round)
	}
}

func popupLayoutAndFit(t *testing.T, round int) {
	var p *Popup
	Do(func() {
		var err error
		p, err = NewPopup(PopupOptions{
			Title:       "SnipGlyph test",
			Anchor:      image.Rect(200, 200, 600, 300),
			Engines:     []string{"test"},
			OnTranslate: func(string, int) {},
		})
		if err != nil {
			t.Fatal(err)
		}
		// The test process may not own the foreground, and an unpinned popup
		// closes as soon as it loses activation.
		p.pin.SetChecked(true)
		p.SetSourceBusy("正在识别…")
	})
	time.Sleep(200 * time.Millisecond)
	Do(func() { p.SetSource("Screenshots often break one sentence\nacross several lines of text.") })
	time.Sleep(300 * time.Millisecond)

	var collapsedH int
	Do(func() {
		h := p.source.Handle()
		var fmtRect win.RECT
		win.SendMessage(h, emGetRect, 0, uintptr(unsafe.Pointer(&fmtRect)))
		if first := win.SendMessage(h, emGetFirstVisibleLine, 0, 0); first != 0 || fmtRect.Top < 0 {
			t.Errorf("first line scrolled out (firstVisible=%d, format top=%d)", first, fmtRect.Top)
		}
		// Hiding children before the form is shown once silently failed: the
		// empty toolbar covered the source box.
		if win.IsWindowVisible(p.mw.ToolBar().Handle()) {
			t.Error("toolbar visible over the content")
		}
		if win.IsWindowVisible(p.trCard.Handle()) {
			t.Error("translation pane open before any translation")
		}
		if !p.copySrc.Enabled() || p.copyRes.Enabled() {
			t.Errorf("copy links: source=%v result=%v", p.copySrc.Enabled(), p.copyRes.Enabled())
		}
		m := newMeasurer(win.HFONT(p.source.SendMessage(win.WM_GETFONT, 0, 0)))
		defer m.close()
		// Two lines of text: the box fits them instead of leaving a blank page.
		if sh := p.source.BoundsPixels().Height; sh < 2*m.lineH || sh > 3*m.lineH {
			t.Errorf("source box %d px for two %d px lines", sh, m.lineH)
		}
		if c := cardPixel(p); c != colorCard {
			t.Errorf("round %d: card painted %#06x, want white", round, uint32(c))
		}
		collapsedH = p.mw.BoundsPixels().Height
		t.Logf("collapsed popup %+v, line %d px", p.mw.BoundsPixels(), m.lineH)

		p.SetTranslation("英语 → 简体中文", "截图通常会将一个句子打断几行文字。")
	})
	time.Sleep(300 * time.Millisecond)
	Do(func() {
		if !win.IsWindowVisible(p.result.Handle()) || !p.copyRes.Enabled() {
			t.Errorf("after SetTranslation: pane=%v copy=%v", win.IsWindowVisible(p.result.Handle()), p.copyRes.Enabled())
		}
		if h := p.mw.BoundsPixels().Height; h <= collapsedH {
			t.Errorf("window did not grow for the translation: %d -> %d", collapsedH, h)
		}
		t.Logf("expanded popup %+v", p.mw.BoundsPixels())
		p.mw.Close()
	})
}

const testManifest = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <dependency><dependentAssembly>
    <assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0"
      processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/>
  </dependentAssembly></dependency>
</assembly>`

// actLikeTheApp gives the test process what the app's embedded manifest
// gives the app: Common Controls v6 (which walk needs) and per-monitor DPI
// awareness.
func actLikeTheApp() error {
	user32 := windows.NewLazySystemDLL("user32.dll")
	user32.NewProc("SetProcessDpiAwarenessContext").Call(^uintptr(3))

	path := filepath.Join(os.TempDir(), "snipglyph-uitest.manifest")
	if err := os.WriteFile(path, []byte(testManifest), 0o644); err != nil {
		return err
	}
	src, _ := windows.UTF16PtrFromString(path)
	actCtx := struct {
		size                  uint32
		flags                 uint32
		source                *uint16
		arch, lang            uint16
		dir, resource, appNam *uint16
		module                uintptr
	}{source: src}
	actCtx.size = uint32(unsafe.Sizeof(actCtx))
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	h, _, err := kernel32.NewProc("CreateActCtxW").Call(uintptr(unsafe.Pointer(&actCtx)))
	if h == uintptr(windows.InvalidHandle) {
		return fmt.Errorf("CreateActCtx: %w", err)
	}
	var cookie uintptr
	if ok, _, err := kernel32.NewProc("ActivateActCtx").Call(h, uintptr(unsafe.Pointer(&cookie))); ok == 0 {
		return fmt.Errorf("ActivateActCtx: %w", err)
	}
	return nil
}

var procPrintWindow = windows.NewLazySystemDLL("user32.dll").NewProc("PrintWindow")

// cardPixel renders the popup and reads a pixel just inside the source
// card's padding.
func cardPixel(p *Popup) walk.Color {
	top, inner := p.mw.Handle(), p.source.Parent().Handle()
	var wr, ir win.RECT
	win.GetWindowRect(top, &wr)
	win.GetWindowRect(inner, &ir)
	w, h := int(wr.Right-wr.Left), int(wr.Bottom-wr.Top)
	dc := win.CreateCompatibleDC(0)
	defer win.DeleteDC(dc)
	bih := win.BITMAPINFOHEADER{BiWidth: int32(w), BiHeight: -int32(h), BiPlanes: 1, BiBitCount: 32}
	bih.BiSize = uint32(unsafe.Sizeof(bih))
	var bits unsafe.Pointer
	bmp := win.CreateDIBSection(dc, &bih, 0, &bits, 0, 0)
	defer win.DeleteObject(win.HGDIOBJ(bmp))
	old := win.SelectObject(dc, win.HGDIOBJ(bmp))
	defer win.SelectObject(dc, old)
	procPrintWindow.Call(uintptr(top), uintptr(dc), 2) // PW_RENDERFULLCONTENT
	x, y := int(ir.Left-wr.Left)+3, int(ir.Top-wr.Top)+3
	px := unsafe.Slice((*byte)(bits), w*h*4)[(y*w+x)*4:]
	return walk.RGB(px[2], px[1], px[0])
}
