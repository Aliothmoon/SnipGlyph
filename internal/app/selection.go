package app

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/tailscale/walk"

	"SnipGlyph/internal/history"
	"SnipGlyph/internal/textgrab"
	"SnipGlyph/internal/toast"
	"SnipGlyph/internal/ui"
	"SnipGlyph/internal/winclip"
)

const grabTimeout = 4 * time.Second

// selectionTranslate translates the text selected in the foreground
// program. Runs on its own goroutine; call input.MaskHotkey first.
func (a *App) selectionTranslate() {
	if !a.grabbing.CompareAndSwap(false, true) {
		return
	}
	defer a.grabbing.Store(false)

	anchor := cursorAnchor()

	ctx, cancel := context.WithTimeout(walk.App().Context(), grabTimeout)
	defer cancel()
	began := time.Now()
	// The copy 划词 may send is not something to keep in clipboard history.
	a.clipPaused.Store(true)
	text, method, err := textgrab.Selected(ctx)
	if method == textgrab.ByCopy {
		a.ownClipboardChange()
	}
	a.clipPaused.Store(false)
	text = strings.TrimSpace(text)
	slog.Info("selection grabbed", "method", method, "runes", len([]rune(text)),
		"took", time.Since(began).Round(time.Millisecond), "err", err)
	switch {
	case errors.Is(err, textgrab.ErrKeysHeld):
		toast.Show("请先松开快捷键再取词", anchor)
		return
	case errors.Is(err, textgrab.ErrNoSelection) || (err == nil && text == ""):
		toast.Show("没有获取到选中的文字", anchor)
		return
	case errors.Is(err, winclip.ErrBusy):
		toast.Show("剪贴板正被其他程序占用，请稍后再试", anchor)
	case errors.Is(err, context.DeadlineExceeded):
		toast.Show("取词超时，请再试一次", anchor)
	case err != nil:
		toast.Show("取词失败："+err.Error(), anchor)
		return
	}

	ui.Do(func() {
		s, err := a.newSession("SnipGlyph · 划词翻译", anchor, history.Selection)
		if err != nil {
			slog.Error("open popup", "err", err)
			return
		}
		s.popup.SetSource(text)
		a.remember(s.rec, text, "", "")
		a.translate(s, text, a.defaultEngine)
	})
}
