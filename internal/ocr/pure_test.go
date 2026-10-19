package ocr

import "testing"

func TestRestorePunct(t *testing.T) {
	tests := map[string]string{
		"你好,世界!":         "你好，世界！",
		"时间:12:30":       "时间：12:30",
		"1,000":          "1,000",
		"Hello, 世界":      "Hello, 世界",
		"测试(括号)。":        "测试（括号）。",
		"call f(x)":      "call f(x)",
		"真的吗?Yes!":       "真的吗？Yes!",
		"":               "",
		"(注)":            "（注）",
		"版本:v1.2; 状态:ok": "版本：v1.2; 状态：ok",
		"日本語です!カタカナ:テスト": "日本語です！カタカナ：テスト",
	}
	for in, want := range tests {
		if got := restorePunct(in); got != want {
			t.Errorf("restorePunct(%q) = %q, want %q", in, got, want)
		}
	}
}

func solid(w, h int, b, g, r byte) Image {
	img := Image{Width: w, Height: h, Pix: make([]byte, w*h*4)}
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = b, g, r, 0xFF
	}
	return img
}

func TestFitPadsSmallImages(t *testing.T) {
	img := solid(200, 20, 10, 20, 30)
	out, back := fit(img)
	if out.Width != 200 || out.Height != 50 {
		t.Fatalf("size = %dx%d, want 200x50", out.Width, out.Height)
	}
	// Padding takes the mean edge colour.
	if got := out.Pix[:4]; got[0] != 10 || got[1] != 20 || got[2] != 30 {
		t.Fatalf("pad colour = %v", got)
	}
	// Point (5, 15+offset) in the padded image is (5, 15) in the original.
	if p := back(Point{5, 15 + 15}); p != (Point{5, 15}) {
		t.Fatalf("back = %v, want {5 15}", p)
	}
}

func TestFitPadsOneShortSide(t *testing.T) {
	img := solid(200, minSide-1, 10, 20, 30)
	out, back := fit(img)
	if out.Width != 200 || out.Height != minSide {
		t.Fatalf("size = %dx%d, want 200x%d", out.Width, out.Height, minSide)
	}
	if p := back(Point{5, 15}); p != (Point{5, 15}) {
		t.Fatalf("back = %v, want {5 15}", p)
	}
}

func TestFitShrinksHugeImages(t *testing.T) {
	img := solid(10001, 60, 0, 0, 0)
	out, back := fit(img)
	if out.Width != 5000 || out.Height != 30+20 {
		// 10001/2 = 5000 wide; 60/2 = 30 tall, then padded to 50.
		t.Fatalf("size = %dx%d, want 5000x50", out.Width, out.Height)
	}
	if p := back(Point{100, 10 + 10}); p != (Point{200, 20}) {
		t.Fatalf("back = %v, want {200 20}", p)
	}
}

func TestFitLeavesNormalImagesAlone(t *testing.T) {
	img := solid(300, 80, 1, 2, 3)
	out, back := fit(img)
	if &out.Pix[0] != &img.Pix[0] {
		t.Fatal("normal image was copied")
	}
	if p := back(Point{7, 9}); p != (Point{7, 9}) {
		t.Fatalf("back = %v", p)
	}
}

func TestLineConfidence(t *testing.T) {
	l := Line{Words: []Word{{Confidence: 0.9}, {Confidence: 0.7}, {Confidence: 0.8}}}
	if c := l.Confidence(); c != 0.7 {
		t.Fatalf("Confidence = %v, want 0.7", c)
	}
	if c := (Line{}).Confidence(); c != 0 {
		t.Fatalf("empty Confidence = %v, want 0", c)
	}
}

func TestUnwrap(t *testing.T) {
	tests := []struct{ in, want string }{
		{"The quick brown fox\njumps over the dog.\nNext sentence", "The quick brown fox jumps over the dog.\nNext sentence"},
		{"这是一段被截图\n折行的中文。\n第二段", "这是一段被截图折行的中文。\n第二段"},
		{"para one\n\npara two", "para one\n\npara two"},
		{"a transla-\ntion error", "a translation error"},
		{"well-\nKnown", "well- Known"},
		{"混合 mixed\ntext 文本", "混合 mixed text 文本"},
		{"Title:\nbody", "Title:\nbody"},
		{"  padded  \r\n  lines ", "padded lines"},
	}
	for _, tt := range tests {
		if got := Unwrap(tt.in); got != tt.want {
			t.Errorf("Unwrap(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
