package app

import (
	"context"
	"errors"
	"image"
	"log/slog"
	"strings"
	"time"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"

	"SnipGlyph/internal/history"
	"SnipGlyph/internal/textgrab"
	"SnipGlyph/internal/toast"
	"SnipGlyph/internal/ui"
)

const grabTimeout = 4 * time.Second

// selectionTranslate translates the text selected in the foreground
// program. Runs on its own goroutine; call textgrab.MaskHotkey first.
func (a *App) selectionTranslate() {
	if !a.grabbing.CompareAndSwap(false, true) {
		return
	}
	defer a.grabbing.Store(false)

	var pt win.POINT
	win.GetCursorPos(&pt)
	anchor := image.Rect(int(pt.X), int(pt.Y), int(pt.X)+1, int(pt.Y)+1)

	ctx, cancel := context.WithTimeout(walk.App().Context(), grabTimeout)
	defer cancel()
	began := time.Now()
	text, method, err := textgrab.Selected(ctx)
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
