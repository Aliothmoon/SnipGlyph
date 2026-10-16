// Package toast shows short-lived messages: a translucent rounded bar near
// the bottom of the screen that fades in and out, never takes focus and lets
// clicks through. Used where a system notification would be too heavy, such
// as confirming a silent OCR copy.
package toast

import (
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

// Timing of a toast: fade in, stay, fade out.
const (
	fadeIn  = 120 * time.Millisecond
	fadeOut = 250 * time.Millisecond
	minHold = 1400 * time.Millisecond
	maxHold = 4 * time.Second
)

// Limits on a toast's text: it is a glance, not a dialog, and an error
// carrying a whole HTTP body must not grow taller than the screen.
const (
	maxLines = 4
	maxRunes = 160
)

// clip shortens text to maxLines lines and maxRunes characters.
func clip(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	cut := len(lines) > maxLines
	if cut {
		lines = lines[:maxLines]
	}
	text = strings.Join(lines, "\n")
	if r := []rune(text); len(r) > maxRunes {
		text, cut = string(r[:maxRunes]), true
	}
	if cut {
		text = strings.TrimRight(text, " \r\n") + "…"
	}
	return text
}

// holdFor gives longer messages more time to be read.
func holdFor(text string) time.Duration {
	extra := time.Duration(max(0, utf8.RuneCountInString(text)-12)) * 60 * time.Millisecond
	return min(minHold+extra, maxHold)
}

// opacity is the toast's alpha (0..1) at elapsed time t of a toast held for
// hold; ok is false once it has fully faded out.
func opacity(t, hold time.Duration) (a float64, ok bool) {
	switch {
	case t < fadeIn:
		return float64(t) / float64(fadeIn), true
	case t < fadeIn+hold:
		return 1, true
	case t < fadeIn+hold+fadeOut:
		return 1 - float64(t-fadeIn-hold)/float64(fadeOut), true
	}
	return 0, false
}

// coverage is how much of pixel (x, y) lies inside a w x h rectangle with
// corner radius r, from 0 to 1, sampled 4x4 for smooth edges.
func coverage(x, y, w, h int, r float64) float64 {
	const n = 4
	in := 0
	for sy := range n {
		for sx := range n {
			px := float64(x) + (float64(sx)+0.5)/n
			py := float64(y) + (float64(sy)+0.5)/n
			if insideRounded(px, py, float64(w), float64(h), r) {
				in++
			}
		}
	}
	return float64(in) / (n * n)
}

func insideRounded(px, py, w, h, r float64) bool {
	dx := math.Max(math.Abs(px-w/2)-(w/2-r), 0)
	dy := math.Max(math.Abs(py-h/2)-(h/2-r), 0)
	return dx*dx+dy*dy <= r*r
}
