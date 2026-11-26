package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/tailscale/walk"

	"SnipGlyph/internal/history"
	"SnipGlyph/internal/i18n"
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
	// The copy selection-grab may send is not something to keep in clipboard history.
	a.clipPaused.Store(true)
	text, method, err := textgrab.Selected(ctx)
	if method == textgrab.ByCopy {
		a.ownClipboardChange()
	}
	a.clipPaused.Store(false)
	text = strings.TrimSpace(text)
	slog.Info("selection grabbed", "method", method, "runes", len([]rune(text)),
		"took", time.Since(began).Round(time.Millisecond), "err", err)
	tr := i18n.Current()
	switch {
	case errors.Is(err, textgrab.ErrKeysHeld):
		toast.Show(tr.GrabReleaseKeys, anchor)
		return
	case errors.Is(err, textgrab.ErrNoSelection) || (err == nil && text == ""):
		toast.Show(tr.GrabNoSelection, anchor)
		return
	case errors.Is(err, winclip.ErrBusy):
		toast.Show(tr.GrabClipboardBusy, anchor)
		return
	case errors.Is(err, context.DeadlineExceeded):
		toast.Show(tr.GrabTimeout, anchor)
		return
	case err != nil:
		toast.Show(fmt.Sprintf(tr.GrabFailed, err.Error()), anchor)
		return
	}

	ui.Do(func() {
		s, err := a.newSession("SnipGlyph · "+SelectionTranslate.String(), anchor, history.Selection)
		if err != nil {
			slog.Error("open popup", "err", err)
			return
		}
		s.popup.SetSource(text)
		a.remember(s.rec, text, "", "")
		a.translate(s, text, a.defaultEngine)
	})
}
