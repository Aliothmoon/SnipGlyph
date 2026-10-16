package textgrab

import (
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32t                = windows.NewLazySystemDLL("user32.dll")
	procCreateWindowExT    = user32t.NewProc("CreateWindowExW")
	procDestroyWindowT     = user32t.NewProc("DestroyWindow")
	procSendMessageT       = user32t.NewProc("SendMessageW")
	procGetMessageT        = user32t.NewProc("GetMessageW")
	procDispatchMessageT   = user32t.NewProc("DispatchMessageW")
	procPostThreadMessageT = user32t.NewProc("PostThreadMessageW")
)

// classWindow creates a hidden control of class holding text with
// [start, end) selected, on a thread that pumps its messages, as UI
// Automation talks to the control through them. stop tears it down.
func classWindow(t *testing.T, class, text string, start, end int) (hwnd windows.HWND, stop func()) {
	t.Helper()
	ready := make(chan windows.HWND)
	var tid uint32
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		tid = windows.GetCurrentThreadId()
		cls, _ := windows.UTF16PtrFromString(class)
		txt, _ := windows.UTF16PtrFromString(text)
		const wsPopup, esMultiline = 0x80000000, 0x0004
		h, _, _ := procCreateWindowExT.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(txt)),
			wsPopup|esMultiline, 0, 0, 300, 100, 0, 0, 0, 0)
		if h == 0 {
			ready <- 0 // no window: no message loop to leave running
			return
		}
		const emSetSel = 0x00B1
		procSendMessageT.Call(h, emSetSel, uintptr(start), uintptr(end))
		ready <- windows.HWND(h)
		var msg [48]byte
		for {
			r, _, _ := procGetMessageT.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0)
			if int32(r) <= 0 {
				break
			}
			procDispatchMessageT.Call(uintptr(unsafe.Pointer(&msg[0])))
		}
		procDestroyWindowT.Call(h)
	}()
	hwnd = <-ready
	if hwnd == 0 {
		t.Fatal("CreateWindowEx(EDIT) failed")
	}
	return hwnd, func() {
		const wmQuit = 0x0012
		procPostThreadMessageT.Call(uintptr(tid), wmQuit, 0, 0)
		<-done
	}
}

// RichEdit implements the UIA text pattern itself, like the new Notepad,
// browsers and Word do.
func richEdit(t *testing.T, text string, start, end int) (windows.HWND, func()) {
	if err := windows.NewLazySystemDLL("msftedit.dll").Load(); err != nil {
		t.Skip("msftedit.dll unavailable")
	}
	return classWindow(t, "RICHEDIT50W", text, start, end)
}

func TestUIASelection(t *testing.T) {
	hwnd, stop := richEdit(t, "hello 世界 world", 6, 8)
	defer stop()
	got, err := withAutomation(func(uia *comObject) (string, error) { return windowSelection(uia, hwnd) })
	if err != nil {
		t.Fatal(err)
	}
	if got != "世界" {
		t.Fatalf("selection = %q, want %q", got, "世界")
	}
}

func TestUIAEmptySelection(t *testing.T) {
	hwnd, stop := richEdit(t, "nothing selected", 3, 3)
	defer stop()
	got, err := withAutomation(func(uia *comObject) (string, error) { return windowSelection(uia, hwnd) })
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("selection = %q, want empty", got)
	}
}

// The classic EDIT control has no text pattern: Selected must fall back to
// copying.
func TestUIAClassicEditHasNoTextPattern(t *testing.T) {
	hwnd, stop := classWindow(t, "EDIT", "hello world", 0, 5)
	defer stop()
	_, err := withAutomation(func(uia *comObject) (string, error) { return windowSelection(uia, hwnd) })
	if err != errNoTextPattern {
		t.Fatalf("err = %v, want errNoTextPattern", err)
	}
}
