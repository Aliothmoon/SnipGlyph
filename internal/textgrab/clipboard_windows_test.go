package textgrab

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func utf16Bytes(s string) []byte {
	u, _ := windows.UTF16FromString(s)
	return unsafe.Slice((*byte)(unsafe.Pointer(&u[0])), len(u)*2)
}

func findItem(items []clipItem, f uint32) []byte {
	for _, it := range items {
		if it.format == f {
			return it.data
		}
	}
	return nil
}

// TestClipboardRoundTrip goes through the real clipboard, so it only runs
// with SNIPGLYPH_CLIPBOARD_TESTS set. It puts the clipboard back as it was,
// using the code under test; writes are kept out of clipboard history.
func TestClipboardRoundTrip(t *testing.T) {
	if os.Getenv("SNIPGLYPH_CLIPBOARD_TESTS") == "" {
		t.Skip("set SNIPGLYPH_CLIPBOARD_TESTS=1 to use the real clipboard")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	original, err := saveClipboard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	originalText, _ := clipboardText(ctx)
	defer func() {
		if err := restoreClipboard(ctx, original); err != nil {
			t.Errorf("put the original clipboard back: %v", err)
		}
		if got, _ := clipboardText(ctx); got != originalText {
			t.Errorf("original clipboard text not restored: %q", got)
		}
	}()

	name, _ := windows.UTF16PtrFromString("SnipGlyphTestFormat")
	custom, _, _ := procRegisterClipboardFormat.Call(uintptr(unsafe.Pointer(name)))
	blob := []byte{1, 2, 3, 0, 255}
	if err := restoreClipboard(ctx, []clipItem{
		{cfUnicodeText, utf16Bytes("原文 test")},
		{uint32(custom), blob},
	}); err != nil {
		t.Fatal(err)
	}

	saved, err := saveClipboard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := findItem(saved, uint32(custom)); !bytes.HasPrefix(got, blob) {
		t.Fatalf("custom format saved as %v", got)
	}

	if err := restoreClipboard(ctx, []clipItem{{cfUnicodeText, utf16Bytes("other")}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := clipboardText(ctx); got != "other" {
		t.Fatalf("overwrite: %q", got)
	}

	if err := restoreClipboard(ctx, saved); err != nil {
		t.Fatal(err)
	}
	if got, _ := clipboardText(ctx); got != "原文 test" {
		t.Errorf("restored text %q", got)
	}
	again, err := saveClipboard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := findItem(again, uint32(custom)); !bytes.HasPrefix(got, blob) {
		t.Errorf("custom format restored as %v", got)
	}
}
