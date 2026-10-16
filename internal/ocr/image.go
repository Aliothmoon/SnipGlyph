package ocr

import "image"

// Image is a top-down 32-bit BGRA bitmap without row padding.
type Image struct {
	Width, Height int
	Pix           []byte // len == Width*Height*4
}

// FromImage converts src to BGRA.
func FromImage(src image.Image) Image {
	b := src.Bounds()
	out := Image{Width: b.Dx(), Height: b.Dy(), Pix: make([]byte, b.Dx()*b.Dy()*4)}
	i := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := src.At(x, y).RGBA()
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = byte(bl>>8), byte(g>>8), byte(r>>8), byte(a>>8)
			i += 4
		}
	}
	return out
}

// OneOCR rejects images with a side outside [minSide, maxSide].
const (
	minSide = 50
	maxSide = 10000
)

// fit returns a version of img that OneOCR accepts, plus the mapping from
// its coordinates back to img's. Oversized images are box-filtered down by an
// integer factor; undersized ones are centred on a canvas filled with the
// mean edge colour, so the padding reads as more background.
func fit(img Image) (Image, func(Point) Point) {
	scale := 1
	for max(img.Width, img.Height) > maxSide*scale {
		scale++
	}
	if scale > 1 {
		img = shrink(img, scale)
	}
	padX := max(0, minSide-img.Width) / 2
	padY := max(0, minSide-img.Height) / 2
	if padX > 0 || padY > 0 {
		img = pad(img, max(img.Width, minSide), max(img.Height, minSide), padX, padY)
	}
	s := float32(scale)
	ox, oy := float32(padX), float32(padY)
	return img, func(p Point) Point {
		return Point{(p.X - ox) * s, (p.Y - oy) * s}
	}
}

func shrink(img Image, k int) Image {
	w, h := img.Width/k, img.Height/k
	out := Image{Width: w, Height: h, Pix: make([]byte, w*h*4)}
	n := k * k
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var sum [4]int
			for dy := 0; dy < k; dy++ {
				row := ((y*k+dy)*img.Width + x*k) * 4
				for dx := 0; dx < k*4; dx++ {
					sum[dx%4] += int(img.Pix[row+dx])
				}
			}
			o := (y*w + x) * 4
			for c := range 4 {
				out.Pix[o+c] = byte(sum[c] / n)
			}
		}
	}
	return out
}

func pad(img Image, w, h, offX, offY int) Image {
	fill := edgeMean(img)
	out := Image{Width: w, Height: h, Pix: make([]byte, w*h*4)}
	for i := 0; i < len(out.Pix); i += 4 {
		copy(out.Pix[i:i+4], fill[:])
	}
	for y := 0; y < img.Height; y++ {
		src := img.Pix[y*img.Width*4 : (y+1)*img.Width*4]
		copy(out.Pix[((y+offY)*w+offX)*4:], src)
	}
	return out
}

func edgeMean(img Image) [4]byte {
	var sum [4]int
	n := 0
	add := func(x, y int) {
		o := (y*img.Width + x) * 4
		for c := range 4 {
			sum[c] += int(img.Pix[o+c])
		}
		n++
	}
	for x := 0; x < img.Width; x++ {
		add(x, 0)
		add(x, img.Height-1)
	}
	for y := 0; y < img.Height; y++ {
		add(0, y)
		add(img.Width-1, y)
	}
	var m [4]byte
	if n == 0 {
		return [4]byte{0xFF, 0xFF, 0xFF, 0xFF}
	}
	for c := range 4 {
		m[c] = byte(sum[c] / n)
	}
	return m
}
