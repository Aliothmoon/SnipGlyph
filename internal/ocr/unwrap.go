package ocr

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Unwrap rejoins the visual lines of OCR text into paragraphs before
// translation, since translators handle each line on its own. A line
// break is kept after sentence-ending punctuation and around blank lines;
// otherwise lines are joined with a space, or directly next to CJK text.
func Unwrap(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var b strings.Builder
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if i > 0 {
			prev := strings.TrimSpace(lines[i-1])
			switch {
			case prev == "" || line == "" || endsSentence(prev):
				b.WriteByte('\n')
			case strings.HasSuffix(prev, "-") && startsLower(line):
				// "transla-" + "tion": drop the hyphen added by the line wrap.
				s := b.String()
				b.Reset()
				b.WriteString(s[:len(s)-1])
			case lastIsCJK(prev) || firstIsCJK(line):
			default:
				b.WriteByte(' ')
			}
		}
		b.WriteString(line)
	}
	return b.String()
}

func endsSentence(s string) bool {
	r, _ := utf8.DecodeLastRuneInString(s)
	return strings.ContainsRune(".!?:;。！？：；…", r)
}

func startsLower(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsLower(r)
}

func lastIsCJK(s string) bool {
	r, _ := utf8.DecodeLastRuneInString(s)
	return isCJK(r)
}

func firstIsCJK(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return isCJK(r)
}
