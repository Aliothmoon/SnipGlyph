package translate

import "testing"

func TestDetect(t *testing.T) {
	tests := map[string]Lang{
		"这是一段中文。":                 ChineseSimplified,
		"Go 语言的 goroutine 调度":     ChineseSimplified,
		"The word 你好 means hello": Auto,
		"Hello, world":            Auto,
		"日本語のテキストです":              Japanese,
		"漢字だけでもカナが一つ":             Japanese,
		"안녕하세요 세계":                Korean,
		"Привет, мир":             Auto,
		"":                        Auto,
		"12345 !!!":               Auto,
		"時間:12:30 價格":             ChineseSimplified, // traditional is still Chinese
	}
	for in, want := range tests {
		if got := Detect(in); got != want {
			t.Errorf("Detect(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestDirection(t *testing.T) {
	if from, to := Direction("中文文本", ChineseSimplified); from != ChineseSimplified || to != English {
		t.Errorf("zh: %s -> %s", from, to)
	}
	if from, to := Direction("English text", ""); from != Auto || to != ChineseSimplified {
		t.Errorf("en with default: %s -> %s", from, to)
	}
	if _, to := Direction("English text", Japanese); to != Japanese {
		t.Errorf("preferred target ignored: %s", to)
	}
}
