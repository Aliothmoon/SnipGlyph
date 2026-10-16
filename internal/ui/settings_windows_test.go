package ui

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tailscale/win"

	"SnipGlyph/internal/config"
	"SnipGlyph/internal/history"
	"SnipGlyph/internal/hotkey"
)

// waitFor polls cond on the UI thread until it holds.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var ok bool
		Do(func() { ok = cond() })
		if ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func waitClosed(t *testing.T, closed <-chan struct{}) {
	t.Helper()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("window did not close")
	}
}

// fitsInClient reports whether w's bottom edge lies inside the form's
// client area, i.e. the window was sized to show all of its content.
func fitsInClient(form, w win.HWND) bool {
	var cr, wr win.RECT
	win.GetClientRect(form, &cr)
	win.GetWindowRect(w, &wr)
	pt := win.POINT{X: wr.Right, Y: wr.Bottom}
	win.ScreenToClient(form, &pt)
	return pt.Y <= cr.Bottom && pt.X <= cr.Right
}

func TestSettingsWindow(t *testing.T) {
	requireUITests(t)
	cfg := config.Default()
	cfg.Translate.Baidu = config.Baidu{AppID: "id", Key: "secret", Proxy: "socks5://127.0.0.1:1080"}

	var (
		s         *Settings
		recording []bool
		saved     *config.Config
		savedAuto bool
		closed    = make(chan struct{})
	)
	Do(func() {
		var err error
		s, err = NewSettings(SettingsOptions{
			Config:    cfg,
			Autostart: true,
			Save: func(c config.Config, auto bool) ([]string, error) {
				saved, savedAuto = &c, auto
				return nil, nil
			},
			Recording: func(on bool) { recording = append(recording, on) },
			TestBaidu: func(context.Context, config.Baidu) (string, error) { return "你好", nil },
			OnClose:   func() { close(closed) },
		})
		if err != nil {
			t.Fatal(err)
		}
		if !s.autostart.Checked() || !s.history.Checked() {
			t.Errorf("checkboxes: autostart=%v history=%v", s.autostart.Checked(), s.history.Checked())
		}
		if got := s.boxes[0].le.Text(); got != "Win+Alt+Shift+C" {
			t.Errorf("first hotkey box shows %q", got)
		}
		if s.proxyMode.CurrentIndex() != proxyCustom || s.proxyURL.Text() != "socks5://127.0.0.1:1080" || !s.proxyURL.Enabled() {
			t.Errorf("proxy: mode %d, url %q", s.proxyMode.CurrentIndex(), s.proxyURL.Text())
		}
		if s.baiduKey.Text() != "secret" {
			t.Errorf("key box %q", s.baiduKey.Text())
		}
		if win.IsWindowVisible(s.mw.ToolBar().Handle()) {
			t.Error("toolbar visible over the content")
		}
		if !fitsInClient(s.mw.Handle(), s.saveBt.Handle()) {
			t.Errorf("save button cut off: window %+v", s.mw.BoundsPixels())
		}
		// Measured before it was shown, the window once came out a few
		// pixels tall, was centred at that size and then grew downwards off
		// the screen.
		b := s.mw.BoundsPixels()
		work, _ := cursorMonitor()
		if b.Y < work.Min.Y || b.Y+b.Height > work.Max.Y {
			t.Errorf("window %+v spills out of the work area %v", b, work)
		} else if top, bottom := b.Y-work.Min.Y, work.Max.Y-(b.Y+b.Height); top-bottom > 2 || bottom-top > 2 {
			t.Errorf("window %+v not centred in %v", b, work)
		}
		t.Logf("settings window %+v", b)

		s.boxes[2].le.SetFocus()
	})

	waitFor(t, "recording to start", func() bool { return len(recording) > 0 })
	Do(func() {
		b := s.boxes[2]
		press := func(mods hotkey.Modifiers, vk uintptr) {
			b.modifiers = func() hotkey.Modifiers { return mods }
			b.handle(&win.MSG{HWnd: b.le.Handle(), Message: win.WM_SYSKEYDOWN, WParam: vk})
		}
		press(hotkey.ModWin|hotkey.ModAlt, win.VK_MENU)
		if got := b.le.Text(); got != "Win+Alt+…" {
			t.Errorf("preview %q", got)
		}
		press(hotkey.ModWin|hotkey.ModAlt, 'Q')
		if b.value != "Win+Alt+Q" {
			t.Errorf("recorded %q", b.value)
		}
		press(hotkey.ModShift, 'A') // just a capital A
		if b.value != "Win+Alt+Q" || s.keyHint.Text() == defaultKeyHint {
			t.Errorf("Shift+A: value %q, hint %q", b.value, s.keyHint.Text())
		}
		c := s.boxes[3]
		c.modifiers = func() hotkey.Modifiers { return 0 }
		c.handle(&win.MSG{HWnd: c.le.Handle(), Message: win.WM_KEYDOWN, WParam: win.VK_BACK})
		if c.value != "" {
			t.Errorf("Backspace left %q", c.value)
		}
		s.saveBt.SetFocus()
	})
	waitFor(t, "recording to stop", func() bool { return len(recording) > 1 })

	Do(func() {
		if !slices.Equal(recording, []bool{true, false}) {
			t.Errorf("recording calls %v", recording)
		}
		// A custom proxy without an address is refused, and the message is
		// shown in full.
		before := s.mw.BoundsPixels().Height
		s.proxyURL.SetText("")
		s.save()
		if saved != nil {
			t.Error("saved with an empty proxy address")
		}
		if !win.IsWindowVisible(s.status.Handle()) || s.mw.BoundsPixels().Height <= before {
			t.Errorf("status shown=%v, height %d -> %d", win.IsWindowVisible(s.status.Handle()), before, s.mw.BoundsPixels().Height)
		}
		if !fitsInClient(s.mw.Handle(), s.saveBt.Handle()) {
			t.Error("save button cut off after the status message")
		}
		s.proxyMode.SetCurrentIndex(proxyDirect)
		s.testBaidu()
	})
	waitFor(t, "the test result", func() bool { return strings.Contains(s.testMsg.Text(), "你好") })
	Do(s.save)
	waitClosed(t, closed)

	if saved == nil {
		t.Fatal("not saved")
	}
	if saved.Hotkeys.ScreenshotTranslate != "Win+Alt+Q" || saved.Hotkeys.SelectionTranslate != "" || saved.Hotkeys.SilentOCR != "Win+Alt+C" {
		t.Errorf("hotkeys %+v", saved.Hotkeys)
	}
	if b := saved.Translate.Baidu; b.Proxy != "direct" || b.Key != "secret" || b.AppID != "id" {
		t.Errorf("baidu %+v", b)
	}
	if !savedAuto || !saved.History.Enabled || saved.Translate.TargetLang != "zh-Hans" {
		t.Errorf("autostart %v, history %v, target %q", savedAuto, saved.History.Enabled, saved.Translate.TargetLang)
	}
}

func TestHistoryWindow(t *testing.T) {
	requireUITests(t)
	st, err := history.Open(filepath.Join(t.TempDir(), "h.db"), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	for _, src := range []string{"first apple", "second banana", "third cherry\nwith a second line"} {
		if _, err := st.Add(ctx, history.Entry{Kind: history.Selection, Source: src, Translation: "译文", Engine: "百度翻译"}); err != nil {
			t.Fatal(err)
		}
	}

	var (
		h      *HistoryWindow
		opened []history.Entry
		closed = make(chan struct{})
	)
	Do(func() {
		h, err = NewHistoryWindow(HistoryOptions{
			Recording: true,
			Search: func(ctx context.Context, q string, max int) ([]history.Entry, int, error) {
				es, err := st.Search(ctx, q, max)
				if err != nil {
					return nil, 0, err
				}
				n, err := st.Count(ctx)
				return es, n, err
			},
			Delete:  st.Delete,
			Clear:   st.Clear,
			Open:    func(e history.Entry) { opened = append(opened, e) },
			OnClose: func() { close(closed) },
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	rows := func(n int) func() bool { return func() bool { return h.model.RowCount() == n } }
	waitFor(t, "three rows", rows(3))
	Do(func() {
		if got := h.model.Value(0, 2); got != "third cherry with a second line" {
			t.Errorf("newest row preview %q", got)
		}
		if e, ok := h.current(); !ok || !strings.HasPrefix(e.Source, "third") {
			t.Errorf("selected %+v", e)
		}
		if !strings.Contains(h.source.Text(), "with a second line") || !strings.Contains(h.info.Text(), "划词翻译") {
			t.Errorf("detail %q / %q", h.source.Text(), h.info.Text())
		}
		if got := h.count.Text(); got != "共 3 条" {
			t.Errorf("count %q", got)
		}
		h.search.SetText("banana")
	})
	waitFor(t, "the search", rows(1))
	Do(func() {
		h.openCurrent()
		if len(opened) != 1 || opened[0].Source != "second banana" {
			t.Errorf("opened %+v", opened)
		}
		h.deleteCurrent()
	})
	waitFor(t, "the deletion", rows(0))
	Do(func() { h.search.SetText("") })
	waitFor(t, "the rest", rows(2))
	Do(func() {
		if got := h.count.Text(); got != "共 2 条" {
			t.Errorf("count after delete %q", got)
		}
		h.mw.Close()
	})
	waitClosed(t, closed)
}
