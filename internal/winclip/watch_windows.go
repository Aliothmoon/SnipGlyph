package winclip

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procAddClipboardFormatListener    = user32.NewProc("AddClipboardFormatListener")
	procRemoveClipboardFormatListener = user32.NewProc("RemoveClipboardFormatListener")
	procRegisterClassEx               = user32.NewProc("RegisterClassExW")
	procDefWindowProc                 = user32.NewProc("DefWindowProcW")
	procGetMessage                    = user32.NewProc("GetMessageW")
	procDispatchMessage               = user32.NewProc("DispatchMessageW")
	procPostMessage                   = user32.NewProc("PostMessageW")
	procPostQuitMessage               = user32.NewProc("PostQuitMessage")
)

const (
	wmDestroy         = 0x0002
	wmClose           = 0x0010
	wmClipboardUpdate = 0x031D
)

const watchClass = "SnipGlyphClipboardWatch"

// watchers maps each listening window to the channel it signals.
var watchers sync.Map // uintptr -> chan struct{}

var registerWatchClass = sync.OnceValue(func() error {
	cls, _ := windows.UTF16PtrFromString(watchClass)
	wc := struct {
		size                uint32
		style               uint32
		wndProc             uintptr
		clsExtra, wndExtra  int32
		instance            windows.Handle
		icon, cursor, brush uintptr
		menuName, className *uint16
		iconSm              uintptr
	}{wndProc: windows.NewCallback(watchProc), className: cls}
	wc.size = uint32(unsafe.Sizeof(wc))
	if err := windows.GetModuleHandleEx(0, nil, &wc.instance); err != nil {
		return err
	}
	if r, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return fmt.Errorf("RegisterClassEx: %w", err)
	}
	return nil
})

func watchProc(hwnd, msg, wp, lp uintptr) uintptr {
	switch msg {
	case wmClipboardUpdate:
		if ch, ok := watchers.Load(hwnd); ok {
			select {
			case ch.(chan struct{}) <- struct{}{}:
			default: // a notification is already pending
			}
		}
		return 0
	case wmClose:
		procRemoveClipboardFormatListener.Call(hwnd)
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wp, lp)
	return r
}

// Watch calls changed on its own goroutine each time the clipboard content
// changes, until ctx ends. Changes that arrive while changed is running
// are reported once more after it returns.
func Watch(ctx context.Context, changed func()) error {
	if err := registerWatchClass(); err != nil {
		return err
	}
	notify := make(chan struct{}, 1)
	ready := make(chan error, 1)
	var hwnd uintptr
	go func() {
		// The window and its messages belong to this thread.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		cls, _ := windows.UTF16PtrFromString(watchClass)
		const hwndMessage = ^uintptr(2) // HWND_MESSAGE (-3)
		h, _, err := procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(cls)), 0, 0, 0, 0, 0, 0, hwndMessage, 0, 0, 0)
		if h == 0 {
			ready <- fmt.Errorf("create clipboard watch window: %w", err)
			return
		}
		watchers.Store(h, notify)
		defer watchers.Delete(h)
		if r, _, err := procAddClipboardFormatListener.Call(h); r == 0 {
			procDestroyWindow.Call(h)
			ready <- fmt.Errorf("AddClipboardFormatListener: %w", err)
			return
		}
		hwnd = h
		ready <- nil
		var msg [48]byte
		for {
			r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0)
			if int32(r) <= 0 {
				break
			}
			procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
		}
	}()
	if err := <-ready; err != nil {
		return err
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				procPostMessage.Call(hwnd, wmClose, 0, 0)
				return
			case <-notify:
				changed()
			}
		}
	}()
	return nil
}
