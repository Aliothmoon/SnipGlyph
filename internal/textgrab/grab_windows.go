// Package textgrab reads the text the user has selected in another program.
package textgrab

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unsafe"
)

// Method says how Selected obtained the text.
type Method string

const (
	ByAutomation Method = "ui-automation"
	ByCopy       Method = "copy"
)

var (
	// ErrNoSelection means neither method found selected text.
	ErrNoSelection = errors.New("no text selected")
	// ErrKeysHeld means the hotkey's modifiers stayed down, so Ctrl+C could
	// not be sent cleanly (Win+Ctrl+C would toggle the colour filter).
	ErrKeysHeld = errors.New("modifier keys still held")
)

const (
	automationTimeout = 600 * time.Millisecond
	releaseTimeout    = 2 * time.Second
	copyTimeout       = 800 * time.Millisecond
)

var procSendInput = user32.NewProc("SendInput")
var procGetAsyncKeyState = user32.NewProc("GetAsyncKeyState")

// Selected returns the text selected in the foreground program. It asks UI
// Automation first, which has no side effects; when the focused control
// does not expose its selection that way, it sends Ctrl+C and puts the
// clipboard back as it was. Call MaskHotkey as soon as the hotkey fires.
func Selected(ctx context.Context) (string, Method, error) {
	actx, cancel := context.WithTimeout(ctx, automationTimeout)
	text, err := automationSelected(actx)
	cancel()
	if err == nil && strings.TrimSpace(text) != "" {
		return text, ByAutomation, nil
	}
	slog.Debug("ui automation gave no selection, copying", "err", err)

	text, err = copySelected(ctx)
	if err != nil {
		return "", ByCopy, err
	}
	if strings.TrimSpace(text) == "" {
		return "", ByCopy, ErrNoSelection
	}
	return text, ByCopy, nil
}

func automationSelected(ctx context.Context) (string, error) {
	type result struct {
		s   string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		s, err := withAutomation(focusedSelection)
		ch <- result{s, err}
	}()
	select {
	case r := <-ch:
		return r.s, r.err
	case <-ctx.Done():
		// A hung target program can block UI Automation; leave that call
		// behind and fall back to copying.
		return "", ctx.Err()
	}
}

func copySelected(ctx context.Context) (string, error) {
	if err := waitKeysReleased(ctx, releaseTimeout); err != nil {
		return "", err
	}
	saved, err := saveClipboard(ctx)
	if err != nil {
		return "", fmt.Errorf("save clipboard: %w", err)
	}
	before := clipboardSequence()
	if err := pressCtrlC(); err != nil {
		return "", err
	}

	// Wait for the program to put something on the clipboard. Programs
	// often empty it first and fill it a moment later, so also wait for the
	// sequence number to settle.
	deadline := time.Now().Add(copyTimeout)
	last, stableSince := before, time.Time{}
	for time.Now().Before(deadline) {
		if seq := clipboardSequence(); seq != last {
			last, stableSince = seq, time.Now()
		} else if last != before && time.Since(stableSince) >= 40*time.Millisecond {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if last == before {
		return "", ErrNoSelection // nothing copied: clipboard untouched
	}

	text, err := clipboardText(ctx)
	if rerr := restoreClipboard(ctx, saved); rerr != nil {
		slog.Warn("restore clipboard after copying the selection", "err", rerr)
	}
	return text, err
}

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
	vkC           = 0x43
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

// MaskHotkey sends a key nobody uses while the hotkey's modifiers are
// still down. Otherwise releasing Alt alone makes the target program
// highlight its menu bar, and releasing Win alone opens Start.
func MaskHotkey() {
	if err := sendKeys(keyInput{Vk: vkMask}, keyInput{Vk: vkMask, Flags: keyUp}); err != nil {
		slog.Warn("mask hotkey", "err", err)
	}
}

func pressCtrlC() error {
	return sendKeys(
		keyInput{Vk: vkControl},
		keyInput{Vk: vkC},
		keyInput{Vk: vkC, Flags: keyUp},
		keyInput{Vk: vkControl, Flags: keyUp},
	)
}

// waitKeysReleased waits until the user lets go of Win, Alt, Ctrl and
// Shift, so the Ctrl+C we send is not combined with them.
func waitKeysReleased(ctx context.Context, timeout time.Duration) error {
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
