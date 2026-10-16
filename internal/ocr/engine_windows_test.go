package ocr

import (
	"errors"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func loadPNG(t testing.TB, path string) Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return FromImage(src)
}

// testCacheDir persists across runs: the staged DLLs stay loaded until the
// test process exits, so a per-test temp dir could never be cleaned up.
var testCacheDir = filepath.Join(os.TempDir(), "snipglyph-ocr-test")

var testEngine = sync.OnceValues(func() (*Engine, error) {
	inst, err := Locate()
	if err != nil {
		return nil, err
	}
	return OpenEngine(inst, testCacheDir)
})

// openTestEngine needs a real Snipping Tool; it skips where there is none.
func openTestEngine(t *testing.T) *Engine {
	t.Helper()
	e, err := testEngine()
	if errors.Is(err, ErrNotInstalled) {
		t.Skip("Snipping Tool not installed")
	}
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestEngineMixedText(t *testing.T) {
	e := openTestEngine(t)
	res, err := e.Recognize(loadPNG(t, "testdata/mixed.png"))
	if err != nil {
		t.Fatal(err)
	}
	got := res.Text()
	t.Logf("recognised:\n%s", got)
	for _, want := range []string{
		"Hello, OneOCR! The quick brown fox.",
		"你好，世界！这是一段中文：测试（括号）。",
		"时间：12:30",
		"1,000",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	if len(res.Lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(res.Lines))
	}
	if c := res.Lines[0].Confidence(); c < 0.8 {
		t.Errorf("first line confidence %v, want >= 0.8", c)
	}
}

func TestEngineThinImageIsPaddedAndMappedBack(t *testing.T) {
	e := openTestEngine(t)
	img := loadPNG(t, "testdata/thin.png") // 30 px tall: below OneOCR's minimum
	res, err := e.Recognize(img)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Lines) != 1 || !strings.Contains(res.Lines[0].Text, "thin line") {
		t.Fatalf("got %q", res.Text())
	}
	for _, p := range res.Lines[0].Box {
		if p.Y < -2 || p.Y > float32(img.Height)+2 {
			t.Errorf("box corner %v outside the original %d px height", p, img.Height)
		}
	}
}

func TestSelfTest(t *testing.T) {
	e := openTestEngine(t)
	if err := selfTest(e); err != nil {
		t.Fatal(err)
	}
}
