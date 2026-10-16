package winclip

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math/bits"
)

// Compression values of BITMAPINFOHEADER.
const (
	biRGB       = 0
	biBitfields = 3
)

const infoHeaderSize = 40 // BITMAPINFOHEADER

var errBadDIB = errors.New("malformed DIB")

// DecodeDIB decodes CF_DIB or CF_DIBV5 data: a bitmap header followed by
// the pixels. It handles 1, 4 and 8 bits per pixel with a palette, and 16,
// 24 and 32 bits uncompressed or with bit fields.
func DecodeDIB(data []byte) (*image.NRGBA, error) {
	if len(data) < infoHeaderSize {
		return nil, errBadDIB
	}
	le := binary.LittleEndian
	hdrSize := int(le.Uint32(data[0:]))
	width := int(int32(le.Uint32(data[4:])))
	height := int(int32(le.Uint32(data[8:])))
	bpp := int(le.Uint16(data[14:]))
	compression := le.Uint32(data[16:])
	clrUsed := int(le.Uint32(data[32:]))
	if hdrSize < infoHeaderSize || hdrSize > len(data) || width <= 0 || height == 0 {
		return nil, errBadDIB
	}
	topDown := height < 0
	if topDown {
		height = -height
	}
	if width > 1<<15 || height > 1<<15 {
		return nil, fmt.Errorf("%w: %dx%d", errBadDIB, width, height)
	}

	// Bit-field masks live in the header from V4 on, and after a plain
	// BITMAPINFOHEADER otherwise.
	var masks [4]uint32 // R, G, B, A
	off := hdrSize
	switch {
	case compression == biBitfields && hdrSize >= 56:
		for i := range masks {
			masks[i] = le.Uint32(data[40+4*i:])
		}
	case compression == biBitfields:
		if len(data) < off+12 {
			return nil, errBadDIB
		}
		for i := range 3 {
			masks[i] = le.Uint32(data[off+4*i:])
		}
		off += 12
	case compression == biRGB && bpp == 16:
		masks = [4]uint32{0x7C00, 0x03E0, 0x001F, 0}
	case compression == biRGB && bpp == 32:
		masks = [4]uint32{0x00FF0000, 0x0000FF00, 0x000000FF, 0xFF000000}
	case compression != biRGB:
		return nil, fmt.Errorf("%w: compression %d", errBadDIB, compression)
	}

	var palette []color.NRGBA
	if bpp <= 8 {
		n := clrUsed
		if n == 0 {
			n = 1 << bpp
		}
		if len(data) < off+4*n {
			return nil, errBadDIB
		}
		for i := range n {
			p := data[off+4*i:]
			palette = append(palette, color.NRGBA{p[2], p[1], p[0], 255})
		}
		off += 4 * n
	} else if compression == biRGB && clrUsed > 0 {
		off += 4 * clrUsed // an optional palette, unused here
	}

	stride := (width*bpp + 31) / 32 * 4
	if bpp != 1 && bpp != 4 && bpp != 8 && bpp != 16 && bpp != 24 && bpp != 32 {
		return nil, fmt.Errorf("%w: %d bits per pixel", errBadDIB, bpp)
	}
	if len(data) < off+stride*height {
		return nil, fmt.Errorf("%w: %d bytes of pixels, want %d", errBadDIB, len(data)-off, stride*height)
	}
	pix := data[off:]

	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	anyAlpha := false
	for y := range height {
		srcY := height - 1 - y
		if topDown {
			srcY = y
		}
		row := pix[srcY*stride:]
		dst := img.Pix[y*img.Stride:]
		for x := range width {
			var c color.NRGBA
			switch bpp {
			case 1, 4, 8:
				bit := x * bpp
				idx := int(row[bit/8]>>(8-bpp-bit%8)) & (1<<bpp - 1)
				if idx < len(palette) {
					c = palette[idx]
				}
			case 24:
				c = color.NRGBA{row[3*x+2], row[3*x+1], row[3*x], 255}
			case 16:
				v := uint32(le.Uint16(row[2*x:]))
				c = color.NRGBA{field(v, masks[0]), field(v, masks[1]), field(v, masks[2]), 255}
			case 32:
				v := le.Uint32(row[4*x:])
				c = color.NRGBA{field(v, masks[0]), field(v, masks[1]), field(v, masks[2]), field(v, masks[3])}
				if c.A != 0 {
					anyAlpha = true
				}
			}
			copy(dst[4*x:], []byte{c.R, c.G, c.B, c.A})
		}
	}
	// Most programs leave the fourth byte of 32-bit pixels zero: then it is
	// padding, not transparency.
	if bpp == 32 && (masks[3] == 0 || !anyAlpha) {
		for i := 3; i < len(img.Pix); i += 4 {
			img.Pix[i] = 255
		}
	}
	return img, nil
}

// field extracts the bits of mask from v, scaled to 0–255.
func field(v, mask uint32) uint8 {
	if mask == 0 {
		return 0
	}
	shift := bits.TrailingZeros32(mask)
	width := bits.OnesCount32(mask)
	x := (v & mask) >> shift
	if width >= 8 {
		return uint8(x >> (width - 8))
	}
	return uint8(x * 255 / (1<<width - 1))
}

// EncodeDIB encodes img as CF_DIB data, bottom-up as most programs expect:
// 24 bits per pixel when it is opaque, else 32 with alpha in the fourth
// byte.
func EncodeDIB(img image.Image) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	opaque := isOpaque(img)
	bpp := 32
	if opaque {
		bpp = 24
	}
	stride := (w*bpp + 31) / 32 * 4
	out := make([]byte, infoHeaderSize+stride*h)
	le := binary.LittleEndian
	le.PutUint32(out[0:], infoHeaderSize)
	le.PutUint32(out[4:], uint32(w))
	le.PutUint32(out[8:], uint32(h))
	le.PutUint16(out[12:], 1)
	le.PutUint16(out[14:], uint16(bpp))
	le.PutUint32(out[16:], biRGB)
	le.PutUint32(out[20:], uint32(stride*h))
	pix := out[infoHeaderSize:]
	for y := range h {
		row := pix[(h-1-y)*stride:]
		for x := range w {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			if opaque {
				row[3*x], row[3*x+1], row[3*x+2] = c.B, c.G, c.R
			} else {
				row[4*x], row[4*x+1], row[4*x+2], row[4*x+3] = c.B, c.G, c.R, c.A
			}
		}
	}
	return out
}

func isOpaque(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return o.Opaque()
	}
	return false
}
