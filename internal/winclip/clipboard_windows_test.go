package winclip

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"
)

func findItem(items []Item, f uint32) []byte {
	for _, it := range items {
		if it.Format == f {
			return it.Data
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

	original, err := Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	originalText, _ := Text(ctx)
	defer func() {
		if err := Restore(ctx, original); err != nil {
			t.Errorf("put the original clipboard back: %v", err)
		}
		if got, _ := Text(ctx); got != originalText {
			t.Errorf("original clipboard text not restored: %q", got)
		}
	}()

	custom := RegisterFormat("SnipGlyphTestFormat")
	blob := []byte{1, 2, 3, 0, 255}
	if err := Restore(ctx, []Item{
		{CFUnicodeText, UTF16("原文 test")},
		{custom, blob},
	}); err != nil {
		t.Fatal(err)
	}

	saved, err := Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := findItem(saved, custom); !bytes.HasPrefix(got, blob) {
		t.Fatalf("custom format saved as %v", got)
	}

	if err := Restore(ctx, []Item{{CFUnicodeText, UTF16("other")}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := Text(ctx); got != "other" {
		t.Fatalf("overwrite: %q", got)
	}

	if err := Restore(ctx, saved); err != nil {
		t.Fatal(err)
	}
	if got, _ := Text(ctx); got != "原文 test" {
		t.Errorf("restored text %q", got)
	}
	again, err := Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := findItem(again, custom); !bytes.HasPrefix(got, blob) {
		t.Errorf("custom format restored as %v", got)
	}
}
