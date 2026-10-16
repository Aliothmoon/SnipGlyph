package textgrab

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                                 = windows.NewLazySystemDLL("user32.dll")
	procOpenClipboard                      = user32.NewProc("OpenClipboard")
	procCreateWindowEx                     = user32.NewProc("CreateWindowExW")
	procDestroyWindow                      = user32.NewProc("DestroyWindow")
	procCloseClipboard                     = user32.NewProc("CloseClipboard")
	procEmptyClipboard                     = user32.NewProc("EmptyClipboard")
	procEnumClipboardFormats               = user32.NewProc("EnumClipboardFormats")
	procGetClipboardData                   = user32.NewProc("GetClipboardData")
	procSetClipboardData                   = user32.NewProc("SetClipboardData")
	procGetClipboardSequenceNumber         = user32.NewProc("GetClipboardSequenceNumber")
	procRegisterClipboardFormat            = user32.NewProc("RegisterClipboardFormatW")
	procGetClipboardFormatName             = user32.NewProc("GetClipboardFormatNameW")
	kernel32                               = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalAlloc                        = kernel32.NewProc("GlobalAlloc")
	procGlobalFree                         = kernel32.NewProc("GlobalFree")
	procGlobalLock                         = kernel32.NewProc("GlobalLock")
	procGlobalUnlock                       = kernel32.NewProc("GlobalUnlock")
	procGlobalSize                         = kernel32.NewProc("GlobalSize")
	procRtlMoveMemory                      = kernel32.NewProc("RtlMoveMemory")
	errClipboardBusy                       = errors.New("clipboard is in use by another program")
	errClipboardTooBig                     = errors.New("clipboard content too large to preserve")
	maxClipboardBytes              uintptr = 256 << 20
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

// clipItem is one clipboard format's data.
type clipItem struct {
	format uint32
	data   []byte
}

// skipFormat reports formats that cannot be copied byte for byte: GDI or
// private handles rather than global memory, and OLE's markers, whose bytes
// refer to an object in the program that copied. Bitmaps survive anyway:
// Windows also offers them as CF_DIB and synthesises the rest.
func skipFormat(f uint32) bool {
	switch f {
	case 2, 3, 9, 14, // CF_BITMAP, CF_METAFILEPICT, CF_PALETTE, CF_ENHMETAFILE
		0x80, 0x82, 0x83, 0x8E: // CF_OWNERDISPLAY, CF_DSP*
		return true
	}
	if f >= 0x200 && f <= 0x3FF { // CF_PRIVATE*, CF_GDIOBJ*
		return true
	}
	if f >= 0xC000 {
		switch clipboardFormatName(f) {
		case "DataObject", "Ole Private Data":
			return true
		}
	}
	return false
}

func clipboardFormatName(f uint32) string {
	var buf [128]uint16
	n, _, _ := procGetClipboardFormatName.Call(uintptr(f), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

// clipSession is the clipboard opened by this process. It is opened for a
// hidden message-only window that owns what we put on it: with no owner
// window, Windows leaves an emptied clipboard in a state where this
// process's later opens report success yet every call fails with "thread
// does not have a clipboard open". The window and the open clipboard belong
// to the calling OS thread, so the goroutine stays on it until close.
type clipSession struct{ hwnd uintptr }

func openClipboard(ctx context.Context) (*clipSession, error) {
	runtime.LockOSThread()
	cls, _ := windows.UTF16PtrFromString("STATIC")
	const hwndMessage = ^uintptr(2) // HWND_MESSAGE (-3)
	hwnd, _, err := procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(cls)), 0, 0, 0, 0, 0, 0, hwndMessage, 0, 0, 0)
	if hwnd == 0 {
		runtime.UnlockOSThread()
		return nil, fmt.Errorf("create clipboard owner window: %w", err)
	}
	c := &clipSession{hwnd}
	for {
		if ok, _, _ := procOpenClipboard.Call(hwnd); ok != 0 {
			return c, nil
		}
		select {
		case <-ctx.Done():
			procDestroyWindow.Call(hwnd)
			runtime.UnlockOSThread()
			return nil, errClipboardBusy
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (c *clipSession) close() {
	procCloseClipboard.Call()
	procDestroyWindow.Call(c.hwnd)
	runtime.UnlockOSThread()
}

func clipboardSequence() uint32 {
	n, _, _ := procGetClipboardSequenceNumber.Call()
	return uint32(n)
}

// saveClipboard copies every format it can preserve.
func saveClipboard(ctx context.Context) ([]clipItem, error) {
	cb, err := openClipboard(ctx)
	if err != nil {
		return nil, err
	}
	defer cb.close()
	var items []clipItem
	var total uintptr
	for f := uintptr(0); ; {
		f, _, _ = procEnumClipboardFormats.Call(f)
		if f == 0 {
			break
		}
		if skipFormat(uint32(f)) {
			continue
		}
		h, _, _ := procGetClipboardData.Call(f)
		if h == 0 {
			continue
		}
		data, err := globalBytes(h)
		if err != nil {
			continue
		}
		if total += uintptr(len(data)); total > maxClipboardBytes {
			return nil, errClipboardTooBig
		}
		items = append(items, clipItem{uint32(f), data})
	}
	return items, nil
}

// restoreClipboard puts items back, marked so that clipboard history and
// cloud sync ignore the round trip.
func restoreClipboard(ctx context.Context, items []clipItem) error {
	cb, err := openClipboard(ctx)
	if err != nil {
		return err
	}
	defer cb.close()
	procEmptyClipboard.Call()
	for _, it := range items {
		if err := setClipboardBytes(it.format, it.data); err != nil {
			return fmt.Errorf("restore clipboard format %d: %w", it.format, err)
		}
	}
	excludeFromHistory()
	return nil
}

func excludeFromHistory() {
	zero := []byte{0, 0, 0, 0} // DWORD 0
	for _, name := range []string{
		"ExcludeClipboardContentFromMonitorProcessing",
		"CanIncludeInClipboardHistory",
		"CanUploadToCloudClipboard",
	} {
		n, _ := windows.UTF16PtrFromString(name)
		if f, _, _ := procRegisterClipboardFormat.Call(uintptr(unsafe.Pointer(n))); f != 0 {
			setClipboardBytes(uint32(f), zero)
		}
	}
}

// clipboardText reads the clipboard as text.
func clipboardText(ctx context.Context) (string, error) {
	cb, err := openClipboard(ctx)
	if err != nil {
		return "", err
	}
	defer cb.close()
	h, _, _ := procGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return "", nil
	}
	data, err := globalBytes(h)
	if err != nil {
		return "", err
	}
	u := unsafe.Slice((*uint16)(unsafe.Pointer(unsafe.SliceData(data))), len(data)/2)
	return windows.UTF16ToString(u), nil
}

// globalBytes copies the contents of a global memory handle.
func globalBytes(h uintptr) ([]byte, error) {
	n, _, _ := procGlobalSize.Call(h)
	if n == 0 {
		return []byte{}, nil
	}
	if n > maxClipboardBytes {
		return nil, errClipboardTooBig
	}
	p, _, err := procGlobalLock.Call(h)
	if p == 0 {
		return nil, fmt.Errorf("GlobalLock: %w", err)
	}
	defer procGlobalUnlock.Call(h)
	data := make([]byte, n)
	procRtlMoveMemory.Call(uintptr(unsafe.Pointer(&data[0])), p, n)
	return data, nil
}

// setClipboardBytes hands the clipboard a global memory copy of data. The
// clipboard must be open.
func setClipboardBytes(format uint32, data []byte) error {
	h, _, err := procGlobalAlloc.Call(gmemMoveable, uintptr(max(len(data), 1)))
	if h == 0 {
		return fmt.Errorf("GlobalAlloc: %w", err)
	}
	if len(data) > 0 {
		p, _, err := procGlobalLock.Call(h)
		if p == 0 {
			procGlobalFree.Call(h)
			return fmt.Errorf("GlobalLock: %w", err)
		}
		procRtlMoveMemory.Call(p, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
		procGlobalUnlock.Call(h)
	}
	if r, _, err := procSetClipboardData.Call(uintptr(format), h); r == 0 {
		procGlobalFree.Call(h) // still ours when SetClipboardData fails
		return fmt.Errorf("SetClipboardData: %w", err)
	}
	return nil
}
