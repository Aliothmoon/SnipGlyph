// Package app wires hotkeys, the tray and (later) the OCR / translation
// pipelines together.
package app

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tailscale/walk"
	"golang.org/x/sys/windows"

	"SnipGlyph/internal/config"
	"SnipGlyph/internal/history"
	"SnipGlyph/internal/hotkey"
	"SnipGlyph/internal/ocr"
	"SnipGlyph/internal/textgrab"
	"SnipGlyph/internal/translate"
	"SnipGlyph/internal/ui"
)

// Action is something the user can trigger by hotkey or tray menu. Its value
// doubles as the hotkey id.
type Action int

const (
	ScreenshotOCR Action = iota + 1
	SilentOCR
	ScreenshotTranslate
	SelectionTranslate
	InputTranslate
)

var allActions = []Action{ScreenshotOCR, SilentOCR, ScreenshotTranslate, SelectionTranslate, InputTranslate}

func (a Action) String() string {
	switch a {
	case ScreenshotOCR:
		return "截图 OCR"
	case SilentOCR:
		return "静默 OCR"
	case ScreenshotTranslate:
		return "截图翻译"
	case SelectionTranslate:
		return "划词翻译"
	case InputTranslate:
		return "输入翻译"
	}
	return fmt.Sprintf("Action(%d)", int(a))
}

func (a Action) hotkey(h config.Hotkeys) string {
	switch a {
	case ScreenshotOCR:
		return h.ScreenshotOCR
	case SilentOCR:
		return h.SilentOCR
	case ScreenshotTranslate:
		return h.ScreenshotTranslate
	case SelectionTranslate:
		return h.SelectionTranslate
	case InputTranslate:
		return h.InputTranslate
	}
	return ""
}

type Options struct {
	ConfigPath string
	ConfigDir  string
	LogDir     string
	// HistoryPath is the history database.
	HistoryPath string
	// Exe is this program, for the autostart entry.
	Exe string
	// OCRWorker returns the command for an OCR worker process: one that
	// calls ocr.ServeWorker on its stdin/stdout.
	OCRWorker func() *exec.Cmd
}

// ocrIdle is how long an unused OCR worker (~150 MB) stays alive.
const ocrIdle = 10 * time.Minute

type App struct {
	opts  Options
	cfg   config.Config
	hk    *hotkey.Manager
	tray  *ui.Tray
	bound map[Action]hotkey.Hotkey
	ocr   *ocr.Client
	busy  atomic.Bool // a screenshot flow is running

	grabbing atomic.Bool // a selection is being read

	// Translation engines from the config, rebuilt on reload. UI thread only.
	engines       []translate.Engine
	engineIDs     []string
	defaultEngine int

	hist     *history.Store // nil when the database could not be opened
	histq    chan func()    // writes, run in order by one goroutine
	histDone chan struct{}

	// Open windows, nil when closed. UI thread only.
	settingsWin *ui.Settings
	histWin     *ui.HistoryWindow
}

// Run starts the tray app and blocks until the user exits. It must be called
// from the main goroutine.
func Run(opts Options) error {
	walkApp, err := walk.InitApp()
	if err != nil {
		return fmt.Errorf("init UI: %w", err)
	}
	walkApp.SetProductName("SnipGlyph")

	a := &App{opts: opts, ocr: ocr.NewClient(opts.OCRWorker, ocrIdle)}
	defer a.ocr.Close()
	var problems []string
	if err := a.loadConfig(); err != nil {
		problems = append(problems, "配置文件有误，已使用默认设置："+err.Error())
	}

	if a.hk, err = hotkey.NewManager(); err != nil {
		return fmt.Errorf("start hotkey manager: %w", err)
	}
	defer a.hk.Close()

	problems = append(problems, a.registerHotkeys()...)
	problems = append(problems, a.buildEngines()...)
	if p := a.openHistory(); p != "" {
		problems = append(problems, p)
	}
	defer a.closeHistory()
	if a.tray, err = ui.NewTray("SnipGlyph", a.menu()); err != nil {
		return fmt.Errorf("create tray icon: %w", err)
	}
	defer a.tray.Dispose()

	if inst, err := ocr.Locate(); err != nil {
		problems = append(problems, "OCR 不可用："+err.Error())
	} else {
		slog.Info("snipping tool found", "version", inst.Version, "dir", inst.Dir)
	}
	if len(problems) > 0 {
		a.tray.Warn("SnipGlyph 启动时发现问题", strings.Join(problems, "\n"))
	}

	go func() {
		for id := range a.hk.Events() {
			a.trigger(Action(id))
		}
	}()

	slog.Info("ready")
	walkApp.Run()
	slog.Info("exiting")
	return nil
}

// loadConfig reads the settings file, writing the defaults on first run.
func (a *App) loadConfig() error {
	cfg, err := config.Load(a.opts.ConfigPath)
	a.cfg = cfg
	if err != nil {
		slog.Error("load config", "err", err)
		return err
	}
	if saveErr := config.Save(a.opts.ConfigPath, cfg); saveErr != nil {
		slog.Warn("save config", "err", saveErr)
	}
	return nil
}

// registerHotkeys (re)binds every action and returns human-readable problems.
func (a *App) registerHotkeys() []string {
	a.bound = map[Action]hotkey.Hotkey{}
	// Release them all first, so two actions can swap hotkeys.
	for _, act := range allActions {
		_ = a.hk.Unregister(int(act))
	}
	var problems []string
	for _, act := range allActions {
		s := act.hotkey(a.cfg.Hotkeys)
		if s == "" {
			continue
		}
		hk, err := hotkey.Parse(s)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s（%s）：格式错误", act, s))
			slog.Warn("bad hotkey", "action", act, "hotkey", s, "err", err)
			continue
		}
		if err := a.hk.Register(int(act), hk); err != nil {
			reason := "注册失败"
			if hotkey.IsTaken(err) {
				reason = "已被其他程序占用"
			}
			problems = append(problems, fmt.Sprintf("%s（%s）：%s", act, hk, reason))
			slog.Warn("register hotkey", "action", act, "hotkey", hk, "err", err)
			continue
		}
		a.bound[act] = hk
		slog.Info("hotkey registered", "action", act, "hotkey", hk)
	}
	return problems
}

func (a *App) menu() []ui.MenuItem {
	item := func(act Action) ui.MenuItem {
		it := ui.MenuItem{Text: act.String(), OnClick: func() { a.trigger(act) }}
		if hk, ok := a.bound[act]; ok {
			it.Shortcut = hk.String()
		}
		return it
	}
	return []ui.MenuItem{
		item(ScreenshotOCR),
		item(SilentOCR),
		item(ScreenshotTranslate),
		item(InputTranslate),
		{},
		{Text: "历史记录", OnClick: a.openHistoryWindow},
		{Text: "设置", OnClick: a.openSettings},
		{},
		{Text: "重新加载配置", OnClick: a.reload},
		{Text: "打开配置目录", OnClick: func() { openPath(a.opts.ConfigDir) }},
		{Text: "打开日志目录", OnClick: func() { openPath(a.opts.LogDir) }},
		{},
		{Text: "退出", OnClick: func() { walk.App().Exit(0) }},
	}
}

// reload re-reads the settings file, rebinds hotkeys and rebuilds the
// translation engines. UI thread.
func (a *App) reload() {
	cfgErr := a.loadConfig()
	problems := append(a.registerHotkeys(), a.buildEngines()...)
	if err := a.tray.SetMenu(a.menu()); err != nil {
		slog.Error("rebuild tray menu", "err", err)
	}
	if a.histWin != nil {
		a.histWin.SetRecording(a.cfg.History.Enabled)
	}
	switch {
	case cfgErr != nil:
		a.tray.Warn("配置文件有误，已使用默认设置", cfgErr.Error())
	case len(problems) > 0:
		a.tray.Warn("配置中有问题", strings.Join(problems, "\n"))
	default:
		a.tray.Info("配置已重新加载", "")
	}
}

// trigger starts an action without waiting for it. Safe to call from any
// goroutine, including the UI thread.
func (a *App) trigger(act Action) {
	slog.Info("action triggered", "action", act)
	switch act {
	case ScreenshotOCR, ScreenshotTranslate:
		go a.screenshotOCR(act, false)
	case SilentOCR:
		go a.screenshotOCR(act, true)
	case SelectionTranslate:
		// Right away, while the hotkey's modifiers are still down.
		textgrab.MaskHotkey()
		go a.selectionTranslate()
	case InputTranslate:
		go a.inputTranslate()
	default:
		a.tray.Info(act.String(), "功能开发中")
	}
}

func openPath(path string) {
	verb, _ := windows.UTF16PtrFromString("open")
	p, err := windows.UTF16PtrFromString(path)
	if err == nil {
		err = windows.ShellExecute(0, verb, p, nil, nil, windows.SW_SHOWNORMAL)
	}
	if err != nil {
		slog.Warn("open path", "path", path, "err", err)
	}
}
