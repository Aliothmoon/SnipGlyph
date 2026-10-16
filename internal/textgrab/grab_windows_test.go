package textgrab

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"SnipGlyph/internal/winclip"
)

var (
	procSetForegroundWindowT = user32t.NewProc("SetForegroundWindow")
	procSetFocusT            = user32t.NewProc("SetFocus")
	procTranslateMessageT    = user32t.NewProc("TranslateMessage")
)

// foregroundEdit shows a top-most EDIT control holding text, all of it
// selected, and tries to bring it to the front. stop removes it.
func foregroundEdit(t *testing.T, text string) (windows.HWND, func()) {
	t.Helper()
	ready := make(chan windows.HWND)
	var tid uint32
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		tid = windows.GetCurrentThreadId()
		cls, _ := windows.UTF16PtrFromString("EDIT")
		txt, _ := windows.UTF16PtrFromString(text)
		const wsExTopmost, wsPopup, wsVisible, wsBorder = 0x8, 0x80000000, 0x10000000, 0x00800000
		h, _, _ := procCreateWindowExT.Call(wsExTopmost, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(txt)),
			wsPopup|wsVisible|wsBorder, 200, 200, 400, 60, 0, 0, 0, 0)
		if h == 0 {
			ready <- 0
			return
		}
		const emSetSel = 0x00B1
		procSendMessageT.Call(h, emSetSel, 0, ^uintptr(0))
		procSetForegroundWindowT.Call(h)
		procSetFocusT.Call(h)
		ready <- windows.HWND(h)
		var msg [48]byte
		for {
			r, _, _ := procGetMessageT.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0)
			if int32(r) <= 0 {
				break
			}
			// EDIT copies on the WM_CHAR (0x03) that Ctrl+C translates to.
			procTranslateMessageT.Call(uintptr(unsafe.Pointer(&msg[0])))
			procDispatchMessageT.Call(uintptr(unsafe.Pointer(&msg[0])))
		}
		procDestroyWindowT.Call(h)
	}()
	hwnd := <-ready
	if hwnd == 0 {
		t.Fatal("CreateWindowEx(EDIT) failed")
	}
	return hwnd, func() {
		const wmQuit = 0x0012
		procPostThreadMessageT.Call(uintptr(tid), wmQuit, 0, 0)
		<-done
	}
}

// TestCopyFallback goes through the Ctrl+C path end to end: real key
// presses and the real clipboard, so it only runs with
// SNIPGLYPH_CLIPBOARD_TESTS set, and only sends keys once its own window
// is in front.
func TestCopyFallback(t *testing.T) {
	if os.Getenv("SNIPGLYPH_CLIPBOARD_TESTS") == "" {
		t.Skip("set SNIPGLYPH_CLIPBOARD_TESTS=1 to send Ctrl+C and use the real clipboard")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	before, err := winclip.Text(ctx)
	if err != nil {
		t.Fatal(err)
	}
	beforeSeq := winclip.Sequence()

	hwnd, stop := foregroundEdit(t, "fallback probe text 兜底")
	defer stop()
	for deadline := time.Now().Add(time.Second); windows.GetForegroundWindow() != hwnd; {
		if time.Now().After(deadline) {
			t.Skip("could not bring the test window to the front; not sending keys elsewhere")
		}
		time.Sleep(20 * time.Millisecond)
	}

	got, err := copySelected(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != "fallback probe text 兜底" {
		t.Errorf("copied %q", got)
	}
	if after, _ := winclip.Text(ctx); after != before {
		t.Errorf("clipboard text not restored: %q, was %q", after, before)
	}
	if winclip.Sequence() == beforeSeq {
		t.Error("clipboard never changed: was Ctrl+C sent?")
	}
}
