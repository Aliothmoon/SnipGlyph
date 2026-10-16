package config

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const secretPrefix = "dpapi:"

// Secret is a credential such as an API key. It is plain text in memory and
// stored encrypted with DPAPI, readable only by the current Windows user.
// A plain value typed into the file by hand is accepted and encrypted on
// the next save.
type Secret string

func (s Secret) MarshalJSON() ([]byte, error) {
	if s == "" {
		return json.Marshal("")
	}
	enc, err := protect([]byte(s))
	if err != nil {
		return nil, err
	}
	return json.Marshal(secretPrefix + base64.StdEncoding.EncodeToString(enc))
}

// UnmarshalJSON never fails on undecryptable data (say, a file copied from
// another account): the secret just comes back empty, so one bad key does
// not throw away the rest of the settings.
func (s *Secret) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	enc, ok := strings.CutPrefix(v, secretPrefix)
	if !ok {
		*s = Secret(v)
		return nil
	}
	*s = ""
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err == nil {
		var plain []byte
		if plain, err = unprotect(raw); err == nil {
			*s = Secret(plain)
			return nil
		}
	}
	slog.Warn("cannot decrypt a stored secret; it must be entered again", "err", err)
	return nil
}

func blob(b []byte) *windows.DataBlob {
	if len(b) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

func takeBlob(out *windows.DataBlob) []byte {
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...)
}

func protect(plain []byte) ([]byte, error) {
	var out windows.DataBlob
	if err := windows.CryptProtectData(blob(plain), nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	return takeBlob(&out), nil
}

func unprotect(enc []byte) ([]byte, error) {
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(blob(enc), nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	return takeBlob(&out), nil
}
