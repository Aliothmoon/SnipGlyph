// Command genicon draws the SnipGlyph app icon (a screenshot frame around
// lines of text) and writes it as a 256x256 PNG for go-winres.
//
//	go run ./tools/genicon -o winres/icon.png
package main

import (
	"flag"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
)

const (
	size = 256
	ss   = 4 // supersampling factor per axis
)

type capsule struct{ x1, y1, x2, y2, r float64 }

func (c capsule) contains(x, y float64) bool {
	dx, dy := c.x2-c.x1, c.y2-c.y1
	t := 0.0
	if l := dx*dx + dy*dy; l > 0 {
		t = math.Max(0, math.Min(1, ((x-c.x1)*dx+(y-c.y1)*dy)/l))
	}
	px, py := c.x1+t*dx-x, c.y1+t*dy-y
	return px*px+py*py <= c.r*c.r
}

func inRoundedRect(x, y, x0, y0, x1, y1, r float64) bool {
	cx, cy := (x0+x1)/2, (y0+y1)/2
	dx := math.Max(math.Abs(x-cx)-((x1-x0)/2-r), 0)
	dy := math.Max(math.Abs(y-cy)-((y1-y0)/2-r), 0)
	return dx*dx+dy*dy <= r*r
}

func main() {
	out := flag.String("o", "winres/icon.png", "output PNG path")
	flag.Parse()

	var white []capsule
	// Screenshot corner brackets.
	const lo, hi, arm, thick = 56.0, 200.0, 44.0, 9.0
	for _, c := range [][2]float64{{lo, lo}, {hi, lo}, {lo, hi}, {hi, hi}} {
		sx, sy := 1.0, 1.0
		if c[0] == hi {
			sx = -1
		}
		if c[1] == hi {
			sy = -1
		}
		white = append(white,
			capsule{c[0], c[1], c[0] + sx*arm, c[1], thick},
			capsule{c[0], c[1], c[0], c[1] + sy*arm, thick},
		)
	}
	// Lines of recognised text.
	for i, end := range []float64{172, 150, 160} {
		y := 100 + float64(i)*28
		white = append(white, capsule{90, y, end, y, 8})
	}

	top := color.NRGBA{0x4F, 0x8D, 0xF7, 0xFF}
	bottom := color.NRGBA{0x1D, 0x4E, 0xD8, 0xFF}

	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := float64(px) + (float64(sx)+0.5)/ss
					y := float64(py) + (float64(sy)+0.5)/ss
					if !inRoundedRect(x, y, 8, 8, 248, 248, 56) {
						continue
					}
					c := lerp(top, bottom, y/size)
					for _, w := range white {
						if w.contains(x, y) {
							c = color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}
							break
						}
					}
					r += float64(c.R)
					g += float64(c.G)
					b += float64(c.B)
					a++
				}
			}
			if a == 0 {
				continue
			}
			img.SetNRGBA(px, py, color.NRGBA{
				uint8(r / a), uint8(g / a), uint8(b / a), uint8(255 * a / (ss * ss)),
			})
		}
	}

	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}

func lerp(a, b color.NRGBA, t float64) color.NRGBA {
	m := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return color.NRGBA{m(a.R, b.R), m(a.G, b.G), m(a.B, b.B), 0xFF}
}
