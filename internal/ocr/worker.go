package ocr

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"strings"
)

// Worker protocol, over the worker's stdin/stdout. Every frame is a
// little-endian uint32 payload length followed by the payload.
//
//	worker -> client  hello      JSON, once, after the engine is ready
//	client -> worker  request    uint32 width, uint32 height, BGRA pixels
//	worker -> client  response   JSON
//
// The worker exits when its stdin closes.

const maxFrame = 1 << 30

type hello struct {
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

type response struct {
	Result Result `json:"result"`
	Error  string `json:"error,omitempty"`
}

//go:embed testdata/selftest.png
var selfTestPNG []byte

// ServeWorker loads OneOCR (staging its DLLs under cacheDir), checks it on a
// built-in image and then answers requests from r on w until r is closed.
func ServeWorker(r io.Reader, w io.Writer, cacheDir string) error {
	inst, err := Locate()
	var e *Engine
	if err == nil {
		e, err = OpenEngine(inst, cacheDir)
	}
	if err == nil {
		if err = selfTest(e); err != nil {
			e.Close()
		}
	}
	if err != nil {
		_ = writeJSON(w, hello{Error: err.Error()})
		return err
	}
	defer e.Close()
	if err := writeJSON(w, hello{Version: inst.Version}); err != nil {
		return err
	}

	br := bufio.NewReader(r)
	for {
		frame, err := readFrame(br)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		var resp response
		img, err := decodeRequest(frame)
		if err == nil {
			resp.Result, err = e.Recognize(img)
		}
		if err != nil {
			resp.Error = err.Error()
		}
		if err := writeJSON(w, resp); err != nil {
			return err
		}
	}
}

// selfTest catches a Snipping Tool update that loads fine but no longer
// behaves the way this package expects.
func selfTest(e *Engine) error {
	src, err := png.Decode(bytes.NewReader(selfTestPNG))
	if err != nil {
		return err
	}
	res, err := e.Recognize(FromImage(src))
	if err != nil {
		return fmt.Errorf("OneOCR 自检失败：%w", err)
	}
	if !strings.Contains(res.Text(), "SnipGlyph") {
		return fmt.Errorf("OneOCR 自检失败：识别结果为 %q", res.Text())
	}
	return nil
}

func writeRequest(w io.Writer, img Image) error {
	var hdr [12]byte
	binary.LittleEndian.PutUint32(hdr[0:], uint32(8+len(img.Pix)))
	binary.LittleEndian.PutUint32(hdr[4:], uint32(img.Width))
	binary.LittleEndian.PutUint32(hdr[8:], uint32(img.Height))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(img.Pix)
	return err
}

func decodeRequest(p []byte) (Image, error) {
	if len(p) < 8 {
		return Image{}, errors.New("short request")
	}
	img := Image{
		Width:  int(binary.LittleEndian.Uint32(p[0:])),
		Height: int(binary.LittleEndian.Uint32(p[4:])),
		Pix:    p[8:],
	}
	if img.Width*img.Height*4 != len(img.Pix) {
		return Image{}, fmt.Errorf("request %dx%d carries %d bytes", img.Width, img.Height, len(img.Pix))
	}
	return img, nil
}

func writeJSON(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	buf := make([]byte, 4+len(data))
	binary.LittleEndian.PutUint32(buf, uint32(len(data)))
	copy(buf[4:], data)
	_, err = w.Write(buf)
	return err
}

func readJSON(r io.Reader, v any) error {
	p, err := readFrame(r)
	if err != nil {
		return err
	}
	return json.Unmarshal(p, v)
}

func readFrame(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err // io.EOF only when the stream ends between frames
	}
	n := binary.LittleEndian.Uint32(hdr[:])
	if n > maxFrame {
		return nil, fmt.Errorf("frame of %d bytes exceeds limit", n)
	}
	p := make([]byte, n)
	if _, err := io.ReadFull(r, p); err != nil {
		return nil, fmt.Errorf("read frame: %w", io.ErrUnexpectedEOF)
	}
	return p, nil
}
