// Package input sends key presses to the foreground program.
package input

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procSendInput        = user32.NewProc("SendInput")
	procGetAsyncKeyState = user32.NewProc("GetAsyncKeyState")
)

// ErrKeysHeld means modifier keys stayed down, so a shortcut could not be
// sent cleanly: Ctrl+C sent while Win is held is Win+Ctrl+C, which toggles
// the colour filter.
var ErrKeysHeld = errors.New("modifier keys still held")

// keyInput is a KEYBDINPUT wrapped in INPUT (40 bytes on amd64).
type keyInput struct {
	Type  uint32
	_     uint32
	Vk    uint16
	Scan  uint16
	Flags uint32
	Time  uint32
	Extra uintptr
	_     [8]byte
}

const (
	inputKeyboard = 1
	keyUp         = 0x0002
	vkControl     = 0x11
	// vkMask is unassigned; AutoHotkey uses it for the same purpose.
	vkMask = 0xE8
)

func sendKeys(keys ...keyInput) error {
	for i := range keys {
		keys[i].Type = inputKeyboard
	}
	n, _, err := procSendInput.Call(uintptr(len(keys)), uintptr(unsafe.Pointer(&keys[0])), unsafe.Sizeof(keys[0]))
	if int(n) != len(keys) {
		return fmt.Errorf("SendInput: %w", err)
	}
	return nil
}

// MaskHotkey sends a key nobody uses while a hotkey's modifiers are still
// down. Otherwise releasing Alt alone makes the focused program highlight
// its menu bar, and releasing Win alone opens Start. Call it as soon as the
// hotkey fires.
func MaskHotkey() {
	if err := sendKeys(keyInput{Vk: vkMask}, keyInput{Vk: vkMask, Flags: keyUp}); err != nil {
		slog.Warn("mask hotkey", "err", err)
	}
}

// Ctrl presses and releases Ctrl+key, key being a virtual-key code such as
// 'C' or 'V'.
func Ctrl(key uint16) error {
	return sendKeys(
		keyInput{Vk: vkControl},
		keyInput{Vk: key},
		keyInput{Vk: key, Flags: keyUp},
		keyInput{Vk: vkControl, Flags: keyUp},
	)
}

// WaitReleased waits until the user lets go of Win, Alt, Ctrl and Shift,
// so a shortcut sent next is not combined with them. It gives up with
// ErrKeysHeld after timeout.
func WaitReleased(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		held := false
		for _, vk := range []uintptr{0x5B, 0x5C, 0x12, 0x11, 0x10} { // LWin RWin Alt Ctrl Shift
			if s, _, _ := procGetAsyncKeyState.Call(vk); s&0x8000 != 0 {
				held = true
				break
			}
		}
		if !held {
			return nil
		}
		if time.Now().After(deadline) {
			return ErrKeysHeld
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(15 * time.Millisecond):
		}
	}
}
