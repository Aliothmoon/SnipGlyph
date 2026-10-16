package cliphist

import (
	"bytes"
	"context"
	"encoding/binary"
	"image/png"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"SnipGlyph/internal/winclip"
)

// Registered formats, looked up once.
var (
	cfHTML          = winclip.RegisterFormat("HTML Format")
	cfRTF           = winclip.RegisterFormat("Rich Text Format")
	cfPNG           = winclip.RegisterFormat("PNG")
	cfDropEffect    = winclip.RegisterFormat("Preferred DropEffect")
	cfExclude       = winclip.RegisterFormat("ExcludeClipboardContentFromMonitorProcessing")
	cfViewerIgnore  = winclip.RegisterFormat("Clipboard Viewer Ignore")
	cfCanInclude    = winclip.RegisterFormat("CanIncludeInClipboardHistory")
	procGetWindowTP = windows.NewLazySystemDLL("user32.dll").NewProc("GetWindowThreadProcessId")
)

// Read takes a snapshot of the clipboard. Pictures are only read when
// there is no text, so a copy from Excel does not drag a bitmap along.
func Read(ctx context.Context) (Snapshot, error) {
	c, err := winclip.Open(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	defer c.Close()
	has := map[uint32]bool{}
	for _, f := range c.Formats() {
		has[f] = true
	}
	var s Snapshot
	if has[cfExclude] || has[cfViewerIgnore] {
		s.Private = true
	} else if has[cfCanInclude] {
		if v, _ := c.Data(cfCanInclude); len(v) >= 4 && binary.LittleEndian.Uint32(v) == 0 {
			s.Private = true
		}
	}
	if s.Private {
		return s, nil
	}
	s.Source = ownerProgram(c.Owner())
	if has[winclip.CFUnicodeText] {
		if s.Text, err = c.Text(); err != nil {
			return s, err
		}
	}
	if has[cfHTML] {
		s.HTML, _ = c.Data(cfHTML)
	}
	if has[cfRTF] {
		s.RTF, _ = c.Data(cfRTF)
	}
	if has[winclip.CFHDrop] {
		if data, _ := c.Data(winclip.CFHDrop); data != nil {
			s.Files, _ = winclip.DecodeDrop(data)
		}
	}
	if s.Text == "" {
		switch {
		case has[cfPNG]:
			s.PNG, err = c.Data(cfPNG)
		case has[winclip.CFDIBV5]:
			s.DIB, err = c.Data(winclip.CFDIBV5)
		case has[winclip.CFDIB]:
			s.DIB, err = c.Data(winclip.CFDIB)
		}
	}
	return s, err
}

// ownerProgram names the executable that owns the clipboard window.
func ownerProgram(hwnd windows.HWND) string {
	if hwnd == 0 {
		return ""
	}
	var pid uint32
	procGetWindowTP.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return ""
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return filepath.Base(windows.UTF16ToString(buf[:n]))
}

// Formats returns the clipboard formats that put it back. Plain leaves out
// HTML and RTF, so the text pastes without formatting.
func Formats(it Item, plain bool) ([]winclip.Item, error) {
	var out []winclip.Item
	add := func(f uint32, data []byte) {
		if len(data) > 0 && f != 0 {
			out = append(out, winclip.Item{Format: f, Data: data})
		}
	}
	switch it.Kind {
	case Text:
		add(winclip.CFUnicodeText, winclip.UTF16(it.Text))
		if !plain {
			add(cfHTML, it.HTML)
			add(cfRTF, it.RTF)
		}
	case Image:
		img, err := png.Decode(bytes.NewReader(it.PNG))
		if err != nil {
			return nil, err
		}
		// PNG for programs that understand transparency; CF_DIB for the
		// rest, from which Windows also makes the other bitmap formats.
		add(cfPNG, it.PNG)
		add(winclip.CFDIB, winclip.EncodeDIB(img))
		if !plain {
			add(cfHTML, it.HTML)
		}
	case Files:
		if !plain {
			add(winclip.CFHDrop, winclip.EncodeDrop(it.Paths()))
			add(cfDropEffect, []byte{1, 0, 0, 0}) // DROPEFFECT_COPY
		}
		add(winclip.CFUnicodeText, winclip.UTF16(strings.Join(it.Paths(), "\r\n")))
	}
	return out, nil
}
