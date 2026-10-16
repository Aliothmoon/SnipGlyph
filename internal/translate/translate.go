// Package translate turns text into another language through pluggable
// engines (Baidu now, an OpenAI-compatible LLM later).
package translate

import (
	"context"
	"unicode"
)

// Lang is a language tag shared by all engines; each engine maps it to its
// own codes.
type Lang string

const (
	Auto               Lang = "auto"
	ChineseSimplified  Lang = "zh-Hans"
	ChineseTraditional Lang = "zh-Hant"
	English            Lang = "en"
	Japanese           Lang = "ja"
	Korean             Lang = "ko"
	French             Lang = "fr"
	German             Lang = "de"
	Spanish            Lang = "es"
	Russian            Lang = "ru"
)

var langNames = map[Lang]string{
	Auto:               "自动",
	ChineseSimplified:  "简体中文",
	ChineseTraditional: "繁体中文",
	English:            "英语",
	Japanese:           "日语",
	Korean:             "韩语",
	French:             "法语",
	German:             "德语",
	Spanish:            "西班牙语",
	Russian:            "俄语",
}

// Name is the Chinese display name, or the tag itself if unknown.
func (l Lang) Name() string {
	if n, ok := langNames[l]; ok {
		return n
	}
	return string(l)
}

type Request struct {
	Text string
	From Lang // Auto lets the engine detect it
	To   Lang
}

type Result struct {
	Text string
	From Lang // the source language the engine detected, or Request.From
}

// Engine translates text. Engines that stream call partial with the
// translation so far; partial may be nil.
type Engine interface {
	Name() string
	Translate(ctx context.Context, req Request, partial func(text string)) (Result, error)
}

// Direction picks the request languages for text: Chinese goes to English,
// anything else to preferred.
func Direction(text string, preferred Lang) (from, to Lang) {
	src := Detect(text)
	if src == ChineseSimplified {
		return src, English
	}
	if preferred == "" {
		preferred = ChineseSimplified
	}
	return src, preferred
}

// Detect makes a cheap guess at the language of text. It only tells apart
// what Direction needs (Chinese, Japanese, Korean) and returns Auto for the
// rest, leaving the engine to work it out.
func Detect(text string) Lang {
	var han, kana, hangul, letters int
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Han, r):
			han++
		case unicode.In(r, unicode.Hiragana, unicode.Katakana):
			kana++
		case unicode.Is(unicode.Hangul, r):
			hangul++
		case unicode.IsLetter(r):
			letters++
		}
	}
	switch {
	case kana > 0:
		return Japanese
	case hangul > 0 && hangul >= han:
		return Korean
	// Each Han character carries about a word, so weigh it against letters:
	// "Go 语言的 goroutine 调度" is Chinese, "The word 你好 means hello" is not.
	case han > 0 && han*3 >= letters:
		return ChineseSimplified
	}
	return Auto
}
