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

func TestPopupLayout(t *testing.T) {
	requireUITests(t)
	for _, expanded := range []bool{false, true} {
		var p *Popup
		Do(func() {
			var err error
			p, err = NewPopup(PopupOptions{
				Title:       "SnipGlyph test",
				Anchor:      image.Rect(200, 200, 600, 300),
				Translation: expanded,
				Engines:     []string{"test"},
				OnTranslate: func(string, int) {},
			})
			if err != nil {
				t.Fatal(err)
			}
			p.SetSourceBusy("正在识别…")
		})
		time.Sleep(200 * time.Millisecond)
		Do(func() { p.SetSource("Screenshots often break one sentence\nacross several lines of text.") })
		time.Sleep(300 * time.Millisecond)
		Do(func() {
			h := p.source.Handle()
			var fmtRect, client win.RECT
			win.SendMessage(h, emGetRect, 0, uintptr(unsafe.Pointer(&fmtRect)))
			win.GetClientRect(h, &client)
			first := win.SendMessage(h, emGetFirstVisibleLine, 0, 0)
			if first != 0 || fmtRect.Top < 0 {
				t.Errorf("expanded=%v: first line scrolled out (firstVisible=%d, format top=%d)", expanded, first, fmtRect.Top)
			}
			// Hiding children before the form is shown once silently failed:
			// the empty toolbar covered the source box and the "collapsed"
			// translation pane took half the window.
			if win.IsWindowVisible(p.mw.ToolBar().Handle()) {
				t.Errorf("expanded=%v: toolbar visible over the content", expanded)
			}
			if got := win.IsWindowVisible(p.result.Handle()); got != expanded {
				t.Errorf("expanded=%v: translation pane visible=%v", expanded, got)
			}
			if sb := p.source.BoundsPixels(); sb.Y < 0 || (!expanded && sb.Height*2 < p.mw.ClientBoundsPixels().Height) {
				t.Errorf("expanded=%v: source box bounds %+v", expanded, sb)
			}
			if !p.copySrc.Enabled() || p.copyRes.Enabled() {
				t.Errorf("expanded=%v: copy buttons source=%v result=%v", expanded, p.copySrc.Enabled(), p.copyRes.Enabled())
			}
			p.SetTranslation("英语 → 简体中文", "译文")
			if !p.copyRes.Enabled() || !win.IsWindowVisible(p.result.Handle()) {
				t.Errorf("expanded=%v: after SetTranslation copy=%v pane=%v", expanded, p.copyRes.Enabled(), win.IsWindowVisible(p.result.Handle()))
			}
			p.mw.Close()
		})
	}
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
