// Package cliphist keeps a history of what was copied to the clipboard:
// text with its HTML and RTF, images and file lists, organised in tabs.
package cliphist

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"image"
	"image/png"
	"strings"
	"time"

	"golang.org/x/image/draw"

	"SnipGlyph/internal/winclip"
)

// Kind is what an item holds.
type Kind string

const (
	Text  Kind = "text"  // text, maybe with HTML and RTF
	Image Kind = "image" // a picture, stored as PNG
	Files Kind = "files" // file paths, pasted back as a file copy
)

// Item is one clipboard entry with everything needed to put it back.
type Item struct {
	ID     int64
	Tab    int64
	Kind   Kind
	Text   string // the text; for Files the paths, one per line
	HTML   []byte // "HTML Format" as copied
	RTF    []byte // "Rich Text Format" as copied
	PNG    []byte
	Thumb  []byte // small PNG for the list
	Width  int    // of the image
	Height int
	Source string // the program that copied it, e.g. "chrome.exe"
	Pinned bool
	Hash   []byte // identifies the content, for spotting copies of it
	Used   time.Time
}

// Paths returns the file paths of a Files item.
func (it Item) Paths() []string {
	if it.Kind != Files || it.Text == "" {
		return nil
	}
	return strings.Split(it.Text, "\n")
}

// Snapshot is what was read from the clipboard, before it becomes an item.
type Snapshot struct {
	// Private means the program asked clipboard managers not to keep it,
	// as password managers do.
	Private bool
	Text    string
	HTML    []byte
	RTF     []byte
	PNG     []byte // the "PNG" format, if offered
	DIB     []byte // CF_DIBV5 or CF_DIB, if offered and there is no PNG
	Files   []string
	Source  string
}

var (
	// ErrSkip means the snapshot has nothing worth keeping.
	ErrSkip = errors.New("nothing to keep")
	// ErrTooLarge means an image exceeds MaxPixels.
	ErrTooLarge = errors.New("image too large to keep")
)

// MaxPixels is the largest image kept (64 megapixels, about 8K x 8K).
const MaxPixels = 64 << 20

// Thumbnail bounds, in pixels: crisp at 200% scaling in the list.
const thumbW, thumbH = 320, 200

// Item turns a snapshot into an item. Text wins over an image (Excel and
// Word offer a picture of every copy), an image over a file list (chat
// programs offer the temporary file of a copied picture).
func (s Snapshot) Item() (Item, error) {
	switch {
	case s.Private:
		return Item{}, ErrSkip
	case strings.TrimSpace(s.Text) != "":
		return Item{Kind: Text, Text: s.Text, HTML: s.HTML, RTF: s.RTF, Source: s.Source,
			Hash: hash(Text, []byte(s.Text))}, nil
	case len(s.PNG) > 0 || len(s.DIB) > 0:
		it, err := imageItem(s.PNG, s.DIB)
		if err != nil {
			return Item{}, err
		}
		it.HTML, it.Source = s.HTML, s.Source
		return it, nil
	case len(s.Files) > 0:
		text := strings.Join(s.Files, "\n")
		return Item{Kind: Files, Text: text, Source: s.Source, Hash: hash(Files, []byte(text))}, nil
	}
	return Item{}, ErrSkip
}

func imageItem(pngData, dib []byte) (Item, error) {
	var img image.Image
	var err error
	if len(pngData) > 0 {
		if cfg, err := png.DecodeConfig(bytes.NewReader(pngData)); err == nil && cfg.Width*cfg.Height > MaxPixels {
			return Item{}, ErrTooLarge
		}
		img, err = png.Decode(bytes.NewReader(pngData))
	} else {
		img, err = winclip.DecodeDIB(dib)
	}
	if err != nil {
		return Item{}, err
	}
	b := img.Bounds()
	if b.Dx()*b.Dy() > MaxPixels {
		return Item{}, ErrTooLarge
	}
	nrgba := toNRGBA(img)
	if len(pngData) == 0 {
		if pngData, err = encodePNG(nrgba); err != nil {
			return Item{}, err
		}
	}
	thumb, err := encodePNG(thumbnail(nrgba))
	if err != nil {
		return Item{}, err
	}
	size := []byte{byte(b.Dx() >> 8), byte(b.Dx()), byte(b.Dy() >> 8), byte(b.Dy())}
	return Item{Kind: Image, PNG: pngData, Thumb: thumb, Width: b.Dx(), Height: b.Dy(),
		Hash: hash(Image, size, nrgba.Pix)}, nil
}

func hash(k Kind, parts ...[]byte) []byte {
	h := sha256.New()
	h.Write([]byte(k))
	h.Write([]byte{0})
	for _, p := range parts {
		h.Write(p)
	}
	return h.Sum(nil)
}

func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) {
		return n
	}
	b := img.Bounds()
	n := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(n, n.Rect, img, b.Min, draw.Src)
	return n
}

// thumbnail scales img down to fit thumbW x thumbH.
func thumbnail(img *image.NRGBA) image.Image {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w <= thumbW && h <= thumbH {
		return img
	}
	scale := min(float64(thumbW)/float64(w), float64(thumbH)/float64(h))
	tw, th := max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
	dst := image.NewNRGBA(image.Rect(0, 0, tw, th))
	draw.BiLinear.Scale(dst, dst.Rect, img, img.Rect, draw.Src, nil)
	return dst
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Preview returns up to n non-blank lines of text, each with its runs of
// spaces and tabs squeezed, for showing in a list.
func Preview(text string, n int) []string {
	var out []string
	for line := range strings.Lines(text) {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		if out = append(out, line); len(out) == n {
			break
		}
	}
	return out
}
