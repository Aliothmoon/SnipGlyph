package appdir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsPortable(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "snipglyph.exe")

	t.Setenv(PortableEnv, "")
	if IsPortable(exe, false) {
		t.Fatal("default should not be portable")
	}
	if !IsPortable(exe, true) {
		t.Fatal("flag should enable portable mode")
	}

	t.Setenv(PortableEnv, "1")
	if !IsPortable(exe, false) {
		t.Fatal("env=1 should enable portable mode")
	}
	t.Setenv(PortableEnv, "false")
	// Marker file is ignored if env explicitly sets false.
	marker := filepath.Join(dir, "portable")
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if IsPortable(exe, false) {
		t.Fatal("env=false should override marker file")
	}

	t.Setenv(PortableEnv, "")
	if !IsPortable(exe, false) {
		t.Fatal("marker file 'portable' should enable portable mode")
	}
	_ = os.Remove(marker)

	txt := filepath.Join(dir, "portable.txt")
	if err := os.WriteFile(txt, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if !IsPortable(exe, false) {
		t.Fatal("marker file 'portable.txt' should enable portable mode")
	}
	_ = os.Remove(txt)

	// A directory named "portable" is not a marker file.
	if err := os.Mkdir(marker, 0o755); err != nil {
		t.Fatal(err)
	}
	if IsPortable(exe, false) {
		t.Fatal("directory named 'portable' should not enable portable mode")
	}
}

func TestResolvePortable(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "snipglyph.exe")

	p, err := Resolve(exe, true)
	if err != nil {
		t.Fatal(err)
	}
	wantData := filepath.Join(dir, "data")
	if !p.Portable {
		t.Fatal("want Portable=true")
	}
	if p.ConfigDir != wantData || p.DataDir != wantData {
		t.Errorf("ConfigDir=%q DataDir=%q, want %q", p.ConfigDir, p.DataDir, wantData)
	}
	if p.LogDir != filepath.Join(wantData, "logs") {
		t.Errorf("LogDir=%q", p.LogDir)
	}
	wantCache := filepath.Join(wantData, "cache", "oneocr")
	if p.OCRCacheDir != wantCache {
		t.Errorf("OCRCacheDir=%q, want %q", p.OCRCacheDir, wantCache)
	}
	if fi, err := os.Stat(wantData); err != nil || !fi.IsDir() {
		t.Errorf("data dir not created: err=%v", err)
	}
}
