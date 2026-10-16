package app

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"

	"SnipGlyph/internal/cliphist"
	"SnipGlyph/internal/input"
	"SnipGlyph/internal/toast"
	"SnipGlyph/internal/ui"
	"SnipGlyph/internal/winclip"
)

// clipboard is the clipboard history part of App.
type clipboard struct {
	clip      *cliphist.Store    // nil when the database could not be opened
	clipStop  context.CancelFunc // stops the watcher; nil when not watching
	clipPanel *ui.ClipPanel      // open panel, UI thread only
	clipPrev  win.HWND           // the window in front before the panel opened

	// clipPaused is set while 划词 sends its own Ctrl+C; clipIgnore is the
	// last clipboard sequence number that SnipGlyph itself caused.
	clipPaused atomic.Bool
	clipIgnore atomic.Uint32
}

// openClipboard opens the database and starts recording if enabled. It
// returns a problem to report, or "".
func (a *App) openClipboard() string {
	st, err := cliphist.Open(a.opts.ClipboardPath, a.cfg.Clipboard.Limit)
	if err != nil {
		slog.Error("open clipboard history", "path", a.opts.ClipboardPath, "err", err)
		return "剪贴板历史不可用：" + err.Error()
	}
	a.clip = st
	return a.applyClipboard()
}

// applyClipboard starts or stops recording to match the config. UI thread.
func (a *App) applyClipboard() string {
	if a.clip == nil {
		return ""
	}
	a.clip.SetLimit(a.cfg.Clipboard.Limit)
	on := a.cfg.Clipboard.Enabled
	switch {
	case on && a.clipStop == nil:
		ctx, stop := context.WithCancel(context.Background())
		if err := winclip.Watch(ctx, a.clipChanged); err != nil {
			stop()
			slog.Error("watch clipboard", "err", err)
			return "无法监听剪贴板：" + err.Error()
		}
		a.clipStop = stop
		slog.Info("recording the clipboard")
	case !on && a.clipStop != nil:
		a.clipStop()
		a.clipStop = nil
		slog.Info("stopped recording the clipboard")
	}
	return ""
}

func (a *App) closeClipboard() {
	if a.clipStop != nil {
		a.clipStop()
	}
	if a.clip != nil {
		a.clip.Close()
	}
}

// clipChanged records the new clipboard content. It runs on the watcher's
// goroutine, one change at a time.
func (a *App) clipChanged() {
	// Some programs open and close the clipboard several times while
	// copying; read once they are done.
	time.Sleep(40 * time.Millisecond)
	if a.clipPaused.Load() || winclip.Sequence() <= a.clipIgnore.Load() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	began := time.Now()
	snap, err := cliphist.Read(ctx)
	if err != nil {
		slog.Warn("read clipboard", "err", err)
		return
	}
	it, err := snap.Item()
	switch {
	case errors.Is(err, cliphist.ErrSkip):
		if snap.Private {
			slog.Debug("clipboard content marked private, not recorded")
		}
		return
	case err != nil:
		slog.Warn("clipboard item", "err", err)
		return
	}
	if _, err := a.clip.Record(ctx, it); err != nil {
		slog.Warn("record clipboard", "err", err)
		return
	}
	slog.Debug("clipboard recorded", "kind", it.Kind, "source", it.Source, "took", time.Since(began).Round(time.Millisecond))
	walk.App().Synchronize(func() {
		if a.clipPanel != nil {
			a.clipPanel.Refresh()
		}
	})
}

// ownClipboardChange marks the clipboard's current content as SnipGlyph's
// own doing, so the recorder leaves it alone.
func (a *App) ownClipboardChange() { a.clipIgnore.Store(winclip.Sequence()) }

// toggleClipPanel opens the clipboard panel, or closes it if open. UI
// thread.
func (a *App) toggleClipPanel() {
	if a.clipPanel != nil {
		a.clipPanel.Close()
		return
	}
	if a.clip == nil {
		a.tray.Warn("剪贴板历史", "剪贴板历史不可用，原因见日志")
		return
	}
	a.clipPrev = win.GetForegroundWindow()
	p, err := ui.NewClipPanel(ui.ClipPanelOptions{
		Store:     a.clip,
		Paste:     a.pasteClip,
		Copy:      func(id int64) { a.putClip(id, false, nil) },
		Recording: a.cfg.Clipboard.Enabled,
		OnClose:   func() { a.clipPanel = nil },
	})
	if err != nil {
		slog.Error("open clipboard panel", "err", err)
		a.tray.Warn("剪贴板历史", "无法打开面板："+err.Error())
		return
	}
	a.clipPanel = p
}

// pasteClip pastes an item into the window that was in front of the panel.
// UI thread.
func (a *App) pasteClip(id int64, plain bool) {
	prev := a.clipPrev
	a.putClip(id, plain, func() {
		if prev == 0 {
			return
		}
		win.SetForegroundWindow(prev)
		for deadline := time.Now().Add(500 * time.Millisecond); win.GetForegroundWindow() != prev && time.Now().Before(deadline); {
			time.Sleep(10 * time.Millisecond)
		}
		// Shift may still be down from Shift+Enter.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := input.WaitReleased(ctx, 2*time.Second); err != nil {
			slog.Warn("paste: keys held", "err", err)
			return
		}
		if err := input.Ctrl('V'); err != nil {
			slog.Warn("paste", "err", err)
		}
	})
}

// putClip puts an item on the clipboard, moves it to the top of its tab and
// then runs after, all off the UI thread. UI thread.
func (a *App) putClip(id int64, plain bool, after func()) {
	st := a.clip
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := func() error {
			it, err := st.Get(ctx, id)
			if err != nil {
				return err
			}
			formats, err := cliphist.Formats(it, plain)
			if err != nil {
				return err
			}
			// Not recorded again: the item is already there, and a plain
			// paste would strip its formatting.
			a.clipPaused.Store(true)
			defer a.clipPaused.Store(false)
			if err := winclip.Write(ctx, formats, winclip.WriteOptions{}); err != nil {
				return err
			}
			a.ownClipboardChange()
			return st.Touch(ctx, id)
		}()
		if err != nil {
			slog.Warn("put clipboard item", "id", id, "err", err)
			toast.Show("无法放入剪贴板："+err.Error(), cursorAnchor())
			return
		}
		if after != nil {
			after()
		}
	}()
}
