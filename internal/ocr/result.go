// Package ocr recognises text in screenshots with OneOCR, the engine inside
// the Windows 11 Snipping Tool. The engine runs in a worker process (see
// Client and ServeWorker); everything here is plain data shared by both ends.
package ocr

import (
	"math"
	"strings"
)

// Point is in pixel coordinates of the recognised image.
type Point struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
}

// Quad is a possibly rotated box: four corners clockwise from top-left.
type Quad [4]Point

type Word struct {
	Text       string  `json:"text"`
	Box        Quad    `json:"box"`
	Confidence float32 `json:"conf"`
}

type Line struct {
	Text  string `json:"text"`
	Box   Quad   `json:"box"`
	Words []Word `json:"words,omitempty"`
}

// Confidence is the lowest word confidence; OneOCR has no line-level score.
// A line without words reports 0.
func (l Line) Confidence() float32 {
	if len(l.Words) == 0 {
		return 0
	}
	m := float32(math.MaxFloat32)
	for _, w := range l.Words {
		m = min(m, w.Confidence)
	}
	return m
}

type Result struct {
	Angle float32 `json:"angle"` // detected text rotation in degrees
	Lines []Line  `json:"lines"`
}

// Text is the recognised text with one line per OCR line, as shown to and
// copied by the user.
func (r Result) Text() string {
	var b strings.Builder
	for i, l := range r.Lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(l.Text)
	}
	return b.String()
}

// mapPoints applies f to every box corner in r.
func (r *Result) mapPoints(f func(Point) Point) {
	for i := range r.Lines {
		l := &r.Lines[i]
		for k := range l.Box {
			l.Box[k] = f(l.Box[k])
		}
		for j := range l.Words {
			for k := range l.Words[j].Box {
				l.Words[j].Box[k] = f(l.Words[j].Box[k])
			}
		}
	}
}
