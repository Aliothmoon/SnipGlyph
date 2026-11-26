// Package app wires hotkeys, the tray and the OCR / translation pipelines
// together.
package app

import (
	"errors"
	"fmt"
	"image"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"
	"golang.org/x/sys/windows"

	"SnipGlyph/internal/config"
	"SnipGlyph/internal/history"
	"SnipGlyph/internal/hotkey"
	"SnipGlyph/internal/i18n"
	"SnipGlyph/internal/input"
	"SnipGlyph/internal/ocr"
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
	ClipboardHistory
)

var allActions = []Action{ScreenshotOCR, SilentOCR, ScreenshotTranslate, SelectionTranslate, InputTranslate, ClipboardHistory}

func (a Action) String() string {
	tr := i18n.Current()
	switch a {
	case ScreenshotOCR:
		return tr.ActionScreenshotOCR
	case SilentOCR:
		return tr.ActionSilentOCR
	case ScreenshotTranslate:
		return tr.ActionScreenshotTranslate
	case SelectionTranslate:
		return tr.ActionSelectionTranslate
	case InputTranslate:
		return tr.ActionInputTranslate
	case ClipboardHistory:
		return tr.ActionClipboardHistory
	}
	return fmt.Sprintf("Action(%d)", int(a))
}

func (a Action) logName() string {
	switch a {
	case ScreenshotOCR:
		return "screenshot_ocr"
	case SilentOCR:
		return "silent_ocr"
	case ScreenshotTranslate:
		return "screenshot_translate"
	case SelectionTranslate:
		return "selection_translate"
	case InputTranslate:
		return "input_translate"
	case ClipboardHistory:
		return "clipboard_history"
	}
	return fmt.Sprintf("action_%d", int(a))
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
	case ClipboardHistory:
		return h.ClipboardHistory
	}
	return ""
}

type Options struct {
	Portable   bool
	ConfigPath string
	ConfigDir  string
	LogDir     string
	// HistoryPath is the translation history database, ClipboardPath the
	// clipboard history database.
	HistoryPath   string
	ClipboardPath string
	// Exe is this program, for the autostart entry and local OneOCR lookup.
	Exe string
	// AutostartArgs are extra flags preserved in the Windows Run command.
	AutostartArgs []string
	// OCRWorker returns the command for an OCR worker process: one that
	// calls ocr.ServeWorkerWith on its stdin/stdout.
	OCRWorker func(customDir string) *exec.Cmd
}

// ocrIdle is how long an unused OCR worker (~150 MB) stays alive.
const ocrIdle = 10 * time.Minute

type App struct {
	opts   Options
	cfg    config.Config
	hk     *hotkey.Manager
	tray   *ui.Tray
	bound  map[Action]hotkey.Hotkey
	ocr    *ocr.Client
	ocrDir atomic.Value // string; custom OneOCR dir read off UI thread
	busy   atomic.Bool  // a screenshot flow is running

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

	clipboard
}

func (a *App) currentOCRDir() string {
	if s, ok := a.ocrDir.Load().(string); ok {
		return s
	}
	return ""
}

// Run starts the tray app and blocks until the user exits. It must be called
// from the main goroutine.
func Run(opts Options) error {
	walkApp, err := walk.InitApp()
	if err != nil {
		return fmt.Errorf("init UI: %w", err)
	}
	walkApp.SetProductName("SnipGlyph")

	a := &App{opts: opts}
	a.ocrDir.Store("")
	a.ocr = ocr.NewClient(func() *exec.Cmd { return opts.OCRWorker(a.currentOCRDir()) }, ocrIdle)
	defer a.ocr.Close()
	var problems []string
	if err := a.loadConfig(); err != nil {
		if errors.Is(err, config.ErrSecretUnreadable) {
			problems = append(problems, i18n.Current().TraySecretUnreadable)
		} else {
			problems = append(problems, fmt.Sprintf(i18n.Current().TrayConfigError, err.Error()))
		}
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
	if p := a.openClipboard(); p != "" {
		problems = append(problems, p)
	}
	defer a.closeClipboard()
	if a.tray, err = ui.NewTray("SnipGlyph", a.menu()); err != nil {
		return fmt.Errorf("create tray icon: %w", err)
	}
	defer a.tray.Dispose()

	if inst, err := ocr.LocateWith(ocr.LocateOptions{
		ExeDir:    filepath.Dir(opts.Exe),
		CustomDir: a.cfg.OCR.Dir,
	}); err != nil {
		problems = append(problems, fmt.Sprintf(i18n.Current().TrayOCRUnavailable, formatOCRError(err)))
	} else {
		slog.Info("oneocr located", "source", inst.Source, "version", inst.Version, "dir", inst.Dir)
	}
	if len(problems) > 0 {
		a.tray.Warn(i18n.Current().TrayStartupProblemsTitle, strings.Join(problems, "\n"))
	}

	go func() {
		for id := range a.hk.Events() {
			a.trigger(Action(id))
		}
	}()
	// Made ahead, once the message loop runs, so the hotkey only shows it.
	walkApp.Synchronize(func() {
		if err := a.makeClipPanel(); err != nil {
			slog.Error("make clipboard panel", "err", err)
		}
	})

	slog.Info("ready", "lang", i18n.Active(), "portable", opts.Portable)
	walkApp.Run()
	slog.Info("exiting")
	return nil
}

// loadConfig reads the settings file, writing the defaults on first run.
func (a *App) loadConfig() error {
	_, statErr := os.Stat(a.opts.ConfigPath)
	firstRun := errors.Is(statErr, fs.ErrNotExist)

	cfg, err := config.Load(a.opts.ConfigPath)
	if firstRun && err == nil && i18n.SystemLang() != i18n.ZhCN {
		cfg.Translate.TargetLang = string(translate.English)
	}
	a.cfg = cfg
	i18n.Set(i18n.Parse(cfg.Language))
	a.ocrDir.Store(cfg.OCR.Dir)

	if errors.Is(err, config.ErrSecretUnreadable) {
		// Saving would replace the stored secret with nothing. Leave the
		// file alone until the user enters the secret again.
		return err
	}
	if err != nil {
		slog.Error("load config", "err", err)
		return err
	}
	if saveErr := config.Save(a.opts.ConfigPath, cfg); saveErr != nil {
		slog.Warn("save config", "err", saveErr)
	}
	return nil
}

// applyLanguageChange rebuilds long-lived UI windows after the UI language
// changes. UI thread only.
func (a *App) applyLanguageChange() {
	a.rebuildClipPanel()
	if a.histWin != nil {
		a.histWin.Close()
		a.openHistoryWindow()
	}
}

// registerHotkeys (re)binds every action and returns human-readable problems.
func (a *App) registerHotkeys() []string {
	tr := i18n.Current()
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
			problems = append(problems, fmt.Sprintf(tr.TrayHotkeyBadFormat, act, s))
			slog.Warn("bad hotkey", "action", act.logName(), "hotkey", s, "err", err)
			continue
		}
		if err := a.hk.Register(int(act), hk); err != nil {
			reason := tr.TrayHotkeyRegFailed
			if hotkey.IsTaken(err) {
				reason = tr.TrayHotkeyTaken
			}
			problems = append(problems, fmt.Sprintf(tr.TrayHotkeyRegErr, act, hk, reason))
			slog.Warn("register hotkey", "action", act.logName(), "hotkey", hk, "err", err)
			continue
		}
		a.bound[act] = hk
		slog.Info("hotkey registered", "action", act.logName(), "hotkey", hk)
	}
	return problems
}

func (a *App) menu() []ui.MenuItem {
	tr := i18n.Current()
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
		item(ClipboardHistory),
		{},
		{Text: tr.TrayHistory, OnClick: a.openHistoryWindow},
		{Text: tr.TraySettings, OnClick: a.openSettings},
		{},
		{Text: tr.TrayReload, OnClick: a.reload},
		{Text: tr.TrayOpenConfigDir, OnClick: func() { openPath(a.opts.ConfigDir) }},
		{Text: tr.TrayOpenLogDir, OnClick: func() { openPath(a.opts.LogDir) }},
		{},
		{Text: tr.TrayExit, OnClick: func() { walk.App().Exit(0) }},
	}
}

// reload re-reads the settings file, rebinds hotkeys and rebuilds the
// translation engines. UI thread.
func (a *App) reload() {
	prevLang := i18n.Active()
	prevOCRDir := a.cfg.OCR.Dir

	cfgErr := a.loadConfig()
	if a.cfg.OCR.Dir != prevOCRDir {
		a.ocr.Close()
	}
	var problems []string
	if errors.Is(cfgErr, config.ErrSecretUnreadable) {
		problems = append(problems, i18n.Current().TraySecretUnreadable)
		cfgErr = nil
	}
	problems = append(problems, a.registerHotkeys()...)
	problems = append(problems, a.buildEngines()...)
	if err := a.tray.SetMenu(a.menu()); err != nil {
		slog.Error("rebuild tray menu", "err", err)
	}
	if i18n.Active() != prevLang {
		a.applyLanguageChange()
	} else if a.histWin != nil {
		a.histWin.SetRecording(a.cfg.History.Enabled)
	}
	if p := a.applyClipboard(); p != "" {
		problems = append(problems, p)
	}
	tr := i18n.Current()
	switch {
	case cfgErr != nil:
		a.tray.Warn(tr.TrayConfigErrorTitle, cfgErr.Error())
	case len(problems) > 0:
		a.tray.Warn(tr.TrayConfigProblemsTitle, strings.Join(problems, "\n"))
	default:
		a.tray.Info(tr.TrayConfigReloaded, "")
	}
}

// trigger starts an action without waiting for it. Safe to call from any
// goroutine, including the UI thread.
func (a *App) trigger(act Action) {
	slog.Info("action triggered", "action", act.logName())
	switch act {
	case ScreenshotOCR, ScreenshotTranslate:
		go a.screenshotOCR(act, false)
	case SilentOCR:
		go a.screenshotOCR(act, true)
	case SelectionTranslate:
		// Right away, while the hotkey's modifiers are still down.
		input.MaskHotkey()
		go a.selectionTranslate()
	case InputTranslate:
		go a.inputTranslate()
	case ClipboardHistory:
		// Right away, while Alt is still down: releasing it alone would put
		// the panel into menu mode and swallow the first key typed.
		input.MaskHotkey()
		ui.Do(a.toggleClipPanel)
	default:
		a.tray.Info(act.String(), i18n.Current().TrayFeatureWIP)
	}
}

func formatOCRError(err error) string {
	var oe *ocr.Error
	if !errors.As(err, &oe) {
		return err.Error()
	}
	tr := i18n.Current()
	switch oe.Code {
	case ocr.ErrCodeNotInstalled:
		return tr.OCRNotInstalled
	case ocr.ErrCodeCustomDirMissing:
		return fmt.Sprintf(tr.OCRCustomDirMissing, oe.Arg, oe.Detail)
	case ocr.ErrCodeStageFailed:
		return fmt.Sprintf(tr.OCRStageFailed, oe.Detail)
	case ocr.ErrCodeLoadDLL:
		return fmt.Sprintf(tr.OCRLoadDLL, oe.Arg, oe.Detail)
	case ocr.ErrCodeBadArch:
		return fmt.Sprintf(tr.OCRBadArch, oe.Arg, oe.Detail)
	case ocr.ErrCodeMissingProc:
		return fmt.Sprintf(tr.OCRMissingProc, oe.Arg, oe.Detail)
	case ocr.ErrCodeModelPathEncoding:
		return fmt.Sprintf(tr.OCRModelPathEncoding, oe.Arg)
	case ocr.ErrCodeModelOpen:
		return fmt.Sprintf(tr.OCRModelOpen, oe.Arg)
	case ocr.ErrCodeModelKey:
		return tr.OCRModelKey
	case ocr.ErrCodePipelineInit:
		return fmt.Sprintf(tr.OCRPipelineInit, oe.Arg, oe.Detail)
	case ocr.ErrCodeSelfTest:
		return fmt.Sprintf(tr.OCRSelfTest, oe.Detail)
	case ocr.ErrCodeWorkerStart:
		return fmt.Sprintf(tr.OCRWorkerStart, oe.Detail)
	case ocr.ErrCodeWorkerTimeout:
		return tr.OCRWorkerTimeout
	case ocr.ErrCodeWorkerIO:
		return fmt.Sprintf(tr.OCRWorkerIO, oe.Detail)
	}
	return oe.Error()
}

// cursorAnchor is a one-pixel rectangle at the mouse pointer, for placing
// popups and toasts on the screen the user is looking at.
func cursorAnchor() image.Rectangle {
	var pt win.POINT
	win.GetCursorPos(&pt)
	return image.Rect(int(pt.X), int(pt.Y), int(pt.X)+1, int(pt.Y)+1)
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
