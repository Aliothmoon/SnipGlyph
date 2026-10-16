// Package winclip reads, writes and watches the Windows clipboard.
package winclip

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
	user32                         = windows.NewLazySystemDLL("user32.dll")
	procOpenClipboard              = user32.NewProc("OpenClipboard")
	procCreateWindowEx             = user32.NewProc("CreateWindowExW")
	procDestroyWindow              = user32.NewProc("DestroyWindow")
	procCloseClipboard             = user32.NewProc("CloseClipboard")
	procEmptyClipboard             = user32.NewProc("EmptyClipboard")
	procEnumClipboardFormats       = user32.NewProc("EnumClipboardFormats")
	procGetClipboardData           = user32.NewProc("GetClipboardData")
	procSetClipboardData           = user32.NewProc("SetClipboardData")
	procGetClipboardSequenceNumber = user32.NewProc("GetClipboardSequenceNumber")
	procGetClipboardOwner          = user32.NewProc("GetClipboardOwner")
	procRegisterClipboardFormat    = user32.NewProc("RegisterClipboardFormatW")
	procGetClipboardFormatName     = user32.NewProc("GetClipboardFormatNameW")
	kernel32                       = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalAlloc                = kernel32.NewProc("GlobalAlloc")
	procGlobalFree                 = kernel32.NewProc("GlobalFree")
	procGlobalLock                 = kernel32.NewProc("GlobalLock")
	procGlobalUnlock               = kernel32.NewProc("GlobalUnlock")
	procGlobalSize                 = kernel32.NewProc("GlobalSize")
	procRtlMoveMemory              = kernel32.NewProc("RtlMoveMemory")
)

var (
	// ErrBusy means another program kept the clipboard open.
	ErrBusy = errors.New("clipboard is in use by another program")
	// ErrTooBig means the content exceeds MaxBytes.
	ErrTooBig = errors.New("clipboard content too large")
)

// MaxBytes caps how much is read from the clipboard in one go.
const MaxBytes = 256 << 20

// Standard formats.
const (
	CFText        = 1
	CFBitmap      = 2
	CFDIB         = 8
	CFUnicodeText = 13
	CFHDrop       = 15
	CFDIBV5       = 17
	gmemMoveable  = 0x0002
)

// Item is one clipboard format's data.
type Item struct {
	Format uint32
	Data   []byte
}

// RegisterFormat returns the id of a named format, registering it if
// needed, or 0 on failure.
func RegisterFormat(name string) uint32 {
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0
	}
	f, _, _ := procRegisterClipboardFormat.Call(uintptr(unsafe.Pointer(n)))
	return uint32(f)
}

// FormatName returns the name of a registered format, or "" for a
// standard one.
func FormatName(f uint32) string {
	var buf [128]uint16
	n, _, _ := procGetClipboardFormatName.Call(uintptr(f), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

// Sequence changes every time the clipboard content changes.
func Sequence() uint32 {
	n, _, _ := procGetClipboardSequenceNumber.Call()
	return uint32(n)
}

// Clipboard is the clipboard, opened by this process. It is opened for a
// hidden message-only window that owns what is put on it: with no owner
// window, Windows leaves an emptied clipboard in a state where this
// process's later opens report success yet every call fails with "thread
// does not have a clipboard open". The window and the open clipboard belong
// to the calling OS thread, so the goroutine stays on it until Close.
type Clipboard struct{ hwnd uintptr }

// Open opens the clipboard, retrying while another program holds it until
// ctx ends. Close it promptly: nobody else can use it meanwhile.
func Open(ctx context.Context) (*Clipboard, error) {
	runtime.LockOSThread()
	cls, _ := windows.UTF16PtrFromString("STATIC")
	const hwndMessage = ^uintptr(2) // HWND_MESSAGE (-3)
	hwnd, _, err := procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(cls)), 0, 0, 0, 0, 0, 0, hwndMessage, 0, 0, 0)
	if hwnd == 0 {
		runtime.UnlockOSThread()
		return nil, fmt.Errorf("create clipboard owner window: %w", err)
	}
	c := &Clipboard{hwnd}
	for {
		if ok, _, _ := procOpenClipboard.Call(hwnd); ok != 0 {
			return c, nil
		}
		select {
		case <-ctx.Done():
			procDestroyWindow.Call(hwnd)
			runtime.UnlockOSThread()
			return nil, ErrBusy
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// Close closes the clipboard.
func (c *Clipboard) Close() {
	procCloseClipboard.Call()
	procDestroyWindow.Call(c.hwnd)
	runtime.UnlockOSThread()
}

// Formats lists the formats on the clipboard, in the owner's order of
// preference.
func (c *Clipboard) Formats() []uint32 {
	var out []uint32
	for f := uintptr(0); ; {
		f, _, _ = procEnumClipboardFormats.Call(f)
		if f == 0 {
			return out
		}
		out = append(out, uint32(f))
	}
}

// Has reports whether format f is on the clipboard.
func (c *Clipboard) Has(f uint32) bool {
	for _, g := range c.Formats() {
		if g == f {
			return true
		}
	}
	return false
}

// Data copies the global-memory data of format f. It returns nil and no
// error when the format is absent.
func (c *Clipboard) Data(f uint32) ([]byte, error) {
	h, _, _ := procGetClipboardData.Call(uintptr(f))
	if h == 0 {
		return nil, nil
	}
	return globalBytes(h)
}

// Owner is the window that put the content on the clipboard, if any.
func (c *Clipboard) Owner() windows.HWND {
	h, _, _ := procGetClipboardOwner.Call()
	return windows.HWND(h)
}

// Empty clears the clipboard and makes this process its owner.
func (c *Clipboard) Empty() { procEmptyClipboard.Call() }

// Set hands the clipboard a global memory copy of data.
func (c *Clipboard) Set(format uint32, data []byte) error {
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

// Text is the clipboard's Unicode text, or "" when it has none.
func (c *Clipboard) Text() (string, error) {
	data, err := c.Data(CFUnicodeText)
	if err != nil || len(data) < 2 {
		return "", err
	}
	return utf16Text(data), nil
}

// excludeFromHistory marks the content being set so clipboard history,
// cloud sync and clipboard managers ignore it.
func (c *Clipboard) excludeFromHistory() {
	zero := []byte{0, 0, 0, 0} // DWORD 0
	c.Set(RegisterFormat("ExcludeClipboardContentFromMonitorProcessing"), zero)
	c.Set(RegisterFormat("CanIncludeInClipboardHistory"), zero)
	c.Set(RegisterFormat("CanUploadToCloudClipboard"), zero)
}

// skipFormat reports formats that cannot be copied byte for byte: GDI or
// private handles rather than global memory, and OLE's markers, whose bytes
// refer to an object in the program that copied. Bitmaps survive anyway:
// Windows also offers them as CF_DIB and synthesises the rest.
func skipFormat(f uint32) bool {
	switch f {
	case CFBitmap, 3, 9, 14, // CF_METAFILEPICT, CF_PALETTE, CF_ENHMETAFILE
		0x80, 0x82, 0x83, 0x8E: // CF_OWNERDISPLAY, CF_DSP*
		return true
	}
	if f >= 0x200 && f <= 0x3FF { // CF_PRIVATE*, CF_GDIOBJ*
		return true
	}
	if f >= 0xC000 {
		switch FormatName(f) {
		case "DataObject", "Ole Private Data":
			return true
		}
	}
	return false
}

// Save copies every format that can be put back with Restore.
func Save(ctx context.Context) ([]Item, error) {
	c, err := Open(ctx)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	var items []Item
	total := 0
	for _, f := range c.Formats() {
		if skipFormat(f) {
			continue
		}
		data, err := c.Data(f)
		if err != nil || data == nil {
			continue
		}
		if total += len(data); total > MaxBytes {
			return nil, ErrTooBig
		}
		items = append(items, Item{f, data})
	}
	return items, nil
}

// WriteOptions says how Write marks what it puts on the clipboard.
type WriteOptions struct {
	// Hidden keeps the content out of clipboard history, cloud sync and
	// clipboard managers.
	Hidden bool
}

// Write replaces the clipboard content with items.
func Write(ctx context.Context, items []Item, opts WriteOptions) error {
	c, err := Open(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	c.Empty()
	for _, it := range items {
		if err := c.Set(it.Format, it.Data); err != nil {
			return fmt.Errorf("set clipboard format %d: %w", it.Format, err)
		}
	}
	if opts.Hidden {
		c.excludeFromHistory()
	}
	return nil
}

// Restore puts saved items back, hidden from clipboard history: the round
// trip is not something the user copied.
func Restore(ctx context.Context, items []Item) error {
	return Write(ctx, items, WriteOptions{Hidden: true})
}

// Text reads the clipboard as text.
func Text(ctx context.Context) (string, error) {
	c, err := Open(ctx)
	if err != nil {
		return "", err
	}
	defer c.Close()
	return c.Text()
}

// UTF16 encodes s as CF_UNICODETEXT data, NUL-terminated.
func UTF16(s string) []byte {
	u, _ := windows.UTF16FromString(s)
	return unsafe.Slice((*byte)(unsafe.Pointer(&u[0])), len(u)*2)
}

func utf16Text(data []byte) string {
	u := unsafe.Slice((*uint16)(unsafe.Pointer(unsafe.SliceData(data))), len(data)/2)
	return windows.UTF16ToString(u)
}

// globalBytes copies the contents of a global memory handle.
func globalBytes(h uintptr) ([]byte, error) {
	n, _, _ := procGlobalSize.Call(h)
	if n == 0 {
		return []byte{}, nil
	}
	if n > MaxBytes {
		return nil, ErrTooBig
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
