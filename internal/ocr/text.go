package ocr

import "unicode"

// fullWidth maps the ASCII punctuation OneOCR emits in place of CJK
// full-width punctuation.
var fullWidth = map[rune]rune{
	',': '，',
	'!': '！',
	':': '：',
	';': '；',
	'?': '？',
	'(': '（',
	')': '）',
}

// restorePunct turns ASCII punctuation back into full-width forms where it
// sits in CJK text: closing marks after a CJK character, '(' before one.
// "时间:12:30" becomes "时间：12:30"; "1,000" and "Hello, 世界" are untouched.
func restorePunct(s string) string {
	rs := []rune(s)
	changed := false
	for i, r := range rs {
		fw, ok := fullWidth[r]
		if !ok {
			continue
		}
		var neighbour rune
		if r == '(' {
			if i+1 < len(rs) {
				neighbour = rs[i+1]
			}
		} else if i > 0 {
			neighbour = rs[i-1]
		}
		if isCJK(neighbour) {
			rs[i] = fw
			changed = true
		}
	}
	if !changed {
		return s
	}
	return string(rs)
}

func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) ||
		(r >= 0x3000 && r <= 0x303F) || // CJK symbols and punctuation
		(r >= 0xFF00 && r <= 0xFFEF) // full-width forms
}

func postprocess(r *Result) {
	for i := range r.Lines {
		r.Lines[i].Text = restorePunct(r.Lines[i].Text)
	}
}
