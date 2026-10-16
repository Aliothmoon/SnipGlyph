package winclip

import (
	"encoding/binary"
	"errors"
	"unicode/utf16"
)

const dropFilesSize = 20 // DROPFILES

// DecodeDrop returns the paths in CF_HDROP data (a DROPFILES header and a
// double-NUL-terminated list).
func DecodeDrop(data []byte) ([]string, error) {
	if len(data) < dropFilesSize {
		return nil, errors.New("malformed CF_HDROP")
	}
	le := binary.LittleEndian
	off := int(le.Uint32(data[0:]))
	wide := le.Uint32(data[16:]) != 0
	if off < dropFilesSize || off > len(data) {
		return nil, errors.New("malformed CF_HDROP")
	}
	if !wide {
		return nil, errors.New("ANSI CF_HDROP is not supported")
	}
	var paths []string
	var cur []uint16
	for i := off; i+1 < len(data); i += 2 {
		c := le.Uint16(data[i:])
		if c != 0 {
			cur = append(cur, c)
			continue
		}
		if len(cur) == 0 {
			break // the second NUL ends the list
		}
		paths = append(paths, string(utf16.Decode(cur)))
		cur = cur[:0]
	}
	return paths, nil
}

// EncodeDrop builds CF_HDROP data for paths.
func EncodeDrop(paths []string) []byte {
	out := make([]byte, dropFilesSize, dropFilesSize+64*len(paths))
	le := binary.LittleEndian
	le.PutUint32(out[0:], dropFilesSize)
	le.PutUint32(out[16:], 1) // fWide
	for _, p := range paths {
		for _, c := range utf16.Encode([]rune(p)) {
			out = le.AppendUint16(out, c)
		}
		out = le.AppendUint16(out, 0)
	}
	return le.AppendUint16(out, 0)
}
