package winclip

import (
	"encoding/binary"
	"image"
	"image/color"
	"slices"
	"testing"
)

func testImage(alpha bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 5, 3)) // odd width: rows need padding
	for y := range 3 {
		for x := range 5 {
			a := uint8(255)
			if alpha {
				a = uint8(40 * (x + 1))
			}
			img.SetNRGBA(x, y, color.NRGBA{uint8(50 * x), uint8(80 * y), 7, a})
		}
	}
	return img
}

func TestDIBRoundTrip(t *testing.T) {
	for _, alpha := range []bool{false, true} {
		src := testImage(alpha)
		data := EncodeDIB(src)
		if bpp := binary.LittleEndian.Uint16(data[14:]); (bpp == 32) != alpha {
			t.Errorf("alpha=%v: encoded at %d bpp", alpha, bpp)
		}
		got, err := DecodeDIB(data)
		if err != nil {
			t.Fatalf("alpha=%v: %v", alpha, err)
		}
		if !slices.Equal(got.Pix, src.Pix) {
			t.Errorf("alpha=%v: pixels differ\n got %v\nwant %v", alpha, got.Pix, src.Pix)
		}
	}
}

// dib32 builds a 2x1 top-down 32-bit DIB whose fourth bytes are zero, the
// way most programs leave them.
func dib32(compression uint32, masks bool) []byte {
	le := binary.LittleEndian
	h := make([]byte, infoHeaderSize)
	le.PutUint32(h[0:], infoHeaderSize)
	le.PutUint32(h[4:], 2)
	le.PutUint32(h[8:], uint32(0xFFFFFFFF)) // height -1: top-down
	le.PutUint16(h[12:], 1)
	le.PutUint16(h[14:], 32)
	le.PutUint32(h[16:], compression)
	if masks {
		h = le.AppendUint32(h, 0x00FF0000)
		h = le.AppendUint32(h, 0x0000FF00)
		h = le.AppendUint32(h, 0x000000FF)
	}
	return append(h, 0x10, 0x20, 0x30, 0, 0xFF, 0xFF, 0xFF, 0)
}

func TestDecodeDIBZeroAlphaIsOpaque(t *testing.T) {
	for _, tc := range []struct {
		name        string
		compression uint32
		masks       bool
	}{{"BI_RGB", biRGB, false}, {"BI_BITFIELDS", biBitfields, true}} {
		img, err := DecodeDIB(dib32(tc.compression, tc.masks))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		want := []byte{0x30, 0x20, 0x10, 255, 255, 255, 255, 255}
		if !slices.Equal(img.Pix, want) {
			t.Errorf("%s: pixels %v, want %v", tc.name, img.Pix, want)
		}
	}
}

func TestDecodeDIBPalette(t *testing.T) {
	le := binary.LittleEndian
	h := make([]byte, infoHeaderSize)
	le.PutUint32(h[0:], infoHeaderSize)
	le.PutUint32(h[4:], 3)
	le.PutUint32(h[8:], 1)
	le.PutUint16(h[12:], 1)
	le.PutUint16(h[14:], 1)                        // 1 bit per pixel
	h = append(h, 0, 0, 0, 0, 0xFF, 0xFF, 0xFF, 0) // black, white
	h = append(h, 0b10100000, 0, 0, 0)             // white black white, padded to 4 bytes
	img, err := DecodeDIB(h)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{255, 255, 255, 255, 0, 0, 0, 255, 255, 255, 255, 255}
	if !slices.Equal(img.Pix, want) {
		t.Errorf("pixels %v, want %v", img.Pix, want)
	}
}

func TestDecodeDIBRejectsGarbage(t *testing.T) {
	for _, data := range [][]byte{nil, make([]byte, 39), dib32(biRGB, false)[:44]} {
		if _, err := DecodeDIB(data); err == nil {
			t.Errorf("DecodeDIB(%d bytes) succeeded", len(data))
		}
	}
}

func TestDropRoundTrip(t *testing.T) {
	paths := []string{`C:\Users\me\报告.docx`, `D:\photos\a b.png`}
	got, err := DecodeDrop(EncodeDrop(paths))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, paths) {
		t.Errorf("got %q, want %q", got, paths)
	}
	if _, err := DecodeDrop([]byte{1, 2, 3}); err == nil {
		t.Error("short data accepted")
	}
}
