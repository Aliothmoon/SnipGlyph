package ui

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"

	"SnipGlyph/internal/cliphist"
	"SnipGlyph/internal/config"
	"SnipGlyph/internal/history"
	"SnipGlyph/internal/translate"
)

const (
	settingsWidth  = 460 // DIP
	defaultKeyHint = "点击输入框后按下新的组合键；Backspace 清除"
	baiduSignupURL = "https://fanyi-api.baidu.com/manage/developer"
)

// SettingsOptions configures the settings window.
type SettingsOptions struct {
	Config    config.Config
	Autostart bool
	// Save checks, stores and applies the settings on the UI thread. An
	// error means nothing was saved; problems went wrong applying them, such
	// as a hotkey taken by another program.
	Save func(cfg config.Config, autostart bool) (problems []string, err error)
	// Recording runs with true when a hotkey box starts listening and with
	// false when it stops. The app releases its hotkeys meanwhile, or
	// pressing one would trigger it instead of reaching the box.
	Recording func(on bool)
	// TestBaidu translates a sample with the given credentials. It runs on
	// a background goroutine.
	TestBaidu func(ctx context.Context, b config.Baidu) (string, error)
	// OnClose runs on the UI thread when the window closes.
	OnClose func()
}

// hotkeyFields lists the hotkey settings in menu order.
var hotkeyFields = []struct {
	name  string
	field func(*config.Hotkeys) *string
}{
	{"截图 OCR", func(h *config.Hotkeys) *string { return &h.ScreenshotOCR }},
	{"静默 OCR", func(h *config.Hotkeys) *string { return &h.SilentOCR }},
	{"截图翻译", func(h *config.Hotkeys) *string { return &h.ScreenshotTranslate }},
	{"划词翻译", func(h *config.Hotkeys) *string { return &h.SelectionTranslate }},
	{"输入翻译", func(h *config.Hotkeys) *string { return &h.InputTranslate }},
	{"剪贴板历史", func(h *config.Hotkeys) *string { return &h.ClipboardHistory }},
}

// Proxy choices, by index in the proxy drop-down.
const (
	proxySystem = iota
	proxyDirect
	proxyCustom
)

// Settings is the settings window. Its methods run on the UI thread.
type Settings struct {
	opts SettingsOptions
	mw   *walk.MainWindow

	autostart *walk.CheckBox
	history   *walk.CheckBox
	clipboard *walk.CheckBox
	boxes     []*hotkeyBox
	keyHint   *walk.TextLabel

	langs     []translate.Lang
	target    *walk.ComboBox
	baiduID   *walk.LineEdit
	baiduKey  *walk.LineEdit
	proxyMode *walk.ComboBox
	proxyURL  *walk.LineEdit
	testBt    *walk.PushButton
	testMsg   *walk.TextLabel

	status *walk.TextLabel
	saveBt *walk.PushButton

	recording  bool
	closing    bool
	cancelTest context.CancelFunc
}

// NewSettings creates and shows the settings window. UI thread only.
func NewSettings(opts SettingsOptions) (*Settings, error) {
	mw, err := newWindow("SnipGlyph 设置", &walk.MainWindowCfg{DisableMaximize: true, DisableResizing: true})
	if err != nil {
		return nil, err
	}
	s := &Settings{opts: opts, mw: mw}
	ok := false
	defer func() {
		if !ok {
			mw.Dispose()
		}
	}()
	if err := s.build(); err != nil {
		return nil, err
	}
	h := mw.Handle()
	walk.App().AddPreTranslateHandlerForHWND(h, settingsKeys{s})
	mw.Closing().Attach(func(*bool, walk.CloseReason) {
		s.closing = true
		walk.App().DeletePreTranslateHandlerForHWND(h)
		if s.cancelTest != nil {
			s.cancelTest()
		}
		if s.recording {
			s.recording = false
			s.opts.Recording(false)
		}
		if opts.OnClose != nil {
			opts.OnClose()
		}
	})
	if err := showFitted(mw, settingsWidth); err != nil {
		return nil, err
	}
	win.SetForegroundWindow(h)
	s.saveBt.SetFocus()
	ok = true
	return s, nil
}

// Activate brings the window to the front.
func (s *Settings) Activate() {
	if !s.gone() {
		bringToFront(s.mw)
	}
}

func (s *Settings) gone() bool { return s.closing || s.mw.IsDisposed() }

func (s *Settings) build() error {
	cfg := s.opts.Config
	layout := walk.NewVBoxLayout()
	layout.SetMargins(walk.Margins{HNear: 12, VNear: 10, HFar: 12, VFar: 10})
	layout.SetSpacing(10)
	if err := s.mw.SetLayout(layout); err != nil {
		return err
	}

	// General.
	_, general, err := titledCard(s.mw, "常规")
	if err != nil {
		return err
	}
	if s.autostart, err = checkBox(general, "开机时自动启动", s.opts.Autostart); err != nil {
		return err
	}
	if s.history, err = checkBox(general, "保存历史记录（只存文字，最近 "+strconv.Itoa(history.DefaultLimit)+" 条）", cfg.History.Enabled); err != nil {
		return err
	}
	clipLimit := cfg.Clipboard.Limit
	if clipLimit <= 0 {
		clipLimit = cliphist.DefaultLimit
	}
	if s.clipboard, err = checkBox(general, "记录剪贴板历史（文字、图片、文件路径；最近 "+strconv.Itoa(clipLimit)+" 条，置顶不计）", cfg.Clipboard.Enabled); err != nil {
		return err
	}

	// Hotkeys.
	_, keys, err := titledCard(s.mw, "快捷键")
	if err != nil {
		return err
	}
	g, gl, err := grid(keys)
	if err != nil {
		return err
	}
	for i, f := range hotkeyFields {
		l, err := label(g, f.name, colorText, nil)
		if err != nil {
			return err
		}
		b, err := newHotkeyBox(g, *f.field(&cfg.Hotkeys), s.keyInput)
		if err != nil {
			return err
		}
		b.le.FocusedChanged().Attach(s.focusChanged)
		gl.SetRange(l, walk.Rectangle{X: 0, Y: i, Width: 1, Height: 1})
		gl.SetRange(b.le, walk.Rectangle{X: 1, Y: i, Width: 1, Height: 1})
		s.boxes = append(s.boxes, b)
	}
	gl.SetColumnStretchFactor(1, 1)
	if s.keyHint, err = textLabel(keys, defaultKeyHint, colorMuted); err != nil {
		return err
	}

	// Translation.
	_, tr, err := titledCard(s.mw, "翻译")
	if err != nil {
		return err
	}
	if g, gl, err = grid(tr); err != nil {
		return err
	}
	cell := func(w walk.Widget, x, y, width int) {
		gl.SetRange(w, walk.Rectangle{X: x, Y: y, Width: width, Height: 1})
	}
	addLabel := func(text string, y int) error {
		l, err := label(g, text, colorText, nil)
		if err == nil {
			cell(l, 0, y, 1)
		}
		return err
	}

	if err := addLabel("目标语言", 0); err != nil {
		return err
	}
	if s.target, err = walk.NewDropDownBox(g); err != nil {
		return err
	}
	s.langs = slices.Clone(translate.Targets)
	cur := translate.Lang(cfg.Translate.TargetLang)
	if !slices.Contains(s.langs, cur) && cur != "" {
		s.langs = append(s.langs, cur) // set by hand in the file
	}
	names := make([]string, len(s.langs))
	for i, l := range s.langs {
		names[i] = l.Name()
	}
	if err := s.target.SetModel(names); err != nil {
		return err
	}
	s.target.SetCurrentIndex(max(0, slices.Index(s.langs, cur)))
	cell(s.target, 1, 0, 1)
	note, err := label(g, "中文原文总是译成英文", colorMuted, nil)
	if err != nil {
		return err
	}
	cell(note, 2, 0, 1)

	head, err := row(g)
	if err != nil {
		return err
	}
	if _, err := label(head, "百度翻译", colorMuted, titleFont()); err != nil {
		return err
	}
	if _, err := walk.NewHSpacer(head); err != nil {
		return err
	}
	if _, err := link(head, "申请密钥", func() { openURL(baiduSignupURL) }); err != nil {
		return err
	}
	cell(head, 0, 1, 3)

	b := cfg.Translate.Baidu
	if err := addLabel("APP ID", 2); err != nil {
		return err
	}
	if s.baiduID, err = lineEdit(g, b.AppID); err != nil {
		return err
	}
	cell(s.baiduID, 1, 2, 2)
	if err := addLabel("密钥", 3); err != nil {
		return err
	}
	if s.baiduKey, err = lineEdit(g, string(b.Key)); err != nil {
		return err
	}
	s.baiduKey.SetPasswordMode(true)
	cell(s.baiduKey, 1, 3, 2)

	if err := addLabel("代理", 4); err != nil {
		return err
	}
	if s.proxyMode, err = walk.NewDropDownBox(g); err != nil {
		return err
	}
	if err := s.proxyMode.SetModel([]string{"跟随系统", "直连", "自定义"}); err != nil {
		return err
	}
	cell(s.proxyMode, 1, 4, 1)
	if s.proxyURL, err = lineEdit(g, ""); err != nil {
		return err
	}
	s.proxyURL.SetCueBanner("http://127.0.0.1:7890")
	cell(s.proxyURL, 2, 4, 1)
	switch b.Proxy {
	case "":
		s.proxyMode.SetCurrentIndex(proxySystem)
	case "direct":
		s.proxyMode.SetCurrentIndex(proxyDirect)
	default:
		s.proxyMode.SetCurrentIndex(proxyCustom)
		s.proxyURL.SetText(b.Proxy)
	}
	s.proxyURL.SetEnabled(s.proxyMode.CurrentIndex() == proxyCustom)
	s.proxyMode.CurrentIndexChanged().Attach(func() {
		custom := s.proxyMode.CurrentIndex() == proxyCustom
		s.proxyURL.SetEnabled(custom)
		if custom {
			s.proxyURL.SetFocus()
		}
	})

	if s.testBt, err = walk.NewPushButton(g); err != nil {
		return err
	}
	s.testBt.SetText("测试")
	s.testBt.Clicked().Attach(s.testBaidu)
	cell(s.testBt, 1, 5, 1)
	gl.SetColumnStretchFactor(1, 1)
	gl.SetColumnStretchFactor(2, 1)
	if s.testMsg, err = textLabel(tr, "", colorMuted); err != nil {
		return err
	}
	setShown(s.testMsg, false)

	// Footer.
	if s.status, err = textLabel(s.mw, "", colorError); err != nil {
		return err
	}
	setShown(s.status, false)
	foot, err := row(s.mw)
	if err != nil {
		return err
	}
	if _, err := walk.NewHSpacer(foot); err != nil {
		return err
	}
	if s.saveBt, err = walk.NewPushButton(foot); err != nil {
		return err
	}
	s.saveBt.SetText("保存")
	s.saveBt.SetMinMaxSize(walk.Size{Width: 88}, walk.Size{})
	s.saveBt.SendMessage(win.BM_SETSTYLE, win.BS_DEFPUSHBUTTON, 1)
	s.saveBt.Clicked().Attach(s.save)
	cancel, err := walk.NewPushButton(foot)
	if err != nil {
		return err
	}
	cancel.SetText("取消")
	cancel.SetMinMaxSize(walk.Size{Width: 88}, walk.Size{})
	cancel.Clicked().Attach(func() { s.mw.Close() })
	return nil
}

// keyInput is called by the hotkey boxes after each key press.
func (s *Settings) keyInput(reason string) {
	if reason == "" {
		s.keyHint.SetTextColor(colorMuted)
		s.keyHint.SetText(defaultKeyHint)
		return
	}
	s.keyHint.SetTextColor(colorError)
	s.keyHint.SetText(reason)
}

// focusChanged tells the app when hotkey recording starts and stops. It
// waits until focus has settled, so tabbing from one box to the next does
// not release and grab the hotkeys in between.
func (s *Settings) focusChanged() {
	walk.App().Synchronize(func() {
		if s.gone() {
			return
		}
		on := slices.ContainsFunc(s.boxes, func(b *hotkeyBox) bool { return b.le.Focused() })
		if on != s.recording {
			s.recording = on
			s.opts.Recording(on)
		}
		if !on {
			s.keyInput("")
		}
	})
}

// collect reads the form into a copy of the config it was opened with.
func (s *Settings) collect() (config.Config, error) {
	cfg := s.opts.Config // keeps settings the window does not show
	for i, f := range hotkeyFields {
		*f.field(&cfg.Hotkeys) = s.boxes[i].value
	}
	cfg.History.Enabled = s.history.Checked()
	cfg.Clipboard.Enabled = s.clipboard.Checked()
	if i := s.target.CurrentIndex(); i >= 0 {
		cfg.Translate.TargetLang = string(s.langs[i])
	}
	b := &cfg.Translate.Baidu
	b.AppID = strings.TrimSpace(s.baiduID.Text())
	b.Key = config.Secret(strings.TrimSpace(s.baiduKey.Text()))
	switch s.proxyMode.CurrentIndex() {
	case proxySystem:
		b.Proxy = ""
	case proxyDirect:
		b.Proxy = "direct"
	default:
		b.Proxy = strings.TrimSpace(s.proxyURL.Text())
		if b.Proxy == "" {
			return cfg, errors.New("请填写百度翻译的代理地址，或选择「跟随系统」")
		}
	}
	if (b.AppID == "") != (b.Key == "") {
		return cfg, errors.New("百度翻译的 APP ID 和密钥要一起填写")
	}
	return cfg, nil
}

func (s *Settings) save() {
	cfg, err := s.collect()
	if err != nil {
		s.setStatus(err.Error())
		return
	}
	problems, err := s.opts.Save(cfg, s.autostart.Checked())
	if err != nil {
		s.setStatus("没有保存：" + err.Error())
		return
	}
	s.opts.Config = cfg
	if len(problems) > 0 {
		s.setStatus("已保存，但有问题：\n" + strings.Join(problems, "\n"))
		return
	}
	s.mw.Close()
}

func (s *Settings) setStatus(msg string) {
	s.status.SetText(msg)
	setShown(s.status, msg != "")
	refitHeight(s.mw)
}

func (s *Settings) testBaidu() {
	cfg, err := s.collect()
	if err == nil && !cfg.Translate.Baidu.Configured() {
		err = errors.New("请先填写 APP ID 和密钥")
	}
	if err != nil {
		s.setTestMsg(err.Error(), colorError)
		return
	}
	if s.cancelTest != nil {
		s.cancelTest()
	}
	ctx, cancel := context.WithTimeout(walk.App().Context(), 15*time.Second)
	s.cancelTest = cancel
	s.testBt.SetEnabled(false)
	s.setTestMsg("正在测试…", colorMuted)
	go func() {
		got, err := s.opts.TestBaidu(ctx, cfg.Translate.Baidu)
		cancel()
		walk.App().Synchronize(func() {
			if s.gone() {
				return
			}
			s.testBt.SetEnabled(true)
			if err != nil {
				s.setTestMsg("测试失败："+err.Error(), colorError)
				return
			}
			s.setTestMsg("可以使用：hello → "+got, colorText)
		})
	}()
}

func (s *Settings) setTestMsg(msg string, c walk.Color) {
	s.testMsg.SetTextColor(c)
	s.testMsg.SetText(msg)
	setShown(s.testMsg, true)
	refitHeight(s.mw)
}

// settingsKeys feeds key presses to the focused hotkey box, and closes the
// window on Esc.
type settingsKeys struct{ s *Settings }

func (k settingsKeys) OnPreTranslate(msg *win.MSG) bool {
	s := k.s
	if s.gone() {
		return false
	}
	for _, b := range s.boxes {
		if msg.HWnd == b.le.Handle() {
			if b.handle(msg) {
				return true
			}
			break
		}
	}
	h := s.mw.Handle()
	if msg.Message == win.WM_KEYDOWN && msg.WParam == win.VK_ESCAPE && (msg.HWnd == h || win.IsChild(h, msg.HWnd)) {
		// An open drop-down list takes Esc to close itself.
		if win.SendMessage(msg.HWnd, win.CB_GETDROPPEDSTATE, 0, 0) != 0 {
			return false
		}
		walk.App().Synchronize(func() {
			if !s.gone() {
				s.mw.Close()
			}
		})
		return true
	}
	return false
}

// titledCard adds a full-width card with a heading; its content lines up
// on the left.
func titledCard(parent walk.Container, title string) (outer, inner *walk.Composite, err error) {
	if outer, inner, err = card(parent); err != nil {
		return nil, nil, err
	}
	head, err := row(inner)
	if err != nil {
		return nil, nil, err
	}
	if _, err := label(head, title, colorMuted, titleFont()); err != nil {
		return nil, nil, err
	}
	// The spacer makes the card take the window's width even when its
	// content (a few check boxes) is narrow.
	if _, err := walk.NewHSpacer(head); err != nil {
		return nil, nil, err
	}
	inner.Layout().SetSpacing(8)
	if bl, ok := inner.Layout().(*walk.BoxLayout); ok {
		bl.SetAlignment(walk.AlignHNearVNear)
	}
	return outer, inner, nil
}

// grid adds a borderless grid for label / field rows.
func grid(parent walk.Container) (*walk.Composite, *walk.GridLayout, error) {
	c, err := walk.NewComposite(parent)
	if err != nil {
		return nil, nil, err
	}
	l := walk.NewGridLayout()
	l.SetMargins(walk.Margins{})
	l.SetSpacing(8)
	return c, l, c.SetLayout(l)
}

func checkBox(parent walk.Container, text string, checked bool) (*walk.CheckBox, error) {
	cb, err := walk.NewCheckBox(parent)
	if err != nil {
		return nil, err
	}
	cb.SetChecked(checked)
	return cb, cb.SetText(text)
}

func lineEdit(parent walk.Container, text string) (*walk.LineEdit, error) {
	le, err := walk.NewLineEdit(parent)
	if err != nil {
		return nil, err
	}
	return le, le.SetText(text)
}

// textLabel adds a label that wraps long text.
func textLabel(parent walk.Container, text string, c walk.Color) (*walk.TextLabel, error) {
	tl, err := walk.NewTextLabel(parent)
	if err != nil {
		return nil, err
	}
	tl.SetTextColor(c)
	return tl, tl.SetText(text)
}
