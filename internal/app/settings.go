package app

import (
	"context"
	"fmt"
	"image"
	"log/slog"

	"github.com/tailscale/win"

	"SnipGlyph/internal/autostart"
	"SnipGlyph/internal/config"
	"SnipGlyph/internal/history"
	"SnipGlyph/internal/hotkey"
	"SnipGlyph/internal/netx"
	"SnipGlyph/internal/ocr"
	"SnipGlyph/internal/translate"
	"SnipGlyph/internal/ui"
)

// runName is the program's value under the Run key.
const runName = "SnipGlyph"

// openSettings shows the settings window, or brings it forward. UI thread.
func (a *App) openSettings() {
	if a.settingsWin != nil {
		a.settingsWin.Activate()
		return
	}
	on, _, err := autostart.Status(runName)
	if err != nil {
		slog.Warn("read autostart", "err", err)
	}
	w, err := ui.NewSettings(ui.SettingsOptions{
		Config:    a.cfg,
		Autostart: on,
		Save:      a.saveSettings,
		Recording: a.pauseHotkeys,
		TestBaidu: testBaidu,
		OnClose:   func() { a.settingsWin = nil },
	})
	if err != nil {
		slog.Error("open settings", "err", err)
		a.tray.Warn("设置", "无法打开设置窗口："+err.Error())
		return
	}
	a.settingsWin = w
}

// saveSettings checks, stores and applies settings from the settings
// window. UI thread.
func (a *App) saveSettings(cfg config.Config, autostartOn bool) ([]string, error) {
	if err := checkHotkeys(cfg.Hotkeys); err != nil {
		return nil, err
	}
	if _, err := netx.Client(cfg.Translate.Baidu.Proxy); err != nil {
		return nil, fmt.Errorf("百度翻译的代理地址有误：%w", err)
	}
	if err := config.Save(a.opts.ConfigPath, cfg); err != nil {
		return nil, fmt.Errorf("写入配置文件失败：%w", err)
	}
	slog.Info("settings saved")
	a.cfg = cfg
	problems := append(a.registerHotkeys(), a.buildEngines()...)
	if err := a.tray.SetMenu(a.menu()); err != nil {
		slog.Error("rebuild tray menu", "err", err)
	}
	if err := a.setAutostart(autostartOn); err != nil {
		slog.Warn("set autostart", "on", autostartOn, "err", err)
		problems = append(problems, "开机自启设置失败："+err.Error())
	}
	if a.histWin != nil {
		a.histWin.SetRecording(cfg.History.Enabled)
	}
	return problems, nil
}

// checkHotkeys rejects malformed hotkeys and one hotkey used twice.
func checkHotkeys(h config.Hotkeys) error {
	seen := map[hotkey.Hotkey]Action{}
	for _, act := range allActions {
		s := act.hotkey(h)
		if s == "" {
			continue
		}
		hk, err := hotkey.Parse(s)
		if err != nil {
			return fmt.Errorf("「%s」的快捷键 %s 格式错误", act, s)
		}
		if other, dup := seen[hk]; dup {
			return fmt.Errorf("「%s」和「%s」用了同一个快捷键 %s", other, act, hk)
		}
		seen[hk] = act
	}
	return nil
}

// pauseHotkeys releases the hotkeys while the settings window records a
// new one, and takes them back after. UI thread.
func (a *App) pauseHotkeys(pause bool) {
	if pause {
		slog.Info("hotkeys paused for recording")
		for _, act := range allActions {
			_ = a.hk.Unregister(int(act))
		}
		return
	}
	a.registerHotkeys()
}

// setAutostart points the Run entry at this executable, or removes it.
func (a *App) setAutostart(on bool) error {
	enabled, cmd, err := autostart.Status(runName)
	if err != nil {
		return err
	}
	switch {
	case on && (!enabled || !autostart.Points(cmd, a.opts.Exe)):
		slog.Info("autostart enabled", "exe", a.opts.Exe)
		return autostart.Enable(runName, a.opts.Exe)
	case !on && cmd != "":
		slog.Info("autostart disabled")
		return autostart.Disable(runName)
	}
	return nil
}

// testBaidu translates a sample word with credentials from the settings
// window, before they are saved.
func testBaidu(ctx context.Context, b config.Baidu) (string, error) {
	hc, err := netx.Client(b.Proxy)
	if err != nil {
		return "", err
	}
	eng := &translate.Baidu{AppID: b.AppID, Key: string(b.Key), HTTP: hc}
	res, err := eng.Translate(ctx, translate.Request{Text: "hello", From: translate.English, To: translate.ChineseSimplified}, nil)
	return res.Text, err
}

// openHistoryWindow shows the history window, or brings it forward. UI
// thread.
func (a *App) openHistoryWindow() {
	if a.histWin != nil {
		a.histWin.Activate()
		return
	}
	st := a.hist
	if st == nil {
		a.tray.Warn("历史记录", "历史记录不可用，原因见日志")
		return
	}
	w, err := ui.NewHistoryWindow(ui.HistoryOptions{
		Recording: a.cfg.History.Enabled,
		Search: func(ctx context.Context, query string, limit int) ([]history.Entry, int, error) {
			entries, err := st.Search(ctx, query, limit)
			if err != nil {
				return nil, 0, err
			}
			total, err := st.Count(ctx)
			return entries, total, err
		},
		Delete:  st.Delete,
		Clear:   st.Clear,
		Open:    a.openEntry,
		OnClose: func() { a.histWin = nil },
	})
	if err != nil {
		slog.Error("open history window", "err", err)
		a.tray.Warn("历史记录", "无法打开历史记录窗口："+err.Error())
		return
	}
	a.histWin = w
}

// openEntry reopens a saved result in a popup. Edits and new translations
// there update the same entry. UI thread.
func (a *App) openEntry(e history.Entry) {
	var pt win.POINT
	win.GetCursorPos(&pt)
	anchor := image.Rect(int(pt.X), int(pt.Y), int(pt.X)+1, int(pt.Y)+1)
	s, err := a.newSession("SnipGlyph · 历史记录", anchor, e.Kind)
	if err != nil {
		slog.Error("open popup", "err", err)
		return
	}
	s.rec = &record{kind: e.Kind, id: e.ID}
	s.popup.SetSource(e.Source)
	if e.Translation != "" {
		src := e.Source
		if s.fromOCR {
			src = ocr.Unwrap(src)
		}
		s.popup.SetTranslation(direction(translate.Direction(src, translate.Lang(a.cfg.Translate.TargetLang))), e.Translation)
	}
}
