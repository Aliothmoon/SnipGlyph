package capture

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/tailscale/win"
	"golang.org/x/sys/windows"
)

// frame is the frozen screen: the original pixels and a dimmed copy, so
// painting is a plain BitBlt from one or the other.
type frame struct {
	w, h int
	orig dib
	dim  dib
	pix  []byte // orig's BGRA pixels, top-down; alpha is garbage
}

// dib is a 32-bit top-down bitmap selected into its own memory DC. Its
// pixels live in a section we own: with GDI's own allocation the process
// kept the memory (32 MB for one 2560x1600 frame) long after DeleteObject.
type dib struct {
	dc      win.HDC
	bmp     win.HBITMAP
	old     win.HGDIOBJ
	section windows.Handle
	pix     []byte
}

// grab copies the virtual screen vs (screen coordinates) into a new frame.
func grab(vs Rect) (*frame, error) {
	screen := win.GetDC(0)
	if screen == 0 {
		return nil, errors.New("GetDC(screen) failed")
	}
	defer win.ReleaseDC(0, screen)

	f := &frame{w: vs.Dx(), h: vs.Dy()}
	if err := f.orig.init(screen, f.w, f.h); err != nil {
		return nil, err
	}
	f.pix = f.orig.pix
	if !win.BitBlt(f.orig.dc, 0, 0, int32(f.w), int32(f.h), screen, int32(vs.Left), int32(vs.Top), win.SRCCOPY|win.CAPTUREBLT) {
		f.close()
		return nil, errors.New("BitBlt(screen) failed")
	}
	if err := f.dim.init(screen, f.w, f.h); err != nil {
		f.close()
		return nil, err
	}
	darken(f.dim.pix, f.pix)
	return f, nil
}

func (d *dib) init(ref win.HDC, w, h int) error {
	size := uint32(w * h * 4)
	section, err := windows.CreateFileMapping(windows.InvalidHandle, nil, windows.PAGE_READWRITE, 0, size, nil)
	if err != nil {
		return fmt.Errorf("CreateFileMapping: %w", err)
	}
	d.section = section
	if d.dc = win.CreateCompatibleDC(ref); d.dc == 0 {
		d.close()
		return errors.New("CreateCompatibleDC failed")
	}
	bih := win.BITMAPINFOHEADER{
		BiWidth:       int32(w),
		BiHeight:      -int32(h), // top-down
		BiPlanes:      1,
		BiBitCount:    32,
		BiCompression: win.BI_RGB,
	}
	bih.BiSize = uint32(unsafe.Sizeof(bih))
	var bits unsafe.Pointer
	d.bmp = win.CreateDIBSection(d.dc, &bih, win.DIB_RGB_COLORS, &bits, win.HANDLE(section), 0)
	if d.bmp == 0 || bits == nil {
		d.close()
		return errors.New("CreateDIBSection failed")
	}
	d.old = win.SelectObject(d.dc, win.HGDIOBJ(d.bmp))
	d.pix = unsafe.Slice((*byte)(bits), int(size))
	return nil
}

func (d *dib) close() {
	if d.old != 0 {
		win.SelectObject(d.dc, d.old)
	}
	if d.bmp != 0 {
		win.DeleteObject(win.HGDIOBJ(d.bmp))
	}
	if d.dc != 0 {
		win.DeleteDC(d.dc)
	}
	if d.section != 0 {
		windows.CloseHandle(d.section)
	}
	*d = dib{}
}

func (f *frame) close() {
	f.dim.close()
	f.orig.close()
	f.pix = nil
}

// crop copies r (frame coordinates) out with opaque alpha.
func (f *frame) crop(r Rect) []byte {
	out := make([]byte, r.Dx()*r.Dy()*4)
	row := r.Dx() * 4
	for y := 0; y < r.Dy(); y++ {
		src := ((r.Top+y)*f.w + r.Left) * 4
		copy(out[y*row:(y+1)*row], f.pix[src:src+row])
	}
	for i := 3; i < len(out); i += 4 {
		out[i] = 0xFF
	}
	return out
}

// darken writes src darkened to about 55% into dst, splitting the work across
// CPUs: a 4K screen is 8M pixels.
func darken(dst, src []byte) {
	const num = 141 // /256
	n := runtime.NumCPU()
	chunk := (len(src)/4/n + 1) * 4
	var wg sync.WaitGroup
	for start := 0; start < len(src); start += chunk {
		end := min(start+chunk, len(src))
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := start; i < end; i += 4 {
				dst[i] = byte(int(src[i]) * num >> 8)
				dst[i+1] = byte(int(src[i+1]) * num >> 8)
				dst[i+2] = byte(int(src[i+2]) * num >> 8)
				dst[i+3] = 0xFF
			}
		}()
	}
	wg.Wait()
}
