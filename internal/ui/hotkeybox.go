package ui

import (
	"errors"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"
	"golang.org/x/sys/windows"

	"SnipGlyph/internal/hotkey"
)

var procImmAssociateContext = windows.NewLazySystemDLL("imm32.dll").NewProc("ImmAssociateContext")

// hotkeyBox records a key combination: the user focuses it and presses the
// keys. Backspace or Delete clears it. Keys reach it through the settings
// window's pre-translate handler, before dialog navigation and menus see
// them.
type hotkeyBox struct {
	le    *walk.LineEdit
	value string // canonical hotkey, or "" for none
	// onInput reports a rejected combination with a reason, or "" after a
	// change or an accepted one.
	onInput func(reason string)
	// modifiers reads the held modifier keys; tests replace it.
	modifiers func() hotkey.Modifiers
}

func newHotkeyBox(parent walk.Container, value string, onInput func(string)) (*hotkeyBox, error) {
	le, err := walk.NewLineEdit(parent)
	if err != nil {
		return nil, err
	}
	b := &hotkeyBox{le: le, onInput: onInput, modifiers: heldModifiers}
	le.SetReadOnly(true)
	setBackground(le, colorCard) // read-only edits are grey otherwise
	le.SetTextAlignment(walk.AlignCenter)
	le.SetCueBanner("未设置")
	// Keys only: no IME composition window over the box.
	procImmAssociateContext.Call(uintptr(le.Handle()), 0)
	b.set(value)
	return b, nil
}

func (b *hotkeyBox) set(value string) {
	b.value = value
	b.le.SetText(value)
}

// handle processes a keyboard message sent to the box and reports whether
// it consumed it.
func (b *hotkeyBox) handle(msg *win.MSG) bool {
	switch msg.Message {
	case win.WM_KEYDOWN, win.WM_SYSKEYDOWN:
	case win.WM_KEYUP, win.WM_SYSKEYUP:
		if b.modifiers() == 0 {
			b.le.SetText(b.value) // drop a "Win+Alt+…" preview
		}
		// Releasing Alt would otherwise enter the window's menu mode.
		return msg.Message == win.WM_SYSKEYUP
	case win.WM_SYSCHAR:
		return true // no mnemonic beep
	default:
		return false
	}

	vk := uint32(msg.WParam)
	mods := b.modifiers()
	switch {
	case isModifierKey(vk):
		b.le.SetText(hotkey.Hotkey{Mods: mods}.Prefix() + "…")
		return true
	case vk == win.VK_TAB && mods&^hotkey.ModShift == 0,
		(vk == win.VK_ESCAPE || vk == win.VK_RETURN) && mods == 0:
		return false // dialog navigation
	case (vk == win.VK_BACK || vk == win.VK_DELETE) && mods == 0:
		b.set("")
		b.onInput("")
		return true
	}
	hk := hotkey.Hotkey{Mods: mods, Key: vk}
	switch err := hk.Valid(); {
	case errors.Is(err, hotkey.ErrReserved):
		// Let it do its usual job: Alt+F4 still closes the window.
		b.le.SetText(b.value)
		b.onInput(hk.String() + " 是系统和各程序通用的组合键，不能用作快捷键")
		return false
	case errors.Is(err, hotkey.ErrNeedsModifier):
		b.le.SetText(b.value)
		b.onInput("快捷键要包含 Win、Ctrl 或 Alt（F1–F24、PrintScreen、Pause 除外）")
		return true
	case err != nil:
		b.le.SetText(b.value)
		b.onInput("不支持这个按键")
		return true
	}
	b.set(hk.String())
	b.onInput("")
	return true
}

func isModifierKey(vk uint32) bool {
	switch vk {
	case win.VK_SHIFT, win.VK_CONTROL, win.VK_MENU, win.VK_LWIN, win.VK_RWIN,
		win.VK_LSHIFT, win.VK_RSHIFT, win.VK_LCONTROL, win.VK_RCONTROL, win.VK_LMENU, win.VK_RMENU:
		return true
	}
	return false
}

// heldModifiers reads the modifier keys as of the message being handled.
func heldModifiers() hotkey.Modifiers {
	down := func(vk int32) bool { return win.GetKeyState(vk) < 0 }
	var m hotkey.Modifiers
	if down(win.VK_LWIN) || down(win.VK_RWIN) {
		m |= hotkey.ModWin
	}
	if down(win.VK_CONTROL) {
		m |= hotkey.ModCtrl
	}
	if down(win.VK_MENU) {
		m |= hotkey.ModAlt
	}
	if down(win.VK_SHIFT) {
		m |= hotkey.ModShift
	}
	return m
}
