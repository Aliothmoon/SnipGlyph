package app

import (
	"context"
	"errors"
	"image"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tailscale/walk"

	"SnipGlyph/internal/history"
	"SnipGlyph/internal/netx"
	"SnipGlyph/internal/ocr"
	"SnipGlyph/internal/translate"
	"SnipGlyph/internal/ui"
)

const translateTimeout = 60 * time.Second

const noEngineMsg = "还没有配置翻译引擎。\n" +
	"请在托盘菜单的「设置」里填写百度翻译的 APP ID 和密钥。"

// session is one popup and its translation requests. UI thread only.
type session struct {
	popup   *ui.Popup
	closed  bool
	fromOCR bool               // source text has OCR line breaks to undo
	rec     *record            // its history entry
	seq     int                // bumped per request; stale results are dropped
	cancel  context.CancelFunc // of the request in flight
}

// buildEngines creates the configured engines and returns problems.
// UI thread (or before the message loop starts).
func (a *App) buildEngines() []string {
	a.engines, a.engineIDs = nil, nil
	t := a.cfg.Translate
	var problems []string
	if t.Baidu.Configured() {
		hc, err := netx.Client(t.Baidu.Proxy)
		if err != nil {
			problems = append(problems, "百度翻译的代理设置有误："+err.Error())
		} else {
			a.engines = append(a.engines, &translate.Baidu{AppID: t.Baidu.AppID, Key: string(t.Baidu.Key), HTTP: hc})
			a.engineIDs = append(a.engineIDs, "baidu")
		}
	}
	a.defaultEngine = max(0, slices.Index(a.engineIDs, t.Engine))
	slog.Info("translation engines", "engines", a.engineIDs, "default", a.defaultEngine)
	return problems
}

// newSession opens a popup for text of the given kind; its translation
// pane opens with the first translation. UI thread.
func (a *App) newSession(title string, anchor image.Rectangle, kind history.Kind) (*session, error) {
	s := &session{
		fromOCR: kind == history.OCR || kind == history.Screenshot,
		rec:     &record{kind: kind},
	}
	names := make([]string, len(a.engines))
	for i, e := range a.engines {
		names[i] = e.Name()
	}
	p, err := ui.NewPopup(ui.PopupOptions{
		Title:       title,
		Anchor:      anchor,
		Engines:     names,
		Engine:      a.defaultEngine,
		OnTranslate: func(text string, engine int) { a.translate(s, text, engine) },
		OnClose: func() {
			s.closed = true
			s.seq++
			if s.cancel != nil {
				s.cancel()
			}
		},
	})
	if err != nil {
		return nil, err
	}
	s.popup = p
	return s, nil
}

// translate starts translating text in s, superseding any request in
// flight. UI thread.
func (a *App) translate(s *session, text string, engine int) {
	if s.closed {
		return
	}
	if len(a.engines) == 0 {
		s.popup.SetTranslationError(noEngineMsg)
		return
	}
	if engine < 0 || engine >= len(a.engines) {
		engine = a.defaultEngine
	}
	eng := a.engines[engine]
	src := text
	if s.fromOCR {
		src = ocr.Unwrap(text)
	}
	// The guess picks the target; the engine detects the source itself,
	// which beats the guess on, say, Japanese written all in kanji.
	_, to := translate.Direction(src, translate.Lang(a.cfg.Translate.TargetLang))
	from := translate.Auto

	if s.cancel != nil {
		s.cancel()
	}
	s.seq++
	seq := s.seq
	ctx, cancel := context.WithTimeout(walk.App().Context(), translateTimeout)
	s.cancel = cancel
	s.popup.ShowTranslating(direction(from, to))

	go func() {
		defer cancel()
		began := time.Now()
		res, err := eng.Translate(ctx, translate.Request{Text: src, From: from, To: to}, func(partial string) {
			walk.App().Synchronize(func() {
				if s.seq == seq {
					s.popup.SetTranslation("", partial)
				}
			})
		})
		slog.Info("translated", "engine", eng.Name(), "runes", utf8.RuneCountInString(src),
			"took", time.Since(began).Round(time.Millisecond), "err", err)
		walk.App().Synchronize(func() {
			if s.seq != seq {
				return // superseded, or the popup closed
			}
			switch {
			case errors.Is(err, context.DeadlineExceeded):
				s.popup.SetTranslationError("翻译超时，请检查网络或代理设置")
			case err != nil:
				s.popup.SetTranslationError("翻译失败：" + err.Error())
			default:
				if res.From != "" && res.From != translate.Auto {
					from = res.From
				}
				s.popup.SetTranslation(direction(from, to), res.Text)
				a.remember(s.rec, text, res.Text, eng.Name())
			}
		})
	}()
}

func direction(from, to translate.Lang) string {
	src := "自动检测"
	if from != "" && from != translate.Auto {
		src = from.Name()
	}
	return src + " → " + to.Name()
}

// inputTranslate opens a popup on the cursor's monitor for typed text,
// prefilled with the clipboard text, selected so typing replaces it. It
// does not translate on its own: the clipboard may hold anything.
func (a *App) inputTranslate() {
	anchor := cursorAnchor()
	ui.Do(func() {
		s, err := a.newSession("SnipGlyph · 输入翻译", anchor, history.Input)
		if err != nil {
			slog.Error("open popup", "err", err)
			a.tray.Warn("输入翻译", "无法打开窗口："+err.Error())
			return
		}
		clip, err := walk.Clipboard().Text()
		if err != nil {
			slog.Warn("read clipboard", "err", err)
		}
		clip = strings.TrimSpace(clip)
		s.popup.SetSource(clip)
		s.popup.SelectSource()
		switch {
		case len(a.engines) == 0:
			s.popup.SetTranslationError(noEngineMsg)
		case clip != "":
			s.popup.SetHint("已填入剪贴板内容，按 Ctrl+Enter 翻译")
		default:
			s.popup.SetHint("输入或粘贴文字，按 Ctrl+Enter 翻译")
		}
	})
}
