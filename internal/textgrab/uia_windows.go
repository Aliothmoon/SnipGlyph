package textgrab

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ole32                = windows.NewLazySystemDLL("ole32.dll")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	oleaut32             = windows.NewLazySystemDLL("oleaut32.dll")
	procSysStringLen     = oleaut32.NewProc("SysStringLen")
	procSysFreeString    = oleaut32.NewProc("SysFreeString")

	clsidCUIAutomation          = mustGUID("{ff48dba4-60ef-4201-aa87-54103eef594e}")
	iidIUIAutomation            = mustGUID("{30cbe57d-d9d0-452a-ab13-7ac5ac4825ee}")
	iidIUIAutomationTextPattern = mustGUID("{32eba289-3583-42c9-9c59-3b6d9a1e9b6a}")
)

const uiaTextPatternID = 10014

// Vtable slots, counting IUnknown's three, from UIAutomationClient.h.
const (
	slotRelease = 2

	automationElementFromHandle = 6
	automationGetFocusedElement = 8

	elementGetCurrentPatternAs = 14

	textPatternGetSelection = 5

	rangeArrayLength     = 3
	rangeArrayGetElement = 4

	textRangeGetText = 12
)

var errNoTextPattern = errors.New("element has no text pattern")

func mustGUID(s string) windows.GUID {
	g, err := windows.GUIDFromString(s)
	if err != nil {
		panic(err)
	}
	return g
}

// comObject is any COM interface pointer: the object starts with a pointer
// to its vtable.
type comObject struct{ vtbl *[64]uintptr }

func (o *comObject) call(slot int, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(o.vtbl[slot], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return r
}

func (o *comObject) release() { o.call(slotRelease) }

func failed(hr uintptr) bool { return int32(hr) < 0 }

// withAutomation runs f with a UI Automation client on a fresh MTA thread.
// UI Automation clients belong off any thread with windows, since providers
// may call back into it.
func withAutomation(f func(uia *comObject) (string, error)) (string, error) {
	type result struct {
		s   string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		runtime.LockOSThread() // never unlocked: the thread dies with the goroutine
		if err := windows.CoInitializeEx(0, windows.COINIT_MULTITHREADED); err != nil {
			ch <- result{err: fmt.Errorf("CoInitializeEx: %w", err)}
			return
		}
		defer windows.CoUninitialize()
		var uia *comObject
		hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidCUIAutomation)), 0,
			windows.CLSCTX_INPROC_SERVER, uintptr(unsafe.Pointer(&iidIUIAutomation)), uintptr(unsafe.Pointer(&uia)))
		if failed(hr) || uia == nil {
			ch <- result{err: fmt.Errorf("create UI Automation: %#x", uint32(hr))}
			return
		}
		defer uia.release()
		s, err := f(uia)
		ch <- result{s, err}
	}()
	r := <-ch
	return r.s, r.err
}

// focusedSelection reads the selected text of the focused element.
func focusedSelection(uia *comObject) (string, error) {
	var elem *comObject
	if hr := uia.call(automationGetFocusedElement, uintptr(unsafe.Pointer(&elem))); failed(hr) || elem == nil {
		return "", fmt.Errorf("GetFocusedElement: %#x", uint32(hr))
	}
	defer elem.release()
	return selectionOf(elem)
}

// windowSelection reads the selected text of the element for hwnd.
func windowSelection(uia *comObject, hwnd windows.HWND) (string, error) {
	var elem *comObject
	if hr := uia.call(automationElementFromHandle, uintptr(hwnd), uintptr(unsafe.Pointer(&elem))); failed(hr) || elem == nil {
		return "", fmt.Errorf("ElementFromHandle: %#x", uint32(hr))
	}
	defer elem.release()
	return selectionOf(elem)
}

func selectionOf(elem *comObject) (string, error) {
	var pattern *comObject
	hr := elem.call(elementGetCurrentPatternAs, uiaTextPatternID,
		uintptr(unsafe.Pointer(&iidIUIAutomationTextPattern)), uintptr(unsafe.Pointer(&pattern)))
	if failed(hr) || pattern == nil {
		return "", errNoTextPattern
	}
	defer pattern.release()

	var ranges *comObject
	if hr := pattern.call(textPatternGetSelection, uintptr(unsafe.Pointer(&ranges))); failed(hr) || ranges == nil {
		return "", fmt.Errorf("GetSelection: %#x", uint32(hr))
	}
	defer ranges.release()
	var n int32
	ranges.call(rangeArrayLength, uintptr(unsafe.Pointer(&n)))

	var parts []string
	for i := range n {
		var r *comObject
		if hr := ranges.call(rangeArrayGetElement, uintptr(i), uintptr(unsafe.Pointer(&r))); failed(hr) || r == nil {
			continue
		}
		var bstr *uint16
		hr := r.call(textRangeGetText, ^uintptr(0) /* -1: no limit */, uintptr(unsafe.Pointer(&bstr)))
		r.release()
		if failed(hr) || bstr == nil {
			continue
		}
		parts = append(parts, takeBSTR(bstr))
	}
	return strings.Join(parts, "\n"), nil
}

func takeBSTR(b *uint16) string {
	n, _, _ := procSysStringLen.Call(uintptr(unsafe.Pointer(b)))
	s := windows.UTF16ToString(unsafe.Slice(b, n))
	procSysFreeString.Call(uintptr(unsafe.Pointer(b)))
	return s
}
