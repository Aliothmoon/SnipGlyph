package app

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"time"

	"github.com/tailscale/walk"

	"SnipGlyph/internal/capture"
	"SnipGlyph/internal/ocr"
	"SnipGlyph/internal/toast"
	"SnipGlyph/internal/ui"
)

const ocrTimeout = 30 * time.Second

// screenshotOCR lets the user pick a region and recognises its text. Silent
// mode copies the text straight to the clipboard and confirms with a toast;
// otherwise a popup centred on the region's screen shows it. Runs on its
// own goroutine.
func (a *App) screenshotOCR(act Action, silent bool) {
	if !a.busy.CompareAndSwap(false, true) {
		return // one screenshot at a time; a second hotkey press is ignored
	}
	defer a.busy.Store(false)

	// Load the engine while the user is still dragging.
	a.ocr.Warm()
	ctx := walk.App().Context()
	shot, err := capture.Select(ctx)
	switch {
	case errors.Is(err, capture.ErrCanceled), errors.Is(err, capture.ErrBusy), errors.Is(err, context.Canceled):
		return
	case err != nil:
		slog.Error("capture", "err", err)
		a.tray.Warn(act.String(), "截图失败："+err.Error())
		return
	}
	b := shot.Bounds
	img := ocr.Image{Width: b.Dx(), Height: b.Dy(), Pix: shot.Pix}
	anchor := image.Rect(b.Left, b.Top, b.Right, b.Bottom)

	translating := act == ScreenshotTranslate
	var s *session
	if !silent {
		ui.Do(func() {
			s, err = a.newSession("SnipGlyph · "+act.String(), anchor, true)
			if err == nil {
				s.popup.SetSourceBusy("正在识别…")
			}
		})
		if err != nil {
			slog.Error("open popup", "err", err)
			a.tray.Warn(act.String(), "无法打开结果窗口："+err.Error())
			return
		}
	}

	began := time.Now()
	rctx, cancel := context.WithTimeout(ctx, ocrTimeout)
	res, err := a.ocr.Recognize(rctx, img)
	cancel()
	slog.Info("ocr done", "size", fmt.Sprintf("%dx%d", b.Dx(), b.Dy()), "lines", len(res.Lines), "took", time.Since(began).Round(time.Millisecond), "err", err)
	text := res.Text()

	if silent {
		// A toast on the selection's screen, not a system notification:
		// lighter, and it does not pile up in the notification centre.
		switch {
		case err != nil:
			toast.Show("识别失败："+err.Error(), anchor)
		case text == "":
			toast.Show("没有识别到文字", anchor)
		default:
			ui.Do(func() { err = walk.Clipboard().SetText(text) })
			if err != nil {
				toast.Show("复制到剪贴板失败："+err.Error(), anchor)
				return
			}
			toast.Show(fmt.Sprintf("已复制 %d 行文字", len(res.Lines)), anchor)
		}
		return
	}

	ui.Do(func() {
		switch {
		case err != nil:
			s.popup.SetSourceError("识别失败：" + err.Error())
		case text == "":
			s.popup.SetSourceError("没有识别到文字")
		default:
			s.popup.SetSource(text)
			if translating {
				a.translate(s, text, a.defaultEngine)
			}
		}
	})
}
