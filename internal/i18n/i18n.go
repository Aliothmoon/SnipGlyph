// Package i18n provides lightweight, type-safe UI localization for SnipGlyph.
package i18n

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// Lang identifies a supported UI language preference.
type Lang string

const (
	Auto Lang = "auto"
	ZhCN Lang = "zh-CN"
	En   Lang = "en"
)

var cur atomic.Pointer[Catalog]

func init() {
	// Default to zh-CN so unit tests that do not call Set stay deterministic
	// regardless of the host OS locale. cmd/snipglyph calls Set(Auto) at startup.
	cur.Store(&zhCN)
}

// Parse normalizes a language setting string to a supported Lang.
func Parse(s string) Lang {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto":
		return Auto
	case "zh", "zh-cn", "zh_cn", "zh-hans":
		return ZhCN
	case "en", "en-us", "en_us", "en-gb", "en_gb":
		return En
	}
	return Auto
}

// Set activates the catalog for pref ("auto", "zh-CN", "en") and returns the
// resolved active language (ZhCN or En).
func Set(pref Lang) Lang {
	resolved := pref
	if resolved == "" || resolved == Auto {
		resolved = SystemLang()
	}
	switch resolved {
	case En:
		cur.Store(&enUS)
		return En
	default:
		cur.Store(&zhCN)
		return ZhCN
	}
}

// Current returns the active translation catalog. Safe for concurrent use.
func Current() *Catalog {
	return cur.Load()
}

// Active returns the currently active resolved language (ZhCN or En).
func Active() Lang {
	return cur.Load().Lang
}

// CopiedLines formats the silent OCR copy toast for n recognized lines.
func (c *Catalog) CopiedLines(n int) string {
	if n == 1 {
		return c.CopiedLinesOne
	}
	return fmt.Sprintf(c.CopiedLinesMany, n)
}
